package imageio

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteEditableMaskedBoardEnforcesMaterialOwnership(t *testing.T) {
	t.Parallel()

	bounds := image.Rect(0, 0, 12, 6)
	raw := image.NewNRGBA(bounds)
	draw.Draw(raw, bounds, image.NewUniform(color.NRGBA{R: 70, G: 60, B: 50, A: 255}), image.Point{}, draw.Src)
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, raw))

	mask := image.NewNRGBA(bounds)
	draw.Draw(mask, bounds, image.NewUniform(color.NRGBA{A: 255}), image.Point{}, draw.Src)
	left := image.Rect(1, 1, 5, 5)
	right := image.Rect(7, 1, 11, 5)
	draw.Draw(mask, left, image.NewUniform(color.NRGBA{}), image.Point{}, draw.Src)
	draw.Draw(mask, right, image.NewUniform(color.NRGBA{}), image.Point{}, draw.Src)
	maskPath := filepath.Join(t.TempDir(), "mask.png")
	require.NoError(t, writePNG(maskPath, mask))

	outputPath := filepath.Join(t.TempDir(), "owned.png")
	require.NoError(t, WriteEditableMaskedBoard(
		outputPath,
		encoded.Bytes(),
		maskPath,
		bounds.Dx(),
		bounds.Dy(),
	))
	result, err := decodeNRGBA(outputPath)
	require.NoError(t, err)
	require.Equal(t, uint8(255), result.NRGBAAt(2, 2).A)
	require.Equal(t, uint8(255), result.NRGBAAt(8, 2).A)
	require.Zero(t, result.NRGBAAt(0, 0).A)
	require.Zero(t, result.NRGBAAt(6, 2).A)
}
