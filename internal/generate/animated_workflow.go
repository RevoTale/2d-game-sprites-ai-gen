package generate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

const (
	characterMasterKind = "character-master"
	animationBoardKind  = "animation-board"
	unitKind            = "unit"
	providerCanvasSize  = 1024
	semanticBoardExtent = 256
)

type animatedWorkflowPlan struct {
	StaticGroups [][]targets.Target
	Units        []animatedUnitPlan
}

type animatedUnitPlan struct {
	ID               string
	ObjectID         string
	MasterID         string
	Directions       []directionPlan
	Animations       []animationPlan
	TargetIDs        []string
	IdentityInputs   []conditioning.Input
	Style            pack.Style
	Archetype        string
	ScaleClass       string
	Size             pack.Size
	ObjectDesc       string
	MagicSources     []pack.MagicSource
	IdentityLocks    []string
	RegistrationMode string
}

type directionPlan struct {
	ID                   string
	Description          string
	ReferencePath        string
	ReferenceDescription string
}

type animationPlan struct {
	ID          string
	BoardID     string
	Description string
	Frames      []animationFramePlan
	Targets     []targets.Target
}

type animationFramePlan struct {
	ID          string
	Description string
}

func buildAnimatedPlan(all, selected []targets.Target) (animatedWorkflowPlan, error) {
	plan := animatedWorkflowPlan{}
	animatedObjects := map[string]bool{}
	staticTargets := make([]targets.Target, 0, len(selected))
	for _, target := range selected {
		if target.AnimationID == "" {
			staticTargets = append(staticTargets, target)
			continue
		}
		animatedObjects[target.ObjectID] = true
	}
	plan.StaticGroups = targets.AtomicGroups(staticTargets)
	for _, objectID := range orderedSelectedObjects(all, animatedObjects) {
		unit, err := buildAnimatedUnitPlan(all, objectID)
		if err != nil {
			return animatedWorkflowPlan{}, err
		}
		plan.Units = append(plan.Units, unit)
	}
	return plan, nil
}

func buildAnimatedUnitPlan(all []targets.Target, objectID string) (animatedUnitPlan, error) {
	var objectTargets []targets.Target
	for _, target := range all {
		if target.ObjectID == objectID && target.AnimationID != "" {
			objectTargets = append(objectTargets, target)
		}
	}
	if len(objectTargets) == 0 {
		return animatedUnitPlan{}, fmt.Errorf("animated object %q has no targets", objectID)
	}
	first := objectTargets[0]
	unit := animatedUnitPlan{
		ID:               unitKind + ":" + objectID,
		ObjectID:         objectID,
		MasterID:         characterMasterKind + ":" + objectID,
		ObjectDesc:       first.ObjectDesc,
		Style:            first.Style,
		Archetype:        first.Archetype,
		ScaleClass:       first.Style.Units.Archetypes[first.Archetype].ScaleClass,
		Size:             first.Size,
		MagicSources:     append([]pack.MagicSource{}, first.MagicSources...),
		IdentityLocks:    append([]string(nil), first.IdentityLocks...),
		IdentityInputs:   filterInputs(first.Inputs, conditioning.RoleStyle, conditioning.RoleIdentity),
		RegistrationMode: first.RegistrationMode,
	}
	directionSeen := map[string]bool{}
	for _, target := range objectTargets {
		if target.DirectionID == "" || directionSeen[target.DirectionID] {
			continue
		}
		if target.DirectionRefPath == "" {
			return animatedUnitPlan{}, fmt.Errorf("animated object %q direction %q has no configured reference", objectID, target.DirectionID)
		}
		directionSeen[target.DirectionID] = true
		unit.Directions = append(unit.Directions, directionPlan{
			ID:                   target.DirectionID,
			Description:          target.DirectionDesc,
			ReferencePath:        target.DirectionRefPath,
			ReferenceDescription: target.DirectionRefDesc,
		})
	}
	animationIndexes := map[string]int{}
	for _, target := range objectTargets {
		animationIndexes[target.AnimationID] = target.AnimationIndex
	}
	animationIDs := make([]string, 0, len(animationIndexes))
	for id := range animationIndexes {
		animationIDs = append(animationIDs, id)
	}
	sort.Slice(animationIDs, func(i, j int) bool { return animationIndexes[animationIDs[i]] < animationIndexes[animationIDs[j]] })
	for _, animationID := range animationIDs {
		animation, err := buildAnimationPlan(objectTargets, unit.Directions, animationID)
		if err != nil {
			return animatedUnitPlan{}, err
		}
		unit.Animations = append(unit.Animations, animation)
		unit.TargetIDs = append(unit.TargetIDs, targetIDs(animation.Targets)...)
	}
	return unit, nil
}

