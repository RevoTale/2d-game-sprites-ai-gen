package generate

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
)

func generateCharacterMaster(
	ctx context.Context,
	gen provider.Provider,
	opts Options,
	manifest *Manifest,
	plan animatedUnitPlan,
) error {
	state := manifest.Intermediates[plan.MasterID]
	if state == nil {
		state = &IntermediateState{ID: plan.MasterID, Kind: characterMasterKind, ObjectID: plan.ObjectID, TargetIDs: append([]string(nil), plan.TargetIDs...)}
		manifest.Intermediates[plan.MasterID] = state
	}
	layout, err := imageio.SemanticMasterLayout(len(plan.Directions))
	if err != nil {
		return err
	}
	dir := filepath.Join(runRoot(opts), "intermediates", plan.ObjectID, "character-master")
	referencePaths := make([]string, len(plan.Directions))
	for index, direction := range plan.Directions {
		referencePaths[index] = direction.ReferencePath
	}
	profileMode := imageio.SubjectRegistrationMode(plan.RegistrationMode)
	profile, err := imageio.BuildCanonicalSubjectProfile(
		referencePaths,
		profileMode,
		imageio.CanonicalScaleClass(plan.ScaleClass),
		plan.Size.Height,
	)
	if err != nil {
		return err
	}
	profilePath := filepath.Join(dir, "canonical-profile.json")
	if err := imageio.WriteCanonicalSubjectProfile(profilePath, profile); err != nil {
		return err
	}
	profileOverlayPath := filepath.Join(dir, "canonical-profile-overlay.png")
	if err := imageio.WriteCanonicalSubjectProfileOverlay(referencePaths, profileOverlayPath, profile); err != nil {
		return err
	}
	unit := manifest.Units[plan.ID]
	if unit == nil {
		return fmt.Errorf("unit %q is missing while building canonical profile", plan.ID)
	}
	unit.Profile = &profile
	unit.Artifacts.CanonicalProfilePath = profilePath
	unit.Artifacts.CanonicalProfileOverlayPath = profileOverlayPath
	referenceBoard := filepath.Join(dir, "current-directional-references.png")
	if err := imageio.WriteSemanticBoard(
		referencePaths,
		referenceBoard,
		layout,
		semanticBoardExtent,
	); err != nil {
		return err
	}
	layoutSource := filepath.Join(dir, "layout-source.png")
	if err := imageio.CopyFile(referenceBoard, layoutSource); err != nil {
		return err
	}
	providerLayout := filepath.Join(dir, "provider", "layout-source.png")
	if _, err := imageio.WriteOpaqueChromaCopies([]string{layoutSource}, []string{providerLayout}); err != nil {
		return err
	}
	providerDirectionReferences := make([]string, len(plan.Directions))
	for index, direction := range plan.Directions {
		providerDirectionReferences[index] = filepath.Join(dir, "provider", "direction-references", direction.ID+".png")
	}
	if _, err := imageio.WriteOpaqueChromaCopies(referencePaths, providerDirectionReferences); err != nil {
		return err
	}
	inputs := []conditioning.Input{{
		ID: "master-layout", Role: conditioning.RolePose, Authority: "cli-protocol", SourcePath: layoutSource, Path: providerLayout,
		Description: "Configured-direction placement board on the flat-chroma semantic master layout.", Required: true,
	}}
	inputs = append(inputs, plan.IdentityInputs...)
	for index, direction := range plan.Directions {
		inputs = append(inputs, conditioning.Input{
			ID: pack.DirectionReferenceID(plan.ObjectID, direction.ID), Role: conditioning.RolePose, Authority: "configured-direction-geometry",
			SourcePath: direction.ReferencePath, Path: providerDirectionReferences[index],
			Description: "Facing, neutral topology, equipment side, grounded registration, and relative roster-size evidence only; legacy colors, stretched anatomy, exact bounds, and proportions are not authoritative. " +
				direction.ReferenceDescription,
			Required: true,
		})
	}
	state.SemanticLayout = &layout
	state.EditSourcePath = layoutSource
	state.Artifacts.CurrentReferenceSheetPath = referenceBoard
	prompt := characterMasterPrompt(plan)
	if err := generateBoardCandidate(
		ctx,
		gen,
		opts,
		manifest,
		state,
		dir,
		prompt,
		inputs,
		nil,
		"",
		"",
		characterMasterKind,
		layout.Canvas(),
	); err != nil {
		return err
	}
	if state.Status == StatusAwaitingReview {
		state.Status = StatusReady
		state.Artifacts.MasterSheetPath = state.NormalizedPath
	}
	return Save(opts.OutputDir, opts.RunID, manifest)
}

