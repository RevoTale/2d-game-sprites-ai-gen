package imageio

import (
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSemanticAnimationLayoutUsesLogicalAnchorsWithOuterReserve(t *testing.T) {
	layout, err := SemanticAnimationLayout(3, 4)

	require.NoError(t, err)
	require.Equal(t, image.Pt(1664, 1280), layout.Canvas())
	require.Equal(t, 384, layout.AnchorSpacing)
	require.Equal(t, []image.Point{
		{X: 256, Y: 256}, {X: 640, Y: 256}, {X: 1024, Y: 256}, {X: 1408, Y: 256},
		{X: 256, Y: 640}, {X: 640, Y: 640}, {X: 1024, Y: 640}, {X: 1408, Y: 640},
		{X: 256, Y: 1024}, {X: 640, Y: 1024}, {X: 1024, Y: 1024}, {X: 1408, Y: 1024},
	}, layout.Anchors)
}

func TestSemanticStaticSetLayoutFitsNativeTargetCanvases(t *testing.T) {
	sizes := []image.Point{
		{X: 384, Y: 320}, {X: 320, Y: 384},
		{X: 384, Y: 384}, {X: 384, Y: 384},
		{X: 384, Y: 224}, {X: 224, Y: 352},
		{X: 352, Y: 288}, {X: 352, Y: 288},
		{X: 352, Y: 352}, {X: 288, Y: 192},
		{X: 288, Y: 192},
	}

	layout, err := SemanticStaticSetLayout(sizes)

	require.NoError(t, err)
	require.Equal(t, image.Pt(1856, 1408), layout.Canvas())
	require.Equal(t, 448, layout.AnchorSpacing)
	require.Equal(t, image.Pt(256, 448), layout.Anchors[0])
	require.Equal(t, image.Pt(1600, 448), layout.Anchors[3])
	require.Equal(t, image.Pt(1152, 1344), layout.Anchors[10])
}

func TestWriteSemanticPlaceholderBoardCreatesOneSeparatedMarkerPerAnchor(t *testing.T) {
	layout, err := SemanticAnimationLayout(1, 2)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "layout-source.png")

	require.NoError(t, WriteSemanticPlaceholderBoard(
		path,
		layout,
		[]image.Point{{X: 96, Y: 80}, {X: 80, Y: 64}},
		256,
	))

	board, err := decodeNRGBA(path)
	require.NoError(t, err)
	require.Equal(t, layout.Canvas(), board.Bounds().Size())
	poses, err := RecoverSemanticPoses(path, layout, nil)
	require.NoError(t, err)
	require.Len(t, poses, 2)
	require.Less(t, poses[0].Bounds.Max.X, poses[1].Bounds.Min.X)
	for index, pose := range poses {
		require.Equal(t, index, pose.Index)
		require.Equal(t, layout.Anchors[index].Y, pose.Bounds.Max.Y)
	}
}

func TestWriteSemanticEditMaskCreatesSeparatedTransparentRegions(t *testing.T) {
	layout, err := SemanticAnimationLayout(1, 2)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "edit-mask.png")

	require.NoError(t, WriteSemanticEditMask(
		path,
		layout,
		[]image.Point{{X: 96, Y: 80}, {X: 80, Y: 64}},
		256,
		24,
	))

	mask, err := decodeNRGBA(path)
	require.NoError(t, err)
	require.Equal(t, layout.Canvas(), mask.Bounds().Size())
	require.Zero(t, mask.NRGBAAt(layout.Anchors[0].X, layout.Anchors[0].Y-1).A)
	require.Zero(t, mask.NRGBAAt(layout.Anchors[1].X, layout.Anchors[1].Y-1).A)
	middle := image.Pt(
		(layout.Anchors[0].X+layout.Anchors[1].X)/2,
		layout.Anchors[0].Y,
	)
	require.Equal(t, uint8(255), mask.NRGBAAt(middle.X, middle.Y).A)
}

