package imageio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const (
	CanonicalSubjectProfileVersion    = 4
	SemanticScaleCalibrationVersion   = 2
	semanticCalibrationFrameIndex     = 0
	standardHumanoidHeightNumerator   = 15
	standardHumanoidHeightDenominator = 32
	standardHumanoidHeightTolerance   = 0.08
)

// CanonicalScaleClass selects a system-owned neutral scale policy. JSON names
// the semantic class; pixel targets and fitting remain CLI-owned.
type CanonicalScaleClass string

const (
	CanonicalScaleClassStandardHumanoid CanonicalScaleClass = "standard-humanoid"
	CanonicalScaleClassReferenceStable  CanonicalScaleClass = "reference-stable"
)

// SubjectRegistrationMode controls only deterministic subject anchoring.
type SubjectRegistrationMode string

const (
	SubjectRegistrationGrounded SubjectRegistrationMode = "grounded"
	SubjectRegistrationCentered SubjectRegistrationMode = "centered"
)

// CanonicalSubjectProfile is reference-derived scale and anchor evidence for
// one isolated object. Animation extents never contribute to this profile.
type CanonicalSubjectProfile struct {
	Version             int                     `json:"version"`
	Mode                SubjectRegistrationMode `json:"mode"`
	ScaleClass          CanonicalScaleClass     `json:"scaleClass"`
	TargetNeutralHeight int                     `json:"targetNeutralHeight"`
	VisualMass          float64                 `json:"visualMass"`
	RobustBounds        image.Rectangle         `json:"robustBounds"`
	FullBounds          image.Rectangle         `json:"fullBounds"`
	ReferenceMasses     []float64               `json:"referenceMasses"`
	ReferenceHeights    []int                   `json:"referenceHeights"`
	ReferenceHashes     []string                `json:"referenceHashes"`
	ReferenceBounds     []image.Rectangle       `json:"referenceBounds"`
	ReferencePivots     []image.Point           `json:"referencePivots"`
	ReferenceCanvases   []image.Point           `json:"referenceCanvases"`
}

// SemanticScaleCalibration proves how an independent provider board is mapped
// back to the canonical appearance scale. SourceRatios cancel board-local
// coordinate differences; they do not permit visible per-animation resizing.
type SemanticScaleCalibration struct {
	Version               int                       `json:"version"`
	CalibrationFrame      int                       `json:"calibrationFrame"`
	MasterMasses          []float64                 `json:"masterMasses"`
	CalibrationMasses     []float64                 `json:"calibrationMasses"`
	SourceRatios          []float64                 `json:"sourceRatios"`
	DirectionScales       []float64                 `json:"directionScales"`
	DirectionPivotOffsets []image.Point             `json:"directionPivotOffsets"`
	PoseMeasurements      []SemanticPoseMeasurement `json:"poseMeasurements,omitempty"`
}

// SemanticPoseMeasurement records real foreground and occupied-size evidence
// before and after deterministic board-local scale calibration.
type SemanticPoseMeasurement struct {
	Index            int             `json:"index"`
	Direction        int             `json:"direction"`
	Frame            int             `json:"frame"`
	ForegroundPixels int             `json:"foregroundPixels"`
	SourceBounds     image.Rectangle `json:"sourceBounds"`
	SourceWidth      int             `json:"sourceWidth"`
	SourceHeight     int             `json:"sourceHeight"`
	CanonicalWidth   int             `json:"canonicalWidth"`
	CanonicalHeight  int             `json:"canonicalHeight"`
	Scale            float64         `json:"scale"`
}

