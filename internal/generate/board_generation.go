package generate

import (
	"context"
	"encoding/json"
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

func runRoot(opts Options) string {
	return filepath.Join(opts.OutputDir, "runs", opts.RunID)
}

func generateBoardCandidate(
	ctx context.Context,
	gen provider.Provider,
	opts Options,
	manifest *Manifest,
	state *IntermediateState,
	dir, prompt string,
	inputs, reviewOnly []conditioning.Input,
	maskPath string,
	kind string,
	canvas image.Point,
) error {
	attempt := prepareIntermediateAttempt(state)
	attempt.Kind = kind
	attemptDir := filepath.Join(dir, "attempts", attempt.ID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return err
	}
	hydrated, err := hydrateInputHashes(inputs)
	if err != nil {
		return err
	}
	prompt = renderProviderPrompt(prompt, hydrated)
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return err
	}
	state.Artifacts.PromptPath = promptPath
	attempt.References, err = collectReferenceEvidence(hydrated, reviewOnly)
	if err != nil {
		return err
	}
	if maskPath != "" {
		maskEvidence, evidenceErr := referenceEvidence(conditioning.Input{
			ID: "edit-mask", Role: conditioning.RolePose,
			Authority: "cli-protocol", SourcePath: maskPath, Path: maskPath,
			Description: "CLI-owned alpha mask defining guarded editable regions.",
		}, true, 0)
		if evidenceErr != nil {
			return evidenceErr
		}
		maskEvidence.Role = "mask"
		attempt.References = append(attempt.References, maskEvidence)
	}
	state.Artifacts.EvidencePath = filepath.Join(attemptDir, "evidence.json")
	if err := writeEvidence(state.Artifacts.EvidencePath, attempt.References); err != nil {
		return err
	}
	if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return err
	}
	opts.report(ProgressEvent{Stage: ProgressIntermediateGenerating, TargetID: state.ID})
	if len(attempt.Candidates) == 0 {
		opts.report(ProgressEvent{Stage: ProgressCandidateGenerating, TargetID: state.ID, Candidate: 1, Candidates: 1})
		result, err := gen.Generate(ctx, provider.Request{
			Prompt: prompt, Size: canvas, Inputs: hydrated, MaskPath: maskPath, CandidateOrdinal: 1,
			Progress: func(current, _ int) {
				opts.report(ProgressEvent{Stage: ProgressProviderProgress, TargetID: state.ID, Candidate: 1, Candidates: 1, ProviderCurrent: current})
			},
		})
		if err != nil {
			return fmt.Errorf("generate %q candidate 01: %w", state.ID, err)
		}
		candidateDir := filepath.Join(attemptDir, "candidates", "01")
		if err := os.MkdirAll(candidateDir, 0o755); err != nil {
			return err
		}
		rawPath := filepath.Join(candidateDir, "raw-candidate.png")
		if err := os.WriteFile(rawPath, result.PNG, 0o644); err != nil {
			return err
		}
		normalizedPath := filepath.Join(candidateDir, "normalized.png")
		evaluation, err := evaluateBoardCandidate(state, rawPath, normalizedPath, canvas)
		if err != nil {
			return err
		}
		candidate := Candidate{
			ID: "01", QualityVersion: candidateQualityVersion, RawPath: rawPath, NormalizedPath: normalizedPath,
			StudyMetrics: &evaluation.Metrics, Metrics: imageio.Metrics{Score: evaluation.Metrics.Score},
			HardRejections: evaluation.BlockingFailures, Warnings: evaluation.Warnings, Metadata: result.Metadata,
		}
		candidate.MetricsPath = filepath.Join(candidateDir, "metrics.json")
		if err := writeBoardCandidateMetrics(candidate.MetricsPath, candidate); err != nil {
			return err
		}
		attempt.Candidates = append(attempt.Candidates, candidate)
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return err
		}
	}
	candidate := &attempt.Candidates[0]
	if candidate.QualityVersion < candidateQualityVersion {
		evaluation, err := evaluateBoardCandidate(state, candidate.RawPath, candidate.NormalizedPath, canvas)
		if err != nil {
			return err
		}
		candidate.QualityVersion = candidateQualityVersion
		candidate.StudyMetrics = &evaluation.Metrics
		candidate.Metrics = imageio.Metrics{Score: evaluation.Metrics.Score}
		candidate.HardRejections = evaluation.BlockingFailures
		candidate.Warnings = evaluation.Warnings
		if candidate.MetricsPath == "" {
			candidate.MetricsPath = filepath.Join(filepath.Dir(candidate.NormalizedPath), "metrics.json")
		}
		if err := writeBoardCandidateMetrics(candidate.MetricsPath, *candidate); err != nil {
			return err
		}
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return err
		}
	}
	state.Artifacts.CandidateSheetPath = filepath.Join(dir, "review", "candidates.png")
	if err := imageio.WriteCandidateReviewSheet([]imageio.CandidateReviewTile{{
		ID: candidate.ID, Path: candidate.NormalizedPath, Valid: len(candidate.HardRejections) == 0,
	}}, state.Artifacts.CandidateSheetPath); err != nil {
		return err
	}
	if len(candidate.HardRejections) != 0 {
		state.Status = StatusRejected
		state.NormalizedPath = ""
		state.Lineage = ""
		state.HardRejections = append([]string(nil), candidate.HardRejections...)
		state.Warnings = append([]string(nil), candidate.Warnings...)
		if err := writeQA(dir, StatusRejected, candidateReviewSummary(candidate)); err != nil {
			return err
		}
		state.Artifacts.QAPath = filepath.Join(dir, "qa.md")
		return Save(opts.OutputDir, opts.RunID, manifest)
	}
	state.Status = StatusAwaitingReview
	state.HardRejections = nil
	state.Warnings = append([]string(nil), candidate.Warnings...)
	state.NormalizedPath = filepath.Join(dir, "normalized.png")
	if err := imageio.CopyFile(candidate.NormalizedPath, state.NormalizedPath); err != nil {
		return err
	}
	if state.SemanticLayout != nil {
		recovered := make([]string, len(state.SemanticLayout.Anchors))
		for index := range recovered {
			recovered[index] = filepath.Join(
				dir,
				"recovered",
				fmt.Sprintf("%02d.png", index),
			)
		}
		poses, recoveryErr := imageio.RecoverSemanticPoses(
			state.NormalizedPath,
			*state.SemanticLayout,
			recovered,
		)
		if recoveryErr != nil {
			return recoveryErr
		}
		state.Poses = poses
		state.Artifacts.RecoveredPosePaths = recovered
		state.Artifacts.OwnershipOverlayPath = filepath.Join(
			dir,
			"review",
			"ownership.png",
		)
		if err := imageio.WriteSemanticOwnershipOverlay(
			state.NormalizedPath,
			*state.SemanticLayout,
			poses,
			state.Artifacts.OwnershipOverlayPath,
		); err != nil {
			return err
		}
		state.Artifacts.RecoveredPoseSheetPath = filepath.Join(
			dir,
			"review",
			"recovered-poses.png",
		)
		if err := imageio.WriteRecoveredPoseSheet(
			recovered,
			state.SemanticLayout.Columns,
			state.Artifacts.RecoveredPoseSheetPath,
		); err != nil {
			return err
		}
	}
	attempt.SelectedCandidate = candidate.ID
	hash, err := fileSHA256(state.NormalizedPath)
	if err != nil {
		return err
	}
	state.SourceSHA256 = hash
	state.Lineage = fmt.Sprintf("%s@%s/%s:%s", state.ID, attempt.ID, candidate.ID, hash)
	state.Artifacts.BoardMetricsPath = candidate.MetricsPath
	if err := writeQA(dir, StatusAwaitingReview, candidateReviewSummary(candidate)); err != nil {
		return err
	}
	state.Artifacts.QAPath = filepath.Join(dir, "qa.md")
	opts.report(ProgressEvent{Stage: ProgressIntermediateReady, TargetID: state.ID})
	return Save(opts.OutputDir, opts.RunID, manifest)
}