func TestWriteSemanticEditMaskFitsDenseElevenPartLayoutSafetyReserve(t *testing.T) {
	layout, err := SemanticMasterLayout(11)
	require.NoError(t, err)
	sizes := []image.Point{
		{X: 192, Y: 160}, {X: 160, Y: 192},
		{X: 192, Y: 192}, {X: 192, Y: 192},
		{X: 192, Y: 112}, {X: 112, Y: 176},
		{X: 176, Y: 144}, {X: 176, Y: 144},
		{X: 176, Y: 176}, {X: 144, Y: 96},
		{X: 144, Y: 96},
	}
	path := filepath.Join(t.TempDir(), "edit-mask.png")

	require.NoError(t, WriteSemanticEditMask(path, layout, sizes, 208, 24))

	mask, err := decodeNRGBA(path)
	require.NoError(t, err)
	require.Equal(t, uint8(255), mask.NRGBAAt(7, 7).A)
	for _, anchor := range layout.Anchors {
		require.Zero(t, mask.NRGBAAt(anchor.X, anchor.Y-1).A)
	}
}

func TestWriteSemanticSizedEditMaskUsesEachProductionSafeBounds(t *testing.T) {
	sizes := []image.Point{{X: 384, Y: 320}, {X: 224, Y: 352}}
	layout, err := SemanticStaticSetLayout(sizes)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "edit-mask.png")

	require.NoError(t, WriteSemanticSizedEditMask(path, layout, sizes))

	mask, err := decodeNRGBA(path)
	require.NoError(t, err)
	for _, anchor := range layout.Anchors {
		require.Zero(t, mask.NRGBAAt(anchor.X, anchor.Y-1).A)
	}
	middle := (layout.Anchors[0].X + layout.Anchors[1].X) / 2
	require.Equal(t, uint8(255), mask.NRGBAAt(middle, layout.Anchors[0].Y-1).A)
}

