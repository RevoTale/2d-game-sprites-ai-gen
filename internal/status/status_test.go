package status

import (
	"bytes"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/stretchr/testify/require"
)

func TestSeedStatusNamesCandidateEligibilityAndUsesPreferredCandidateInNextCommand(t *testing.T) {
	state := &generate.IntermediateState{
		Kind:     directionSeedBoardKind,
		ObjectID: "relic-knight",
		Status:   generate.StatusAwaitingReview,
		Attempts: []generate.Attempt{{
			SelectedCandidate: "02",
			Candidates: []generate.Candidate{
				{ID: "01", HardRejections: []string{"board_trailing_cell_occupied"}},
				{ID: "02"},
				{ID: "03"},
			},
		}},
	}

	require.Equal(t, []string{
		"eligible_candidates: 02, 03",
		"invalid_candidates: 01",
		"mechanically_preferred_candidate: 02",
	}, candidateStatusLines(state))
	require.Equal(t,
		"sprites-ai-gen review --run 2026-07-15-m1354 --object relic-knight --candidate 02 --status accepted",
		nextIntermediateAction(&generate.Manifest{RunID: "2026-07-15-m1354"}, state, nil, targets.Filter{}),
	)
}

func TestSeedStatusWithNoEligibleCandidateSuggestsExplicitRejection(t *testing.T) {
	state := &generate.IntermediateState{
		Kind:     directionSeedBoardKind,
		ObjectID: "relic-knight",
		Status:   generate.StatusAwaitingReview,
		Attempts: []generate.Attempt{{Candidates: []generate.Candidate{
			{ID: "01", HardRejections: []string{"invalid"}},
			{ID: "02", HardRejections: []string{"invalid"}},
			{ID: "03", HardRejections: []string{"invalid"}},
		}}},
	}

	action := nextIntermediateAction(&generate.Manifest{RunID: "run"}, state, nil, targets.Filter{})

	require.Equal(t, "sprites-ai-gen review --run run --object relic-knight --stage seed --status rejected --reason no-mechanically-valid-seed-candidate", action)
	require.NotContains(t, action, "<candidate-id>")
}

func TestStatusIncludesObjectSeedGateForAnySelectedAnimation(t *testing.T) {
	all := []targets.Target{
		{
			ID:          "relic-knight__walk__direction-right__00",
			ObjectID:    "relic-knight",
			AnimationID: "walk",
			FrameID:     "00",
			Variants:    []targets.VariantSelection{{AxisID: "direction", ValueID: "right"}},
		},
		{
			ID:          "relic-knight__attack__direction-right__00",
			ObjectID:    "relic-knight",
			AnimationID: "attack",
			FrameID:     "00",
			Variants:    []targets.VariantSelection{{AxisID: "direction", ValueID: "right"}},
		},
	}
	manifest := &generate.Manifest{
		RunID: "run",
		Targets: map[string]*generate.TargetState{
			all[0].ID: {ID: all[0].ID, Status: generate.StatusPending},
			all[1].ID: {ID: all[1].ID, Status: generate.StatusPending},
		},
		Intermediates: map[string]*generate.IntermediateState{
			"direction-seed-board:relic-knight": {
				ID:        "direction-seed-board:relic-knight",
				Kind:      directionSeedBoardKind,
				ObjectID:  "relic-knight",
				TargetIDs: []string{all[0].ID},
				Status:    generate.StatusAwaitingReview,
				Attempts: []generate.Attempt{{
					SelectedCandidate: "02",
					Candidates:        []generate.Candidate{{ID: "01", HardRejections: []string{"invalid"}}, {ID: "02"}},
				}},
			},
		},
	}

	var output bytes.Buffer
	Print(&output, manifest, all, targets.Filter{Object: "relic-knight", Animation: "attack"})

	require.Contains(t, output.String(), "state: awaiting_review\n")
	require.Contains(t, output.String(), "scope_intermediates: 1\n")
	require.Contains(t, output.String(), "intermediate_awaiting_review: 1\n")
	require.Contains(t, output.String(), "scope: direction-seed-board:relic-knight state=awaiting_review\n")
	require.Contains(t, output.String(), "next: sprites-ai-gen review --run run --object relic-knight --candidate 02 --status accepted\n")
	require.NotContains(t, output.String(), "next: none")
}

func TestStatusCommandsPreserveTheCompleteSelectedRow(t *testing.T) {
	all := statusRowTargets()
	filter := targets.Filter{Object: "relic-knight", Animation: "walk", Variants: map[string]string{"direction": "right"}}
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "review", status: generate.StatusAwaitingReview, want: "next: sprites-ai-gen review --run run --object relic-knight --animation walk --variant direction=right --status accepted\n"},
		{name: "forced retry", status: generate.StatusRejected, want: "next: sprites-ai-gen generate --run run --object relic-knight --animation walk --variant direction=right --force\n"},
		{name: "deploy", status: generate.StatusAccepted, want: "next: sprites-ai-gen deploy --run run --object relic-knight --animation walk --variant direction=right\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := statusManifest(all, test.status, &generate.IntermediateState{
				ID:          "animation-row:relic-knight__direction-right__animation-walk",
				Kind:        "animation-row",
				ObjectID:    "relic-knight",
				AnimationID: "walk",
				VariantKey:  "direction-right",
				TargetIDs:   targetIDsForStatus(all),
				Status:      test.status,
			})
			var output bytes.Buffer

			Print(&output, manifest, all, filter)

			require.Contains(t, output.String(), test.want)
		})
	}
}

