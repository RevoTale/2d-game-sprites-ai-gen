// Package status renders manifest-V5 scope summaries and executable next actions.
package status

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

const directionSeedBoardKind = "direction-seed-board"

// Print writes the current state, review artifacts, and exact next actions for
// the selected target scope.
func Print(writer io.Writer, manifest *generate.Manifest, all []targets.Target, filter targets.Filter) error {
	selected, err := targets.Select(all, filter)
	if err != nil {
		return fmt.Errorf("select status scope: %w", err)
	}
	wanted := make(map[string]bool, len(selected))
	wantedObjects := make(map[string]bool)
	targetCounts := map[string]int{}
	for _, target := range selected {
		wanted[target.ID] = true
		wantedObjects[target.ObjectID] = true
		if state := manifest.Targets[target.ID]; state != nil {
			targetCounts[state.Status]++
		}
	}
	ids := make([]string, 0, len(manifest.Intermediates))
	intermediateCounts := map[string]int{}
	for id, state := range manifest.Intermediates {
		if intermediateInScope(state, wanted, wantedObjects) {
			ids = append(ids, id)
			intermediateCounts[state.Status]++
		}
	}
	sort.Strings(ids)
	fmt.Fprintf(writer, "run_id: %s\nscope_targets: %d\nscope_intermediates: %d\nstate: %s\n", manifest.RunID, len(selected), len(ids), scopeState(targetCounts, intermediateCounts))
	for _, status := range orderedStatuses() {
		fmt.Fprintf(writer, "target_%s: %d\n", status, targetCounts[status])
	}
	for _, status := range orderedStatuses() {
		fmt.Fprintf(writer, "intermediate_%s: %d\n", status, intermediateCounts[status])
	}
	for _, id := range ids {
		state := manifest.Intermediates[id]
		fmt.Fprintf(writer, "scope: %s state=%s\n", id, state.Status)
		if state.Kind == directionSeedBoardKind {
			for _, line := range candidateStatusLines(state) {
				fmt.Fprintln(writer, line)
			}
		}
		for _, path := range reviewArtifactPaths(state.Artifacts) {
			if path != "" {
				fmt.Fprintf(writer, "artifact: %s\n", path)
			}
		}
		if action := nextIntermediateAction(manifest, state, selected, filter); action != "" {
			fmt.Fprintf(writer, "next: %s\n", action)
		}
	}
	for _, target := range selected {
		if target.AnimationID != "" {
			continue
		}
		state := manifest.Targets[target.ID]
		if state == nil {
			continue
		}
		for _, path := range reviewArtifactPaths(state.Artifacts) {
			if path != "" {
				fmt.Fprintf(writer, "artifact: %s\n", path)
			}
		}
		if action := nextTargetAction(manifest.RunID, target, state); action != "" {
			fmt.Fprintf(writer, "next: %s\n", action)
		}
	}
	return nil
}

func reviewArtifactPaths(artifacts generate.ReviewArtifacts) []string {
	paths := []string{artifacts.PromptPath, artifacts.QAPath, artifacts.CandidateSheetPath, artifacts.ContactSheetPath, artifacts.AnimationGIFPath}
	return append(paths, artifacts.FramePaths...)
}

func orderedStatuses() []string {
	return []string{generate.StatusPending, generate.StatusAwaitingReview, generate.StatusAccepted, generate.StatusRejected, generate.StatusDeployed}
}

func intermediateInScope(state *generate.IntermediateState, wanted, wantedObjects map[string]bool) bool {
	if state.Kind == directionSeedBoardKind && wantedObjects[state.ObjectID] {
		return true
	}
	for _, id := range state.TargetIDs {
		if wanted[id] {
			return true
		}
	}
	return false
}

func scopeState(countGroups ...map[string]int) string {
	for _, status := range []string{generate.StatusAwaitingReview, generate.StatusRejected, generate.StatusAccepted, generate.StatusDeployed, generate.StatusPending} {
		for _, counts := range countGroups {
			if counts[status] != 0 {
				return status
			}
		}
	}
	return generate.StatusPending
}

