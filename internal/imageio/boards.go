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

var ErrProductionFrameClipping = errors.New("foreground would touch or cross a production frame edge")

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

// GridLayout describes a deterministic board of equally sized cells.
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

// AnimationRowLayout places four ordered frames in one centered horizontal row.
func AnimationRowLayout(canvasSize int) (GridLayout, error) {
	return CanvasGridLayout(4, 4, canvasSize)
}

// CanvasGridLayoutRect places a fixed columns-by-rows grid on a rectangular
// provider canvas with the production 32px outer margin and 16px true gutter.
func CanvasGridLayoutRect(count, columns, rows, canvasWidth, canvasHeight int) (GridLayout, error) {
	if count <= 0 || columns <= 0 || rows <= 0 || count > columns*rows || canvasWidth <= 0 || canvasHeight <= 0 {
		return GridLayout{}, fmt.Errorf("invalid rectangular canvas grid count=%d grid=%dx%d canvas=%dx%d", count, columns, rows, canvasWidth, canvasHeight)
	}
	const outerMargin = 32
	const gutter = 16
	availableWidth := canvasWidth - 2*outerMargin - (columns-1)*gutter
	availableHeight := canvasHeight - 2*outerMargin - (rows-1)*gutter
	cellWidth, cellHeight := availableWidth/columns, availableHeight/rows
	if cellWidth <= 0 || cellHeight <= 0 {
		return GridLayout{}, fmt.Errorf("canvas %dx%d cannot fit %dx%d cells", canvasWidth, canvasHeight, columns, rows)
	}
	gridWidth := columns*cellWidth + (columns-1)*gutter
	gridHeight := rows*cellHeight + (rows-1)*gutter
	return GridLayout{
		Columns:      columns,
		Rows:         rows,
		CanvasWidth:  canvasWidth,
		CanvasHeight: canvasHeight,
		OffsetX:      (canvasWidth - gridWidth) / 2,
		OffsetY:      (canvasHeight - gridHeight) / 2,
		CellWidth:    cellWidth,
		CellHeight:   cellHeight,
		Gutter:       gutter,
		Count:        count,
	}, nil
}

// FixedCellGridLayout centers a fixed-size grid in the smallest 16px-aligned
// canvas that satisfies the configured minimum dimensions and outer margin.
func FixedCellGridLayout(count, columns, cell, outerMargin, gutter, minimumWidth, minimumHeight int) (GridLayout, error) {
	if count <= 0 || columns <= 0 || cell <= 0 || outerMargin < 0 || gutter < 0 || minimumWidth <= 0 || minimumHeight <= 0 {
		return GridLayout{}, fmt.Errorf("invalid fixed grid count=%d columns=%d cell=%d margin=%d gutter=%d minimum=%dx%d", count, columns, cell, outerMargin, gutter, minimumWidth, minimumHeight)
	}
	columns = min(columns, count)
	rows := (count + columns - 1) / columns
	gridWidth := columns*cell + max(0, columns-1)*gutter
	gridHeight := rows*cell + max(0, rows-1)*gutter
	canvasWidth := roundUp(max(minimumWidth, gridWidth+2*outerMargin), 16)
	canvasHeight := roundUp(max(minimumHeight, gridHeight+2*outerMargin), 16)
	return GridLayout{
		Columns:      columns,
		Rows:         rows,
		CanvasWidth:  canvasWidth,
		CanvasHeight: canvasHeight,
		OffsetX:      (canvasWidth - gridWidth) / 2,
		OffsetY:      (canvasHeight - gridHeight) / 2,
		CellWidth:    cell,
		CellHeight:   cell,
		Gutter:       gutter,
		Count:        count,
	}, nil
}

func roundUp(value, multiple int) int {
	return (value + multiple - 1) / multiple * multiple
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
	_, err := writeCanvasBoard(paths, outputPath, layout, guard, true)
	return err
}

// WriteCanvasBoardAtNativeScale composes canonical sprites without enlarging
// their occupied bounds. Sources are still reduced when required to fit the
// fixed cell guard.
func WriteCanvasBoardAtNativeScale(paths []string, outputPath string, layout GridLayout, guard int) error {
	_, err := writeCanvasBoard(paths, outputPath, layout, guard, false)
	return err
}

