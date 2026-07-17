package imageio

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

var ErrCanonicalScaleCropping = errors.New("canonical seed scale would crop generated frame")

// PNGDimensions returns the decoded PNG dimensions without loading all pixels.
func PNGDimensions(path string) (image.Point, error) {
	file, err := os.Open(path)
	if err != nil {
		return image.Point{}, err
	}
	defer file.Close()
	config, err := png.DecodeConfig(file)
	if err != nil {
		return image.Point{}, err
	}
	return image.Pt(config.Width, config.Height), nil
}

// GridLayout describes a deterministic square board of equally sized cells.
// Expected cells are filled in row-major order; remaining cells stay empty.
type GridLayout struct {
	Side         int `json:"side,omitempty"`
	Columns      int `json:"columns,omitempty"`
	Rows         int `json:"rows,omitempty"`
	CanvasWidth  int `json:"canvasWidth,omitempty"`
	CanvasHeight int `json:"canvasHeight,omitempty"`
	OffsetX      int `json:"offsetX,omitempty"`
	OffsetY      int `json:"offsetY,omitempty"`
	CellWidth    int `json:"cellWidth"`
	CellHeight   int `json:"cellHeight"`
	Gutter       int `json:"gutter"`
	Count        int `json:"count"`
}

func (layout GridLayout) Width() int {
	if layout.CanvasWidth > 0 {
		return layout.CanvasWidth
	}
	return layout.columns()*layout.CellWidth + max(0, layout.columns()-1)*layout.Gutter
}

func (layout GridLayout) Height() int {
	if layout.CanvasHeight > 0 {
		return layout.CanvasHeight
	}
	return layout.rows()*layout.CellHeight + max(0, layout.rows()-1)*layout.Gutter
}

func (layout GridLayout) Cell(index int) image.Rectangle {
	column, row := index%layout.columns(), index/layout.columns()
	x := layout.OffsetX + column*(layout.CellWidth+layout.Gutter)
	y := layout.OffsetY + row*(layout.CellHeight+layout.Gutter)
	return image.Rect(x, y, x+layout.CellWidth, y+layout.CellHeight)
}

func (layout GridLayout) columns() int {
	if layout.Columns > 0 {
		return layout.Columns
	}
	return layout.Side
}

func (layout GridLayout) rows() int {
	if layout.Rows > 0 {
		return layout.Rows
	}
	return layout.Side
}

func (layout GridLayout) slots() int { return layout.columns() * layout.rows() }

func SquareGridLayout(count, cellWidth, cellHeight, gutter int) (GridLayout, error) {
	if count <= 0 || cellWidth <= 0 || cellHeight <= 0 || gutter < 0 {
		return GridLayout{}, fmt.Errorf("invalid grid count=%d cell=%dx%d gutter=%d", count, cellWidth, cellHeight, gutter)
	}
	return GridLayout{Side: int(math.Ceil(math.Sqrt(float64(count)))), CellWidth: cellWidth, CellHeight: cellHeight, Gutter: gutter, Count: count}, nil
}

// CanvasGridLayout places up to maxColumns cells in row-major order on a
// square canvas. Cells stay square and the occupied grid is centered.
func CanvasGridLayout(count, maxColumns, canvasSize int) (GridLayout, error) {
	if count <= 0 || maxColumns <= 0 || canvasSize <= 0 {
		return GridLayout{}, fmt.Errorf("invalid canvas grid count=%d columns=%d canvas=%d", count, maxColumns, canvasSize)
	}
	columns := min(count, maxColumns)
	if count == 3 {
		columns = 2
	}
	rows := (count + columns - 1) / columns
	outerMargin := min(32, max(1, canvasSize/32))
	gutter := min(16, max(1, canvasSize/64))
	availableWidth := canvasSize - 2*outerMargin - max(0, columns-1)*gutter
	availableHeight := canvasSize - 2*outerMargin - max(0, rows-1)*gutter
	cell := min(availableWidth/columns, availableHeight/rows)
	if cell <= 0 {
		return GridLayout{}, fmt.Errorf("canvas %d cannot fit %d cells", canvasSize, count)
	}
	width := columns*cell + max(0, columns-1)*gutter
	height := rows*cell + max(0, rows-1)*gutter
	return GridLayout{
		Columns:      columns,
		Rows:         rows,
		CanvasWidth:  canvasSize,
		CanvasHeight: canvasSize,
		OffsetX:      (canvasSize - width) / 2,
		OffsetY:      (canvasSize - height) / 2,
		CellWidth:    cell,
		CellHeight:   cell,
		Gutter:       gutter,
		Count:        count,
	}, nil
}

