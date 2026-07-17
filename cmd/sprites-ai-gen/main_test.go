package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestFormatGenerationProgressExplainsCurrentLongRunningOperation(t *testing.T) {
	tests := []struct {
		name  string
		event generate.ProgressEvent
		want  string
	}{
		{
			name:  "run started",
			event: generate.ProgressEvent{Stage: generate.ProgressRunStarted, RunID: "run"},
			want:  "run: run",
		},
		{
			name:  "target provider request",
			event: generate.ProgressEvent{Stage: generate.ProgressTargetGenerating, TargetID: "relic-knight__walk__direction-right__01", Current: 2, Total: 4},
			want:  "progress: target 2/4 relic-knight__walk__direction-right__01 generating",
		},
		{
			name:  "target ready",
			event: generate.ProgressEvent{Stage: generate.ProgressTargetReady, TargetID: "relic-knight__walk__direction-right__01", Current: 2, Total: 4},
			want:  "progress: target 2/4 relic-knight__walk__direction-right__01 ready",
		},
		{
			name:  "target skipped while resuming",
			event: generate.ProgressEvent{Stage: generate.ProgressTargetSkipped, TargetID: "relic-knight__walk__direction-right__01", Current: 2, Total: 4},
			want:  "progress: target 2/4 relic-knight__walk__direction-right__01 skipped",
		},
		{
			name:  "run completed",
			event: generate.ProgressEvent{Stage: generate.ProgressRunCompleted},
			want:  "progress: run complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, formatGenerationProgress(tt.event))
		})
	}
}

