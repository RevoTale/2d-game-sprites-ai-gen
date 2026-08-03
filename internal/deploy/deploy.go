// Package deploy previews and atomically copies accepted target images into deploy directories.
package deploy

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
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
}

type Plan struct {
	Replace   []Item
	Unchanged []Item
}

type Item struct {
	TargetID string
	Path     string
	GroupID  string
	Reason   string
	Blocking bool
}

func BuildPlan(all []targets.Target, opts Options) (Plan, error) {
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	if err != nil {
		return Plan{}, err
	}
	selected, err := targets.Select(all, opts.Filter)
	if err != nil {
		return Plan{}, err
	}
	scope := deploymentScope(all, selected)
	groups := groupTargets(scope)
	var plan Plan
	for _, key := range sortedGroupKeys(groups) {
		group := groups[key]
		if group[0].AnimationID == "" {
			appendStaticItem(&plan, group[0], manifest, opts.DeployDir)
			continue
		}
		appendValidatedUnit(&plan, key, group, manifest, opts.DeployDir)
	}
	if blocked := blockingCount(plan.Unchanged); blocked != 0 {
		return plan, fmt.Errorf("deployment blocked by %d stale or invalid targets", blocked)
	}
	if len(plan.Replace) == 0 {
		return plan, errors.New("no accepted, not-yet-deployed targets matched deploy scope")
	}
	return plan, nil
}

func deploymentScope(all, selected []targets.Target) []targets.Target {
	wanted := map[string]bool{}
	objects := map[string]bool{}
	for _, target := range selected {
		if target.AnimationID == "" {
			wanted[target.ID] = true
		} else {
			objects[target.ObjectID] = true
		}
	}
	var scope []targets.Target
	for _, target := range all {
		if wanted[target.ID] || (target.AnimationID != "" && objects[target.ObjectID]) {
			scope = append(scope, target)
		}
	}
	return scope
}

func groupTargets(scope []targets.Target) map[string][]targets.Target {
	groups := map[string][]targets.Target{}
	for _, target := range scope {
		key := "static:" + target.ID
		if target.AnimationID != "" {
			key = "unit:" + target.ObjectID
		}
		groups[key] = append(groups[key], target)
	}
	return groups
}

func appendStaticItem(plan *Plan, target targets.Target, manifest *generate.Manifest, deployDir string) {
	path, err := RenderPath(deployDir, target)
	if err != nil {
		plan.Unchanged = append(plan.Unchanged, Item{TargetID: target.ID, Reason: err.Error(), Blocking: true})
		return
	}
	state := manifest.Targets[target.ID]
	if deployable(state, true) {
		if err := productionUnchanged(state, path); err != nil {
			plan.Unchanged = append(plan.Unchanged, Item{TargetID: target.ID, Path: path, Reason: err.Error(), Blocking: true})
			return
		}
		plan.Replace = append(plan.Replace, Item{TargetID: target.ID, Path: path})
		return
	}
	plan.Unchanged = append(plan.Unchanged, Item{TargetID: target.ID, Path: path, Reason: stateReason(state)})
}