// BuildCanonicalSubjectProfile derives one object-level profile from the
// configured neutral references without changing their scale.
func BuildCanonicalSubjectProfile(
	paths []string,
	mode SubjectRegistrationMode,
	scaleClass CanonicalScaleClass,
	outputHeight int,
) (CanonicalSubjectProfile, error) {
	if len(paths) == 0 {
		return CanonicalSubjectProfile{}, fmt.Errorf("canonical subject profile requires references")
	}
	if mode != SubjectRegistrationGrounded && mode != SubjectRegistrationCentered {
		return CanonicalSubjectProfile{}, fmt.Errorf("unsupported subject registration mode %q", mode)
	}
	if outputHeight <= 0 {
		return CanonicalSubjectProfile{}, fmt.Errorf("canonical subject profile requires a positive output height")
	}
	switch scaleClass {
	case CanonicalScaleClassStandardHumanoid, CanonicalScaleClassReferenceStable:
	default:
		return CanonicalSubjectProfile{}, fmt.Errorf("unsupported canonical scale class %q", scaleClass)
	}
	profile := CanonicalSubjectProfile{
		Version:           CanonicalSubjectProfileVersion,
		Mode:              mode,
		ScaleClass:        scaleClass,
		ReferenceMasses:   make([]float64, len(paths)),
		ReferenceHeights:  make([]int, len(paths)),
		ReferenceHashes:   make([]string, len(paths)),
		ReferenceBounds:   make([]image.Rectangle, len(paths)),
		ReferencePivots:   make([]image.Point, len(paths)),
		ReferenceCanvases: make([]image.Point, len(paths)),
	}
	relativeBounds := make([]image.Rectangle, len(paths))
	for index, path := range paths {
		mass, bounds, pivot, canvas, hash, err := subjectEvidence(path, mode)
		if err != nil {
			return CanonicalSubjectProfile{}, fmt.Errorf("canonical reference %02d: %w", index, err)
		}
		profile.ReferenceMasses[index] = mass
		profile.ReferenceHeights[index] = bounds.Dy()
		profile.ReferenceHashes[index] = hash
		profile.ReferenceBounds[index] = bounds
		profile.ReferencePivots[index] = pivot
		profile.ReferenceCanvases[index] = canvas
		relativeBounds[index] = bounds.Sub(pivot)
	}
	profile.VisualMass = medianFloat(profile.ReferenceMasses)
	switch scaleClass {
	case CanonicalScaleClassStandardHumanoid:
		profile.TargetNeutralHeight = max(
			1,
			(outputHeight*standardHumanoidHeightNumerator+
				standardHumanoidHeightDenominator/2)/standardHumanoidHeightDenominator,
		)
	case CanonicalScaleClassReferenceStable:
		profile.TargetNeutralHeight = medianInt(profile.ReferenceHeights)
	}
	profile.RobustBounds = medianRectangle(relativeBounds)
	profile.FullBounds = relativeBounds[0]
	for _, bounds := range relativeBounds[1:] {
		profile.FullBounds = profile.FullBounds.Union(bounds)
	}
	return profile, nil
}

