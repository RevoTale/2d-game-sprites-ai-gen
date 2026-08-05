package imageio_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
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

func TestWriteReviewPreviewPNGMayEnlargeDisplayEvidence(t *testing.T) {
	var raw bytes.Buffer
	require.NoError(
		t,
		png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 16, 16))),
	)
	path := filepath.Join(t.TempDir(), "preview.png")

	_, err := imageio.WriteReviewPreviewPNG(path, raw.Bytes(), 96, 96, nil)

	require.NoError(t, err)
	dimensions, err := imageio.PNGDimensions(path)
	require.NoError(t, err)
	require.Equal(t, image.Pt(96, 96), dimensions)
}

func TestWriteIsolatedReviewPreviewPNGFillsPortraitFromTransparentSubjectBounds(t *testing.T) {
	var raw bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 384, 384))
	for y := 92; y < 332; y++ {
		for x := 112; x < 272; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: 186, G: 194, B: 209, A: 255})
		}
	}
	require.NoError(t, png.Encode(&raw, source))
	path := filepath.Join(t.TempDir(), "portrait.png")

	_, err := imageio.WriteIsolatedReviewPreviewPNG(
		path,
		raw.Bytes(),
		96,
		96,
		nil,
		imageio.SubjectRegistrationGrounded,
	)

	require.NoError(t, err)
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	preview, err := png.Decode(file)
	require.NoError(t, err)
	bounds := testOpaqueBounds(preview)
	require.GreaterOrEqual(t, bounds.Dy(), 88)
	require.GreaterOrEqual(t, bounds.Dx(), 58)
	require.Equal(t, 93, bounds.Max.Y)
}

func TestWriteNormalizedPNGIsByteIdenticalAndUsesAtMostThirtyTwoOpaqueColors(t *testing.T) {
	var raw bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: uint8((x + y) * 2), A: 255})
		}
	}
	require.NoError(t, png.Encode(&raw, img))
	first := filepath.Join(t.TempDir(), "first.png")
	second := filepath.Join(t.TempDir(), "second.png")

	require.NoError(t, imageio.WriteNormalizedPNG(first, raw.Bytes(), 16, 16))
	require.NoError(t, imageio.WriteNormalizedPNG(second, raw.Bytes(), 16, 16))
	firstData, err := os.ReadFile(first)
	require.NoError(t, err)
	secondData, err := os.ReadFile(second)
	require.NoError(t, err)
	require.Equal(t, firstData, secondData)
	file, err := os.Open(first)
	require.NoError(t, err)
	normalized, err := png.Decode(file)
	file.Close()
	require.NoError(t, err)
	colors := map[color.NRGBA]struct{}{}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			value := color.NRGBAModel.Convert(normalized.At(x, y)).(color.NRGBA)
			require.Contains(t, []uint8{0, 255}, value.A)
			if value.A != 0 {
				colors[value] = struct{}{}
			}
		}
	}
	require.LessOrEqual(t, len(colors), 32)
}

