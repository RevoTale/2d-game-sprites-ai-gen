package generate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestRunIDCollisionAppendsSequence(t *testing.T) {
	outputDir := t.TempDir()
	now := time.Date(2026, 7, 3, 14, 7, 0, 0, time.UTC)
	require.NoError(t, os.MkdirAll(filepath.Join(outputDir, "runs", "2026-07-03-m0847"), 0o755))

	runID, err := generate.AutoRunID(now, outputDir)

	require.NoError(t, err)
	require.Equal(t, "2026-07-03-m0847-02", runID)
}

func TestGenerateFailsRequiredReferencesWhenProviderDoesNotSupportReferences(t *testing.T) {
	target := targets.Target{ID: "duelist", Size: pack.Size{Width: 16, Height: 16}, References: []pack.Reference{{Path: "ref.png", Required: true}}}

	_, err := generate.Run(context.Background(), []targets.Target{target}, provider.Fake{ReferenceSupport: false}, generate.Options{OutputDir: t.TempDir(), RunID: "run"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "provider does not support references")
}

func TestGenerateDropsOptionalReferencesWhenProviderDoesNotSupportReferences(t *testing.T) {
	target := targets.Target{ID: "duelist", Size: pack.Size{Width: 16, Height: 16}, References: []pack.Reference{{Path: "style.png", Description: "Optional style reference."}}}
	gen := noReferenceProvider{}

	result, err := generate.Run(context.Background(), []targets.Target{target}, gen, generate.Options{OutputDir: t.TempDir(), RunID: "run"})

	require.NoError(t, err)
	require.Equal(t, 1, result.Generated)
}

func TestGenerateSkipsAcceptedTargetsWithoutForce(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "grass"}

	first, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	require.Equal(t, 1, first.Generated)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	manifest.Targets["grass"].Status = generate.StatusAccepted
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	second, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})

	require.NoError(t, err)
	require.Equal(t, 0, second.Generated)
	require.Equal(t, 1, second.Skipped)
}

func TestGenerateWithForcePreservesPreviousReviewHistory(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "grass"}
	_, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	state := manifest.Targets["grass"]
	state.Status = generate.StatusAccepted
	state.Review = &generate.ReviewRecord{Status: generate.StatusAccepted, Reason: "looks good", ReviewedAt: "2026-07-03T12:00:00Z"}
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	result, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Force: true})

	require.NoError(t, err)
	require.Equal(t, 1, result.Generated)
	updated, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Nil(t, updated.Targets["grass"].Review)
	require.Len(t, updated.Targets["grass"].ReviewHistory, 1)
	require.Equal(t, "looks good", updated.Targets["grass"].ReviewHistory[0].Reason)
}

type noReferenceProvider struct{}

func (noReferenceProvider) SupportsReferences() bool { return false }

func (noReferenceProvider) Generate(_ context.Context, req provider.Request) (provider.Result, error) {
	if len(req.References) > 0 {
		return provider.Result{}, os.ErrInvalid
	}
	return provider.Fake{ReferenceSupport: false}.Generate(context.Background(), req)
}