func TestStatusPrintsEveryRecordedReviewArtifact(t *testing.T) {
	all := statusRowTargets()
	manifest := statusManifest(all, generate.StatusAwaitingReview, &generate.IntermediateState{
		ID:          "animation-row:relic-knight__direction-right__animation-walk",
		Kind:        "animation-row",
		ObjectID:    "relic-knight",
		AnimationID: "walk",
		VariantKey:  "direction-right",
		TargetIDs:   targetIDsForStatus(all),
		Status:      generate.StatusAwaitingReview,
		Artifacts: generate.ReviewArtifacts{
			PromptPath:         "review/prompt.md",
			QAPath:             "review/qa.md",
			CandidateSheetPath: "review/candidates.png",
			ContactSheetPath:   "review/contact-sheet.png",
			AnimationGIFPath:   "review/animation.gif",
			FramePaths:         []string{"review/frames/00.png", "review/frames/01.png"},
		},
	})
	var output bytes.Buffer

	require.NoError(t, Print(&output, manifest, all, targets.Filter{Object: "relic-knight", Animation: "walk", Variants: map[string]string{"direction": "right"}}))

	for _, path := range []string{
		"review/prompt.md",
		"review/qa.md",
		"review/candidates.png",
		"review/contact-sheet.png",
		"review/animation.gif",
		"review/frames/00.png",
		"review/frames/01.png",
	} {
		require.Contains(t, output.String(), "artifact: "+path+"\n")
	}
}

func TestAcceptedSeedStatusNamesAnExecutablePendingRowGeneration(t *testing.T) {
	all := statusRowTargets()
	manifest := statusManifest(all, generate.StatusPending, &generate.IntermediateState{
		ID:        "direction-seed-board:relic-knight",
		Kind:      directionSeedBoardKind,
		ObjectID:  "relic-knight",
		TargetIDs: []string{all[0].ID},
		Status:    generate.StatusAccepted,
	})
	var output bytes.Buffer

	Print(&output, manifest, all, targets.Filter{Object: "relic-knight", Animation: "walk", Variants: map[string]string{"direction": "right"}})

	require.Contains(t, output.String(), "next: sprites-ai-gen generate --run run --object relic-knight --animation walk --variant direction=right\n")
	require.NotContains(t, output.String(), "<same selectors>")
}

func TestPendingStaticStatusDoesNotPrintAnEmptyNextAction(t *testing.T) {
	target := targets.Target{ID: "terrain-rock__material-basalt", ObjectID: "terrain-rock", Variants: []targets.VariantSelection{{AxisID: "material", ValueID: "basalt"}}}
	manifest := &generate.Manifest{
		RunID:         "run",
		Targets:       map[string]*generate.TargetState{target.ID: {ID: target.ID, Status: generate.StatusPending}},
		Intermediates: map[string]*generate.IntermediateState{},
	}
	var output bytes.Buffer

	require.NoError(t, Print(&output, manifest, []targets.Target{target}, targets.Filter{}))

	require.NotContains(t, output.String(), "next:")
}

func TestStatusRejectsAnEmptySelectorMatch(t *testing.T) {
	target := targets.Target{ID: "terrain-rock", ObjectID: "terrain-rock"}
	manifest := &generate.Manifest{
		RunID:         "run",
		Targets:       map[string]*generate.TargetState{target.ID: {ID: target.ID, Status: generate.StatusPending}},
		Intermediates: map[string]*generate.IntermediateState{},
	}
	var output bytes.Buffer

	err := Print(&output, manifest, []targets.Target{target}, targets.Filter{Object: "missing"})

	require.ErrorContains(t, err, "no targets matched selector")
	require.Empty(t, output.String())
}

func statusRowTargets() []targets.Target {
	frames := []string{"00", "01", "02", "03"}
	result := make([]targets.Target, len(frames))
	for index, frame := range frames {
		result[index] = targets.Target{
			ID:          "relic-knight__walk__direction-right__" + frame,
			ObjectID:    "relic-knight",
			AnimationID: "walk",
			FrameID:     frame,
			FrameIndex:  index,
			Variants:    []targets.VariantSelection{{AxisID: "direction", ValueID: "right"}},
		}
	}
	return result
}

func statusManifest(all []targets.Target, targetStatus string, intermediate *generate.IntermediateState) *generate.Manifest {
	states := make(map[string]*generate.TargetState, len(all))
	for _, target := range all {
		states[target.ID] = &generate.TargetState{ID: target.ID, Status: targetStatus}
	}
	return &generate.Manifest{
		RunID:         "run",
		Targets:       states,
		Intermediates: map[string]*generate.IntermediateState{intermediate.ID: intermediate},
	}
}

func targetIDsForStatus(all []targets.Target) []string {
	ids := make([]string, len(all))
	for index, target := range all {
		ids[index] = target.ID
	}
	return ids
}