// FitCanonicalSubjectTransform matches the generated master to the configured
// neutral references. Dependent animation poses are intentionally absent.
func FitCanonicalSubjectTransform(
	profile CanonicalSubjectProfile,
	masterPaths []string,
	masterPoses []SemanticPose,
	width, height int,
) (SemanticUnitTransform, error) {
	if profile.Version != CanonicalSubjectProfileVersion {
		return SemanticUnitTransform{}, fmt.Errorf("unsupported canonical subject profile v%d", profile.Version)
	}
	if len(masterPaths) == 0 || len(masterPaths) != len(masterPoses) ||
		len(masterPaths) != len(profile.ReferenceMasses) ||
		len(masterPaths) != len(profile.ReferenceCanvases) {
		return SemanticUnitTransform{}, fmt.Errorf(
			"canonical transform requires matching references, master images, and poses",
		)
	}
	ratios := make([]float64, len(masterPaths))
	for index, path := range masterPaths {
		_, bounds, _, _, _, err := subjectEvidence(path, profile.Mode)
		if err != nil {
			return SemanticUnitTransform{}, fmt.Errorf("master direction %02d: %w", index, err)
		}
		if bounds.Dy() <= 0 || profile.TargetNeutralHeight <= 0 {
			return SemanticUnitTransform{}, fmt.Errorf("master direction %02d has invalid neutral height", index)
		}
		ratios[index] = float64(profile.TargetNeutralHeight) / float64(bounds.Dy())
	}
	transform := SemanticUnitTransform{
		Scale:            medianFloat(ratios),
		DirectionAnchors: make([]image.Point, len(profile.ReferencePivots)),
	}
	for index, pivot := range profile.ReferencePivots {
		canvas := profile.ReferenceCanvases[index]
		delta := image.Pt(width-canvas.X, height-canvas.Y)
		if delta.X < 0 || delta.Y < 0 || delta.X%2 != 0 || delta.Y%2 != 0 {
			return SemanticUnitTransform{}, fmt.Errorf(
				"canonical direction %02d reference canvas %v cannot be centered in %dx%d frame",
				index,
				canvas,
				width,
				height,
			)
		}
		transform.DirectionAnchors[index] = pivot.Add(image.Pt(delta.X/2, delta.Y/2))
	}
	switch profile.Mode {
	case SubjectRegistrationGrounded:
		// The robust body pivot from each neutral reference preserves the
		// authored registration for that view.
	case SubjectRegistrationCentered:
		for index, pose := range masterPoses {
			center := image.Pt(
				(pose.Bounds.Min.X+pose.Bounds.Max.X)/2,
				(pose.Bounds.Min.Y+pose.Bounds.Max.Y)/2,
			)
			transform.DirectionAnchors[index] = image.Pt(
				transform.DirectionAnchors[index].X+
					int(math.Round(float64(pose.Pivot.X-center.X)*transform.Scale)),
				transform.DirectionAnchors[index].Y+
					int(math.Round(float64(pose.Pivot.Y-center.Y)*transform.Scale)),
			)
		}
	default:
		return SemanticUnitTransform{}, fmt.Errorf("unsupported subject registration mode %q", profile.Mode)
	}
	safe := image.Rect(0, 0, width, height).Inset(
		CanonicalFrameEdgePadding(width, height),
	)
	neutralHeights := make([]int, 0, len(masterPoses))
	for index, pose := range masterPoses {
		anchor := transform.DirectionAnchors[index]
		if !anchor.In(image.Rect(0, 0, width, height)) {
			return SemanticUnitTransform{}, fmt.Errorf(
				"canonical direction %02d anchor %v is outside %dx%d frame",
				index,
				anchor,
				width,
				height,
			)
		}
		destination := semanticPoseDestination(
			pose,
			transform.Scale,
			anchor,
		)
		if profile.ScaleClass == CanonicalScaleClassStandardHumanoid {
			neutralHeights = append(neutralHeights, destination.Dy())
		}
		if destination.Dx() > safe.Dx() || destination.Dy() > safe.Dy() {
			return SemanticUnitTransform{}, fmt.Errorf(
				"%w: master direction %02d extent %v is larger than safe canonical frame %v",
				ErrProductionFrameClipping,
				index,
				destination,
				safe,
			)
		}
	}
	if profile.ScaleClass == CanonicalScaleClassStandardHumanoid {
		medianHeight, withinTolerance := medianHeightWithinTolerance(
			neutralHeights,
			profile.TargetNeutralHeight,
			standardHumanoidHeightTolerance,
		)
		if !withinTolerance {
			delta := float64(absInt(medianHeight-profile.TargetNeutralHeight)) /
				float64(profile.TargetNeutralHeight)
			return SemanticUnitTransform{}, fmt.Errorf(
				"master median neutral height %d differs from system target %d by %.1f%%",
				medianHeight,
				profile.TargetNeutralHeight,
				delta*100,
			)
		}
	}
	return transform, nil
}

// withinPixelTolerance rounds the fractional policy allowance outward to the
// nearest whole pixel. Measurements are integer pixel bounds, so rejecting the
// single boundary pixel would make an exact percentage policy depend on
// unrepresentable fractions.
func withinPixelTolerance(actual, target int, tolerance float64) bool {
	if target <= 0 {
		return false
	}
	allowedDelta := int(math.Ceil(float64(target) * tolerance))
	return absInt(actual-target) <= allowedDelta
}

func medianHeightWithinTolerance(heights []int, target int, tolerance float64) (int, bool) {
	if len(heights) == 0 {
		return 0, false
	}
	medianHeight := medianInt(append([]int(nil), heights...))
	return medianHeight, withinPixelTolerance(medianHeight, target, tolerance)
}

