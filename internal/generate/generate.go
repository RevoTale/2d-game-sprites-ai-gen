// Package generate owns resumable draft runs and provider-backed target generation.
package generate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

const (
	StatusPending   = "pending"
	StatusGenerated = "generated"
	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
	StatusDeployed  = "deployed"
)

type Manifest struct {
	RunID   string                  `json:"runId"`
	Targets map[string]*TargetState `json:"targets"`
}

type TargetState struct {
	ID             string         `json:"id"`
	Status         string         `json:"status"`
	DeployPath     string         `json:"deployPath,omitempty"`
	NormalizedPath string         `json:"normalizedPath,omitempty"`
	Attempts       []Attempt      `json:"attempts,omitempty"`
	Review         *ReviewRecord  `json:"review,omitempty"`
	ReviewHistory  []ReviewRecord `json:"reviewHistory,omitempty"`
	Deploy         *DeployRecord  `json:"deploy,omitempty"`
}

type Attempt struct {
	ID        string            `json:"id"`
	RawPath   string            `json:"rawPath"`
	CreatedAt string            `json:"createdAt"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ReviewRecord struct {
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	ReviewedAt string `json:"reviewedAt"`
}

type DeployRecord struct {
	Path       string   `json:"path"`
	DeployedAt string   `json:"deployedAt"`
	Skipped    []string `json:"skipped,omitempty"`
}

type Options struct {
	OutputDir string
	RunID     string
	Filter    targets.Filter
	Force     bool
}

type Result struct {
	RunID     string
	Generated int
	Skipped   int
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
	if opts.RunID == "" || opts.RunID == "auto" {
		runID, err := AutoRunID(time.Now(), opts.OutputDir)
		if err != nil {
			return Result{}, err
		}
		opts.RunID = runID
	}
	selected := targets.FilterTargets(all, opts.Filter)
	if len(selected) == 0 {
		return Result{}, errors.New("no targets matched generation scope")
	}
	manifest, err := LoadOrCreate(opts.OutputDir, opts.RunID, all)
	if err != nil {
		return Result{}, err
	}
	result := Result{RunID: opts.RunID}
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		if state == nil {
			state = pendingState(target)
			manifest.Targets[target.ID] = state
		}
		if (state.Status == StatusAccepted || state.Status == StatusDeployed) && !opts.Force {
			result.Skipped++
			continue
		}
		if err := ensureReferenceSupport(target, gen); err != nil {
			return result, err
		}
		attemptID := fmt.Sprintf("%03d", len(state.Attempts)+1)
		targetDir := TargetDir(opts.OutputDir, opts.RunID, target.ID)
		attemptDir := filepath.Join(targetDir, "attempts", attemptID)
		if err := os.MkdirAll(attemptDir, 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(filepath.Join(targetDir, "prompt.md"), []byte(target.Prompt), 0o644); err != nil {
			return result, err
		}
		providerResult, err := gen.Generate(ctx, provider.Request{Prompt: target.Prompt, Size: image.Pt(target.Size.Width, target.Size.Height), References: providerRefs(target)})
		if err != nil {
			return result, err
		}
		rawPath := filepath.Join(attemptDir, "raw-candidate.png")
		if err := os.WriteFile(rawPath, providerResult.PNG, 0o644); err != nil {
			return result, err
		}
		normalizedPath := filepath.Join(targetDir, "normalized.png")
		if err := imageio.WriteNormalizedPNG(normalizedPath, providerResult.PNG, target.Size.Width, target.Size.Height); err != nil {
			return result, err
		}
		archiveReview(state)
		state.Status = StatusGenerated
		state.NormalizedPath = normalizedPath
		state.Attempts = append(state.Attempts, Attempt{ID: attemptID, RawPath: rawPath, CreatedAt: time.Now().UTC().Format(time.RFC3339), Metadata: providerResult.Metadata})
		state.Review = nil
		if err := writeQA(targetDir, "generated", "Needs visual QA."); err != nil {
			return result, err
		}
		result.Generated++
	}
	return result, Save(opts.OutputDir, opts.RunID, manifest)
}

func LoadOrCreate(outputDir, runID string, all []targets.Target) (*Manifest, error) {
	if m, err := Load(outputDir, runID); err == nil {
		for _, target := range all {
			if m.Targets[target.ID] == nil {
				m.Targets[target.ID] = pendingState(target)
			}
		}
		return m, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	m := &Manifest{RunID: runID, Targets: map[string]*TargetState{}}
	for _, target := range all {
		m.Targets[target.ID] = pendingState(target)
	}
	return m, Save(outputDir, runID, m)
}

func Load(outputDir, runID string) (*Manifest, error) {
	data, err := os.ReadFile(ManifestPath(outputDir, runID))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Targets == nil {
		m.Targets = map[string]*TargetState{}
	}
	return &m, nil
}

func Save(outputDir, runID string, m *Manifest) error {
	if err := os.MkdirAll(filepath.Join(outputDir, "runs", runID), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ManifestPath(outputDir, runID), append(data, '\n'), 0o644)
}

func ManifestPath(outputDir, runID string) string {
	return filepath.Join(outputDir, "runs", runID, "manifest.json")
}

func TargetDir(outputDir, runID, targetID string) string {
	return filepath.Join(outputDir, "runs", runID, "targets", targetID)
}

func SortedTargetIDs(m *Manifest) []string {
	ids := make([]string, 0, len(m.Targets))
	for id := range m.Targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func archiveReview(state *TargetState) {
	if state.Review == nil {
		return
	}
	state.ReviewHistory = append(state.ReviewHistory, *state.Review)
}

func pendingState(target targets.Target) *TargetState {
	return &TargetState{ID: target.ID, Status: StatusPending}
}

func ensureReferenceSupport(target targets.Target, gen provider.Provider) error {
	if gen.SupportsReferences() {
		return nil
	}
	for _, ref := range target.References {
		if ref.Required {
			return fmt.Errorf("target %s requires reference %q, but provider does not support references", target.ID, ref.Path)
		}
	}
	return nil
}

func providerRefs(target targets.Target) []provider.Reference {
	refs := make([]provider.Reference, 0, len(target.References))
	for _, ref := range target.References {
		refs = append(refs, provider.Reference{Path: ref.Path, Description: ref.Description, Required: ref.Required})
	}
	return refs
}

func writeQA(targetDir, status, reason string) error {
	content := fmt.Sprintf("# QA\n\nStatus: %s\n\nReason: %s\n", status, reason)
	return os.WriteFile(filepath.Join(targetDir, "qa.md"), []byte(content), 0o644)
}

func WriteQA(targetDir, status, reason string) error {
	return writeQA(targetDir, status, reason)
}

func OutputDir(p *pack.Pack, override string) string {
	if override != "" {
		return override
	}
	return pack.OutputDir(p)
}
