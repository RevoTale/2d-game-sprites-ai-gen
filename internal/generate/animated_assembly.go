package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
)

func assembleAnimatedUnit(opts Options, manifest *Manifest, plan animatedUnitPlan) error {
	unit := manifest.Units[plan.ID]
	master := manifest.Intermediates[plan.MasterID]
	masterPaletteSources := append(
		[]string(nil),
		master.Artifacts.RecoveredPosePaths...,
	)
	palette, err := imageio.SharedPaletteFromPNGs(masterPaletteSources, 32)
	if err != nil {
		return err
	}
	if len(plan.Animations) == 0 || len(plan.Animations[0].Targets) == 0 {
		return nil
	}
	frameSize := plan.Animations[0].Targets[0].Size
	if unit.Profile == nil {
		return rejectAnimatedAssembly(
			opts,
			manifest,
			unit,
			"missing_canonical_subject_profile",
		)
	}
	if unit.Profile.Version != imageio.CanonicalSubjectProfileVersion {
		if err := rebuildCanonicalSubjectProfile(opts, unit, plan); err != nil {
			return rejectAnimatedAssembly(
				opts,
				manifest,
				unit,
				"invalid_canonical_subject_profile: "+err.Error(),
			)
		}
	}
	transform, err := imageio.FitCanonicalSubjectTransform(
		*unit.Profile,
		master.Artifacts.RecoveredPosePaths,
		master.Poses,
		frameSize.Width,
		frameSize.Height,
	)
	if err != nil {
		return rejectAnimatedAssembly(
			opts,
			manifest,
			unit,
			"unsafe_shared_unit_transform: "+err.Error(),
		)
	}
	masterPosesByDirection := make([][]imageio.SemanticPose, len(plan.Directions))
	for directionIndex, pose := range master.Poses {
		masterPosesByDirection[directionIndex] = append(
			masterPosesByDirection[directionIndex],
			pose,
		)
	}
	masterScales := make([]float64, len(plan.Directions))
	for directionIndex := range masterScales {
		masterScales[directionIndex] = transform.Scale
	}
	poseSets := []imageio.SemanticPoseSet{{
		PosesByDirection: masterPosesByDirection,
		DirectionScales:  masterScales,
	}}
	calibrations := make(map[string]imageio.SemanticScaleCalibration, len(plan.Animations))
	registeredPoses := make(map[string][]imageio.SemanticPose, len(plan.Animations))
	for _, animation := range plan.Animations {
		board := manifest.Intermediates[animation.BoardID]
		expectedPoses := len(plan.Directions) * len(animation.Frames)
		if len(board.Poses) != expectedPoses {
			return rejectAnimatedAssembly(
				opts,
				manifest,
				unit,
				"invalid_animation_pose_count: "+animation.ID,
			)
		}
		registered, pivotOffsets, registrationErr :=
			imageio.PrepareSemanticPosesForSharedBodyAnchor(
				board.Poses,
				len(animation.Frames),
			)
		if registrationErr != nil {
			return rejectAnimatedAssembly(
				opts,
				manifest,
				unit,
				"invalid_board_registration: "+animation.ID+": "+
					registrationErr.Error(),
			)
		}
		registeredPoses[animation.BoardID] = registered
		calibrationPaths := make([]string, len(plan.Directions))
		boardPosesByDirection := make([][]imageio.SemanticPose, len(plan.Directions))
		for directionIndex := range plan.Directions {
			start := directionIndex * len(animation.Frames)
			calibrationPaths[directionIndex] = board.Artifacts.RecoveredPosePaths[start]
			boardPosesByDirection[directionIndex] = append(
				boardPosesByDirection[directionIndex],
				registered[start:start+len(animation.Frames)]...,
			)
		}
		calibration, calibrationErr := imageio.CalibrateSemanticPoseSet(
			master.Artifacts.RecoveredPosePaths,
			calibrationPaths,
			unit.Profile.Mode,
			transform.Scale,
		)
		if calibrationErr != nil {
			return rejectAnimatedAssembly(
				opts,
				manifest,
				unit,
				"invalid_board_scale_calibration: "+animation.ID+": "+
					calibrationErr.Error(),
			)
		}
		calibration.DirectionPivotOffsets = pivotOffsets
		calibration.PoseMeasurements, calibrationErr = imageio.MeasureSemanticPoses(
			board.Artifacts.RecoveredPosePaths,
			registered,
			calibration.DirectionScales,
			len(animation.Frames),
			unit.Profile.Mode,
		)
		if calibrationErr != nil {
			return rejectAnimatedAssembly(
				opts,
				manifest,
				unit,
				"invalid_board_pose_measurement: "+animation.ID+": "+
					calibrationErr.Error(),
			)
		}
		calibrationPath := filepath.Join(
			filepath.Dir(board.NormalizedPath),
			"scale-calibration.json",
		)
		if err := imageio.WriteSemanticScaleCalibration(
			calibrationPath,
			calibration,
		); err != nil {
			return err
		}
		board.ScaleCalibration = &calibration
		board.Artifacts.ScaleCalibrationPath = calibrationPath
		calibrations[animation.BoardID] = calibration
		safeWidth := frameSize.Width -
			2*imageio.CanonicalFrameEdgePadding(frameSize.Width, frameSize.Height)
		safeHeight := frameSize.Height -
			2*imageio.CanonicalFrameEdgePadding(frameSize.Width, frameSize.Height)
		for _, measurement := range calibration.PoseMeasurements {
			if measurement.CanonicalWidth <= safeWidth &&
				measurement.CanonicalHeight <= safeHeight {
				continue
			}
			return rejectAnimatedAssembly(
				opts,
				manifest,
				unit,
				fmt.Sprintf(
					"unsafe_canonical_pose_extent: %s direction %s frame %s requires %dx%d, safe frame is %dx%d",
					animation.ID,
					plan.Directions[measurement.Direction].ID,
					animation.Frames[measurement.Frame].ID,
					measurement.CanonicalWidth,
					measurement.CanonicalHeight,
					safeWidth,
					safeHeight,
				),
			)
		}
		poseSets = append(poseSets, imageio.SemanticPoseSet{
			PosesByDirection: boardPosesByDirection,
			DirectionScales:  calibration.DirectionScales,
		})
	}
	transform, err = imageio.ConstrainSemanticUnitAnchorsAcrossPoseSets(
		transform,
		poseSets,
		frameSize.Width,
		frameSize.Height,
	)
	if err != nil {
		return rejectAnimatedAssembly(
			opts,
			manifest,
			unit,
			"unsafe_shared_unit_anchor: "+err.Error(),
		)
	}
	unit.HardRejections = nil
	unit.Transform = &transform
	unitDir := filepath.Join(runRoot(opts), "units", plan.ObjectID)
	if err := imageio.WritePalette(filepath.Join(unitDir, "palette.json"), palette); err != nil {
		return err
	}
	unit.Artifacts = ReviewArtifacts{
		CurrentReferenceSheetPath:   master.Artifacts.CurrentReferenceSheetPath,
		MasterSheetPath:             master.NormalizedPath,
		CanonicalProfilePath:        unit.Artifacts.CanonicalProfilePath,
		CanonicalProfileOverlayPath: unit.Artifacts.CanonicalProfileOverlayPath,
	}
	unit.MasterLineage = master.Lineage
	unit.AnimationLineages = map[string]string{}
	var allFrames []string
	animationFrames := make(map[string][]string, len(plan.Animations))
	for _, animation := range plan.Animations {
		board := manifest.Intermediates[animation.BoardID]
		calibration := calibrations[animation.BoardID]
		outputs := make([]string, len(animation.Targets))
		for index, target := range animation.Targets {
			outputs[index] = filepath.Join(TargetDir(opts.OutputDir, opts.RunID, target.ID), "normalized.png")
		}
		transforms, normalizeErr := imageio.WriteCalibratedSemanticPoses(
			board.Artifacts.RecoveredPosePaths,
			registeredPoses[animation.BoardID],
			outputs,
			animation.Targets[0].Size.Width,
			animation.Targets[0].Size.Height,
			palette,
			transform.DirectionAnchors,
			calibration.DirectionScales,
			len(animation.Frames),
		)
		if normalizeErr != nil {
			board.Status = StatusRejected
			board.HardRejections = []string{
				"unsafe_production_frame: " + normalizeErr.Error(),
			}
			return rejectAnimatedAssembly(
				opts,
				manifest,
				unit,
				board.HardRejections[0],
			)
		}
		board.Artifacts.FramePaths = outputs
		board.Artifacts.AnimationBoardPaths = []string{board.NormalizedPath}
		animationFrames[animation.ID] = outputs
		unit.Artifacts.AnimationBoardPaths = append(unit.Artifacts.AnimationBoardPaths, board.NormalizedPath)
		unit.AnimationLineages[animation.ID] = board.Lineage
		for directionIndex, direction := range plan.Directions {
			start := directionIndex * len(animation.Frames)
			gifPath := filepath.Join(unitDir, "review", "gifs", animation.ID+"-"+direction.ID+".gif")
			if err := imageio.WriteLoopingGIF(outputs[start:start+len(animation.Frames)], gifPath, 12); err != nil {
				return err
			}
			unit.Artifacts.AnimationGIFPaths = append(unit.Artifacts.AnimationGIFPaths, gifPath)
		}
		for index, target := range animation.Targets {
			state := manifest.Targets[target.ID]
			state.Status = StatusAwaitingReview
			state.NormalizedPath = outputs[index]
			state.UnitID = unit.ID
			state.CharacterMasterID = master.ID
			state.AnimationBoardID = board.ID
			state.MasterLineage = master.Lineage
			state.AnimationLineage = board.Lineage
			state.SourceCandidate = selectedCandidate(board)
			state.CellIndex = index
			state.Dependencies = []string{master.ID, board.ID, unit.ID}
			state.ProductionEligible = true
			state.CapabilityMode = "v13-board-calibrated-registration"
			state.Palette = palette
			state.Normalization = &NormalizationRecord{
				ScaleAlgorithm: "reference-derived-board-calibrated-subject",
				PaletteMethod:  "deterministic-median-cut",
				MaximumColors:  32, ColorSpace: "linear-srgb",
				Dithering: false, AlphaThreshold: 128,
				Anchor:   "semantic-body-pivot",
				Scale:    transforms[index].Scale,
				Baseline: transforms[index].Baseline,
				CenterX:  transforms[index].CenterX,
				OffsetX:  transforms[index].OffsetX,
				OffsetY:  transforms[index].OffsetY,
			}
			state.Review = nil
			state.Deploy = nil
			state.DeployPath = ""
		}
		allFrames = append(allFrames, outputs...)
	}
	unit.Artifacts.CompleteUnitSheetPath = filepath.Join(unitDir, "review", "complete-unit.png")
	if err := imageio.WriteNearestNeighborContactSheet(allFrames, unit.Artifacts.CompleteUnitSheetPath, 1); err != nil {
		return err
	}
	masterFrames := make([]string, len(plan.Directions))
	for index, direction := range plan.Directions {
		masterFrames[index] = filepath.Join(unitDir, "review", "master-directions", direction.ID+".png")
	}
	if _, err := imageio.WriteRegisteredSemanticPoses(
		master.Artifacts.RecoveredPosePaths,
		master.Poses,
		masterFrames,
		frameSize.Width,
		frameSize.Height,
		palette,
		transform,
		1,
	); err != nil {
		master.Status = StatusRejected
		master.HardRejections = []string{"unsafe_character_master_frame: " + err.Error()}
		return rejectAnimatedAssembly(
			opts,
			manifest,
			unit,
			master.HardRejections[0],
		)
	}
	unit.Artifacts.NativePreviewPath = masterFrames[0]
	unit.Artifacts.PortraitPreviewPath = filepath.Join(unitDir, "review", "portrait-96.png")
	neutralPNG, err := os.ReadFile(unit.Artifacts.NativePreviewPath)
	if err != nil {
		return err
	}
	if _, err := imageio.WriteIsolatedReviewPreviewPNG(
		unit.Artifacts.PortraitPreviewPath,
		neutralPNG,
		96,
		96,
		palette,
		imageio.SubjectRegistrationGrounded,
	); err != nil {
		return err
	}
	comparisonColumns := 1
	for _, animation := range plan.Animations {
		comparisonColumns += len(animation.Frames)
	}
	comparisonFrames := make([]string, 0, comparisonColumns*len(plan.Directions))
	comparisonColumnLabels := []string{"MASTER"}
	for _, animation := range plan.Animations {
		for _, frame := range animation.Frames {
			comparisonColumnLabels = append(comparisonColumnLabels, strings.ToUpper(animation.ID)+" "+frame.ID)
		}
	}
	comparisonRowLabels := make([]string, 0, len(plan.Directions))
	for directionIndex := range plan.Directions {
		comparisonRowLabels = append(comparisonRowLabels, strings.ToUpper(plan.Directions[directionIndex].ID))
		comparisonFrames = append(comparisonFrames, masterFrames[directionIndex])
		for _, animation := range plan.Animations {
			start := directionIndex * len(animation.Frames)
			comparisonFrames = append(comparisonFrames, animationFrames[animation.ID][start:start+len(animation.Frames)]...)
		}
	}
	unit.Artifacts.IdentityComparisonPath = filepath.Join(unitDir, "review", "master-to-animation.png")
	if err := imageio.WriteLabeledNearestNeighborContactGrid(
		comparisonFrames,
		unit.Artifacts.IdentityComparisonPath,
		comparisonColumns,
		1,
		comparisonColumnLabels,
		comparisonRowLabels,
	); err != nil {
		return err
	}
	unit.Artifacts.FramePaths = allFrames
	unit.AssemblyVersion = AnimatedAssemblyVersion
	unit.Status = StatusAwaitingReview
	unit.Review = nil
	unit.Deploy = nil
	if err := writeQA(unitDir, StatusAwaitingReview, "Review the complete unit, every animation/direction GIF, the canonical master, and identity consistency before acceptance."); err != nil {
		return err
	}
	unit.Artifacts.QAPath = filepath.Join(unitDir, "qa.md")
	for _, targetID := range unit.TargetIDs {
		manifest.Targets[targetID].Artifacts = unit.Artifacts
	}
	return Save(opts.OutputDir, opts.RunID, manifest)
}

