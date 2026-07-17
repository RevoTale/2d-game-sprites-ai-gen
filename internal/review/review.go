// Package review records visual QA decisions for generated targets.
package review

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

type Options struct {
	OutputDir string
	RunID     string
	Filter    targets.Filter
	Status    string
	Reason    string
	Stage     string
	Candidate string
}

type Result struct {
	Reviewed       int
	SkippedPending int
	Warnings       []string
}

func Apply(all []targets.Target, opts Options) (Result, error) {
	if opts.Status != generate.StatusAccepted && opts.Status != generate.StatusRejected {
		return Result{}, fmt.Errorf("unsupported review status %q", opts.Status)
	}
	if opts.Status == generate.StatusRejected && opts.Reason == "" {
		return Result{}, errors.New("reject review requires --reason")
	}
	if opts.Stage != "" {
		if opts.Stage != "seed" {
			return Result{}, fmt.Errorf("unsupported review stage %q", opts.Stage)
		}
		if opts.Filter.Object == "" {
			return Result{}, errors.New("seed review requires --object")
		}
		if opts.Status == generate.StatusAccepted && opts.Candidate == "" {
			return Result{}, errors.New("seed acceptance requires --candidate")
		}
		if opts.Filter.Animation != "" || opts.Filter.Frame != "" || len(opts.Filter.Variants) != 0 {
			return Result{}, errors.New("seed review must use object-wide scope")
		}
		if opts.Status == generate.StatusAccepted && opts.Reason == "" {
			opts.Reason = "Directional seed board accepted by visual review."
		}
		if err := generate.SelectSeedCandidate(all, opts.OutputDir, opts.RunID, opts.Filter.Object, opts.Candidate, opts.Status, opts.Reason); err != nil {
			return Result{}, err
		}
		manifest, err := generate.Load(opts.OutputDir, opts.RunID)
		if err != nil {
			return Result{}, err
		}
		seed := manifest.Intermediates["direction-seed-board:"+opts.Filter.Object]
		result := Result{Reviewed: 1}
		if seed != nil && opts.Status == generate.StatusAccepted {
			result.Warnings = append([]string(nil), seed.Warnings...)
		}
		return result, nil
	}
	if opts.Candidate != "" {
		return Result{}, errors.New("--candidate is valid only for directional-seed review with --stage seed")
	}
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	if err != nil {
		return Result{}, err
	}
	selected, err := targets.Select(all, opts.Filter)
	if err != nil {
		return Result{}, err
	}
	if opts.Status == generate.StatusAccepted && opts.Reason == "" {
		opts.Reason = "Accepted by manual visual review."
	}
	var result Result
	for _, group := range targets.AtomicGroups(selected) {
		ready := true
		for _, target := range group {
			state := manifest.Targets[target.ID]
			if state == nil || state.Status == generate.StatusPending || state.NormalizedPath == "" {
				ready = false
				break
			}
		}
		if !ready {
			result.SkippedPending += len(group)
			continue
		}
		reviewedAt := time.Now().UTC().Format(time.RFC3339)
		for _, target := range group {
			state := manifest.Targets[target.ID]
			state.Status = opts.Status
			state.Review = &generate.ReviewRecord{Status: opts.Status, Reason: opts.Reason, ReviewedAt: reviewedAt}
			targetDir := filepath.Join(opts.OutputDir, "runs", opts.RunID, "targets", target.ID)
			if err := generate.WriteQA(targetDir, opts.Status, opts.Reason); err != nil {
				return result, err
			}
			state.Artifacts.QAPath = filepath.Join(targetDir, "qa.md")
			if target.AnimationID == "" && state.Artifacts.PromptPath == "" {
				state.Artifacts.PromptPath = filepath.Join(targetDir, "prompt.md")
			}
			result.Reviewed++
		}
		if group[0].AnimationID != "" {
			rowID := manifest.Targets[group[0].ID].AnimationRowID
			row := manifest.Intermediates[rowID]
			if row == nil {
				return result, fmt.Errorf("animation row %q is missing from run manifest", rowID)
			}
			row.Status = opts.Status
			row.Review = &generate.ReviewRecord{Status: opts.Status, Reason: opts.Reason, ReviewedAt: reviewedAt}
			if row.NormalizedPath == "" {
				return result, fmt.Errorf("animation row %q has no normalized review artifact", rowID)
			}
			rowDir := filepath.Dir(row.NormalizedPath)
			if err := generate.WriteQA(rowDir, opts.Status, opts.Reason); err != nil {
				return result, err
			}
			row.Artifacts.QAPath = filepath.Join(rowDir, "qa.md")
			if row.Artifacts.PromptPath == "" {
				row.Artifacts.PromptPath = filepath.Join(rowDir, "prompt.md")
			}
		}
	}
	if err := generate.Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return result, err
	}
	return result, nil
}
