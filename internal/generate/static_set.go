package generate

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

const (
	staticSetMarkerInset    = 32
	staticSetScaleAlgorithm = "shared-static-set-alpha-fit-v1"
)

func generateStaticSet(
	ctx context.Context,
	gen provider.Provider,
	opts Options,
	manifest *Manifest,
	group []targets.Target,
) error {
	if len(group) == 0 {
		return fmt.Errorf("static set requires at least one target")
	}
	objectID := group[0].ObjectID
	setID := pack.KindStaticSet + ":" + objectID
	layoutSizes := make([]image.Point, len(group))
	for index, target := range group {
		layoutSizes[index] = image.Pt(target.Size.Width*2, target.Size.Height*2)
	}
	state := manifest.Intermediates[setID]
	if state == nil {
		layout, err := imageio.SemanticStaticSetLayout(layoutSizes)
		if err != nil {
			return err
		}
		state = &IntermediateState{
			ID:             setID,
			Kind:           pack.KindStaticSet,
			Status:         StatusPending,
			ObjectID:       objectID,
			TargetIDs:      targetIDs(group),
			SemanticLayout: &layout,
		}
		manifest.Intermediates[setID] = state
	}
	dir := filepath.Join(runRoot(opts), "static-sets", objectID)
	layoutSource := filepath.Join(dir, "layout-source.png")
	if err := imageio.WriteSemanticSizedPlaceholderBoard(
		layoutSource,
		*state.SemanticLayout,
		layoutSizes,
		staticSetMarkerInset,
	); err != nil {
		return err
	}
	providerLayout := filepath.Join(dir, "provider", "layout-source.png")
	if _, err := imageio.WriteOpaqueChromaCopies(
		[]string{layoutSource},
		[]string{providerLayout},
	); err != nil {
		return err
	}
	state.EditSourcePath = layoutSource
	maskPath := filepath.Join(dir, "edit-mask.png")
	if err := imageio.WriteSemanticSizedEditMask(
		maskPath,
		*state.SemanticLayout,
		layoutSizes,
	); err != nil {
		return err
	}
	state.EditMaskPath = maskPath
	shared := group[0]
	shared.SetPartID = ""
	shared.SetPartRole = ""
	shared.SetPartDesc = ""
	prompt := staticSetPrompt(shared, group)
	inputs := []conditioning.Input{{
		ID: "static-set-layout", Role: conditioning.RolePose,
		Authority: "cli-protocol", SourcePath: layoutSource, Path: providerLayout,
		Description: "Ordered neutral placement silhouettes; replace each marker completely with its corresponding part and keep every subject separated.",
		Required:    true,
	}}
	inputs = append(inputs, filterInputs(
		shared.Inputs,
		conditioning.RoleStyle,
		conditioning.RoleIdentity,
	)...)
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
		maskPath,
		pack.KindStaticSet,
		state.SemanticLayout.Canvas(),
	); err != nil {
		return err
	}
	if state.Status == StatusRejected {
		for _, target := range group {
			targetState := manifest.Targets[target.ID]
			targetState.Status = StatusRejected
			targetState.ProductionEligible = false
			targetState.Dependencies = []string{setID}
			targetState.HardRejections = append([]string(nil), state.HardRejections...)
		}
		return Save(opts.OutputDir, opts.RunID, manifest)
	}
	if len(state.Artifacts.RecoveredPosePaths) != len(group) {
		return fmt.Errorf(
			"static set %q recovered %d parts, expected %d",
			objectID,
			len(state.Artifacts.RecoveredPosePaths),
			len(group),
		)
	}
	sharedPalette, err := imageio.SharedPaletteFromPNGs(
		state.Artifacts.RecoveredPosePaths,
		32,
	)
	if err != nil {
		return fmt.Errorf("derive static set shared palette: %w", err)
	}
	partSpecs := make([]imageio.StaticSetPart, 0, len(group))
	for index, target := range group {
		registration := imageio.SubjectRegistrationCentered
		if target.RegistrationMode == "grounded" {
			registration = imageio.SubjectRegistrationGrounded
		}
		partSpecs = append(partSpecs, imageio.StaticSetPart{
			ID:           target.ID,
			SourcePath:   state.Artifacts.RecoveredPosePaths[index],
			OutputPath:   filepath.Join(runRoot(opts), "targets", target.ID, "normalized.png"),
			Size:         image.Pt(target.Size.Width*2, target.Size.Height*2),
			Registration: registration,
		})
	}
	calibration, err := imageio.WriteSharedScaleTransparentStaticSet(
		partSpecs,
		sharedPalette,
	)
	if err != nil {
		state.Status = StatusRejected
		state.StaticSetScale = nil
		state.HardRejections = []string{fmt.Sprintf("static set shared normalization: %v", err)}
		if qaErr := writeQA(dir, StatusRejected, state.HardRejections[0]); qaErr != nil {
			return qaErr
		}
		state.Artifacts.QAPath = filepath.Join(dir, "qa.md")
		for _, target := range group {
			targetState := manifest.Targets[target.ID]
			targetState.Status = StatusRejected
			targetState.ProductionEligible = false
			targetState.HardRejections = append([]string(nil), state.HardRejections...)
		}
		return Save(opts.OutputDir, opts.RunID, manifest)
	}
	state.StaticSetScale = &calibration
	if calibration.Scale < 1 {
		state.Warnings = append(
			state.Warnings,
			fmt.Sprintf(
				"shared static-set scale %.6f is limited by %s %s; inspect every part at native and logical size",
				calibration.Scale,
				calibration.LimitingPartID,
				calibration.LimitingAxis,
			),
		)
	}
	nativePaths := make([]string, 0, len(group))
	logicalPaths := make([]string, 0, len(group))
	for index, target := range group {
		targetDir := filepath.Join(runRoot(opts), "targets", target.ID)
		recordNormalizedStaticSetPart(
			target,
			manifest.Targets[target.ID],
			state,
			partSpecs[index].OutputPath,
			sharedPalette,
			calibration.Scale,
		)
		targetState := manifest.Targets[target.ID]
		if targetState.Status == StatusAwaitingReview {
			logicalPath := filepath.Join(targetDir, "review", "logical-preview.png")
			if err := imageio.WriteDensityReducedPNG(
				targetState.NormalizedPath,
				logicalPath,
				targetState.SourceDensity,
			); err != nil {
				state.Status = StatusRejected
				state.HardRejections = append(state.HardRejections, fmt.Sprintf("%s: %v", target.ID, err))
				continue
			}
			targetState.Artifacts.BattlefieldPreviewPath = logicalPath
			nativePaths = append(nativePaths, targetState.NormalizedPath)
			logicalPaths = append(logicalPaths, logicalPath)
		}
	}
	if state.Status != StatusRejected {
		reviewDir := filepath.Join(dir, "review")
		state.Artifacts.RuntimeOverrideRoot = filepath.Join(reviewDir, "runtime-overrides")
		if err := writeStaticSetRuntimeOverrides(
			state.Artifacts.RuntimeOverrideRoot,
			group,
			manifest,
		); err != nil {
			return err
		}
		state.Artifacts.MasterSheetPath = filepath.Join(reviewDir, "native-parts.png")
		state.Artifacts.ContactSheetPath = filepath.Join(reviewDir, "logical-parts.png")
		if err := imageio.WriteRecoveredPoseSheet(
			nativePaths,
			state.SemanticLayout.Columns,
			state.Artifacts.MasterSheetPath,
		); err != nil {
			return err
		}
		if err := imageio.WriteRecoveredPoseSheet(
			logicalPaths,
			state.SemanticLayout.Columns,
			state.Artifacts.ContactSheetPath,
		); err != nil {
			return err
		}
	}
	if state.Status == StatusRejected {
		for _, target := range group {
			targetState := manifest.Targets[target.ID]
			targetState.Status = StatusRejected
			targetState.ProductionEligible = false
			targetState.HardRejections = append([]string(nil), state.HardRejections...)
		}
	} else {
		reason := "Static set passed structural validation with one shared material scale. Manual visual review is still required."
		if calibration.Scale < 1 {
			reason = fmt.Sprintf(
				"Static set passed structural validation after one shared %.6f scale limited by %s %s. Manual visual review is still required.",
				calibration.Scale,
				calibration.LimitingPartID,
				calibration.LimitingAxis,
			)
		}
		if err := writeQA(dir, StatusAwaitingReview, reason); err != nil {
			return err
		}
		state.Artifacts.QAPath = filepath.Join(dir, "qa.md")
	}
	return Save(opts.OutputDir, opts.RunID, manifest)
}

