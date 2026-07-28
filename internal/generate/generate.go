// Package generate owns resumable draft runs and provider-backed target generation.
package generate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

const (
	ManifestVersion         = 9
	candidateQualityVersion = 13

	StatusPending        = "pending"
	StatusAwaitingReview = "awaiting_review"
	StatusAccepted       = "accepted"
	StatusRejected       = "rejected"
	StatusDeployed       = "deployed"
	StatusReady          = "ready"

	removableBackgroundInstruction = "Render every empty pixel with one uniform flat opaque background color that is visually distinct from the subject. Choose a single high-saturation chroma-key RGB color absent from the subject palette, preferring pure green, magenta, or cyan only when that color does not occur in the subject. Never use gray, black, white, beige, or another low-saturation color as the background. Use the exact same RGB value across the entire canvas so it can be removed cleanly. Do not draw grid lines, dashed guides, checkerboards, gradients, vignettes, lighting falloff, texture, transparency, or shadows."
)

type Manifest struct {
	Version       int                           `json:"version"`
	RunID         string                        `json:"runId"`
	Intermediates map[string]*IntermediateState `json:"intermediates,omitempty"`
	Units         map[string]*UnitState         `json:"units,omitempty"`
	Targets       map[string]*TargetState       `json:"targets"`
}

type UnitState struct {
	ID                string                         `json:"id"`
	ObjectID          string                         `json:"objectId"`
	Status            string                         `json:"status"`
	MasterID          string                         `json:"masterId"`
	AnimationBoardIDs []string                       `json:"animationBoardIds"`
	TargetIDs         []string                       `json:"targetIds"`
	MasterLineage     string                         `json:"masterLineage,omitempty"`
	AnimationLineages map[string]string              `json:"animationLineages,omitempty"`
	Transform         *imageio.SemanticUnitTransform `json:"transform,omitempty"`
	HardRejections    []string                       `json:"hardRejections,omitempty"`
	Artifacts         ReviewArtifacts                `json:"artifacts,omitempty"`
	Review            *ReviewRecord                  `json:"review,omitempty"`
	Deploy            *DeployRecord                  `json:"deploy,omitempty"`
}

// IntermediateState stores one character master or one complete animation
// board. Neither intermediate is reviewed or deployed independently.
type IntermediateState struct {
	ID             string                  `json:"id"`
	Kind           string                  `json:"kind"`
	Status         string                  `json:"status,omitempty"`
	ObjectID       string                  `json:"objectId"`
	AnimationID    string                  `json:"animationId,omitempty"`
	TargetIDs      []string                `json:"targetIds,omitempty"`
	Dependencies   []string                `json:"dependencies,omitempty"`
	ParentID       string                  `json:"parentId,omitempty"`
	NormalizedPath string                  `json:"normalizedPath,omitempty"`
	SourceSHA256   string                  `json:"sourceSha256,omitempty"`
	Lineage        string                  `json:"lineage,omitempty"`
	EditSourcePath string                  `json:"editSourcePath,omitempty"`
	SemanticLayout *imageio.SemanticLayout `json:"semanticLayout,omitempty"`
	Poses          []imageio.SemanticPose  `json:"poses,omitempty"`
	HardRejections []string                `json:"hardRejections,omitempty"`
	Warnings       []string                `json:"warnings,omitempty"`
	Artifacts      ReviewArtifacts         `json:"artifacts,omitempty"`
	Attempts       []Attempt               `json:"attempts,omitempty"`
}

