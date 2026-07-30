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
  "references": [{"id":"style","role":"style","path":"style.png","description":"Style."}],`, 1)
	content = strings.Replace(content, `"description": "Elegant demonic duelist.",`, `"description": "Elegant demonic duelist.",
      "references": [{"id":"identity","role":"identity","path":"identity.png","description":"Identity."}],`, 1)
	content = strings.Replace(content, `"description": "Facing direction.",`, `"description": "Facing direction.",
          "references": [{"id":"variant-motion","role":"motion","path":"variant.png","description":"Variant pose."}],`, 1)
	content = strings.Replace(content, `{ "id": "right", "description": "Right side view." }`, `{ "id": "right", "description": "Right side view.", "references": [{"id":"direction-motion","role":"motion","path":"direction.png","description":"Direction pose."}] }`, 1)
	content = strings.Replace(content, `"description": "Attack motion.",`, `"description": "Attack motion.",
          "references": [{"id":"animation-motion","role":"motion","path":"animation.png","description":"Animation pose."}],`, 1)
	content = strings.Replace(content, `{ "id": "contact", "description": "Hit." }`, `{ "id": "contact", "description": "Hit.", "references": [{"id":"frame-motion","role":"motion","path":"frame.png","description":"Frame pose."}] }`, 1)
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
  "version": 4,
  "outputDir": "output",
  "deployDir": "deploy",
  "objects": [
    {
      "id": "blood-duelist",
      "description": "Elegant demonic duelist.",
      "identityLocks": ["Gold horned silhouette.", "Sword remains in the right hand."],
      "registration": {"mode":"grounded"},
      "size": { "width": 16, "height": 16 },
      "variants": [
        {
          "id": "direction",
          "description": "Facing direction.",
          "values": [
            { "id": "right", "description": "Right side view.", "reference": {"path":"direction-right.png","description":"Current right-facing view-geometry and registration reference; colors are not authoritative."} },
            { "id": "up", "description": "Back view.", "reference": {"path":"direction-up.png","description":"Current up-facing view-geometry and registration reference; colors are not authoritative."} }
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
	for _, name := range []string{"direction-right.png", "direction-up.png"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), PNG(t, 16, 16), 0o644))
	}
	return dir
}

func WriteFullUnitPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("smooth pixel art"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 4,
  "outputDir": "output",
  "deployDir": "deploy",
  "objects": [{
    "id": "relic-knight",
    "description": "Heavy blue-and-gold knight with a crested helmet, kite shield, and sword.",
    "identityLocks": [
      "Helmet has a swept gold crest and angular narrow visor.",
      "Shield remains a blue-and-gold kite shield with cross heraldry.",
      "Sword and shield remain on their established sides."
    ],
    "registration":{"mode":"grounded"},
    "size": {"width": 384, "height": 384},
    "variants": [{
      "id": "direction",
      "description": "Authored battlefield direction.",
      "values": [
        {"id":"down","description":"Front-facing down view.","reference":{"path":"deploy/units/relic-knight__walk__down__00.png","description":"Current down-facing view-geometry and registration reference; colors are not authoritative."}},
        {"id":"up","description":"Back-facing up view.","reference":{"path":"deploy/units/relic-knight__walk__up__00.png","description":"Current up-facing view-geometry and registration reference; colors are not authoritative."}},
        {"id":"right","description":"Side view facing right.","reference":{"path":"deploy/units/relic-knight__walk__right__00.png","description":"Current right-facing view-geometry and registration reference; colors are not authoritative."}}
      ]
    }],
    "animations": [
      {"id":"walk","description":"Grounded four-beat walk.","frames":[
        {"id":"00","description":"Neutral ready step."},
        {"id":"01","description":"Forward step."},
        {"id":"02","description":"Opposite passing step."},
        {"id":"03","description":"Settled step."}
      ]},
      {"id":"attack","description":"Sword strike with ready, windup, contact, and recovery.","frames":[
        {"id":"00","description":"Ready stance."},
        {"id":"01","description":"Windup."},
        {"id":"02","description":"Contact strike."},
        {"id":"03","description":"Recovery."}
      ]}
    ],
    "deploy":{"pathTemplate":"units/{object}__{animation}__{variant.direction}__{frame}.png"}
  }, {
    "id":"grass",
    "description":"Smooth grass tile.",
    "size":{"width":16,"height":16},
    "deploy":{"pathTemplate":"terrain/{object}.png"}
  }]
}`), 0o644))
	for _, direction := range []string{"down", "up", "right"} {
		for _, animation := range []string{"walk", "attack"} {
			for _, frame := range []string{"00", "01", "02", "03"} {
				path := filepath.Join(dir, "deploy", "units", "relic-knight__"+animation+"__"+direction+"__"+frame+".png")
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, PNGWithMargin(t, 320, 320, 40), 0o644))
			}
		}
	}
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
			if all[targetIndex].Inputs[inputIndex].SourcePath != "" && !filepath.IsAbs(all[targetIndex].Inputs[inputIndex].SourcePath) {
				all[targetIndex].Inputs[inputIndex].SourcePath = filepath.Join(dir, all[targetIndex].Inputs[inputIndex].SourcePath)
			}
		}
		for variantIndex := range all[targetIndex].Variants {
			if all[targetIndex].Variants[variantIndex].ReferencePath != "" && !filepath.IsAbs(all[targetIndex].Variants[variantIndex].ReferencePath) {
				all[targetIndex].Variants[variantIndex].ReferencePath = filepath.Join(dir, all[targetIndex].Variants[variantIndex].ReferencePath)
			}
		}
	}
	return p, all
}
