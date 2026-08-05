// Package status renders manifest-V12 scope summaries and executable next actions.
package status

import (
	"fmt"
	"io"
	"sort"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

func Print(writer io.Writer, manifest *generate.Manifest, all []targets.Target, filter targets.Filter) error {
	selected, err := targets.Select(all, filter)
	if err != nil {
		return fmt.Errorf("select status scope: %w", err)
	}
	targetCounts := map[string]int{}
	objects := map[string]bool{}
	var statics []targets.Target
	for _, target := range selected {
		if state := manifest.Targets[target.ID]; state != nil {
			targetCounts[state.Status]++
		}
		if target.AnimationID == "" {
			statics = append(statics, target)
		} else {
			objects[target.ObjectID] = true
		}
	}
	fmt.Fprintf(
		writer,
		"run_id: %s\nconfig_sha256: %s\nstyle_guide_sha256: %s\nscope_targets: %d\nprovider_calls_remaining: %d\n",
		manifest.RunID,
		manifest.ConfigSHA256,
		manifest.StyleGuideSHA256,
		len(selected),
		generate.ProviderCallsRemaining(manifest, selected),
	)
	for _, failure := range manifest.Failures {
		if objects[failure.ObjectID] || targetObjectSelected(statics, failure.ObjectID) {
			fmt.Fprintf(
				writer,
				"failure: object=%s stage=%s ambiguous=%t error=%s\n",
				failure.ObjectID,
				failure.Stage,
				failure.Ambiguous,
				failure.Error,
			)
		}
	}
	for _, status := range orderedStatuses() {
		fmt.Fprintf(writer, "target_%s: %d\n", status, targetCounts[status])
	}
	objectIDs := make([]string, 0, len(objects))
	for objectID := range objects {
		objectIDs = append(objectIDs, objectID)
	}
	sort.Strings(objectIDs)
	for _, objectID := range objectIDs {
		printUnit(writer, manifest, objectID)
	}
	printedStaticSets := map[string]bool{}
	for _, target := range statics {
		if target.ObjectKind == pack.KindStaticSet {
			if !printedStaticSets[target.ObjectID] {
				printStaticSet(writer, manifest, target.ObjectID, statics)
				printedStaticSets[target.ObjectID] = true
			}
			continue
		}
		printStatic(writer, manifest, target)
	}
	return nil
}

func printStaticSet(
	writer io.Writer,
	manifest *generate.Manifest,
	objectID string,
	selected []targets.Target,
) {
	set := manifest.Intermediates[pack.KindStaticSet+":"+objectID]
	if set == nil {
		fmt.Fprintf(writer, "static_set: %s state=%s\n", objectID, generate.StatusPending)
		fmt.Fprintf(writer, "next: sprites-ai-gen generate --run %s --object %s\n", manifest.RunID, objectID)
		return
	}
	fmt.Fprintf(writer, "static_set: %s state=%s lineage=%s\n", objectID, set.Status, set.Lineage)
	printFindings(writer, set.HardRejections, set.Warnings)
	printCandidate(writer, set)
	for _, path := range artifactPaths(set.Artifacts) {
		fmt.Fprintf(writer, "artifact: %s\n", path)
	}
	for _, target := range selected {
		if target.ObjectID != objectID {
			continue
		}
		state := manifest.Targets[target.ID]
		if state == nil {
			continue
		}
		fmt.Fprintf(
			writer,
			"part: %s logical=%dx%d intrinsic=%dx%d density=%d state=%s\n",
			target.ID,
			state.LogicalSize.Width,
			state.LogicalSize.Height,
			state.IntrinsicSize.Width,
			state.IntrinsicSize.Height,
			state.SourceDensity,
			state.Status,
		)
		for _, path := range artifactPaths(state.Artifacts) {
			fmt.Fprintf(writer, "artifact: %s\n", path)
		}
	}
	switch set.Status {
	case generate.StatusAwaitingReview:
		fmt.Fprintf(writer, "next: sprites-ai-gen review --run %s --object %s --status accepted\n", manifest.RunID, objectID)
	case generate.StatusRejected:
		fmt.Fprintf(writer, "next: sprites-ai-gen generate --run auto --object %s\n", objectID)
	case generate.StatusAccepted:
		fmt.Fprintf(writer, "next: sprites-ai-gen deploy --run %s --object %s --dry-run\n", manifest.RunID, objectID)
	}
}

func printUnit(writer io.Writer, manifest *generate.Manifest, objectID string) {
	unit := manifest.Units["unit:"+objectID]
	if unit == nil {
		fmt.Fprintf(writer, "unit: %s state=%s\n", objectID, generate.StatusPending)
		fmt.Fprintf(writer, "next: sprites-ai-gen generate --run %s --object %s\n", manifest.RunID, objectID)
		return
	}
	fmt.Fprintf(writer, "unit: %s state=%s\n", objectID, unit.Status)
	printFindings(writer, unit.HardRejections, nil)
	if master := manifest.Intermediates[unit.MasterID]; master != nil {
		fmt.Fprintf(writer, "master: %s state=%s lineage=%s\n", master.ID, master.Status, master.Lineage)
		printFindings(writer, master.HardRejections, master.Warnings)
		printCandidate(writer, master)
		for _, path := range artifactPaths(master.Artifacts) {
			fmt.Fprintf(writer, "artifact: %s\n", path)
		}
	}
	for _, boardID := range unit.AnimationBoardIDs {
		board := manifest.Intermediates[boardID]
		if board == nil {
			fmt.Fprintf(writer, "animation_board: %s state=%s\n", boardID, generate.StatusPending)
			continue
		}
		fmt.Fprintf(writer, "animation_board: %s state=%s lineage=%s\n", board.ID, board.Status, board.Lineage)
		printFindings(writer, board.HardRejections, board.Warnings)
		printCandidate(writer, board)
		for _, path := range artifactPaths(board.Artifacts) {
			fmt.Fprintf(writer, "artifact: %s\n", path)
		}
	}
	for _, path := range artifactPaths(unit.Artifacts) {
		fmt.Fprintf(writer, "artifact: %s\n", path)
	}
	switch unit.Status {
	case generate.StatusAwaitingReview:
		fmt.Fprintf(writer, "next: sprites-ai-gen review --run %s --object %s --status accepted\n", manifest.RunID, objectID)
	case generate.StatusAccepted:
		fmt.Fprintf(writer, "next: sprites-ai-gen deploy --run %s --object %s --dry-run\n", manifest.RunID, objectID)
	case generate.StatusRejected:
		fmt.Fprintf(
			writer,
			"next: sprites-ai-gen generate --run auto --object %s\n",
			objectID,
		)
	case generate.StatusPending:
		fmt.Fprintf(writer, "next: sprites-ai-gen generate --run %s --object %s\n", manifest.RunID, objectID)
	}
}

func printStatic(writer io.Writer, manifest *generate.Manifest, target targets.Target) {
	state := manifest.Targets[target.ID]
	if state == nil {
		return
	}
	fmt.Fprintf(writer, "static: %s state=%s\n", target.ID, state.Status)
	printFindings(writer, state.HardRejections, state.Warnings)
	if state.SourceCandidate != "" {
		fmt.Fprintf(writer, "source_candidate: %s\n", state.SourceCandidate)
	}
	for _, path := range artifactPaths(state.Artifacts) {
		fmt.Fprintf(writer, "artifact: %s\n", path)
	}
	switch state.Status {
	case generate.StatusAwaitingReview:
		fmt.Fprintf(
			writer,
			"next: sprites-ai-gen review --run %s %s --status accepted\n",
			manifest.RunID,
			scopeFlag(target),
		)
	case generate.StatusRejected:
		fmt.Fprintf(writer, "next: sprites-ai-gen generate --run auto %s\n", scopeFlag(target))
	case generate.StatusAccepted:
		fmt.Fprintf(
			writer,
			"next: sprites-ai-gen deploy --run %s %s --dry-run\n",
			manifest.RunID,
			scopeFlag(target),
		)
	}
}

func printFindings(writer io.Writer, blockers, warnings []string) {
	for _, blocker := range blockers {
		fmt.Fprintf(writer, "blocker: %s\n", blocker)
	}
	for _, warning := range warnings {
		fmt.Fprintf(writer, "warning: %s\n", warning)
	}
}

func scopeFlag(target targets.Target) string {
	if target.ObjectKind == targets.StyleGuideTargetID {
		return "--style-guide"
	}
	return "--object " + target.ObjectID
}

func targetObjectSelected(selected []targets.Target, objectID string) bool {
	for _, target := range selected {
		if target.ObjectID == objectID {
			return true
		}
	}
	return false
}

func printCandidate(writer io.Writer, state *generate.IntermediateState) {
	if len(state.Attempts) == 0 {
		return
	}
	attempt := state.Attempts[len(state.Attempts)-1]
	candidate := "none"
	if len(attempt.Candidates) == 1 {
		candidate = attempt.ID + "/" + attempt.Candidates[0].ID
	}
	fmt.Fprintf(writer, "current_candidate: %s\n", candidate)
	if len(attempt.Candidates) == 1 {
		current := attempt.Candidates[0]
		for _, path := range []string{current.RawPath, current.NormalizedPath, current.MetricsPath} {
			if path != "" {
				fmt.Fprintf(writer, "artifact: %s\n", path)
			}
		}
	}
}

func artifactPaths(artifacts generate.ReviewArtifacts) []string {
	paths := []string{
		artifacts.PromptPath,
		artifacts.EvidencePath,
		artifacts.QAPath,
		artifacts.CurrentReferenceSheetPath,
		artifacts.CanonicalProfilePath,
		artifacts.CanonicalProfileOverlayPath,
		artifacts.MasterSheetPath,
		artifacts.CandidateSheetPath,
		artifacts.BoardMetricsPath,
		artifacts.IdentityComparisonPath,
		artifacts.OwnershipOverlayPath,
		artifacts.RecoveredPoseSheetPath,
		artifacts.CompleteUnitSheetPath,
		artifacts.ContactSheetPath,
		artifacts.AnimationGIFPath,
		artifacts.NativePreviewPath,
		artifacts.PortraitPreviewPath,
		artifacts.BattlefieldPreviewPath,
		artifacts.TiledPreviewPath,
		artifacts.RuntimeOverrideRoot,
	}
	paths = append(paths, artifacts.AnimationBoardPaths...)
	paths = append(paths, artifacts.AnimationGIFPaths...)
	paths = append(paths, artifacts.RecoveredPosePaths...)
	paths = append(paths, artifacts.FramePaths...)
	var present []string
	seen := map[string]bool{}
	for _, path := range paths {
		if path != "" && !seen[path] {
			present = append(present, path)
			seen[path] = true
		}
	}
	return present
}

func orderedStatuses() []string {
	return []string{
		generate.StatusPending,
		generate.StatusReady,
		generate.StatusAwaitingReview,
		generate.StatusAccepted,
		generate.StatusRejected,
		generate.StatusDeployed,
	}
}