func nextIntermediateAction(manifest *generate.Manifest, state *generate.IntermediateState, selected []targets.Target, filter targets.Filter) string {
	runID := manifest.RunID
	scope := intermediateTargets(state, selected)
	if targetsDeployed(manifest, scope) {
		return ""
	}

	if state.Kind == directionSeedBoardKind {
		switch state.Status {
		case generate.StatusAwaitingReview:
			evidence := generate.CandidateEvidence(state)
			candidate := evidence.MechanicallyPreferred
			if candidate == "" {
				return fmt.Sprintf("sprites-ai-gen review --run %s --object %s --stage seed --status rejected --reason no-mechanically-valid-seed-candidate", runID, state.ObjectID)
			}
			return fmt.Sprintf("sprites-ai-gen review --run %s --object %s --candidate %s --status accepted", runID, state.ObjectID, candidate)
		case generate.StatusPending, generate.StatusRejected, generate.StatusAccepted:
			target, ok := nextPendingTarget(manifest, scope)
			if !ok && state.Status != generate.StatusAccepted && len(scope) != 0 {
				target, ok = scope[0], true
			}
			if !ok {
				return ""
			}
			return targetCommand("generate", runID, target, filter.Frame, state.Status == generate.StatusRejected)
		default:
			return ""
		}
	}

	if len(scope) == 0 {
		return ""
	}
	target := scope[0]
	switch state.Status {
	case generate.StatusAwaitingReview:
		return targetCommand("review", runID, target, "", false) + " --status accepted"
	case generate.StatusRejected:
		return targetCommand("generate", runID, target, latestCorrectedFrame(state), true)
	case generate.StatusAccepted:
		return targetCommand("deploy", runID, target, "", false)
	case generate.StatusPending:
		return targetCommand("generate", runID, target, latestCorrectedFrame(state), false)
	default:
		return ""
	}
}

func intermediateTargets(state *generate.IntermediateState, selected []targets.Target) []targets.Target {
	wanted := make(map[string]bool, len(state.TargetIDs))
	for _, id := range state.TargetIDs {
		wanted[id] = true
	}
	result := make([]targets.Target, 0, len(selected))
	for _, target := range selected {
		if state.Kind == directionSeedBoardKind {
			if target.ObjectID == state.ObjectID && target.AnimationID != "" {
				result = append(result, target)
			}
			continue
		}
		if wanted[target.ID] {
			result = append(result, target)
		}
	}
	return result
}

func targetsDeployed(manifest *generate.Manifest, selected []targets.Target) bool {
	if len(selected) == 0 {
		return false
	}
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		if state == nil || state.Status != generate.StatusDeployed {
			return false
		}
	}
	return true
}

func nextPendingTarget(manifest *generate.Manifest, selected []targets.Target) (targets.Target, bool) {
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		if state != nil && state.Status == generate.StatusPending {
			return target, true
		}
	}
	return targets.Target{}, false
}

func latestCorrectedFrame(state *generate.IntermediateState) string {
	if len(state.Attempts) == 0 {
		return ""
	}
	return state.Attempts[len(state.Attempts)-1].CorrectedFrame
}

func targetCommand(command, runID string, target targets.Target, frame string, force bool) string {
	parts := []string{"sprites-ai-gen", command, "--run", runID, "--object", target.ObjectID}
	if target.AnimationID != "" {
		parts = append(parts, "--animation", target.AnimationID)
	}
	for _, variant := range target.Variants {
		parts = append(parts, "--variant", variant.AxisID+"="+variant.ValueID)
	}
	if frame != "" {
		parts = append(parts, "--frame", frame)
	}
	if force {
		parts = append(parts, "--force")
	}
	return strings.Join(parts, " ")
}

func candidateStatusLines(state *generate.IntermediateState) []string {
	evidence := generate.CandidateEvidence(state)
	return []string{
		"eligible_candidates: " + joinedCandidateIDs(evidence.Eligible),
		"invalid_candidates: " + joinedCandidateIDs(evidence.Invalid),
		"mechanically_preferred_candidate: " + joinedCandidateIDs([]string{evidence.MechanicallyPreferred}),
	}
}

func joinedCandidateIDs(ids []string) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			values = append(values, id)
		}
	}
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func nextTargetAction(runID string, target targets.Target, state *generate.TargetState) string {
	switch state.Status {
	case generate.StatusAwaitingReview:
		return targetCommand("review", runID, target, "", false) + " --status accepted"
	case generate.StatusRejected:
		return targetCommand("generate", runID, target, "", true)
	case generate.StatusAccepted:
		return targetCommand("deploy", runID, target, "", false)
	default:
		return ""
	}
}
