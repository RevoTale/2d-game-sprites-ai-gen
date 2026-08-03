package status_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/status"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestStatusPrintsV11UnitStagesArtifactsAndReviewAction(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{
		OutputDir: outputDir, DeployDir: filepath.Join(dir, p.DeployDir), RunID: "run",
		Filter: targets.Filter{Object: "relic-knight"},
	})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	var output bytes.Buffer

	require.NoError(t, status.Print(&output, manifest, all, targets.Filter{Object: "relic-knight"}))

	require.Contains(t, output.String(), "unit: relic-knight state=awaiting_review")
	require.Contains(t, output.String(), "provider_calls_remaining: 0")
	require.Contains(t, output.String(), "master: character-master:relic-knight state=ready")
	require.Contains(t, output.String(), "animation_board: animation-board:relic-knight:walk state=ready")
	require.Contains(t, output.String(), "animation_board: animation-board:relic-knight:attack state=ready")
	require.Contains(t, output.String(), "current_candidate: 001/01")
	require.Contains(t, output.String(), "complete-unit.png")
	require.Contains(t, output.String(), "ownership.png")
	require.Contains(t, output.String(), "recovered-poses.png")
	require.Contains(t, output.String(), "next: sprites-ai-gen review --run run --object relic-knight --status accepted")
	require.NotContains(t, output.String(), "--stage")
	require.NotContains(t, output.String(), "--variant")
	require.NotContains(t, output.String(), "--frame")
}

func TestRejectedUnitStatusRequiresFreshCompleteRun(t *testing.T) {
	target := targets.Target{ID: "knight__walk__direction-right__00", ObjectID: "knight", AnimationID: "walk"}
	manifest := &generate.Manifest{
		RunID:   "run",
		Targets: map[string]*generate.TargetState{target.ID: {ID: target.ID, Status: generate.StatusRejected}},
		Intermediates: map[string]*generate.IntermediateState{
			"character-master:knight": {ID: "character-master:knight", Kind: "character-master", Status: generate.StatusReady},
			"animation-board:knight:walk": {
				ID: "animation-board:knight:walk", Kind: "animation-board", AnimationID: "walk",
				Status: generate.StatusRejected, HardRejections: []string{"foreground touches canvas edge"},
			},
		},
		Units: map[string]*generate.UnitState{
			"unit:knight": {
				ID: "unit:knight", ObjectID: "knight", Status: generate.StatusRejected,
				MasterID: "character-master:knight", AnimationBoardIDs: []string{"animation-board:knight:walk"}, TargetIDs: []string{target.ID},
			},
		},
	}
	var output bytes.Buffer

	require.NoError(t, status.Print(&output, manifest, []targets.Target{target}, targets.Filter{Object: "knight"}))

	require.Contains(t, output.String(), "next: sprites-ai-gen generate --run auto --object knight")
	require.Contains(t, output.String(), "blocker: foreground touches canvas edge")
	require.NotContains(t, output.String(), "--animation")
	require.NotContains(t, output.String(), "--force")
}

func TestManualUnitRejectionRequiresFreshCompleteRun(t *testing.T) {
	target := targets.Target{ID: "knight__walk__direction-right__00", ObjectID: "knight", AnimationID: "walk"}
	manifest := &generate.Manifest{
		RunID: "run", Targets: map[string]*generate.TargetState{target.ID: {ID: target.ID, Status: generate.StatusRejected}},
		Intermediates: map[string]*generate.IntermediateState{
			"character-master:knight":     {ID: "character-master:knight", Status: generate.StatusReady},
			"animation-board:knight:walk": {ID: "animation-board:knight:walk", AnimationID: "walk", Status: generate.StatusReady},
		},
		Units: map[string]*generate.UnitState{
			"unit:knight": {
				ID: "unit:knight", ObjectID: "knight", Status: generate.StatusRejected,
				MasterID: "character-master:knight", AnimationBoardIDs: []string{"animation-board:knight:walk"},
				TargetIDs: []string{target.ID}, Review: &generate.ReviewRecord{Status: generate.StatusRejected, Reason: "visual inconsistency"},
			},
		},
	}
	var output bytes.Buffer

	require.NoError(t, status.Print(&output, manifest, []targets.Target{target}, targets.Filter{Object: "knight"}))

	require.Contains(t, output.String(), "next: sprites-ai-gen generate --run auto --object knight")
	require.NotContains(t, output.String(), "--animation")
	require.NotContains(t, output.String(), "--force")
}

func TestStaticStatusRemainsSupported(t *testing.T) {
	target := targets.Target{ID: "terrain-rock", ObjectID: "terrain-rock"}
	manifest := &generate.Manifest{
		RunID: "run",
		Targets: map[string]*generate.TargetState{target.ID: {
			ID: target.ID, Status: generate.StatusAwaitingReview, SourceCandidate: "002/01",
		}},
		Intermediates: map[string]*generate.IntermediateState{},
		Units:         map[string]*generate.UnitState{},
	}
	var output bytes.Buffer

	require.NoError(t, status.Print(&output, manifest, []targets.Target{target}, targets.Filter{}))
	require.Contains(t, output.String(), "provider_calls_remaining: 0")
	require.Contains(t, output.String(), "source_candidate: 002/01")
	require.Contains(t, output.String(), "next: sprites-ai-gen review --run run --object terrain-rock --status accepted")
}
