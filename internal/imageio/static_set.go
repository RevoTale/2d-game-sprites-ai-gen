package imageio

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"sort"
)

const canvasStaticSetTransparentPerimeter = 2

// StaticSetScaleCalibrationVersion identifies the deterministic shared-scale
// transform used for coupled static-set parts.
const StaticSetScaleCalibrationVersion = 3

// StaticSetPart declares one recovered static-set crop and its production
// canvas. A normalizer decides which compatible parts share a scale.
type StaticSetPart struct {
	ID           string
	SourcePath   string
	OutputPath   string
	Size         image.Point
	Registration SubjectRegistrationMode
}

// StaticSetScaleCalibration records the most restrictive transform plus exact
// per-canvas-class transforms. An empty limiter means every source fit at 1x.
type StaticSetScaleCalibration struct {
	Version        int                   `json:"version"`
	Scale          float64               `json:"scale"`
	Numerator      int                   `json:"numerator"`
	Denominator    int                   `json:"denominator"`
	LimitingPartID string                `json:"limitingPartId,omitempty"`
	LimitingAxis   string                `json:"limitingAxis,omitempty"`
	Groups         []StaticSetScaleGroup `json:"groups,omitempty"`
}

// StaticSetScaleGroup records the shared transform for one rotation-neutral
// target-canvas class. Parts with different authored world sizes must not make
// one another artificially small.
type StaticSetScaleGroup struct {
	CanvasClass    image.Point `json:"canvasClass"`
	Scale          float64     `json:"scale"`
	Numerator      int         `json:"numerator"`
	Denominator    int         `json:"denominator"`
	LimitingPartID string      `json:"limitingPartId,omitempty"`
	LimitingAxis   string      `json:"limitingAxis,omitempty"`
	PartIDs        []string    `json:"partIds"`
}

func (calibration StaticSetScaleCalibration) ScaleForPart(partID string) float64 {
	for _, group := range calibration.Groups {
		for _, candidate := range group.PartIDs {
			if candidate == partID {
				return group.Scale
			}
		}
	}
	return calibration.Scale
}

type measuredStaticSetPart struct {
	spec        StaticSetPart
	foreground  *image.NRGBA
	bounds      image.Rectangle
	safe        image.Rectangle
	destination image.Rectangle
}

// WriteSharedScaleTransparentStaticSet measures every recovered crop before
// writing any output, selects one non-upscaling transform, and applies that
// transform to the complete set. Per-part fitting is intentionally forbidden.
func WriteSharedScaleTransparentStaticSet(
	parts []StaticSetPart,
	locked []PaletteColor,
) (StaticSetScaleCalibration, error) {
	calibration := StaticSetScaleCalibration{
		Version:     StaticSetScaleCalibrationVersion,
		Scale:       1,
		Numerator:   1,
		Denominator: 1,
	}
	if len(parts) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("static set requires at least one part")
	}
	if len(locked) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("static set requires a shared locked palette")
	}
	measured := make([]measuredStaticSetPart, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item, err := measureStaticSetPart(part)
		if err != nil {
			return StaticSetScaleCalibration{}, err
		}
		if _, exists := seen[part.ID]; exists {
			return StaticSetScaleCalibration{}, fmt.Errorf("duplicate static set part id %q", part.ID)
		}
		seen[part.ID] = struct{}{}
		measured = append(measured, item)
		considerStaticSetLimit(
			&calibration,
			part.ID,
			"width",
			item.safe.Dx(),
			item.bounds.Dx(),
		)
		considerStaticSetLimit(
			&calibration,
			part.ID,
			"height",
			item.safe.Dy(),
			item.bounds.Dy(),
		)
	}
	calibration.Scale = float64(calibration.Numerator) / float64(calibration.Denominator)
	for index := range measured {
		destination, err := staticSetDestination(measured[index], calibration)
		if err != nil {
			return StaticSetScaleCalibration{}, err
		}
		measured[index].destination = destination
	}
	if err := writeMeasuredStaticSet(measured, locked); err != nil {
		return StaticSetScaleCalibration{}, err
	}
	return calibration, nil
}