// WriteGridBoard assembles exact source images without scaling. The resulting
// board is reference evidence; it is never a source for deployed target frames.
func WriteGridBoard(paths []string, outputPath string, gutter int) (GridLayout, error) {
	if len(paths) == 0 {
		return GridLayout{}, fmt.Errorf("grid board requires at least one image")
	}
	images := make([]*image.NRGBA, len(paths))
	for index, path := range paths {
		img, err := decodeNRGBA(path)
		if err != nil {
			return GridLayout{}, fmt.Errorf("decode board image %q: %w", path, err)
		}
		if index > 0 && img.Bounds().Size() != images[0].Bounds().Size() {
			return GridLayout{}, fmt.Errorf("board image %q size %s differs from %s", path, img.Bounds().Size(), images[0].Bounds().Size())
		}
		images[index] = img
	}
	layout, err := SquareGridLayout(len(images), images[0].Bounds().Dx(), images[0].Bounds().Dy(), gutter)
	if err != nil {
		return GridLayout{}, err
	}
	board := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	for index, img := range images {
		draw.Draw(board, layout.Cell(index), img, img.Bounds().Min, draw.Src)
	}
	if err := writePNG(outputPath, board); err != nil {
		return GridLayout{}, err
	}
	return layout, nil
}

// WriteCanvasBoard composes source sprites into fixed cells on a square
// provider canvas. Every source is fitted inside a cell guard band; cell
// boundaries are never inferred from generated pixels.
func WriteCanvasBoard(paths []string, outputPath string, layout GridLayout, guard int) error {
	if len(paths) != layout.Count {
		return fmt.Errorf("canvas board has %d sources for %d cells", len(paths), layout.Count)
	}
	if guard < 0 || guard*2 >= layout.CellWidth || guard*2 >= layout.CellHeight {
		return fmt.Errorf("invalid canvas cell guard %d", guard)
	}
	sources := make([]*image.NRGBA, len(paths))
	bounds := make([]image.Rectangle, len(paths))
	maximumWidth, maximumHeight := 0, 0
	for index, path := range paths {
		source, err := decodeNRGBA(path)
		if err != nil {
			return fmt.Errorf("decode board source %q: %w", path, err)
		}
		sources[index] = source
		bounds[index], err = alphaBounds(source)
		if err != nil {
			return fmt.Errorf("board source %q: %w", path, err)
		}
		maximumWidth = max(maximumWidth, bounds[index].Dx())
		maximumHeight = max(maximumHeight, bounds[index].Dy())
	}
	availableWidth := layout.CellWidth - 2*guard
	availableHeight := layout.CellHeight - 2*guard
	scale := min(float64(availableWidth)/float64(maximumWidth), float64(availableHeight)/float64(maximumHeight))
	board := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	for index, source := range sources {
		cell := layout.Cell(index)
		width := max(1, int(math.Round(float64(bounds[index].Dx())*scale)))
		height := max(1, int(math.Round(float64(bounds[index].Dy())*scale)))
		left := cell.Min.X + (cell.Dx()-width)/2
		bottom := cell.Max.Y - guard
		destination := image.Rect(left, bottom-height, left+width, bottom)
		areaScale(board, destination, source, bounds[index])
	}
	return writePNG(outputPath, board)
}

