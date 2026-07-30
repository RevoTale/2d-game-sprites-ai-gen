package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestE2EV10InitializesGeneratesReviewsDryRunsAndDeploysUnit(t *testing.T) {
	packDir := t.TempDir()

	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"validate", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"generate", "--fake", "--pack", packDir, "--run", "run", "--object", "blood-duelist"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--status", "accepted"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--dry-run", "--pack", packDir, "--run", "run", "--object", "blood-duelist"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--pack", packDir, "--run", "run", "--object", "blood-duelist"}))

	for _, frame := range []string{"00", "01", "contact", "03"} {
		require.FileExists(t, filepath.Join(packDir, "deploy", "units", "blood-duelist__attack__right__"+frame+".png"))
	}
}

func TestE2EStaticGenerationReviewAndDeployRemainsSupported(t *testing.T) {
	packDir := testkit.WritePack(t)
	base := []string{"--pack", packDir, "--run", "run", "--object", "grass"}

	require.NoError(t, run(context.Background(), append([]string{"generate", "--fake"}, base...)))
	require.NoError(t, run(context.Background(), append([]string{"review"}, append(base, "--status", "accepted")...)))
	require.NoError(t, run(context.Background(), append([]string{"deploy"}, base...)))
	require.FileExists(t, filepath.Join(packDir, "deploy", "terrain", "grass.png"))
}

func TestRealGenerationRequiresExplicitObjectOrAllBeforeProviderSelection(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	packDir := testkit.WritePack(t)

	err := run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run"})

	require.ErrorContains(t, err, "--object <id>")
	require.NoFileExists(t, generate.ManifestPath(filepath.Join(packDir, "output"), "run"))
}

func TestAllCallCountIncludesOneMasterAndEachAnimationPerUnit(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	_, all := testkit.LoadTargets(t, dir)

	require.Equal(t, 4, plannedProviderCalls(all, targets.Filter{}))
	require.Equal(t, 3, plannedProviderCalls(all, targets.Filter{Object: "relic-knight"}))
}

func TestAnimatedGenerationRejectsAnimationSelector(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)

	err := run(context.Background(), []string{
		"generate", "--fake", "--pack", dir, "--run", "run",
		"--object", "relic-knight", "--animation", "walk",
	})

	require.ErrorContains(t, err, "complete-unit only in V10")
	require.NoFileExists(t, generate.ManifestPath(filepath.Join(dir, "output"), "run"))
}

func TestAnimatedGenerationRejectsForce(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)

	err := run(context.Background(), []string{
		"generate", "--fake", "--pack", dir, "--run", "run",
		"--object", "relic-knight", "--force",
	})

	require.ErrorContains(t, err, "complete-unit only in V10")
	require.NoFileExists(t, generate.ManifestPath(filepath.Join(dir, "output"), "run"))
}

func TestAnimatedPartialSelectorsReturnV10MigrationErrors(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	tests := [][]string{
		{"generate", "--fake", "--pack", dir, "--run", "run", "--object", "relic-knight", "--variant", "direction=right"},
		{"generate", "--fake", "--pack", dir, "--run", "run", "--object", "relic-knight", "--frame", "00"},
		{"review", "--pack", dir, "--run", "run", "--object", "relic-knight", "--animation", "walk", "--status", "accepted"},
		{"deploy", "--pack", dir, "--run", "run", "--object", "relic-knight", "--animation", "walk"},
	}
	for _, args := range tests {
		err := run(context.Background(), args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "V10")
	}
}

func TestRemovedStageAndCandidateFlagsReturnMigrationErrors(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	for _, args := range [][]string{
		{"review", "--pack", dir, "--run", "run", "--object", "relic-knight", "--stage", "seed", "--status", "accepted"},
		{"review", "--pack", dir, "--run", "run", "--object", "relic-knight", "--candidate", "01", "--status", "accepted"},
	} {
		err := run(context.Background(), args)
		require.ErrorContains(t, err, "removed")
	}
}

func TestGenerateLoadsPackEnvironmentButNeverAllowsImplicitFake(t *testing.T) {
	t.Setenv("SPRITES_AI_GEN_PROVIDER", "")
	t.Setenv("OPENAI_API_KEY", "")
	dir := testkit.WritePack(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("SPRITES_AI_GEN_PROVIDER=fake\n"), 0o600))

	err := run(context.Background(), []string{"generate", "--pack", dir, "--run", "run", "--object", "grass"})

	require.ErrorContains(t, err, "--fake")
}
