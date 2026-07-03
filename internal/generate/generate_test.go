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
