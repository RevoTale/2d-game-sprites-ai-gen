package imageio_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/stretchr/testify/require"
)

func TestWriteNormalizedPNGDownscalesLargerProviderCanvasToTargetSize(t *testing.T) {
	var raw bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: 120, A: 255})
		}
	}
	require.NoError(t, png.Encode(&raw, img))
	out := t.TempDir() + "/normalized.png"

	require.NoError(t, imageio.WriteNormalizedPNG(out, raw.Bytes(), 16, 16))

	file, err := os.Open(out)
	require.NoError(t, err)
	defer file.Close()
	normalized, err := png.Decode(file)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 16, 16), normalized.Bounds())
}

func TestWriteNormalizedPNGPreservesAspectRatioAndPadsNonSquareTargets(t *testing.T) {
	var raw bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 220, G: 40, B: 80, A: 255})
		}
	}
	require.NoError(t, png.Encode(&raw, img))
	out := t.TempDir() + "/normalized.png"

	require.NoError(t, imageio.WriteNormalizedPNG(out, raw.Bytes(), 32, 16))

	file, err := os.Open(out)
	require.NoError(t, err)
	defer file.Close()
	normalized, err := png.Decode(file)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 32, 16), normalized.Bounds())
	require.Equal(t, color.NRGBA{}, color.NRGBAModel.Convert(normalized.At(0, 8)))
	require.Equal(t, color.NRGBA{R: 220, G: 40, B: 80, A: 255}, color.NRGBAModel.Convert(normalized.At(16, 8)))
}