func appendValidatedUnit(plan *Plan, unitID string, group []targets.Target, manifest *generate.Manifest, deployDir string) {
	var blockers []string
	unit := manifest.Units[unitID]
	if unit != nil && unit.Status == generate.StatusDeployed {
		for _, target := range group {
			path, _ := RenderPath(deployDir, target)
			plan.Unchanged = append(plan.Unchanged, Item{TargetID: target.ID, Path: path, GroupID: unitID, Reason: "already deployed"})
		}
		return
	}
	if unit == nil || unit.Status != generate.StatusAccepted || unit.Review == nil || unit.Review.Status != generate.StatusAccepted {
		blockers = append(blockers, "complete unit is not accepted")
	}
	for _, target := range group {
		state := manifest.Targets[target.ID]
		if !deployable(state, false) {
			blockers = append(blockers, target.ID+": "+stateReason(state))
			continue
		}
		if _, err := os.Stat(state.NormalizedPath); err != nil {
			blockers = append(blockers, target.ID+": normalized source is missing")
		} else if dimensions, dimensionErr := imageio.PNGDimensions(state.NormalizedPath); dimensionErr != nil {
			blockers = append(blockers, target.ID+": normalized source is not a decodable PNG: "+dimensionErr.Error())
		} else if expected := image.Pt(target.Size.Width, target.Size.Height); dimensions != expected {
			blockers = append(
				blockers,
				fmt.Sprintf("%s: normalized source dimensions = %v, want %v from the current pack", target.ID, dimensions, expected),
			)
		}
		if unit == nil {
			continue
		}
		if state.UnitID != unit.ID {
			blockers = append(blockers, target.ID+": unit lineage mismatch")
		}
		master := manifest.Intermediates[state.CharacterMasterID]
		if master == nil || master.Status != generate.StatusReady || master.Lineage == "" || master.Lineage != unit.MasterLineage || state.MasterLineage != master.Lineage {
			blockers = append(blockers, target.ID+": character master lineage mismatch")
		}
		board := manifest.Intermediates[state.AnimationBoardID]
		if board == nil || board.Status != generate.StatusReady || board.Lineage == "" || state.AnimationLineage != board.Lineage || unit.AnimationLineages[target.AnimationID] != board.Lineage {
			blockers = append(blockers, target.ID+": animation board lineage mismatch")
		}
		path, err := RenderPath(deployDir, target)
		if err != nil {
			blockers = append(blockers, target.ID+": "+err.Error())
			continue
		}
		if err := productionUnchanged(state, path); err != nil {
			blockers = append(blockers, target.ID+": "+err.Error())
		}
	}
	if len(blockers) != 0 {
		reason := "unit blocked: " + strings.Join(blockers, "; ")
		for _, target := range group {
			path, _ := RenderPath(deployDir, target)
			plan.Unchanged = append(plan.Unchanged, Item{TargetID: target.ID, Path: path, GroupID: unitID, Reason: reason, Blocking: true})
		}
		return
	}
	for _, target := range group {
		path, err := RenderPath(deployDir, target)
		if err != nil {
			plan.Unchanged = append(plan.Unchanged, Item{TargetID: target.ID, GroupID: unitID, Reason: err.Error()})
			continue
		}
		plan.Replace = append(plan.Replace, Item{TargetID: target.ID, Path: path, GroupID: unitID})
	}
}

func deployable(state *generate.TargetState, static bool) bool {
	if state == nil || state.NormalizedPath == "" {
		return false
	}
	if state.Status != generate.StatusAccepted {
		return false
	}
	return static || state.ProductionEligible
}

func stateReason(state *generate.TargetState) string {
	if state == nil {
		return "pending generation"
	}
	if state.NormalizedPath == "" {
		return state.Status + ": missing selected normalized output"
	}
	if !state.ProductionEligible && state.CapabilityMode != "static" {
		return state.Status + ": provider result is not production eligible"
	}
	reason := state.Status
	if state.Review != nil && state.Review.Reason != "" {
		reason += ": " + state.Review.Reason
	}
	return reason
}

func productionUnchanged(state *generate.TargetState, destination string) error {
	if state == nil || state.Production == nil {
		return errors.New("missing production hash captured at generation time")
	}
	if filepath.Clean(state.Production.Path) != filepath.Clean(destination) {
		return staleProductionError()
	}
	_, err := os.Stat(destination)
	if !state.Production.Exists {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect production destination: %w", err)
		}
		return staleProductionError()
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return staleProductionError()
		}
		return fmt.Errorf("inspect production destination: %w", err)
	}
	hash, err := fileSHA256(destination)
	if err != nil {
		return err
	}
	if hash != state.Production.SHA256 {
		return staleProductionError()
	}
	return nil
}

