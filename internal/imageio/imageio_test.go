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
	require.Contains(t, reasons, "board_gutter_occupied")
	require.Contains(t, reasons, "board_trailing_cell_occupied")
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

	evaluation, err := imageio.EvaluateBoard(candidatePath, poseBoardPath, layout, 2, imageio.BoardValidationSeed)

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

	evaluation, err := imageio.EvaluateBoard(candidatePath, poseBoardPath, layout, 2, imageio.BoardValidationAnimationRow)

	require.NoError(t, err)
	require.Empty(t, evaluation.BlockingFailures)
	require.Contains(t, evaluation.Warnings, "cell_00_silhouette_overlap_below_threshold")
	require.Contains(t, evaluation.Warnings, "cell_00_baseline_drift")
	require.Contains(t, evaluation.Warnings, "cell_00_occupied_bounds_drift")
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

func TestCanvasGridLayoutUsesOneHorizontalRowForFourFrames(t *testing.T) {
	layout, err := imageio.CanvasGridLayout(4, 4, 1024)

	require.NoError(t, err)
	require.Equal(t, 4, layout.Columns)
	require.Equal(t, 1, layout.Rows)
	require.Equal(t, 16, layout.Gutter)
	require.Equal(t, image.Rect(32, 398, 260, 626), layout.Cell(0))
	require.Equal(t, image.Rect(764, 398, 992, 626), layout.Cell(3))
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
	require.ErrorIs(t, err, imageio.ErrCanonicalScaleCropping)
	require.NoFileExists(t, filepath.Join(dir, "00.png"))
}
