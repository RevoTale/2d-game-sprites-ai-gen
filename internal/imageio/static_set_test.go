package imageio

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCanvasRegisteredTransparentStaticSetRejectsOccupiedEdge(t *testing.T) {
	t.Parallel()

	const extent = 64
	layout, err := SemanticStaticSetLayout([]image.Point{image.Pt(extent, extent)})
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := semanticSizedBounds(layout, []image.Point{image.Pt(extent, extent)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, bounds[0], color.NRGBA{R: 50, G: 90, B: 40, A: 255})
	dir := t.TempDir()
	boardPath := filepath.Join(dir, "board.png")
	if err := writePNG(boardPath, board); err != nil {
		t.Fatal(err)
	}

	_, err = WriteCanvasRegisteredTransparentStaticSet(
		boardPath,
		layout,
		[]image.Point{image.Pt(extent, extent)},
		[]StaticSetPart{{
			ID:         "moss-a",
			OutputPath: filepath.Join(dir, "moss-a.png"),
			Size:       image.Pt(extent, extent),
		}},
		[]PaletteColor{{R: 50, G: 90, B: 40}},
	)

	if err == nil || !strings.Contains(err.Error(), "transparent perimeter") {
		t.Fatalf("occupied edge error = %v, want transparent perimeter rejection", err)
	}
}