func rebuildCanonicalSubjectProfile(
	opts Options,
	unit *UnitState,
	plan animatedUnitPlan,
) error {
	referencePaths := make([]string, len(plan.Directions))
	for index, direction := range plan.Directions {
		referencePaths[index] = direction.ReferencePath
	}
	profile, err := imageio.BuildCanonicalSubjectProfile(
		referencePaths,
		imageio.SubjectRegistrationMode(plan.RegistrationMode),
		imageio.CanonicalScaleClass(plan.ScaleClass),
		plan.Size.Height,
	)
	if err != nil {
		return err
	}
	if unit.Profile != nil {
		if len(unit.Profile.ReferenceHashes) != len(profile.ReferenceHashes) {
			return fmt.Errorf("canonical profile reference lineage is incomplete")
		}
		for index := range profile.ReferenceHashes {
			if unit.Profile.ReferenceHashes[index] != profile.ReferenceHashes[index] {
				return fmt.Errorf(
					"direction %s reference changed after generation",
					plan.Directions[index].ID,
				)
			}
		}
	}
	dir := filepath.Join(
		runRoot(opts),
		"intermediates",
		plan.ObjectID,
		"character-master",
	)
	profilePath := filepath.Join(dir, "canonical-profile.json")
	if err := imageio.WriteCanonicalSubjectProfile(profilePath, profile); err != nil {
		return err
	}
	overlayPath := filepath.Join(dir, "canonical-profile-overlay.png")
	if err := imageio.WriteCanonicalSubjectProfileOverlay(
		referencePaths,
		overlayPath,
		profile,
	); err != nil {
		return err
	}
	unit.Profile = &profile
	unit.Artifacts.CanonicalProfilePath = profilePath
	unit.Artifacts.CanonicalProfileOverlayPath = overlayPath
	return nil
}