func writeStaticSetRuntimeOverrides(
	root string,
	group []targets.Target,
	manifest *Manifest,
) error {
	temporary := root + ".tmp"
	if err := os.RemoveAll(temporary); err != nil {
		return fmt.Errorf("clean static set runtime overrides: %w", err)
	}
	if err := os.MkdirAll(temporary, 0o755); err != nil {
		return fmt.Errorf("create static set runtime overrides: %w", err)
	}
	for _, target := range group {
		state := manifest.Targets[target.ID]
		if state == nil || state.NormalizedPath == "" {
			return fmt.Errorf("static set target %q has no normalized runtime override", target.ID)
		}
		if err := imageio.CopyFile(
			state.NormalizedPath,
			filepath.Join(temporary, target.ID+".png"),
		); err != nil {
			return fmt.Errorf("copy static set runtime override %q: %w", target.ID, err)
		}
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("replace static set runtime overrides: %w", err)
	}
	if err := os.Rename(temporary, root); err != nil {
		return fmt.Errorf("publish static set runtime overrides: %w", err)
	}
	return nil
}

func staticSetPrompt(shared targets.Target, group []targets.Target) string {
	var builder strings.Builder
	builder.WriteString("# CLI Protocol\n")
	builder.WriteString("Return one coherent set of isolated sprites on one removable flat chroma background. ")
	builder.WriteString("Place exactly one complete subject at each ordered logical anchor. ")
	builder.WriteString("Input 01 is the system layout: replace every neutral gray placeholder completely with the corresponding ordered part, preserving its location and relative extent. ")
	builder.WriteString("The CLI mask exposes one guarded region around each placeholder; keep the complete corresponding subject inside that region with visible chroma reserve at every editable edge. ")
	builder.WriteString("Never merge, touch, or connect neighboring subjects; preserve a wide uninterrupted chroma gap around every part. ")
	builder.WriteString("Keep material scale, construction, projection, lighting, and palette consistent across every part. ")
	builder.WriteString("Anchors establish order and ownership, not visible boxes. Do not draw labels, guides, borders, scenery, or shadows.\n\n")
	builder.WriteString(targets.BuildPrompt(shared))
	builder.WriteString("\n# Ordered Set Parts\n")
	for index, target := range group {
		fmt.Fprintf(
			&builder,
			"%02d. %s — %s. %s\n",
			index+1,
			target.SetPartID,
			target.SetPartRole,
			target.SetPartDesc,
		)
	}
	builder.WriteString("\n")
	builder.WriteString(removableBackgroundInstruction)
	builder.WriteString("\n")
	return builder.String()
}

