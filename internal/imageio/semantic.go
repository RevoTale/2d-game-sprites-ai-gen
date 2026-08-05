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
	semanticOuterReserve  = 64
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

// SemanticStaticSetLayout derives provider geometry from production canvas
// sizes. Every target receives enough room at native 2x scale plus a real
// chroma corridor; no per-part rescaling is required after extraction.
func SemanticStaticSetLayout(sizes []image.Point) (SemanticLayout, error) {
	if len(sizes) == 0 {
		return SemanticLayout{}, fmt.Errorf("semantic static set requires at least one size")
	}
	maximumWidth, maximumHeight := 0, 0
	for _, size := range sizes {
		if size.X <= 0 || size.Y <= 0 {
			return SemanticLayout{}, fmt.Errorf("semantic static set size must be positive")
		}
		maximumWidth = max(maximumWidth, size.X)
		maximumHeight = max(maximumHeight, size.Y)
	}
	columns := int(math.Ceil(math.Sqrt(float64(len(sizes)))))
	rows := (len(sizes) + columns - 1) / columns
	spacing := roundUp(max(maximumWidth, maximumHeight)+64, 16)
	width := roundUp(2*semanticOuterReserve+maximumWidth+(columns-1)*spacing, 16)
	height := roundUp(2*semanticOuterReserve+maximumHeight+(rows-1)*spacing, 16)
	layout := SemanticLayout{
		CanvasWidth: width, CanvasHeight: height,
		Columns: columns, Rows: rows, AnchorSpacing: spacing,
		Anchors: make([]image.Point, len(sizes)),
	}
	left := semanticOuterReserve + maximumWidth/2
	top := semanticOuterReserve + maximumHeight
	for index := range layout.Anchors {
		layout.Anchors[index] = image.Pt(
			left+(index%columns)*spacing,
			top+(index/columns)*spacing,
		)
	}
	return layout, nil
}

// WriteSemanticSizedPlaceholderBoard places smaller neutral silhouettes inside
// the exact production-sized regions. The inset gives generation room without
// changing the relative target canvases.
func WriteSemanticSizedPlaceholderBoard(
	outputPath string,
	layout SemanticLayout,
	sizes []image.Point,
	inset int,
) error {
	bounds, err := semanticSizedBounds(layout, sizes, inset)
	if err != nil {
		return err
	}
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	marker := color.NRGBA{R: 82, G: 86, B: 94, A: 255}
	for _, markerBounds := range bounds {
		draw.Draw(board, markerBounds, image.NewUniform(marker), image.Point{}, draw.Src)
	}
	return writePNG(outputPath, board)
}

// WriteSemanticSizedEditMask exposes each target's exact safe production
// rectangle while keeping every neighboring region opaque and disconnected.
func WriteSemanticSizedEditMask(
	outputPath string,
	layout SemanticLayout,
	sizes []image.Point,
) error {
	if len(sizes) != len(layout.Anchors) {
		return fmt.Errorf("semantic sized mask requires one size per anchor")
	}
	canvas := image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight)
	mask := image.NewNRGBA(canvas)
	draw.Draw(mask, canvas, image.NewUniform(color.NRGBA{R: 255, G: 255, B: 255, A: 255}), image.Point{}, draw.Src)
	regions := make([]image.Rectangle, len(sizes))
	for index, size := range sizes {
		guard := max(1, min(size.X, size.Y)/32)
		bounds, boundsErr := semanticSizedBounds(
			layout,
			[]image.Point{size},
			guard,
		)
		if boundsErr != nil {
			return boundsErr
		}
		width, height := bounds[0].Dx(), bounds[0].Dy()
		anchor := layout.Anchors[index]
		region := image.Rect(
			anchor.X-width/2,
			anchor.Y-height,
			anchor.X+(width+1)/2,
			anchor.Y,
		)
		if !region.In(canvas.Inset(semanticCanvasGuard)) {
			return fmt.Errorf("semantic edit region %02d exceeds safe canvas", index)
		}
		for previous := range index {
			if region.Overlaps(regions[previous]) {
				return fmt.Errorf("semantic edit regions %02d and %02d overlap", previous, index)
			}
		}
		regions[index] = region
		draw.Draw(mask, region, image.NewUniform(color.NRGBA{}), image.Point{}, draw.Src)
	}
	return writePNG(outputPath, mask)
}

