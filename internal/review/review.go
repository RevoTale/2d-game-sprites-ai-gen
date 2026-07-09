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
	OutputDir    string
	RunID        string
	Filter       targets.Filter
	Status       string
	Reason       string
	AllowPartial bool
}

type Result struct {
	Reviewed       int
	SkippedPending int
}

func Apply(all []targets.Target, opts Options) (Result, error) {
	if opts.Status != generate.StatusAccepted && opts.Status != generate.StatusRejected {
		return Result{}, fmt.Errorf("unsupported review status %q", opts.Status)
	}
	if opts.Status == generate.StatusRejected && opts.Reason == "" {
		return Result{}, errors.New("reject review requires --reason")
	}
	if opts.Status == generate.StatusAccepted && opts.Reason == "" {
		opts.Reason = "Bulk accepted by scoped review command."
	}
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	if err != nil {
		return Result{}, err
	}
	selected := targets.FilterTargets(all, opts.Filter)
	if len(selected) == 0 {
		return Result{}, errors.New("no targets matched review scope")
	}
	var result Result
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		if state == nil || state.Status == generate.StatusPending || state.NormalizedPath == "" {
			result.SkippedPending++
			continue
		}
		if state.Review != nil {
			state.ReviewHistory = append(state.ReviewHistory, *state.Review)
		}
		state.Status = opts.Status
		state.Review = &generate.ReviewRecord{Status: opts.Status, Reason: opts.Reason, ReviewedAt: time.Now().UTC().Format(time.RFC3339)}
		if err := generate.WriteQA(filepath.Join(opts.OutputDir, "runs", opts.RunID, "targets", target.ID), opts.Status, opts.Reason); err != nil {
			return result, err
		}
		result.Reviewed++
	}
	if err := generate.Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return result, err
	}
	if result.SkippedPending > 0 && !opts.AllowPartial {
		return result, fmt.Errorf("%d matched targets were pending or missing generated output", result.SkippedPending)
	}
	return result, nil
}
