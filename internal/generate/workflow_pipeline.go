package generate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

func preflight(plan workflowPlan, capabilities provider.Capabilities, deployDir string) error {
	if len(plan.Rows) != 0 && (!capabilities.References || !capabilities.Masks) {
		return errors.New("animated row generation requires provider reference and mask support")
	}
	if capabilities.References {
		return nil
	}
	for _, target := range plan.StaticTargets {
		if len(target.Inputs) != 0 {
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

func runRoot(opts Options) string {
	return filepath.Join(opts.OutputDir, "runs", opts.RunID)
}

func runWorkflow(ctx context.Context, selected []targets.Target, plan workflowPlan, gen provider.Provider, opts Options, manifest *Manifest) (Result, error) {
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

	for _, seed := range plan.Seeds {
		if err := generateSeedBoard(ctx, gen, opts, manifest, seed, opts.Force); err != nil {
			return result, err
		}
	}
	for _, row := range plan.Rows {
		seed := manifest.Intermediates[row.SeedID]
		if seed == nil || seed.Status != StatusAccepted || seed.Extracted[row.VariantKey] == "" {
			result.AwaitingReview += len(row.Targets)
			continue
		}
		forceRow := opts.Force
		correctedFrame := opts.Filter.Frame
		generated, err := generateAnimationRow(ctx, gen, opts, manifest, row, forceRow, correctedFrame)
		if err != nil {
			return result, err
		}
		if generated {
			result.Generated += len(row.Targets)
		} else {
			result.Skipped += len(row.Targets)
		}
	}
	if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return result, err
	}
	opts.report(ProgressEvent{Stage: ProgressRunCompleted, RunID: opts.RunID, Total: len(selected)})
	return result, nil
}

func generateSeedBoard(ctx context.Context, gen provider.Provider, opts Options, manifest *Manifest, plan seedPlan, force bool) error {
	state := manifest.Intermediates[plan.ID]
	if state != nil && state.Status == StatusAccepted && state.NormalizedPath != "" {
		if _, err := os.Stat(state.NormalizedPath); err == nil {
			return nil
		}
	}
	if state != nil {
		if state.Status == StatusAwaitingReview {
			return nil
		}
		if state.Status == StatusRejected && !force {
			return nil
		}
	}
	if state == nil {
		state = &IntermediateState{ID: plan.ID, Kind: directionSeedBoardKind, ObjectID: plan.ObjectID, TargetIDs: plan.TargetIDs}
		manifest.Intermediates[plan.ID] = state
	}
	if force {
		state.Review = nil
		state.Extracted = nil
		state.ExtractedPalettes = nil
		invalidateSeedDependents(manifest, plan.ID)
	}
	layout, err := imageio.CanvasGridLayout(len(plan.PosePaths), 4, providerCanvasSize)
	if err != nil {
		return err
	}
	dir := filepath.Join(runRoot(opts), "intermediates", plan.ObjectID, "direction-seeds")
	sourcePath := filepath.Join(dir, "source-board.png")
	if err := imageio.WriteCanvasBoard(plan.PosePaths, sourcePath, layout, max(8, layout.CellWidth/32)); err != nil {
		return err
	}
	maskPath := filepath.Join(dir, "attempts", nextAttemptID(state.Attempts, force, seedCandidateCount), "mask.png")
	if err := imageio.WriteBoardEditMask(maskPath, layout, -1); err != nil {
		return err
	}
	inputs := []conditioning.Input{{Role: conditioning.RolePose, Path: sourcePath, Description: "Authoritative combined directional source board and fixed cell layout.", Required: true}}
	for _, reference := range plan.References {
		inputs = append(inputs, filterInputs(reference.Inputs, conditioning.RoleStyle, conditioning.RoleIdentity)...)
	}
	inputs = uniqueConditioningInputs(inputs)
	inputs = append(inputs, conditioning.Input{Role: conditioning.RoleMask, Path: maskPath, Description: "Editable directional subject cells; gutters and trailing cells remain protected.", Required: true})
	state.Layout = &layout
	state.ChromaKey = "opaque edge-connected background"
	prompt := seedBoardPrompt(plan, layout)
	return generateBoardCandidates(ctx, gen, opts, manifest, state, dir, prompt, sourcePath, inputs, seedCandidateCount, force, "seed")
}

func generateAnimationRow(ctx context.Context, gen provider.Provider, opts Options, manifest *Manifest, plan rowPlan, force bool, correctedFrame string) (bool, error) {
	state := manifest.Intermediates[plan.ID]
	if state != nil && state.Status == StatusRejected && !force {
		return false, nil
	}
	if state != nil && correctedFrame == "" && !force {
		switch state.Status {
		case StatusAccepted, StatusRejected:
			return false, nil
		case StatusAwaitingReview:
			if _, err := os.Stat(state.NormalizedPath); err == nil {
				return false, nil
			}
		}
	}
	if state != nil && correctedFrame == "" && force && (state.Status == StatusAccepted || state.Status == StatusAwaitingReview) {
		return false, nil
	}
	seed := manifest.Intermediates[plan.SeedID]
	if seed == nil || seed.Status != StatusAccepted {
		return false, fmt.Errorf("row %q requires an accepted directional seed board", plan.ID)
	}
	seedPath := seed.Extracted[plan.VariantKey]
	if seedPath == "" {
		return false, fmt.Errorf("row %q has no extracted directional seed %q", plan.ID, plan.VariantKey)
	}
	if state == nil {
		state = &IntermediateState{ID: plan.ID, Kind: animationRowKind, ObjectID: plan.ObjectID, AnimationID: plan.AnimationID, VariantKey: plan.VariantKey, TargetIDs: targetIDs(plan.Targets), Dependencies: []string{plan.SeedID}}
		manifest.Intermediates[plan.ID] = state
	}
	layout, err := imageio.CanvasGridLayout(len(plan.Targets), 4, providerCanvasSize)
	if err != nil {
		return false, err
	}
	dir := filepath.Join(runRoot(opts), "intermediates", plan.ObjectID, "animations", plan.AnimationID, plan.VariantKey, "row")
	poseBoardPath := filepath.Join(dir, "pose-board.png")
	alignedPosePaths := make([]string, len(plan.PosePaths))
	for index := range alignedPosePaths {
		alignedPosePaths[index] = filepath.Join(dir, "pose-guides", fmt.Sprintf("%02d.png", index))
	}
	if err := imageio.WriteAlignedPoseGuides(plan.PosePaths, seedPath, alignedPosePaths); err != nil {
		return false, err
	}
	if err := imageio.WriteCanvasBoard(alignedPosePaths, poseBoardPath, layout, max(8, layout.CellWidth/32)); err != nil {
		return false, err
	}
	editSourcePath := filepath.Join(dir, "edit-source.png")
	lockedCell := -1
	if correctedFrame != "" {
		if err := imageio.WriteCanvasBoard(plan.ProductionPaths, editSourcePath, layout, max(8, layout.CellWidth/32)); err != nil {
			return false, err
		}
		lockedCell = rowFrameIndex(plan.Targets, correctedFrame)
		if lockedCell < 0 {
			return false, fmt.Errorf("frame %q does not belong to row %q", correctedFrame, plan.ID)
		}
	} else {
		sourcePaths := append([]string(nil), plan.PosePaths...)
		if plan.AnimationIndex == 0 {
			sourcePaths[0] = seedPath
			lockedCell = 0
		}
		if err := imageio.WriteCanvasBoard(sourcePaths, editSourcePath, layout, max(8, layout.CellWidth/32)); err != nil {
			return false, err
		}
	}
	maskPath := filepath.Join(dir, "attempts", nextAttemptID(state.Attempts, force || correctedFrame != "", rowCandidateCount), "mask.png")
	if correctedFrame != "" {
		if err := imageio.WriteCellEditMask(maskPath, layout, lockedCell); err != nil {
			return false, err
		}
	} else if err := imageio.WriteBoardEditMask(maskPath, layout, lockedCell); err != nil {
		return false, err
	}
	inputs := []conditioning.Input{
		{Role: conditioning.RolePose, Path: editSourcePath, Description: "Complete editable animation row; preserve fixed slots and unchanged cells.", Required: true},
		{Role: conditioning.RoleIdentity, Path: seedPath, Description: "Approved directional identity seed for character design, palette, equipment, and facing.", Required: true},
		{Role: conditioning.RolePose, Path: poseBoardPath, Description: "Ordered production pose board for frame silhouettes, baseline, and cadence.", Required: true},
		{Role: conditioning.RoleMask, Path: maskPath, Description: "Board edit mask; cell boundaries and protected cells must remain unchanged.", Required: true},
	}
	for _, target := range plan.Targets {
		inputs = append(inputs, filterInputs(target.Inputs, conditioning.RolePose)...)
	}
	inputs = uniqueConditioningInputs(inputs)
	state.Layout = &layout
	state.EditSourcePath = editSourcePath
	prompt := animationRowPrompt(plan, layout, correctedFrame)
	guidePath := poseBoardPath
	if correctedFrame == "" {
		guidePath = editSourcePath
	}
	if correctedFrame != "" || force {
		resetRowState(manifest, plan.Targets)
	}
	if err := generateBoardCandidates(ctx, gen, opts, manifest, state, dir, prompt, guidePath, inputs, rowCandidateCount, force || correctedFrame != "", "row"); err != nil {
		return false, err
	}
	if state.Status != StatusAwaitingReview {
		for _, target := range plan.Targets {
			manifest.Targets[target.ID].Status = StatusRejected
			manifest.Targets[target.ID].HardRejections = append([]string(nil), state.HardRejections...)
		}
		return false, Save(opts.OutputDir, opts.RunID, manifest)
	}
	if err := extractAnimationRow(opts, manifest, plan, state, seedPath); err != nil {
		if errors.Is(err, imageio.ErrCanonicalScaleCropping) {
			state.Status = StatusRejected
			state.HardRejections = []string{"canonical_seed_scale_cropping"}
			if err := recordSelectedCandidateRejection(state, state.HardRejections[0]); err != nil {
				return false, err
			}
			for _, target := range plan.Targets {
				targetState := manifest.Targets[target.ID]
				targetState.Status = StatusRejected
				targetState.HardRejections = append([]string(nil), state.HardRejections...)
				targetState.ProductionEligible = false
			}
			if err := writeQA(filepath.Dir(state.NormalizedPath), StatusRejected, "Generated row cannot fit the canonical directional-seed scale without cropping."); err != nil {
				return false, err
			}
			return false, Save(opts.OutputDir, opts.RunID, manifest)
		}
		return false, err
	}
	return true, Save(opts.OutputDir, opts.RunID, manifest)
}

func recordSelectedCandidateRejection(state *IntermediateState, reason string) error {
	if len(state.Attempts) == 0 {
		return nil
	}
	attempt := &state.Attempts[len(state.Attempts)-1]
	for index := range attempt.Candidates {
		candidate := &attempt.Candidates[index]
		if candidate.ID != attempt.SelectedCandidate {
			continue
		}
		if !containsString(candidate.HardRejections, reason) {
			candidate.HardRejections = append(candidate.HardRejections, reason)
		}
		return writeBoardCandidateMetrics(candidate.MetricsPath, *candidate)
	}
	return nil
}

func generateBoardCandidates(ctx context.Context, gen provider.Provider, opts Options, manifest *Manifest, state *IntermediateState, dir, prompt, guidePath string, inputs []conditioning.Input, candidateCount int, force bool, kind string) error {
	attempt := prepareIntermediateAttempt(state, force, candidateCount)
	attempt.Kind = kind
	attemptDir := filepath.Join(dir, "attempts", attempt.ID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return err
	}
	state.Artifacts.PromptPath = promptPath
	hydrated, err := hydrateInputHashes(inputs)
	if err != nil {
		return err
	}
	attempt.References, err = collectReferenceEvidence(hydrated)
	if err != nil {
		return err
	}
	for _, input := range hydrated {
		if input.Role == conditioning.RoleMask {
			attempt.MaskSHA256 = input.SHA256
		}
	}
	if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return err
	}
	opts.report(ProgressEvent{Stage: ProgressIntermediateGenerating, TargetID: state.ID})
	for index := len(attempt.Candidates); index < candidateCount; index++ {
		candidateID := fmt.Sprintf("%02d", index+1)
		candidateDir := filepath.Join(attemptDir, "candidates", candidateID)
		if err := os.MkdirAll(candidateDir, 0o755); err != nil {
			return err
		}
		opts.report(ProgressEvent{Stage: ProgressCandidateGenerating, TargetID: state.ID, Candidate: index + 1, Candidates: candidateCount})
		result, err := gen.Generate(ctx, provider.Request{Prompt: prompt, Size: image.Pt(providerCanvasSize, providerCanvasSize), Inputs: hydrated, CandidateOrdinal: index + 1, Progress: func(current, _ int) {
			opts.report(ProgressEvent{Stage: ProgressProviderProgress, TargetID: state.ID, Candidate: index + 1, Candidates: candidateCount, ProviderCurrent: current})
		}})
		if err != nil {
			return fmt.Errorf("generate %q candidate %s: %w", state.ID, candidateID, err)
		}
		rawPath := filepath.Join(candidateDir, "raw-candidate.png")
		if err := os.WriteFile(rawPath, result.PNG, 0o644); err != nil {
			return err
		}
		normalizedPath := filepath.Join(candidateDir, "normalized.png")
		if err := imageio.WriteTransparentBoard(normalizedPath, result.PNG, providerCanvasSize, providerCanvasSize); err != nil {
			return err
		}
		evaluation, err := imageio.EvaluateBoard(normalizedPath, guidePath, *state.Layout, max(8, state.Layout.CellWidth/32), boardValidationPurpose(kind))
		if err != nil {
			return err
		}
		candidate := Candidate{ID: candidateID, QualityVersion: candidateQualityVersion, RawPath: rawPath, NormalizedPath: normalizedPath, StudyMetrics: &evaluation.Metrics, Metrics: imageio.Metrics{Score: evaluation.Metrics.Score}, HardRejections: evaluation.BlockingFailures, Warnings: evaluation.Warnings, Metadata: result.Metadata}
		candidate.MetricsPath = filepath.Join(candidateDir, "metrics.json")
		if err := writeBoardCandidateMetrics(candidate.MetricsPath, candidate); err != nil {
			return err
		}
		attempt.Candidates = append(attempt.Candidates, candidate)
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return err
		}
	}
	state.Artifacts.CandidateSheetPath = filepath.Join(dir, "review", "candidates.png")
	if err := imageio.WriteCandidateReviewSheet(candidateReviewTiles(attempt.Candidates), state.Artifacts.CandidateSheetPath); err != nil {
		return err
	}
	selected := bestCandidate(attempt.Candidates)
	if selected == nil {
		state.Status = StatusRejected
		state.NormalizedPath = ""
		state.Lineage = ""
		state.HardRejections = candidateRejections(attempt.Candidates)
		state.Warnings = candidateWarnings(attempt.Candidates)
		if err := writeQA(dir, StatusRejected, CandidateReviewSummary(state)); err != nil {
			return err
		}
		state.Artifacts.QAPath = filepath.Join(dir, "qa.md")
		return Save(opts.OutputDir, opts.RunID, manifest)
	}
	state.Status = StatusAwaitingReview
	state.HardRejections = nil
	state.Warnings = append([]string(nil), selected.Warnings...)
	state.NormalizedPath = filepath.Join(dir, "normalized.png")
	state.EditSourcePath = filepath.Join(dir, "edit-source.png")
	if err := imageio.CopyFile(selected.NormalizedPath, state.NormalizedPath); err != nil {
		return err
	}
	if err := imageio.CopyFile(selected.NormalizedPath, state.EditSourcePath); err != nil {
		return err
	}
	attempt.SelectedCandidate = selected.ID
	hash, err := fileSHA256(state.NormalizedPath)
	if err != nil {
		return err
	}
	state.SourceSHA256 = hash
	state.Lineage = fmt.Sprintf("%s@%s/%s:%s", state.ID, attempt.ID, selected.ID, hash)
	if err := writeQA(dir, StatusAwaitingReview, CandidateReviewSummary(state)); err != nil {
		return err
	}
	state.Artifacts.QAPath = filepath.Join(dir, "qa.md")
	opts.report(ProgressEvent{Stage: ProgressIntermediateReady, TargetID: state.ID})
	return Save(opts.OutputDir, opts.RunID, manifest)
}

