package targets_test

import (
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestExpandV5CreatesConfiguredDirectionsAnimationsFramesAndStatics(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)

	require.Equal(t, []string{
		"blood-duelist__attack__direction-down__00",
		"blood-duelist__attack__direction-down__contact",
		"blood-duelist__attack__direction-up__00",
		"blood-duelist__attack__direction-up__contact",
		"blood-duelist__attack__direction-right__00",
		"blood-duelist__attack__direction-right__contact",
		"grass",
	}, targetIDs(all))
}

func TestExpandV5BuildsPromptOnlyFromJSONStyleAndObjectFacts(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)
	var target targets.Target
	for _, candidate := range targets.FilterTargets(all, targets.Filter{Object: "blood-duelist"}) {
		if candidate.AnimationID == "attack" && candidate.DirectionID == "right" {
			target = candidate
			break
		}
	}

	require.Contains(t, target.Prompt, "# Style: compact-dark-fantasy-tactics")
	require.Contains(t, target.Prompt, "# Unit archetype: agile-armored-humanoid")
	require.Contains(t, target.Prompt, "Elegant demonic duelist")
	require.Contains(t, target.Prompt, "# Direction: right")
	require.Contains(t, target.Prompt, "# Animation: attack")
	require.NotContains(t, target.Prompt, "# Theme")
}

func TestExpandV5SendsApprovedGuideThenObjectIdentityForMasterAuthority(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	_, all := testkit.LoadTargets(t, dir)
	target := targets.FilterTargets(all, targets.Filter{Object: "blood-duelist"})[0]

	require.Equal(t, []conditioning.Role{
		conditioning.RoleStyle,
		conditioning.RoleIdentity,
	}, inputRoles(target.Inputs))
	require.Equal(t, "compact-dark-fantasy-style-v1", target.Inputs[0].ID)
	require.Equal(t, "identity", target.Inputs[1].ID)
}

func TestDeployPathResolvesDirectionPlaceholderSafely(t *testing.T) {
	path, err := targets.DeployPath("/tmp/deploy", targets.Target{
		ID:             "knight__walk__direction-right__00",
		ObjectID:       "knight",
		AnimationID:    "walk",
		FrameID:        "00",
		DirectionID:    "right",
		DeployTemplate: "frames/{object}/{animation}/{direction}/{frame}.png",
	})

	require.NoError(t, err)
	require.Equal(t, filepath.Join("/tmp/deploy", "frames/knight/walk/right/00.png"), path)
}

func TestSelectAppliesBatchExclusionsBeforePlanning(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)

	selected, err := targets.Select(all, targets.Filter{Exclude: map[string]bool{"blood-duelist": true}})

	require.NoError(t, err)
	require.Equal(t, []string{"grass"}, targetIDs(selected))
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
