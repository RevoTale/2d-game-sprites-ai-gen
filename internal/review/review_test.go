package review_test

import (
	"context"
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
	_, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusRejected})

	require.Error(t, err)
	require.Contains(t, err.Error(), "requires --reason")
}

func TestReviewAcceptReasonIsOptionalAndWritesBulkAuditNote(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	result, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusAccepted})

	require.NoError(t, err)
	require.Equal(t, 1, result.Reviewed)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusAccepted, manifest.Targets["grass"].Status)
	require.Contains(t, manifest.Targets["grass"].Review.Reason, "Bulk accepted")
}

func TestReviewBulkAcceptsGeneratedTargetsOnly(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	result, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Status: generate.StatusAccepted})

	require.Error(t, err)
	require.Equal(t, 1, result.Reviewed)
	require.Greater(t, result.SkippedPending, 0)
}