func boardValidationPurpose(kind string) imageio.BoardValidationPurpose {
	if kind == "seed" {
		return imageio.BoardValidationSeed
	}
	return imageio.BoardValidationAnimationRow
}

func uniqueConditioningInputs(inputs []conditioning.Input) []conditioning.Input {
	type inputKey struct {
		role conditioning.Role
		path string
	}
	seen := map[inputKey]bool{}
	unique := make([]conditioning.Input, 0, len(inputs))
	for _, input := range inputs {
		key := inputKey{role: input.Role, path: input.Path}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, input)
	}
	return unique
}

func extractAnimationRow(opts Options, manifest *Manifest, plan rowPlan, row *IntermediateState, seedPath string) error {
	outputs := make([]string, len(plan.Targets))
	for index, target := range plan.Targets {
		outputs[index] = filepath.Join(TargetDir(opts.OutputDir, opts.RunID, target.ID), "normalized.png")
	}
	lockedFirst := ""
	if plan.AnimationIndex == 0 {
		lockedFirst = seedPath
	}
	seed := manifest.Intermediates[plan.SeedID]
	lockedPalette := seed.ExtractedPalettes[plan.VariantKey]
	palette, transform, err := imageio.WriteCanonicalNormalizedCells(row.NormalizedPath, *row.Layout, outputs, plan.Targets[0].Size.Width, plan.Targets[0].Size.Height, lockedPalette, seedPath, lockedFirst != "")
	if err != nil {
		return err
	}
	selected := selectedCandidate(row)
	for index, target := range plan.Targets {
		state := manifest.Targets[target.ID]
		state.Review = nil
		state.Deploy = nil
		state.DeployPath = ""
		state.Status = StatusAwaitingReview
		state.NormalizedPath = outputs[index]
		state.SeedBoardID = plan.SeedID
		state.AnimationRowID = plan.ID
		state.SeedLineage = manifest.Intermediates[plan.SeedID].Lineage
		state.RowLineage = row.Lineage
		state.SourceCandidate = selected
		state.CellIndex = index
		state.Dependencies = []string{plan.SeedID, plan.ID}
		state.ProductionEligible = true
		state.CapabilityMode = "validated_row_extraction"
		state.Normalization = &NormalizationRecord{ScaleAlgorithm: "canonical-seed", PaletteMethod: "deterministic-median-cut", MaximumColors: 32, ColorSpace: "linear-srgb", Dithering: false, AlphaThreshold: 128, Anchor: "bottom-center", Scale: transform.Scale, Baseline: transform.Baseline, CenterX: transform.CenterX}
		if err := imageio.WritePalette(filepath.Join(TargetDir(opts.OutputDir, opts.RunID, target.ID), "palette.json"), palette); err != nil {
			return err
		}
		if err := writeQA(TargetDir(opts.OutputDir, opts.RunID, target.ID), "generated", "Extracted only after complete-row mechanical validation; mandatory whole-row visual QA remains required."); err != nil {
			return err
		}
	}
	artifacts, err := writeFrameReviewArtifacts(outputs, filepath.Dir(row.NormalizedPath), true)
	if err != nil {
		return err
	}
	row.Artifacts.ContactSheetPath = artifacts.ContactSheetPath
	row.Artifacts.AnimationGIFPath = artifacts.AnimationGIFPath
	row.Artifacts.FramePaths = artifacts.FramePaths
	for _, target := range plan.Targets {
		manifest.Targets[target.ID].Artifacts = artifacts
	}
	return nil
}

