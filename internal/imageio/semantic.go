package imageio

import (
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
	semanticAnchorSpacing = 384
	semanticMinimumCanvas = 1024
	semanticCanvasGuard   = 8
)

// SemanticLayout describes ordered pose anchors. Anchors establish semantic
// order only; they are not clipping or validation boundaries.
type SemanticLayout struct {
	CanvasWidth   int           `json:"canvasWidth"`
	CanvasHeight  int           `json:"canvasHeight"`
	Columns       int           `json:"columns"`
	Rows          int           `json:"rows"`
	AnchorSpacing int           `json:"anchorSpacing"`
	Anchors       []image.Point `json:"anchors"`
}

func (layout SemanticLayout) Canvas() image.Point {
	return image.Pt(layout.CanvasWidth, layout.CanvasHeight)
}

// SemanticAnimationLayout returns direction-major row anchors for one complete
// animation family.
func SemanticAnimationLayout(directions, frames int) (SemanticLayout, error) {
	return semanticLayout(directions*frames, frames)
}

// SemanticMasterLayout returns a deterministic near-square direction layout.
func SemanticMasterLayout(directions int) (SemanticLayout, error) {
	if directions <= 0 {
		return SemanticLayout{}, fmt.Errorf("semantic master requires at least one direction")
	}
	columns := int(math.Ceil(math.Sqrt(float64(directions))))
	return semanticLayout(directions, columns)
}

func semanticLayout(count, columns int) (SemanticLayout, error) {
	if count <= 0 || columns <= 0 || columns > count {
		return SemanticLayout{}, fmt.Errorf("invalid semantic layout count=%d columns=%d", count, columns)
	}
	rows := (count + columns - 1) / columns
	width := roundUp(max(semanticMinimumCanvas, columns*semanticAnchorSpacing), 16)
	height := roundUp(max(semanticMinimumCanvas, rows*semanticAnchorSpacing), 16)
	gridWidth := columns * semanticAnchorSpacing
	gridHeight := rows * semanticAnchorSpacing
	left := (width-gridWidth)/2 + semanticAnchorSpacing/2
	top := (height-gridHeight)/2 + semanticAnchorSpacing/2
	layout := SemanticLayout{
		CanvasWidth: width, CanvasHeight: height,
		Columns: columns, Rows: rows, AnchorSpacing: semanticAnchorSpacing,
		Anchors: make([]image.Point, count),
	}
	for index := range layout.Anchors {
		layout.Anchors[index] = image.Pt(
			left+(index%columns)*semanticAnchorSpacing,
			top+(index/columns)*semanticAnchorSpacing,
		)
	}
	return layout, nil
}

// WriteSemanticBoard places complete sources around logical anchors using one
// shared scale and baseline. The layout does not create cells or guides.
func WriteSemanticBoard(
	paths []string,
	outputPath string,
	layout SemanticLayout,
	maximumExtent int,
) error {
	return writeSemanticBoard(paths, outputPath, layout, maximumExtent, true)
}

// WriteSemanticBoardAtNativeScale preserves source scale unless a shared
// reduction is required to retain real outer-canvas reserve.
func WriteSemanticBoardAtNativeScale(
	paths []string,
	outputPath string,
	layout SemanticLayout,
	maximumExtent int,
) error {
	return writeSemanticBoard(paths, outputPath, layout, maximumExtent, false)
}