func evaluateBoardCandidate(
	state *IntermediateState,
	rawPath, normalizedPath string,
	canvas image.Point,
) (imageio.BoardEvaluation, error) {
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		return imageio.BoardEvaluation{}, err
	}
	if err := imageio.WriteTransparentBoard(normalizedPath, raw, canvas.X, canvas.Y); err != nil {
		return imageio.BoardEvaluation{}, err
	}
	if state.SemanticLayout == nil {
		return imageio.BoardEvaluation{}, fmt.Errorf(
			"intermediate %q has no semantic layout",
			state.ID,
		)
	}
	if _, err := imageio.RecoverSemanticPoses(
		normalizedPath,
		*state.SemanticLayout,
		nil,
	); err != nil {
		return imageio.BoardEvaluation{
			BlockingFailures: []string{
				"semantic_pose_recovery_failed: " + err.Error(),
			},
		}, nil
	}
	return imageio.BoardEvaluation{
		Metrics: imageio.StudyMetrics{Score: 1},
	}, nil
}

func candidateReviewSummary(candidate *Candidate) string {
	if candidate == nil {
		return "No candidate was recorded."
	}
	if len(candidate.HardRejections) != 0 {
		return "Candidate 01 failed structural validation: " + strings.Join(candidate.HardRejections, ", ") + "."
	}
	if len(candidate.Warnings) != 0 {
		return "Candidate 01 passed structural validation. Manual review evidence: " + strings.Join(candidate.Warnings, ", ") + "."
	}
	return "Candidate 01 passed structural validation. Manual visual review is still required."
}