type TargetState struct {
	ID                 string                 `json:"id"`
	Status             string                 `json:"status"`
	DeployPath         string                 `json:"deployPath,omitempty"`
	NormalizedPath     string                 `json:"normalizedPath,omitempty"`
	Dependencies       []string               `json:"dependencies,omitempty"`
	UnitID             string                 `json:"unitId,omitempty"`
	CharacterMasterID  string                 `json:"characterMasterId,omitempty"`
	AnimationBoardID   string                 `json:"animationBoardId,omitempty"`
	MasterLineage      string                 `json:"masterLineage,omitempty"`
	AnimationLineage   string                 `json:"animationLineage,omitempty"`
	SourceCandidate    string                 `json:"sourceCandidate,omitempty"`
	CellIndex          int                    `json:"cellIndex,omitempty"`
	ProductionEligible bool                   `json:"productionEligible"`
	CapabilityMode     string                 `json:"capabilityMode,omitempty"`
	Palette            []imageio.PaletteColor `json:"palette,omitempty"`
	Normalization      *NormalizationRecord   `json:"normalization,omitempty"`
	HardRejections     []string               `json:"hardRejections,omitempty"`
	Artifacts          ReviewArtifacts        `json:"artifacts,omitempty"`
	Attempts           []Attempt              `json:"attempts,omitempty"`
	Review             *ReviewRecord          `json:"review,omitempty"`
	Deploy             *DeployRecord          `json:"deploy,omitempty"`
	Production         *ProductionEvidence    `json:"production,omitempty"`
}

type NormalizationRecord struct {
	ScaleAlgorithm string  `json:"scaleAlgorithm"`
	PaletteMethod  string  `json:"paletteMethod"`
	MaximumColors  int     `json:"maximumColors"`
	ColorSpace     string  `json:"colorSpace"`
	Dithering      bool    `json:"dithering"`
	AlphaThreshold uint8   `json:"alphaThreshold"`
	Anchor         string  `json:"anchor,omitempty"`
	Scale          float64 `json:"scale,omitempty"`
	Baseline       int     `json:"baseline,omitempty"`
	CenterX        int     `json:"centerX,omitempty"`
	OffsetX        int     `json:"offsetX,omitempty"`
	OffsetY        int     `json:"offsetY,omitempty"`
}

type ReviewArtifacts struct {
	PromptPath                string   `json:"promptPath,omitempty"`
	EvidencePath              string   `json:"evidencePath,omitempty"`
	QAPath                    string   `json:"qaPath,omitempty"`
	CurrentReferenceSheetPath string   `json:"currentReferenceSheetPath,omitempty"`
	MasterSheetPath           string   `json:"masterSheetPath,omitempty"`
	CompleteUnitSheetPath     string   `json:"completeUnitSheetPath,omitempty"`
	CandidateSheetPath        string   `json:"candidateSheetPath,omitempty"`
	BoardMetricsPath          string   `json:"boardMetricsPath,omitempty"`
	IdentityComparisonPath    string   `json:"identityComparisonPath,omitempty"`
	OwnershipOverlayPath      string   `json:"ownershipOverlayPath,omitempty"`
	RecoveredPosePaths        []string `json:"recoveredPosePaths,omitempty"`
	RecoveredPoseSheetPath    string   `json:"recoveredPoseSheetPath,omitempty"`
	ContactSheetPath          string   `json:"contactSheetPath,omitempty"`
	AnimationGIFPath          string   `json:"animationGifPath,omitempty"`
	AnimationBoardPaths       []string `json:"animationBoardPaths,omitempty"`
	AnimationGIFPaths         []string `json:"animationGifPaths,omitempty"`
	FramePaths                []string `json:"framePaths,omitempty"`
}

type Attempt struct {
	ID                string              `json:"id"`
	CreatedAt         string              `json:"createdAt"`
	Metadata          map[string]string   `json:"metadata,omitempty"`
	References        []ReferenceEvidence `json:"references,omitempty"`
	Candidates        []Candidate         `json:"candidates,omitempty"`
	SelectedCandidate string              `json:"selectedCandidate,omitempty"`
	PoseGuideSHA256   string              `json:"poseGuideSha256,omitempty"`
	Kind              string              `json:"kind,omitempty"`
}

type Candidate struct {
	ID             string                 `json:"id"`
	QualityVersion int                    `json:"qualityVersion,omitempty"`
	RawPath        string                 `json:"rawPath,omitempty"`
	NormalizedPath string                 `json:"normalizedPath"`
	MetricsPath    string                 `json:"metricsPath,omitempty"`
	Metrics        imageio.Metrics        `json:"metrics"`
	StudyMetrics   *imageio.StudyMetrics  `json:"studyMetrics,omitempty"`
	Palette        []imageio.PaletteColor `json:"palette,omitempty"`
	HardRejections []string               `json:"hardRejections,omitempty"`
	Warnings       []string               `json:"warnings,omitempty"`
	Metadata       map[string]string      `json:"metadata,omitempty"`
}