// WriteTransparentBoard removes an opaque edge-connected background while
// preserving the provider canvas dimensions used by fixed-coordinate QA.
func WriteTransparentBoard(path string, data []byte, width, height int) error {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode generated board: %w", err)
	}
	if decoded.Bounds().Dx() != width || decoded.Bounds().Dy() != height {
		return fmt.Errorf("generated board is %dx%d, expected %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy(), width, height)
	}
	return writePNG(path, removeEdgeBackground(decoded))
}

// WriteCellEditMask creates an OpenAI edit mask for a complete board. Opaque
// pixels preserve every cell except the selected editable cell.
func WriteCellEditMask(outputPath string, layout GridLayout, editableCell int) error {
	if editableCell < 0 || editableCell >= layout.Count {
		return fmt.Errorf("editable cell %d outside board count %d", editableCell, layout.Count)
	}
	mask := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	draw.Draw(mask, mask.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
	cell := layout.Cell(editableCell)
	draw.Draw(mask, cell, image.Transparent, image.Point{}, draw.Src)
	return writePNG(outputPath, mask)
}

// WriteBoardEditMask exposes all expected cells while preserving gutters,
// trailing cells, and the optional locked cell.
func WriteBoardEditMask(outputPath string, layout GridLayout, lockedCell int) error {
	mask := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	draw.Draw(mask, mask.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
	for index := 0; index < layout.Count; index++ {
		if index == lockedCell {
			continue
		}
		draw.Draw(mask, layout.Cell(index), image.Transparent, image.Point{}, draw.Src)
	}
	return writePNG(outputPath, mask)
}

// WriteSharedNormalizedCells extracts validated fixed cells and writes target
// sprites with one shared scale and bottom-center anchor. The caller must run
// whole-board validation first; this function never searches for boundaries.
func WriteSharedNormalizedCells(boardPath string, layout GridLayout, outputPaths []string, width, height int, lockedPalette []PaletteColor, lockedFirst string) ([]PaletteColor, error) {
	if len(outputPaths) != layout.Count {
		return nil, fmt.Errorf("normalization has %d outputs for %d cells", len(outputPaths), layout.Count)
	}
	board, err := decodeNRGBA(boardPath)
	if err != nil {
		return nil, err
	}
	if board.Bounds() != image.Rect(0, 0, layout.Width(), layout.Height()) {
		return nil, fmt.Errorf("board dimensions do not match fixed layout")
	}
	cells := make([]*image.NRGBA, layout.Count)
	bounds := make([]image.Rectangle, layout.Count)
	maximumWidth, maximumHeight := 0, 0
	for index := range cells {
		cells[index] = copyCell(board, layout.Cell(index))
		bounds[index], err = alphaBounds(cells[index])
		if err != nil {
			return nil, fmt.Errorf("cell %02d: %w", index, err)
		}
		maximumWidth = max(maximumWidth, bounds[index].Dx())
		maximumHeight = max(maximumHeight, bounds[index].Dy())
	}
	padding := max(1, min(width, height)/20)
	scale := min(float64(width-2*padding)/float64(maximumWidth), float64(height-2*padding)/float64(maximumHeight))
	if scale <= 0 {
		return nil, fmt.Errorf("shared row scale is not positive")
	}
	palette := append([]PaletteColor(nil), lockedPalette...)
	for index, outputPath := range outputPaths {
		if index == 0 && lockedFirst != "" {
			point, sizeErr := PNGDimensions(lockedFirst)
			if sizeErr != nil {
				return nil, sizeErr
			}
			if point != image.Pt(width, height) {
				return nil, fmt.Errorf("locked first frame is %dx%d, expected %dx%d", point.X, point.Y, width, height)
			}
			if err := CopyFile(lockedFirst, outputPath); err != nil {
				return nil, err
			}
			continue
		}
		destination := image.NewNRGBA(image.Rect(0, 0, width, height))
		destinationWidth := max(1, int(math.Round(float64(bounds[index].Dx())*scale)))
		destinationHeight := max(1, int(math.Round(float64(bounds[index].Dy())*scale)))
		left := (width - destinationWidth) / 2
		bottom := height - padding
		destinationRect := image.Rect(left, bottom-destinationHeight, left+destinationWidth, bottom)
		areaScale(destination, destinationRect, cells[index], bounds[index])
		if len(palette) == 0 {
			palette = extractPalette(destination, defaultPaletteSize)
		}
		destination = applyPalette(destination, palette)
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nil, err
		}
		if err := writePNG(outputPath, destination); err != nil {
			return nil, err
		}
	}
	return palette, nil
}

type CanonicalTransform struct {
	Scale    float64 `json:"scale"`
	Baseline int     `json:"baseline"`
	CenterX  int     `json:"centerX"`
}

// WriteCanonicalNormalizedCells maps a generated row through one transform
// derived from its first cell and the approved directional seed. It rejects a
// frame that cannot fit; it never shrinks or crops the row to hide bad output.
func WriteCanonicalNormalizedCells(boardPath string, layout GridLayout, outputPaths []string, width, height int, lockedPalette []PaletteColor, canonicalSeed string, lockedFirst bool) ([]PaletteColor, CanonicalTransform, error) {
	if len(outputPaths) != layout.Count {
		return nil, CanonicalTransform{}, fmt.Errorf("normalization has %d outputs for %d cells", len(outputPaths), layout.Count)
	}
	board, err := decodeNRGBA(boardPath)
	if err != nil {
		return nil, CanonicalTransform{}, err
	}
	if board.Bounds() != image.Rect(0, 0, layout.Width(), layout.Height()) {
		return nil, CanonicalTransform{}, fmt.Errorf("board dimensions do not match fixed layout")
	}
	seed, err := decodeNRGBA(canonicalSeed)
	if err != nil {
		return nil, CanonicalTransform{}, err
	}
	if seed.Bounds().Size() != image.Pt(width, height) {
		return nil, CanonicalTransform{}, fmt.Errorf("canonical seed is %dx%d, expected %dx%d", seed.Bounds().Dx(), seed.Bounds().Dy(), width, height)
	}
	seedBounds, err := alphaBounds(seed)
	if err != nil {
		return nil, CanonicalTransform{}, fmt.Errorf("canonical seed: %w", err)
	}
	cells := make([]*image.NRGBA, layout.Count)
	bounds := make([]image.Rectangle, layout.Count)
	for index := range cells {
		cells[index] = copyCell(board, layout.Cell(index))
		bounds[index], err = alphaBounds(cells[index])
		if err != nil {
			return nil, CanonicalTransform{}, fmt.Errorf("cell %02d: %w", index, err)
		}
	}
	transform := CanonicalTransform{
		Scale:    float64(seedBounds.Dy()) / float64(bounds[0].Dy()),
		Baseline: seedBounds.Max.Y,
		CenterX:  width / 2,
	}
	if transform.Scale <= 0 {
		return nil, CanonicalTransform{}, errors.New("canonical scale is not positive")
	}
	destinations := make([]image.Rectangle, len(cells))
	canvas := image.Rect(0, 0, width, height)
	for index := range cells {
		destinationWidth := max(1, int(math.Round(float64(bounds[index].Dx())*transform.Scale)))
		destinationHeight := max(1, int(math.Round(float64(bounds[index].Dy())*transform.Scale)))
		left := transform.CenterX - destinationWidth/2
		destinations[index] = image.Rect(left, transform.Baseline-destinationHeight, left+destinationWidth, transform.Baseline)
		if !destinations[index].In(canvas) {
			return nil, transform, fmt.Errorf("%w: cell %02d maps to %v outside %v", ErrCanonicalScaleCropping, index, destinations[index], canvas)
		}
	}
	palette := append([]PaletteColor(nil), lockedPalette...)
	for index, outputPath := range outputPaths {
		if index == 0 && lockedFirst {
			if err := CopyFile(canonicalSeed, outputPath); err != nil {
				return nil, transform, err
			}
			continue
		}
		destination := image.NewNRGBA(canvas)
		areaScale(destination, destinations[index], cells[index], bounds[index])
		if len(palette) == 0 {
			palette = extractPalette(destination, defaultPaletteSize)
		}
		destination = applyPalette(destination, palette)
		if err := writePNG(outputPath, destination); err != nil {
			return nil, transform, err
		}
	}
	return palette, transform, nil
}

// WriteAlignedPoseGuides places legacy motion evidence at the approved seed's
// body scale and baseline before it is sent to the provider.
func WriteAlignedPoseGuides(sourcePaths []string, canonicalSeed string, outputPaths []string) error {
	if len(sourcePaths) == 0 || len(sourcePaths) != len(outputPaths) {
		return errors.New("aligned pose guides require matching non-empty source and output paths")
	}
	seed, err := decodeNRGBA(canonicalSeed)
	if err != nil {
		return err
	}
	seedBounds, err := alphaBounds(seed)
	if err != nil {
		return fmt.Errorf("canonical seed: %w", err)
	}
	sources := make([]*image.NRGBA, len(sourcePaths))
	bounds := make([]image.Rectangle, len(sourcePaths))
	for index, path := range sourcePaths {
		sources[index], err = decodeNRGBA(path)
		if err != nil {
			return err
		}
		if sources[index].Bounds().Size() != seed.Bounds().Size() {
			return fmt.Errorf("pose guide %q is %dx%d, expected %dx%d", path, sources[index].Bounds().Dx(), sources[index].Bounds().Dy(), seed.Bounds().Dx(), seed.Bounds().Dy())
		}
		bounds[index], err = alphaBounds(sources[index])
		if err != nil {
			return fmt.Errorf("pose guide %q: %w", path, err)
		}
	}
	scale := float64(seedBounds.Dy()) / float64(bounds[0].Dy())
	canvas := image.Rect(0, 0, seed.Bounds().Dx(), seed.Bounds().Dy())
	for index, outputPath := range outputPaths {
		width := max(1, int(math.Round(float64(bounds[index].Dx())*scale)))
		height := max(1, int(math.Round(float64(bounds[index].Dy())*scale)))
		left := canvas.Dx()/2 - width/2
		destinationRect := image.Rect(left, seedBounds.Max.Y-height, left+width, seedBounds.Max.Y)
		if !destinationRect.In(canvas) {
			return fmt.Errorf("pose guide %q does not fit accepted seed scale and baseline", sourcePaths[index])
		}
		destination := image.NewNRGBA(canvas)
		areaScale(destination, destinationRect, sources[index], bounds[index])
		if err := writePNG(outputPath, destination); err != nil {
			return err
		}
	}
	return nil
}

func alphaBounds(img *image.NRGBA) (image.Rectangle, error) {
	bounds := img.Bounds()
	result := image.Rectangle{Min: bounds.Max, Max: bounds.Min}
	found := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			found = true
			result.Min.X = min(result.Min.X, x)
			result.Min.Y = min(result.Min.Y, y)
			result.Max.X = max(result.Max.X, x+1)
			result.Max.Y = max(result.Max.Y, y+1)
		}
	}
	if !found {
		return image.Rectangle{}, fmt.Errorf("contains no foreground subject")
	}
	return result, nil
}

