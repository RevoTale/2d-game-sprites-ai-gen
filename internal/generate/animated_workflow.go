package generate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
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
	StaticTargets []targets.Target
	Units         []animatedUnitPlan
	SelectedIDs   map[string]bool
}

type animatedUnitPlan struct {
	ID             string
	ObjectID       string
	MasterID       string
	Directions     []directionPlan
	Animations     []animationPlan
	TargetIDs      []string
	IdentityInputs []conditioning.Input
	ObjectDesc     string
	IdentityLocks  []string
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

func buildAnimatedPlan(all, selected []targets.Target, filter targets.Filter) (animatedWorkflowPlan, error) {
	if filter.Animation != "" || filter.Frame != "" || len(filter.Variants) != 0 {
		return animatedWorkflowPlan{}, errors.New(
			"animated generation is complete-unit only in V9; start a new object run",
		)
	}
	plan := animatedWorkflowPlan{SelectedIDs: make(map[string]bool, len(selected))}
	animatedObjects := map[string]bool{}
	for _, target := range selected {
		plan.SelectedIDs[target.ID] = true
		if target.AnimationID == "" {
			plan.StaticTargets = append(plan.StaticTargets, target)
			continue
		}
		animatedObjects[target.ObjectID] = true
	}
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
		ID:             unitKind + ":" + objectID,
		ObjectID:       objectID,
		MasterID:       characterMasterKind + ":" + objectID,
		ObjectDesc:     first.ObjectDesc,
		IdentityLocks:  append([]string(nil), first.IdentityLocks...),
		IdentityInputs: filterInputs(first.Inputs, conditioning.RoleStyle, conditioning.RoleIdentity),
	}
	directionSeen := map[string]bool{}
	for _, target := range objectTargets {
		direction, ok := directionSelection(target)
		if !ok || directionSeen[direction.ValueID] {
			continue
		}
		if direction.ReferencePath == "" {
			return animatedUnitPlan{}, fmt.Errorf("animated object %q direction %q has no configured reference", objectID, direction.ValueID)
		}
		directionSeen[direction.ValueID] = true
		unit.Directions = append(unit.Directions, directionPlan{
			ID:                   direction.ValueID,
			Description:          direction.Description,
			ReferencePath:        direction.ReferencePath,
			ReferenceDescription: direction.ReferenceDescription,
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
		direction, ok := directionSelection(target)
		if !ok {
			return animationPlan{}, fmt.Errorf("target %q has no direction", target.ID)
		}
		byDirection[direction.ValueID] = append(byDirection[direction.ValueID], target)
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

func directionSelection(target targets.Target) (targets.VariantSelection, bool) {
	for _, variant := range target.Variants {
		if variant.AxisID == "direction" {
			return variant, true
		}
	}
	return targets.VariantSelection{}, false
}

func preflightAnimated(plan animatedWorkflowPlan, capabilities provider.Capabilities, deployDir string) error {
	if len(plan.Units) != 0 && !capabilities.References {
		return errors.New("animated unit generation requires provider reference support")
	}
	if capabilities.References {
		return nil
	}
	for _, target := range plan.StaticTargets {
		if len(filterInputs(target.Inputs, conditioning.RoleStyle, conditioning.RoleIdentity)) != 0 {
			return fmt.Errorf("target %q uses image references unsupported by the selected provider", target.ID)
		}
		posePath, err := existingDeployPath(target, deployDir)
		if err != nil {
			return err
		}
		if posePath != "" {
			return fmt.Errorf("target %q uses existing production art as an image reference unsupported by the selected provider", target.ID)
		}
	}
	return nil
}

func validateAnimatedStart(manifest *Manifest, plan animatedWorkflowPlan, opts Options) error {
	for _, unit := range plan.Units {
		existing := manifest.Units[unit.ID]
		if opts.Force {
			return errors.New("animated generation is complete-unit only in V9; rejected runs require a new --run auto run")
		}
		if existing != nil && existing.Status == StatusRejected {
			return fmt.Errorf("rejected animated runs are immutable in V9; start a new --run auto run for %q", unit.ObjectID)
		}
	}
	return nil
}

func runAnimatedWorkflow(ctx context.Context, selected []targets.Target, plan animatedWorkflowPlan, gen provider.Provider, opts Options, manifest *Manifest) (Result, error) {
	result := Result{RunID: opts.RunID}
	opts.report(ProgressEvent{Stage: ProgressRunStarted, RunID: opts.RunID, Total: len(selected)})
	for _, target := range plan.StaticTargets {
		state := manifest.Targets[target.ID]
		force := opts.Force && plan.SelectedIDs[target.ID]
		if shouldSkipGeneration(state, force) {
			result.Skipped++
			continue
		}
		posePath, _ := existingDeployPath(target, opts.DeployDir)
		if err := generateStaticTarget(ctx, gen, opts, manifest, target, posePath, state, force, result.Generated+1, len(selected)); err != nil {
			return result, err
		}
		result.Generated++
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return result, err
		}
	}
	for _, unitPlan := range plan.Units {
		unit := manifest.Units[unitPlan.ID]
		if unit != nil && (unit.Status == StatusAwaitingReview || unit.Status == StatusAccepted || unit.Status == StatusDeployed) {
			result.Skipped += len(unit.TargetIDs)
			continue
		}
		generated, err := generateAnimatedUnit(ctx, gen, opts, manifest, unitPlan)
		if err != nil {
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
