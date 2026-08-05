package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestE2EV13GeneratesReviewsDryRunsAndDeploysCompleteUnit(t *testing.T) {
	injectFakeProvider(t)
	packDir := testkit.WriteFullUnitPack(t)

	require.NoError(t, run(context.Background(), []string{"validate", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--object", "relic-knight"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "relic-knight", "--status", "accepted"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--dry-run", "--pack", packDir, "--run", "run", "--object", "relic-knight"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--pack", packDir, "--run", "run", "--object", "relic-knight"}))

	require.FileExists(t, filepath.Join(packDir, "deploy", "units", "relic-knight__attack__right__02.png"))
}

func TestE2EStaticGenerationReviewAndDeployRemainsSupported(t *testing.T) {
	injectFakeProvider(t)
	packDir := testkit.WritePack(t)
	base := []string{"--pack", packDir, "--run", "run", "--object", "grass"}

	require.NoError(t, run(context.Background(), append([]string{"generate"}, base...)))
	require.NoError(t, run(context.Background(), append([]string{"review"}, append(base, "--status", "accepted")...)))
	require.NoError(t, run(context.Background(), append([]string{"deploy"}, base...)))
	require.FileExists(t, filepath.Join(packDir, "deploy", "terrain", "grass.png"))
}

func TestCatalogBuildsWithoutSelectingProvider(t *testing.T) {
	packDir := testkit.WritePack(t)
	called := false
	previous := productionProvider
	productionProvider = func(map[string]string) (provider.Provider, error) {
		called = true
		return provider.Fake{}, nil
	}
	t.Cleanup(func() { productionProvider = previous })

	err := run(context.Background(), []string{"catalog", "--pack", packDir})

	require.NoError(t, err)
	require.False(t, called)
	require.FileExists(t, filepath.Join(packDir, "output", "catalog", "index.html"))
}

func TestRealGenerationRequiresExplicitObjectOrAllBeforeProviderSelection(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	packDir := testkit.WritePack(t)

	err := run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run"})

	require.ErrorContains(t, err, "--object <id>")
	require.NoFileExists(t, generate.ManifestPath(filepath.Join(packDir, "output"), "run"))
}

func TestUnknownObjectFailsBeforeProviderSelection(t *testing.T) {
	dir := testkit.WritePack(t)
	called := false
	previous := productionProvider
	productionProvider = func(map[string]string) (provider.Provider, error) {
		called = true
		return provider.Fake{}, nil
	}
	t.Cleanup(func() { productionProvider = previous })

	err := run(context.Background(), []string{
		"generate", "--pack", dir, "--run", "run", "--object", "missing",
	})

	require.ErrorContains(t, err, "no targets matched selector")
	require.False(t, called)
	require.NoFileExists(t, generate.ManifestPath(filepath.Join(dir, "output"), "run"))
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
		"generate", "--pack", dir, "--run", "run",
		"--object", "relic-knight", "--animation", "walk",
	})

	require.ErrorContains(t, err, "V13")
	require.NoFileExists(t, generate.ManifestPath(filepath.Join(dir, "output"), "run"))
}

func TestAnimatedGenerationRejectsForce(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)

	err := run(context.Background(), []string{
		"generate", "--pack", dir, "--run", "run",
		"--object", "relic-knight", "--force",
	})

	require.ErrorContains(t, err, "fresh V13 run")
	require.NoFileExists(t, generate.ManifestPath(filepath.Join(dir, "output"), "run"))
}

func TestAnimatedPartialSelectorsReturnV13MigrationErrors(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	tests := [][]string{
		{"generate", "--pack", dir, "--run", "run", "--object", "relic-knight", "--variant", "direction=right"},
		{"generate", "--pack", dir, "--run", "run", "--object", "relic-knight", "--frame", "00"},
		{"review", "--pack", dir, "--run", "run", "--object", "relic-knight", "--animation", "walk", "--status", "accepted"},
		{"deploy", "--pack", dir, "--run", "run", "--object", "relic-knight", "--animation", "walk"},
	}
	for _, args := range tests {
		err := run(context.Background(), args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "V13")
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

func TestGenerateRequiresOpenAICredentialsWithoutInjectedProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	dir := testkit.WritePack(t)

	err := run(context.Background(), []string{"generate", "--pack", dir, "--run", "run", "--object", "grass"})

	require.ErrorContains(t, err, "OPENAI_API_KEY")
}

func TestE2EStyleGuideBootstrapIsOneReviewableAndDeployableTarget(t *testing.T) {
	injectFakeProvider(t)
	dir := testkit.WritePack(t)
	guidePath := filepath.Join(dir, "references", "style", "compact-dark-fantasy-style-v1.png")
	require.NoError(t, os.Remove(guidePath))

	require.NoError(t, run(context.Background(), []string{
		"generate", "--pack", dir, "--run", "guide-run", "--style-guide",
	}))
	require.NoError(t, run(context.Background(), []string{
		"review", "--pack", dir, "--run", "guide-run", "--style-guide", "--status", "accepted",
	}))
	require.NoError(t, run(context.Background(), []string{
		"deploy", "--pack", dir, "--run", "guide-run", "--style-guide", "--dry-run",
	}))
	require.NoError(t, run(context.Background(), []string{
		"deploy", "--pack", dir, "--run", "guide-run", "--style-guide",
	}))
	require.FileExists(t, guidePath)
}

func TestAssetGenerationFailsBeforeProviderWhenApprovedGuideIsMissing(t *testing.T) {
	called := false
	old := productionProvider
	productionProvider = func(map[string]string) (provider.Provider, error) {
		called = true
		return provider.Fake{}, nil
	}
	t.Cleanup(func() { productionProvider = old })
	dir := testkit.WritePack(t)
	require.NoError(t, os.Remove(filepath.Join(
		dir,
		"references",
		"style",
		"compact-dark-fantasy-style-v1.png",
	)))

	err := run(context.Background(), []string{
		"generate", "--pack", dir, "--run", "run", "--object", "grass",
	})

	require.ErrorContains(t, err, "approved style guide")
	require.False(t, called)
}

func TestExcludeObjectRequiresAllAndRejectsUnknownObjects(t *testing.T) {
	dir := testkit.WritePack(t)

	err := run(context.Background(), []string{
		"generate", "--pack", dir, "--run", "run", "--object", "grass",
		"--exclude-object", "blood-duelist",
	})
	require.ErrorContains(t, err, "valid only with --all")

	err = run(context.Background(), []string{
		"generate", "--pack", dir, "--run", "run", "--all",
		"--exclude-object", "missing",
	})
	require.ErrorContains(t, err, "does not match a configured object")
}

func TestProviderAndPublicFakeFlagsAreRemoved(t *testing.T) {
	dir := testkit.WritePack(t)
	for _, args := range [][]string{
		{"generate", "--pack", dir, "--run", "run", "--object", "grass", "--provider", "openai"},
		{"generate", "--pack", dir, "--run", "run", "--object", "grass", "--fake"},
	} {
		err := run(context.Background(), args)
		require.ErrorContains(t, err, "removed")
	}
}

func injectFakeProvider(t *testing.T) {
	t.Helper()
	old := productionProvider
	productionProvider = func(map[string]string) (provider.Provider, error) {
		return provider.Fake{}, nil
	}
	t.Cleanup(func() { productionProvider = old })
}