func writeSemanticBoard(
	paths []string,
	outputPath string,
	layout SemanticLayout,
	maximumExtent int,
	allowUpscale bool,
) error {
	if len(paths) != len(layout.Anchors) || maximumExtent <= 0 {
		return fmt.Errorf(
			"semantic board has %d sources for %d anchors and extent %d",
			len(paths),
			len(layout.Anchors),
			maximumExtent,
		)
	}
	sources := make([]*image.NRGBA, len(paths))
	bounds := make([]image.Rectangle, len(paths))
	pivots := make([]image.Point, len(paths))
	maximumWidth, maximumHeight := 0, 0
	for index, path := range paths {
		source, err := decodeNRGBA(path)
		if err != nil {
			return fmt.Errorf("decode semantic source %q: %w", path, err)
		}
		foreground, err := alphaBounds(source)
		if err != nil {
			return fmt.Errorf("semantic source %q: %w", path, err)
		}
		components := semanticComponents(source)
		var points []image.Point
		for _, component := range components {
			points = append(points, component.points...)
		}
		sources[index] = source
		bounds[index] = foreground
		pivots[index] = robustComponentPivot(points)
		maximumWidth = max(maximumWidth, foreground.Dx())
		maximumHeight = max(maximumHeight, foreground.Dy())
	}
	scale := min(
		float64(maximumExtent)/float64(maximumWidth),
		float64(maximumExtent)/float64(maximumHeight),
	)
	if !allowUpscale {
		scale = min(1, scale)
	}
	board := image.NewNRGBA(
		image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight),
	)
	safe := board.Bounds().Inset(semanticCanvasGuard)
	scale, err := fitSemanticBoardScale(
		bounds,
		pivots,
		layout,
		maximumExtent,
		scale,
		safe,
	)
	if err != nil {
		return err
	}
	for index, source := range sources {
		destination := semanticDestination(
			bounds[index],
			pivots[index],
			layout.Anchors[index],
			maximumExtent,
			scale,
		)
		if !destination.In(safe) {
			return fmt.Errorf(
				"semantic source %02d destination %v exceeds safe canvas %v",
				index,
				destination,
				safe,
			)
		}
		areaScale(board, destination, source, bounds[index])
	}
	return writePNG(outputPath, board)
}

func fitSemanticBoardScale(
	bounds []image.Rectangle,
	pivots []image.Point,
	layout SemanticLayout,
	maximumExtent int,
	scale float64,
	safe image.Rectangle,
) (float64, error) {
	fits := func(candidate float64) bool {
		for index := range bounds {
			destination := semanticDestination(
				bounds[index],
				pivots[index],
				layout.Anchors[index],
				maximumExtent,
				candidate,
			)
			if !destination.In(safe) {
				return false
			}
		}
		return true
	}
	if fits(scale) {
		return scale, nil
	}
	if !fits(0) {
		return 0, fmt.Errorf("semantic anchors cannot fit inside safe canvas %v", safe)
	}

	low, high := 0.0, scale
	for range 64 {
		middle := (low + high) / 2
		if fits(middle) {
			low = middle
		} else {
			high = middle
		}
	}
	return low, nil
}

func semanticDestination(
	bounds image.Rectangle,
	pivot image.Point,
	anchor image.Point,
	maximumExtent int,
	scale float64,
) image.Rectangle {
	left := anchor.X + int(math.Round(
		float64(bounds.Min.X-pivot.X)*scale,
	))
	top := anchor.Y + maximumExtent/3 + int(math.Round(
		float64(bounds.Min.Y-pivot.Y)*scale,
	))
	width := max(1, int(math.Round(float64(bounds.Dx())*scale)))
	height := max(1, int(math.Round(float64(bounds.Dy())*scale)))
	return image.Rect(left, top, left+width, top+height)
}

// SemanticPose records a complete pose and its primary body core. Bounds and
// pivots use source-board coordinates so every ownership decision is auditable.
type SemanticPose struct {
	Index      int               `json:"index"`
	Anchor     image.Point       `json:"anchor"`
	CoreBounds image.Rectangle   `json:"coreBounds"`
	Bounds     image.Rectangle   `json:"bounds"`
	Pivot      image.Point       `json:"pivot"`
	Components []image.Rectangle `json:"components"`
	Path       string            `json:"path,omitempty"`
}

type semanticComponent struct {
	points []image.Point
	bounds image.Rectangle
	pivot  image.Point
	area   int
}