func TestSemanticStaticSetLayoutUsesProviderMinimumCanvasForSmallParts(t *testing.T) {
	t.Parallel()

	layout, err := SemanticStaticSetLayout([]image.Point{
		image.Pt(128, 128),
		image.Pt(128, 128),
		image.Pt(128, 128),
	})
	require.NoError(t, err)
	require.Equal(t, image.Pt(1024, 1024), layout.Canvas())

	maskPath := filepath.Join(t.TempDir(), "mask.png")
	require.NoError(t, WriteSemanticSizedEditMaskWithGuard(
		maskPath,
		layout,
		[]image.Point{image.Pt(128, 128), image.Pt(128, 128), image.Pt(128, 128)},
		0,
	))
	dimensions, err := PNGDimensions(maskPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(1024, 1024), dimensions)
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

func TestSemanticPrimaryComponentIgnoresGroundedPixelNoise(t *testing.T) {
	components := []semanticComponent{
		{area: 1, pivot: image.Pt(320, 610)},
		{area: 18000, pivot: image.Pt(365, 642)},
		{area: 3, pivot: image.Pt(312, 608)},
	}

	require.Equal(t, 1, semanticPrimaryComponent(components, image.Pt(320, 512)))
}

func TestRecoverSemanticPosesRejectsAmbiguousDetachedComponent(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(505, 480, 520, 500), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	_, err := RecoverSemanticPoses(path, layout, nil)

	require.ErrorContains(t, err, "ambiguous ownership")
}

func TestRecoverSemanticPosesIgnoresTinyAmbiguousChromaResidue(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(507, 480, 518, 489), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Len(t, poses[0].Components, 1)
	require.Len(t, poses[1].Components, 1)
}

func TestRecoverSemanticPosesKeepsTinyUnambiguousDetachedDetail(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(250, 580, 258, 588), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Len(t, poses[0].Components, 2)
	require.Len(t, poses[1].Components, 1)
}

func TestRecoverSemanticPosesAssignsDetachedComponentOutsideExactBisector(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 345, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(670, 400, 735, 610), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(513, 480, 518, 500), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Len(t, poses[0].Components, 1)
	require.Len(t, poses[1].Components, 2)
}

func TestRecoverSemanticPosesAttachesSmallTrailingFragmentToNearbyGroup(t *testing.T) {
	layout, err := SemanticMasterLayout(11)
	require.NoError(t, err)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	for _, anchor := range layout.Anchors[:10] {
		fillOpaque(
			board,
			image.Rect(anchor.X-50, anchor.Y-100, anchor.X+50, anchor.Y),
			color.NRGBA{R: 30, G: 80, B: 180, A: 255},
		)
	}
	last := layout.Anchors[10]
	fillOpaque(
		board,
		image.Rect(last.X-74, last.Y-100, last.X+366, last.Y),
		color.NRGBA{R: 30, G: 80, B: 180, A: 255},
	)
	fillOpaque(
		board,
		image.Rect(last.X+376, last.Y+26, last.X+390, last.Y+38),
		color.NRGBA{R: 220, G: 190, B: 80, A: 255},
	)
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Len(t, poses, 11)
	require.Len(t, poses[10].Components, 2)
	require.Equal(t, image.Rect(last.X-74, last.Y-100, last.X+390, last.Y+38), poses[10].Bounds)
}

func TestRecoverSemanticPosesRejectsDistantSmallTrailingFragment(t *testing.T) {
	layout, err := SemanticMasterLayout(3)
	require.NoError(t, err)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	for _, anchor := range layout.Anchors {
		fillOpaque(
			board,
			image.Rect(anchor.X-50, anchor.Y-100, anchor.X+50, anchor.Y),
			color.NRGBA{R: 30, G: 80, B: 180, A: 255},
		)
	}
	fillOpaque(
		board,
		image.Rect(layout.CanvasWidth-100, layout.CanvasHeight-100, layout.CanvasWidth-90, layout.CanvasHeight-90),
		color.NRGBA{R: 220, G: 190, B: 80, A: 255},
	)
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	_, err = RecoverSemanticPoses(path, layout, nil)

	require.ErrorContains(t, err, "no pose ownership")
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

func TestRecoverSemanticPosesAllowsOverlappingBoundsForSeparatedPoses(t *testing.T) {
	layout := twoPoseSemanticLayout(t)
	board := image.NewNRGBA(image.Rect(0, 0, layout.CanvasWidth, layout.CanvasHeight))
	fillOpaque(board, image.Rect(280, 400, 340, 620), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(335, 420, 590, 440), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	fillOpaque(board, image.Rect(670, 400, 730, 620), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	fillOpaque(board, image.Rect(500, 580, 675, 600), color.NRGBA{R: 220, G: 190, B: 80, A: 255})
	path := filepath.Join(t.TempDir(), "board.png")
	require.NoError(t, writePNG(path, board))

	poses, err := RecoverSemanticPoses(path, layout, nil)

	require.NoError(t, err)
	require.Len(t, poses, 2)
	require.True(t, poses[0].Bounds.Overlaps(poses[1].Bounds))
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

func TestCanonicalUnitRegistrationUsesOneScaleAndStableBodyPivot(t *testing.T) {
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

	referencePaths := []string{filepath.Join(dir, "reference-0.png"), filepath.Join(dir, "reference-1.png")}
	for index := range posePaths {
		writePaddedReference(t, posePaths[index], referencePaths[index])
	}
	profile, err := BuildCanonicalSubjectProfile(referencePaths, SubjectRegistrationGrounded, CanonicalScaleClassReferenceStable, 320)
	require.NoError(t, err)
	transform, err := FitCanonicalSubjectTransform(profile, posePaths, poses, 320, 320)
	require.NoError(t, err)
	outputs := []string{filepath.Join(dir, "frame-0.png"), filepath.Join(dir, "frame-1.png")}
	evidence, err := WriteRegisteredSemanticPoses(
		posePaths,
		poses,
		outputs,
		320,
		320,
		nil,
		transform,
		1,
	)

	require.NoError(t, err)
	require.Len(t, evidence, 2)
	require.Equal(t, evidence[0].Scale, evidence[1].Scale)
	require.Equal(t, profile.ReferencePivots[0], transform.DirectionAnchors[0])
	require.Equal(t, profile.ReferencePivots[1], transform.DirectionAnchors[1])
	require.FileExists(t, outputs[0])
	require.FileExists(t, outputs[1])
}

func TestCanonicalProfileUsesAbsoluteStandardHumanoidHeightWithoutCompounding(t *testing.T) {
	dir := t.TempDir()
	referencePath := filepath.Join(dir, "reference.png")
	masterPath := filepath.Join(dir, "master.png")
	for _, path := range []string{referencePath, masterPath} {
		sprite := image.NewNRGBA(image.Rect(0, 0, 384, 384))
		fillOpaque(
			sprite,
			image.Rect(142, 92, 242, 292),
			color.NRGBA{R: 30, G: 80, B: 180, A: 255},
		)
		require.NoError(t, writePNG(path, sprite))
	}
	profile, err := BuildCanonicalSubjectProfile(
		[]string{referencePath},
		SubjectRegistrationGrounded,
		CanonicalScaleClassStandardHumanoid,
		384,
	)
	require.NoError(t, err)
	pose := SemanticPose{
		Bounds:     image.Rect(142, 92, 242, 292),
		CoreBounds: image.Rect(142, 92, 242, 292),
		Pivot:      image.Pt(192, 292),
	}

	transform, err := FitCanonicalSubjectTransform(
		profile,
		[]string{masterPath},
		[]SemanticPose{pose},
		384,
		384,
	)

	require.NoError(t, err)
	require.Equal(t, 180, profile.TargetNeutralHeight)
	require.InDelta(t, 0.9, transform.Scale, 0.0001)
	require.Equal(t, image.Pt(192, 292), transform.DirectionAnchors[0])

	normalizedReferencePath := filepath.Join(dir, "normalized-reference.png")
	normalized := image.NewNRGBA(image.Rect(0, 0, 384, 384))
	fillOpaque(normalized, image.Rect(147, 102, 237, 282), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	require.NoError(t, writePNG(normalizedReferencePath, normalized))
	stable, err := BuildCanonicalSubjectProfile(
		[]string{normalizedReferencePath},
		SubjectRegistrationGrounded,
		CanonicalScaleClassStandardHumanoid,
		384,
	)
	require.NoError(t, err)
	stablePose := SemanticPose{Bounds: image.Rect(147, 102, 237, 282), CoreBounds: image.Rect(147, 102, 237, 282), Pivot: image.Pt(192, 282)}
	stableTransform, err := FitCanonicalSubjectTransform(stable, []string{normalizedReferencePath}, []SemanticPose{stablePose}, 384, 384)
	require.NoError(t, err)
	require.InDelta(t, 1, stableTransform.Scale, 0.0001)
}

func TestCanonicalProfileReferenceStablePreservesCurrentHeightExactly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reference.png")
	sprite := image.NewNRGBA(image.Rect(0, 0, 384, 384))
	fillOpaque(sprite, image.Rect(102, 76, 282, 276), color.NRGBA{R: 90, G: 60, B: 160, A: 255})
	require.NoError(t, writePNG(path, sprite))

	profile, err := BuildCanonicalSubjectProfile(
		[]string{path},
		SubjectRegistrationGrounded,
		CanonicalScaleClassReferenceStable,
		384,
	)
	require.NoError(t, err)
	pose := SemanticPose{Bounds: image.Rect(102, 76, 282, 276), CoreBounds: image.Rect(102, 76, 282, 276), Pivot: image.Pt(192, 276)}
	transform, err := FitCanonicalSubjectTransform(profile, []string{path}, []SemanticPose{pose}, 384, 384)

	require.NoError(t, err)
	require.Equal(t, 200, profile.TargetNeutralHeight)
	require.InDelta(t, 1, transform.Scale, 0.0001)
}

func TestStandardHumanoidHeightToleranceRoundsFractionalBoundaryToWholePixel(t *testing.T) {
	require.True(t, withinPixelTolerance(195, 180, standardHumanoidHeightTolerance))
	require.True(t, withinPixelTolerance(165, 180, standardHumanoidHeightTolerance))
	require.False(t, withinPixelTolerance(196, 180, standardHumanoidHeightTolerance))
	require.False(t, withinPixelTolerance(164, 180, standardHumanoidHeightTolerance))
}

func TestStandardHumanoidScaleUsesMedianNeutralDirectionHeight(t *testing.T) {
	medianHeight, withinTolerance := medianHeightWithinTolerance(
		[]int{164, 180, 195},
		180,
		standardHumanoidHeightTolerance,
	)
	require.Equal(t, 180, medianHeight)
	require.True(t, withinTolerance)

	medianHeight, withinTolerance = medianHeightWithinTolerance(
		[]int{164, 164, 195},
		180,
		standardHumanoidHeightTolerance,
	)
	require.Equal(t, 164, medianHeight)
	require.False(t, withinTolerance)
}

func TestCanonicalRegistrationCentersLegacyReferenceCanvasWithoutChangingBodyScale(t *testing.T) {
	dir := t.TempDir()
	referencePath := filepath.Join(dir, "reference.png")
	reference := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	fillOpaque(
		reference,
		image.Rect(80, 40, 180, 240),
		color.NRGBA{R: 30, G: 80, B: 180, A: 255},
	)
	require.NoError(t, writePNG(referencePath, reference))
	masterPath := filepath.Join(dir, "master.png")
	master := image.NewNRGBA(image.Rect(0, 0, 100, 200))
	fillOpaque(master, master.Bounds(), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	require.NoError(t, writePNG(masterPath, master))
	pose := SemanticPose{
		Bounds:     master.Bounds(),
		CoreBounds: master.Bounds(),
		Pivot:      image.Pt(50, 200),
	}
	profile, err := BuildCanonicalSubjectProfile(
		[]string{referencePath},
		SubjectRegistrationGrounded,
		CanonicalScaleClassReferenceStable,
		384,
	)
	require.NoError(t, err)

	transform, err := FitCanonicalSubjectTransform(
		profile,
		[]string{masterPath},
		[]SemanticPose{pose},
		384,
		384,
	)

	require.NoError(t, err)
	require.InDelta(t, 1, transform.Scale, 0.01)
	require.Equal(
		t,
		profile.ReferencePivots[0].Add(image.Pt(32, 32)),
		transform.DirectionAnchors[0],
	)
}

func TestCanonicalRegistrationDoesNotShrinkBodyScaleForWideActionExtent(t *testing.T) {
	dir := t.TempDir()
	referencePaths := []string{
		filepath.Join(dir, "reference-down.png"),
		filepath.Join(dir, "reference-up.png"),
	}
	masterPaths := []string{
		filepath.Join(dir, "master-down.png"),
		filepath.Join(dir, "master-up.png"),
	}
	for _, path := range referencePaths {
		sprite := image.NewNRGBA(image.Rect(0, 0, 320, 320))
		fillOpaque(sprite, image.Rect(120, 60, 200, 260), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
		require.NoError(t, writePNG(path, sprite))
	}
	for _, path := range masterPaths {
		sprite := image.NewNRGBA(image.Rect(0, 0, 120, 200))
		fillOpaque(sprite, image.Rect(20, 0, 100, 200), color.NRGBA{R: 30, G: 80, B: 180, A: 255})
		require.NoError(t, writePNG(path, sprite))
	}
	profile, err := BuildCanonicalSubjectProfile(referencePaths, SubjectRegistrationGrounded, CanonicalScaleClassReferenceStable, 320)
	require.NoError(t, err)
	masterPoses := []SemanticPose{
		{
			Index: 0, Bounds: image.Rect(0, 0, 120, 200),
			CoreBounds: image.Rect(20, 0, 100, 200), Pivot: image.Pt(60, 200),
		},
		{
			Index: 1, Bounds: image.Rect(0, 0, 120, 200),
			CoreBounds: image.Rect(20, 0, 100, 200), Pivot: image.Pt(60, 200),
		},
	}

	transform, err := FitCanonicalSubjectTransform(profile, masterPaths, masterPoses, 320, 320)

	require.NoError(t, err)
	require.InDelta(t, 1, transform.Scale, 0.01)
	wideAction := SemanticPose{
		Index: 2, Bounds: image.Rect(-300, 0, 400, 200),
		CoreBounds: image.Rect(20, 0, 100, 200), Pivot: image.Pt(60, 200),
	}
	safe := image.Rect(0, 0, 320, 320).Inset(CanonicalFrameEdgePadding(320, 320))
	require.False(t, semanticPoseDestination(
		wideAction,
		transform.Scale,
		transform.DirectionAnchors[0],
	).In(safe))
}

func TestCanonicalTransformAllowsFeasiblePreferredAnchorTranslation(t *testing.T) {
	dir := t.TempDir()
	referencePath := filepath.Join(dir, "reference.png")
	reference := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	fillOpaque(
		reference,
		image.Rect(10, 60, 90, 260),
		color.NRGBA{R: 30, G: 80, B: 180, A: 255},
	)
	require.NoError(t, writePNG(referencePath, reference))
	masterPath := filepath.Join(dir, "master.png")
	master := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	fillOpaque(
		master,
		master.Bounds(),
		color.NRGBA{R: 30, G: 80, B: 180, A: 255},
	)
	require.NoError(t, writePNG(masterPath, master))
	profile, err := BuildCanonicalSubjectProfile(
		[]string{referencePath},
		SubjectRegistrationGrounded,
		CanonicalScaleClassReferenceStable,
		320,
	)
	require.NoError(t, err)
	pose := SemanticPose{
		Bounds: master.Bounds(),
		Pivot:  image.Pt(100, 200),
	}

	transform, err := FitCanonicalSubjectTransform(
		profile,
		[]string{masterPath},
		[]SemanticPose{pose},
		320,
		320,
	)

	require.NoError(t, err)
	require.Equal(t, profile.ReferencePivots[0], transform.DirectionAnchors[0])
	preferredDestination := semanticPoseDestination(
		pose,
		transform.Scale,
		transform.DirectionAnchors[0],
	)
	safe := image.Rect(0, 0, 320, 320).Inset(CanonicalFrameEdgePadding(320, 320))
	require.False(t, preferredDestination.In(safe))
	adjusted, err := ConstrainSemanticUnitAnchors(
		transform,
		[][]SemanticPose{{pose}},
		320,
		320,
	)
	require.NoError(t, err)
	require.True(t, semanticPoseDestination(
		pose,
		adjusted.Scale,
		adjusted.DirectionAnchors[0],
	).In(safe))
}

func TestConstrainSemanticUnitAnchorsUsesOneMinimalShiftPerDirection(t *testing.T) {
	transform := SemanticUnitTransform{
		Scale:            1,
		DirectionAnchors: []image.Point{{X: 150, Y: 280}},
	}
	neutral := SemanticPose{
		Bounds: image.Rect(-60, -200, 80, 0),
		Pivot:  image.Point{},
	}
	wideAttack := SemanticPose{
		Bounds: image.Rect(-80, -200, 220, 0),
		Pivot:  image.Point{},
	}

	adjusted, err := ConstrainSemanticUnitAnchors(
		transform,
		[][]SemanticPose{{neutral, wideAttack}},
		320,
		320,
	)

	require.NoError(t, err)
	require.Equal(t, transform.Scale, adjusted.Scale)
	require.Equal(t, image.Pt(99, 280), adjusted.DirectionAnchors[0])
	safe := image.Rect(0, 0, 320, 320).Inset(CanonicalFrameEdgePadding(320, 320))
	require.True(t, semanticPoseDestination(
		neutral,
		adjusted.Scale,
		adjusted.DirectionAnchors[0],
	).In(safe))
	require.True(t, semanticPoseDestination(
		wideAttack,
		adjusted.Scale,
		adjusted.DirectionAnchors[0],
	).In(safe))
}

func TestConstrainSemanticUnitAnchorsRejectsEmptyFeasibleInterval(t *testing.T) {
	transform := SemanticUnitTransform{
		Scale:            1,
		DirectionAnchors: []image.Point{{X: 150, Y: 280}},
	}
	rightHeavy := SemanticPose{
		Bounds: image.Rect(-10, -200, 270, 0),
		Pivot:  image.Point{},
	}
	leftHeavy := SemanticPose{
		Bounds: image.Rect(-120, -200, 180, 0),
		Pivot:  image.Point{},
	}

	_, err := ConstrainSemanticUnitAnchors(
		transform,
		[][]SemanticPose{{rightHeavy, leftHeavy}},
		320,
		320,
	)

	require.ErrorIs(t, err, ErrProductionFrameClipping)
	require.ErrorContains(t, err, "direction 00 has no shared feasible anchor")
}

func TestPrepareSemanticPosesForSharedBodyAnchorPreservesDetectedPivots(t *testing.T) {
	poses := []SemanticPose{
		{
			Index: 0, Anchor: image.Pt(100, 200),
			Bounds: image.Rect(-40, 0, 80, 160),
			Pivot:  image.Pt(20, 160),
		},
		{
			Index: 1, Anchor: image.Pt(500, 200),
			Bounds: image.Rect(430, 20, 610, 160),
			Pivot:  image.Pt(470, 160),
		},
	}
	transform := SemanticUnitTransform{
		Scale:            1,
		DirectionAnchors: []image.Point{image.Pt(160, 319)},
	}

	registered, offsets, err := PrepareSemanticPosesForSharedBodyAnchor(poses, 2)

	require.NoError(t, err)
	require.Equal(t, []image.Point{{X: -80, Y: -40}}, offsets)
	require.Equal(t, image.Pt(20, 160), registered[0].Pivot)
	// Every detected grounded body pivot is mapped to the shared output anchor.
	// Provider placement inside a logical board slot must not move the unit body.
	require.Equal(t, image.Pt(470, 160), registered[1].Pivot)
	adjusted, err := ConstrainSemanticUnitAnchors(
		transform,
		[][]SemanticPose{registered},
		320,
		320,
	)
	require.NoError(t, err)
	require.Equal(t, transform.Scale, adjusted.Scale)
}

func TestCalibrateSemanticPoseSetCancelsIndependentBoardScale(t *testing.T) {
	dir := t.TempDir()
	masterPaths := []string{
		filepath.Join(dir, "master-down.png"),
		filepath.Join(dir, "master-up.png"),
	}
	calibrationPaths := []string{
		filepath.Join(dir, "walk-down-00.png"),
		filepath.Join(dir, "walk-up-00.png"),
	}
	writeOpaqueRectPNG(t, masterPaths[0], image.Rect(0, 0, 100, 200))
	writeOpaqueRectPNG(t, masterPaths[1], image.Rect(0, 0, 120, 180))
	writeOpaqueRectPNG(t, calibrationPaths[0], image.Rect(0, 0, 50, 100))
	writeOpaqueRectPNG(t, calibrationPaths[1], image.Rect(0, 0, 60, 90))

	calibration, err := CalibrateSemanticPoseSet(
		masterPaths,
		calibrationPaths,
		SubjectRegistrationGrounded,
		0.7,
	)

	require.NoError(t, err)
	require.Equal(t, SemanticScaleCalibrationVersion, calibration.Version)
	require.Equal(t, 0, calibration.CalibrationFrame)
	require.Equal(t, []float64{2, 2}, calibration.SourceRatios)
	require.InDelta(t, 1.4, calibration.DirectionScales[0], 0.0001)
	require.InDelta(t, 1.4, calibration.DirectionScales[1], 0.0001)
}

func TestConstrainSemanticUnitAnchorsAcrossPoseSetsUsesCalibratedScales(t *testing.T) {
	transform := SemanticUnitTransform{
		Scale:            0.5,
		DirectionAnchors: []image.Point{{X: 160, Y: 280}},
	}
	master := SemanticPose{
		Bounds: image.Rect(-100, -400, 100, 0),
		Pivot:  image.Point{},
	}
	smallBoardNeutral := SemanticPose{
		Bounds: image.Rect(-50, -200, 50, 0),
		Pivot:  image.Point{},
	}
	smallBoardAttack := SemanticPose{
		Bounds: image.Rect(-120, -200, 80, 0),
		Pivot:  image.Point{},
	}

	adjusted, err := ConstrainSemanticUnitAnchorsAcrossPoseSets(
		transform,
		[]SemanticPoseSet{
			{
				PosesByDirection: [][]SemanticPose{{master}},
				DirectionScales:  []float64{0.5},
			},
			{
				PosesByDirection: [][]SemanticPose{{smallBoardNeutral, smallBoardAttack}},
				DirectionScales:  []float64{1},
			},
		},
		320,
		320,
	)

	require.NoError(t, err)
	require.Equal(t, transform.Scale, adjusted.Scale)
	require.Equal(t, image.Pt(160, 280), adjusted.DirectionAnchors[0])
}

func TestConstrainSemanticUnitAnchorsAcrossPoseSetsRejectsCanonicalWideAction(t *testing.T) {
	transform := SemanticUnitTransform{
		Scale:            0.5,
		DirectionAnchors: []image.Point{{X: 160, Y: 280}},
	}
	master := SemanticPose{
		Bounds: image.Rect(-100, -400, 100, 0),
		Pivot:  image.Point{},
	}
	leftHeavy := SemanticPose{
		Bounds: image.Rect(-180, -200, 120, 0),
		Pivot:  image.Point{},
	}
	rightHeavy := SemanticPose{
		Bounds: image.Rect(-120, -200, 180, 0),
		Pivot:  image.Point{},
	}

	_, err := ConstrainSemanticUnitAnchorsAcrossPoseSets(
		transform,
		[]SemanticPoseSet{
			{
				PosesByDirection: [][]SemanticPose{{master}},
				DirectionScales:  []float64{0.5},
			},
			{
				PosesByDirection: [][]SemanticPose{{leftHeavy, rightHeavy}},
				DirectionScales:  []float64{1},
			},
		},
		320,
		320,
	)

	require.ErrorIs(t, err, ErrProductionFrameClipping)
	require.ErrorContains(t, err, "direction 00 has no shared feasible anchor")
}

func TestCenteredCanonicalRegistrationPreservesReferenceVisualCenter(t *testing.T) {
	dir := t.TempDir()
	referencePath := filepath.Join(dir, "reference.png")
	reference := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	fillOpaque(
		reference,
		image.Rect(80, 40, 180, 240),
		color.NRGBA{R: 30, G: 80, B: 180, A: 255},
	)
	require.NoError(t, writePNG(referencePath, reference))
	masterPath := filepath.Join(dir, "master.png")
	master := image.NewNRGBA(image.Rect(0, 0, 100, 200))
	fillOpaque(
		master,
		master.Bounds(),
		color.NRGBA{R: 30, G: 80, B: 180, A: 255},
	)
	require.NoError(t, writePNG(masterPath, master))
	pose := SemanticPose{
		Bounds:     master.Bounds(),
		CoreBounds: master.Bounds(),
		Pivot:      image.Pt(50, 200),
	}
	profile, err := BuildCanonicalSubjectProfile(
		[]string{referencePath},
		SubjectRegistrationCentered,
		CanonicalScaleClassReferenceStable,
		320,
	)
	require.NoError(t, err)

	transform, err := FitCanonicalSubjectTransform(
		profile,
		[]string{masterPath},
		[]SemanticPose{pose},
		320,
		320,
	)

	require.NoError(t, err)
	require.InDelta(t, 1, transform.Scale, 0.01)
	destination := semanticPoseDestination(
		pose,
		transform.Scale,
		transform.DirectionAnchors[0],
	)
	require.Equal(t, profile.ReferencePivots[0].X, (destination.Min.X+destination.Max.X)/2)
	require.Equal(t, profile.ReferencePivots[0].Y, (destination.Min.Y+destination.Max.Y)/2)
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

func writePaddedReference(t *testing.T, posePath, outputPath string) {
	t.Helper()
	pose, err := decodeNRGBA(posePath)
	require.NoError(t, err)
	output := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	offset := image.Pt((320-pose.Bounds().Dx())/2, 280-pose.Bounds().Dy())
	draw.Draw(output, pose.Bounds().Add(offset), pose, pose.Bounds().Min, draw.Src)
	require.NoError(t, writePNG(outputPath, output))
}

func writeOpaqueRectPNG(t *testing.T, path string, bounds image.Rectangle) {
	t.Helper()
	sprite := image.NewNRGBA(bounds)
	fillOpaque(sprite, bounds, color.NRGBA{R: 30, G: 80, B: 180, A: 255})
	require.NoError(t, writePNG(path, sprite))
}
