// Package testkit provides deterministic fixtures for generator tests.
package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/stretchr/testify/require"
)

func WritePack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("smooth pixel art"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 1,
  "outputDir": "output",
  "deployDir": "deploy",
  "objects": [
    {
      "id": "blood-duelist",
      "description": "Elegant demonic duelist.",
      "size": { "width": 16, "height": 16 },
      "variants": [
        {
          "id": "direction",
          "description": "Facing direction.",
          "values": [
            { "id": "right", "description": "Right side view." },
            { "id": "up", "description": "Back view." }
          ]
        }
      ],
      "animations": [
        {
          "id": "attack",
          "description": "Attack motion.",
          "frames": [
            { "description": "Ready." },
            { "id": "contact", "description": "Hit." }
          ]
        }
      ],
      "deploy": {
        "pathTemplate": "units/{object}__{animation}__{variant.direction}__{frame}.png"
      }
    },
    {
      "id": "grass",
      "description": "Smooth grass tile.",
      "size": { "width": 16, "height": 16 },
      "deploy": { "pathTemplate": "terrain/{object}.png" }
    }
  ]
}`), 0o644))
	return dir
}

func LoadTargets(t *testing.T, dir string) (*pack.Pack, []targets.Target) {
	t.Helper()
	p, theme, err := pack.Load(dir)
	require.NoError(t, err)
	all, err := targets.Expand(p, theme)
	require.NoError(t, err)
	return p, all
}