// RecoverSemanticPoses proves component ownership before cropping complete
// groups. A nil outputPaths validates and records ownership without writing.
func RecoverSemanticPoses(
	boardPath string,
	layout SemanticLayout,
	outputPaths []string,
) ([]SemanticPose, error) {
	if len(layout.Anchors) == 0 {
		return nil, fmt.Errorf("semantic recovery requires expected anchors")
	}
	if outputPaths != nil && len(outputPaths) != len(layout.Anchors) {
		return nil, fmt.Errorf(
			"semantic recovery has %d outputs for %d poses",
			len(outputPaths),
			len(layout.Anchors),
		)
	}
	board, err := decodeNRGBA(boardPath)
	if err != nil {
		return nil, err
	}
	expected := image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight)
	if board.Bounds() != expected {
		return nil, fmt.Errorf(
			"semantic board is %dx%d, expected %dx%d",
			board.Bounds().Dx(),
			board.Bounds().Dy(),
			expected.Dx(),
			expected.Dy(),
		)
	}
	if guardOccupied(board, semanticCanvasGuard) {
		return nil, fmt.Errorf("semantic foreground touches the outer canvas safety edge")
	}
	components := semanticComponents(board)
	expectedCount := len(layout.Anchors)
	if len(components) < expectedCount {
		return nil, fmt.Errorf(
			"semantic board has %d primary body cores, expected %d",
			len(components),
			expectedCount,
		)
	}
	groups, err := assignComponentsToAnchors(components, layout)
	if err != nil {
		return nil, err
	}
	poses := make([]SemanticPose, len(groups))
	for index, group := range groups {
		pose := SemanticPose{
			Index: index, Anchor: layout.Anchors[index],
			CoreBounds: group[0].bounds, Pivot: group[0].pivot,
		}
		for _, component := range group {
			pose.Components = append(pose.Components, component.bounds)
			if pose.Bounds.Empty() {
				pose.Bounds = component.bounds
			} else {
				pose.Bounds = pose.Bounds.Union(component.bounds)
			}
		}
		poses[index] = pose
	}
	if err := validateSemanticSeparation(poses); err != nil {
		return nil, err
	}
	for index := range poses {
		if outputPaths == nil {
			continue
		}
		poses[index].Path = outputPaths[index]
		if err := writeSemanticPose(board, groups[index], poses[index].Bounds, outputPaths[index]); err != nil {
			return nil, err
		}
	}
	return poses, nil
}

func assignComponentsToAnchors(
	components []semanticComponent,
	layout SemanticLayout,
) ([][]semanticComponent, error) {
	groups := make([][]semanticComponent, len(layout.Anchors))
	maximumDistance := float64(layout.AnchorSpacing) * 0.75
	ambiguityDistance := float64(layout.AnchorSpacing) / 12
	for _, component := range components {
		best, second := -1, -1
		bestDistance, secondDistance := math.MaxFloat64, math.MaxFloat64
		for anchorIndex, anchor := range layout.Anchors {
			distance := pointDistance(component.pivot, anchor)
			if distance < bestDistance {
				second, secondDistance = best, bestDistance
				best, bestDistance = anchorIndex, distance
			} else if distance < secondDistance {
				second, secondDistance = anchorIndex, distance
			}
		}
		if best < 0 || bestDistance > maximumDistance {
			return nil, fmt.Errorf("semantic component %v has no pose ownership", component.bounds)
		}
		if second >= 0 && secondDistance-bestDistance <= ambiguityDistance {
			return nil, fmt.Errorf("semantic component %v has ambiguous ownership", component.bounds)
		}
		groups[best] = append(groups[best], component)
	}
	for anchorIndex := range groups {
		if len(groups[anchorIndex]) == 0 {
			return nil, fmt.Errorf(
				"semantic anchor %02d has no primary body core",
				anchorIndex,
			)
		}
		primary := semanticPrimaryComponent(
			groups[anchorIndex],
			layout.Anchors[anchorIndex],
		)
		groups[anchorIndex][0], groups[anchorIndex][primary] =
			groups[anchorIndex][primary], groups[anchorIndex][0]
	}
	return groups, nil
}