type StudyMetrics struct {
	Cells            []Metrics `json:"cells"`
	Score            float64   `json:"score"`
	CadenceDelta     float64   `json:"cadenceDelta"`
	MarginOccupied   bool      `json:"marginOccupied"`
	GutterOccupied   bool      `json:"gutterOccupied"`
	TrailingOccupied bool      `json:"trailingOccupied"`
}

// BoardValidationPurpose controls whether pose-guide differences are advisory
// or block fixed-cell extraction.
type BoardValidationPurpose uint8

const (
	// BoardValidationSeed keeps structural cell safety mandatory while leaving
	// pose and scale differences to the explicit human seed-review gate.
	BoardValidationSeed BoardValidationPurpose = iota + 1
	// BoardValidationAnimationRow keeps structural safety mandatory while
	// recording legacy pose and cadence differences for human review.
	BoardValidationAnimationRow
)

// BoardEvaluation separates non-overridable extraction failures from visual
// differences that a human seed reviewer may intentionally accept.
type BoardEvaluation struct {
	Metrics          StudyMetrics
	BlockingFailures []string
	Warnings         []string
}

// EvaluateBoard validates a generated board according to its pipeline role.
func EvaluateBoard(candidatePath, guideBoardPath string, layout GridLayout, guard int, purpose BoardValidationPurpose) (BoardEvaluation, error) {
	if purpose != BoardValidationSeed && purpose != BoardValidationAnimationRow {
		return BoardEvaluation{}, fmt.Errorf("unsupported board validation purpose %d", purpose)
	}
	candidate, err := decodeNRGBA(candidatePath)
	if err != nil {
		return BoardEvaluation{}, err
	}
	guideBoard, err := decodeNRGBA(guideBoardPath)
	if err != nil {
		return BoardEvaluation{}, err
	}
	expected := image.Rect(0, 0, layout.Width(), layout.Height())
	if candidate.Bounds() != expected || guideBoard.Bounds() != expected {
		return BoardEvaluation{}, fmt.Errorf("generated board and guide board must both be %dx%d", expected.Dx(), expected.Dy())
	}
	evaluation := BoardEvaluation{Metrics: StudyMetrics{
		MarginOccupied:   boardMarginOccupied(candidate, layout),
		GutterOccupied:   boardGutterOccupied(candidate, layout),
		TrailingOccupied: trailingCellsOccupied(candidate, layout),
	}}
	if evaluation.Metrics.MarginOccupied {
		evaluation.BlockingFailures = append(evaluation.BlockingFailures, "board_margin_occupied")
	}
	if evaluation.Metrics.GutterOccupied {
		evaluation.BlockingFailures = append(evaluation.BlockingFailures, "board_gutter_occupied")
	}
	if evaluation.Metrics.TrailingOccupied {
		evaluation.BlockingFailures = append(evaluation.BlockingFailures, "board_trailing_cell_occupied")
	}
	for index := 0; index < layout.Count; index++ {
		cell := layout.Cell(index)
		candidateCell := copyCell(candidate, cell)
		guideCell := copyCell(guideBoard, cell)
		cellMetrics := compareMasks(candidateCell, guideCell, guard)
		evaluation.Metrics.Cells = append(evaluation.Metrics.Cells, cellMetrics)
		evaluation.Metrics.Score += cellMetrics.Score
		blocking, warnings := boardCellFindings(cellMetrics, purpose)
		for _, reason := range blocking {
			evaluation.BlockingFailures = append(evaluation.BlockingFailures, fmt.Sprintf("cell_%02d_%s", index, reason))
		}
		for _, reason := range warnings {
			evaluation.Warnings = append(evaluation.Warnings, fmt.Sprintf("cell_%02d_%s", index, reason))
		}
	}
	evaluation.Metrics.Score /= float64(layout.Count)
	for index := 1; index < len(evaluation.Metrics.Cells); index++ {
		evaluation.Metrics.CadenceDelta += math.Abs(evaluation.Metrics.Cells[index].CenterDistance - evaluation.Metrics.Cells[index-1].CenterDistance)
	}
	if len(evaluation.Metrics.Cells) > 1 {
		evaluation.Metrics.CadenceDelta /= float64(len(evaluation.Metrics.Cells) - 1)
	}
	if evaluation.Metrics.CadenceDelta > 0.1 {
		evaluation.Warnings = append(evaluation.Warnings, "row_cadence_difference")
	}
	return evaluation, nil
}