// CalibrateSemanticPoseSet derives one source-coordinate correction per
// direction from the neutral/ready frame-00 pose. Every other frame in the
// same animation direction inherits that exact scale.
func CalibrateSemanticPoseSet(
	masterPaths []string,
	calibrationPaths []string,
	mode SubjectRegistrationMode,
	canonicalScale float64,
) (SemanticScaleCalibration, error) {
	if canonicalScale <= 0 ||
		len(masterPaths) == 0 ||
		len(masterPaths) != len(calibrationPaths) {
		return SemanticScaleCalibration{}, fmt.Errorf(
			"semantic scale calibration requires a positive canonical scale and matching directions",
		)
	}
	calibration := SemanticScaleCalibration{
		Version:           SemanticScaleCalibrationVersion,
		CalibrationFrame:  semanticCalibrationFrameIndex,
		MasterMasses:      make([]float64, len(masterPaths)),
		CalibrationMasses: make([]float64, len(masterPaths)),
		SourceRatios:      make([]float64, len(masterPaths)),
		DirectionScales:   make([]float64, len(masterPaths)),
	}
	for directionIndex := range masterPaths {
		masterMass, _, _, _, _, err := subjectEvidence(
			masterPaths[directionIndex],
			mode,
		)
		if err != nil {
			return SemanticScaleCalibration{}, fmt.Errorf(
				"master direction %02d: %w",
				directionIndex,
				err,
			)
		}
		calibrationMass, _, _, _, _, err := subjectEvidence(
			calibrationPaths[directionIndex],
			mode,
		)
		if err != nil {
			return SemanticScaleCalibration{}, fmt.Errorf(
				"calibration direction %02d frame %02d: %w",
				directionIndex,
				semanticCalibrationFrameIndex,
				err,
			)
		}
		if masterMass <= 0 || calibrationMass <= 0 {
			return SemanticScaleCalibration{}, fmt.Errorf(
				"semantic scale calibration direction %02d has invalid visual mass",
				directionIndex,
			)
		}
		ratio := math.Sqrt(masterMass / calibrationMass)
		calibration.MasterMasses[directionIndex] = masterMass
		calibration.CalibrationMasses[directionIndex] = calibrationMass
		calibration.SourceRatios[directionIndex] = ratio
		calibration.DirectionScales[directionIndex] = canonicalScale * ratio
	}
	return calibration, nil
}

// MeasureSemanticPoses records occupied size for every pose using the one
// calibrated source scale shared by its animation direction.
func MeasureSemanticPoses(
	posePaths []string,
	poses []SemanticPose,
	directionScales []float64,
	framesPerDirection int,
	mode SubjectRegistrationMode,
) ([]SemanticPoseMeasurement, error) {
	if len(posePaths) == 0 ||
		len(posePaths) != len(poses) ||
		framesPerDirection <= 0 ||
		len(poses) != len(directionScales)*framesPerDirection {
		return nil, fmt.Errorf(
			"semantic pose measurement requires matching poses, direction scales, and frames",
		)
	}
	measurements := make([]SemanticPoseMeasurement, len(poses))
	for index, path := range posePaths {
		mass, _, _, _, _, err := subjectEvidence(path, mode)
		if err != nil {
			return nil, fmt.Errorf("measure semantic pose %02d: %w", index, err)
		}
		direction := index / framesPerDirection
		scale := directionScales[direction]
		if scale <= 0 {
			return nil, fmt.Errorf(
				"measure semantic pose %02d has non-positive direction scale",
				index,
			)
		}
		bounds := poses[index].Bounds
		measurements[index] = SemanticPoseMeasurement{
			Index: index, Direction: direction,
			Frame:            index % framesPerDirection,
			ForegroundPixels: int(mass),
			SourceBounds:     bounds,
			SourceWidth:      bounds.Dx(), SourceHeight: bounds.Dy(),
			CanonicalWidth: max(
				1,
				int(math.Round(float64(bounds.Dx())*scale)),
			),
			CanonicalHeight: max(
				1,
				int(math.Round(float64(bounds.Dy())*scale)),
			),
			Scale: scale,
		}
	}
	return measurements, nil
}

