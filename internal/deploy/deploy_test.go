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
	deployDir := filepath.Join(dir, p.DeployDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}, Status: generate.StatusAccepted})
	require.NoError(t, err)

	plan, err := deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir})

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
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
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

func TestDeployFailsWhenScopeHasNoAcceptedTargets(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})
	require.NoError(t, err)

	_, err = deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: filepath.Join(dir, p.DeployDir), Filter: targets.Filter{Object: "grass"}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "no accepted")
	require.NoFileExists(t, filepath.Join(dir, p.DeployDir, "terrain/grass.png"))
}

func TestDeployBlocksWhenProductionChangedAfterGeneration(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	destination := filepath.Join(deployDir, "terrain", "grass.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
	require.NoError(t, os.WriteFile(destination, testkit.PNGWithMargin(t, 16, 16, 2), 0o644))
	filter := targets.Filter{Object: "grass"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(destination, testkit.PNGWithMargin(t, 16, 16, 4), 0o644))

	plan, err := deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: filter})

	require.ErrorContains(t, err, "deployment blocked")
	require.Empty(t, plan.Replace)
	require.Len(t, plan.Unchanged, 1)
	require.True(t, plan.Unchanged[0].Blocking)
	require.Contains(t, plan.Unchanged[0].Reason, "changed after generation")
}

func TestAlreadyDeployedTargetCannotOverwriteLaterManualEdit(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	filter := targets.Filter{Object: "grass"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})
	require.NoError(t, err)
	_, err = deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: filter})
	require.NoError(t, err)
	destination := filepath.Join(deployDir, "terrain", "grass.png")
	manual := testkit.PNGWithMargin(t, 16, 16, 4)
	require.NoError(t, os.WriteFile(destination, manual, 0o644))

	_, err = deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: filter})

	require.ErrorContains(t, err, "no accepted")
	current, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	require.Equal(t, manual, current)
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

func TestE2EDeployPlanBlocksWholeAnimatedRowWhenOneFrameIsNotAccepted(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	require.NoError(t, generateDeployableRow(t, all, outputDir, filter))
	_, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	manifest.Targets["blood-duelist__attack__direction-right__contact"].Status = generate.StatusRejected
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	plan, err := deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: filepath.Join(dir, p.DeployDir), Filter: targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}, Frame: "00"}})

	require.ErrorContains(t, err, "no accepted")
	require.Empty(t, plan.Replace)
	require.Len(t, plan.Unchanged, 2)
	require.Contains(t, plan.Unchanged[0].Reason, "row blocked")
}

func TestE2EDeployStagesCompleteAnimatedRowBeforeReplacingAnyExistingFile(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	old := map[string][]byte{}
	for index, frame := range []string{"00", "contact"} {
		path := filepath.Join(deployDir, "units", "blood-duelist__attack__right__"+frame+".png")
		old[frame] = testkit.PNGWithMargin(t, 16, 16, index+2)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, old[frame], 0o644))
	}
	require.NoError(t, generateDeployableRow(t, all, outputDir, filter))
	_, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	missing := manifest.Targets["blood-duelist__attack__direction-right__contact"].NormalizedPath
	require.NoError(t, os.Remove(missing))
	_, err = deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: filter})

	require.Error(t, err)
	for _, frame := range []string{"00", "contact"} {
		data, readErr := os.ReadFile(filepath.Join(deployDir, "units", "blood-duelist__attack__right__"+frame+".png"))
		require.NoError(t, readErr)
		require.Equal(t, old[frame], data)
	}
}

func TestDeployPlanBlocksRowWhenFramesDoNotShareSelectedRowLineage(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
	require.NoError(t, generateDeployableRow(t, all, outputDir, filter))
	_, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	manifest.Targets["blood-duelist__attack__direction-right__contact"].RowLineage = "different-row-attempt"
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	plan, err := deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: filepath.Join(dir, p.DeployDir), Filter: filter})

	require.ErrorContains(t, err, "deployment blocked")
	require.Empty(t, plan.Replace)
	require.Len(t, plan.Unchanged, 2)
	require.Contains(t, plan.Unchanged[0].Reason, "animation row lineage mismatch")

	contact := manifest.Targets["blood-duelist__attack__direction-right__contact"]
	ready := manifest.Targets["blood-duelist__attack__direction-right__00"]
	contact.RowLineage = ready.RowLineage
	manifest.Intermediates[ready.AnimationRowID].Status = generate.StatusPending
	require.NoError(t, generate.Save(outputDir, "run", manifest))
	plan, err = deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: filepath.Join(dir, p.DeployDir), Filter: filter})
	require.ErrorContains(t, err, "deployment blocked")
	require.Empty(t, plan.Replace)
	require.Contains(t, plan.Unchanged[0].Reason, "animation row is no longer the selected lineage")
}

func generateDeployableRow(t *testing.T, all []targets.Target, outputDir string, filter targets.Filter) error {
	t.Helper()
	deployDir := filepath.Join(filepath.Dir(outputDir), "deploy")
	if _, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: filter}); err != nil {
		return err
	}
	if err := generate.SelectSeedCandidate(all, outputDir, "run", "blood-duelist", "01", generate.StatusAccepted, "Approved in deploy test."); err != nil {
		return err
	}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: filter})
	return err
}
