package imageio_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
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

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(file, img))
	require.NoError(t, file.Close())
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

func TestWriteGridBoardUsesDeterministicSquareLayoutAndLeavesTrailingCellsEmpty(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	colors := []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}}
	for index, value := range colors {
		path := filepath.Join(dir, fmt.Sprintf("%d.png", index))
		img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		fillRect(img, img.Bounds(), value)
		writePNG(t, path, img)
		paths = append(paths, path)
	}
	output := filepath.Join(dir, "board.png")

	layout, err := imageio.WriteGridBoard(paths, output, 2)

	require.NoError(t, err)
	require.Equal(t, imageio.GridLayout{Side: 2, CellWidth: 4, CellHeight: 4, Gutter: 2, Count: 3}, layout)
	file, err := os.Open(output)
	require.NoError(t, err)
	board, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, image.Rect(0, 0, 10, 10), board.Bounds())
	require.Equal(t, colors[0], color.NRGBAModel.Convert(board.At(1, 1)))
	require.Equal(t, colors[1], color.NRGBAModel.Convert(board.At(7, 1)))
	require.Equal(t, colors[2], color.NRGBAModel.Convert(board.At(1, 7)))
	require.Equal(t, color.NRGBA{}, color.NRGBAModel.Convert(board.At(7, 7)))
	require.Equal(t, color.NRGBA{}, color.NRGBAModel.Convert(board.At(4, 1)))
}

func TestEvaluateMotionStudyRejectsOccupiedGutterAndTrailingCell(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 3)
	for index := range paths {
		paths[index] = filepath.Join(dir, fmt.Sprintf("pose-%d.png", index))
		img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		fillRect(img, image.Rect(5, 4, 11, 14), color.NRGBA{R: 120, G: 80, B: 180, A: 255})
		writePNG(t, paths[index], img)
	}
	poseBoardPath := filepath.Join(dir, "pose-board.png")
	layout, err := imageio.WriteGridBoard(paths, poseBoardPath, 2)
	require.NoError(t, err)
	candidateFile, err := os.Open(poseBoardPath)
	require.NoError(t, err)
	candidate, err := png.Decode(candidateFile)
	require.NoError(t, err)
	require.NoError(t, candidateFile.Close())
	study := image.NewNRGBA(candidate.Bounds())
	draw.Draw(study, study.Bounds(), candidate, candidate.Bounds().Min, draw.Src)
	study.SetNRGBA(16, 1, color.NRGBA{R: 255, A: 255})
	fillRect(study, image.Rect(23, 22, 29, 32), color.NRGBA{R: 120, G: 80, B: 180, A: 255})
	studyPath := filepath.Join(dir, "study.png")
	writePNG(t, studyPath, study)

	metrics, reasons, err := imageio.EvaluateMotionStudy(studyPath, poseBoardPath, layout, 1)

	require.NoError(t, err)
	require.True(t, metrics.GutterOccupied)
	require.True(t, metrics.TrailingOccupied)
	require.Contains(t, reasons, "cell_00_gutter_occupied")
	require.NotContains(t, reasons, "board_gutter_occupied")
	require.Contains(t, reasons, "board_trailing_cell_occupied")
}