func TestWriteNormalizedOpaqueTilePNGRemovesMicroClustersAndPreservesWrappedRegions(t *testing.T) {
	base := color.NRGBA{R: 28, G: 32, B: 24, A: 255}
	accent := color.NRGBA{R: 76, G: 84, B: 48, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(img, img.Bounds(), base)
	img.SetNRGBA(8, 8, accent)
	for y := 2; y < 7; y++ {
		img.SetNRGBA(0, y, accent)
		img.SetNRGBA(15, y, accent)
	}
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	out := filepath.Join(t.TempDir(), "opaque-tile.png")

	_, err := imageio.WriteNormalizedOpaqueTilePNG(
		out,
		raw.Bytes(),
		16,
		16,
		[]imageio.PaletteColor{
			{R: base.R, G: base.G, B: base.B},
			{R: accent.R, G: accent.G, B: accent.B},
		},
	)

	require.NoError(t, err)
	normalized := readPNG(t, out)
	require.Equal(t, base, color.NRGBAModel.Convert(normalized.At(8, 8)).(color.NRGBA))
	for y := 2; y < 7; y++ {
		require.Equal(t, accent, color.NRGBAModel.Convert(normalized.At(0, y)).(color.NRGBA))
		require.Equal(t, accent, color.NRGBAModel.Convert(normalized.At(15, y)).(color.NRGBA))
	}
}

func testOpaqueBounds(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X-1, bounds.Min.Y-1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A == 0 {
				continue
			}
			minX = min(minX, x)
			minY = min(minY, y)
			maxX = max(maxX, x)
			maxY = max(maxY, y)
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func TestWriteNormalizedCompositePNGPreservesIndependentMaterialRamps(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for index := 0; index < 64; index++ {
		x := (index % 8) * 16
		y := (index / 8) * 16
		fillRect(
			img,
			image.Rect(x, y, x+16, y+16),
			color.NRGBA{
				R: uint8((index * 37) % 256),
				G: uint8((index * 73) % 256),
				B: uint8((index * 109) % 256),
				A: 255,
			},
		)
	}
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	out := filepath.Join(t.TempDir(), "composite.png")

	palette, err := imageio.WriteNormalizedCompositePNG(
		out,
		raw.Bytes(),
		128,
		128,
	)

	require.NoError(t, err)
	require.Len(t, palette, 64)
}

func TestEvaluateCandidateRejectsMultipleSubjectsAndOccupiedEdgeGuard(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(pose, image.Rect(6, 4, 10, 12), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidate := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(candidate, image.Rect(6, 4, 10, 12), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	fillRect(candidate, image.Rect(0, 0, 4, 6), color.NRGBA{R: 255, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, posePath, pose)
	writePNG(t, candidatePath, candidate)

	metrics, reasons, err := imageio.EvaluateCandidate(candidatePath, posePath, 1)

	require.NoError(t, err)
	require.True(t, metrics.EdgeGuardOccupied)
	require.Equal(t, 2, metrics.Components)
	require.Contains(t, reasons, "edge_guard_occupied")
	require.Contains(t, reasons, "foreground_components_2")
}

func TestEvaluateCandidateAllowsDisconnectedEquipmentBelowPrimarySubjectThreshold(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	fillRect(pose, image.Rect(8, 4, 24, 28), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidate := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	fillRect(candidate, image.Rect(8, 4, 24, 28), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	fillRect(candidate, image.Rect(25, 10, 29, 20), color.NRGBA{R: 200, G: 160, B: 80, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, posePath, pose)
	writePNG(t, candidatePath, candidate)

	metrics, reasons, err := imageio.EvaluateCandidate(candidatePath, posePath, 1)

	require.NoError(t, err)
	require.Equal(t, 1, metrics.Components)
	require.Equal(t, 1, metrics.SecondaryComponents)
	require.NotContains(t, reasons, "foreground_components_2")
}

func TestWriteNormalizedPNGWithOptionsRemovesFlatEdgeConnectedBackground(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(img, img.Bounds(), color.NRGBA{R: 20, G: 240, B: 40, A: 255})
	fillRect(img, image.Rect(16, 12, 48, 56), color.NRGBA{R: 100, G: 40, B: 180, A: 255})
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	out := filepath.Join(t.TempDir(), "normalized.png")

	_, err := imageio.WriteNormalizedPNGWithOptions(out, raw.Bytes(), 16, 16, nil, true)

	require.NoError(t, err)
	file, err := os.Open(out)
	require.NoError(t, err)
	normalized, err := png.Decode(file)
	file.Close()
	require.NoError(t, err)
	require.Zero(t, color.NRGBAModel.Convert(normalized.At(0, 0)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(normalized.At(8, 8)).(color.NRGBA).A)
}

func TestWriteNormalizedIsolatedPNGExtractsWideForegroundBeforeTargetFit(t *testing.T) {
	background := color.NRGBA{R: 255, B: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	fillRect(img, img.Bounds(), background)
	fillRect(
		img,
		image.Rect(152, 392, 872, 632),
		color.NRGBA{R: 64, G: 72, B: 88, A: 255},
	)
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	out := filepath.Join(t.TempDir(), "wide-isolated.png")

	_, err := imageio.WriteNormalizedIsolatedPNG(
		out,
		raw.Bytes(),
		277,
		150,
		nil,
		imageio.SubjectRegistrationCentered,
	)

	require.NoError(t, err)
	normalized := readPNG(t, out)
	foreground := opaqueBounds(normalized)
	require.GreaterOrEqual(t, foreground.Dx(), 265)
	require.LessOrEqual(t, foreground.Dx(), 269)
	require.InDelta(t, 3.0, float64(foreground.Dx())/float64(foreground.Dy()), 0.08)
	require.InDelta(t, 138.5, float64(foreground.Min.X+foreground.Max.X)/2, 1)
	require.InDelta(t, 75, float64(foreground.Min.Y+foreground.Max.Y)/2, 1)
}

func TestWriteNormalizedIsolatedPNGGroundsCompleteForeground(t *testing.T) {
	background := color.NRGBA{R: 255, B: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	fillRect(img, img.Bounds(), background)
	fillRect(
		img,
		image.Rect(412, 212, 612, 612),
		color.NRGBA{R: 96, G: 48, B: 128, A: 255},
	)
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	out := filepath.Join(t.TempDir(), "grounded-isolated.png")

	_, err := imageio.WriteNormalizedIsolatedPNG(
		out,
		raw.Bytes(),
		160,
		160,
		nil,
		imageio.SubjectRegistrationGrounded,
	)

	require.NoError(t, err)
	foreground := opaqueBounds(readPNG(t, out))
	require.Equal(t, 155, foreground.Max.Y)
	require.InDelta(t, 80, float64(foreground.Min.X+foreground.Max.X)/2, 1)
}

func TestWriteNormalizedTransparentIsolatedPNGPreservesRecoveredCropEdges(t *testing.T) {
	dir := t.TempDir()
	input := image.NewNRGBA(image.Rect(0, 0, 12, 10))
	for y := input.Bounds().Min.Y; y < input.Bounds().Max.Y; y++ {
		for x := input.Bounds().Min.X; x < input.Bounds().Max.X; x++ {
			input.SetNRGBA(x, y, color.NRGBA{R: 70, G: 80, B: 90, A: 255})
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, input))
	path := filepath.Join(dir, "normalized.png")

	_, err := imageio.WriteNormalizedTransparentIsolatedPNG(
		path,
		encoded.Bytes(),
		10,
		10,
		nil,
		imageio.SubjectRegistrationGrounded,
	)

	require.NoError(t, err)
	require.FileExists(t, path)
}

func TestWriteNativeScaleTransparentIsolatedPNGOnlyPadsForeground(t *testing.T) {
	input := image.NewNRGBA(image.Rect(0, 0, 100, 80))
	fillRect(input, input.Bounds(), color.NRGBA{R: 70, G: 80, B: 90, A: 255})
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, input))
	path := filepath.Join(t.TempDir(), "native.png")

	_, err := imageio.WriteNativeScaleTransparentIsolatedPNG(
		path,
		encoded.Bytes(),
		160,
		160,
		nil,
		imageio.SubjectRegistrationGrounded,
	)

	require.NoError(t, err)
	foreground := opaqueBounds(readPNG(t, path))
	require.Equal(t, image.Pt(100, 80), foreground.Size())
	require.Equal(t, 155, foreground.Max.Y)
}

func TestWriteSharedScaleTransparentStaticSetUsesOneLimitingScale(t *testing.T) {
	dir := t.TempDir()
	wide := filepath.Join(dir, "wide.png")
	compact := filepath.Join(dir, "compact.png")
	writeOpaqueCropPNG(t, wide, image.Pt(200, 80))
	writeOpaqueCropPNG(t, compact, image.Pt(60, 100))
	wideOutput := filepath.Join(dir, "wide-output.png")
	compactOutput := filepath.Join(dir, "compact-output.png")

	calibration, err := imageio.WriteSharedScaleTransparentStaticSet(
		[]imageio.StaticSetPart{
			{
				ID: "wide", SourcePath: wide, OutputPath: wideOutput,
				Size: image.Pt(160, 160), Registration: imageio.SubjectRegistrationCentered,
			},
			{
				ID: "compact", SourcePath: compact, OutputPath: compactOutput,
				Size: image.Pt(160, 160), Registration: imageio.SubjectRegistrationGrounded,
			},
		},
		[]imageio.PaletteColor{{R: 70, G: 80, B: 90}},
	)

	require.NoError(t, err)
	require.Equal(t, "wide", calibration.LimitingPartID)
	require.Equal(t, "width", calibration.LimitingAxis)
	require.Less(t, calibration.Scale, 1.0)
	wideBounds := opaqueBounds(readPNG(t, wideOutput))
	compactBounds := opaqueBounds(readPNG(t, compactOutput))
	require.Equal(t, image.Pt(150, 60), wideBounds.Size())
	require.Equal(t, image.Pt(45, 75), compactBounds.Size())
	require.InDelta(t, 80, float64(wideBounds.Min.X+wideBounds.Max.X)/2, 1)
	require.Equal(t, 155, compactBounds.Max.Y)
}

func TestWriteSharedScaleTransparentStaticSetCanBeHeightLimited(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "tall.png")
	writeOpaqueCropPNG(t, source, image.Pt(50, 201))
	output := filepath.Join(dir, "output.png")

	calibration, err := imageio.WriteSharedScaleTransparentStaticSet(
		[]imageio.StaticSetPart{{
			ID: "tall", SourcePath: source, OutputPath: output,
			Size: image.Pt(161, 159), Registration: imageio.SubjectRegistrationGrounded,
		}},
		[]imageio.PaletteColor{{R: 70, G: 80, B: 90}},
	)

	require.NoError(t, err)
	require.Equal(t, "tall", calibration.LimitingPartID)
	require.Equal(t, "height", calibration.LimitingAxis)
	foreground := opaqueBounds(readPNG(t, output))
	require.Equal(t, 151, foreground.Dy())
	require.Equal(t, 155, foreground.Max.Y)
}

func TestWriteSharedScaleTransparentStaticSetDoesNotUpscale(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.png")
	writeOpaqueCropPNG(t, source, image.Pt(40, 30))
	first := filepath.Join(dir, "first.png")
	second := filepath.Join(dir, "second.png")
	parts := []imageio.StaticSetPart{
		{ID: "first", SourcePath: source, OutputPath: first, Size: image.Pt(160, 160), Registration: imageio.SubjectRegistrationCentered},
		{ID: "second", SourcePath: source, OutputPath: second, Size: image.Pt(200, 180), Registration: imageio.SubjectRegistrationGrounded},
	}

	firstCalibration, err := imageio.WriteSharedScaleTransparentStaticSet(
		parts,
		[]imageio.PaletteColor{{R: 70, G: 80, B: 90}},
	)

	require.NoError(t, err)
	require.Equal(t, 1.0, firstCalibration.Scale)
	require.Empty(t, firstCalibration.LimitingPartID)
	require.Equal(t, image.Pt(40, 30), opaqueBounds(readPNG(t, first)).Size())
	require.Equal(t, image.Pt(40, 30), opaqueBounds(readPNG(t, second)).Size())
	firstBytes, err := os.ReadFile(first)
	require.NoError(t, err)
	secondCalibration, err := imageio.WriteSharedScaleTransparentStaticSet(
		parts,
		[]imageio.PaletteColor{{R: 70, G: 80, B: 90}},
	)
	require.NoError(t, err)
	require.Equal(t, firstCalibration, secondCalibration)
	secondBytes, err := os.ReadFile(first)
	require.NoError(t, err)
	require.Equal(t, firstBytes, secondBytes)
}

func TestWriteSharedScaleTransparentStaticSetMeasuresCompleteSetBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	validSource := filepath.Join(dir, "valid.png")
	writeOpaqueCropPNG(t, validSource, image.Pt(40, 30))
	validOutput := filepath.Join(dir, "valid-output.png")

	_, err := imageio.WriteSharedScaleTransparentStaticSet(
		[]imageio.StaticSetPart{
			{ID: "valid", SourcePath: validSource, OutputPath: validOutput, Size: image.Pt(80, 80), Registration: imageio.SubjectRegistrationCentered},
			{ID: "missing", SourcePath: filepath.Join(dir, "missing.png"), OutputPath: filepath.Join(dir, "missing-output.png"), Size: image.Pt(80, 80), Registration: imageio.SubjectRegistrationCentered},
		},
		[]imageio.PaletteColor{{R: 70, G: 80, B: 90}},
	)

	require.ErrorContains(t, err, `decode static set part "missing"`)
	require.NoFileExists(t, validOutput)
}

func TestWriteNormalizedIsolatedPNGRejectsRequiredUpscale(t *testing.T) {
	background := color.NRGBA{R: 255, B: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	fillRect(img, img.Bounds(), background)
	fillRect(
		img,
		image.Rect(500, 500, 520, 510),
		color.NRGBA{R: 96, G: 48, B: 128, A: 255},
	)
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))

	_, err := imageio.WriteNormalizedIsolatedPNG(
		filepath.Join(t.TempDir(), "too-small.png"),
		raw.Bytes(),
		277,
		150,
		nil,
		imageio.SubjectRegistrationCentered,
	)

	require.ErrorContains(t, err, "requires upscaling")
}

func TestWriteNormalizedPNGWithOptionsRemovesEnclosedChromaAndEdgeSpill(t *testing.T) {
	background := color.NRGBA{R: 60, G: 0, B: 96, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(img, img.Bounds(), background)
	fillRect(img, image.Rect(3, 3, 13, 13), color.NRGBA{R: 116, G: 18, B: 132, A: 255})
	fillRect(img, image.Rect(4, 4, 12, 12), color.NRGBA{R: 92, G: 116, B: 68, A: 255})
	fillRect(img, image.Rect(7, 7, 9, 9), background)
	img.SetNRGBA(4, 4, color.NRGBA{R: 180, G: 60, B: 200, A: 255})
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	out := filepath.Join(t.TempDir(), "normalized.png")

	_, err := imageio.WriteNormalizedPNGWithOptions(out, raw.Bytes(), 16, 16, nil, true)

	require.NoError(t, err)
	file, err := os.Open(out)
	require.NoError(t, err)
	normalized, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Zero(t, color.NRGBAModel.Convert(normalized.At(3, 3)).(color.NRGBA).A, "chroma spill adjacent to the subject must not enter the locked palette")
	require.Zero(t, color.NRGBAModel.Convert(normalized.At(7, 7)).(color.NRGBA).A, "enclosed background holes must become transparent")
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(normalized.At(5, 5)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(normalized.At(4, 4)).(color.NRGBA).A, "distinct subject colors must survive chroma removal")
}

func TestWriteNormalizedPNGWithOptionsRemovesDesaturatedGreenFringeWithoutErasingSubject(t *testing.T) {
	background := color.NRGBA{R: 0, G: 255, B: 0, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(img, img.Bounds(), background)
	fillRect(img, image.Rect(4, 4, 12, 12), color.NRGBA{R: 232, G: 236, B: 224, A: 255})
	fringe := color.NRGBA{R: 128, G: 255, B: 128, A: 255}
	for x := 3; x <= 12; x++ {
		img.SetNRGBA(x, 3, fringe)
		img.SetNRGBA(x, 12, fringe)
	}
	for y := 4; y < 12; y++ {
		img.SetNRGBA(3, y, fringe)
		img.SetNRGBA(12, y, fringe)
	}
	// The chroma key is selected to be absent from the subject. A separate
	// interior green material must nevertheless survive because it is not
	// connected to the removed background.
	img.SetNRGBA(8, 8, color.NRGBA{R: 40, G: 180, B: 72, A: 255})
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	out := filepath.Join(t.TempDir(), "normalized.png")

	_, err := imageio.WriteNormalizedPNGWithOptions(out, raw.Bytes(), 16, 16, nil, true)

	require.NoError(t, err)
	file, err := os.Open(out)
	require.NoError(t, err)
	normalized, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Zero(t, color.NRGBAModel.Convert(normalized.At(3, 8)).(color.NRGBA).A, "desaturated green fringe must not enter the sprite palette")
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(normalized.At(4, 8)).(color.NRGBA).A, "neutral subject edge must survive chroma removal")
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(normalized.At(8, 8)).(color.NRGBA).A, "interior subject green must survive chroma removal")
}

func TestEvaluateCandidateRejectsBaselineDrift(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(pose, image.Rect(5, 4, 11, 14), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidate := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(candidate, image.Rect(5, 1, 11, 11), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, posePath, pose)
	writePNG(t, candidatePath, candidate)

	metrics, reasons, err := imageio.EvaluateCandidate(candidatePath, posePath, 0)

	require.NoError(t, err)
	require.Greater(t, metrics.BaselineDelta, 0.05)
	require.Contains(t, reasons, "baseline_drift")
}

func TestEvaluateCandidateAllowsDenserDetailWhenOccupiedBoundsStayAligned(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 4; y < 14; y++ {
		for x := 5; x < 11; x++ {
			if (x+y)%2 == 0 {
				pose.SetNRGBA(x, y, color.NRGBA{R: 100, G: 120, B: 180, A: 255})
			}
		}
	}
	candidate := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(candidate, image.Rect(5, 4, 11, 14), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, posePath, pose)
	writePNG(t, candidatePath, candidate)

	metrics, reasons, err := imageio.EvaluateCandidate(candidatePath, posePath, 0)

	require.NoError(t, err)
	require.Greater(t, metrics.OccupiedAreaDelta, 0.35)
	require.Zero(t, metrics.OccupiedBoundsDelta)
	require.NotContains(t, reasons, "occupied_area_drift")
	require.NotContains(t, reasons, "occupied_bounds_drift")
}

func TestEvaluateCandidateRejectsOccupiedBoundsDrift(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(pose, image.Rect(5, 4, 11, 14), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidate := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(candidate, image.Rect(1, 4, 15, 14), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, posePath, pose)
	writePNG(t, candidatePath, candidate)

	metrics, reasons, err := imageio.EvaluateCandidate(candidatePath, posePath, 0)

	require.NoError(t, err)
	require.Greater(t, metrics.OccupiedBoundsDelta, 0.15)
	require.Contains(t, reasons, "occupied_bounds_drift")
}

func fillRect(img *image.NRGBA, bounds image.Rectangle, value color.NRGBA) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetNRGBA(x, y, value)
		}
	}
}

func opaquePixelCount(img image.Image) int {
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha != 0 {
				count++
			}
		}
	}
	return count
}

func opaqueBounds(img image.Image) image.Rectangle {
	bounds := image.Rectangle{}
	found := false
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}
			if !found {
				bounds = image.Rect(x, y, x+1, y+1)
				found = true
				continue
			}
			bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	return bounds
}

func readPNG(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	img, err := png.Decode(file)
	require.NoError(t, err)
	return img
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(file, img))
	require.NoError(t, file.Close())
}

func writeOpaqueCropPNG(t *testing.T, path string, size image.Point) {
	t.Helper()
	img := image.NewNRGBA(image.Rectangle{Max: size})
	fillRect(img, img.Bounds(), color.NRGBA{R: 70, G: 80, B: 90, A: 255})
	writePNG(t, path, img)
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

func TestValidateCanonicalFrameRejectsOpaqueOutermostPixel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frame.png")
	frame := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	fillRect(frame, image.Rect(32, 32, 128, 128), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	frame.SetNRGBA(35, 159, color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	writePNG(t, path, frame)

	err := imageio.ValidateCanonicalFrame(path, 160, 160)

	require.ErrorIs(t, err, imageio.ErrProductionFrameClipping)
}

func TestWriteReviewArtifactsUsesNearestNeighborSheetAndNativeLoopingGIF(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for index, value := range []color.NRGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
		img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		fillRect(img, image.Rect(index+2, 2, index+5, 7), value)
		path := filepath.Join(dir, fmt.Sprintf("%d.png", index))
		writePNG(t, path, img)
		paths = append(paths, path)
	}
	sheetPath := filepath.Join(dir, "contact-sheet.png")
	gifPath := filepath.Join(dir, "animation.gif")

	require.NoError(t, imageio.WriteNearestNeighborContactSheet(paths, sheetPath, 4))
	require.NoError(t, imageio.WriteLoopingGIF(paths, gifPath, 12))

	sheetFile, err := os.Open(sheetPath)
	require.NoError(t, err)
	sheet, err := png.Decode(sheetFile)
	require.NoError(t, err)
	require.NoError(t, sheetFile.Close())
	require.Equal(t, image.Rect(0, 0, 64, 32), sheet.Bounds())
	require.Equal(t, color.NRGBA{R: 255, A: 255}, color.NRGBAModel.Convert(sheet.At(8, 8)))

	gifFile, err := os.Open(gifPath)
	require.NoError(t, err)
	animation, err := gif.DecodeAll(gifFile)
	require.NoError(t, err)
	require.NoError(t, gifFile.Close())
	require.Len(t, animation.Image, 2)
	require.Equal(t, image.Rect(0, 0, 8, 8), animation.Image[0].Bounds())
	require.Equal(t, 0, animation.LoopCount)
	require.Equal(t, []int{12, 12}, animation.Delay)
}

func TestWriteTiledRepeatPreviewRepeatsNativePixelsWithoutInterpolation(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "tile.png")
	tile := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	tile.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	tile.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	tile.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	tile.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, A: 255})
	writePNG(t, sourcePath, tile)
	previewPath := filepath.Join(dir, "repeat.png")

	require.NoError(t, imageio.WriteTiledRepeatPreview(sourcePath, previewPath, 3, 3))
	file, err := os.Open(previewPath)
	require.NoError(t, err)
	preview, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, image.Rect(0, 0, 6, 6), preview.Bounds())
	require.Equal(t, preview.At(0, 0), preview.At(2, 0))
	require.Equal(t, preview.At(1, 1), preview.At(5, 5))
}

