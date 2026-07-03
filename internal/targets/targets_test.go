package targets_test

import (
	"testing"

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

func targetIDs(all []targets.Target) []string {
	ids := make([]string, 0, len(all))
	for _, target := range all {
		ids = append(ids, target.ID)
	}
	return ids
}
