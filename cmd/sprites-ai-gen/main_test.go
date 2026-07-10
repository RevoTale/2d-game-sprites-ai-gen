package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2EHappyPathInitializesGeneratesReviewsPlansAndDeploys(t *testing.T) {
	packDir := t.TempDir()

	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"validate", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"generate", "--fake", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--frame", "contact"}))
	require.NoError(t, run(context.Background(), []string{"sheet", "--pack", packDir, "--run", "run", "--object", "blood-duelist"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--status", "accepted", "--allow-partial"}))
	require.NoError(t, run(context.Background(), []string{"deploy-plan", "--pack", packDir, "--run", "run"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--pack", packDir, "--run", "run"}))

	require.FileExists(t, filepath.Join(packDir, "deploy", "units", "blood-duelist__attack__right__contact.png"))
}

func TestE2ECompleteDeployFailsWhenGeneratedPackIsOnlyPartiallyAccepted(t *testing.T) {
	packDir := t.TempDir()

	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"generate", "--fake", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--frame", "contact"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--animation", "attack", "--frame", "contact", "--status", "accepted"}))

	err := run(context.Background(), []string{"deploy", "--complete", "--pack", packDir, "--run", "run"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "complete deploy blocked")
}

func TestGenerateRejectsUnknownProviderName(t *testing.T) {
	packDir := t.TempDir()
	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))

	err := run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--provider", "typo", "--object", "blood-duelist"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown provider")
}

func TestGenerateRejectsProviderFakeBecauseFakeRequiresFlag(t *testing.T) {
	packDir := t.TempDir()
	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))

	err := run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--provider", "fake", "--object", "blood-duelist"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "--fake")
}

func TestGenerateRejectsFakeEnvironmentBecauseFakeRequiresFlag(t *testing.T) {
	t.Setenv("SPRITES_AI_GEN_PROVIDER", "fake")
	t.Setenv("OPENAI_API_KEY", "")
	packDir := t.TempDir()
	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))

	err := run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--frame", "contact"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "--fake")
}

func TestGenerateRequiresProviderEnvironmentOrFakeFlag(t *testing.T) {
	t.Setenv("SPRITES_AI_GEN_PROVIDER", "")
	t.Setenv("OPENAI_API_KEY", "")
	packDir := t.TempDir()
	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))

	err := run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--object", "blood-duelist"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "OPENAI_API_KEY")
	require.Contains(t, err.Error(), "--fake")
}

func TestGenerateRejectsFakeFlagWithExplicitProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test")
	packDir := t.TempDir()
	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))

	err := run(context.Background(), []string{"generate", "--fake", "--provider", "openai", "--pack", packDir, "--run", "run", "--object", "blood-duelist"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "--fake cannot be used with --provider")
}

func TestGenerateLoadsPackEnvBeforeProviderDetection(t *testing.T) {
	t.Setenv("SPRITES_AI_GEN_PROVIDER", "")
	t.Setenv("OPENAI_API_KEY", "")
	packDir := t.TempDir()
	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	require.NoError(t, os.WriteFile(filepath.Join(packDir, ".env"), []byte("SPRITES_AI_GEN_PROVIDER=fake\n"), 0o600))

	err := run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--object", "blood-duelist"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "--fake")
}