func TestWriteNearestNeighborContactGridUsesConfiguredRowsAndColumns(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for index := range 6 {
		img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		fillRect(img, image.Rect(2, 2, 6, 6), color.NRGBA{R: uint8(index + 1), A: 255})
		path := filepath.Join(dir, fmt.Sprintf("%d.png", index))
		writePNG(t, path, img)
		paths = append(paths, path)
	}
	out := filepath.Join(dir, "comparison.png")

	require.NoError(t, imageio.WriteNearestNeighborContactGrid(paths, out, 3, 1))

	dimensions, err := imageio.PNGDimensions(out)
	require.NoError(t, err)
	require.Equal(t, image.Pt(24, 16), dimensions)
}

func TestWriteLabeledNearestNeighborContactGridNamesMasterAnimationsAndDirections(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for index := range 6 {
		img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		fillRect(img, image.Rect(2, 2, 6, 6), color.NRGBA{R: uint8(index + 1), A: 255})
		path := filepath.Join(dir, fmt.Sprintf("%d.png", index))
		writePNG(t, path, img)
		paths = append(paths, path)
	}
	out := filepath.Join(dir, "comparison.png")

	require.NoError(t, imageio.WriteLabeledNearestNeighborContactGrid(
		paths,
		out,
		3,
		1,
		[]string{"MASTER", "WALK 00", "ATTACK 00"},
		[]string{"DOWN", "RIGHT"},
	))

	file, err := os.Open(out)
	require.NoError(t, err)
	sheet, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, image.Rect(0, 0, 136, 56), sheet.Bounds())
	require.NotEqual(t, color.NRGBA{}, color.NRGBAModel.Convert(sheet.At(112, 0)))
	require.NotEqual(t, color.NRGBA{}, color.NRGBAModel.Convert(sheet.At(0, 40)))
	require.Equal(t, color.NRGBA{R: 1, A: 255}, color.NRGBAModel.Convert(sheet.At(114, 42)))
}

