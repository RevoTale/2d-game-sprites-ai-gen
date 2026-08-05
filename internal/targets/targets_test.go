package targets_test

import (
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestExpandV6CreatesConfiguredDirectionsAnimationsFramesAndStatics(t *testing.T) {
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

func TestExpandV6BuildsPromptOnlyFromJSONStyleAndObjectFacts(t *testing.T) {
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

func TestExpandV6SendsApprovedGuideThenObjectIdentityForMasterAuthority(t *testing.T) {
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

// This prompt contract exists so an empty causal declaration actively blocks
// generic fantasy ornament instead of leaving the provider to fill the gap.
func TestExpandV6MakesMundaneObjectsExplicitlyNonMagical(t *testing.T) {
	dir := testkit.WritePack(t)
	_, all := testkit.LoadTargets(t, dir)
	target := targets.FilterTargets(all, targets.Filter{Object: "grass"})[0]

	require.Contains(t, target.Prompt, "# Supernatural sources\nNone declared")
	require.Contains(t, target.Prompt, "Do not invent glow, runes, magical filigree")
}

// This prompt contract keeps each effect attached to its authored cause and
// carries its visual bounds into both master and static requests.
func TestExpandV6RendersOnlyDeclaredMagicSources(t *testing.T) {
	dir := testkit.WritePack(t)
	p, _ := testkit.LoadTargets(t, dir)
	sources := []pack.MagicSource{{
		ID:          "ward-rune",
		Description: "A deliberately carved dormant ward rune.",
		Location:    "On one intact face stone.",
		Palette:     "Deep violet and a small lavender core.",
		Expression:  "Light remains inside the carved channels.",
		Limits:      []string{"No detached glow or flowing ornament."},
	}}
	p.Objects[1].MagicSources = &sources
	all, err := targets.Expand(p)
	require.NoError(t, err)
	target := targets.FilterTargets(all, targets.Filter{Object: "grass"})[0]

	require.Contains(t, target.Prompt, "# Supernatural source: ward-rune")
	require.Contains(t, target.Prompt, "On one intact face stone")
	require.Contains(t, target.Prompt, "No detached glow or flowing ornament")
	require.NotContains(t, target.Prompt, "None declared")
}

func TestExpandV6KeepsStaticSetPartsInOneAtomicGroup(t *testing.T) {
	dir := testkit.WritePack(t)
	p, _ := testkit.LoadTargets(t, dir)
	emptySources := []pack.MagicSource{}
	p.Objects = append(p.Objects, pack.Object{
		ID:           "water-cycle",
		Kind:         pack.KindStaticSet,
		Family:       "ground",
		Description:  "One coupled water material cycle.",
		MagicSources: &emptySources,
		RenderMode:   pack.RenderModeOpaqueTile,
		Registration: pack.RegistrationModeCanvas,
		Parts: []pack.StaticSetPart{
			{ID: "phase-00", Role: "First phase.", Description: "First reflection state.", Size: pack.Size{Width: 32, Height: 32}, Deploy: pack.Deploy{PathTemplate: "water/00.png"}},
			{ID: "phase-01", Role: "Second phase.", Description: "Second reflection state.", Size: pack.Size{Width: 32, Height: 32}, Deploy: pack.Deploy{PathTemplate: "water/01.png"}},
		},
	})

	all, err := targets.Expand(p)
	require.NoError(t, err)
	selected := targets.FilterTargets(all, targets.Filter{Object: "water-cycle"})

	require.Equal(t, []string{"water-cycle-part-phase-00", "water-cycle-part-phase-01"}, targetIDs(selected))
	require.Equal(t, "phase-00", selected[0].SetPartID)
	require.Equal(t, "First phase.", selected[0].SetPartRole)
	require.Equal(t, "water/00.png", selected[0].DeployTemplate)
	require.Len(t, targets.AtomicGroups(selected), 1)
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
