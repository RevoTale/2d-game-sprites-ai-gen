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
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusRejected})

	require.ErrorContains(t, err, "requires --reason")
}

func TestStaticReviewBehaviorRemainsTargetAtomic(t *testing.T) {
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
	require.Equal(t, generate.StatusAccepted, manifest.Targets["grass"].Status)
	require.Contains(t, manifest.Targets["grass"].Review.Reason, "manual visual review")
}

func TestAnimatedReviewAcceptsCompleteUnitOnly(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "relic-knight"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{
		OutputDir: outputDir, DeployDir: filepath.Join(dir, p.DeployDir), RunID: "run", Filter: filter,
	})
	require.NoError(t, err)

	result, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})

	require.NoError(t, err)
	require.Equal(t, 24, result.Reviewed)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusAccepted, manifest.Units["unit:relic-knight"].Status)
	for _, targetID := range manifest.Units["unit:relic-knight"].TargetIDs {
		require.Equal(t, generate.StatusAccepted, manifest.Targets[targetID].Status)
		require.Equal(t, manifest.Units["unit:relic-knight"].Review.Reason, manifest.Targets[targetID].Review.Reason)
	}
}

func TestAnimatedPartialReviewIsRejected(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{
		OutputDir: outputDir, DeployDir: filepath.Join(dir, p.DeployDir), RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})
	require.NoError(t, err)

	_, err = review.Apply(all, review.Options{
		OutputDir: outputDir, RunID: "run",
		Filter: targets.Filter{Object: "relic-knight", Animation: "walk"},
		Status: generate.StatusAccepted,
	})

	require.ErrorContains(t, err, "unit-atomic")
}

func TestRejectedUnitRecordsImmutableCompleteUnitDecision(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "relic-knight"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{
		OutputDir: outputDir, DeployDir: filepath.Join(dir, p.DeployDir), RunID: "run", Filter: filter,
	})
	require.NoError(t, err)

	_, err = review.Apply(all, review.Options{
		OutputDir: outputDir, RunID: "run", Filter: filter,
		Status: generate.StatusRejected, Reason: "walk identity drift",
	})

	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusRejected, manifest.Units["unit:relic-knight"].Status)
}