// WriteCanvasBoardAtConditioningScale enlarges accepted seed foreground by one
// shared deterministic factor so the provider sees readable identity evidence.
// Callers apply the exact inverse factor during canonical extraction.
func WriteCanvasBoardAtConditioningScale(paths []string, outputPath string, layout GridLayout, guard int) (float64, error) {
	return writeCanvasBoard(paths, outputPath, layout, guard, true)
}

// WriteCanvasBoardAtFrameScale places each complete source frame at its exact
// configured dimensions and preserves its transparent padding coordinates.
func WriteCanvasBoardAtFrameScale(paths []string, outputPath string, layout GridLayout, width, height int) error {
	return writeCanvasBoardAtFrameScale(paths, outputPath, layout, width, height, false)
}

// WriteCanvasBoardAtFrameScaleWithBlankCells composes an exact-scale edit
// target where empty source paths deliberately leave their cells transparent.
func WriteCanvasBoardAtFrameScaleWithBlankCells(paths []string, outputPath string, layout GridLayout, width, height int) error {
	return writeCanvasBoardAtFrameScale(paths, outputPath, layout, width, height, true)
}

func writeCanvasBoardAtFrameScale(paths []string, outputPath string, layout GridLayout, width, height int, allowBlank bool) error {
	if len(paths) != layout.Count {
		return fmt.Errorf("canvas board has %d sources for %d cells", len(paths), layout.Count)
	}
	board := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	for index, path := range paths {
		if path == "" && allowBlank {
			continue
		}
		source, err := decodeNRGBA(path)
		if err != nil {
			return fmt.Errorf("decode board source %q: %w", path, err)
		}
		if source.Bounds().Size() != image.Pt(width, height) {
			return fmt.Errorf("board source %q is %dx%d, expected %dx%d", path, source.Bounds().Dx(), source.Bounds().Dy(), width, height)
		}
		frame, err := centeredFrameRect(layout.Cell(index), width, height)
		if err != nil {
			return err
		}
		draw.Draw(board, frame, source, source.Bounds().Min, draw.Src)
	}
	return writePNG(outputPath, board)
}

func centeredFrameRect(cell image.Rectangle, width, height int) (image.Rectangle, error) {
	if width <= 0 || height <= 0 || width > cell.Dx() || height > cell.Dy() {
		return image.Rectangle{}, fmt.Errorf("frame %dx%d cannot fit cell %dx%d", width, height, cell.Dx(), cell.Dy())
	}
	left := cell.Min.X + (cell.Dx()-width)/2
	top := cell.Min.Y + (cell.Dy()-height)/2
	return image.Rect(left, top, left+width, top+height), nil
}