func buildAnimationPlan(objectTargets []targets.Target, directions []directionPlan, animationID string) (animationPlan, error) {
	animation := animationPlan{ID: animationID, BoardID: animationBoardKind + ":" + objectTargets[0].ObjectID + ":" + animationID}
	byDirection := map[string][]targets.Target{}
	for _, target := range objectTargets {
		if target.AnimationID != animationID {
			continue
		}
		if target.DirectionID == "" {
			return animationPlan{}, fmt.Errorf("target %q has no direction", target.ID)
		}
		byDirection[target.DirectionID] = append(byDirection[target.DirectionID], target)
		animation.Description = target.AnimationDesc
	}
	for directionIndex, direction := range directions {
		row := byDirection[direction.ID]
		sort.Slice(row, func(i, j int) bool { return row[i].FrameIndex < row[j].FrameIndex })
		if len(row) == 0 {
			return animationPlan{}, fmt.Errorf("animation %q has no direction %q", animationID, direction.ID)
		}
		if directionIndex == 0 {
			for _, target := range row {
				animation.Frames = append(animation.Frames, animationFramePlan{ID: target.FrameID, Description: target.FrameDesc})
			}
		} else if len(row) != len(animation.Frames) {
			return animationPlan{}, fmt.Errorf("animation %q direction %q has %d frames, expected %d", animationID, direction.ID, len(row), len(animation.Frames))
		}
		animation.Targets = append(animation.Targets, row...)
	}
	return animation, nil
}

func preflightAnimated(plan animatedWorkflowPlan, capabilities provider.Capabilities) error {
	if len(plan.Units) != 0 && !capabilities.References {
		return errors.New("animated unit generation requires provider reference support")
	}
	if capabilities.References {
		return nil
	}
	for _, group := range plan.StaticGroups {
		if len(group) != 0 && group[0].ObjectKind == pack.KindStaticSet && !capabilities.Masks {
			return fmt.Errorf("static set %q requires provider mask support", group[0].ObjectID)
		}
		for _, target := range group {
			if len(filterInputs(target.Inputs, conditioning.RoleStyle, conditioning.RoleIdentity)) != 0 {
				return fmt.Errorf("target %q uses image references unsupported by the selected provider", target.ID)
			}
		}
	}
	return nil
}

func validateAnimatedStart(manifest *Manifest, plan animatedWorkflowPlan) error {
	for _, unit := range plan.Units {
		existing := manifest.Units[unit.ID]
		if existing != nil &&
			existing.Status == StatusRejected &&
			(existing.AssemblyVersion >= AnimatedAssemblyVersion ||
				existing.Review != nil) {
			return fmt.Errorf("rejected animated runs are immutable in V13; start a new --run auto run for %q", unit.ObjectID)
		}
	}
	return nil
}

