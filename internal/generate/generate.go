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
	ManifestVersion                    = 12
	candidateQualityVersion            = 21
	AnimatedAssemblyVersion            = 8
	opaqueTileHardMeanEdgeDelta        = 0.08
	opaqueTileHardMaximumEdgeDelta     = 0.55
	opaqueTileWarningMeanEdgeDelta     = 0.03
	opaqueTileWarningSmallClusterRatio = 0.08

	StatusPending        = "pending"
	StatusAwaitingReview = "awaiting_review"
	StatusAccepted       = "accepted"
	StatusRejected       = "rejected"
	StatusDeployed       = "deployed"
	StatusReady          = "ready"

	removableBackgroundInstruction = "Render every empty pixel with one uniform flat opaque background color that is visually distinct from the subject. Choose a single high-saturation chroma-key RGB color absent from the subject palette, preferring pure green, magenta, or cyan only when that color does not occur in the subject. Never use gray, black, white, beige, or another low-saturation color as the background. Use the exact same RGB value across the entire canvas so it can be removed cleanly. Do not draw grid lines, dashed guides, checkerboards, gradients, vignettes, lighting falloff, texture, transparency, or shadows."
)

type Manifest struct {
	Version          int                           `json:"version"`
	RunID            string                        `json:"runId"`
	ConfigSHA256     string                        `json:"configSha256"`
	StyleGuideSHA256 string                        `json:"styleGuideSha256,omitempty"`
	Failures         []RunFailure                  `json:"failures,omitempty"`
	Intermediates    map[string]*IntermediateState `json:"intermediates,omitempty"`
	Units            map[string]*UnitState         `json:"units,omitempty"`
	Targets          map[string]*TargetState       `json:"targets"`
}

type RunFailure struct {
	ObjectID  string `json:"objectId"`
	Stage     string `json:"stage"`
	Error     string `json:"error"`
	Ambiguous bool   `json:"ambiguous"`
	FailedAt  string `json:"failedAt"`
}

type UnitState struct {
	ID                string                           `json:"id"`
	ObjectID          string                           `json:"objectId"`
	Status            string                           `json:"status"`
	MasterID          string                           `json:"masterId"`
	AnimationBoardIDs []string                         `json:"animationBoardIds"`
	TargetIDs         []string                         `json:"targetIds"`
	MasterLineage     string                           `json:"masterLineage,omitempty"`
	AnimationLineages map[string]string                `json:"animationLineages,omitempty"`
	AssemblyVersion   int                              `json:"assemblyVersion,omitempty"`
	Transform         *imageio.SemanticUnitTransform   `json:"transform,omitempty"`
	Profile           *imageio.CanonicalSubjectProfile `json:"profile,omitempty"`
	HardRejections    []string                         `json:"hardRejections,omitempty"`
	Artifacts         ReviewArtifacts                  `json:"artifacts,omitempty"`
	Review            *ReviewRecord                    `json:"review,omitempty"`
	Deploy            *DeployRecord                    `json:"deploy,omitempty"`
}

