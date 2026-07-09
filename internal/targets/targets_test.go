package targets_test

import (
	"os"
	"path/filepath"
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

func TestExpandPreservesFrameArrayOrderWhenExplicitFrameIDsSortDifferently(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("theme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 1,
  "objects": [
    {
      "id": "duelist",
      "description": "A duelist.",
      "size": { "width": 16, "height": 16 },
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
	_, all := testkit.LoadTargets(t, dir)

	selected := targets.FilterTargets(all, targets.Filter{Object: "duelist", Animation: "attack"})

	require.Len(t, selected, 2)
	require.Equal(t, "windup", selected[0].FrameID)
	require.Equal(t, "contact", selected[1].FrameID)
}
