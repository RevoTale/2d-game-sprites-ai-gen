package imageio

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSemanticAnimationLayoutUsesLogicalAnchorsWithoutCellGutters(t *testing.T) {
	layout, err := SemanticAnimationLayout(3, 4)

	require.NoError(t, err)
	require.Equal(t, image.Pt(1536, 1152), layout.Canvas())
	require.Equal(t, 384, layout.AnchorSpacing)
	require.Equal(t, []image.Point{
		{X: 192, Y: 192}, {X: 576, Y: 192}, {X: 960, Y: 192}, {X: 1344, Y: 192},
		{X: 192, Y: 576}, {X: 576, Y: 576}, {X: 960, Y: 576}, {X: 1344, Y: 576},
		{X: 192, Y: 960}, {X: 576, Y: 960}, {X: 960, Y: 960}, {X: 1344, Y: 960},
	}, layout.Anchors)
}

func TestWriteSemanticBoardAtNativeScaleUsesOneReductionForAsymmetricOuterPose(t *testing.T) {
	layout, err := SemanticAnimationLayout(3, 4)
	require.NoError(t, err)

	source := image.NewNRGBA(image.Rect(0, 0, 256, 224))
	fillOpaque(source, image.Rect(30, 20, 100, 224), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(source, image.Rect(95, 100, 254, 116), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "asymmetric-pose.png")
	require.NoError(t, writePNG(sourcePath, source))
	paths := make([]string, len(layout.Anchors))
	for index := range paths {
		paths[index] = sourcePath
	}
	outputPath := filepath.Join(dir, "board.png")

	require.NoError(t, WriteSemanticBoardAtNativeScale(paths, outputPath, layout, 256))
	board, err := decodeNRGBA(outputPath)
	require.NoError(t, err)
	foreground, err := alphaBounds(board)
	require.NoError(t, err)
	require.True(t, foreground.In(board.Bounds().Inset(semanticCanvasGuard)))
}

func TestRecoverSemanticPosesKeepsConnectedSwordAcrossLogicalMidpoint(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(340, 485, 545, 500), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Len(t, poses, 2)
	require.Greater(t, poses[0].Bounds.Max.X, 512)
	require.Equal(t, 0, poses[0].Index)
	require.Equal(t, 1, poses[1].Index)
}

func TestRecoverSemanticPosesAttachesUnambiguousDetachedEquipment(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(235, 470, 270, 550), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Len(t, poses[0].Components, 2)
	require.Equal(t, image.Rect(235, 400, 345, 610), poses[0].Bounds)
}

func TestRecoverSemanticPosesDoesNotMistakeLargeDetachedShieldForBody(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(110, 510, 270, 610), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Equal(t, image.Rect(280, 400, 345, 610), poses[0].CoreBounds)
	require.Len(t, poses[0].Components, 2)
}

func TestRecoverSemanticPosesRejectsAmbiguousDetachedComponent(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(500, 480, 515, 500), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	_, err := RecoverSemanticPoses(path, layout, nil)

	require.ErrorContains(t, err, "ambiguous ownership")
}

func TestRecoverSemanticPosesRejectsMergedPrimaryBodies(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(340, 500, 675, 510), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	_, err := RecoverSemanticPoses(path, layout, nil)

	require.ErrorContains(t, err, "primary body cores")
}

func TestRecoverSemanticPosesKeepsBodyPivotStableWithWideConnectedWeapon(t *testing.T) {
	layout, err := SemanticAnimationLayout(1, 1)
	require.NoError(t, err)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(480, 400, 540, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(535, 450, 835, 500), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.InDelta(t, 510, poses[0].Pivot.X, 3)
	require.Equal(t, 610, poses[0].Pivot.Y)
}

func TestSemanticUnitRegistrationUsesOneScaleAndStableBodyPivot(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 600), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(340, 480, 470, 495), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 600), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	dir := t.TempDir()
	boardPath := filepath.Join(dir, "board.png")
	require.NoError(t, writePNG(boardPath, board))
	posePaths := []string{filepath.Join(dir, "pose-0.png"), filepath.Join(dir, "pose-1.png")}
	poses, err := RecoverSemanticPoses(boardPath, layout, posePaths)
	require.NoError(t, err)

	transform, err := FitSemanticUnitTransform(poses, 160, 160)
	require.NoError(t, err)
	outputs := []string{filepath.Join(dir, "frame-0.png"), filepath.Join(dir, "frame-1.png")}
	evidence, err := WriteRegisteredSemanticPoses(posePaths, poses, outputs, 160, 160, nil, transform)

	require.NoError(t, err)
	require.Len(t, evidence, 2)
	require.Equal(t, evidence[0].Scale, evidence[1].Scale)
	require.Equal(t, evidence[0].CenterX, evidence[1].CenterX)
	require.Equal(t, evidence[0].Baseline, evidence[1].Baseline)
	require.FileExists(t, outputs[0])
	require.FileExists(t, outputs[1])
}

func TestSemanticUnitRegistrationFitsCompleteEnvelopeWithOneScale(t *testing.T) {
	poses := []SemanticPose{
		{
			Index: 0, Bounds: image.Rect(0, 0, 120, 200),
			CoreBounds: image.Rect(20, 0, 100, 200), Pivot: image.Pt(60, 200),
		},
		{
			Index: 1, Bounds: image.Rect(-300, 0, 400, 200),
			CoreBounds: image.Rect(20, 0, 100, 200), Pivot: image.Pt(60, 200),
		},
	}

	transform, err := FitSemanticUnitTransform(poses, 160, 160)

	require.NoError(t, err)
	require.Less(t, transform.Scale, 0.5)
	safe := image.Rect(0, 0, 160, 160).Inset(CanonicalFrameEdgePadding(160, 160))
	for _, pose := range poses {
		require.True(t, semanticPoseDestination(pose, transform).In(safe))
	}
}

func twoPoseSemanticLayout(t *testing.T) SemanticLayout {
	t.Helper()
	layout, err := SemanticAnimationLayout(1, 2)
	require.NoError(t, err)
	return layout
}

func fillOpaque(img *image.NRGBA, bounds image.Rectangle, value color.NRGBA) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetNRGBA(x, y, value)
		}
	}
}
