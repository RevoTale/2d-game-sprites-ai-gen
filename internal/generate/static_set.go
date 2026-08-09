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
	staticSetMarkerInset           = 32
	staticSetMinimumProviderExtent = 256
	staticSetScaleAlgorithm        = "shared-static-set-alpha-fit-v1"
	staticSetOverlayProviderGuard  = 8
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
	layoutSizes := staticSetProviderLayoutSizes(group)
	opaqueTiles := group[0].RenderMode == pack.RenderModeOpaqueTile
	materialSwatches := group[0].RenderMode == pack.RenderModeMaterialSwatch
	transparentOverlay := group[0].RenderMode == pack.RenderModeTransparentOverlay
	markerInset := staticSetMarkerInset
	maskGuard := -1
	if opaqueTiles || materialSwatches {
		markerInset = 0
		maskGuard = 0
	} else if transparentOverlay {
		markerInset = staticSetOverlayProviderGuard
		maskGuard = staticSetOverlayProviderGuard
	} else {
		for _, size := range layoutSizes {
			markerInset = min(markerInset, min(size.X, size.Y)/4)
		}
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
		markerInset,
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
	if err := imageio.WriteSemanticSizedEditMaskWithGuard(
		maskPath,
		*state.SemanticLayout,
		layoutSizes,
		maskGuard,
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
	ownershipMaskPath := ""
	if opaqueTiles || materialSwatches {
		ownershipMaskPath = maskPath
	}
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
		ownershipMaskPath,
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
	calibration := imageio.StaticSetScaleCalibration{}
	if transparentOverlay {
		calibration, err = imageio.WriteCanvasRegisteredTransparentStaticSet(
			state.NormalizedPath,
			*state.SemanticLayout,
			layoutSizes,
			partSpecs,
			sharedPalette,
		)
	} else if opaqueTiles || materialSwatches {
		calibration, err = imageio.WriteFullBleedOpaqueStaticSet(
			partSpecs,
			sharedPalette,
		)
	} else {
		calibration, err = imageio.WriteSharedScaleTransparentStaticSet(
			partSpecs,
			sharedPalette,
		)
	}
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
	if opaqueTiles || materialSwatches {
		for _, part := range partSpecs {
			evidence, evidenceErr := imageio.MeasureStaticEvidence(part.OutputPath)
			if evidenceErr != nil {
				return fmt.Errorf("measure opaque static set part %q: %w", part.ID, evidenceErr)
			}
			if evidence.OpaqueRatio != 1 {
				state.HardRejections = append(state.HardRejections, part.ID+":opaque_tile_has_transparency")
			}
			if opaqueTiles &&
				(max(evidence.HorizontalEdgeDelta, evidence.VerticalEdgeDelta) > opaqueTileHardMeanEdgeDelta ||
					max(evidence.MaximumHorizontalEdgeDelta, evidence.MaximumVerticalEdgeDelta) > opaqueTileHardMaximumEdgeDelta) {
				state.HardRejections = append(state.HardRejections, part.ID+":opaque_tile_severe_edge_mismatch")
			} else if opaqueTiles && max(evidence.HorizontalEdgeDelta, evidence.VerticalEdgeDelta) > opaqueTileWarningMeanEdgeDelta {
				state.Warnings = append(state.Warnings, part.ID+":opaque_tile_edge_mismatch_needs_repeat_review")
			}
			if evidence.SmallClusterRatio > opaqueTileWarningSmallClusterRatio {
				state.Warnings = append(state.Warnings, part.ID+":opaque_tile_micro_clusters_need_noise_review")
			}
		}
		if len(state.HardRejections) != 0 {
			state.Status = StatusRejected
		}
	}
	state.StaticSetScale = &calibration
	if calibration.Scale < 1 && !transparentOverlay && !opaqueTiles && !materialSwatches {
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
	scaleAlgorithm := staticSetScaleAlgorithm
	if transparentOverlay {
		scaleAlgorithm = "canvas-registered-transparent-overlay-v1"
	} else if opaqueTiles {
		scaleAlgorithm = "full-bleed-opaque-static-set-v1"
	} else if materialSwatches {
		scaleAlgorithm = "full-bleed-material-swatch-v1"
	}
	for index, target := range group {
		targetDir := filepath.Join(runRoot(opts), "targets", target.ID)
		recordNormalizedStaticSetPart(
			target,
			manifest.Targets[target.ID],
			state,
			partSpecs[index].OutputPath,
			sharedPalette,
			calibration.Scale,
			scaleAlgorithm,
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
	if len(nativePaths) == len(group) {
		reviewDir := filepath.Join(dir, "review")
		if state.Status != StatusRejected {
			state.Artifacts.RuntimeOverrideRoot = filepath.Join(reviewDir, "runtime-overrides")
			if err := writeStaticSetRuntimeOverrides(
				state.Artifacts.RuntimeOverrideRoot,
				group,
				manifest,
			); err != nil {
				return err
			}
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
		if opaqueTiles || materialSwatches {
			if err := writeMaterialSetReviewArtifacts(reviewDir, group, logicalPaths, materialSwatches); err != nil {
				return err
			}
		}
	}
	if state.Status == StatusRejected {
		if err := writeQA(
			dir,
			StatusRejected,
			"Static set failed structural overlay validation: "+strings.Join(state.HardRejections, ", ")+".",
		); err != nil {
			return err
		}
		state.Artifacts.QAPath = filepath.Join(dir, "qa.md")
		for _, target := range group {
			targetState := manifest.Targets[target.ID]
			targetState.Status = StatusRejected
			targetState.ProductionEligible = false
			targetState.HardRejections = append([]string(nil), state.HardRejections...)
		}
	} else {
		reason := "Static set passed structural validation with one shared material scale. Manual visual review is still required."
		if transparentOverlay {
			reason = "Transparent overlay set preserved fixed canvas topology and passed structural alpha validation. Manual visual review is still required."
		} else if opaqueTiles {
			reason = "Opaque tile set fills every production canvas and passed structural validation. Manual seam and material review is still required."
		} else if materialSwatches {
			reason = "Material swatch set fills every production canvas and passed structural validation. Manual material and mirrored-repeat review is still required."
		} else if calibration.Scale < 1 {
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

// staticSetProviderLayoutSizes preserves every part's relative extent while
// enlarging an entirely small set enough for reliable structured composition.
// GPT Image masks guide edits but do not guarantee exact placement, and tiny
// cells are especially prone to being regrouped. Normalization still applies
// one shared downscale to the configured production canvases afterward.
func staticSetProviderLayoutSizes(group []targets.Target) []image.Point {
	sizes := make([]image.Point, len(group))
	maximumExtent := 0
	for index, target := range group {
		sizes[index] = image.Pt(target.Size.Width*2, target.Size.Height*2)
		maximumExtent = max(maximumExtent, sizes[index].X, sizes[index].Y)
	}
	scale := 1
	if maximumExtent > 0 && maximumExtent < staticSetMinimumProviderExtent {
		scale = (staticSetMinimumProviderExtent + maximumExtent - 1) / maximumExtent
	}
	if scale > 1 {
		for index := range sizes {
			sizes[index].X *= scale
			sizes[index].Y *= scale
		}
	}
	return sizes
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
	if shared.RenderMode == pack.RenderModeTransparentOverlay {
		builder.WriteString("Return one coherent ordered set of canvas-registered transparent overlay sprites on one removable flat chroma background. ")
		fmt.Fprintf(&builder, "The layout contains exactly %d editable parts and no trailing cells; return exactly %d parts and never add another part anywhere on the canvas. ", len(group), len(group))
		builder.WriteString("Input 01 contains neutral placement rectangles only; the marker color is not artwork and must not survive in the result. ")
		builder.WriteString("Replace each rectangle with its described transparent overlay while preserving canvas registration. ")
		builder.WriteString("Keep one uninterrupted chroma perimeter around all four edges of every part; foreground must never touch a part edge. ")
		builder.WriteString("Keep one shared material scale, projection, lighting, and palette across every part. Do not draw labels, guides, borders, scenery, or shadows.\n\n")
	} else if shared.RenderMode == pack.RenderModeOpaqueTile {
		builder.WriteString("Return one coherent ordered set of opaque seamless material tiles on one removable flat chroma background. ")
		builder.WriteString("At every ordered anchor, replace the complete neutral rectangle and fill its complete declared tile rectangle. ")
		builder.WriteString("Every tile must be opaque through all four edges, tile seamlessly on both axes, and preserve one shared palette, material scale, and average brightness. ")
		builder.WriteString("Keep each tile inside its separate editable rectangle and keep the chroma corridors between rectangles unchanged. ")
		builder.WriteString("Follow each ordered part description while preserving the shared material. If the parts are animation phases, vary only the explicitly described restrained details and keep coverage and average brightness stable. ")
		builder.WriteString("Do not draw labels, guides, borders, scenery, or shadows.\n\n")
	} else if shared.RenderMode == pack.RenderModeMaterialSwatch {
		builder.WriteString("Return one coherent ordered set of full-bleed opaque material swatches on one removable flat chroma background. ")
		fmt.Fprintf(&builder, "The layout contains exactly %d separate editable rectangles; return exactly %d swatches. ", len(group), len(group))
		builder.WriteString("Full-bleed means filling each editable rectangle, never filling the complete provider canvas. ")
		builder.WriteString("At every ordered anchor, replace the complete neutral rectangle with the requested flat material sample. ")
		builder.WriteString("Fill every swatch completely; its outer silhouette is only a source crop and must not depict a shoreline, bank, corner, border, frame, or map composition. ")
		builder.WriteString("Keep every non-editable corridor and all outer canvas reserve as one unchanged flat chroma background. ")
		builder.WriteString("Preserve one shared palette, material scale, projection, and lighting. Do not draw labels, guides, scenery, or shadows.\n\n")
	} else {
		builder.WriteString("Return one coherent set of isolated sprites on one removable flat chroma background. ")
		builder.WriteString("Place exactly one complete subject at each ordered logical anchor. ")
		builder.WriteString("Input 01 is the system layout: replace every neutral gray placeholder completely with the corresponding ordered part, preserving its location and relative extent. ")
		builder.WriteString("The CLI mask exposes one guarded region around each placeholder; keep the complete corresponding subject inside that region with visible chroma reserve at every editable edge. ")
		builder.WriteString("Never merge, touch, or connect neighboring subjects; preserve a wide uninterrupted chroma gap around every part. ")
		builder.WriteString("Keep material scale, construction, projection, lighting, and palette consistent across every part. ")
		builder.WriteString("Anchors establish order and ownership, not visible boxes. Do not draw labels, guides, borders, scenery, or shadows.\n\n")
	}
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

func writeMaterialSetReviewArtifacts(
	reviewDir string,
	group []targets.Target,
	logicalPaths []string,
	mirrored bool,
) error {
	if len(group) != len(logicalPaths) {
		return fmt.Errorf("opaque tile review requires one path per set part")
	}
	repeatDir := filepath.Join(reviewDir, "repeats")
	for index, target := range group {
		preview := imageio.WriteTiledRepeatPreview
		if mirrored {
			preview = imageio.WriteMirroredRepeatPreview
		}
		if err := preview(logicalPaths[index], filepath.Join(repeatDir, target.SetPartID+"-3x3.png"), 3, 3); err != nil {
			return fmt.Errorf("write repeat preview for %q: %w", target.SetPartID, err)
		}
	}
	if err := imageio.WriteLoopingGIF(
		logicalPaths,
		filepath.Join(reviewDir, "loop.gif"),
		200,
	); err != nil {
		return fmt.Errorf("write opaque tile set loop: %w", err)
	}
	return nil
}

func recordNormalizedStaticSetPart(
	target targets.Target,
	state *TargetState,
	set *IntermediateState,
	normalizedPath string,
	sharedPalette []imageio.PaletteColor,
	sharedScale float64,
	scaleAlgorithm string,
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
		ScaleAlgorithm: scaleAlgorithm,
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
