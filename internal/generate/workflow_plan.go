package generate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

const (
	directionSeedBoardKind = "direction-seed-board"
	animationRowKind       = "animation-row"
	providerCanvasSize     = 1024
	seedCandidateCount     = 3
	rowCandidateCount      = 1
)

type workflowPlan struct {
	StaticTargets []targets.Target
	Seeds         []seedPlan
	Rows          []rowPlan
	SelectedIDs   map[string]bool
}

type seedPlan struct {
	ID         string
	ObjectID   string
	References []targets.Target
	PosePaths  []string
	VariantKey []string
	TargetIDs  []string
}

type rowPlan struct {
	ID              string
	ObjectID        string
	AnimationID     string
	AnimationIndex  int
	VariantKey      string
	SeedID          string
	SeedCell        int
	Targets         []targets.Target
	PosePaths       []string
	ProductionPaths []string
}

func buildPlan(all, selected []targets.Target, deployDir string) (workflowPlan, error) {
	plan := workflowPlan{SelectedIDs: make(map[string]bool, len(selected))}
	selectedRows := map[string]bool{}
	selectedObjects := map[string]bool{}
	for _, target := range selected {
		plan.SelectedIDs[target.ID] = true
		if target.AnimationID == "" {
			plan.StaticTargets = append(plan.StaticTargets, target)
			continue
		}
		selectedRows[animationRowKey(target)] = true
		selectedObjects[target.ObjectID] = true
	}
	if len(selectedObjects) == 0 {
		return plan, nil
	}

	firstFrames := map[string][]targets.Target{}
	rows := map[string][]targets.Target{}
	for _, target := range all {
		if target.AnimationID == "" || !selectedObjects[target.ObjectID] {
			continue
		}
		rows[animationRowKey(target)] = append(rows[animationRowKey(target)], target)
		if target.AnimationIndex == 0 && target.FrameIndex == 0 {
			firstFrames[target.ObjectID] = append(firstFrames[target.ObjectID], target)
		}
	}

	seedByObject := map[string]seedPlan{}
	seedCellByVariant := map[string]int{}
	for _, objectID := range orderedSelectedObjects(all, selectedObjects) {
		references := firstFrames[objectID]
		if len(references) == 0 {
			return workflowPlan{}, fmt.Errorf("animated object %q has no first-animation frame 00 seed poses", objectID)
		}
		seed := seedPlan{ID: "direction-seed-board:" + objectID, ObjectID: objectID, References: references}
		for index, reference := range references {
			posePath, err := resolvePoseGuide(reference, deployDir)
			if err != nil {
				return workflowPlan{}, err
			}
			seed.PosePaths = append(seed.PosePaths, posePath)
			seed.VariantKey = append(seed.VariantKey, safeVariantKey(reference))
			seed.TargetIDs = append(seed.TargetIDs, reference.ID)
			seedCellByVariant[directionKey(reference)] = index
		}
		seedByObject[objectID] = seed
		plan.Seeds = append(plan.Seeds, seed)
	}

	for _, rowKey := range orderedSelectedRows(all, selectedRows) {
		rowTargets := append([]targets.Target(nil), rows[rowKey]...)
		if len(rowTargets) == 0 {
			return workflowPlan{}, fmt.Errorf("selected animation row %q has no targets", rowKey)
		}
		sort.Slice(rowTargets, func(i, j int) bool { return rowTargets[i].FrameIndex < rowTargets[j].FrameIndex })
		posePaths := make([]string, len(rowTargets))
		productionPaths := make([]string, len(rowTargets))
		for index, target := range rowTargets {
			posePath, err := resolvePoseGuide(target, deployDir)
			if err != nil {
				return workflowPlan{}, err
			}
			posePaths[index] = posePath
			productionPaths[index], _ = existingDeployPath(target, deployDir)
		}
		first := rowTargets[0]
		seed := seedByObject[first.ObjectID]
		plan.Rows = append(plan.Rows, rowPlan{
			ID:              "animation-row:" + safeIntermediateID(rowKey),
			ObjectID:        first.ObjectID,
			AnimationID:     first.AnimationID,
			AnimationIndex:  first.AnimationIndex,
			VariantKey:      safeVariantKey(first),
			SeedID:          seed.ID,
			SeedCell:        seedCellByVariant[directionKey(first)],
			Targets:         rowTargets,
			PosePaths:       posePaths,
			ProductionPaths: productionPaths,
		})
	}
	return plan, nil
}

func animationRowKey(target targets.Target) string {
	return directionKey(target) + "\x00animation=" + target.AnimationID
}

func orderedSelectedObjects(all []targets.Target, selected map[string]bool) []string {
	seen := map[string]bool{}
	var result []string
	for _, target := range all {
		if !selected[target.ObjectID] || seen[target.ObjectID] {
			continue
		}
		seen[target.ObjectID] = true
		result = append(result, target.ObjectID)
	}
	return result
}

func orderedSelectedRows(all []targets.Target, selected map[string]bool) []string {
	seen := map[string]bool{}
	var result []string
	for _, target := range all {
		key := animationRowKey(target)
		if !selected[key] || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, key)
	}
	return result
}

func targetIDs(values []targets.Target) []string {
	ids := make([]string, len(values))
	for index, target := range values {
		ids[index] = target.ID
	}
	sort.Strings(ids)
	return ids
}

func safeIntermediateID(id string) string {
	return strings.NewReplacer("\x00", "__", "=", "-").Replace(id)
}