// IntermediateState stores one character master or one complete animation
// board. Neither intermediate is reviewed or deployed independently.
type IntermediateState struct {
	ID               string                             `json:"id"`
	Kind             string                             `json:"kind"`
	Status           string                             `json:"status,omitempty"`
	ObjectID         string                             `json:"objectId"`
	AnimationID      string                             `json:"animationId,omitempty"`
	TargetIDs        []string                           `json:"targetIds,omitempty"`
	Dependencies     []string                           `json:"dependencies,omitempty"`
	ParentID         string                             `json:"parentId,omitempty"`
	NormalizedPath   string                             `json:"normalizedPath,omitempty"`
	SourceSHA256     string                             `json:"sourceSha256,omitempty"`
	Lineage          string                             `json:"lineage,omitempty"`
	EditSourcePath   string                             `json:"editSourcePath,omitempty"`
	EditMaskPath     string                             `json:"editMaskPath,omitempty"`
	SemanticLayout   *imageio.SemanticLayout            `json:"semanticLayout,omitempty"`
	Poses            []imageio.SemanticPose             `json:"poses,omitempty"`
	ScaleCalibration *imageio.SemanticScaleCalibration  `json:"scaleCalibration,omitempty"`
	StaticSetScale   *imageio.StaticSetScaleCalibration `json:"staticSetScale,omitempty"`
	HardRejections   []string                           `json:"hardRejections,omitempty"`
	Warnings         []string                           `json:"warnings,omitempty"`
	Artifacts        ReviewArtifacts                    `json:"artifacts,omitempty"`
	Attempts         []Attempt                          `json:"attempts,omitempty"`
	Review           *ReviewRecord                      `json:"review,omitempty"`
	Deploy           *DeployRecord                      `json:"deploy,omitempty"`
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
	Warnings           []string               `json:"warnings,omitempty"`
	Artifacts          ReviewArtifacts        `json:"artifacts,omitempty"`
	Attempts           []Attempt              `json:"attempts,omitempty"`
	Review             *ReviewRecord          `json:"review,omitempty"`
	Deploy             *DeployRecord          `json:"deploy,omitempty"`
	Production         *ProductionEvidence    `json:"production,omitempty"`
	LogicalSize        pack.Size              `json:"logicalSize"`
	IntrinsicSize      pack.Size              `json:"intrinsicSize"`
	SourceDensity      int                    `json:"sourceDensity"`
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
	PromptPath                  string   `json:"promptPath,omitempty"`
	EvidencePath                string   `json:"evidencePath,omitempty"`
	QAPath                      string   `json:"qaPath,omitempty"`
	CurrentReferenceSheetPath   string   `json:"currentReferenceSheetPath,omitempty"`
	CanonicalProfilePath        string   `json:"canonicalProfilePath,omitempty"`
	CanonicalProfileOverlayPath string   `json:"canonicalProfileOverlayPath,omitempty"`
	ScaleCalibrationPath        string   `json:"scaleCalibrationPath,omitempty"`
	MasterSheetPath             string   `json:"masterSheetPath,omitempty"`
	CompleteUnitSheetPath       string   `json:"completeUnitSheetPath,omitempty"`
	CandidateSheetPath          string   `json:"candidateSheetPath,omitempty"`
	BoardMetricsPath            string   `json:"boardMetricsPath,omitempty"`
	IdentityComparisonPath      string   `json:"identityComparisonPath,omitempty"`
	OwnershipOverlayPath        string   `json:"ownershipOverlayPath,omitempty"`
	RecoveredPosePaths          []string `json:"recoveredPosePaths,omitempty"`
	RecoveredPoseSheetPath      string   `json:"recoveredPoseSheetPath,omitempty"`
	ContactSheetPath            string   `json:"contactSheetPath,omitempty"`
	AnimationGIFPath            string   `json:"animationGifPath,omitempty"`
	AnimationBoardPaths         []string `json:"animationBoardPaths,omitempty"`
	AnimationGIFPaths           []string `json:"animationGifPaths,omitempty"`
	FramePaths                  []string `json:"framePaths,omitempty"`
	NativePreviewPath           string   `json:"nativePreviewPath,omitempty"`
	PortraitPreviewPath         string   `json:"portraitPreviewPath,omitempty"`
	BattlefieldPreviewPath      string   `json:"battlefieldPreviewPath,omitempty"`
	TiledPreviewPath            string   `json:"tiledPreviewPath,omitempty"`
	RuntimeOverrideRoot         string   `json:"runtimeOverrideRoot,omitempty"`
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
	OutputDir        string
	DeployDir        string
	RunID            string
	Filter           targets.Filter
	ConfigSHA256     string
	StyleGuideSHA256 string
	ContinueOnError  bool
	Progress         func(ProgressEvent)
}

type Result struct {
	RunID          string
	Generated      int
	Skipped        int
	AwaitingReview int
	Failed         int
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
	plan, err := buildAnimatedPlan(all, selected)
	if err != nil {
		return Result{}, err
	}
	if err := preflightAnimated(plan, gen.Capabilities()); err != nil {
		return Result{}, err
	}
	manifest, err := LoadOrCreate(opts.OutputDir, opts.RunID, all)
	if err != nil {
		return Result{}, err
	}
	if err := bindManifestEvidence(manifest, opts); err != nil {
		return Result{}, err
	}
	if err := validateAnimatedStart(manifest, plan); err != nil {
		return Result{}, err
	}
	if err := captureProductionEvidence(manifest, selected, opts.DeployDir); err != nil {
		return Result{}, err
	}
	if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return Result{}, err
	}
	return runAnimatedWorkflow(ctx, selected, plan, gen, opts, manifest)
}

