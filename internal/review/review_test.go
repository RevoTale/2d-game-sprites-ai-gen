package review_test

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/review"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestReviewRejectRequiresReason(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusRejected})

	require.Error(t, err)
	require.Contains(t, err.Error(), "requires --reason")
}

func TestReviewAcceptReasonIsOptionalAndWritesManualAuditNote(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	result, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusAccepted})

	require.NoError(t, err)
	require.Equal(t, 1, result.Reviewed)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	state := manifest.Targets["grass"]
	require.Equal(t, generate.StatusAccepted, state.Status)
	require.Contains(t, state.Review.Reason, "manual visual review")
	require.FileExists(t, state.Artifacts.PromptPath)
	require.FileExists(t, state.Artifacts.QAPath)
	qa, err := os.ReadFile(state.Artifacts.QAPath)
	require.NoError(t, err)
	require.Contains(t, string(qa), "Status: accepted")
}

func TestReviewBulkAcceptsGeneratedTargetsOnly(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	result, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Status: generate.StatusAccepted})

	require.NoError(t, err)
	require.Equal(t, 1, result.Reviewed)
	require.Greater(t, result.SkippedPending, 0)
}

func TestFrameReviewAcceptanceChangesTheCompleteRowStatus(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}, Frame: "00"}
	require.NoError(t, generateReviewedRow(t, all, outputDir, filter))
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.True(t, manifest.Targets["blood-duelist__attack__direction-right__00"].ProductionEligible)
	rowID := manifest.Targets["blood-duelist__attack__direction-right__00"].AnimationRowID

	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})

	require.NoError(t, err)
	manifest, err = generate.Load(outputDir, "run")
	require.NoError(t, err)
	for _, frame := range []string{"00", "contact"} {
		require.Equal(t, generate.StatusAccepted, manifest.Targets["blood-duelist__attack__direction-right__"+frame].Status)
	}
	row := manifest.Intermediates[rowID]
	require.Equal(t, generate.StatusAccepted, row.Status)
	require.FileExists(t, row.Artifacts.QAPath)
	qa, err := os.ReadFile(row.Artifacts.QAPath)
	require.NoError(t, err)
	require.Contains(t, string(qa), "Status: accepted")
	require.Contains(t, string(qa), "Accepted by manual visual review.")
}

func TestSeedReviewRequiresObjectWideCandidateSelection(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)

	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Stage: "seed", Candidate: "01", Status: generate.StatusAccepted})

	require.ErrorContains(t, err, "object-wide scope")
}

func TestSeedRejectionRequiresReasonButNotCandidate(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)

	result, err := review.Apply(all, review.Options{
		OutputDir: outputDir,
		RunID:     "run",
		Filter:    targets.Filter{Object: "blood-duelist"},
		Stage:     "seed",
		Status:    generate.StatusRejected,
		Reason:    "no mechanically valid seed candidate",
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Reviewed)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	seed := manifest.Intermediates["direction-seed-board:blood-duelist"]
	require.Equal(t, generate.StatusRejected, seed.Status)
	require.Empty(t, seed.Review.Candidate)
	require.FileExists(t, seed.Artifacts.PromptPath)
	require.FileExists(t, seed.Artifacts.QAPath)
}

func TestSeedReviewInvalidCandidateReportsStructurallyEligibleCandidates(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	seed := manifest.Intermediates["direction-seed-board:blood-duelist"]
	invalid := image.NewNRGBA(image.Rect(0, 0, seed.Layout.Width(), seed.Layout.Height()))
	for y := 0; y < invalid.Bounds().Dy(); y++ {
		invalid.SetNRGBA(0, y, color.NRGBA{R: 255, A: 255})
	}
	require.NoError(t, writeReviewPNG(seed.Attempts[0].Candidates[0].NormalizedPath, invalid))
	seed.Attempts[0].Candidates[0].QualityVersion = 0
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "blood-duelist"}, Stage: "seed", Candidate: "01", Status: generate.StatusAccepted})

	require.ErrorContains(t, err, "failed structural validation")
	require.ErrorContains(t, err, "eligible candidates: 02, 03")
}

func TestSeedReviewAcceptsStructurallySafeCandidateAndReportsPoseWarnings(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	seed := manifest.Intermediates["direction-seed-board:blood-duelist"]
	candidate := image.NewNRGBA(image.Rect(0, 0, seed.Layout.Width(), seed.Layout.Height()))
	for index := 0; index < seed.Layout.Count; index++ {
		cell := seed.Layout.Cell(index)
		fillReviewRect(candidate, cell.Inset(180), color.NRGBA{R: 180, G: 120, B: 60, A: 255})
	}
	require.NoError(t, writeReviewPNG(seed.Attempts[0].Candidates[0].NormalizedPath, candidate))
	seed.Attempts[0].Candidates[0].QualityVersion = 0
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	result, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "blood-duelist"}, Stage: "seed", Candidate: "01", Status: generate.StatusAccepted})

	require.NoError(t, err)
	require.NotEmpty(t, result.Warnings)
	manifest, err = generate.Load(outputDir, "run")
	require.NoError(t, err)
	seed = manifest.Intermediates["direction-seed-board:blood-duelist"]
	require.Equal(t, generate.StatusAccepted, seed.Status)
	require.Equal(t, "01", seed.Review.Candidate)
	require.Equal(t, result.Warnings, seed.Warnings)
}

func TestRowCandidateSelectionRejectsPartialFrameScope(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	row := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	require.NoError(t, generateReviewedRow(t, all, outputDir, row))
	frame := row
	frame.Frame = "00"

	_, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: frame, Candidate: "02", Status: generate.StatusAccepted})

	require.ErrorContains(t, err, "only for directional-seed review")
}

func generateReviewedRow(t *testing.T, all []targets.Target, outputDir string, filter targets.Filter) error {
	t.Helper()
	generationFilter := filter
	generationFilter.Frame = ""
	if _, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: generationFilter}); err != nil {
		return err
	}
	if err := generate.SelectSeedCandidate(all, outputDir, "run", "blood-duelist", "01", generate.StatusAccepted, "Approved in test."); err != nil {
		return err
	}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: generationFilter})
	return err
}

func writeReviewPNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func fillReviewRect(img *image.NRGBA, bounds image.Rectangle, value color.NRGBA) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetNRGBA(x, y, value)
		}
	}
}
