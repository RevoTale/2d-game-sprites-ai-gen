package imageio_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/stretchr/testify/require"
)

func TestMeasureStaticEvidenceRecordsOpacityEdgesAndContrast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tile.png")
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 40), G: uint8(y * 30), B: 20, A: 255})
		}
	}
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(file, img))
	require.NoError(t, file.Close())

	evidence, err := imageio.MeasureStaticEvidence(path)

	require.NoError(t, err)
	require.Equal(t, 1.0, evidence.OpaqueRatio)
	require.Positive(t, evidence.HorizontalEdgeDelta)
	require.Positive(t, evidence.VerticalEdgeDelta)
	require.GreaterOrEqual(t, evidence.MaximumHorizontalEdgeDelta, evidence.HorizontalEdgeDelta)
	require.GreaterOrEqual(t, evidence.MaximumVerticalEdgeDelta, evidence.VerticalEdgeDelta)
	require.Positive(t, evidence.LuminanceRange)
	require.Positive(t, evidence.SmallClusterRatio)
}

func TestMeasureStaticEvidenceRecognizesOneConnectedColorPlane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plane.png")
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetNRGBA(x, y, color.NRGBA{R: 40, G: 70, B: 50, A: 255})
		}
	}
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(file, img))
	require.NoError(t, file.Close())

	evidence, err := imageio.MeasureStaticEvidence(path)

	require.NoError(t, err)
	require.Zero(t, evidence.HorizontalEdgeDelta)
	require.Zero(t, evidence.VerticalEdgeDelta)
	require.Zero(t, evidence.SmallClusterRatio)
}