// EvaluateMotionStudy compares board cells in memory for QA only. No extracted
// cell is written or exposed as a target candidate.
func EvaluateMotionStudy(candidatePath, poseBoardPath string, layout GridLayout, guard int) (StudyMetrics, []string, error) {
	evaluation, err := EvaluateBoard(candidatePath, poseBoardPath, layout, guard, BoardValidationAnimationRow)
	return evaluation.Metrics, append(evaluation.BlockingFailures, evaluation.Warnings...), err
}

func boardCellFindings(metrics Metrics, purpose BoardValidationPurpose) ([]string, []string) {
	var blocking []string
	if metrics.EdgeGuardOccupied {
		blocking = append(blocking, "edge_guard_occupied")
	}
	if metrics.Components != 1 {
		blocking = append(blocking, fmt.Sprintf("foreground_components_%d", metrics.Components))
	}
	var poseDifferences []string
	if metrics.SilhouetteOverlap < 0.35 {
		poseDifferences = append(poseDifferences, "silhouette_overlap_below_threshold")
	}
	if metrics.CenterDistance > 0.15 {
		poseDifferences = append(poseDifferences, "subject_center_drift")
	}
	if metrics.BaselineDelta > 0.1 {
		poseDifferences = append(poseDifferences, "baseline_drift")
	}
	if metrics.OccupiedBoundsDelta > 0.15 {
		poseDifferences = append(poseDifferences, "occupied_bounds_drift")
	}
	if metrics.SecondaryComponents > 0 {
		poseDifferences = append(poseDifferences, fmt.Sprintf("secondary_components_%d", metrics.SecondaryComponents))
	}
	if metrics.PaletteDistance > 0.25 {
		poseDifferences = append(poseDifferences, "palette_distance")
	}
	return blocking, poseDifferences
}