type ReferenceEvidence struct {
	ID             string `json:"id"`
	Role           string `json:"role"`
	Authority      string `json:"authority"`
	Description    string `json:"description,omitempty"`
	SourcePath     string `json:"sourcePath"`
	SourceSHA256   string `json:"sourceSha256"`
	SentPath       string `json:"sentPath,omitempty"`
	SentSHA256     string `json:"sentSha256,omitempty"`
	ProviderIndex  int    `json:"providerIndex"`
	SentToProvider bool   `json:"sentToProvider"`
}

type ReviewRecord struct {
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	ReviewedAt string `json:"reviewedAt"`
}

type DeployRecord struct {
	Path       string   `json:"path"`
	GroupID    string   `json:"groupId,omitempty"`
	DeployedAt string   `json:"deployedAt"`
	Skipped    []string `json:"skipped,omitempty"`
}

type ProductionEvidence struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
}

type Options struct {
	OutputDir string
	DeployDir string
	RunID     string
	Filter    targets.Filter
	Force     bool
	Progress  func(ProgressEvent)
}

type Result struct {
	RunID          string
	Generated      int
	Skipped        int
	AwaitingReview int
}

func AutoRunID(now time.Time, outputDir string) (string, error) {
	minutes := now.Hour()*60 + now.Minute()
	base := fmt.Sprintf("%04d-%02d-%02d-m%04d", now.Year(), now.Month(), now.Day(), minutes)
	candidate := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(outputDir, "runs", candidate)); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%02d", base, i)
	}
}

func Run(ctx context.Context, all []targets.Target, gen provider.Provider, opts Options) (Result, error) {
	selected, err := targets.Select(all, opts.Filter)
	if err != nil {
		return Result{}, err
	}
	if opts.RunID == "" || opts.RunID == "auto" {
		runID, err := AutoRunID(time.Now(), opts.OutputDir)
		if err != nil {
			return Result{}, err
		}
		opts.RunID = runID
	}
	if err := validateRunID(opts.RunID); err != nil {
		return Result{}, err
	}
	plan, err := buildAnimatedPlan(all, selected, opts.Filter)
	if err != nil {
		return Result{}, err
	}
	if err := preflightAnimated(plan, gen.Capabilities(), opts.DeployDir); err != nil {
		return Result{}, err
	}
	manifest, err := LoadOrCreate(opts.OutputDir, opts.RunID, all)
	if err != nil {
		return Result{}, err
	}
	if err := validateAnimatedStart(manifest, plan, opts); err != nil {
		return Result{}, err
	}
	if err := captureProductionEvidence(manifest, selected, opts.DeployDir, opts.Force && len(plan.Units) == 0); err != nil {
		return Result{}, err
	}
	if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return Result{}, err
	}
	return runAnimatedWorkflow(ctx, selected, plan, gen, opts, manifest)
}