func TestE2EHappyPathInitializesGeneratesReviewsDryRunsAndDeploys(t *testing.T) {
	packDir := t.TempDir()

	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	seedStarterPoseGuides(t, packDir)
	require.NoError(t, run(context.Background(), []string{"validate", "--pack", packDir}))
	require.NoError(t, run(context.Background(), []string{"generate", "--fake", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--stage", "seed", "--candidate", "01", "--status", "accepted"}))
	require.NoError(t, run(context.Background(), []string{"generate", "--fake", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--status", "accepted"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--dry-run", "--pack", packDir, "--run", "run", "--object", "blood-duelist"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--pack", packDir, "--run", "run", "--object", "blood-duelist"}))

	require.FileExists(t, filepath.Join(packDir, "deploy", "units", "blood-duelist__attack__right__contact.png"))
}

func TestE2EStaticTargetRejectsWithoutForceThenRetriesReviewsAndDeploys(t *testing.T) {
	packDir := testkit.WritePack(t)
	base := []string{"--pack", packDir, "--run", "run", "--object", "grass"}

	require.NoError(t, run(context.Background(), append([]string{"validate"}, "--pack", packDir)))
	require.NoError(t, run(context.Background(), append([]string{"generate", "--fake"}, base...)))
	require.NoError(t, run(context.Background(), append([]string{"review"}, append(base, "--status", "rejected", "--reason", "visual QA failed")...)))

	manifest, err := generate.Load(filepath.Join(packDir, "output"), "run")
	require.NoError(t, err)
	require.Len(t, manifest.Targets["grass"].Attempts, 1)

	require.NoError(t, run(context.Background(), append([]string{"generate", "--fake"}, base...)))
	manifest, err = generate.Load(filepath.Join(packDir, "output"), "run")
	require.NoError(t, err)
	require.Len(t, manifest.Targets["grass"].Attempts, 1)

	require.NoError(t, run(context.Background(), append([]string{"generate", "--fake", "--force"}, base...)))
	manifest, err = generate.Load(filepath.Join(packDir, "output"), "run")
	require.NoError(t, err)
	require.Len(t, manifest.Targets["grass"].Attempts, 2)

	require.NoError(t, run(context.Background(), append([]string{"review"}, append(base, "--status", "accepted")...)))
	require.NoError(t, run(context.Background(), append([]string{"deploy", "--dry-run"}, base...)))
	require.NoError(t, run(context.Background(), append([]string{"deploy"}, base...)))
	require.FileExists(t, filepath.Join(packDir, "deploy", "terrain", "grass.png"))
}

func TestE2EFrameSelectionReviewsAndDeploysTheCompleteRow(t *testing.T) {
	packDir := t.TempDir()

	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	seedStarterPoseGuides(t, packDir)
	require.NoError(t, run(context.Background(), []string{"generate", "--fake", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--frame", "contact"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--stage", "seed", "--candidate", "01", "--status", "accepted"}))
	require.NoError(t, run(context.Background(), []string{"generate", "--fake", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--variant", "direction=right", "--animation", "attack", "--frame", "contact"}))
	require.NoError(t, run(context.Background(), []string{"review", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--animation", "attack", "--frame", "contact", "--status", "accepted"}))

	require.NoError(t, run(context.Background(), []string{"deploy", "--dry-run", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--animation", "attack", "--frame", "contact"}))
	require.NoError(t, run(context.Background(), []string{"deploy", "--pack", packDir, "--run", "run", "--object", "blood-duelist", "--animation", "attack", "--frame", "contact"}))
	for _, frame := range []string{"00", "01", "contact", "03"} {
		require.FileExists(t, filepath.Join(packDir, "deploy", "units", "blood-duelist__attack__right__"+frame+".png"))
	}
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

func TestRemovedCommandsReturnMigrationErrors(t *testing.T) {
	tests := []struct {
		command     string
		replacement string
	}{
		{command: "sheet", replacement: "generated automatically"},
		{command: "deploy-plan", replacement: "deploy --dry-run"},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			err := run(context.Background(), []string{test.command})

			require.ErrorContains(t, err, "removed")
			require.ErrorContains(t, err, test.replacement)
		})
	}
}

func TestRemovedFlagsReturnMigrationErrors(t *testing.T) {
	packDir := t.TempDir()
	require.NoError(t, run(context.Background(), []string{"init", "--pack", packDir}))
	tests := []struct {
		name string
		args []string
	}{
		{name: "output", args: []string{"validate", "--pack", packDir, "--output", "draft"}},
		{name: "deploy directory", args: []string{"deploy", "--pack", packDir, "--run", "run", "--deploy-dir", "deploy"}},
		{name: "partial review", args: []string{"review", "--pack", packDir, "--run", "run", "--allow-partial", "--status", "accepted"}},
		{name: "complete deploy", args: []string{"deploy", "--pack", packDir, "--run", "run", "--complete"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := run(context.Background(), test.args)

			require.ErrorContains(t, err, "removed")
		})
	}
}

func TestVariantFlagRejectsMalformedAndDuplicateAxes(t *testing.T) {
	var variants multiFlag
	require.Error(t, variants.Set("direction"))
	require.Error(t, variants.Set("=right"))
	require.Error(t, variants.Set("direction="))
	require.NoError(t, variants.Set(" direction = right "))
	require.Equal(t, multiFlag{"direction=right"}, variants)
	require.ErrorContains(t, variants.Set("direction=up"), "more than once")
}

func TestResolveReferencePathsResolvesTypedConditioningInputs(t *testing.T) {
	packDir := t.TempDir()
	all := []targets.Target{{
		Inputs: []conditioning.Input{{Role: conditioning.RoleIdentity, Path: "references/identity.png"}, {Role: conditioning.RolePose, Path: "references/pose.png"}},
	}}

	resolved := resolveReferencePaths(all, packDir)

	require.Equal(t, filepath.Join(packDir, "references/identity.png"), resolved[0].Inputs[0].Path)
	require.Equal(t, filepath.Join(packDir, "references/pose.png"), resolved[0].Inputs[1].Path)
	require.Equal(t, "references/identity.png", all[0].Inputs[0].Path)
	require.Equal(t, "references/pose.png", all[0].Inputs[1].Path)
}

func seedStarterPoseGuides(t *testing.T, packDir string) {
	t.Helper()
	for _, frame := range []string{"00", "01", "contact", "03"} {
		path := filepath.Join(packDir, "deploy", "units", "blood-duelist__attack__right__"+frame+".png")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, testkit.PNGWithMargin(t, 160, 160, 20), 0o644))
	}
}
