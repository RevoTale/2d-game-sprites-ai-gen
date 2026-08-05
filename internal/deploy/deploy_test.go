package deploy_test

import (
	"context"
	"image"
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

func TestStaticDeploymentRemainsTargetAtomic(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	filter := targets.Filter{Object: "grass"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})
	require.NoError(t, err)

	plan, err := deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: filter})

	require.NoError(t, err)
	require.Len(t, plan.Replace, 1)
	require.FileExists(t, filepath.Join(deployDir, "terrain", "grass.png"))
}

func TestStaticSetDeploymentPlanContainsCompleteSet(t *testing.T) {
	_, all, outputDir, deployDir := generatedAcceptedStaticSet(t)

	plan, err := deploy.BuildPlan(all, deploy.Options{
		OutputDir: outputDir,
		RunID:     "run",
		DeployDir: deployDir,
		Filter:    targets.Filter{Object: "fortification"},
	})

	require.NoError(t, err)
	require.Len(t, plan.Replace, 2)
	require.Empty(t, plan.Unchanged)
	for _, item := range plan.Replace {
		require.Equal(t, "static-set:fortification", item.GroupID)
	}
}

func TestStaleStaticSetPartBlocksCompleteSetBeforeWrites(t *testing.T) {
	_, all, outputDir, deployDir := generatedAcceptedStaticSet(t)
	destination := filepath.Join(deployDir, "terrain", "fortification", "collapse-left.png")
	require.NoError(t, os.WriteFile(destination, testkit.PNGWithMargin(t, 80, 64, 8), 0o644))

	plan, err := deploy.BuildPlan(all, deploy.Options{
		OutputDir: outputDir,
		RunID:     "run",
		DeployDir: deployDir,
		Filter:    targets.Filter{Object: "fortification"},
	})

	require.ErrorContains(t, err, "deployment blocked")
	require.Empty(t, plan.Replace)
	require.Len(t, plan.Unchanged, 2)
	for _, item := range plan.Unchanged {
		require.True(t, item.Blocking)
		require.Equal(t, "static-set:fortification", item.GroupID)
	}
}

func TestUnitDeployDryPlanContainsAllFrames(t *testing.T) {
	dir, all, outputDir, deployDir := generatedAcceptedUnit(t)

	plan, err := deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: targets.Filter{Object: "relic-knight"}})

	require.NoError(t, err)
	require.Len(t, plan.Replace, 24)
	require.Empty(t, plan.Unchanged)
	require.DirExists(t, dir)
}

func TestUnitDeploymentIsAtomicAndMarksAggregateDeployed(t *testing.T) {
	_, all, outputDir, deployDir := generatedAcceptedUnit(t)

	_, err := deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: targets.Filter{Object: "relic-knight"}})

	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusDeployed, manifest.Units["unit:relic-knight"].Status)
	for _, targetID := range manifest.Units["unit:relic-knight"].TargetIDs {
		require.Equal(t, generate.StatusDeployed, manifest.Targets[targetID].Status)
		require.FileExists(t, manifest.Targets[targetID].DeployPath)
	}
}

func TestMissingFrameBlocksWholeUnitBeforeWrites(t *testing.T) {
	_, all, outputDir, deployDir := generatedAcceptedUnit(t)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	productionBefore := map[string][]byte{}
	for _, targetID := range manifest.Units["unit:relic-knight"].TargetIDs {
		path := manifest.Targets[targetID].Production.Path
		productionBefore[path], err = os.ReadFile(path)
		require.NoError(t, err)
	}
	missing := manifest.Targets[manifest.Units["unit:relic-knight"].TargetIDs[7]].NormalizedPath
	require.NoError(t, os.Remove(missing))

	plan, err := deploy.Execute(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: targets.Filter{Object: "relic-knight"}})

	require.Error(t, err)
	require.Empty(t, plan.Replace)
	for path, before := range productionBefore {
		after, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, before, after)
	}
}

func TestWrongSizedNormalizedFrameBlocksWholeUnit(t *testing.T) {
	_, all, outputDir, deployDir := generatedAcceptedUnit(t)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	targetID := manifest.Units["unit:relic-knight"].TargetIDs[7]
	require.NoError(t, os.WriteFile(manifest.Targets[targetID].NormalizedPath, testkit.PNGWithMargin(t, 160, 160, 20), 0o644))

	plan, err := deploy.BuildPlan(all, deploy.Options{
		OutputDir: outputDir,
		RunID:     "run",
		DeployDir: deployDir,
		Filter:    targets.Filter{Object: "relic-knight"},
	})

	require.ErrorContains(t, err, "deployment blocked")
	require.Empty(t, plan.Replace)
	require.Len(t, plan.Unchanged, 24)
	require.Contains(t, plan.Unchanged[0].Reason, "normalized source dimensions")
	require.Contains(t, plan.Unchanged[0].Reason, image.Pt(384, 384).String())
}

func TestStaleProductionHashBlocksWholeUnit(t *testing.T) {
	_, all, outputDir, deployDir := generatedAcceptedUnit(t)
	destination := filepath.Join(deployDir, "units", "relic-knight__walk__down__00.png")
	require.NoError(t, os.WriteFile(destination, testkit.PNGWithMargin(t, 320, 320, 48), 0o644))

	plan, err := deploy.BuildPlan(all, deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: targets.Filter{Object: "relic-knight"}})

	require.ErrorContains(t, err, "deployment blocked")
	require.Empty(t, plan.Replace)
	require.Len(t, plan.Unchanged, 24)
	require.Contains(t, plan.Unchanged[0].Reason, "changed after generation")
}

func TestDeployedRunCannotOverwritePainterEdit(t *testing.T) {
	_, all, outputDir, deployDir := generatedAcceptedUnit(t)
	opts := deploy.Options{OutputDir: outputDir, RunID: "run", DeployDir: deployDir, Filter: targets.Filter{Object: "relic-knight"}}
	_, err := deploy.Execute(all, opts)
	require.NoError(t, err)
	destination := filepath.Join(deployDir, "units", "relic-knight__walk__down__00.png")
	painterEdit := testkit.PNGWithMargin(t, 320, 320, 56)
	require.NoError(t, os.WriteFile(destination, painterEdit, 0o644))

	_, err = deploy.Execute(all, opts)

	require.ErrorContains(t, err, "no accepted")
	current, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	require.Equal(t, painterEdit, current)
}

func generatedAcceptedUnit(t *testing.T) (string, []targets.Target, string, string) {
	t.Helper()
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	filter := targets.Filter{Object: "relic-knight"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Status: generate.StatusAccepted})
	require.NoError(t, err)
	return dir, all, outputDir, deployDir
}

func generatedAcceptedStaticSet(t *testing.T) (string, []targets.Target, string, string) {
	t.Helper()
	dir := testkit.WriteStaticSetPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	for _, target := range all {
		path, err := targets.DeployPath(deployDir, target)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, testkit.PNG(t, target.Size.Width, target.Size.Height), 0o644))
	}
	filter := targets.Filter{Object: "fortification"}
	_, err := generate.Run(context.Background(), all, &testkit.StaticSetProvider{}, generate.Options{
		OutputDir: outputDir,
		DeployDir: deployDir,
		RunID:     "run",
		Filter:    filter,
	})
	require.NoError(t, err)
	_, err = review.Apply(all, review.Options{
		OutputDir: outputDir,
		RunID:     "run",
		Filter:    filter,
		Status:    generate.StatusAccepted,
	})
	require.NoError(t, err)
	return dir, all, outputDir, deployDir
}