func writeCanvasBoard(paths []string, outputPath string, layout GridLayout, guard int, allowUpscale bool) (float64, error) {
	if len(paths) != layout.Count {
		return 0, fmt.Errorf("canvas board has %d sources for %d cells", len(paths), layout.Count)
	}
	if guard < 0 || guard*2 >= layout.CellWidth || guard*2 >= layout.CellHeight {
		return 0, fmt.Errorf("invalid canvas cell guard %d", guard)
	}
	sources := make([]*image.NRGBA, len(paths))
	bounds := make([]image.Rectangle, len(paths))
	maximumWidth, maximumHeight := 0, 0
	for index, path := range paths {
		source, err := decodeNRGBA(path)
		if err != nil {
			return 0, fmt.Errorf("decode board source %q: %w", path, err)
		}
		sources[index] = source
		bounds[index], err = alphaBounds(source)
		if err != nil {
			return 0, fmt.Errorf("board source %q: %w", path, err)
		}
		maximumWidth = max(maximumWidth, bounds[index].Dx())
		maximumHeight = max(maximumHeight, bounds[index].Dy())
	}
	availableWidth := layout.CellWidth - 2*guard
	availableHeight := layout.CellHeight - 2*guard
	scale := min(float64(availableWidth)/float64(maximumWidth), float64(availableHeight)/float64(maximumHeight))
	if !allowUpscale {
		scale = min(1, scale)
	}
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
	if err := writePNG(outputPath, board); err != nil {
		return 0, err
	}
	return scale, nil
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

// WriteBlankCanvas writes an empty transparent provider edit source.
func WriteBlankCanvas(path string, width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("blank canvas dimensions must be positive")
	}
	return writePNG(path, image.NewNRGBA(image.Rect(0, 0, width, height)))
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

// WriteCellInsetEditMask exposes only the inset area of one selected cell and
// preserves every other cell, boundary, gutter, and trailing slot.
func WriteCellInsetEditMask(outputPath string, layout GridLayout, editableCell, inset int) error {
	if editableCell < 0 || editableCell >= layout.Count {
		return fmt.Errorf("editable cell %d outside board count %d", editableCell, layout.Count)
	}
	if err := validateBoardEditMaskInset(layout, inset); err != nil {
		return err
	}
	mask := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	draw.Draw(mask, mask.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(mask, layout.Cell(editableCell).Inset(inset), image.Transparent, image.Point{}, draw.Src)
	return writePNG(outputPath, mask)
}

// RestoreProtectedBoardCells enforces cell-specific edit semantics after a
// provider call. Image edit masks are advisory for some providers, so every
// pixel outside the selected cell comes from the authoritative edit source.
// The selected cell remains untouched provider output for normal validation.
func RestoreProtectedBoardCells(generatedPath, editSourcePath, outputPath string, layout GridLayout, editableCell int) error {
	if editableCell < 0 || editableCell >= layout.Count {
		return fmt.Errorf("editable cell %d outside board count %d", editableCell, layout.Count)
	}
	generated, err := decodeNRGBA(generatedPath)
	if err != nil {
		return err
	}
	editSource, err := decodeNRGBA(editSourcePath)
	if err != nil {
		return err
	}
	wantBounds := image.Rect(0, 0, layout.Width(), layout.Height())
	if generated.Bounds() != wantBounds {
		return fmt.Errorf("generated board dimensions are %v, expected %v", generated.Bounds(), wantBounds)
	}
	if editSource.Bounds() != wantBounds {
		return fmt.Errorf("edit-source board dimensions are %v, expected %v", editSource.Bounds(), wantBounds)
	}
	restored := image.NewNRGBA(wantBounds)
	draw.Draw(restored, wantBounds, editSource, wantBounds.Min, draw.Src)
	cell := layout.Cell(editableCell)
	draw.Draw(restored, cell, generated, cell.Min, draw.Src)
	return writePNG(outputPath, restored)
}

// RestoreLockedBoardCell enforces one immutable cell after a provider edit
// while leaving every generated editable cell and board boundary untouched.
func RestoreLockedBoardCell(generatedPath, editSourcePath, outputPath string, layout GridLayout, lockedCell int) error {
	return RestoreLockedBoardCells(generatedPath, editSourcePath, outputPath, layout, []int{lockedCell})
}

// RestoreLockedBoardCells reasserts every immutable source cell after a
// provider edit while preserving generated pixels in all other cells.
func RestoreLockedBoardCells(generatedPath, editSourcePath, outputPath string, layout GridLayout, lockedCells []int) error {
	protected := make(map[int]bool, len(lockedCells))
	for _, lockedCell := range lockedCells {
		if lockedCell < 0 || lockedCell >= layout.Count {
			return fmt.Errorf("locked cell %d outside board count %d", lockedCell, layout.Count)
		}
		protected[lockedCell] = true
	}
	generated, err := decodeNRGBA(generatedPath)
	if err != nil {
		return err
	}
	editSource, err := decodeNRGBA(editSourcePath)
	if err != nil {
		return err
	}
	wantBounds := image.Rect(0, 0, layout.Width(), layout.Height())
	if generated.Bounds() != wantBounds {
		return fmt.Errorf("generated board dimensions are %v, expected %v", generated.Bounds(), wantBounds)
	}
	if editSource.Bounds() != wantBounds {
		return fmt.Errorf("edit-source board dimensions are %v, expected %v", editSource.Bounds(), wantBounds)
	}
	restored := image.NewNRGBA(wantBounds)
	draw.Draw(restored, wantBounds, generated, wantBounds.Min, draw.Src)
	for lockedCell := range protected {
		cell := layout.Cell(lockedCell)
		draw.Draw(restored, cell, editSource, cell.Min, draw.Src)
	}
	return writePNG(outputPath, restored)
}

// WriteBoardEditMask exposes all expected cells while preserving gutters,
// trailing cells, and the optional locked cell.
func WriteBoardEditMask(outputPath string, layout GridLayout, lockedCell int) error {
	return writeBoardEditMask(outputPath, layout, lockedCell, 0)
}

// WriteBoardInsetEditMask exposes only the inset target-placement area of each
// expected cell while preserving gutters, guards, trailing cells, and the
// optional locked cell.
func WriteBoardInsetEditMask(outputPath string, layout GridLayout, lockedCell, inset int) error {
	if err := validateBoardEditMaskInset(layout, inset); err != nil {
		return err
	}
	return writeBoardEditMask(outputPath, layout, lockedCell, inset)
}

// WriteBoardEditMaskTargets exposes explicit editable rectangles while keeping
// every target inside its assigned cell and preserving the optional lock.
func WriteBoardEditMaskTargets(outputPath string, layout GridLayout, lockedCell int, targets []image.Rectangle) error {
	if len(targets) != layout.Count {
		return fmt.Errorf("board edit mask has %d targets for %d cells", len(targets), layout.Count)
	}
	mask := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	draw.Draw(mask, mask.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
	for index := 0; index < layout.Count; index++ {
		if index == lockedCell {
			continue
		}
		cell := layout.Cell(index)
		target := targets[index]
		if target.Empty() || !target.In(cell) {
			return fmt.Errorf("board edit-mask target %d %v is not a non-empty subset of cell %v", index, target, cell)
		}
		draw.Draw(mask, target, image.Transparent, image.Point{}, draw.Src)
	}
	return writePNG(outputPath, mask)
}

// WriteBoardInsetEditMaskProtected exposes every inset expected cell except
// the explicitly protected cells.
func WriteBoardInsetEditMaskProtected(outputPath string, layout GridLayout, protectedCells []int, inset int) error {
	return WriteBoardRectInsetEditMaskProtected(outputPath, layout, protectedCells, inset, inset)
}

// WriteBoardRectInsetEditMaskProtected exposes rectangular target-placement
// areas while preserving explicitly protected cells and all board geometry.
func WriteBoardRectInsetEditMaskProtected(outputPath string, layout GridLayout, protectedCells []int, insetX, insetY int) error {
	if err := validateBoardEditMaskInsets(layout, insetX, insetY); err != nil {
		return err
	}
	protected := make(map[int]bool, len(protectedCells))
	for _, cell := range protectedCells {
		if cell < 0 || cell >= layout.Count {
			return fmt.Errorf("protected cell %d outside board count %d", cell, layout.Count)
		}
		protected[cell] = true
	}
	mask := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	draw.Draw(mask, mask.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
	for index := 0; index < layout.Count; index++ {
		if protected[index] {
			continue
		}
		cell := layout.Cell(index)
		target := image.Rect(cell.Min.X+insetX, cell.Min.Y+insetY, cell.Max.X-insetX, cell.Max.Y-insetY)
		draw.Draw(mask, target, image.Transparent, image.Point{}, draw.Src)
	}
	return writePNG(outputPath, mask)
}

func validateBoardEditMaskInset(layout GridLayout, inset int) error {
	return validateBoardEditMaskInsets(layout, inset, inset)
}

func validateBoardEditMaskInsets(layout GridLayout, insetX, insetY int) error {
	if insetX < 0 || insetY < 0 || insetX*2 >= layout.CellWidth || insetY*2 >= layout.CellHeight {
		return fmt.Errorf("invalid board edit-mask insets %dx%d for %dx%d cells", insetX, insetY, layout.CellWidth, layout.CellHeight)
	}
	return nil
}

func writeBoardEditMask(outputPath string, layout GridLayout, lockedCell, inset int) error {
	mask := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	draw.Draw(mask, mask.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
	for index := 0; index < layout.Count; index++ {
		if index == lockedCell {
			continue
		}
		draw.Draw(mask, layout.Cell(index).Inset(inset), image.Transparent, image.Point{}, draw.Src)
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
	// Accepted directional seeds reserve ten percent of the canonical frame on
	// every side so attack poses can expand without changing body scale.
	padding := max(1, min(width, height)/10)
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

// WriteWholeCellFrames reduces each complete fixed cell to the configured
// frame size. It never searches foreground bounds or changes per-frame scale.
func WriteWholeCellFrames(boardPath string, layout GridLayout, outputPaths []string, width, height int, palette []PaletteColor) error {
	if len(outputPaths) != layout.Count {
		return fmt.Errorf("whole-cell normalization has %d outputs for %d cells", len(outputPaths), layout.Count)
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("whole-cell frame size must be positive")
	}
	board, err := decodeNRGBA(boardPath)
	if err != nil {
		return err
	}
	if board.Bounds() != image.Rect(0, 0, layout.Width(), layout.Height()) {
		return fmt.Errorf("board dimensions do not match fixed layout")
	}
	for index, outputPath := range outputPaths {
		cell := copyCell(board, layout.Cell(index))
		frame := image.NewNRGBA(image.Rect(0, 0, width, height))
		areaScale(frame, frame.Bounds(), cell, cell.Bounds())
		frame = applyPalette(frame, palette)
		if err := writePNG(outputPath, frame); err != nil {
			return err
		}
		if err := ValidateCanonicalFrame(outputPath, width, height); err != nil {
			return fmt.Errorf("cell %02d: %w", index, err)
		}
	}
	return nil
}

// WriteGridCellCopies extracts every configured cell without resizing or
// palette conversion. It is used to send consistently scaled and aligned
// reference evidence derived from a deterministic board.
func WriteGridCellCopies(boardPath string, layout GridLayout, outputPaths []string) error {
	if len(outputPaths) != layout.Count {
		return fmt.Errorf("grid cell extraction has %d outputs for %d cells", len(outputPaths), layout.Count)
	}
	board, err := decodeNRGBA(boardPath)
	if err != nil {
		return err
	}
	if board.Bounds() != image.Rect(0, 0, layout.Width(), layout.Height()) {
		return fmt.Errorf("board dimensions do not match fixed layout")
	}
	for index, outputPath := range outputPaths {
		if err := writePNG(outputPath, copyCell(board, layout.Cell(index))); err != nil {
			return err
		}
	}
	return nil
}

type CanonicalTransform struct {
	Scale    float64 `json:"scale"`
	Baseline int     `json:"baseline"`
	CenterX  int     `json:"centerX"`
	OffsetX  int     `json:"offsetX,omitempty"`
	OffsetY  int     `json:"offsetY,omitempty"`
}

// WriteFixedFrameCells preserves the exact 1:1 scale established by the
// accepted seed's conditioned provider board, then bottom-centers every row
// frame at the seed baseline. Provider-returned frame 00 is not a reliable
// scale source because image edits may alter protected pixels. Foreground is
// never cropped or shrunk; a conditioned frame that cannot fit is rejected.
func WriteFixedFrameCells(boardPath string, layout GridLayout, outputPaths []string, width, height int, lockedPalette []PaletteColor, canonicalSeed string, lockedFirst bool) ([]PaletteColor, []CanonicalTransform, error) {
	return WriteFixedFrameCellsAtScale(boardPath, layout, outputPaths, width, height, lockedPalette, canonicalSeed, lockedFirst, 1)
}

// WriteFixedFrameCellsAtScale applies the fixed inverse of the provider
// conditioning scale to every generated cell. It never derives a fit scale
// from candidate output and still rejects any normalized foreground that crops.
func WriteFixedFrameCellsAtScale(boardPath string, layout GridLayout, outputPaths []string, width, height int, lockedPalette []PaletteColor, canonicalSeed string, lockedFirst bool, scale float64) ([]PaletteColor, []CanonicalTransform, error) {
	if len(outputPaths) != layout.Count {
		return nil, nil, fmt.Errorf("normalization has %d outputs for %d cells", len(outputPaths), layout.Count)
	}
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return nil, nil, fmt.Errorf("fixed frame scale must be finite and positive")
	}
	board, err := decodeNRGBA(boardPath)
	if err != nil {
		return nil, nil, err
	}
	if board.Bounds() != image.Rect(0, 0, layout.Width(), layout.Height()) {
		return nil, nil, fmt.Errorf("board dimensions do not match fixed layout")
	}
	seed, err := decodeNRGBA(canonicalSeed)
	if err != nil {
		return nil, nil, err
	}
	if seed.Bounds().Size() != image.Pt(width, height) {
		return nil, nil, fmt.Errorf("canonical seed is %dx%d, expected %dx%d", seed.Bounds().Dx(), seed.Bounds().Dy(), width, height)
	}
	seedBounds, err := alphaBounds(seed)
	if err != nil {
		return nil, nil, fmt.Errorf("canonical seed: %w", err)
	}
	cells := make([]*image.NRGBA, layout.Count)
	bounds := make([]image.Rectangle, layout.Count)
	for index := range cells {
		cells[index] = copyCell(board, layout.Cell(index))
		bounds[index], err = alphaBounds(cells[index])
		if err != nil {
			return nil, nil, fmt.Errorf("cell %02d: %w", index, err)
		}
	}
	canvas := image.Rect(0, 0, width, height)
	safeCanvas := canvas.Inset(CanonicalFrameEdgePadding(width, height))
	if safeCanvas.Empty() {
		return nil, nil, fmt.Errorf("fixed frame %v has no transparent-edge-safe area", canvas)
	}
	if !seedBounds.In(safeCanvas) {
		return nil, nil, fmt.Errorf("%w: canonical seed foreground %v contacts fixed frame edge %v", ErrProductionFrameClipping, seedBounds, canvas)
	}
	destinations := make([]image.Rectangle, layout.Count)
	transforms := make([]CanonicalTransform, layout.Count)
	for index := range cells {
		destinationWidth := max(1, int(math.Round(float64(bounds[index].Dx())*scale)))
		destinationHeight := max(1, int(math.Round(float64(bounds[index].Dy())*scale)))
		if destinationWidth > safeCanvas.Dx() || destinationHeight > safeCanvas.Dy() {
			return nil, nil, fmt.Errorf("%w: cell %02d canonical foreground size %dx%d cannot leave a transparent edge in fixed frame %dx%d", ErrProductionFrameClipping, index, destinationWidth, destinationHeight, width, height)
		}
		left := width/2 - destinationWidth/2
		destination := image.Rect(left, seedBounds.Max.Y-destinationHeight, left+destinationWidth, seedBounds.Max.Y)
		offsetX := minimumContainmentOffset(destination.Min.X, destination.Max.X, safeCanvas.Min.X, safeCanvas.Max.X)
		offsetY := minimumContainmentOffset(destination.Min.Y, destination.Max.Y, safeCanvas.Min.Y, safeCanvas.Max.Y)
		destination = destination.Add(image.Pt(offsetX, offsetY))
		if !destination.In(safeCanvas) {
			return nil, nil, fmt.Errorf("%w: cell %02d canonical destination %v cannot leave a transparent edge in fixed frame %v", ErrProductionFrameClipping, index, destination, canvas)
		}
		destinations[index] = destination
		transforms[index] = CanonicalTransform{Scale: scale, Baseline: seedBounds.Max.Y, CenterX: width / 2, OffsetX: offsetX, OffsetY: offsetY}
	}
	palette := append([]PaletteColor(nil), lockedPalette...)
	for index, outputPath := range outputPaths {
		if index == 0 && lockedFirst {
			if err := CopyFile(canonicalSeed, outputPath); err != nil {
				return nil, nil, err
			}
			if err := ValidateCanonicalFrame(outputPath, width, height); err != nil {
				return nil, nil, fmt.Errorf("cell %02d: %w", index, err)
			}
			transforms[index].OffsetX = 0
			transforms[index].OffsetY = 0
			continue
		}
		destination := image.NewNRGBA(image.Rect(0, 0, width, height))
		areaScale(destination, destinations[index], cells[index], bounds[index])
		if len(palette) == 0 {
			palette = extractPalette(destination, defaultPaletteSize)
		}
		destination = applyPalette(destination, palette)
		if err := writePNG(outputPath, destination); err != nil {
			return nil, nil, err
		}
		if err := ValidateCanonicalFrame(outputPath, width, height); err != nil {
			return nil, nil, fmt.Errorf("cell %02d: %w", index, err)
		}
	}
	return palette, transforms, nil
}

func minimumContainmentOffset(subjectMin, subjectMax, frameMin, frameMax int) int {
	minimum := frameMin - subjectMin
	maximum := frameMax - subjectMax
	if 0 < minimum {
		return minimum
	}
	if 0 > maximum {
		return maximum
	}
	return 0
}

// CanonicalFrameEdgePadding returns the transparent final-frame gutter that
// canonical normalization preserves without shrinking generated foreground.
func CanonicalFrameEdgePadding(_, _ int) int {
	return 1
}

// ValidateCanonicalFrame verifies the final encoded artifact, not only its
// planned placement. Palette conversion, file replacement, or later pipeline
// changes must never reintroduce foreground on the production canvas edge.
func ValidateCanonicalFrame(path string, width, height int) error {
	frame, err := decodeNRGBA(path)
	if err != nil {
		return err
	}
	canvas := image.Rect(0, 0, width, height)
	if frame.Bounds() != canvas {
		return fmt.Errorf("canonical frame dimensions are %dx%d, expected %dx%d", frame.Bounds().Dx(), frame.Bounds().Dy(), width, height)
	}
	foreground, err := alphaBounds(frame)
	if err != nil {
		return err
	}
	safeCanvas := canvas.Inset(CanonicalFrameEdgePadding(width, height))
	if !foreground.In(safeCanvas) {
		return fmt.Errorf("%w: final encoded foreground %v contacts fixed frame edge %v", ErrProductionFrameClipping, foreground, canvas)
	}
	return nil
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
			return nil, transform, fmt.Errorf("%w: cell %02d maps to %v outside %v", ErrProductionFrameClipping, index, destinations[index], canvas)
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

// ForegroundBounds returns the exact non-transparent bounds of a PNG without
// changing its pixels. Generation prompts use it to state accepted seed scale
// in final-frame coordinates.
func ForegroundBounds(path string) (image.Rectangle, error) {
	img, err := decodeNRGBA(path)
	if err != nil {
		return image.Rectangle{}, err
	}
	return alphaBounds(img)
}

type StudyMetrics struct {
	Cells            []Metrics          `json:"cells"`
	Registration     []CellRegistration `json:"registration,omitempty"`
	Score            float64            `json:"score"`
	CadenceDelta     float64            `json:"cadenceDelta"`
	MarginOccupied   bool               `json:"marginOccupied"`
	GutterOccupied   bool               `json:"gutterOccupied"`
	TrailingOccupied bool               `json:"trailingOccupied"`
}

// BoardValidationPurpose controls whether pose-guide differences are advisory
// or block fixed-cell extraction.
type BoardValidationPurpose uint8

const (
	// BoardValidationCharacterMaster keeps structural cell safety mandatory
	// while leaving identity and pose differences to complete-unit review.
	BoardValidationCharacterMaster BoardValidationPurpose = iota + 1
	// BoardValidationAnimationBoard keeps structural safety mandatory while
	// recording appearance, pose, and cadence differences for human review.
	BoardValidationAnimationBoard
)

// BoardEvaluation separates non-overridable extraction failures from visual
// differences that a human unit reviewer may intentionally accept.
type BoardEvaluation struct {
	Metrics          StudyMetrics
	BlockingFailures []string
	Warnings         []string
}

// EvaluateBoard validates a generated board according to its pipeline role.
func EvaluateBoard(candidatePath, guideBoardPath string, layout GridLayout, guard int, purpose BoardValidationPurpose) (BoardEvaluation, error) {
	if purpose != BoardValidationCharacterMaster && purpose != BoardValidationAnimationBoard {
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
	gutterFailures := boardGutterFailures(candidate, layout)
	evaluation := BoardEvaluation{Metrics: StudyMetrics{
		MarginOccupied:   boardMarginOccupied(candidate, layout),
		GutterOccupied:   len(gutterFailures) != 0,
		TrailingOccupied: trailingCellsOccupied(candidate, layout),
	}}
	if evaluation.Metrics.MarginOccupied {
		evaluation.BlockingFailures = append(evaluation.BlockingFailures, "board_margin_occupied")
	}
	if evaluation.Metrics.GutterOccupied {
		evaluation.BlockingFailures = append(evaluation.BlockingFailures, gutterFailures...)
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
		evaluation.Warnings = append(evaluation.Warnings, "adjacent_cell_cadence_difference")
	}
	return evaluation, nil
}

// EvaluateMotionStudy compares board cells in memory for QA only. No extracted
// cell is written or exposed as a target candidate.
func EvaluateMotionStudy(candidatePath, poseBoardPath string, layout GridLayout, guard int) (StudyMetrics, []string, error) {
	evaluation, err := EvaluateBoard(candidatePath, poseBoardPath, layout, guard, BoardValidationAnimationBoard)
	return evaluation.Metrics, append(evaluation.BlockingFailures, evaluation.Warnings...), err
}

func boardCellFindings(metrics Metrics, purpose BoardValidationPurpose) ([]string, []string) {
	var blocking []string
	if metrics.Components != 1 {
		blocking = append(blocking, fmt.Sprintf("foreground_components_%d", metrics.Components))
	}
	if metrics.CellEdgeOccupied {
		blocking = append(blocking, "cell_edge_occupied")
	}
	if metrics.BackdropLike {
		blocking = append(blocking, "non_removable_background")
	}
	var poseDifferences []string
	if metrics.EdgeGuardOccupied {
		poseDifferences = append(poseDifferences, "edge_guard_occupied")
	}
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
	return len(boardGutterFailures(img, layout)) != 0
}

func boardGutterFailures(img *image.NRGBA, layout GridLayout) []string {
	bounds := img.Bounds()
	seen := make([]bool, bounds.Dx()*bounds.Dy())
	affected := make([]bool, layout.Count)
	global := false
	grid := layoutGridBounds(layout)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			startIndex := (y-bounds.Min.Y)*bounds.Dx() + x - bounds.Min.X
			if seen[startIndex] || img.NRGBAAt(x, y).A == 0 {
				continue
			}
			queue := []image.Point{{X: x, Y: y}}
			seen[startIndex] = true
			componentCells := make([]bool, layout.Count)
			var gutterPoints []image.Point
			for head := 0; head < len(queue); head++ {
				point := queue[head]
				if cell := expectedCellAt(layout, point); cell >= 0 {
					componentCells[cell] = true
				} else if point.In(grid) && !layoutContains(layout, point.X, point.Y) {
					gutterPoints = append(gutterPoints, point)
				}
				for _, next := range [...]image.Point{
					{X: point.X - 1, Y: point.Y - 1}, {X: point.X, Y: point.Y - 1}, {X: point.X + 1, Y: point.Y - 1},
					{X: point.X - 1, Y: point.Y}, {X: point.X + 1, Y: point.Y},
					{X: point.X - 1, Y: point.Y + 1}, {X: point.X, Y: point.Y + 1}, {X: point.X + 1, Y: point.Y + 1},
				} {
					if !next.In(bounds) || img.NRGBAAt(next.X, next.Y).A == 0 {
						continue
					}
					nextIndex := (next.Y-bounds.Min.Y)*bounds.Dx() + next.X - bounds.Min.X
					if !seen[nextIndex] {
						seen[nextIndex] = true
						queue = append(queue, next)
					}
				}
			}
			if len(gutterPoints) == 0 {
				continue
			}
			owned := false
			for cell, present := range componentCells {
				if present {
					affected[cell] = true
					owned = true
				}
			}
			if owned {
				continue
			}
			nearest := nearestExpectedCells(layout, gutterPoints)
			if len(nearest) == 0 {
				global = true
				continue
			}
			for _, cell := range nearest {
				affected[cell] = true
			}
		}
	}
	var failures []string
	if global {
		failures = append(failures, "board_gutter_occupied")
	}
	for cell, occupied := range affected {
		if occupied {
			failures = append(failures, fmt.Sprintf("cell_%02d_gutter_occupied", cell))
		}
	}
	return failures
}

func expectedCellAt(layout GridLayout, point image.Point) int {
	for index := 0; index < layout.Count; index++ {
		if point.In(layout.Cell(index)) {
			return index
		}
	}
	return -1
}

func nearestExpectedCells(layout GridLayout, points []image.Point) []int {
	maximum := int(^uint(0) >> 1)
	best := maximum
	var nearest []int
	for cell := 0; cell < layout.Count; cell++ {
		distance := maximum
		for _, point := range points {
			distance = min(distance, pointRectangleDistanceSquared(point, layout.Cell(cell)))
		}
		switch {
		case distance < best:
			best = distance
			nearest = []int{cell}
		case distance == best:
			nearest = append(nearest, cell)
		}
	}
	return nearest
}

func pointRectangleDistanceSquared(point image.Point, rectangle image.Rectangle) int {
	dx, dy := 0, 0
	if point.X < rectangle.Min.X {
		dx = rectangle.Min.X - point.X
	} else if point.X >= rectangle.Max.X {
		dx = point.X - rectangle.Max.X + 1
	}
	if point.Y < rectangle.Min.Y {
		dy = rectangle.Min.Y - point.Y
	} else if point.Y >= rectangle.Max.Y {
		dy = point.Y - rectangle.Max.Y + 1
	}
	return dx*dx + dy*dy
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