func runAnimatedWorkflow(ctx context.Context, selected []targets.Target, plan animatedWorkflowPlan, gen provider.Provider, opts Options, manifest *Manifest) (Result, error) {
	result := Result{RunID: opts.RunID}
	opts.report(ProgressEvent{Stage: ProgressRunStarted, RunID: opts.RunID, Total: len(selected)})
	for _, group := range plan.StaticGroups {
		if len(group) == 0 {
			continue
		}
		if staticGroupComplete(manifest, group) {
			result.Skipped += len(group)
			continue
		}
		var err error
		if group[0].ObjectKind == pack.KindStaticSet {
			err = generateStaticSet(ctx, gen, opts, manifest, group)
		} else {
			target := group[0]
			err = generateStaticTarget(
				ctx,
				gen,
				opts,
				manifest,
				target,
				manifest.Targets[target.ID],
				result.Generated+1,
				len(selected),
			)
		}
		if err != nil {
			if opts.ContinueOnError {
				recordRunFailure(manifest, group[0].ObjectID, "static-target", err)
				result.Failed++
				if saveErr := Save(opts.OutputDir, opts.RunID, manifest); saveErr != nil {
					return result, saveErr
				}
				continue
			}
			return result, err
		}
		result.Generated++
		for _, target := range group {
			if state := manifest.Targets[target.ID]; state != nil && state.Status == StatusAwaitingReview {
				result.AwaitingReview++
			}
		}
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return result, err
		}
	}
	for _, unitPlan := range plan.Units {
		unit := manifest.Units[unitPlan.ID]
		if unit != nil &&
			(unit.Status == StatusAccepted ||
				unit.Status == StatusDeployed ||
				unit.Status == StatusAwaitingReview &&
					unit.AssemblyVersion >= AnimatedAssemblyVersion) {
			result.Skipped += len(unit.TargetIDs)
			continue
		}
		generated, err := generateAnimatedUnit(ctx, gen, opts, manifest, unitPlan)
		if err != nil {
			if opts.ContinueOnError {
				recordRunFailure(manifest, unitPlan.ObjectID, "animated-unit", err)
				result.Failed++
				if saveErr := Save(opts.OutputDir, opts.RunID, manifest); saveErr != nil {
					return result, saveErr
				}
				continue
			}
			return result, err
		}
		result.Generated += generated
		unit = manifest.Units[unitPlan.ID]
		if unit != nil && unit.Status == StatusAwaitingReview {
			result.AwaitingReview += len(unit.TargetIDs)
		}
	}
	RefreshUnitStatuses(manifest)
	if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return result, err
	}
	opts.report(ProgressEvent{Stage: ProgressRunCompleted, RunID: opts.RunID, Total: len(selected)})
	return result, nil
}

func staticGroupComplete(manifest *Manifest, group []targets.Target) bool {
	if len(group) != 0 && group[0].ObjectKind == pack.KindStaticSet {
		set := manifest.Intermediates[pack.KindStaticSet+":"+group[0].ObjectID]
		if intermediateNeedsReprocessing(set) {
			return false
		}
	}
	for _, target := range group {
		if !shouldSkipGeneration(manifest.Targets[target.ID]) {
			return false
		}
	}
	return true
}

func intermediateNeedsReprocessing(state *IntermediateState) bool {
	if state == nil || state.Review != nil || len(state.Attempts) == 0 {
		return false
	}
	latest := state.Attempts[len(state.Attempts)-1]
	if len(latest.Candidates) != 1 {
		return false
	}
	if latest.Candidates[0].QualityVersion < candidateQualityVersion {
		return true
	}
	return state.Kind == pack.KindStaticSet &&
		(state.StaticSetScale == nil ||
			state.StaticSetScale.Version < imageio.StaticSetScaleCalibrationVersion ||
			staticSetRuntimeOverrideMissing(state))
}

func staticSetRuntimeOverrideMissing(state *IntermediateState) bool {
	if state.Artifacts.RuntimeOverrideRoot == "" {
		return true
	}
	info, err := os.Stat(state.Artifacts.RuntimeOverrideRoot)
	return err != nil || !info.IsDir()
}

