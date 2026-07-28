package targets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestExpandTargetsCombinesVariantsAnimationsAndFrames(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)

	ids := targetIDs(all)
	require.Contains(t, ids, "blood-duelist__attack__direction-right__00")
	require.Contains(t, ids, "blood-duelist__attack__direction-right__contact")
	require.Contains(t, ids, "blood-duelist__attack__direction-up__00")
	require.Contains(t, ids, "grass")
}

func TestExpandRecordsAnimationAndFrameOrderForConsistencyPlanning(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)

	selected := targets.FilterTargets(all, targets.Filter{
		Object:    "blood-duelist",
		Variants:  map[string]string{"direction": "right"},
		Animation: "attack",
	})

	require.Len(t, selected, 2)
	require.Equal(t, 0, selected[0].AnimationIndex)
	require.Equal(t, 0, selected[0].FrameIndex)
	require.Equal(t, 1, selected[1].FrameIndex)
	require.Contains(t, selected[0].Prompt, "Elegant demonic duelist")
	require.Contains(t, selected[0].Prompt, "Attack motion")
}

func TestExpandInfersConditioningRolesWithoutChangingPackSchema(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	_, all := testkit.LoadTargets(t, dir)
	target := targets.FilterTargets(all, targets.Filter{Object: "blood-duelist", Animation: "attack", Frame: "contact", Variants: map[string]string{"direction": "right"}})[0]

	require.Equal(t, []conditioning.Role{
		conditioning.RoleStyle,
		conditioning.RoleIdentity,
		conditioning.RolePose,
		conditioning.RolePose,
		conditioning.RolePose,
	}, inputRoles(target.Inputs))
}

func TestDeployPathResolvesExistingTargetTemplateSafely(t *testing.T) {
	path, err := targets.DeployPath("/tmp/deploy", targets.Target{
		ID:             "knight__walk__direction-right__00",
		ObjectID:       "knight",
		AnimationID:    "walk",
		FrameID:        "00",
		Variants:       []targets.VariantSelection{{AxisID: "direction", ValueID: "right"}},
		DeployTemplate: "frames/{object}/{animation}/{variant.direction}/{frame}.png",
	})

	require.NoError(t, err)
	require.Equal(t, filepath.Join("/tmp/deploy", "frames/knight/walk/right/00.png"), path)
}

func TestFilterTargetsSelectsVariantAnimationAndFrame(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)

	selected := targets.FilterTargets(all, targets.Filter{
		Object:    "blood-duelist",
		Variants:  map[string]string{"direction": "right"},
		Animation: "attack",
		Frame:     "contact",
	})

	require.Len(t, selected, 1)
	require.Equal(t, "blood-duelist__attack__direction-right__contact", selected[0].ID)
}

func TestSelectExpandsOneFrameToItsCompleteAnimationRow(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)

	selected, err := targets.Select(all, targets.Filter{
		Object:    "blood-duelist",
		Variants:  map[string]string{"direction": "right"},
		Animation: "attack",
		Frame:     "contact",
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"blood-duelist__attack__direction-right__00",
		"blood-duelist__attack__direction-right__contact",
	}, targetIDs(selected))
}

func TestSelectRejectsEmptySelectorMatches(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)

	_, err := targets.Select(all, targets.Filter{Object: "missing"})

	require.ErrorContains(t, err, "no targets matched selector")
}

func targetIDs(all []targets.Target) []string {
	ids := make([]string, 0, len(all))
	for _, target := range all {
		ids = append(ids, target.ID)
	}
	return ids
}

func inputRoles(inputs []conditioning.Input) []conditioning.Role {
	roles := make([]conditioning.Role, len(inputs))
	for i, input := range inputs {
		roles[i] = input.Role
	}
	return roles
}

func TestExpandPreservesFrameArrayOrderWhenExplicitFrameIDsSortDifferently(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("theme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 3,
  "objects": [
    {
      "id": "duelist",
      "description": "A duelist.",
      "identityLocks": ["The same duelist appears in every frame."],
      "size": { "width": 16, "height": 16 },
      "variants": [{
        "id": "direction",
        "description": "Direction.",
        "values": [{"id":"right","description":"Right.","reference":{"path":"right.png","description":"Right reference."}}]
      }],
      "animations": [
        {
          "id": "attack",
          "description": "Attack.",
          "frames": [
            { "id": "windup", "description": "Windup." },
            { "id": "contact", "description": "Hit." }
          ]
        }
      ],
      "deploy": { "pathTemplate": "units/{object}__{animation}__{frame}.png" }
    }
  ]
}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "right.png"), testkit.PNG(t, 16, 16), 0o644))
	_, all := testkit.LoadTargets(t, dir)

	selected := targets.FilterTargets(all, targets.Filter{Object: "duelist", Animation: "attack"})

	require.Len(t, selected, 2)
	require.Equal(t, "windup", selected[0].FrameID)
	require.Equal(t, "contact", selected[1].FrameID)
}