func writeFrameReviewArtifacts(paths []string, dir string, animated bool) (ReviewArtifacts, error) {
	artifacts := ReviewArtifacts{
		ContactSheetPath: filepath.Join(dir, "review", "contact-sheet.png"),
		FramePaths:       append([]string(nil), paths...),
	}
	if err := imageio.WriteNearestNeighborContactSheet(paths, artifacts.ContactSheetPath, 4); err != nil {
		return ReviewArtifacts{}, err
	}
	if animated {
		artifacts.AnimationGIFPath = filepath.Join(dir, "review", "animation.gif")
		if err := imageio.WriteLoopingGIF(paths, artifacts.AnimationGIFPath, 12); err != nil {
			return ReviewArtifacts{}, err
		}
	}
	return artifacts, nil
}

func selectedCandidate(state *IntermediateState) string {
	if len(state.Attempts) == 0 {
		return ""
	}
	return state.Attempts[len(state.Attempts)-1].SelectedCandidate
}

func prepareIntermediateAttempt(state *IntermediateState, force bool, candidateCount int) *Attempt {
	if !force && state.Status == StatusPending && len(state.Attempts) > 0 {
		latest := &state.Attempts[len(state.Attempts)-1]
		if latest.SelectedCandidate == "" && len(latest.Candidates) < candidateCount {
			return latest
		}
	}
	state.Status = StatusPending
	state.NormalizedPath = ""
	state.SourceSHA256 = ""
	state.Lineage = ""
	state.HardRejections = nil
	state.Warnings = nil
	id := fmt.Sprintf("%03d", len(state.Attempts)+1)
	state.Attempts = append(state.Attempts, Attempt{ID: id, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
	return &state.Attempts[len(state.Attempts)-1]
}

func nextAttemptID(attempts []Attempt, force bool, candidateCount int) string {
	if !force && len(attempts) > 0 && attempts[len(attempts)-1].SelectedCandidate == "" && len(attempts[len(attempts)-1].Candidates) < candidateCount {
		return attempts[len(attempts)-1].ID
	}
	return fmt.Sprintf("%03d", len(attempts)+1)
}

func writeBoardCandidateMetrics(path string, candidate Candidate) error {
	document := struct {
		Metrics        imageio.Metrics       `json:"metrics"`
		BoardMetrics   *imageio.StudyMetrics `json:"boardMetrics,omitempty"`
		HardRejections []string              `json:"hardRejections,omitempty"`
		Warnings       []string              `json:"warnings,omitempty"`
	}{Metrics: candidate.Metrics, BoardMetrics: candidate.StudyMetrics, HardRejections: candidate.HardRejections, Warnings: candidate.Warnings}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONData(path, data)
}

func rowFrameIndex(row []targets.Target, frame string) int {
	for index, target := range row {
		if target.FrameID == frame {
			return index
		}
	}
	return -1
}

func resetRowState(manifest *Manifest, row []targets.Target) {
	for _, target := range row {
		state := manifest.Targets[target.ID]
		state.Review = nil
		state.Deploy = nil
		state.DeployPath = ""
		state.Status = StatusPending
		state.NormalizedPath = ""
		state.Dependencies = nil
		state.SeedBoardID = ""
		state.AnimationRowID = ""
		state.SeedLineage = ""
		state.RowLineage = ""
		state.SourceCandidate = ""
		state.CellIndex = 0
		state.ProductionEligible = false
		state.CapabilityMode = ""
		state.Palette = nil
		state.Normalization = nil
		state.HardRejections = nil
		state.Artifacts = ReviewArtifacts{}
	}
}

func invalidateSeedDependents(manifest *Manifest, seedID string) {
	for _, intermediate := range manifest.Intermediates {
		if intermediate.Kind == animationRowKind && containsString(intermediate.Dependencies, seedID) {
			intermediate.Status = StatusPending
			intermediate.NormalizedPath = ""
			intermediate.Lineage = ""
		}
	}
	for _, target := range manifest.Targets {
		if target.SeedBoardID == seedID {
			target.Review = nil
			target.Status = StatusPending
			target.NormalizedPath = ""
			target.ProductionEligible = false
		}
	}
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func seedBoardPrompt(plan seedPlan, layout imageio.GridLayout) string {
	var cells strings.Builder
	for index, reference := range plan.References {
		fmt.Fprintf(&cells, "%d. %s — %s: %s\n", index+1, gridCellDescription(layout, index), plan.VariantKey[index], reference.ObjectDesc)
		for _, variant := range reference.Variants {
			fmt.Fprintf(&cells, "   Variant %s=%s: %s\n", variant.AxisID, variant.ValueID, strings.TrimSpace(variant.Description))
		}
	}
	var unused strings.Builder
	for index := layout.Count; index < gridSlotCount(layout); index++ {
		fmt.Fprintf(&unused, "Unused cell: %s. It must remain completely empty; do not place any subject or decoration there.\n", gridCellDescription(layout, index))
	}
	return fmt.Sprintf(`# Combined Directional Seed Board
Create exactly %d consistent views of one character on the supplied %dx%d fixed board. Expected cells use row-major order and must remain at the exact coordinates listed below. Treat every configured variant description below as a semantic requirement: preserve its named orientation and never mirror or reverse it. Preserve each source cell's facing, baseline, scale, silhouette, equipment sides, and empty padding. The same face, proportions, clothing, weapon, palette, and heraldry must appear in every view. Keep every unused cell and all space outside expected cells empty. Use one isolated subject per expected cell. No labels, borders, shadows, projectiles, scenery, overlap, or content crossing cell boundaries.

# Expected cells
%s
# Forbidden unused cells
%s`, layout.Count, layout.Columns, layout.Rows, cells.String(), unused.String())
}

func gridSlotCount(layout imageio.GridLayout) int {
	columns, rows := layout.Columns, layout.Rows
	if columns == 0 {
		columns = layout.Side
	}
	if rows == 0 {
		rows = layout.Side
	}
	return columns * rows
}

func gridCellDescription(layout imageio.GridLayout, index int) string {
	columns, rows := layout.Columns, layout.Rows
	if columns == 0 {
		columns = layout.Side
	}
	if rows == 0 {
		rows = layout.Side
	}
	row, column := index/columns+1, index%columns+1
	description := fmt.Sprintf("row %d, column %d", row, column)
	if rows == 2 && columns == 2 {
		positions := [4]string{"top-left", "top-right", "bottom-left", "bottom-right"}
		description += " (" + positions[index] + ")"
	}
	return description
}

func animationRowPrompt(plan rowPlan, layout imageio.GridLayout, correctedFrame string) string {
	var requirements strings.Builder
	if len(plan.Targets) != 0 {
		first := plan.Targets[0]
		if description := strings.TrimSpace(first.ObjectDesc); description != "" {
			fmt.Fprintf(&requirements, "# Object requirement\n%s\n\n", description)
		}
		for _, variant := range first.Variants {
			fmt.Fprintf(&requirements, "# Variant requirement: %s=%s\n%s\n\n", variant.AxisID, variant.ValueID, strings.TrimSpace(variant.Description))
		}
		if description := strings.TrimSpace(first.AnimationDesc); description != "" {
			fmt.Fprintf(&requirements, "# Animation requirement: %s\n%s\n\n", plan.AnimationID, description)
		}
	}
	var frames strings.Builder
	for index, target := range plan.Targets {
		fmt.Fprintf(&frames, "%d. %s — %s: %s\n", index+1, gridCellDescription(layout, index), target.FrameID, strings.TrimSpace(target.FrameDesc))
	}
	correction := ""
	if correctedFrame != "" {
		index := rowFrameIndex(plan.Targets, correctedFrame)
		correction = fmt.Sprintf("\nCorrect only frame %s inside %s, which is the sole masked cell. Preserve all other cells exactly.\n", correctedFrame, gridCellDescription(layout, index))
	}
	return fmt.Sprintf(`# Complete Animation Row
Create one complete %s animation row with exactly %d ordered frames in the supplied fixed cells. The approved directional seed is immutable identity guidance. Preserve face, proportions, clothing, weapon, palette, heraldry, facing, equipment sides, scale, and baseline across every frame. Follow the production pose board for the intended motion. Keep gutters, space outside cells, and trailing cells empty. No labels, borders, shadows, projectiles, scenery, overlap, or content crossing cell boundaries.%s

%s
# Ordered frames
Expected cells use left-to-right fixed order. Put each named frame only in its assigned cell:
%s`, plan.AnimationID, layout.Count, correction, requirements.String(), frames.String())
}

// SelectSeedCandidate applies the human seed decision and extracts normalized
// directional seeds by fixed coordinates.
func SelectSeedCandidate(all []targets.Target, outputDir, runID, objectID, candidateID, status, reason string) error {
	if status != StatusAccepted && status != StatusRejected {
		return fmt.Errorf("unsupported seed review status %q", status)
	}
	manifest, err := Load(outputDir, runID)
	if err != nil {
		return err
	}
	state := manifest.Intermediates["direction-seed-board:"+objectID]
	if state == nil || state.Layout == nil {
		return fmt.Errorf("object %q has no generated seed board", objectID)
	}
	seedDir := filepath.Join(outputDir, "runs", runID, "intermediates", objectID, "direction-seeds")
	if state.Artifacts.PromptPath == "" {
		state.Artifacts.PromptPath = filepath.Join(seedDir, "prompt.md")
	}
	if err := refreshBoardCandidateValidation(state, filepath.Join(seedDir, "source-board.png"), imageio.BoardValidationSeed); err != nil {
		return err
	}
	state.Review = &ReviewRecord{Status: status, Reason: reason, Candidate: candidateID, ReviewedAt: time.Now().UTC().Format(time.RFC3339)}
	if status == StatusRejected {
		if candidateID != "" {
			if _, err := findIntermediateCandidate(state, candidateID); err != nil {
				return err
			}
		}
		state.Status = StatusRejected
		if err := writeIntermediateCandidateMetrics(state); err != nil {
			return err
		}
		if err := writeQA(seedDir, status, reason); err != nil {
			return err
		}
		state.Artifacts.QAPath = filepath.Join(seedDir, "qa.md")
		return Save(outputDir, runID, manifest)
	}
	candidate, err := findIntermediateCandidate(state, candidateID)
	if err != nil {
		return err
	}
	if len(candidate.HardRejections) != 0 {
		return seedCandidateValidationError(state, candidate)
	}
	selectIntermediateCandidate(state, candidate)
	state.NormalizedPath = filepath.Join(seedDir, "normalized.png")
	state.EditSourcePath = filepath.Join(seedDir, "edit-source.png")
	if err := imageio.CopyFile(candidate.NormalizedPath, state.NormalizedPath); err != nil {
		return err
	}
	if err := imageio.CopyFile(candidate.NormalizedPath, state.EditSourcePath); err != nil {
		return err
	}
	state.Status = StatusAccepted
	state.HardRejections = nil
	state.Warnings = append([]string(nil), candidate.Warnings...)
	state.Extracted = map[string]string{}
	state.ExtractedPalettes = map[string][]imageio.PaletteColor{}
	var references []targets.Target
	for _, target := range all {
		if target.ObjectID == objectID && target.AnimationIndex == 0 && target.FrameIndex == 0 {
			references = append(references, target)
		}
	}
	if len(references) != state.Layout.Count {
		return fmt.Errorf("seed board expects %d variants, pack now expands %d", state.Layout.Count, len(references))
	}
	paths := make([]string, len(references))
	for index, reference := range references {
		key := safeVariantKey(reference)
		paths[index] = filepath.Join(filepath.Dir(state.NormalizedPath), "seeds", key+".png")
		state.Extracted[key] = paths[index]
	}
	palette, err := imageio.WriteSharedNormalizedCells(state.NormalizedPath, *state.Layout, paths, references[0].Size.Width, references[0].Size.Height, nil, "")
	if err != nil {
		return err
	}
	for _, reference := range references {
		state.ExtractedPalettes[safeVariantKey(reference)] = palette
	}
	artifacts, err := writeFrameReviewArtifacts(paths, seedDir, false)
	if err != nil {
		return err
	}
	artifacts.CandidateSheetPath = state.Artifacts.CandidateSheetPath
	artifacts.PromptPath = state.Artifacts.PromptPath
	artifacts.QAPath = state.Artifacts.QAPath
	state.Artifacts = artifacts
	hash, err := fileSHA256(state.NormalizedPath)
	if err != nil {
		return err
	}
	state.Lineage = fmt.Sprintf("%s@%s:%s", state.ID, candidate.ID, hash)
	if len(candidate.Warnings) != 0 {
		reason += "\n\nAdvisory seed differences accepted by visual review: " + strings.Join(candidate.Warnings, ", ")
	}
	if err := writeQA(seedDir, status, reason); err != nil {
		return err
	}
	state.Artifacts.QAPath = filepath.Join(seedDir, "qa.md")
	if err := writeIntermediateCandidateMetrics(state); err != nil {
		return err
	}
	return Save(outputDir, runID, manifest)
}

func findIntermediateCandidate(state *IntermediateState, candidateID string) (*Candidate, error) {
	for attemptIndex := len(state.Attempts) - 1; attemptIndex >= 0; attemptIndex-- {
		for candidateIndex := range state.Attempts[attemptIndex].Candidates {
			candidate := &state.Attempts[attemptIndex].Candidates[candidateIndex]
			if candidate.ID == candidateID {
				return candidate, nil
			}
		}
	}
	return nil, fmt.Errorf("candidate %q not found", candidateID)
}

func selectIntermediateCandidate(state *IntermediateState, selected *Candidate) {
	for attemptIndex := len(state.Attempts) - 1; attemptIndex >= 0; attemptIndex-- {
		for candidateIndex := range state.Attempts[attemptIndex].Candidates {
			if &state.Attempts[attemptIndex].Candidates[candidateIndex] == selected {
				state.Attempts[attemptIndex].SelectedCandidate = selected.ID
				return
			}
		}
	}
}

func refreshBoardCandidateValidation(state *IntermediateState, guidePath string, purpose imageio.BoardValidationPurpose) error {
	for attemptIndex := range state.Attempts {
		for candidateIndex := range state.Attempts[attemptIndex].Candidates {
			candidate := &state.Attempts[attemptIndex].Candidates[candidateIndex]
			if candidate.QualityVersion >= candidateQualityVersion {
				continue
			}
			evaluation, err := imageio.EvaluateBoard(candidate.NormalizedPath, guidePath, *state.Layout, max(2, state.Layout.CellWidth/32), purpose)
			if err != nil {
				return fmt.Errorf("revalidate candidate %q: %w", candidate.ID, err)
			}
			candidate.QualityVersion = candidateQualityVersion
			candidate.StudyMetrics = &evaluation.Metrics
			candidate.Metrics.Score = evaluation.Metrics.Score
			candidate.HardRejections = evaluation.BlockingFailures
			candidate.Warnings = evaluation.Warnings
		}
	}
	if len(state.Attempts) != 0 {
		latest := state.Attempts[len(state.Attempts)-1].Candidates
		state.HardRejections = candidateRejections(latest)
		state.Warnings = candidateWarnings(latest)
	}
	return nil
}

func writeIntermediateCandidateMetrics(state *IntermediateState) error {
	for _, attempt := range state.Attempts {
		for _, candidate := range attempt.Candidates {
			if candidate.MetricsPath == "" {
				continue
			}
			if err := writeBoardCandidateMetrics(candidate.MetricsPath, candidate); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedCandidateValidationError(state *IntermediateState, candidate *Candidate) error {
	eligible := CandidateEvidence(state).Eligible
	eligibleText := "none"
	if len(eligible) != 0 {
		eligibleText = strings.Join(eligible, ", ")
	}
	return fmt.Errorf("candidate %q failed structural validation: %s; eligible candidates: %s", candidate.ID, strings.Join(candidate.HardRejections, ", "), eligibleText)
}

func eligibleCandidateIDs(state *IntermediateState) []string {
	return CandidateEvidence(state).Eligible
}

// CandidateReviewEvidence separates mechanical eligibility from the candidate
// the pipeline selected for normalized evidence. Neither is visual approval.
type CandidateReviewEvidence struct {
	Eligible              []string
	Invalid               []string
	MechanicallyPreferred string
}

// CandidateEvidence reports the latest attempt only, matching review behavior.
func CandidateEvidence(state *IntermediateState) CandidateReviewEvidence {
	var evidence CandidateReviewEvidence
	if state == nil || len(state.Attempts) == 0 {
		return evidence
	}
	latest := state.Attempts[len(state.Attempts)-1]
	for _, candidate := range latest.Candidates {
		if len(candidate.HardRejections) == 0 {
			evidence.Eligible = append(evidence.Eligible, candidate.ID)
			if candidate.ID == latest.SelectedCandidate {
				evidence.MechanicallyPreferred = candidate.ID
			}
			continue
		}
		evidence.Invalid = append(evidence.Invalid, candidate.ID)
	}
	if evidence.MechanicallyPreferred == "" {
		if preferred := bestCandidate(latest.Candidates); preferred != nil {
			evidence.MechanicallyPreferred = preferred.ID
		}
	}
	return evidence
}

// CandidateReviewSummary provides the same classification used by status and
// review while retaining candidate-specific rejection evidence.
func CandidateReviewSummary(state *IntermediateState) string {
	evidence := CandidateEvidence(state)
	eligible := joinedOrNone(evidence.Eligible)
	preferred := evidence.MechanicallyPreferred
	if preferred == "" {
		preferred = "none"
	}
	invalid := []string(nil)
	if state != nil && len(state.Attempts) != 0 {
		latest := state.Attempts[len(state.Attempts)-1]
		for _, candidate := range latest.Candidates {
			if len(candidate.HardRejections) != 0 {
				invalid = append(invalid, fmt.Sprintf("%s (%s)", candidate.ID, strings.Join(candidate.HardRejections, ", ")))
			}
		}
	}
	next := "Manual visual review is still required."
	if len(evidence.Eligible) == 0 {
		next = "No candidate can be reviewed; start a forced generation attempt after correcting the prompt or references."
	}
	return fmt.Sprintf("Eligible candidates: %s. Mechanically preferred candidate: %s. Invalid candidates: %s. %s", eligible, preferred, joinedOrNone(invalid), next)
}

func joinedOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func candidateReviewTiles(candidates []Candidate) []imageio.CandidateReviewTile {
	tiles := make([]imageio.CandidateReviewTile, len(candidates))
	for index, candidate := range candidates {
		tiles[index] = imageio.CandidateReviewTile{ID: candidate.ID, Path: candidate.NormalizedPath, Valid: len(candidate.HardRejections) == 0}
	}
	return tiles
}