// WriteCanvasClassScaleTransparentStaticSet keeps a shared scale among parts
// with the same authored canvas dimensions, treating rotated dimensions as the
// same class. This preserves joinable segments while preventing a narrow root
// or trim piece from shrinking large trees in a heterogeneous visual set.
func WriteCanvasClassScaleTransparentStaticSet(
	parts []StaticSetPart,
	locked []PaletteColor,
) (StaticSetScaleCalibration, error) {
	if len(parts) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("static set requires at least one part")
	}
	groups := make(map[image.Point][]StaticSetPart)
	for _, part := range parts {
		key := part.Size
		if key.X > key.Y {
			key.X, key.Y = key.Y, key.X
		}
		clone := part
		clone.OutputPath += ".canvas-class.tmp"
		groups[key] = append(groups[key], clone)
	}
	keys := make([]image.Point, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].X != keys[j].X {
			return keys[i].X < keys[j].X
		}
		return keys[i].Y < keys[j].Y
	})
	calibration := StaticSetScaleCalibration{
		Version:     StaticSetScaleCalibrationVersion,
		Scale:       1,
		Numerator:   1,
		Denominator: 1,
	}
	temporaryPaths := make([]string, 0, len(parts))
	for _, key := range keys {
		classParts := groups[key]
		classCalibration, err := WriteSharedScaleTransparentStaticSet(classParts, locked)
		if err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			return StaticSetScaleCalibration{}, err
		}
		group := StaticSetScaleGroup{
			CanvasClass: key,
			Scale:       classCalibration.Scale, Numerator: classCalibration.Numerator,
			Denominator:    classCalibration.Denominator,
			LimitingPartID: classCalibration.LimitingPartID,
			LimitingAxis:   classCalibration.LimitingAxis,
			PartIDs:        make([]string, 0, len(classParts)),
		}
		for _, part := range classParts {
			group.PartIDs = append(group.PartIDs, part.ID)
			temporaryPaths = append(temporaryPaths, part.OutputPath)
		}
		sort.Strings(group.PartIDs)
		calibration.Groups = append(calibration.Groups, group)
		if group.Scale < calibration.Scale {
			calibration.Scale = group.Scale
			calibration.Numerator = group.Numerator
			calibration.Denominator = group.Denominator
			calibration.LimitingPartID = group.LimitingPartID
			calibration.LimitingAxis = group.LimitingAxis
		}
	}
	for _, part := range parts {
		temporaryPath := part.OutputPath + ".canvas-class.tmp"
		if err := os.Rename(temporaryPath, part.OutputPath); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			return StaticSetScaleCalibration{}, fmt.Errorf(
				"publish canvas-class static set part %q: %w",
				part.ID,
				err,
			)
		}
	}
	return calibration, nil
}

// WriteCanvasRegisteredTransparentStaticSet preserves each provider cell as a
// complete transparent canvas. It is for composable overlays whose pixel
// coordinates carry meaning; alpha-bounds fitting would destroy their joins.
func WriteCanvasRegisteredTransparentStaticSet(
	boardPath string,
	layout SemanticLayout,
	providerSizes []image.Point,
	parts []StaticSetPart,
	locked []PaletteColor,
) (StaticSetScaleCalibration, error) {
	if len(parts) == 0 || len(parts) != len(providerSizes) {
		return StaticSetScaleCalibration{}, fmt.Errorf(
			"canvas static set requires one provider size per part",
		)
	}
	if len(locked) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("canvas static set requires a shared locked palette")
	}
	board, err := decodeNRGBA(boardPath)
	if err != nil {
		return StaticSetScaleCalibration{}, fmt.Errorf("decode canvas static set board: %w", err)
	}
	if board.Bounds() != image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight) {
		return StaticSetScaleCalibration{}, fmt.Errorf("canvas static set board dimensions do not match its layout")
	}
	bounds, err := semanticSizedBounds(layout, providerSizes, 0)
	if err != nil {
		return StaticSetScaleCalibration{}, err
	}
	calibration := StaticSetScaleCalibration{Version: StaticSetScaleCalibrationVersion}
	for index, part := range parts {
		if part.Size.X <= 0 || part.Size.Y <= 0 {
			return StaticSetScaleCalibration{}, fmt.Errorf("canvas static set part %q has invalid size", part.ID)
		}
		if index == 0 {
			calibration.Numerator = part.Size.X
			calibration.Denominator = bounds[index].Dx()
			calibration.Scale = float64(calibration.Numerator) / float64(calibration.Denominator)
		}
		xScale := float64(part.Size.X) / float64(bounds[index].Dx())
		yScale := float64(part.Size.Y) / float64(bounds[index].Dy())
		if math.Abs(xScale-calibration.Scale) > 1e-9 || math.Abs(yScale-calibration.Scale) > 1e-9 {
			return StaticSetScaleCalibration{}, fmt.Errorf(
				"canvas static set part %q does not share scale %.6f",
				part.ID,
				calibration.Scale,
			)
		}
	}
	temporaryPaths := make([]string, 0, len(parts))
	for index, part := range parts {
		temporaryPath := part.OutputPath + ".canvas-set.tmp"
		normalized := image.NewNRGBA(image.Rectangle{Max: part.Size})
		areaScale(normalized, normalized.Bounds(), board, bounds[index])
		normalized = applyPalette(normalized, locked)
		if !hasTransparentPerimeter(normalized, canvasStaticSetTransparentPerimeter) {
			removeStaticSetTemporaryPaths(temporaryPaths)
			return StaticSetScaleCalibration{}, fmt.Errorf(
				"canvas static set part %q requires a transparent perimeter",
				part.ID,
			)
		}
		if err := writePNG(temporaryPath, normalized); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			_ = os.Remove(temporaryPath)
			return StaticSetScaleCalibration{}, fmt.Errorf("write canvas static set part %q: %w", part.ID, err)
		}
		temporaryPaths = append(temporaryPaths, temporaryPath)
	}
	for index, part := range parts {
		if err := os.Rename(temporaryPaths[index], part.OutputPath); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths[index:])
			return StaticSetScaleCalibration{}, fmt.Errorf("publish canvas static set part %q: %w", part.ID, err)
		}
	}
	return calibration, nil
}