func captureProductionEvidence(manifest *Manifest, selected []targets.Target, deployDir string, refresh bool) error {
	if deployDir == "" {
		return nil
	}
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		if state == nil {
			return fmt.Errorf("target %q is missing from run manifest", target.ID)
		}
		if state.Production != nil && !refresh {
			continue
		}
		path, err := targets.DeployPath(deployDir, target)
		if err != nil {
			return err
		}
		evidence := &ProductionEvidence{Path: path}
		if _, err := os.Stat(path); err == nil {
			evidence.Exists = true
			evidence.SHA256, err = fileSHA256(path)
			if err != nil {
				return fmt.Errorf("hash production target %q: %w", target.ID, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect production target %q: %w", target.ID, err)
		}
		state.Production = evidence
	}
	return nil
}

func generateStaticTarget(ctx context.Context, gen provider.Provider, opts Options, manifest *Manifest, target targets.Target, posePath string, state *TargetState, force bool, current, total int) error {
	attemptIndex := -1
	if state.Status == StatusPending && len(state.Attempts) > 0 {
		latest := len(state.Attempts) - 1
		if state.Attempts[latest].SelectedCandidate == "" && len(state.Attempts[latest].Candidates) < 1 {
			attemptIndex = latest
		}
	}
	if attemptIndex < 0 {
		state.Review = nil
		state.Status = StatusPending
		state.SourceCandidate = ""
		state.ProductionEligible = false
		state.HardRejections = nil
		attemptIndex = len(state.Attempts)
		state.Attempts = append(state.Attempts, Attempt{ID: fmt.Sprintf("%03d", attemptIndex+1), CreatedAt: time.Now().UTC().Format(time.RFC3339)})
	}
	attempt := &state.Attempts[attemptIndex]
	attemptID := attempt.ID
	targetDir := TargetDir(opts.OutputDir, opts.RunID, target.ID)
	attemptDir := filepath.Join(targetDir, "attempts", attemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return err
	}
	inputs := filterInputs(target.Inputs, conditioning.RoleStyle, conditioning.RoleIdentity)
	reviewOnly := filterInputs(target.Inputs, conditioning.RolePose)
	var palette []imageio.PaletteColor
	if posePath != "" {
		inputs = append(inputs, conditioning.Input{Role: conditioning.RoleIdentity, Path: posePath, Description: "Existing production asset used for identity, layout, and category palette.", Required: true})
		var err error
		palette, err = imageio.PaletteFromPNG(posePath, 32)
		if err != nil {
			return err
		}
	}
	inputs, err := hydrateInputHashes(inputs)
	if err != nil {
		return err
	}
	evidence, err := collectReferenceEvidence(inputs, reviewOnly)
	if err != nil {
		return err
	}
	opaqueTile := target.RenderMode == pack.RenderModeOpaqueTile
	protocol := "Return one isolated subject only. No sheet, label, border, shadow, or scenery. Use clean clustered color ramps and minimal dithering. " + removableBackgroundInstruction
	if opaqueTile {
		protocol = "Return one full-bleed opaque terrain texture. Fill every pixel to every edge: no transparency, empty background, padding, margin, isolated slab, floating island, frame, or border. The left and right edges and the top and bottom edges must join without a visible seam when this image is repeated. Preserve a consistent orthographic ground-plane scale across the entire image. Use clean clustered color ramps and minimal dithering."
	}
	prompt := strings.TrimSpace(target.Prompt) + "\n\n" + protocol + "\n"
	promptPath := filepath.Join(targetDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return err
	}
	state.Artifacts.PromptPath = promptPath
	attempt.References = evidence
	state.Artifacts.EvidencePath = filepath.Join(attemptDir, "evidence.json")
	if err := writeEvidence(state.Artifacts.EvidencePath, evidence); err != nil {
		return err
	}
	if posePath != "" {
		attempt.PoseGuideSHA256, err = fileSHA256(posePath)
		if err != nil {
			return err
		}
	}
	if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return err
	}
	opts.report(ProgressEvent{Stage: ProgressTargetGenerating, TargetID: target.ID, Current: current, Total: total})
	removeBackground := !opaqueTile
	for candidateIndex := len(attempt.Candidates); candidateIndex < 1; candidateIndex++ {
		candidateID := fmt.Sprintf("%02d", candidateIndex+1)
		opts.report(ProgressEvent{Stage: ProgressCandidateGenerating, TargetID: target.ID, Current: current, Total: total, Candidate: candidateIndex + 1, Candidates: 1})
		candidateDir := filepath.Join(attemptDir, "candidates", candidateID)
		if err := os.MkdirAll(candidateDir, 0o755); err != nil {
			return err
		}
		providerResult, err := gen.Generate(ctx, provider.Request{Prompt: prompt, Size: image.Pt(providerCanvasSize, providerCanvasSize), Inputs: inputs, CandidateOrdinal: candidateIndex + 1, Progress: func(providerCurrent, _ int) {
			opts.report(ProgressEvent{Stage: ProgressProviderProgress, TargetID: target.ID, Current: current, Total: total, Candidate: candidateIndex + 1, Candidates: 1, ProviderCurrent: providerCurrent})
		}})
		if err != nil {
			return fmt.Errorf("generate %q candidate %s: %w", target.ID, candidateID, err)
		}
		rawPath := filepath.Join(candidateDir, "raw-candidate.png")
		if err := os.WriteFile(rawPath, providerResult.PNG, 0o644); err != nil {
			return err
		}
		normalizedPath := filepath.Join(candidateDir, "normalized.png")
		candidatePalette, err := imageio.WriteNormalizedPNGWithOptions(normalizedPath, providerResult.PNG, target.Size.Width, target.Size.Height, palette, removeBackground)
		if err != nil {
			return fmt.Errorf("normalize %q candidate %s: %w", target.ID, candidateID, err)
		}
		guidePath := posePath
		if guidePath == "" {
			guidePath = normalizedPath
		}
		metrics, _, err := imageio.EvaluateCandidate(normalizedPath, guidePath, max(1, min(target.Size.Width, target.Size.Height)/32))
		if err != nil {
			return err
		}
		var hardRejections []string
		if opaqueTile {
			hasTransparency, transparencyErr := imageio.HasTransparency(normalizedPath)
			if transparencyErr != nil {
				return transparencyErr
			}
			if hasTransparency {
				hardRejections = append(hardRejections, "opaque_tile_has_transparency")
			}
		} else {
			if metrics.EdgeGuardOccupied {
				hardRejections = append(hardRejections, "edge_guard_occupied")
			}
			if metrics.Components != 1 {
				hardRejections = append(hardRejections, fmt.Sprintf("foreground_components_%d", metrics.Components))
			}
		}
		var warnings []string
		if metrics.SecondaryComponents != 0 {
			warnings = append(warnings, fmt.Sprintf("secondary_components_%d", metrics.SecondaryComponents))
		}
		if posePath != "" {
			if metrics.SilhouetteOverlap < 0.35 {
				warnings = append(warnings, "legacy_silhouette_overlap_below_threshold")
			}
			if metrics.OccupiedBoundsDelta > 0.15 {
				warnings = append(warnings, "legacy_occupied_bounds_drift")
			}
			if metrics.CenterDistance > 0.15 {
				warnings = append(warnings, "legacy_subject_center_drift")
			}
			if metrics.BaselineDelta > 0.1 {
				warnings = append(warnings, "legacy_baseline_drift")
			}
			if metrics.PaletteDistance > 0.25 {
				warnings = append(warnings, "palette_distance")
			}
		}
		metricsPath := filepath.Join(candidateDir, "metrics.json")
		if err := writeCandidateMetrics(metricsPath, metrics, hardRejections); err != nil {
			return err
		}
		attempt.Candidates = append(attempt.Candidates, Candidate{ID: candidateID, QualityVersion: candidateQualityVersion, RawPath: rawPath, NormalizedPath: normalizedPath, MetricsPath: metricsPath, Metrics: metrics, Palette: candidatePalette, HardRejections: hardRejections, Warnings: warnings, Metadata: providerResult.Metadata})
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return err
		}
		opts.report(ProgressEvent{Stage: ProgressCandidateReady, TargetID: target.ID, Current: current, Total: total, Candidate: candidateIndex + 1, Candidates: 1})
	}
	selected := bestCandidate(attempt.Candidates)
	state.CapabilityMode = "static"
	if opaqueTile {
		state.CapabilityMode = "static-opaque-tile"
	}
	state.ProductionEligible = true
	state.Normalization = &NormalizationRecord{ScaleAlgorithm: "area", PaletteMethod: "deterministic-median-cut", MaximumColors: 32, ColorSpace: "linear-srgb", Dithering: false, AlphaThreshold: 128}
	if selected == nil {
		state.Status = StatusRejected
		state.HardRejections = candidateRejections(attempt.Candidates)
		if err := writeQA(targetDir, "rejected", "The generated static candidate failed mechanical QA."); err != nil {
			return err
		}
		state.Artifacts.QAPath = filepath.Join(targetDir, "qa.md")
		return nil
	}
	state.Status = StatusAwaitingReview
	state.HardRejections = nil
	state.SourceCandidate = attempt.ID + "/" + selected.ID
	state.NormalizedPath = filepath.Join(targetDir, "normalized.png")
	state.Palette = selected.Palette
	if err := imageio.CopyFile(selected.NormalizedPath, state.NormalizedPath); err != nil {
		return err
	}
	artifacts, err := writeFrameReviewArtifacts([]string{state.NormalizedPath}, targetDir, true)
	if err != nil {
		return err
	}
	artifacts.PromptPath = promptPath
	artifacts.EvidencePath = state.Artifacts.EvidencePath
	state.Artifacts = artifacts
	attempt.SelectedCandidate = selected.ID
	if len(state.Palette) == 0 {
		state.Palette = palette
	}
	if err := imageio.WritePalette(filepath.Join(targetDir, "palette.json"), state.Palette); err != nil {
		return err
	}
	qaNote := "Needs mandatory visual QA for identity, composition, texture, edge cleanliness, and game-scale readability."
	if opaqueTile {
		qaNote = "Needs mandatory visual QA for texture identity, orthographic scale, edge-to-edge coverage, repeated-edge seams, and game-scale readability."
	}
	if err := writeQA(targetDir, "generated", qaNote); err != nil {
		return err
	}
	state.Artifacts.QAPath = filepath.Join(targetDir, "qa.md")
	opts.report(ProgressEvent{Stage: ProgressTargetReady, TargetID: target.ID, Current: current, Total: total})
	return nil
}

