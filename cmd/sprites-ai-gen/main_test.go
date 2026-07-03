package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2EHappyPathInitializesGeneratesReviewsPlansAndDeploys(t *testing.T) {
	packDir := t.TempDir()

	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"validate", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--frame", "contact"}))
	require.NoError(t, run(context.Background(), []string{"sheet", "--pack", packDir, "--run", "run", "--object", "blood-duelist"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--status", "accepted", "--allow-partial"}))
	require.NoError(t, run(context.Background(), []string{"deploy-plan", "--pack", packDir, "--run", "run"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--pack", packDir, "--run", "run"}))

	require.FileExists(t, filepath.Join(packDir, "deploy", "units", "blood-duelist__attack__right__contact.png"))
}

func TestE2ECompleteDeployFailsWhenGeneratedPackIsOnlyPartiallyAccepted(t *testing.T) {
	packDir := t.TempDir()

	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"generate", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--frame", "contact"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--animation", "attack", "--frame", "contact", "--status", "accepted"}))

	err := run(context.Background(), []string{"deploy", "--complete", "--pack", packDir, "--run", "run"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "complete deploy blocked")
}