func rejectAnimatedAssembly(
	opts Options,
	manifest *Manifest,
	unit *UnitState,
	reason string,
) error {
	unit.Status = StatusRejected
	unit.AssemblyVersion = AnimatedAssemblyVersion
	unit.HardRejections = []string{reason}
	unit.Review = nil
	unit.Deploy = nil
	unit.Artifacts.CompleteUnitSheetPath = ""
	unit.Artifacts.IdentityComparisonPath = ""
	unit.Artifacts.AnimationGIFPaths = nil
	unit.Artifacts.FramePaths = nil
	unitDir := filepath.Join(runRoot(opts), "units", unit.ObjectID)
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return err
	}
	if err := writeQA(unitDir, StatusRejected, reason); err != nil {
		return err
	}
	unit.Artifacts.QAPath = filepath.Join(unitDir, "qa.md")
	for _, targetID := range unit.TargetIDs {
		target := manifest.Targets[targetID]
		if target == nil {
			continue
		}
		target.Status = StatusRejected
		target.NormalizedPath = ""
		target.ProductionEligible = false
		target.Normalization = nil
		target.HardRejections = []string{reason}
		target.Review = nil
		target.Deploy = nil
		target.Artifacts = ReviewArtifacts{}
	}
	return Save(opts.OutputDir, opts.RunID, manifest)
}
