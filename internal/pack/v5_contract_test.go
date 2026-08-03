package pack_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestLoadV5DoesNotReadThemeAndAllowsMissingBootstrapGuide(t *testing.T) {
	dir := writeV5ContractPack(t)

	p, err := pack.Load(dir)

	require.NoError(t, err)
	require.Equal(t, 5, p.Version)
	require.Equal(t, "compact-dark-fantasy-tactics", p.Style.ID)
	require.NoFileExists(t, filepath.Join(dir, p.Style.Reference.Path))
}

func TestLoadRejectsPreV5PackWithDirectMigrationError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "sprites.json"),
		[]byte(`{"version":4,"objects":[]}`),
		0o644,
	))

	_, err := pack.Load(dir)

	require.ErrorContains(t, err, "sprites.json v4 is unsupported; migrate the pack to v5")
}

func TestDecodeV5RejectsUnknownStyleFields(t *testing.T) {
	data := strings.Replace(
		v5ContractJSON(),
		`"description": "Original compact dark-fantasy tactics pixel art."`,
		`"description": "Original compact dark-fantasy tactics pixel art.", "temperature": 0.2`,
		1,
	)

	_, err := pack.Decode(strings.NewReader(data))

	require.ErrorContains(t, err, `unknown field "temperature"`)
}

func TestValidateV5RequiresKnownObjectArchetypesAndFamilies(t *testing.T) {
	tests := []struct {
		name    string
		replace string
		with    string
		want    string
	}{
		{
			name:    "animated archetype",
			replace: `"archetype": "heavy-armored-humanoid"`,
			with:    `"archetype": "missing-archetype"`,
			want:    `unknown archetype "missing-archetype"`,
		},
		{
			name:    "static family",
			replace: `"family": "ground"`,
			with:    `"family": "missing-family"`,
			want:    `unknown terrain family "missing-family"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeV5ContractPack(t)
			path := filepath.Join(dir, "sprites.json")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(
				path,
				[]byte(strings.Replace(string(data), tt.replace, tt.with, 1)),
				0o644,
			))

			_, err = pack.Load(dir)

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateV5AllowsOnlyExactLegacy320DirectionReferencesFor384Output(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	for _, direction := range []string{"down", "up", "right"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "deploy", "units", "relic-knight__walk__"+direction+"__00.png"),
			testkit.PNG(t, 320, 320),
			0o644,
		))
	}

	_, err := pack.Load(dir)

	require.NoError(t, err)

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "deploy", "units", "relic-knight__walk__down__00.png"),
		testkit.PNG(t, 319, 320),
		0o644,
	))
	_, err = pack.Load(dir)
	require.ErrorContains(t, err, "expected 384x384 or the exact legacy 320x320")
}

func writeV5ContractPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "style-input.png"),
		testkit.PNG(t, 16, 16),
		0o644,
	))
	for _, direction := range []string{"down", "up", "right"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "direction-"+direction+".png"),
			testkit.PNG(t, 16, 16),
			0o644,
		))
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "sprites.json"),
		[]byte(v5ContractJSON()),
		0o644,
	))
	return dir
}

func v5ContractJSON() string {
	return `{
  "version": 5,
  "outputDir": "output",
  "deployDir": "deploy",
  "style": {
    "id": "compact-dark-fantasy-tactics",
    "description": "Original compact dark-fantasy tactics pixel art.",
    "principles": ["Broad readable clusters."],
    "palette": {
      "maxColors": 32,
      "colorSpace": "linear-srgb",
      "alpha": "binary",
      "dithering": "none"
    },
    "contrastHierarchy": ["magic-effects", "units", "structures", "terrain"],
    "units": {
      "common": ["Compact broad silhouettes."],
      "archetypes": {
        "heavy-armored-humanoid": {
          "description": "Broad armored humanoid.",
          "scaleClass": "standard-humanoid",
          "rules": ["Large helmet and shoulders."]
        }
      }
    },
    "terrain": {
      "common": ["Broad connected material clusters."],
      "families": {
        "ground": {
          "description": "Quiet seamless battlefield ground.",
          "rules": ["Keep the center low contrast."]
        }
      }
    },
    "forbidden": ["No copied proprietary motifs."],
    "reference": {
      "id": "compact-dark-fantasy-style-v1",
      "path": "references/style/compact-dark-fantasy-style-v1.png",
      "description": "Approved original Essence Wars style guide."
    }
  },
  "styleGuide": {
    "description": "One original visual style board.",
    "size": {"width": 1536, "height": 1024},
    "inputs": [{
      "id": "original-style-input",
      "role": "style",
      "path": "style-input.png",
      "description": "Original repository art evidence."
    }],
    "deploy": {
      "path": "references/style/compact-dark-fantasy-style-v1.png"
    }
  },
  "objects": [{
    "id": "relic-knight",
    "kind": "animated",
    "archetype": "heavy-armored-humanoid",
    "description": "Compact silver-and-gold heroic knight.",
    "identityLocks": ["The swept crest, kite shield, and sword remain readable."],
    "registration": "grounded",
    "size": {"width": 16, "height": 16},
    "directions": [
      {"id":"down","description":"Front/down view.","reference":{"path":"direction-down.png","description":"Facing and topology evidence only."}},
      {"id":"up","description":"Back/up view.","reference":{"path":"direction-up.png","description":"Facing and topology evidence only."}},
      {"id":"right","description":"Right side view.","reference":{"path":"direction-right.png","description":"Facing and topology evidence only."}}
    ],
    "animations": [{
      "id": "walk",
      "description": "Compact grounded walk.",
      "frames": [
        {"id":"00","description":"Neutral step."},
        {"id":"01","description":"Forward step."},
        {"id":"02","description":"Passing step."},
        {"id":"03","description":"Recovery step."}
      ]
    }],
    "deploy": {"pathTemplate":"frames/units/{object}/{animation}/{direction}/{frame}.png"}
  }, {
    "id": "terrain-ground",
    "kind": "static",
    "family": "ground",
    "renderMode": "opaque-tile",
    "registration": "canvas",
    "description": "Quiet dark moss battlefield ground.",
    "size": {"width":16,"height":16},
    "deploy": {"pathTemplate":"terrain/ground.png"}
  }]
}`
}

func TestValidateV5RequiresKnownUnitScaleClass(t *testing.T) {
	tests := []struct {
		name        string
		scaleClass  string
		wantMessage string
	}{
		{name: "missing", scaleClass: "", wantMessage: "scaleClass is required"},
		{name: "unknown", scaleClass: "tall-knight", wantMessage: `unknown scaleClass "tall-knight"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := strings.Replace(
				v5ContractJSON(),
				`"scaleClass": "standard-humanoid"`,
				`"scaleClass": "`+tt.scaleClass+`"`,
				1,
			)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "style-input.png"), testkit.PNG(t, 16, 16), 0o644))
			for _, direction := range []string{"down", "up", "right"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "direction-"+direction+".png"), testkit.PNG(t, 16, 16), 0o644))
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(data), 0o644))

			_, err := pack.Load(dir)

			require.ErrorContains(t, err, tt.wantMessage)
		})
	}
}