func bindManifestEvidence(manifest *Manifest, opts Options) error {
	if manifest.ConfigSHA256 != "" && opts.ConfigSHA256 != "" &&
		manifest.ConfigSHA256 != opts.ConfigSHA256 {
		return errors.New("sprites.json changed after the run started; start a new run")
	}
	if manifest.StyleGuideSHA256 != "" && opts.StyleGuideSHA256 != "" &&
		manifest.StyleGuideSHA256 != opts.StyleGuideSHA256 {
		return errors.New("approved style guide changed after the run started; start a new run")
	}
	if manifest.ConfigSHA256 == "" {
		manifest.ConfigSHA256 = opts.ConfigSHA256
	}
	if manifest.StyleGuideSHA256 == "" {
		manifest.StyleGuideSHA256 = opts.StyleGuideSHA256
	}
	return nil
}

func captureProductionEvidence(manifest *Manifest, selected []targets.Target, deployDir string) error {
	if deployDir == "" {
		return nil
	}
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		if state == nil {
			return fmt.Errorf("target %q is missing from run manifest", target.ID)
		}
		if state.Production != nil {
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

func generateStaticTarget(ctx context.Context, gen provider.Provider, opts Options, manifest *Manifest, target targets.Target, state *TargetState, current, total int) error {
	attemptIndex := -1
	if state.Status == StatusPending && len(state.Attempts) > 0 {
		latest := len(state.Attempts) - 1
		if state.Attempts[latest].SelectedCandidate == "" && len(state.Attempts[latest].Candidates) < 1 {
			attemptIndex = latest
		}
	}
	if state.Status == StatusAwaitingReview && len(state.Attempts) > 0 {
		latest := len(state.Attempts) - 1
		if candidateNeedsReprocessing(state.Attempts[latest]) {
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
	styleGuide := target.ObjectKind == targets.StyleGuideTargetID
	inputs := filterInputs(target.Inputs, conditioning.RoleStyle, conditioning.RoleIdentity)
	reviewOnly := filterInputs(target.Inputs, conditioning.RolePose)
	var palette []imageio.PaletteColor
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
		protocol = "Return one full-bleed opaque terrain texture. Fill every pixel to every edge: no transparency, empty background, padding, margin, isolated slab, floating island, frame, or border. The left and right edges and the top and bottom edges must join without a visible seam when this image is repeated in a 3x3 field. Keep the same quiet boundary material and value distribution on all four edges. Preserve a consistent orthographic ground-plane scale across the entire image. Do not compose a centered focal patch, framed square, axis-aligned region, unique landmark, or per-tile vignette. Use broad connected color planes, clean clustered ramps, and no dithering or single-pixel grain."
	}
	if styleGuide {
		protocol = "Return one original full-bleed opaque visual style board at the requested dimensions. Arrange only the configured examples in a clean comparison composition. Do not add labels, UI frames, copied characters, copied terrain, logos, or proprietary motifs."
	}
	prompt := renderProviderPrompt(
		strings.TrimSpace(target.Prompt)+"\n\n"+protocol+"\n",
		inputs,
	)
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
		requestSize := image.Pt(
			max(providerCanvasSize, target.Size.Width),
			max(providerCanvasSize, target.Size.Height),
		)
		providerResult, err := gen.Generate(ctx, provider.Request{Prompt: prompt, Size: requestSize, Inputs: inputs, CandidateOrdinal: candidateIndex + 1, Progress: func(providerCurrent, _ int) {
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
		metricsPath := filepath.Join(candidateDir, "metrics.json")
		candidate := Candidate{
			ID:             candidateID,
			RawPath:        rawPath,
			NormalizedPath: normalizedPath,
			MetricsPath:    metricsPath,
			Metadata:       providerResult.Metadata,
		}
		if err := normalizeStaticCandidate(
			&candidate,
			target,
			styleGuide,
			opaqueTile,
			palette,
			removeBackground,
		); err != nil {
			return err
		}
		attempt.Candidates = append(attempt.Candidates, candidate)
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return err
		}
		opts.report(ProgressEvent{Stage: ProgressCandidateReady, TargetID: target.ID, Current: current, Total: total, Candidate: candidateIndex + 1, Candidates: 1})
	}
	for candidateIndex := range attempt.Candidates {
		candidate := &attempt.Candidates[candidateIndex]
		if candidate.QualityVersion >= candidateQualityVersion {
			continue
		}
		if err := normalizeStaticCandidate(
			candidate,
			target,
			styleGuide,
			opaqueTile,
			palette,
			removeBackground,
		); err != nil {
			return err
		}
		if err := Save(opts.OutputDir, opts.RunID, manifest); err != nil {
			return err
		}
	}
	selected := bestCandidate(attempt.Candidates)
	state.CapabilityMode = "static"
	if opaqueTile {
		state.CapabilityMode = "static-opaque-tile"
	}
	if styleGuide {
		state.CapabilityMode = "style-guide"
	}
	state.ProductionEligible = true
	maximumColors := 32
	paletteMethod := "deterministic-median-cut"
	if styleGuide {
		maximumColors = imageio.CompositePaletteSize
		paletteMethod = "deterministic-composite-median-cut"
	}
	scaleAlgorithm := "full-canvas-area"
	anchor := ""
	if !styleGuide && !opaqueTile {
		scaleAlgorithm = "alpha-bounds-area-fit"
		anchor = target.RegistrationMode
	}
	state.Normalization = &NormalizationRecord{
		ScaleAlgorithm: scaleAlgorithm,
		PaletteMethod:  paletteMethod,
		MaximumColors:  maximumColors,
		ColorSpace:     "linear-srgb",
		Dithering:      false,
		AlphaThreshold: 128,
		Anchor:         anchor,
	}
	if opaqueTile && !styleGuide && len(attempt.Candidates) != 0 {
		latest := attempt.Candidates[len(attempt.Candidates)-1]
		state.Artifacts.TiledPreviewPath = filepath.Join(targetDir, "review", "tiled-repeat-3x3.png")
		if err := imageio.WriteTiledRepeatPreview(
			latest.NormalizedPath,
			state.Artifacts.TiledPreviewPath,
			3,
			3,
		); err != nil {
			return err
		}
	}
	if selected == nil {
		state.Status = StatusRejected
		state.ProductionEligible = false
		state.HardRejections = candidateRejections(attempt.Candidates)
		state.Warnings = nil
		if err := writeQA(targetDir, "rejected", "The generated static candidate failed mechanical QA."); err != nil {
			return err
		}
		state.Artifacts.QAPath = filepath.Join(targetDir, "qa.md")
		return nil
	}
	state.Status = StatusAwaitingReview
	state.HardRejections = nil
	state.Warnings = append([]string(nil), selected.Warnings...)
	state.SourceCandidate = attempt.ID + "/" + selected.ID
	state.NormalizedPath = filepath.Join(targetDir, "normalized.png")
	state.LogicalSize = target.Size
	state.IntrinsicSize = staticOutputSize(target, styleGuide)
	state.SourceDensity = 2
	if styleGuide {
		state.SourceDensity = 1
	}
	state.Palette = selected.Palette
	if err := imageio.CopyFile(selected.NormalizedPath, state.NormalizedPath); err != nil {
		return err
	}
	artifacts, err := writeFrameReviewArtifacts([]string{state.NormalizedPath}, targetDir, false)
	if err != nil {
		return err
	}
	artifacts.PromptPath = promptPath
	artifacts.EvidencePath = state.Artifacts.EvidencePath
	artifacts.NativePreviewPath = state.NormalizedPath
	if opaqueTile && !styleGuide {
		artifacts.TiledPreviewPath = state.Artifacts.TiledPreviewPath
	}
	if styleGuide {
		portraitPath := filepath.Join(targetDir, "review", "style-guide-96.png")
		normalizedPNG, readErr := os.ReadFile(state.NormalizedPath)
		if readErr != nil {
			return readErr
		}
		if _, normalizeErr := imageio.WriteReviewPreviewPNG(
			portraitPath,
			normalizedPNG,
			96,
			96,
			selected.Palette,
		); normalizeErr != nil {
			return normalizeErr
		}
		artifacts.PortraitPreviewPath = portraitPath
	} else {
		battlefieldPath := filepath.Join(targetDir, "review", "battlefield-preview-96.png")
		normalizedPNG, readErr := os.ReadFile(state.NormalizedPath)
		if readErr != nil {
			return readErr
		}
		if _, normalizeErr := imageio.WriteReviewPreviewPNG(
			battlefieldPath,
			normalizedPNG,
			96,
			96,
			selected.Palette,
		); normalizeErr != nil {
			return normalizeErr
		}
		artifacts.BattlefieldPreviewPath = battlefieldPath
	}
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
	if styleGuide {
		qaNote = "Needs mandatory visual QA for originality, compact shape language, material hierarchy, cluster scale, palette discipline, and native/96px readability."
	}
	if err := writeQA(targetDir, "generated", qaNote); err != nil {
		return err
	}
	state.Artifacts.QAPath = filepath.Join(targetDir, "qa.md")
	opts.report(ProgressEvent{Stage: ProgressTargetReady, TargetID: target.ID, Current: current, Total: total})
	return nil
}

func normalizeStaticCandidate(
	candidate *Candidate,
	target targets.Target,
	styleGuide, opaqueTile bool,
	palette []imageio.PaletteColor,
	removeBackground bool,
) error {
	raw, err := os.ReadFile(candidate.RawPath)
	if err != nil {
		return fmt.Errorf("read %q candidate %s raw png: %w", target.ID, candidate.ID, err)
	}
	removableEdgeBackground := true
	if !styleGuide && !opaqueTile {
		removableEdgeBackground, err = imageio.HasRemovableEdgeBackground(raw)
		if err != nil {
			return fmt.Errorf("inspect %q candidate %s background: %w", target.ID, candidate.ID, err)
		}
	}
	outputSize := staticOutputSize(target, styleGuide)
	if styleGuide {
		candidate.Palette, err = imageio.WriteNormalizedCompositePNG(
			candidate.NormalizedPath,
			raw,
			outputSize.Width,
			outputSize.Height,
		)
	} else if opaqueTile {
		candidate.Palette, err = imageio.WriteNormalizedOpaqueTilePNG(
			candidate.NormalizedPath,
			raw,
			outputSize.Width,
			outputSize.Height,
			palette,
		)
	} else {
		candidate.Palette, err = imageio.WriteNormalizedIsolatedPNG(
			candidate.NormalizedPath,
			raw,
			outputSize.Width,
			outputSize.Height,
			palette,
			imageio.SubjectRegistrationMode(target.RegistrationMode),
		)
	}
	if err != nil {
		return fmt.Errorf("normalize %q candidate %s: %w", target.ID, candidate.ID, err)
	}
	candidate.Metrics, _, err = imageio.EvaluateCandidate(
		candidate.NormalizedPath,
		candidate.NormalizedPath,
		max(1, min(outputSize.Width, outputSize.Height)/32),
	)
	if err != nil {
		return err
	}
	staticEvidence, err := imageio.MeasureStaticEvidence(candidate.NormalizedPath)
	if err != nil {
		return err
	}
	candidate.Metrics.OpaqueRatio = staticEvidence.OpaqueRatio
	candidate.Metrics.HorizontalEdgeDelta = staticEvidence.HorizontalEdgeDelta
	candidate.Metrics.VerticalEdgeDelta = staticEvidence.VerticalEdgeDelta
	candidate.Metrics.MaximumHorizontalEdgeDelta = staticEvidence.MaximumHorizontalEdgeDelta
	candidate.Metrics.MaximumVerticalEdgeDelta = staticEvidence.MaximumVerticalEdgeDelta
	candidate.Metrics.SmallClusterRatio = staticEvidence.SmallClusterRatio
	candidate.Metrics.LuminanceRange = staticEvidence.LuminanceRange
	candidate.HardRejections = nil
	if opaqueTile {
		hasTransparency, transparencyErr := imageio.HasTransparency(candidate.NormalizedPath)
		if transparencyErr != nil {
			return transparencyErr
		}
		if hasTransparency {
			candidate.HardRejections = append(candidate.HardRejections, "opaque_tile_has_transparency")
		}
		if !styleGuide && (max(candidate.Metrics.HorizontalEdgeDelta, candidate.Metrics.VerticalEdgeDelta) > opaqueTileHardMeanEdgeDelta ||
			max(candidate.Metrics.MaximumHorizontalEdgeDelta, candidate.Metrics.MaximumVerticalEdgeDelta) > opaqueTileHardMaximumEdgeDelta) {
			candidate.HardRejections = append(candidate.HardRejections, "opaque_tile_severe_edge_mismatch")
		}
	} else {
		if !removableEdgeBackground {
			candidate.HardRejections = append(
				candidate.HardRejections,
				"foreground_is_nonremovable_backdrop",
			)
		}
		if candidate.Metrics.EdgeGuardOccupied {
			candidate.HardRejections = append(candidate.HardRejections, "edge_guard_occupied")
		}
		if candidate.Metrics.Components != 1 {
			candidate.HardRejections = append(
				candidate.HardRejections,
				fmt.Sprintf("foreground_components_%d", candidate.Metrics.Components),
			)
		}
	}
	candidate.Warnings = nil
	if candidate.Metrics.SecondaryComponents != 0 {
		candidate.Warnings = append(
			candidate.Warnings,
			fmt.Sprintf("secondary_components_%d", candidate.Metrics.SecondaryComponents),
		)
	}
	if opaqueTile && !styleGuide && max(candidate.Metrics.HorizontalEdgeDelta, candidate.Metrics.VerticalEdgeDelta) > opaqueTileWarningMeanEdgeDelta {
		candidate.Warnings = append(candidate.Warnings, "opaque_tile_edge_mismatch_needs_repeat_review")
	}
	if opaqueTile && !styleGuide && candidate.Metrics.SmallClusterRatio > opaqueTileWarningSmallClusterRatio {
		candidate.Warnings = append(candidate.Warnings, "opaque_tile_micro_clusters_need_noise_review")
	}
	candidate.QualityVersion = candidateQualityVersion
	if candidate.MetricsPath == "" {
		candidate.MetricsPath = filepath.Join(
			filepath.Dir(candidate.NormalizedPath),
			"metrics.json",
		)
	}
	return writeCandidateMetrics(
		candidate.MetricsPath,
		candidate.Metrics,
		candidate.HardRejections,
	)
}

func staticOutputSize(target targets.Target, styleGuide bool) pack.Size {
	if styleGuide {
		return target.Size
	}
	return pack.Size{
		Width:  target.Size.Width * 2,
		Height: target.Size.Height * 2,
	}
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
	return &TargetState{
		ID:          target.ID,
		Status:      StatusPending,
		LogicalSize: target.Size,
	}
}

func shouldSkipGeneration(state *TargetState) bool {
	switch state.Status {
	case StatusAccepted, StatusDeployed:
		return true
	case StatusAwaitingReview:
		return generatedArtifactsExist(state) && !targetNeedsReprocessing(state)
	case StatusRejected:
		return true
	default:
		return false
	}
}

func targetNeedsReprocessing(state *TargetState) bool {
	if len(state.Attempts) == 0 {
		return false
	}
	return candidateNeedsReprocessing(state.Attempts[len(state.Attempts)-1])
}

func candidateNeedsReprocessing(attempt Attempt) bool {
	if attempt.SelectedCandidate == "" {
		return false
	}
	for _, candidate := range attempt.Candidates {
		if candidate.ID == attempt.SelectedCandidate {
			return candidate.QualityVersion < candidateQualityVersion
		}
	}
	return false
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
		providerIndex++
		item, err := referenceEvidence(input, true, providerIndex)
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

func renderProviderPrompt(prompt string, inputs []conditioning.Input) string {
	if len(inputs) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n# Image References\n")
	for index, input := range inputs {
		fmt.Fprintf(
			&b,
			"%02d. [%s] %s",
			index+1,
			input.Role.String(),
			filepath.Base(input.Path),
		)
		if input.Description != "" {
			fmt.Fprintf(&b, ": %s", input.Description)
		}
		b.WriteByte('\n')
	}
	return b.String()
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