func semanticSizedBounds(
	layout SemanticLayout,
	sizes []image.Point,
	inset int,
) ([]image.Rectangle, error) {
	if len(sizes) == 0 || inset < 0 {
		return nil, fmt.Errorf("semantic sized bounds require positive sizes and non-negative inset")
	}
	bounds := make([]image.Rectangle, len(sizes))
	for index, size := range sizes {
		width, height := size.X-2*inset, size.Y-2*inset
		if width <= 0 || height <= 0 {
			return nil, fmt.Errorf("semantic sized bounds inset exceeds size %dx%d", size.X, size.Y)
		}
		anchorIndex := index
		if len(sizes) == 1 && len(layout.Anchors) != 1 {
			anchorIndex = 0
		}
		if anchorIndex >= len(layout.Anchors) {
			return nil, fmt.Errorf("semantic sized bounds require one anchor per size")
		}
		anchor := layout.Anchors[anchorIndex]
		bounds[index] = image.Rect(
			anchor.X-width/2,
			anchor.Y-height,
			anchor.X+(width+1)/2,
			anchor.Y,
		)
	}
	return bounds, nil
}

// WriteSemanticPlaceholderBoard gives a provider visible one-to-one placement
// and relative-extent evidence when no existing subject images exist yet.
// Neutral silhouettes are protocol markers, not visual references.
func WriteSemanticPlaceholderBoard(
	outputPath string,
	layout SemanticLayout,
	sizes []image.Point,
	maximumExtent int,
) error {
	bounds, err := semanticPlaceholderBounds(layout, sizes, maximumExtent)
	if err != nil {
		return err
	}
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	marker := color.NRGBA{R: 82, G: 86, B: 94, A: 255}
	for _, markerBounds := range bounds {
		draw.Draw(board, markerBounds, image.NewUniform(marker), image.Point{}, draw.Src)
	}
	return writePNG(outputPath, board)
}

// WriteSemanticEditMask exposes one guarded editable region around each
// protocol marker. Transparent pixels are editable according to the OpenAI
// Image Edits contract; opaque pixels preserve the chroma separation.
func WriteSemanticEditMask(
	outputPath string,
	layout SemanticLayout,
	sizes []image.Point,
	maximumExtent, padding int,
) error {
	if padding <= 0 {
		return fmt.Errorf("semantic edit mask padding must be positive")
	}
	markerBounds, err := semanticPlaceholderBounds(layout, sizes, maximumExtent)
	if err != nil {
		return err
	}
	canvas := image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight)
	mask := image.NewNRGBA(canvas)
	draw.Draw(mask, canvas, image.NewUniform(color.NRGBA{R: 255, G: 255, B: 255, A: 255}), image.Point{}, draw.Src)
	regions := make([]image.Rectangle, len(markerBounds))
	for index, bounds := range markerBounds {
		region := bounds.Inset(-padding)
		if !region.In(canvas.Inset(semanticCanvasGuard)) {
			return fmt.Errorf("semantic edit region %02d exceeds safe canvas", index)
		}
		for previous := range index {
			if region.Overlaps(regions[previous]) {
				return fmt.Errorf("semantic edit regions %02d and %02d overlap", previous, index)
			}
		}
		regions[index] = region
		draw.Draw(mask, region, image.NewUniform(color.NRGBA{}), image.Point{}, draw.Src)
	}
	return writePNG(outputPath, mask)
}