func prepareIntermediateAttempt(state *IntermediateState) *Attempt {
	if intermediateNeedsReprocessing(state) {
		resetIntermediateResult(state)
		return &state.Attempts[len(state.Attempts)-1]
	}
	if state.Status == StatusPending && len(state.Attempts) > 0 {
		latest := &state.Attempts[len(state.Attempts)-1]
		if latest.SelectedCandidate == "" && len(latest.Candidates) <= 1 {
			return latest
		}
	}
	resetIntermediateResult(state)
	id := fmt.Sprintf("%03d", len(state.Attempts)+1)
	state.Attempts = append(state.Attempts, Attempt{ID: id, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
	return &state.Attempts[len(state.Attempts)-1]
}

func resetIntermediateResult(state *IntermediateState) {
	state.Status = StatusPending
	state.NormalizedPath = ""
	state.SourceSHA256 = ""
	state.Lineage = ""
	state.HardRejections = nil
	state.Warnings = nil
}

func writeBoardCandidateMetrics(path string, candidate Candidate) error {
	document := struct {
		Metrics        imageio.Metrics       `json:"metrics"`
		BoardMetrics   *imageio.StudyMetrics `json:"boardMetrics,omitempty"`
		HardRejections []string              `json:"hardRejections,omitempty"`
		Warnings       []string              `json:"warnings,omitempty"`
	}{
		Metrics: candidate.Metrics, BoardMetrics: candidate.StudyMetrics,
		HardRejections: candidate.HardRejections, Warnings: candidate.Warnings,
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONData(path, data)
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
	if state == nil || len(state.Attempts) == 0 {
		return ""
	}
	latest := state.Attempts[len(state.Attempts)-1]
	if latest.SelectedCandidate == "" {
		return ""
	}
	return latest.ID + "/" + latest.SelectedCandidate
}

func orderedSelectedObjects(all []targets.Target, selected map[string]bool) []string {
	seen := map[string]bool{}
	var result []string
	for _, target := range all {
		if selected[target.ObjectID] && !seen[target.ObjectID] {
			seen[target.ObjectID] = true
			result = append(result, target.ObjectID)
		}
	}
	return result
}

func targetIDs(values []targets.Target) []string {
	ids := make([]string, len(values))
	for index, value := range values {
		ids[index] = value.ID
	}
	return ids
}