func LoadOrCreate(outputDir, runID string, all []targets.Target) (*Manifest, error) {
	if manifest, err := Load(outputDir, runID); err == nil {
		for _, target := range all {
			if manifest.Targets[target.ID] == nil {
				manifest.Targets[target.ID] = pendingState(target)
			}
		}
		return manifest, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	manifest := &Manifest{Version: ManifestVersion, RunID: runID, Intermediates: map[string]*IntermediateState{}, Units: map[string]*UnitState{}, Targets: map[string]*TargetState{}}
	for _, target := range all {
		manifest.Targets[target.ID] = pendingState(target)
	}
	return manifest, Save(outputDir, runID, manifest)
}

func Load(outputDir, runID string) (*Manifest, error) {
	if err := validateRunID(runID); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(ManifestPath(outputDir, runID))
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	if manifest.Version != ManifestVersion {
		return nil, fmt.Errorf("run %q uses unsupported manifest v%d; start a new run with manifest v%d", runID, manifest.Version, ManifestVersion)
	}
	if manifest.Targets == nil {
		manifest.Targets = map[string]*TargetState{}
	}
	if manifest.Intermediates == nil {
		manifest.Intermediates = map[string]*IntermediateState{}
	}
	if manifest.Units == nil {
		manifest.Units = map[string]*UnitState{}
	}
	RefreshUnitStatuses(&manifest)
	return &manifest, nil
}

func Save(outputDir, runID string, manifest *Manifest) error {
	if err := validateRunID(runID); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONData(ManifestPath(outputDir, runID), data)
}

func validateRunID(runID string) error {
	if runID == "" {
		return fmt.Errorf("invalid run id %q: use letters, digits, dots, underscores, or hyphens", runID)
	}
	for index := 0; index < len(runID); index++ {
		value := runID[index]
		letter := value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
		digit := value >= '0' && value <= '9'
		if letter || digit || index > 0 && (value == '.' || value == '_' || value == '-') {
			continue
		}
		return fmt.Errorf("invalid run id %q: use letters, digits, dots, underscores, or hyphens", runID)
	}
	return nil
}

func writeJSONData(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func ManifestPath(outputDir, runID string) string {
	return filepath.Join(outputDir, "runs", runID, "manifest.json")
}

func TargetDir(outputDir, runID, targetID string) string {
	return filepath.Join(outputDir, "runs", runID, "targets", targetID)
}

func SortedTargetIDs(manifest *Manifest) []string {
	ids := make([]string, 0, len(manifest.Targets))
	for id := range manifest.Targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func pendingState(target targets.Target) *TargetState {
	return &TargetState{ID: target.ID, Status: StatusPending}
}

func shouldSkipGeneration(state *TargetState, force bool) bool {
	switch state.Status {
	case StatusAccepted, StatusDeployed:
		return true
	case StatusAwaitingReview:
		return generatedArtifactsExist(state)
	case StatusRejected:
		return !force
	default:
		return false
	}
}

func generatedArtifactsExist(state *TargetState) bool {
	if state.NormalizedPath == "" {
		return false
	}
	_, err := os.Stat(state.NormalizedPath)
	return err == nil
}

func writeCandidateMetrics(path string, metrics imageio.Metrics, hardRejections []string) error {
	document := struct {
		Metrics        imageio.Metrics `json:"metrics"`
		HardRejections []string        `json:"hardRejections,omitempty"`
	}{Metrics: metrics, HardRejections: hardRejections}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONData(path, data)
}

func collectReferenceEvidence(sent, reviewOnly []conditioning.Input) ([]ReferenceEvidence, error) {
	evidence := make([]ReferenceEvidence, 0, len(sent)+len(reviewOnly))
	providerIndex := 0
	for _, input := range sent {
		index := 0
		if input.Role != conditioning.RoleMask {
			providerIndex++
			index = providerIndex
		}
		item, err := referenceEvidence(input, true, index)
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, item)
	}
	for _, input := range reviewOnly {
		item, err := referenceEvidence(input, false, 0)
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, item)
	}
	return evidence, nil
}

func referenceEvidence(input conditioning.Input, sent bool, providerIndex int) (ReferenceEvidence, error) {
	sourcePath := input.SourcePath
	if sourcePath == "" {
		sourcePath = input.Path
	}
	sourceHash, err := fileSHA256(sourcePath)
	if err != nil {
		return ReferenceEvidence{}, fmt.Errorf("hash evidence source %q: %w", sourcePath, err)
	}
	id := input.ID
	if id == "" {
		id = input.Role.String() + ":" + filepath.Base(sourcePath)
	}
	authority := input.Authority
	if authority == "" {
		authority = input.Role.String()
	}
	item := ReferenceEvidence{ID: id, Role: input.Role.String(), Authority: authority, Description: input.Description, SourcePath: sourcePath, SourceSHA256: sourceHash, SentToProvider: sent, ProviderIndex: providerIndex}
	if sent {
		item.SentPath = input.Path
		item.SentSHA256 = input.SHA256
		if item.SentSHA256 == "" {
			item.SentSHA256, err = fileSHA256(input.Path)
			if err != nil {
				return ReferenceEvidence{}, fmt.Errorf("hash provider input %q: %w", input.Path, err)
			}
		}
	}
	return item, nil
}

func writeEvidence(path string, evidence []ReferenceEvidence) error {
	data, err := json.MarshalIndent(struct {
		Evidence []ReferenceEvidence `json:"evidence"`
	}{Evidence: evidence}, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONData(path, data)
}

func hydrateInputHashes(inputs []conditioning.Input) ([]conditioning.Input, error) {
	out := append([]conditioning.Input(nil), inputs...)
	for index := range out {
		hash, err := fileSHA256(out[index].Path)
		if err != nil {
			return nil, fmt.Errorf("hash generation input %q: %w", out[index].Path, err)
		}
		out[index].SHA256 = hash
	}
	return out, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func bestCandidate(candidates []Candidate) *Candidate {
	best := -1
	for index := range candidates {
		if len(candidates[index].HardRejections) != 0 {
			continue
		}
		if best < 0 || candidates[index].Metrics.Score > candidates[best].Metrics.Score {
			best = index
		}
	}
	if best < 0 {
		return nil
	}
	return &candidates[best]
}

func candidateRejections(candidates []Candidate) []string {
	set := map[string]bool{}
	for _, candidate := range candidates {
		for _, reason := range candidate.HardRejections {
			set[reason] = true
		}
	}
	var reasons []string
	for reason := range set {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return reasons
}

func candidateWarnings(candidates []Candidate) []string {
	set := map[string]bool{}
	for _, candidate := range candidates {
		for _, warning := range candidate.Warnings {
			set[warning] = true
		}
	}
	var warnings []string
	for warning := range set {
		warnings = append(warnings, warning)
	}
	sort.Strings(warnings)
	return warnings
}

func writeQA(targetDir, status, reason string) error {
	content := fmt.Sprintf("# QA\n\nStatus: %s\n\nReason: %s\n", status, reason)
	return os.WriteFile(filepath.Join(targetDir, "qa.md"), []byte(content), 0o644)
}

func WriteQA(targetDir, status, reason string) error { return writeQA(targetDir, status, reason) }