func hasTransparentPerimeter(source *image.NRGBA, width int) bool {
	if width <= 0 || source.Bounds().Dx() <= width*2 || source.Bounds().Dy() <= width*2 {
		return false
	}
	bounds := source.Bounds()
	for inset := 0; inset < width; inset++ {
		top, bottom := bounds.Min.Y+inset, bounds.Max.Y-1-inset
		left, right := bounds.Min.X+inset, bounds.Max.X-1-inset
		for x := left; x <= right; x++ {
			if source.NRGBAAt(x, top).A != 0 || source.NRGBAAt(x, bottom).A != 0 {
				return false
			}
		}
		for y := top; y <= bottom; y++ {
			if source.NRGBAAt(left, y).A != 0 || source.NRGBAAt(right, y).A != 0 {
				return false
			}
		}
	}
	return true
}

// WriteFullBleedOpaqueStaticSet converts each recovered material rectangle
// into its complete production tile. Provider placement inside the shared
// board is irrelevant, while every output pixel belongs to the material.
func WriteFullBleedOpaqueStaticSet(
	parts []StaticSetPart,
	locked []PaletteColor,
) (StaticSetScaleCalibration, error) {
	if len(parts) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("opaque static set requires at least one part")
	}
	if len(locked) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("opaque static set requires a shared locked palette")
	}
	temporaryPaths := make([]string, 0, len(parts))
	for _, part := range parts {
		measured, err := measureStaticSetPart(part)
		if err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			return StaticSetScaleCalibration{}, err
		}
		materialBounds, err := fullyOpaqueInset(measured.foreground, measured.bounds)
		if err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			return StaticSetScaleCalibration{}, fmt.Errorf("opaque static set part %q: %w", part.ID, err)
		}
		temporaryPath := part.OutputPath + ".opaque-set.tmp"
		normalized := image.NewNRGBA(image.Rectangle{Max: part.Size})
		areaScale(normalized, normalized.Bounds(), measured.foreground, materialBounds)
		normalized = applyPalette(normalized, locked)
		if err := writePNG(temporaryPath, normalized); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			_ = os.Remove(temporaryPath)
			return StaticSetScaleCalibration{}, fmt.Errorf("write opaque static set part %q: %w", part.ID, err)
		}
		temporaryPaths = append(temporaryPaths, temporaryPath)
	}
	for index, part := range parts {
		if err := os.Rename(temporaryPaths[index], part.OutputPath); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths[index:])
			return StaticSetScaleCalibration{}, fmt.Errorf("publish opaque static set part %q: %w", part.ID, err)
		}
	}
	return StaticSetScaleCalibration{
		Version:     StaticSetScaleCalibrationVersion,
		Scale:       1,
		Numerator:   1,
		Denominator: 1,
	}, nil
}

func fullyOpaqueInset(source *image.NRGBA, bounds image.Rectangle) (image.Rectangle, error) {
	for inset := 0; ; inset++ {
		candidate := bounds.Inset(inset)
		if candidate.Empty() {
			return image.Rectangle{}, fmt.Errorf("has no fully opaque interior")
		}
		opaque := true
		for y := candidate.Min.Y; y < candidate.Max.Y && opaque; y++ {
			for x := candidate.Min.X; x < candidate.Max.X; x++ {
				if source.NRGBAAt(x, y).A < 128 {
					opaque = false
					break
				}
			}
		}
		if opaque {
			return candidate, nil
		}
	}
}