// semanticPrimaryComponent selects the grounded component for body
// registration. Area alone is unsafe because a detached shield or wing can be
// larger than the body.
func semanticPrimaryComponent(
	components []semanticComponent,
	anchor image.Point,
) int {
	const groundedTolerance = semanticAnchorSpacing / 12
	maximumBottom := components[0].pivot.Y
	for _, component := range components[1:] {
		maximumBottom = max(maximumBottom, component.pivot.Y)
	}
	selected := -1
	for index, component := range components {
		if maximumBottom-component.pivot.Y > groundedTolerance {
			continue
		}
		horizontalDistance := absInt(component.pivot.X - anchor.X)
		if selected < 0 ||
			horizontalDistance <
				absInt(components[selected].pivot.X-anchor.X) ||
			horizontalDistance ==
				absInt(components[selected].pivot.X-anchor.X) &&
				component.area > components[selected].area {
			selected = index
		}
	}
	return selected
}

func validateSemanticSeparation(poses []SemanticPose) error {
	for left := range poses {
		for right := left + 1; right < len(poses); right++ {
			if poses[left].Bounds.Overlaps(poses[right].Bounds) {
				return fmt.Errorf(
					"semantic poses %02d and %02d have overlapping ownership bounds",
					left,
					right,
				)
			}
		}
	}
	return nil
}

func semanticComponents(source *image.NRGBA) []semanticComponent {
	bounds := source.Bounds()
	seen := make([]bool, bounds.Dx()*bounds.Dy())
	var result []semanticComponent
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			index := (y-bounds.Min.Y)*bounds.Dx() + x - bounds.Min.X
			if seen[index] || source.NRGBAAt(x, y).A == 0 {
				continue
			}
			component := semanticComponent{}
			queue := []image.Point{{X: x, Y: y}}
			seen[index] = true
			for head := 0; head < len(queue); head++ {
				point := queue[head]
				component.points = append(component.points, point)
				pixelBounds := image.Rect(point.X, point.Y, point.X+1, point.Y+1)
				if component.bounds.Empty() {
					component.bounds = pixelBounds
				} else {
					component.bounds = component.bounds.Union(pixelBounds)
				}
				for _, next := range neighboringPixels(point) {
					if !next.In(bounds) || source.NRGBAAt(next.X, next.Y).A == 0 {
						continue
					}
					nextIndex := (next.Y-bounds.Min.Y)*bounds.Dx() + next.X - bounds.Min.X
					if !seen[nextIndex] {
						seen[nextIndex] = true
						queue = append(queue, next)
					}
				}
			}
			component.area = len(component.points)
			component.pivot = robustComponentPivot(component.points)
			result = append(result, component)
		}
	}
	return result
}

func robustComponentPivot(points []image.Point) image.Point {
	if len(points) == 0 {
		return image.Point{}
	}
	minimumY, maximumY := points[0].Y, points[0].Y
	for _, point := range points[1:] {
		minimumY = min(minimumY, point.Y)
		maximumY = max(maximumY, point.Y)
	}
	bottomBand := max(2, (maximumY-minimumY+1)/8)
	xs := make([]int, 0, len(points))
	for _, point := range points {
		if point.Y >= maximumY-bottomBand+1 {
			xs = append(xs, point.X)
		}
	}
	sort.Ints(xs)
	return image.Pt(xs[len(xs)/2], maximumY+1)
}

func pointDistance(left, right image.Point) float64 {
	x := float64(left.X - right.X)
	y := float64(left.Y - right.Y)
	return math.Hypot(x, y)
}

