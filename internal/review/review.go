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
	animatedObjects := map[string]bool{}
	var staticTargets []targets.Target
	for _, target := range selected {
		if target.AnimationID == "" {
			staticTargets = append(staticTargets, target)
		} else {
			animatedObjects[target.ObjectID] = true
		}
	}
	if len(animatedObjects) != 0 && (opts.Filter.Animation != "" || opts.Filter.Frame != "" || len(opts.Filter.Variants) != 0) {
		return Result{}, errors.New("animated review is unit-atomic; select only --object")
	}
	for objectID := range animatedObjects {
		unit := manifest.Units["unit:"+objectID]
		if unit == nil || unit.Status != generate.StatusAwaitingReview {
			if unit != nil {
				result.SkippedPending += len(unit.TargetIDs)
			}
			continue
		}
		reviewedAt := time.Now().UTC().Format(time.RFC3339)
		unit.Status = opts.Status
		unit.Review = &generate.ReviewRecord{Status: opts.Status, Reason: opts.Reason, ReviewedAt: reviewedAt}
		for _, targetID := range unit.TargetIDs {
			state := manifest.Targets[targetID]
			if state == nil || state.NormalizedPath == "" {
				return result, fmt.Errorf("unit %q target %q is missing normalized output", objectID, targetID)
			}
			state.Status = opts.Status
			state.Review = &generate.ReviewRecord{Status: opts.Status, Reason: opts.Reason, ReviewedAt: reviewedAt}
			result.Reviewed++
		}
		unitDir := filepath.Join(opts.OutputDir, "runs", opts.RunID, "units", objectID)
		if err := generate.WriteQA(unitDir, opts.Status, opts.Reason); err != nil {
			return result, err
		}
		unit.Artifacts.QAPath = filepath.Join(unitDir, "qa.md")
	}
	for _, group := range targets.AtomicGroups(staticTargets) {
		ready := true
		for _, target := range group {
			state := manifest.Targets[target.ID]
			if state == nil || state.Status != generate.StatusAwaitingReview || state.NormalizedPath == "" {
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
	}
	generate.RefreshUnitStatuses(manifest)
	if err := generate.Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		return result, err
	}
	return result, nil
}