func staleProductionError() error {
	return errors.New("production destination changed after generation; start a new run from the edited production source")
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

func blockingCount(items []Item) int {
	count := 0
	for _, item := range items {
		if item.Blocking {
			count++
		}
	}
	return count
}

func Execute(all []targets.Target, opts Options) (Plan, error) {
	plan, err := BuildPlan(all, opts)
	if err != nil {
		return plan, err
	}
	if len(plan.Replace) == 0 {
		return plan, errors.New("no accepted, not-yet-deployed targets matched deploy scope")
	}
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	if err != nil {
		return plan, err
	}
	staged, err := stageReplacements(plan.Replace, manifest)
	if err != nil {
		return plan, err
	}
	if err := commitReplacements(staged); err != nil {
		return plan, err
	}
	deployedAt := time.Now().UTC().Format(time.RFC3339)
	for _, item := range plan.Replace {
		state := manifest.Targets[item.TargetID]
		state.Status = generate.StatusDeployed
		state.DeployPath = item.Path
		state.Deploy = &generate.DeployRecord{Path: item.Path, GroupID: item.GroupID, DeployedAt: deployedAt, Skipped: skippedIDs(plan.Unchanged)}
	}
	markDeployedUnits(manifest, plan.Replace, deployedAt)
	generate.RefreshUnitStatuses(manifest)
	if err := generate.Save(opts.OutputDir, opts.RunID, manifest); err != nil {
		if rollbackErr := rollbackReplacements(staged); rollbackErr != nil {
			return plan, fmt.Errorf("save deployment manifest: %w; rollback failed: %v", err, rollbackErr)
		}
		return plan, fmt.Errorf("save deployment manifest: %w", err)
	}
	return plan, nil
}

func markDeployedUnits(manifest *generate.Manifest, replaced []Item, deployedAt string) {
	units := map[string]bool{}
	for _, item := range replaced {
		if state := manifest.Targets[item.TargetID]; state != nil && state.UnitID != "" {
			units[state.UnitID] = true
		}
	}
	for unitID := range units {
		unit := manifest.Units[unitID]
		if unit == nil {
			continue
		}
		deployed := len(unit.TargetIDs) != 0
		for _, targetID := range unit.TargetIDs {
			state := manifest.Targets[targetID]
			if state == nil || state.Status != generate.StatusDeployed {
				deployed = false
				break
			}
		}
		if deployed {
			unit.Status = generate.StatusDeployed
			unit.Deploy = &generate.DeployRecord{Path: unitID, DeployedAt: deployedAt}
		}
	}
}

type stagedFile struct {
	temporary    string
	destination  string
	previous     []byte
	previousMode os.FileMode
	hadPrevious  bool
}

func stageReplacements(items []Item, manifest *generate.Manifest) ([]stagedFile, error) {
	var staged []stagedFile
	cleanup := func() {
		for _, file := range staged {
			_ = os.Remove(file.temporary)
		}
	}
	for index, item := range items {
		if err := os.MkdirAll(filepath.Dir(item.Path), 0o755); err != nil {
			cleanup()
			return nil, err
		}
		temporary := filepath.Join(filepath.Dir(item.Path), fmt.Sprintf(".%s.sprites-ai-gen-%03d.tmp", filepath.Base(item.Path), index))
		stagedItem := stagedFile{temporary: temporary, destination: item.Path}
		if info, statErr := os.Stat(item.Path); statErr == nil {
			previous, readErr := os.ReadFile(item.Path)
			if readErr != nil {
				cleanup()
				return nil, readErr
			}
			stagedItem.previous = previous
			stagedItem.previousMode = info.Mode().Perm()
			stagedItem.hadPrevious = true
		} else if !errors.Is(statErr, os.ErrNotExist) {
			cleanup()
			return nil, statErr
		}
		if err := imageio.CopyFile(manifest.Targets[item.TargetID].NormalizedPath, temporary); err != nil {
			cleanup()
			_ = os.Remove(temporary)
			return nil, fmt.Errorf("stage %q: %w", item.TargetID, err)
		}
		staged = append(staged, stagedItem)
	}
	return staged, nil
}

func commitReplacements(staged []stagedFile) error {
	for index, file := range staged {
		if err := os.Rename(file.temporary, file.destination); err != nil {
			for _, remaining := range staged[index:] {
				_ = os.Remove(remaining.temporary)
			}
			rollbackErr := rollbackReplacements(staged[:index])
			if rollbackErr != nil {
				return fmt.Errorf("commit deploy file %q: %w; rollback failed: %v", file.destination, err, rollbackErr)
			}
			return fmt.Errorf("commit deploy file %q: %w", file.destination, err)
		}
	}
	return nil
}

func rollbackReplacements(committed []stagedFile) error {
	var failures []string
	for index := len(committed) - 1; index >= 0; index-- {
		file := committed[index]
		if file.hadPrevious {
			if err := os.WriteFile(file.destination, file.previous, file.previousMode); err != nil {
				failures = append(failures, fmt.Sprintf("restore %s: %v", file.destination, err))
			}
			continue
		}
		if err := os.Remove(file.destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Sprintf("remove %s: %v", file.destination, err))
		}
	}
	if len(failures) != 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func RenderPath(deployDir string, target targets.Target) (string, error) {
	return targets.DeployPath(deployDir, target)
}

func FormatPlan(plan Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Will replace: %d\n", len(plan.Replace))
	for _, item := range plan.Replace {
		fmt.Fprintf(&b, "  %s -> %s\n", item.TargetID, item.Path)
	}
	fmt.Fprintf(&b, "Will not change: %d\n", len(plan.Unchanged))
	for _, item := range plan.Unchanged {
		label := "SKIP"
		if item.Blocking {
			label = "BLOCKED"
		}
		fmt.Fprintf(&b, "  [%s] %s -> %s (%s)\n", label, item.TargetID, item.Path, item.Reason)
	}
	return b.String()
}

func EnsureDeployDir(path string) error { return os.MkdirAll(path, 0o755) }

func sortedGroupKeys(groups map[string][]targets.Target) []string {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func skippedIDs(items []Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.TargetID)
	}
	return ids
}