func recordNormalizedStaticSetPart(
	target targets.Target,
	state *TargetState,
	set *IntermediateState,
	normalizedPath string,
	sharedPalette []imageio.PaletteColor,
	sharedScale float64,
) {
	state.Status = StatusAwaitingReview
	state.NormalizedPath = normalizedPath
	state.Dependencies = []string{set.ID}
	state.SourceCandidate = set.Lineage
	state.CapabilityMode = pack.KindStaticSet
	state.ProductionEligible = true
	state.Palette = append([]imageio.PaletteColor(nil), sharedPalette...)
	state.LogicalSize = target.Size
	state.IntrinsicSize = pack.Size{
		Width: target.Size.Width * 2, Height: target.Size.Height * 2,
	}
	state.SourceDensity = 2
	state.Warnings = append([]string(nil), set.Warnings...)
	state.Normalization = &NormalizationRecord{
		ScaleAlgorithm: staticSetScaleAlgorithm,
		PaletteMethod:  "shared-set-deterministic-median-cut",
		MaximumColors:  32,
		ColorSpace:     "linear-srgb",
		Dithering:      false,
		AlphaThreshold: 128,
		Anchor:         target.RegistrationMode,
		Scale:          sharedScale,
	}
	state.Artifacts.PromptPath = set.Artifacts.PromptPath
	state.Artifacts.EvidencePath = set.Artifacts.EvidencePath
	state.Artifacts.OwnershipOverlayPath = set.Artifacts.OwnershipOverlayPath
	state.Artifacts.NativePreviewPath = normalizedPath
}