func semanticPlaceholderBounds(
	layout SemanticLayout,
	sizes []image.Point,
	maximumExtent int,
) ([]image.Rectangle, error) {
	if len(sizes) != len(layout.Anchors) || maximumExtent <= 0 {
		return nil, fmt.Errorf("semantic placeholder board requires one positive size per anchor")
	}
	maximumWidth, maximumHeight := 0, 0
	for _, size := range sizes {
		if size.X <= 0 || size.Y <= 0 {
			return nil, fmt.Errorf("semantic placeholder size must be positive")
		}
		maximumWidth = max(maximumWidth, size.X)
		maximumHeight = max(maximumHeight, size.Y)
	}
	scale := min(
		float64(maximumExtent)/float64(maximumWidth),
		float64(maximumExtent)/float64(maximumHeight),
	)
	bounds := make([]image.Rectangle, len(sizes))
	for index, size := range sizes {
		width := max(1, int(math.Round(float64(size.X)*scale)))
		height := max(1, int(math.Round(float64(size.Y)*scale)))
		anchor := layout.Anchors[index]
		bounds[index] = image.Rect(
			anchor.X-width/2,
			anchor.Y-height,
			anchor.X+(width+1)/2,
			anchor.Y,
		)
	}
	return bounds, nil
}

func semanticLayout(count, columns int) (SemanticLayout, error) {
	if count <= 0 || columns <= 0 || columns > count {
		return SemanticLayout{}, fmt.Errorf("invalid semantic layout count=%d columns=%d", count, columns)
	}
	rows := (count + columns - 1) / columns
	gridWidth := columns * semanticAnchorSpacing
	gridHeight := rows * semanticAnchorSpacing
	// A provider must see real canvas outside the outermost logical poses. The
	// recovery guard detects clipping, but it cannot prevent a generated pose
	// from expanding into an edge when the semantic grid itself fills the canvas.
	width := roundUp(max(semanticMinimumCanvas, gridWidth+2*semanticOuterReserve), 16)
	height := roundUp(max(semanticMinimumCanvas, gridHeight+2*semanticOuterReserve), 16)
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
	maximumArea := 0
	for _, component := range components {
		maximumArea = max(maximumArea, component.area)
	}
	primary := make([]semanticComponent, 0, len(components))
	detached := make([]semanticComponent, 0)
	for _, component := range components {
		if component.area*16 < maximumArea {
			detached = append(detached, component)
			continue
		}
		primary = append(primary, component)
	}
	if len(primary) == len(layout.Anchors) {
		assignPrimaryComponentsInReadingOrder(groups, primary, layout)
	} else if err := assignPrimaryComponentsByAnchor(groups, primary, layout); err != nil {
		return nil, err
	}
	for anchorIndex := range groups {
		if len(groups[anchorIndex]) == 0 {
			return nil, fmt.Errorf(
				"semantic anchor %02d has no primary body core",
				anchorIndex,
			)
		}
	}
	for _, component := range detached {
		owner, err := detachedSemanticComponentOwner(component, groups, layout)
		if err != nil {
			return nil, err
		}
		groups[owner] = append(groups[owner], component)
	}
	for anchorIndex := range groups {
		primaryIndex := semanticPrimaryComponent(
			groups[anchorIndex],
			layout.Anchors[anchorIndex],
		)
		groups[anchorIndex][0], groups[anchorIndex][primaryIndex] =
			groups[anchorIndex][primaryIndex], groups[anchorIndex][0]
	}
	return groups, nil
}

func assignPrimaryComponentsInReadingOrder(
	groups [][]semanticComponent,
	components []semanticComponent,
	layout SemanticLayout,
) {
	sort.SliceStable(components, func(first, second int) bool {
		return components[first].pivot.Y < components[second].pivot.Y
	})
	offset := 0
	for row := 0; row < layout.Rows && offset < len(components); row++ {
		count := min(layout.Columns, len(components)-offset)
		rowComponents := components[offset : offset+count]
		sort.SliceStable(rowComponents, func(first, second int) bool {
			return rowComponents[first].pivot.X < rowComponents[second].pivot.X
		})
		for column, component := range rowComponents {
			groups[offset+column] = append(groups[offset+column], component)
		}
		offset += count
	}
}

