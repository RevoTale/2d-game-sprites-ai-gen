package generate

import (
	"path/filepath"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
)

func assembleAnimatedUnit(opts Options, manifest *Manifest, plan animatedUnitPlan) error {
	unit := manifest.Units[plan.ID]
	master := manifest.Intermediates[plan.MasterID]
	paletteSources := append(
		[]string(nil),
		master.Artifacts.RecoveredPosePaths...,
	)
	allPoses := append([]imageio.SemanticPose(nil), master.Poses...)
	for _, animation := range plan.Animations {
		board := manifest.Intermediates[animation.BoardID]
		paletteSources = append(
			paletteSources,
			board.Artifacts.RecoveredPosePaths...,
		)
		allPoses = append(allPoses, board.Poses...)
	}
	palette, err := imageio.SharedPaletteFromPNGs(paletteSources, 32)
	if err != nil {
		return err
	}
	if len(plan.Animations) == 0 || len(plan.Animations[0].Targets) == 0 {
		return nil
	}
	frameSize := plan.Animations[0].Targets[0].Size
	transform, err := imageio.FitSemanticUnitTransform(
		allPoses,
		frameSize.Width,
		frameSize.Height,
	)
	if err != nil {
		unit.Status = StatusRejected
		unit.HardRejections = []string{
			"unsafe_shared_unit_transform: " + err.Error(),
		}
		return Save(opts.OutputDir, opts.RunID, manifest)
	}
	unit.HardRejections = nil
	unit.Transform = &transform
	unitDir := filepath.Join(runRoot(opts), "units", plan.ObjectID)
	if err := imageio.WritePalette(filepath.Join(unitDir, "palette.json"), palette); err != nil {
		return err
	}
	unit.Artifacts = ReviewArtifacts{
		CurrentReferenceSheetPath: master.Artifacts.CurrentReferenceSheetPath,
		MasterSheetPath:           master.NormalizedPath,
	}
	unit.MasterLineage = master.Lineage
	unit.AnimationLineages = map[string]string{}
	var allFrames []string
	animationFrames := make(map[string][]string, len(plan.Animations))
	for _, animation := range plan.Animations {
		board := manifest.Intermediates[animation.BoardID]
		outputs := make([]string, len(animation.Targets))
		for index, target := range animation.Targets {
			outputs[index] = filepath.Join(TargetDir(opts.OutputDir, opts.RunID, target.ID), "normalized.png")
		}
		transforms, normalizeErr := imageio.WriteRegisteredSemanticPoses(
			board.Artifacts.RecoveredPosePaths,
			board.Poses,
			outputs,
			animation.Targets[0].Size.Width,
			animation.Targets[0].Size.Height,
			palette,
			transform,
		)
		if normalizeErr != nil {
			board.Status = StatusRejected
			board.HardRejections = []string{
				"unsafe_production_frame: " + normalizeErr.Error(),
			}
			return Save(opts.OutputDir, opts.RunID, manifest)
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
			state.CapabilityMode = "v9-semantic-board"
			state.Palette = palette
			state.Normalization = &NormalizationRecord{
				ScaleAlgorithm: "unit-wide-body-pivot-area",
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
	); err != nil {
		master.Status = StatusRejected
		master.HardRejections = []string{"unsafe_character_master_frame: " + err.Error()}
		return Save(opts.OutputDir, opts.RunID, manifest)
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
