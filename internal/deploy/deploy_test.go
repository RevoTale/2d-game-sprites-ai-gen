package deploy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/deploy"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/review"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestDeployPlanWithoutObjectReportsWholePackImpact(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusAccepted})
	require.NoError(t, err)

	plan, err := deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: filepath.Join(dir, p.DeployDir)})

	require.NoError(t, err)
	require.Len(t, plan.Replace, 1)
	require.NotEmpty(t, plan.Unchanged)
}

func TestPartialDeployLeavesUnacceptedExistingFilesUnchanged(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	existingPath := filepath.Join(deployDir, "units/blood-duelist__attack__right__00.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(existingPath), 0o755))
	require.NoError(t, os.WriteFile(existingPath, []byte("old"), 0o644))
	_, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusAccepted})
	require.NoError(t, err)

	_, err = deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir})

	require.NoError(t, err)
	unchanged, err := os.ReadFile(existingPath)
	require.NoError(t, err)
	require.Equal(t, []byte("old"), unchanged)
	require.FileExists(t, filepath.Join(deployDir, "terrain/grass.png"))
}

func TestCompleteDeployFailsWhenScopeHasUnacceptedTargets(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{ReferenceSupport: true}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusAccepted})
	require.NoError(t, err)

	_, err = deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: filepath.Join(dir, p.DeployDir), Complete: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "complete deploy blocked")
}

func TestRenderPathSupportsTargetPlaceholderForStaticVariants(t *testing.T) {
	path, err := deploy.RenderPath(t.TempDir(), targets.Target{
		ID:             "grass__season-winter",
		ObjectID:       "grass",
		DeployTemplate: "sprites/{target}.png",
	})

	require.NoError(t, err)
	require.Equal(t, "grass__season-winter.png", filepath.Base(path))
	require.Equal(t, "sprites", filepath.Base(filepath.Dir(path)))
}
