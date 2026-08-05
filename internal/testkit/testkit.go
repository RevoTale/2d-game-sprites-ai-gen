// Package testkit provides deterministic fixtures for generator tests.
package testkit

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/stretchr/testify/require"
)

type StaticSetProvider struct {
	Requests  []provider.Request
	PartCount int
}

func (p *StaticSetProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true, Masks: true}
}

func (p *StaticSetProvider) Generate(
	_ context.Context,
	request provider.Request,
) (provider.Result, error) {
	p.Requests = append(p.Requests, request)
	count := p.PartCount
	if count == 0 {
		data, err := os.ReadFile(request.Inputs[0].Path)
		if err != nil {
			return provider.Result{}, err
		}
		return provider.Result{PNG: data}, nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, request.Size.X, request.Size.Y))
	for index := range count {
		anchor := image.Pt(request.Size.X/2, request.Size.Y/2)
		width := 180 - index%3*20
		height := 150 - index%3*20
		bounds := image.Rect(
			anchor.X-width/2,
			anchor.Y-height,
			anchor.X+(width+1)/2,
			anchor.Y,
		)
		fill := color.NRGBA{
			R: uint8(80 + index%4*30),
			G: 90,
			B: 110,
			A: 255,
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				img.SetNRGBA(x, y, fill)
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return provider.Result{}, err
	}
	return provider.Result{PNG: encoded.Bytes()}, nil
}

func WritePackWithReferences(t *testing.T) string {
	t.Helper()
	dir := WritePack(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "identity.png"), PNG(t, 16, 16), 0o644))
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := strings.Replace(
		string(data),
		`"description": "Elegant demonic duelist.",`,
		`"description": "Elegant demonic duelist.",
      "references": [{"id":"identity","role":"identity","path":"identity.png","description":"Identity evidence."}],`,
		1,
	)
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
	writeStyleEvidence(t, dir)
	for _, direction := range []string{"down", "up", "right"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "direction-"+direction+".png"),
			PNG(t, 16, 16),
			0o644,
		))
	}
	objects := `[
    {
      "id": "blood-duelist",
      "kind": "animated",
      "archetype": "agile-armored-humanoid",
      "description": "Elegant demonic duelist.",
      "magicSources": [],
      "identityLocks": ["Gold horned silhouette.", "Sword remains in the right hand."],
      "registration": "grounded",
      "size": {"width":16,"height":16},
      "directions": [
        {"id":"down","description":"Front/down view.","reference":{"path":"direction-down.png","description":"Facing and topology evidence only."}},
        {"id":"up","description":"Back/up view.","reference":{"path":"direction-up.png","description":"Facing and topology evidence only."}},
        {"id":"right","description":"Right side view.","reference":{"path":"direction-right.png","description":"Facing and topology evidence only."}}
      ],
      "animations": [{
        "id":"attack",
        "description":"Attack motion.",
        "frames":[
          {"id":"00","description":"Ready."},
          {"id":"contact","description":"Hit."}
        ]
      }],
      "deploy":{"pathTemplate":"units/{object}__{animation}__{direction}__{frame}.png"}
    },
    {
      "id":"grass",
      "kind":"static",
      "family":"ground",
      "registration":"centered",
      "description":"Smooth grass tile.",
      "magicSources":[],
      "size":{"width":16,"height":16},
      "deploy":{"pathTemplate":"terrain/{object}.png"}
    }
  ]`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "sprites.json"),
		[]byte(packJSON(objects)),
		0o644,
	))
	return dir
}

func WriteFullUnitPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeStyleEvidence(t, dir)
	objects := `[
    {
      "id":"relic-knight",
      "kind":"animated",
      "archetype":"heavy-armored-humanoid",
      "description":"Heavy silver-and-gold knight with a crested helmet, kite shield, and sword.",
      "magicSources":[],
      "identityLocks":[
        "Helmet has a swept gold crest and angular narrow visor.",
        "Shield remains a blue-and-gold kite shield with cross heraldry.",
        "Sword and shield remain on their established sides."
      ],
      "registration":"grounded",
      "size":{"width":384,"height":384},
      "directions":[
        {"id":"down","description":"Front-facing down view.","reference":{"path":"deploy/units/relic-knight__walk__down__00.png","description":"Current down-facing topology and registration evidence only."}},
        {"id":"up","description":"Back-facing up view.","reference":{"path":"deploy/units/relic-knight__walk__up__00.png","description":"Current up-facing topology and registration evidence only."}},
        {"id":"right","description":"Side view facing right.","reference":{"path":"deploy/units/relic-knight__walk__right__00.png","description":"Current right-facing topology and registration evidence only."}}
      ],
      "animations":[
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
      "deploy":{"pathTemplate":"units/{object}__{animation}__{direction}__{frame}.png"}
    },
    {
      "id":"grass",
      "kind":"static",
      "family":"ground",
      "registration":"centered",
      "description":"Smooth grass tile.",
      "magicSources":[],
      "size":{"width":16,"height":16},
      "deploy":{"pathTemplate":"terrain/{object}.png"}
    }
  ]`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "sprites.json"),
		[]byte(packJSON(objects)),
		0o644,
	))
	for _, direction := range []string{"down", "up", "right"} {
		for _, animation := range []string{"walk", "attack"} {
			for _, frame := range []string{"00", "01", "02", "03"} {
				path := filepath.Join(
					dir,
					"deploy",
					"units",
					fmt.Sprintf("relic-knight__%s__%s__%s.png", animation, direction, frame),
				)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, PNGWithMargin(t, 384, 384, 72), 0o644))
			}
		}
	}
	return dir
}

func WriteStaticSetPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeStyleEvidence(t, dir)
	objects := `[
    {
      "id":"fortification",
      "kind":"static-set",
      "family":"ground",
      "registration":"grounded",
      "description":"One coherent former black-stone fortification.",
      "magicSources":[],
      "parts":[
        {
          "id":"tall-horizontal",
          "role":"surviving horizontal section",
          "description":"Tall surviving masonry with intact joins.",
          "size":{"width":96,"height":80},
          "deploy":{"pathTemplate":"terrain/fortification/tall-horizontal.png"}
        },
        {
          "id":"collapse-left",
          "role":"left collapse transition",
          "description":"Stepped broken courses ending at a clear passage.",
          "size":{"width":80,"height":64},
          "deploy":{"pathTemplate":"terrain/fortification/collapse-left.png"}
        }
      ]
    }
  ]`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "sprites.json"),
		[]byte(packJSON(objects)),
		0o644,
	))
	return dir
}

func LoadTargets(t *testing.T, dir string) (*pack.Pack, []targets.Target) {
	t.Helper()
	p, err := pack.Load(dir)
	require.NoError(t, err)
	all, err := targets.Expand(p)
	require.NoError(t, err)
	for targetIndex := range all {
		for inputIndex := range all[targetIndex].Inputs {
			if !filepath.IsAbs(all[targetIndex].Inputs[inputIndex].Path) {
				all[targetIndex].Inputs[inputIndex].Path = filepath.Join(dir, all[targetIndex].Inputs[inputIndex].Path)
			}
			if all[targetIndex].Inputs[inputIndex].SourcePath != "" &&
				!filepath.IsAbs(all[targetIndex].Inputs[inputIndex].SourcePath) {
				all[targetIndex].Inputs[inputIndex].SourcePath = filepath.Join(
					dir,
					all[targetIndex].Inputs[inputIndex].SourcePath,
				)
			}
		}
		if all[targetIndex].DirectionRefPath != "" && !filepath.IsAbs(all[targetIndex].DirectionRefPath) {
			all[targetIndex].DirectionRefPath = filepath.Join(dir, all[targetIndex].DirectionRefPath)
		}
	}
	return p, all
}

func writeStyleEvidence(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "style-input.png"), PNG(t, 16, 16), 0o644))
	guidePath := filepath.Join(dir, "references", "style", "compact-dark-fantasy-style-v1.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(guidePath), 0o755))
	require.NoError(t, os.WriteFile(guidePath, PNG(t, 1536, 1024), 0o644))
}

func packJSON(objects string) string {
	return `{
  "version":6,
  "outputDir":"output",
  "deployDir":"deploy",
  "style":{
    "id":"compact-dark-fantasy-tactics",
    "description":"Original compact dark-fantasy tactics pixel art.",
    "principles":["Broad readable clusters."],
    "palette":{"maxColors":32,"colorSpace":"linear-srgb","alpha":"binary","dithering":"none"},
    "contrastHierarchy":["magic-effects","units","structures","terrain"],
    "units":{
      "common":["Compact broad silhouettes."],
      "archetypes":{
        "heavy-armored-humanoid":{"description":"Broad armored humanoid.","scaleClass":"standard-humanoid","rules":["Large helmet and shoulders."]},
        "agile-armored-humanoid":{"description":"Compact agile humanoid.","scaleClass":"standard-humanoid","rules":["Clear weapon silhouette."]}
      }
    },
    "terrain":{
      "common":["Broad connected material clusters."],
      "families":{
        "ground":{"description":"Quiet seamless battlefield ground.","rules":["Keep the center low contrast."]}
      }
    },
    "forbidden":["No copied proprietary motifs."],
    "reference":{
      "id":"compact-dark-fantasy-style-v1",
      "path":"references/style/compact-dark-fantasy-style-v1.png",
      "description":"Approved original Essence Wars style guide."
    }
  },
  "styleGuide":{
    "description":"One original visual style board.",
    "size":{"width":1536,"height":1024},
    "inputs":[{"id":"original-style-input","role":"style","path":"style-input.png","description":"Original repository art evidence."}],
    "deploy":{"path":"references/style/compact-dark-fantasy-style-v1.png"}
  },
  "objects":` + objects + `
}`
}