func TestEvaluateBoardAttributesDetachedGutterForegroundToNearestCell(t *testing.T) {
	dir := t.TempDir()
	layout := imageio.GridLayout{Columns: 2, Rows: 2, CellWidth: 16, CellHeight: 16, Gutter: 4, Count: 4}
	guide := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	for index := 0; index < layout.Count; index++ {
		fillRect(guide, layout.Cell(index).Inset(4), color.NRGBA{R: 80, G: 120, B: 160, A: 255})
	}
	candidate := image.NewNRGBA(guide.Bounds())
	draw.Draw(candidate, candidate.Bounds(), guide, guide.Bounds().Min, draw.Src)
	// This detached pixel is in the horizontal gutter and one pixel closer to
	// the lower-left cell than the upper-left cell.
	candidate.SetNRGBA(8, 18, color.NRGBA{R: 255, A: 255})
	guidePath := filepath.Join(dir, "guide.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, guidePath, guide)
	writePNG(t, candidatePath, candidate)

	evaluation, err := imageio.EvaluateBoard(candidatePath, guidePath, layout, 1, imageio.BoardValidationAnimationBoard)

	require.NoError(t, err)
	require.Contains(t, evaluation.BlockingFailures, "cell_02_gutter_occupied")
	require.NotContains(t, evaluation.BlockingFailures, "cell_00_gutter_occupied")
	require.NotContains(t, evaluation.BlockingFailures, "board_gutter_occupied")
}

func TestEvaluateMotionStudyRejectsWrongDimensionsWithoutExtractingFrames(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(pose, image.Rect(5, 4, 11, 14), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	writePNG(t, posePath, pose)
	poseBoardPath := filepath.Join(dir, "pose-board.png")
	layout, err := imageio.WriteGridBoard([]string{posePath}, poseBoardPath, 2)
	require.NoError(t, err)
	wrong := image.NewNRGBA(image.Rect(0, 0, 15, 16))
	wrongPath := filepath.Join(dir, "wrong.png")
	writePNG(t, wrongPath, wrong)

	_, _, err = imageio.EvaluateMotionStudy(wrongPath, poseBoardPath, layout, 1)

	require.ErrorContains(t, err, "must both be 16x16")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 3)
}

func TestEvaluateMotionStudyReportsMissingSubjectAndSecondaryEquipmentByCell(t *testing.T) {
	dir := t.TempDir()
	var poses []string
	for index := 0; index < 2; index++ {
		img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		fillRect(img, image.Rect(5, 4, 11, 14), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
		path := filepath.Join(dir, fmt.Sprintf("pose-%d.png", index))
		writePNG(t, path, img)
		poses = append(poses, path)
	}
	poseBoardPath := filepath.Join(dir, "pose-board.png")
	layout, err := imageio.WriteGridBoard(poses, poseBoardPath, 2)
	require.NoError(t, err)
	file, err := os.Open(poseBoardPath)
	require.NoError(t, err)
	decoded, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	candidate := image.NewNRGBA(decoded.Bounds())
	draw.Draw(candidate, candidate.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	draw.Draw(candidate, layout.Cell(0), image.NewUniform(color.NRGBA{}), image.Point{}, draw.Src)
	second := layout.Cell(1)
	fillRect(candidate, image.Rect(second.Min.X, second.Min.Y, second.Min.X+4, second.Min.Y+4), color.NRGBA{R: 255, A: 255})
	candidatePath := filepath.Join(dir, "study.png")
	writePNG(t, candidatePath, candidate)

	_, reasons, err := imageio.EvaluateMotionStudy(candidatePath, poseBoardPath, layout, 1)

	require.NoError(t, err)
	require.Contains(t, reasons, "cell_00_foreground_components_0")
	require.Contains(t, reasons, "cell_01_secondary_components_1")
	require.NotContains(t, reasons, "cell_01_foreground_components_2")
}

func TestEvaluateMotionStudyReportsPerCellBaselineAndBoundsDrift(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(pose, image.Rect(5, 4, 11, 14), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	writePNG(t, posePath, pose)
	poseBoardPath := filepath.Join(dir, "pose-board.png")
	layout, err := imageio.WriteGridBoard([]string{posePath}, poseBoardPath, 2)
	require.NoError(t, err)
	candidate := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(candidate, image.Rect(1, 1, 15, 11), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidatePath := filepath.Join(dir, "study.png")
	writePNG(t, candidatePath, candidate)

	_, reasons, err := imageio.EvaluateMotionStudy(candidatePath, poseBoardPath, layout, 0)

	require.NoError(t, err)
	require.Contains(t, reasons, "cell_00_baseline_drift")
	require.Contains(t, reasons, "cell_00_occupied_bounds_drift")
}

func TestEvaluateBoardTreatsSeedPoseDriftAsWarningsWhenCellStructureIsSafe(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(pose, image.Rect(26, 18, 38, 56), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	writePNG(t, posePath, pose)
	poseBoardPath := filepath.Join(dir, "pose-board.png")
	layout, err := imageio.WriteGridBoard([]string{posePath}, poseBoardPath, 0)
	require.NoError(t, err)
	candidate := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(candidate, image.Rect(12, 8, 44, 48), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, candidatePath, candidate)

	evaluation, err := imageio.EvaluateBoard(candidatePath, poseBoardPath, layout, 2, imageio.BoardValidationCharacterMaster)

	require.NoError(t, err)
	require.Empty(t, evaluation.BlockingFailures)
	require.Contains(t, evaluation.Warnings, "cell_00_silhouette_overlap_below_threshold")
	require.Contains(t, evaluation.Warnings, "cell_00_baseline_drift")
	require.Contains(t, evaluation.Warnings, "cell_00_occupied_bounds_drift")
}

func TestEvaluateBoardTreatsAnimationRowLegacyPoseDriftAsWarnings(t *testing.T) {
	dir := t.TempDir()
	pose := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(pose, image.Rect(26, 18, 38, 56), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	posePath := filepath.Join(dir, "pose.png")
	writePNG(t, posePath, pose)
	poseBoardPath := filepath.Join(dir, "pose-board.png")
	layout, err := imageio.WriteGridBoard([]string{posePath}, poseBoardPath, 0)
	require.NoError(t, err)
	candidate := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(candidate, image.Rect(12, 8, 44, 48), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, candidatePath, candidate)

	evaluation, err := imageio.EvaluateBoard(candidatePath, poseBoardPath, layout, 2, imageio.BoardValidationAnimationBoard)

	require.NoError(t, err)
	require.Empty(t, evaluation.BlockingFailures)
	require.Contains(t, evaluation.Warnings, "cell_00_silhouette_overlap_below_threshold")
	require.Contains(t, evaluation.Warnings, "cell_00_baseline_drift")
	require.Contains(t, evaluation.Warnings, "cell_00_occupied_bounds_drift")
}

func TestEvaluateBoardTreatsInternalCellGuardContactAsReviewEvidence(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayout(1, 1, 64)
	require.NoError(t, err)
	cell := layout.Cell(0)
	guide := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(guide, image.Rect(cell.Min.X+12, cell.Min.Y+12, cell.Max.X-12, cell.Max.Y-12), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidate := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(candidate, image.Rect(cell.Min.X+1, cell.Min.Y+12, cell.Max.X-12, cell.Max.Y-12), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	guidePath := filepath.Join(dir, "guide.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, guidePath, guide)
	writePNG(t, candidatePath, candidate)

	evaluation, err := imageio.EvaluateBoard(candidatePath, guidePath, layout, 4, imageio.BoardValidationAnimationBoard)

	require.NoError(t, err)
	require.True(t, evaluation.Metrics.Cells[0].EdgeGuardOccupied)
	require.NotContains(t, evaluation.BlockingFailures, "cell_00_cell_edge_occupied")
	require.Contains(t, evaluation.Warnings, "cell_00_edge_guard_occupied")
	require.NotContains(t, evaluation.BlockingFailures, "board_margin_occupied")
	require.NotContains(t, evaluation.BlockingFailures, "board_gutter_occupied")
}

func TestEvaluateBoardRejectsCellEdgeContact(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayout(1, 1, 64)
	require.NoError(t, err)
	cell := layout.Cell(0)
	guide := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(guide, image.Rect(cell.Min.X+12, cell.Min.Y+12, cell.Max.X-12, cell.Max.Y-12), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidate := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(candidate, image.Rect(cell.Min.X, cell.Min.Y+12, cell.Max.X-12, cell.Max.Y-12), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	guidePath := filepath.Join(dir, "guide.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, guidePath, guide)
	writePNG(t, candidatePath, candidate)

	evaluation, err := imageio.EvaluateBoard(candidatePath, guidePath, layout, 4, imageio.BoardValidationAnimationBoard)

	require.NoError(t, err)
	require.Contains(t, evaluation.BlockingFailures, "cell_00_cell_edge_occupied")
}

func TestRegisterCharacterMasterBoardTranslatesCompleteSubjectsIntoSafeCells(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.FixedCellGridLayout(3, 2, 32, 4, 4, 80, 80)
	require.NoError(t, err)
	source := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	first := color.NRGBA{R: 120, G: 80, B: 220, A: 255}
	second := color.NRGBA{R: 220, G: 160, B: 40, A: 255}
	third := color.NRGBA{R: 40, G: 180, B: 120, A: 255}
	fillRect(source, image.Rect(15, 2, 25, 22), first)
	fillRect(source, image.Rect(50, 4, 60, 24), second)
	fillRect(source, image.Rect(15, 48, 25, 68), third)
	sourcePath := filepath.Join(dir, "source.png")
	outputPath := filepath.Join(dir, "registered.png")
	writePNG(t, sourcePath, source)

	registrations, err := imageio.RegisterCharacterMasterBoard(sourcePath, outputPath, layout, 3)

	require.NoError(t, err)
	require.Equal(t, []imageio.CellRegistration{
		{Cell: 0, OffsetX: 0, OffsetY: 7, SourceBounds: image.Rect(15, 2, 25, 22), RegisteredBounds: image.Rect(15, 9, 25, 29)},
		{Cell: 1, OffsetX: 0, OffsetY: 5, SourceBounds: image.Rect(50, 4, 60, 24), RegisteredBounds: image.Rect(50, 9, 60, 29)},
		{Cell: 2, OffsetX: 0, OffsetY: 0, SourceBounds: image.Rect(15, 48, 25, 68), RegisteredBounds: image.Rect(15, 48, 25, 68)},
	}, registrations)
	registered, err := os.Open(outputPath)
	require.NoError(t, err)
	defer registered.Close()
	result, err := png.Decode(registered)
	require.NoError(t, err)
	require.Equal(t, opaquePixelCount(source), opaquePixelCount(result), "registration must preserve every foreground pixel")
	require.Equal(t, first, color.NRGBAModel.Convert(result.At(15, 9)).(color.NRGBA))
	require.Equal(t, second, color.NRGBAModel.Convert(result.At(50, 9)).(color.NRGBA))
	require.Equal(t, third, color.NRGBAModel.Convert(result.At(15, 48)).(color.NRGBA))
}

func TestRegisterCharacterMasterBoardRejectsActualCanvasClipping(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.FixedCellGridLayout(1, 1, 32, 4, 4, 40, 40)
	require.NoError(t, err)
	source := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	fillRect(source, image.Rect(0, 8, 20, 28), color.NRGBA{R: 120, G: 80, B: 220, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	writePNG(t, sourcePath, source)

	_, err = imageio.RegisterCharacterMasterBoard(sourcePath, filepath.Join(dir, "registered.png"), layout, 3)

	require.ErrorContains(t, err, "provider canvas edge")
}

func TestRegisterCharacterMasterBoardRejectsMergedSubjectsAcrossCells(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.FixedCellGridLayout(2, 2, 32, 4, 4, 80, 48)
	require.NoError(t, err)
	source := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	left, right := layout.Cell(0), layout.Cell(1)
	fillRect(source, image.Rect(left.Min.X+8, left.Min.Y+8, right.Max.X-8, left.Max.Y-8), color.NRGBA{R: 120, G: 80, B: 220, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	writePNG(t, sourcePath, source)

	_, err = imageio.RegisterCharacterMasterBoard(sourcePath, filepath.Join(dir, "registered.png"), layout, 3)

	require.ErrorContains(t, err, "spans expected cells 00 and 01")
}

func TestRegisterCharacterMasterBoardRejectsSubjectThatCannotFitSafeCell(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.FixedCellGridLayout(1, 1, 32, 4, 4, 40, 40)
	require.NoError(t, err)
	cell := layout.Cell(0)
	source := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	fillRect(source, image.Rect(cell.Min.X-2, cell.Min.Y+2, cell.Max.X+2, cell.Max.Y-2), color.NRGBA{R: 120, G: 80, B: 220, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	writePNG(t, sourcePath, source)

	_, err = imageio.RegisterCharacterMasterBoard(sourcePath, filepath.Join(dir, "registered.png"), layout, 3)

	require.ErrorContains(t, err, "cannot fit safe cell")
}

func TestEvaluateBoardRejectsBackdropLikeForeground(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayout(1, 1, 64)
	require.NoError(t, err)
	cell := layout.Cell(0)
	guide := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(guide, image.Rect(cell.Min.X+20, cell.Min.Y+12, cell.Max.X-20, cell.Max.Y-12), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
	candidate := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(candidate, image.Rect(cell.Min.X+8, cell.Min.Y+8, cell.Max.X-8, cell.Max.Y-8), color.NRGBA{R: 10, G: 10, B: 10, A: 255})
	guidePath := filepath.Join(dir, "guide.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, guidePath, guide)
	writePNG(t, candidatePath, candidate)

	evaluation, err := imageio.EvaluateBoard(candidatePath, guidePath, layout, 4, imageio.BoardValidationCharacterMaster)

	require.NoError(t, err)
	require.Contains(t, evaluation.BlockingFailures, "cell_00_non_removable_background")
}

func TestEvaluateAnimationBoardRejectsPerCellBlackPanels(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.FixedCellGridLayout(2, 2, 32, 4, 4, 76, 40)
	require.NoError(t, err)
	guide := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	candidate := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	for index := 0; index < layout.Count; index++ {
		cell := layout.Cell(index)
		fillRect(guide, image.Rect(cell.Min.X+12, cell.Min.Y+8, cell.Max.X-12, cell.Max.Y-8), color.NRGBA{R: 100, G: 120, B: 180, A: 255})
		fillRect(candidate, image.Rect(cell.Min.X+2, cell.Min.Y+2, cell.Max.X-2, cell.Max.Y-2), color.NRGBA{R: 8, G: 8, B: 8, A: 255})
	}
	guidePath := filepath.Join(dir, "guide.png")
	candidatePath := filepath.Join(dir, "candidate.png")
	writePNG(t, guidePath, guide)
	writePNG(t, candidatePath, candidate)

	evaluation, err := imageio.EvaluateBoard(candidatePath, guidePath, layout, 4, imageio.BoardValidationAnimationBoard)

	require.NoError(t, err)
	require.Contains(t, evaluation.BlockingFailures, "cell_00_non_removable_background")
	require.Contains(t, evaluation.BlockingFailures, "cell_01_non_removable_background")
}

func TestWriteBoardInsetEditMaskExposesOnlyTargetPlacementBounds(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayout(4, 4, 1024)
	require.NoError(t, err)
	path := filepath.Join(dir, "mask.png")

	require.NoError(t, imageio.WriteBoardInsetEditMask(path, layout, 0, 48))

	file, err := os.Open(path)
	require.NoError(t, err)
	mask, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	locked := layout.Cell(0)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(locked.Min.X+locked.Dx()/2, locked.Min.Y+locked.Dy()/2)).(color.NRGBA).A)
	editable := layout.Cell(1)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(editable.Min.X+47, editable.Min.Y+48)).(color.NRGBA).A)
	require.Zero(t, color.NRGBAModel.Convert(mask.At(editable.Min.X+48, editable.Min.Y+48)).(color.NRGBA).A)
	require.Zero(t, color.NRGBAModel.Convert(mask.At(editable.Max.X-49, editable.Max.Y-49)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(editable.Max.X-48, editable.Max.Y-48)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(editable.Max.X+1, editable.Min.Y+editable.Dy()/2)).(color.NRGBA).A)
}

func TestWriteBoardEditMaskTargetsReservesForwardMotionSpace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mask.png")
	layout := imageio.GridLayout{Columns: 2, Rows: 1, CanvasWidth: 64, CanvasHeight: 32, CellWidth: 32, CellHeight: 32, Count: 2}

	require.NoError(t, imageio.WriteBoardEditMaskTargets(path, layout, 0, []image.Rectangle{
		image.Rect(6, 6, 26, 26),
		image.Rect(34, 6, 54, 26),
	}))

	file, err := os.Open(path)
	require.NoError(t, err)
	mask, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(33, 10)).(color.NRGBA).A)
	require.Zero(t, color.NRGBAModel.Convert(mask.At(34, 10)).(color.NRGBA).A)
	require.Zero(t, color.NRGBAModel.Convert(mask.At(53, 10)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(54, 10)).(color.NRGBA).A)
}

func TestWriteCellInsetEditMaskExposesOnlySelectedSafeArea(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayout(4, 4, 1024)
	require.NoError(t, err)
	path := filepath.Join(dir, "mask.png")

	require.NoError(t, imageio.WriteCellInsetEditMask(path, layout, 2, 42))

	file, err := os.Open(path)
	require.NoError(t, err)
	mask, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	protected := layout.Cell(1)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(protected.Min.X+protected.Dx()/2, protected.Min.Y+protected.Dy()/2)).(color.NRGBA).A)
	editable := layout.Cell(2)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(editable.Min.X+41, editable.Min.Y+42)).(color.NRGBA).A)
	require.Zero(t, color.NRGBAModel.Convert(mask.At(editable.Min.X+42, editable.Min.Y+42)).(color.NRGBA).A)
	require.Zero(t, color.NRGBAModel.Convert(mask.At(editable.Max.X-43, editable.Max.Y-43)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(editable.Max.X-42, editable.Max.Y-42)).(color.NRGBA).A)
}

func TestRestoreProtectedBoardCellsKeepsOnlySelectedProviderCell(t *testing.T) {
	dir := t.TempDir()
	layout := imageio.GridLayout{
		Columns:      2,
		Rows:         1,
		CanvasWidth:  32,
		CanvasHeight: 16,
		CellWidth:    16,
		CellHeight:   16,
		Count:        2,
	}
	source := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	fillRect(source, layout.Cell(0), color.NRGBA{R: 240, A: 255})
	fillRect(source, layout.Cell(1), color.NRGBA{B: 240, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	writePNG(t, sourcePath, source)
	generated := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	fillRect(generated, generated.Bounds(), color.NRGBA{G: 240, A: 255})
	generatedPath := filepath.Join(dir, "generated.png")
	writePNG(t, generatedPath, generated)

	require.NoError(t, imageio.RestoreProtectedBoardCells(generatedPath, sourcePath, generatedPath, layout, 1))

	file, err := os.Open(generatedPath)
	require.NoError(t, err)
	restored, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, color.NRGBA{R: 240, A: 255}, color.NRGBAModel.Convert(restored.At(4, 4)).(color.NRGBA))
	require.Equal(t, color.NRGBA{G: 240, A: 255}, color.NRGBAModel.Convert(restored.At(20, 4)).(color.NRGBA))
}

func TestRestoreLockedBoardCellKeepsGeneratedEditableCells(t *testing.T) {
	dir := t.TempDir()
	layout := imageio.GridLayout{
		Columns:      2,
		Rows:         1,
		CanvasWidth:  32,
		CanvasHeight: 16,
		CellWidth:    16,
		CellHeight:   16,
		Count:        2,
	}
	source := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	fillRect(source, layout.Cell(0), color.NRGBA{R: 240, A: 255})
	fillRect(source, layout.Cell(1), color.NRGBA{B: 240, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	writePNG(t, sourcePath, source)
	generated := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	fillRect(generated, generated.Bounds(), color.NRGBA{G: 240, A: 255})
	generatedPath := filepath.Join(dir, "generated.png")
	writePNG(t, generatedPath, generated)

	require.NoError(t, imageio.RestoreLockedBoardCell(generatedPath, sourcePath, generatedPath, layout, 0))

	file, err := os.Open(generatedPath)
	require.NoError(t, err)
	restored, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, color.NRGBA{R: 240, A: 255}, color.NRGBAModel.Convert(restored.At(4, 4)).(color.NRGBA))
	require.Equal(t, color.NRGBA{G: 240, A: 255}, color.NRGBAModel.Convert(restored.At(20, 4)).(color.NRGBA))
}

func TestWriteCanvasBoardAtFrameScalePreservesTransparentFrameCoordinates(t *testing.T) {
	dir := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(source, image.Rect(2, 3, 14, 15), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	writePNG(t, sourcePath, source)
	layout, err := imageio.CanvasGridLayout(1, 1, 64)
	require.NoError(t, err)
	boardPath := filepath.Join(dir, "board.png")

	require.NoError(t, imageio.WriteCanvasBoardAtFrameScale([]string{sourcePath}, boardPath, layout, 16, 16))

	file, err := os.Open(boardPath)
	require.NoError(t, err)
	board, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	frame := image.Rect(24, 24, 40, 40)
	require.Zero(t, color.NRGBAModel.Convert(board.At(frame.Min.X, frame.Min.Y)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(board.At(frame.Min.X+2, frame.Min.Y+3)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(board.At(frame.Max.X-3, frame.Max.Y-2)).(color.NRGBA).A)
}

func TestWriteCanvasBoardAtFrameScaleWithBlankCellsCreatesTargetCanvas(t *testing.T) {
	dir := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(source, image.Rect(2, 3, 14, 15), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	writePNG(t, sourcePath, source)
	layout := imageio.GridLayout{Columns: 2, Rows: 1, CanvasWidth: 64, CanvasHeight: 32, CellWidth: 32, CellHeight: 32, Count: 2}
	boardPath := filepath.Join(dir, "board.png")

	require.NoError(t, imageio.WriteCanvasBoardAtFrameScaleWithBlankCells([]string{sourcePath, ""}, boardPath, layout, 16, 16))

	file, err := os.Open(boardPath)
	require.NoError(t, err)
	board, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(board.At(10, 11)).(color.NRGBA).A)
	require.Zero(t, color.NRGBAModel.Convert(board.At(42, 11)).(color.NRGBA).A)
}

func TestWriteFixedFrameCellsPreservesCoordinatesAndRejectsOutsideForeground(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayout(1, 1, 64)
	require.NoError(t, err)
	board := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	frame := image.Rect(24, 24, 40, 40)
	fillRect(board, frame.Inset(2), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	boardPath := filepath.Join(dir, "board.png")
	writePNG(t, boardPath, board)
	seed := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(seed, image.Rect(2, 2, 14, 14), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	seedPath := filepath.Join(dir, "seed.png")
	writePNG(t, seedPath, seed)
	outputPath := filepath.Join(dir, "frame.png")

	_, transforms, err := imageio.WriteFixedFrameCells(boardPath, layout, []string{outputPath}, 16, 16, nil, seedPath, false)

	require.NoError(t, err)
	require.Equal(t, 1.0, transforms[0].Scale)
	require.Zero(t, transforms[0].OffsetX)
	require.Zero(t, transforms[0].OffsetY)
	file, err := os.Open(outputPath)
	require.NoError(t, err)
	extracted, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Zero(t, color.NRGBAModel.Convert(extracted.At(0, 0)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(extracted.At(2, 2)).(color.NRGBA).A)

	board = image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(board, image.Rect(frame.Min.X-1, frame.Min.Y+2, frame.Max.X-1, frame.Max.Y-2), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	writePNG(t, boardPath, board)
	_, _, err = imageio.WriteFixedFrameCells(boardPath, layout, []string{outputPath}, 16, 16, nil, seedPath, false)
	require.ErrorIs(t, err, imageio.ErrProductionFrameClipping)

	board = image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(board, frame.Inset(1), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	writePNG(t, boardPath, board)
	_, transforms, err = imageio.WriteFixedFrameCells(boardPath, layout, []string{outputPath}, 16, 16, nil, seedPath, false)
	require.NoError(t, err)
	require.Equal(t, 1.0, transforms[0].Scale, "provider-returned frame 00 drift must not rescale the conditioned board")

	board = image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fillRect(board, image.Rect(frame.Min.X-1, frame.Min.Y+2, frame.Max.X+1, frame.Max.Y-2), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	writePNG(t, boardPath, board)
	_, _, err = imageio.WriteFixedFrameCells(boardPath, layout, []string{outputPath}, 16, 16, nil, seedPath, false)
	require.ErrorIs(t, err, imageio.ErrProductionFrameClipping)
}

func TestWriteFixedFrameCellsRejectsCanonicalCanvasEdgeContact(t *testing.T) {
	dir := t.TempDir()
	layout := imageio.GridLayout{
		Columns:      2,
		Rows:         1,
		CanvasWidth:  32,
		CanvasHeight: 16,
		CellWidth:    16,
		CellHeight:   16,
		Count:        2,
	}
	board := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	fillRect(board, image.Rect(2, 2, 14, 14), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	fillRect(board, image.Rect(17, 0, 31, 15), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	boardPath := filepath.Join(dir, "board.png")
	writePNG(t, boardPath, board)
	seed := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(seed, image.Rect(2, 2, 14, 14), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	seedPath := filepath.Join(dir, "seed.png")
	writePNG(t, seedPath, seed)

	_, _, err := imageio.WriteFixedFrameCells(
		boardPath,
		layout,
		[]string{filepath.Join(dir, "00.png"), filepath.Join(dir, "01.png")},
		16,
		16,
		nil,
		seedPath,
		false,
	)

	require.ErrorIs(t, err, imageio.ErrProductionFrameClipping)
}

func TestWriteFixedFrameCellsKeepsTransparentProductionEdge(t *testing.T) {
	dir := t.TempDir()
	layout := imageio.GridLayout{
		Columns:      2,
		Rows:         1,
		CanvasWidth:  320,
		CanvasHeight: 160,
		CellWidth:    160,
		CellHeight:   160,
		Count:        2,
	}
	board := image.NewNRGBA(image.Rect(0, 0, 320, 160))
	fillRect(board, image.Rect(16, 16, 144, 144), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	fillRect(board, image.Rect(160, 16, 320, 144), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	boardPath := filepath.Join(dir, "board.png")
	writePNG(t, boardPath, board)
	seed := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	fillRect(seed, image.Rect(16, 16, 144, 144), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	seedPath := filepath.Join(dir, "seed.png")
	writePNG(t, seedPath, seed)

	_, _, err := imageio.WriteFixedFrameCells(
		boardPath,
		layout,
		[]string{filepath.Join(dir, "00.png"), filepath.Join(dir, "01.png")},
		160,
		160,
		nil,
		seedPath,
		false,
	)

	require.ErrorIs(t, err, imageio.ErrProductionFrameClipping)
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

func TestCanvasGridLayoutCentersThreeCellsInTwoByTwoSquare(t *testing.T) {
	layout, err := imageio.CanvasGridLayout(3, 4, 1024)

	require.NoError(t, err)
	require.Equal(t, 2, layout.Columns)
	require.Equal(t, 2, layout.Rows)
	require.Equal(t, 16, layout.Gutter)
	require.Equal(t, image.Rect(32, 32, 504, 504), layout.Cell(0))
	require.Equal(t, image.Rect(520, 520, 992, 992), layout.Cell(3))
}

func TestWriteGridCellCopiesPreservesConditionedCellGeometry(t *testing.T) {
	dir := t.TempDir()
	layout := imageio.GridLayout{Columns: 2, Rows: 1, CellWidth: 8, CellHeight: 8, Gutter: 2, Count: 2}
	board := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	fillRect(board, image.Rect(2, 3, 5, 7), color.NRGBA{R: 255, A: 255})
	fillRect(board, image.Rect(12, 1, 17, 6), color.NRGBA{B: 255, A: 255})
	boardPath := filepath.Join(dir, "board.png")
	writePNG(t, boardPath, board)
	outputs := []string{filepath.Join(dir, "first.png"), filepath.Join(dir, "second.png")}

	require.NoError(t, imageio.WriteGridCellCopies(boardPath, layout, outputs))

	firstBounds, err := imageio.ForegroundBounds(outputs[0])
	require.NoError(t, err)
	require.Equal(t, image.Rect(2, 3, 5, 7), firstBounds)
	secondBounds, err := imageio.ForegroundBounds(outputs[1])
	require.NoError(t, err)
	require.Equal(t, image.Rect(2, 1, 7, 6), secondBounds)
}

func TestCanvasGridLayoutUsesOneHorizontalRowForFourFrames(t *testing.T) {
	layout, err := imageio.CanvasGridLayout(4, 4, 1024)

	require.NoError(t, err)
	require.Equal(t, 4, layout.Columns)
	require.Equal(t, 1, layout.Rows)
	require.Equal(t, 16, layout.Gutter)
	require.Equal(t, image.Rect(32, 398, 260, 626), layout.Cell(0))
	require.Equal(t, image.Rect(764, 398, 992, 626), layout.Cell(3))
}

func TestAnimationRowLayoutUsesOneHorizontalProviderRow(t *testing.T) {
	layout, err := imageio.AnimationRowLayout(1024)

	require.NoError(t, err)
	require.Equal(t, 4, layout.Columns)
	require.Equal(t, 1, layout.Rows)
	require.Equal(t, 16, layout.Gutter)
	require.Equal(t, image.Rect(32, 398, 260, 626), layout.Cell(0))
	require.Equal(t, image.Rect(764, 398, 992, 626), layout.Cell(3))
}

func TestCanvasGridLayoutRectUsesFixedPortraitFourBySixGeometry(t *testing.T) {
	layout, err := imageio.CanvasGridLayoutRect(24, 4, 6, 1024, 1536)

	require.NoError(t, err)
	require.Equal(t, 4, layout.Columns)
	require.Equal(t, 6, layout.Rows)
	require.Equal(t, 1024, layout.Width())
	require.Equal(t, 1536, layout.Height())
	require.Equal(t, 16, layout.Gutter)
	require.Equal(t, image.Rect(32, 32, 260, 264), layout.Cell(0))
	require.Equal(t, image.Rect(764, 1272, 992, 1504), layout.Cell(23))
}

func TestProtectedBoardMaskAndRestoreKeepEveryListedCellByteIdentical(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayoutRect(24, 4, 6, 1024, 1536)
	require.NoError(t, err)
	protected := []int{0, 4, 8, 12, 16, 20}

	source := image.NewNRGBA(image.Rect(0, 0, layout.Width(), layout.Height()))
	generated := image.NewNRGBA(source.Bounds())
	fillRect(generated, generated.Bounds(), color.NRGBA{R: 20, G: 40, B: 60, A: 255})
	for index, cell := range protected {
		fillRect(source, layout.Cell(cell).Inset(12), color.NRGBA{R: uint8(80 + index), G: 120, B: 200, A: 255})
	}
	sourcePath := filepath.Join(dir, "source.png")
	generatedPath := filepath.Join(dir, "generated.png")
	maskPath := filepath.Join(dir, "mask.png")
	resultPath := filepath.Join(dir, "restored.png")
	writePNG(t, sourcePath, source)
	writePNG(t, generatedPath, generated)

	require.NoError(t, imageio.WriteBoardInsetEditMaskProtected(maskPath, layout, protected, 8))
	require.NoError(t, imageio.RestoreLockedBoardCells(generatedPath, sourcePath, resultPath, layout, protected))
	for _, cell := range protected {
		requirePNGRectEqual(t, sourcePath, resultPath, layout.Cell(cell))
	}
}

func TestRectangularProtectedBoardMaskPreservesExactProductionExtent(t *testing.T) {
	dir := t.TempDir()
	layout, err := imageio.CanvasGridLayoutRect(24, 4, 6, 1024, 1536)
	require.NoError(t, err)
	maskPath := filepath.Join(dir, "mask.png")

	require.NoError(t, imageio.WriteBoardRectInsetEditMaskProtected(maskPath, layout, []int{0}, 35, 37))

	file, err := os.Open(maskPath)
	require.NoError(t, err)
	defer file.Close()
	mask, err := png.Decode(file)
	require.NoError(t, err)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(146, 148)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(310, 68)).(color.NRGBA).A)
	require.Equal(t, uint8(0), color.NRGBAModel.Convert(mask.At(311, 69)).(color.NRGBA).A)
	require.Equal(t, uint8(0), color.NRGBAModel.Convert(mask.At(468, 226)).(color.NRGBA).A)
	require.Equal(t, uint8(255), color.NRGBAModel.Convert(mask.At(469, 227)).(color.NRGBA).A)
}

func TestWriteCanvasBoardAtConditioningScaleReturnsExactInverseContract(t *testing.T) {
	dir := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(source, image.Rect(4, 4, 12, 12), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	sourcePath := filepath.Join(dir, "source.png")
	boardPath := filepath.Join(dir, "board.png")
	writePNG(t, sourcePath, source)
	layout := imageio.GridLayout{Columns: 1, Rows: 1, CellWidth: 32, CellHeight: 32, Count: 1}

	scale, err := imageio.WriteCanvasBoardAtConditioningScale([]string{sourcePath}, boardPath, layout, 4)

	require.NoError(t, err)
	require.Equal(t, 3.0, scale)
	file, err := os.Open(boardPath)
	require.NoError(t, err)
	board, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, color.NRGBA{R: 80, G: 120, B: 200, A: 255}, color.NRGBAModel.Convert(board.At(4, 4)))
	require.Equal(t, color.NRGBA{R: 80, G: 120, B: 200, A: 255}, color.NRGBAModel.Convert(board.At(27, 27)))
}

func TestWriteFixedFrameCellsAppliesFixedConditioningInverse(t *testing.T) {
	dir := t.TempDir()
	seed := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(seed, image.Rect(4, 4, 12, 14), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	seedPath := filepath.Join(dir, "seed.png")
	writePNG(t, seedPath, seed)
	board := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	fillRect(board, image.Rect(4, 4, 28, 28), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	boardPath := filepath.Join(dir, "board.png")
	outputPath := filepath.Join(dir, "output.png")
	writePNG(t, boardPath, board)
	layout := imageio.GridLayout{Columns: 1, Rows: 1, CellWidth: 32, CellHeight: 32, Count: 1}

	_, transforms, err := imageio.WriteFixedFrameCellsAtScale(boardPath, layout, []string{outputPath}, 16, 16, nil, seedPath, false, 0.5)

	require.NoError(t, err)
	require.Equal(t, 0.5, transforms[0].Scale)
	require.NoError(t, imageio.ValidateCanonicalFrame(outputPath, 16, 16))
}

func TestWriteCanvasBoardAtNativeScaleDoesNotEnlargeCanonicalSprites(t *testing.T) {
	dir := t.TempDir()
	first := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(first, image.Rect(4, 4, 12, 14), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	second := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(second, image.Rect(2, 2, 14, 14), color.NRGBA{R: 200, G: 120, B: 80, A: 255})
	firstPath, secondPath := filepath.Join(dir, "first.png"), filepath.Join(dir, "second.png")
	writePNG(t, firstPath, first)
	writePNG(t, secondPath, second)
	layout, err := imageio.CanvasGridLayout(2, 4, 64)
	require.NoError(t, err)
	boardPath := filepath.Join(dir, "board.png")

	require.NoError(t, imageio.WriteCanvasBoardAtNativeScale([]string{firstPath, secondPath}, boardPath, layout, 4))

	file, err := os.Open(boardPath)
	require.NoError(t, err)
	board, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	for index, expected := range []image.Point{image.Pt(8, 10), image.Pt(12, 12)} {
		bounds := occupiedBounds(t, board, layout.Cell(index))
		require.Equal(t, expected, bounds.Size())
		require.Equal(t, layout.Cell(index).Max.Y-4, bounds.Max.Y)
	}
}

func TestWriteSharedNormalizedCellsUsesOneScaleAndBottomCenterAnchor(t *testing.T) {
	dir := t.TempDir()
	first := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(first, image.Rect(4, 4, 12, 14), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	second := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(second, image.Rect(2, 2, 14, 14), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	firstPath, secondPath := filepath.Join(dir, "first.png"), filepath.Join(dir, "second.png")
	writePNG(t, firstPath, first)
	writePNG(t, secondPath, second)
	layout, err := imageio.CanvasGridLayout(2, 4, 64)
	require.NoError(t, err)
	board := filepath.Join(dir, "board.png")
	require.NoError(t, imageio.WriteCanvasBoard([]string{firstPath, secondPath}, board, layout, 4))
	outputs := []string{filepath.Join(dir, "00.png"), filepath.Join(dir, "01.png")}

	palette, err := imageio.WriteSharedNormalizedCells(board, layout, outputs, 16, 16, nil, "")

	require.NoError(t, err)
	require.NotEmpty(t, palette)
	for _, output := range outputs {
		dimensions, sizeErr := imageio.PNGDimensions(output)
		require.NoError(t, sizeErr)
		require.Equal(t, image.Pt(16, 16), dimensions)
	}
}

func TestWriteSharedNormalizedCellsReservesAnimationHeadroomAtProductionSize(t *testing.T) {
	dir := t.TempDir()
	layout := imageio.GridLayout{Columns: 1, Rows: 1, CellWidth: 32, CellHeight: 32, Count: 1}
	boardImage := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	fillRect(boardImage, image.Rect(8, 8, 24, 24), color.NRGBA{R: 120, G: 160, B: 220, A: 255})
	boardPath := filepath.Join(dir, "board.png")
	outputPath := filepath.Join(dir, "seed.png")
	writePNG(t, boardPath, boardImage)

	_, err := imageio.WriteSharedNormalizedCells(boardPath, layout, []string{outputPath}, 160, 160, nil, "")

	require.NoError(t, err)
	bounds, err := imageio.ForegroundBounds(outputPath)
	require.NoError(t, err)
	require.Equal(t, image.Rect(16, 16, 144, 144), bounds)
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

func TestCanonicalNormalizationRejectsInsteadOfShrinkingOversizedRowFrame(t *testing.T) {
	dir := t.TempDir()
	canonical := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fillRect(canonical, image.Rect(4, 0, 12, 1), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	canonicalPath := filepath.Join(dir, "canonical.png")
	writePNG(t, canonicalPath, canonical)
	canonicalFile, err := os.Open(canonicalPath)
	require.NoError(t, err)
	decodedCanonical, err := png.Decode(canonicalFile)
	require.NoError(t, err)
	require.NoError(t, canonicalFile.Close())
	require.Zero(t, color.NRGBAModel.Convert(decodedCanonical.At(15, 15)).(color.NRGBA).A)
	layout, err := imageio.CanvasGridLayout(2, 4, 64)
	require.NoError(t, err)
	boardPath := filepath.Join(dir, "row.png")
	board := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	firstCell := layout.Cell(0)
	fillRect(board, image.Rect(firstCell.Min.X+4, firstCell.Min.Y+4, firstCell.Min.X+12, firstCell.Min.Y+14), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	fillRect(board, layout.Cell(1), color.NRGBA{R: 80, G: 120, B: 200, A: 255})
	writePNG(t, boardPath, board)

	_, transform, err := imageio.WriteCanonicalNormalizedCells(boardPath, layout, []string{filepath.Join(dir, "00.png"), filepath.Join(dir, "01.png")}, 16, 16, nil, canonicalPath, false)

	require.InDelta(t, 0.1, transform.Scale, 0.001)
	require.Equal(t, 1, transform.Baseline)
	require.ErrorIs(t, err, imageio.ErrProductionFrameClipping)
	require.NoFileExists(t, filepath.Join(dir, "00.png"))
}

func occupiedBounds(t *testing.T, img image.Image, scope image.Rectangle) image.Rectangle {
	t.Helper()
	bounds := image.Rectangle{Min: scope.Max, Max: scope.Min}
	found := false
	for y := scope.Min.Y; y < scope.Max.Y; y++ {
		for x := scope.Min.X; x < scope.Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A == 0 {
				continue
			}
			found = true
			bounds.Min.X = min(bounds.Min.X, x)
			bounds.Min.Y = min(bounds.Min.Y, y)
			bounds.Max.X = max(bounds.Max.X, x+1)
			bounds.Max.Y = max(bounds.Max.Y, y+1)
		}
	}
	require.True(t, found)
	return bounds
}

func requirePNGRectEqual(t *testing.T, expectedPath, actualPath string, scope image.Rectangle) {
	t.Helper()
	expectedFile, err := os.Open(expectedPath)
	require.NoError(t, err)
	expected, err := png.Decode(expectedFile)
	require.NoError(t, err)
	require.NoError(t, expectedFile.Close())
	actualFile, err := os.Open(actualPath)
	require.NoError(t, err)
	actual, err := png.Decode(actualFile)
	require.NoError(t, err)
	require.NoError(t, actualFile.Close())
	for y := scope.Min.Y; y < scope.Max.Y; y++ {
		for x := scope.Min.X; x < scope.Max.X; x++ {
			require.Equal(t, expected.At(x, y), actual.At(x, y), "pixel (%d,%d)", x, y)
		}
	}
}