func generateAnimationBoard(
	ctx context.Context,
	gen provider.Provider,
	opts Options,
	manifest *Manifest,
	unit animatedUnitPlan,
	animation animationPlan,
	master *IntermediateState,
) error {
	state := manifest.Intermediates[animation.BoardID]
	if state == nil {
		state = &IntermediateState{
			ID: animation.BoardID, Kind: animationBoardKind, ObjectID: unit.ObjectID, AnimationID: animation.ID,
			TargetIDs: targetIDs(animation.Targets), Dependencies: []string{unit.MasterID},
		}
		manifest.Intermediates[animation.BoardID] = state
	}
	layout, err := imageio.SemanticAnimationLayout(
		len(unit.Directions),
		len(animation.Frames),
	)
	if err != nil {
		return err
	}
	dir := filepath.Join(runRoot(opts), "intermediates", unit.ObjectID, "animations", animation.ID)
	if master.SemanticLayout == nil {
		return fmt.Errorf("character master %q has no semantic layout", master.ID)
	}
	masterDirectionPaths := master.Artifacts.RecoveredPosePaths
	if len(masterDirectionPaths) != len(unit.Directions) {
		return fmt.Errorf(
			"character master %q has %d recovered directions, expected %d",
			master.ID,
			len(masterDirectionPaths),
			len(unit.Directions),
		)
	}
	masterPlacementPaths := make([]string, 0, len(animation.Targets))
	for index := range unit.Directions {
		for range animation.Frames {
			masterPlacementPaths = append(masterPlacementPaths, masterDirectionPaths[index])
		}
	}
	layoutSource := filepath.Join(dir, "layout-source.png")
	// Preserve one master-derived scale across every semantic pose anchor. The
	// provider receives no mask or visible boundary contract.
	if err := imageio.WriteSemanticBoardAtNativeScale(
		masterPlacementPaths,
		layoutSource,
		layout,
		semanticBoardExtent,
	); err != nil {
		return err
	}
	providerLayout := filepath.Join(dir, "provider", "layout-source.png")
	if _, err := imageio.WriteOpaqueChromaCopies([]string{layoutSource}, []string{providerLayout}); err != nil {
		return err
	}
	guideBoard := filepath.Join(dir, "master-comparison-guide.png")
	if err := imageio.CopyFile(layoutSource, guideBoard); err != nil {
		return err
	}
	inputs := []conditioning.Input{{
		ID: "animation-layout", Role: conditioning.RolePose,
		Authority: "cli-protocol", SourcePath: layoutSource, Path: providerLayout,
		Description: "Generated-master placement board on the flat-chroma semantic animation layout.",
		Required:    true,
	}}
	reviewOnly := []conditioning.Input{{
		ID:          "character-master:" + unit.ObjectID,
		Role:        conditioning.RoleIdentity,
		Authority:   "character-master",
		SourcePath:  master.NormalizedPath,
		Path:        master.NormalizedPath,
		Description: "Appearance authority embedded at every provider-layout anchor; retained for lineage and review only.",
		Required:    true,
	}}
	for _, direction := range unit.Directions {
		reviewOnly = append(reviewOnly, conditioning.Input{
			ID: pack.DirectionReferenceID(unit.ObjectID, direction.ID), Role: conditioning.RoleIdentity, Authority: "review-only",
			Path: direction.ReferencePath, SourcePath: direction.ReferencePath, Description: direction.ReferenceDescription,
		})
	}
	state.SemanticLayout = &layout
	state.EditSourcePath = layoutSource
	state.Dependencies = []string{unit.MasterID}
	prompt := animationBoardPrompt(unit, animation)
	if err := generateBoardCandidate(
		ctx,
		gen,
		opts,
		manifest,
		state,
		dir,
		prompt,
		inputs,
		reviewOnly,
		"",
		"",
		animationBoardKind,
		layout.Canvas(),
	); err != nil {
		return err
	}
	if state.Status == StatusAwaitingReview {
		state.Status = StatusReady
		state.ParentID = unit.ID
	}
	return Save(opts.OutputDir, opts.RunID, manifest)
}