func measureStaticSetPart(part StaticSetPart) (measuredStaticSetPart, error) {
	if part.ID == "" {
		return measuredStaticSetPart{}, fmt.Errorf("static set part id is required")
	}
	if part.SourcePath == "" || part.OutputPath == "" {
		return measuredStaticSetPart{}, fmt.Errorf("static set part %q requires source and output paths", part.ID)
	}
	if part.Size.X <= 0 || part.Size.Y <= 0 {
		return measuredStaticSetPart{}, fmt.Errorf(
			"static set part %q target size must be positive, got %dx%d",
			part.ID,
			part.Size.X,
			part.Size.Y,
		)
	}
	if part.Registration != SubjectRegistrationCentered &&
		part.Registration != SubjectRegistrationGrounded {
		return measuredStaticSetPart{}, fmt.Errorf(
			"static set part %q has unsupported registration %q",
			part.ID,
			part.Registration,
		)
	}
	decoded, err := decodeNRGBA(part.SourcePath)
	if err != nil {
		return measuredStaticSetPart{}, fmt.Errorf("decode static set part %q: %w", part.ID, err)
	}
	foreground := image.NewNRGBA(decoded.Bounds())
	draw.Draw(foreground, foreground.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	bounds, err := alphaBounds(foreground)
	if err != nil {
		return measuredStaticSetPart{}, fmt.Errorf("static set part %q foreground: %w", part.ID, err)
	}
	guard := max(1, min(part.Size.X, part.Size.Y)/32)
	safe := image.Rect(guard, guard, part.Size.X-guard, part.Size.Y-guard)
	if safe.Empty() {
		return measuredStaticSetPart{}, fmt.Errorf(
			"static set part %q target %dx%d has no safe foreground rectangle",
			part.ID,
			part.Size.X,
			part.Size.Y,
		)
	}
	return measuredStaticSetPart{
		spec:       part,
		foreground: foreground,
		bounds:     bounds,
		safe:       safe,
	}, nil
}

func considerStaticSetLimit(
	calibration *StaticSetScaleCalibration,
	partID, axis string,
	numerator, denominator int,
) {
	if numerator >= denominator {
		return
	}
	left := int64(numerator) * int64(calibration.Denominator)
	right := int64(calibration.Numerator) * int64(denominator)
	if left >= right {
		return
	}
	calibration.Numerator = numerator
	calibration.Denominator = denominator
	calibration.LimitingPartID = partID
	calibration.LimitingAxis = axis
}

func staticSetDestination(
	part measuredStaticSetPart,
	calibration StaticSetScaleCalibration,
) (image.Rectangle, error) {
	width := int(int64(part.bounds.Dx()) * int64(calibration.Numerator) / int64(calibration.Denominator))
	height := int(int64(part.bounds.Dy()) * int64(calibration.Numerator) / int64(calibration.Denominator))
	if width <= 0 || height <= 0 {
		return image.Rectangle{}, fmt.Errorf(
			"static set part %q becomes empty at shared scale %.6f",
			part.spec.ID,
			calibration.Scale,
		)
	}
	left := part.safe.Min.X + (part.safe.Dx()-width)/2
	top := part.safe.Min.Y + (part.safe.Dy()-height)/2
	if part.spec.Registration == SubjectRegistrationGrounded {
		top = part.safe.Max.Y - height
	}
	destination := image.Rect(left, top, left+width, top+height)
	if !destination.In(part.safe) {
		return image.Rectangle{}, fmt.Errorf(
			"static set part %q destination %v exceeds safe target %v",
			part.spec.ID,
			destination,
			part.safe,
		)
	}
	return destination, nil
}

func writeMeasuredStaticSet(parts []measuredStaticSetPart, locked []PaletteColor) error {
	temporaryPaths := make([]string, 0, len(parts))
	for _, part := range parts {
		temporaryPath := part.spec.OutputPath + ".static-set.tmp"
		normalized := image.NewNRGBA(image.Rectangle{Max: part.spec.Size})
		areaScale(normalized, part.destination, part.foreground, part.bounds)
		normalized = applyPalette(normalized, locked)
		if err := writePNG(temporaryPath, normalized); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("write static set part %q: %w", part.spec.ID, err)
		}
		temporaryPaths = append(temporaryPaths, temporaryPath)
	}
	for index, part := range parts {
		if err := os.Rename(temporaryPaths[index], part.spec.OutputPath); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths[index:])
			return fmt.Errorf("publish static set part %q: %w", part.spec.ID, err)
		}
	}
	return nil
}

func removeStaticSetTemporaryPaths(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