func assignPrimaryComponentsByAnchor(
	groups [][]semanticComponent,
	components []semanticComponent,
	layout SemanticLayout,
) error {
	maximumDistance := float64(layout.AnchorSpacing) * 0.75
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
			return fmt.Errorf("semantic component %v has no pose ownership", component.bounds)
		}
		if second >= 0 && math.Abs(secondDistance-bestDistance) <= 1e-9 {
			return fmt.Errorf("semantic component %v has ambiguous ownership", component.bounds)
		}
		groups[best] = append(groups[best], component)
	}
	return nil
}

func detachedSemanticComponentOwner(
	component semanticComponent,
	groups [][]semanticComponent,
	layout SemanticLayout,
) (int, error) {
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
	if bestDistance <= float64(layout.AnchorSpacing)*0.75 {
		if second >= 0 && math.Abs(secondDistance-bestDistance) <= 1e-9 {
			return -1, fmt.Errorf("semantic component %v has ambiguous ownership", component.bounds)
		}
		return best, nil
	}
	return nearbySemanticGroupOwner(component, groups, layout.AnchorSpacing)
}

// nearbySemanticGroupOwner handles small detached material such as one loose
// stone beside a rubble pile. Anchor distance alone is insufficient at an
// intentionally unused trailing grid position, so ownership may fall back to
// direct, unambiguous proximity to an established group. Large components are
// never recovered through this path because they may be an extra subject.
func nearbySemanticGroupOwner(
	component semanticComponent,
	groups [][]semanticComponent,
	anchorSpacing int,
) (int, error) {
	const maximumAreaRatio = 16
	maximumGap := float64(anchorSpacing) / 8
	best, second := -1, -1
	bestDistance, secondDistance := math.MaxFloat64, math.MaxFloat64
	for groupIndex, group := range groups {
		if len(group) == 0 {
			continue
		}
		bounds := group[0].bounds
		maximumArea := group[0].area
		for _, member := range group[1:] {
			bounds = bounds.Union(member.bounds)
			maximumArea = max(maximumArea, member.area)
		}
		if component.area*maximumAreaRatio >= maximumArea {
			continue
		}
		distance := rectangleDistance(component.bounds, bounds)
		if distance < bestDistance {
			second, secondDistance = best, bestDistance
			best, bestDistance = groupIndex, distance
		} else if distance < secondDistance {
			second, secondDistance = groupIndex, distance
		}
	}
	if best < 0 || bestDistance > maximumGap {
		return -1, fmt.Errorf("semantic component %v has no pose ownership", component.bounds)
	}
	if second >= 0 && math.Abs(secondDistance-bestDistance) <= 1e-9 {
		return -1, fmt.Errorf("semantic component %v has ambiguous ownership", component.bounds)
	}
	return best, nil
}

func rectangleDistance(first, second image.Rectangle) float64 {
	dx := max(0, max(second.Min.X-first.Max.X, first.Min.X-second.Max.X))
	dy := max(0, max(second.Min.Y-first.Max.Y, first.Min.Y-second.Max.Y))
	return math.Hypot(float64(dx), float64(dy))
}

// semanticPrimaryComponent selects the grounded component for body
// registration. Area alone is unsafe because a detached shield or wing can be
// larger than the body.
func semanticPrimaryComponent(
	components []semanticComponent,
	anchor image.Point,
) int {
	const groundedTolerance = semanticAnchorSpacing / 12
	maximumArea := 0
	for _, component := range components {
		maximumArea = max(maximumArea, component.area)
	}
	maximumBottom := 0
	for _, component := range components {
		if component.area*16 < maximumArea {
			continue
		}
		maximumBottom = max(maximumBottom, component.pivot.Y)
	}
	selected := -1
	for index, component := range components {
		// Chroma recovery can leave isolated edge pixels. They may be closer to
		// the logical anchor than the character, but cannot plausibly be its
		// grounded body core. Keep large detached equipment eligible while
		// excluding components below one sixteenth of the group's largest mass.
		if component.area*16 < maximumArea {
			continue
		}
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
