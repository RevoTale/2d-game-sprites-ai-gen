// Package deploy previews and copies accepted target images into deploy directories.
package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

type Options struct {
	OutputDir string
	RunID     string
	DeployDir string
	Filter    targets.Filter
	Complete  bool
}

type Plan struct {
	Replace   []Item
	Unchanged []Item
}

type Item struct {
	TargetID string
	Path     string
	Reason   string
}

func BuildPlan(all []targets.Target, opts Options) (Plan, error) {
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	if err != nil {
		return Plan{}, err
	}
	selected := targets.FilterTargets(all, opts.Filter)
	if len(selected) == 0 {
		return Plan{}, errors.New("no targets matched deploy scope")
	}
	var plan Plan
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		path, err := RenderPath(opts.DeployDir, target)
		if err != nil {
			return plan, err
		}
		if state != nil && (state.Status == generate.StatusAccepted || state.Status == generate.StatusDeployed) && state.NormalizedPath != "" {
			plan.Replace = append(plan.Replace, Item{TargetID: target.ID, Path: path})
			continue
		}
		reason := "pending generation"
		if state != nil {
			reason = state.Status
			if state.Review != nil && state.Review.Reason != "" {
				reason += ": " + state.Review.Reason
			}
		}
		plan.Unchanged = append(plan.Unchanged, Item{TargetID: target.ID, Path: path, Reason: reason})
	}
	if opts.Complete && len(plan.Unchanged) > 0 {
		return plan, fmt.Errorf("complete deploy blocked by %d unaccepted targets", len(plan.Unchanged))
	}
	return plan, nil
}

func Execute(all []targets.Target, opts Options) (Plan, error) {
	plan, err := BuildPlan(all, opts)
	if err != nil {
		return plan, err
	}
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	if err != nil {
		return plan, err
	}
	for _, item := range plan.Replace {
		state := manifest.Targets[item.TargetID]
		if err := imageio.CopyFile(state.NormalizedPath, item.Path); err != nil {
			return plan, err
		}
		state.Status = generate.StatusDeployed
		state.DeployPath = item.Path
		state.Deploy = &generate.DeployRecord{Path: item.Path, DeployedAt: time.Now().UTC().Format(time.RFC3339), Skipped: skippedIDs(plan.Unchanged)}
	}
	return plan, generate.Save(opts.OutputDir, opts.RunID, manifest)
}

func RenderPath(deployDir string, target targets.Target) (string, error) {
	path := target.DeployTemplate
	path = strings.ReplaceAll(path, "{object}", target.ObjectID)
	path = strings.ReplaceAll(path, "{animation}", target.AnimationID)
	path = strings.ReplaceAll(path, "{frame}", target.FrameID)
	for _, variant := range target.Variants {
		path = strings.ReplaceAll(path, "{variant."+variant.AxisID+"}", variant.ValueID)
	}
	if filepath.IsAbs(path) || path == "." || strings.HasPrefix(filepath.Clean(path), "..") {
		return "", fmt.Errorf("deploy path %q is not safely relative", path)
	}
	return filepath.Join(deployDir, path), nil
}

func FormatPlan(plan Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Will replace: %d\n", len(plan.Replace))
	for _, item := range plan.Replace {
		fmt.Fprintf(&b, "  %s -> %s\n", item.TargetID, item.Path)
	}
	fmt.Fprintf(&b, "Will not change: %d\n", len(plan.Unchanged))
	for _, item := range plan.Unchanged {
		fmt.Fprintf(&b, "  %s -> %s (%s)\n", item.TargetID, item.Path, item.Reason)
	}
	return b.String()
}

func EnsureDeployDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func skippedIDs(items []Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.TargetID)
	}
	return ids
}