func recordRunFailure(manifest *Manifest, objectID, stage string, err error) {
	manifest.Failures = append(manifest.Failures, RunFailure{
		ObjectID:  objectID,
		Stage:     stage,
		Error:     err.Error(),
		Ambiguous: true,
		FailedAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

func generateAnimatedUnit(ctx context.Context, gen provider.Provider, opts Options, manifest *Manifest, plan animatedUnitPlan) (int, error) {
	unit := manifest.Units[plan.ID]
	if unit == nil {
		unit = &UnitState{
			ID:                plan.ID,
			ObjectID:          plan.ObjectID,
			Status:            StatusPending,
			MasterID:          plan.MasterID,
			TargetIDs:         append([]string(nil), plan.TargetIDs...),
			AnimationLineages: map[string]string{},
		}
		for _, animation := range plan.Animations {
			unit.AnimationBoardIDs = append(unit.AnimationBoardIDs, animation.BoardID)
		}
		manifest.Units[plan.ID] = unit
	}
	generated := 0
	master := manifest.Intermediates[plan.MasterID]
	if !readyIntermediate(master) {
		if err := generateCharacterMaster(ctx, gen, opts, manifest, plan); err != nil {
			return generated, err
		}
		generated++
		master = manifest.Intermediates[plan.MasterID]
	}
	if !readyIntermediate(master) {
		RefreshUnitStatuses(manifest)
		return generated, Save(opts.OutputDir, opts.RunID, manifest)
	}
	for _, animation := range plan.Animations {
		board := manifest.Intermediates[animation.BoardID]
		if readyIntermediate(board) {
			continue
		}
		if board != nil && board.Status == StatusRejected {
			break
		}
		if err := generateAnimationBoard(ctx, gen, opts, manifest, plan, animation, master); err != nil {
			return generated, err
		}
		generated++
		if !readyIntermediate(manifest.Intermediates[animation.BoardID]) {
			break
		}
	}
	if allAnimationBoardsReady(manifest, plan) {
		if err := assembleAnimatedUnit(opts, manifest, plan); err != nil {
			return generated, err
		}
	}
	RefreshUnitStatuses(manifest)
	return generated, Save(opts.OutputDir, opts.RunID, manifest)
}

func readyIntermediate(state *IntermediateState) bool {
	if state == nil || state.Status != StatusReady || state.NormalizedPath == "" || state.Lineage == "" {
		return false
	}
	_, err := os.Stat(state.NormalizedPath)
	return err == nil
}

// ProviderCallsRemaining reports calls that the current manifest can still
// resume. A nil manifest represents a fresh run.
func ProviderCallsRemaining(manifest *Manifest, selected []targets.Target) int {
	animationsByObject := map[string]map[string]bool{}
	staticSets := map[string]bool{}
	total := 0
	for _, target := range selected {
		if target.AnimationID == "" {
			if target.ObjectKind == pack.KindStaticSet {
				if staticSets[target.ObjectID] {
					continue
				}
				staticSets[target.ObjectID] = true
			}
			if manifest == nil {
				total++
				continue
			}
			state := manifest.Targets[target.ID]
			if state == nil || state.Status == StatusPending {
				total++
			}
			continue
		}
		if animationsByObject[target.ObjectID] == nil {
			animationsByObject[target.ObjectID] = map[string]bool{}
		}
		animationsByObject[target.ObjectID][target.AnimationID] = true
	}
	for objectID, animations := range animationsByObject {
		if manifest == nil {
			total += 1 + len(animations)
			continue
		}
		unit := manifest.Units[unitKind+":"+objectID]
		if unit != nil {
			switch unit.Status {
			case StatusAwaitingReview, StatusAccepted, StatusRejected, StatusDeployed:
				continue
			}
		}
		if !readyIntermediate(manifest.Intermediates[characterMasterKind+":"+objectID]) {
			total++
		}
		for animationID := range animations {
			if !readyIntermediate(manifest.Intermediates[animationBoardKind+":"+objectID+":"+animationID]) {
				total++
			}
		}
	}
	return total
}

func allAnimationBoardsReady(manifest *Manifest, plan animatedUnitPlan) bool {
	for _, animation := range plan.Animations {
		if !readyIntermediate(manifest.Intermediates[animation.BoardID]) {
			return false
		}
	}
	return true
}

// RefreshUnitStatuses derives aggregate unit state without introducing a
// second review decision for character masters or animation boards.
func RefreshUnitStatuses(manifest *Manifest) {
	for _, unit := range manifest.Units {
		if unit == nil {
			continue
		}
		if unit.Review != nil {
			unit.Status = unit.Review.Status
		}
		if len(unit.HardRejections) != 0 {
			unit.Status = StatusRejected
			continue
		}
		allDeployed := len(unit.TargetIDs) != 0
		allReady := len(unit.TargetIDs) != 0
		intermediatesReady := readyIntermediate(manifest.Intermediates[unit.MasterID])
		for _, boardID := range unit.AnimationBoardIDs {
			if !readyIntermediate(manifest.Intermediates[boardID]) {
				intermediatesReady = false
			}
		}
		for _, targetID := range unit.TargetIDs {
			target := manifest.Targets[targetID]
			if target == nil || target.Status != StatusDeployed {
				allDeployed = false
			}
			if target == nil || target.NormalizedPath == "" {
				allReady = false
			}
		}
		if allDeployed {
			unit.Status = StatusDeployed
		} else if unit.Review == nil && allReady && intermediatesReady {
			unit.Status = StatusAwaitingReview
		} else if unit.Review == nil {
			unit.Status = StatusPending
			for _, boardID := range unit.AnimationBoardIDs {
				if board := manifest.Intermediates[boardID]; board != nil && board.Status == StatusRejected {
					unit.Status = StatusRejected
					break
				}
			}
			if master := manifest.Intermediates[unit.MasterID]; master != nil && master.Status == StatusRejected {
				unit.Status = StatusRejected
			}
		}
	}
}