func candidateReasons(metrics Metrics) []string {
	var reasons []string
	if metrics.EdgeGuardOccupied {
		reasons = append(reasons, "edge_guard_occupied")
	}
	if metrics.Components != 1 {
		reasons = append(reasons, fmt.Sprintf("foreground_components_%d", metrics.Components))
	}
	if metrics.SilhouetteOverlap < 0.35 {
		reasons = append(reasons, "silhouette_overlap_below_threshold")
	}
	if metrics.CenterDistance > 0.15 {
		reasons = append(reasons, "subject_center_drift")
	}
	if metrics.BaselineDelta > 0.05 {
		reasons = append(reasons, "baseline_drift")
	}
	if metrics.OccupiedBoundsDelta > 0.15 {
		reasons = append(reasons, "occupied_bounds_drift")
	}
	return reasons
}

func boardGutterOccupied(img *image.NRGBA, layout GridLayout) bool {
	grid := layoutGridBounds(layout)
	for y := grid.Min.Y; y < grid.Max.Y; y++ {
		for x := grid.Min.X; x < grid.Max.X; x++ {
			if !layoutContains(layout, x, y) && img.NRGBAAt(x, y).A > 0 {
				return true
			}
		}
	}
	return false
}

func boardMarginOccupied(img *image.NRGBA, layout GridLayout) bool {
	grid := layoutGridBounds(layout)
	for y := 0; y < layout.Height(); y++ {
		for x := 0; x < layout.Width(); x++ {
			if image.Pt(x, y).In(grid) {
				continue
			}
			if img.NRGBAAt(x, y).A > 0 {
				return true
			}
		}
	}
	return false
}

func layoutGridBounds(layout GridLayout) image.Rectangle {
	first := layout.Cell(0)
	last := layout.Cell(layout.slots() - 1)
	return image.Rect(first.Min.X, first.Min.Y, last.Max.X, last.Max.Y)
}

func trailingCellsOccupied(img *image.NRGBA, layout GridLayout) bool {
	for index := layout.Count; index < layout.slots(); index++ {
		cell := layout.Cell(index)
		for y := cell.Min.Y; y < cell.Max.Y; y++ {
			for x := cell.Min.X; x < cell.Max.X; x++ {
				if img.NRGBAAt(x, y).A > 0 {
					return true
				}
			}
		}
	}
	return false
}

func layoutContains(layout GridLayout, x, y int) bool {
	for index := 0; index < layout.slots(); index++ {
		if image.Pt(x, y).In(layout.Cell(index)) {
			return true
		}
	}
	return false
}

func copyCell(source *image.NRGBA, cell image.Rectangle) *image.NRGBA {
	destination := image.NewNRGBA(image.Rect(0, 0, cell.Dx(), cell.Dy()))
	draw.Draw(destination, destination.Bounds(), source, cell.Min, draw.Src)
	return destination
}