func writeSemanticPose(
	board *image.NRGBA,
	components []semanticComponent,
	bounds image.Rectangle,
	outputPath string,
) error {
	pose := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for _, component := range components {
		for _, point := range component.points {
			pose.SetNRGBA(
				point.X-bounds.Min.X,
				point.Y-bounds.Min.Y,
				board.NRGBAAt(point.X, point.Y),
			)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return writePNG(outputPath, pose)
}

// WriteSemanticOwnershipOverlay records logical anchors and proven complete
// pose bounds for human review. It never participates in generation.
func WriteSemanticOwnershipOverlay(
	boardPath string,
	layout SemanticLayout,
	poses []SemanticPose,
	outputPath string,
) error {
	source, err := decodeNRGBA(boardPath)
	if err != nil {
		return err
	}
	overlay := image.NewNRGBA(source.Bounds())
	draw.Draw(overlay, overlay.Bounds(), source, source.Bounds().Min, draw.Src)
	colors := [...]color.NRGBA{
		{R: 255, G: 64, B: 64, A: 255},
		{R: 64, G: 255, B: 128, A: 255},
		{R: 64, G: 160, B: 255, A: 255},
		{R: 255, G: 192, B: 64, A: 255},
	}
	for index, pose := range poses {
		value := colors[index%len(colors)]
		drawRectangleOutline(overlay, pose.Bounds, value)
		drawCross(overlay, pose.Anchor, value)
		drawCross(overlay, pose.Pivot, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}
	return writePNG(outputPath, overlay)
}

// WriteRecoveredPoseSheet assembles proven complete-pose crops for manual
// inspection without changing their scale.
func WriteRecoveredPoseSheet(
	paths []string,
	columns int,
	outputPath string,
) error {
	if len(paths) == 0 || columns <= 0 {
		return fmt.Errorf("recovered pose sheet requires paths and columns")
	}
	images := make([]*image.NRGBA, len(paths))
	cellWidth, cellHeight := 0, 0
	for index, path := range paths {
		decoded, err := decodeNRGBA(path)
		if err != nil {
			return err
		}
		images[index] = decoded
		cellWidth = max(cellWidth, decoded.Bounds().Dx())
		cellHeight = max(cellHeight, decoded.Bounds().Dy())
	}
	const padding = 8
	cellWidth += padding * 2
	cellHeight += padding * 2
	rows := (len(images) + columns - 1) / columns
	sheet := image.NewNRGBA(
		image.Rect(0, 0, columns*cellWidth, rows*cellHeight),
	)
	for index, source := range images {
		column, row := index%columns, index/columns
		left := column*cellWidth + (cellWidth-source.Bounds().Dx())/2
		top := row*cellHeight + (cellHeight-source.Bounds().Dy())/2
		draw.Draw(
			sheet,
			image.Rect(
				left,
				top,
				left+source.Bounds().Dx(),
				top+source.Bounds().Dy(),
			),
			source,
			source.Bounds().Min,
			draw.Src,
		)
	}
	return writePNG(outputPath, sheet)
}

func drawRectangleOutline(destination *image.NRGBA, bounds image.Rectangle, value color.NRGBA) {
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		if image.Pt(x, bounds.Min.Y).In(destination.Bounds()) {
			destination.SetNRGBA(x, bounds.Min.Y, value)
		}
		if image.Pt(x, bounds.Max.Y-1).In(destination.Bounds()) {
			destination.SetNRGBA(x, bounds.Max.Y-1, value)
		}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if image.Pt(bounds.Min.X, y).In(destination.Bounds()) {
			destination.SetNRGBA(bounds.Min.X, y, value)
		}
		if image.Pt(bounds.Max.X-1, y).In(destination.Bounds()) {
			destination.SetNRGBA(bounds.Max.X-1, y, value)
		}
	}
}

func drawCross(destination *image.NRGBA, point image.Point, value color.NRGBA) {
	for offset := -4; offset <= 4; offset++ {
		for _, target := range []image.Point{
			{X: point.X + offset, Y: point.Y},
			{X: point.X, Y: point.Y + offset},
		} {
			if target.In(destination.Bounds()) {
				destination.SetNRGBA(target.X, target.Y, value)
			}
		}
	}
}