func TestWriteCandidateReviewSheetLabelsCandidateIDsAndMechanicalValidity(t *testing.T) {
	dir := t.TempDir()
	var tiles []imageio.CandidateReviewTile
	for index, value := range []color.NRGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
		img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		fillRect(img, img.Bounds(), value)
		path := filepath.Join(dir, fmt.Sprintf("%02d.png", index+1))
		writePNG(t, path, img)
		tiles = append(tiles, imageio.CandidateReviewTile{ID: fmt.Sprintf("%02d", index+1), Path: path, Valid: index == 0})
	}
	out := filepath.Join(dir, "candidates.png")

	require.NoError(t, imageio.WriteCandidateReviewSheet(tiles, out))

	file, err := os.Open(out)
	require.NoError(t, err)
	sheet, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, image.Rect(0, 0, 32, 56), sheet.Bounds())
	require.Equal(t, color.NRGBA{R: 30, G: 122, B: 70, A: 255}, color.NRGBAModel.Convert(sheet.At(0, 0)))
	require.Equal(t, color.NRGBA{R: 165, G: 36, B: 36, A: 255}, color.NRGBAModel.Convert(sheet.At(16, 0)))
	require.Equal(t, color.NRGBA{R: 255, A: 255}, color.NRGBAModel.Convert(sheet.At(0, 40)))
	require.Equal(t, color.NRGBA{B: 255, A: 255}, color.NRGBAModel.Convert(sheet.At(16, 40)))
}

func TestWriteDensityReducedPNGPreservesCompleteCanvas(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.png")
	outputPath := filepath.Join(dir, "logical.png")
	source := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	fillRect(source, image.Rect(8, 8, 56, 40), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	writePNG(t, sourcePath, source)

	require.NoError(t, imageio.WriteDensityReducedPNG(sourcePath, outputPath, 2))
	dimensions, err := imageio.PNGDimensions(outputPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(32, 24), dimensions)
}
