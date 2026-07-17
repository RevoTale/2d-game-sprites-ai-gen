// Package testkit provides deterministic fixtures for generator tests.
package testkit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/stretchr/testify/require"
)

func WritePackWithReferences(t *testing.T) string {
	t.Helper()
	dir := WritePack(t)
	for _, name := range []string{"style.png", "identity.png", "variant.png", "direction.png", "animation.png", "frame.png"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), PNG(t, 16, 16), 0o644))
	}
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	content = strings.Replace(content, `"deployDir": "deploy",`, `"deployDir": "deploy",
  "references": [{"path":"style.png","description":"Style."}],`, 1)
	content = strings.Replace(content, `"description": "Elegant demonic duelist.",`, `"description": "Elegant demonic duelist.",
      "references": [{"path":"identity.png","description":"Identity."}],`, 1)
	content = strings.Replace(content, `"description": "Facing direction.",`, `"description": "Facing direction.",
          "references": [{"path":"variant.png","description":"Variant pose."}],`, 1)
	content = strings.Replace(content, `{ "id": "right", "description": "Right side view." }`, `{ "id": "right", "description": "Right side view.", "references": [{"path":"direction.png","description":"Direction pose."}] }`, 1)
	content = strings.Replace(content, `"description": "Attack motion.",`, `"description": "Attack motion.",
          "references": [{"path":"animation.png","description":"Animation pose."}],`, 1)
	content = strings.Replace(content, `{ "id": "contact", "description": "Hit." }`, `{ "id": "contact", "description": "Hit.", "references": [{"path":"frame.png","description":"Frame pose."}] }`, 1)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return dir
}

func PNG(t *testing.T, width, height int) []byte {
	return PNGWithMargin(t, width, height, 2)
}

func PNGWithMargin(t *testing.T, width, height, margin int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := margin; y < height-margin; y++ {
		for x := margin; x < width-margin; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 80, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

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
	for targetIndex := range all {
		for inputIndex := range all[targetIndex].Inputs {
			if !filepath.IsAbs(all[targetIndex].Inputs[inputIndex].Path) {
				all[targetIndex].Inputs[inputIndex].Path = filepath.Join(dir, all[targetIndex].Inputs[inputIndex].Path)
			}
		}
	}
	return p, all
}