// WriteCanonicalSubjectProfile writes auditable run evidence.
func WriteCanonicalSubjectProfile(path string, profile CanonicalSubjectProfile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// WriteSemanticScaleCalibration writes auditable board-local scale evidence.
func WriteSemanticScaleCalibration(
	path string,
	calibration SemanticScaleCalibration,
) error {
	if calibration.Version != SemanticScaleCalibrationVersion {
		return fmt.Errorf(
			"unsupported semantic scale calibration v%d",
			calibration.Version,
		)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(calibration, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// WriteCanonicalSubjectProfileOverlay draws only review evidence; it never
// changes a deployable frame or feeds the provider.
func WriteCanonicalSubjectProfileOverlay(
	paths []string,
	outputPath string,
	profile CanonicalSubjectProfile,
) error {
	if len(paths) != len(profile.ReferenceBounds) || len(paths) != len(profile.ReferencePivots) {
		return fmt.Errorf("canonical profile overlay requires matching reference evidence")
	}
	sources := make([]*image.NRGBA, len(paths))
	width, height := 0, 0
	for index, path := range paths {
		source, err := decodeNRGBA(path)
		if err != nil {
			return err
		}
		sources[index] = source
		width = max(width, source.Bounds().Dx())
		height = max(height, source.Bounds().Dy())
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, width*len(sources), height))
	for index, source := range sources {
		offset := image.Pt(index*width, 0)
		draw.Draw(canvas, source.Bounds().Add(offset), source, source.Bounds().Min, draw.Over)
		drawProfileRectangle(canvas, profile.ReferenceBounds[index].Add(offset))
		pivot := profile.ReferencePivots[index].Add(offset)
		for delta := -5; delta <= 5; delta++ {
			setOverlayPixel(canvas, pivot.X+delta, pivot.Y, color.NRGBA{G: 255, B: 255, A: 255})
			setOverlayPixel(canvas, pivot.X, pivot.Y+delta, color.NRGBA{G: 255, B: 255, A: 255})
		}
	}
	return writePNG(outputPath, canvas)
}

func drawProfileRectangle(target *image.NRGBA, bounds image.Rectangle) {
	value := color.NRGBA{R: 255, G: 64, A: 255}
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		setOverlayPixel(target, x, bounds.Min.Y, value)
		setOverlayPixel(target, x, bounds.Max.Y-1, value)
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		setOverlayPixel(target, bounds.Min.X, y, value)
		setOverlayPixel(target, bounds.Max.X-1, y, value)
	}
}

func setOverlayPixel(target *image.NRGBA, x, y int, value color.NRGBA) {
	if image.Pt(x, y).In(target.Bounds()) {
		target.SetNRGBA(x, y, value)
	}
}

func subjectEvidence(
	path string,
	mode SubjectRegistrationMode,
) (float64, image.Rectangle, image.Point, image.Point, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, image.Rectangle{}, image.Point{}, image.Point{}, "", err
	}
	source, err := decodeNRGBA(path)
	if err != nil {
		return 0, image.Rectangle{}, image.Point{}, image.Point{}, "", err
	}
	var points []image.Point
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			if source.NRGBAAt(x, y).A >= 128 {
				points = append(points, image.Pt(x, y))
			}
		}
	}
	if len(points) == 0 {
		return 0, image.Rectangle{}, image.Point{}, image.Point{}, "", fmt.Errorf("reference has no subject foreground")
	}
	bounds := pointsBounds(points)
	pivot := robustComponentPivot(points)
	if mode == SubjectRegistrationCentered {
		pivot = image.Pt((bounds.Min.X+bounds.Max.X)/2, (bounds.Min.Y+bounds.Max.Y)/2)
	}
	sum := sha256.Sum256(data)
	return float64(len(points)), bounds, pivot, source.Bounds().Size(), hex.EncodeToString(sum[:]), nil
}

func pointsBounds(points []image.Point) image.Rectangle {
	bounds := image.Rect(points[0].X, points[0].Y, points[0].X+1, points[0].Y+1)
	for _, point := range points[1:] {
		bounds = bounds.Union(image.Rect(point.X, point.Y, point.X+1, point.Y+1))
	}
	return bounds
}

func medianFloat(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}
	return sorted[middle]
}

func medianRectangle(values []image.Rectangle) image.Rectangle {
	minX := make([]int, len(values))
	minY := make([]int, len(values))
	maxX := make([]int, len(values))
	maxY := make([]int, len(values))
	for index, value := range values {
		minX[index] = value.Min.X
		minY[index] = value.Min.Y
		maxX[index] = value.Max.X
		maxY[index] = value.Max.Y
	}
	return image.Rect(medianInt(minX), medianInt(minY), medianInt(maxX), medianInt(maxY))
}

func medianInt(values []int) int {
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}
