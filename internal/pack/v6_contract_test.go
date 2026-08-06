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

// This contract test exists because an omitted source used to let prompts
// invent unexplained magical decoration on otherwise mundane objects.
func TestDecodeV6RequiresExplicitMagicSources(t *testing.T) {
	tests := []struct {
		name    string
		object  string
		wantErr string
	}{
		{
			name: "omitted",
			object: `{
  "id":"wall",
  "kind":"static",
  "family":"ruin",
  "description":"Collapsed mundane block wall.",
  "registration":"centered",
  "size":{"width":64,"height":32},
  "deploy":{"pathTemplate":"terrain/wall.png"}
}`,
			wantErr: `object "wall" magicSources is required`,
		},
		{
			name: "empty list is explicit mundane evidence",
			object: `{
  "id":"wall",
  "kind":"static",
  "family":"ruin",
  "description":"Collapsed mundane block wall.",
  "magicSources":[],
  "registration":"centered",
  "size":{"width":64,"height":32},
  "deploy":{"pathTemplate":"terrain/wall.png"}
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := decodeAndValidateV6(t, v6MinimalPack(tt.object))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, p.Objects[0].MagicSources)
			require.Empty(t, *p.Objects[0].MagicSources)
		})
	}
}

// This test keeps supernatural appearance causal and bounded instead of
// accepting a vague color hint that can spread across the complete sprite.
func TestDecodeV6ValidatesMagicSourceFacts(t *testing.T) {
	object := `{
  "id":"rune-wall",
  "kind":"static",
  "family":"ruin",
  "description":"Collapsed block wall with one carved ward stone.",
  "magicSources":[{
    "id":"ward-rune",
    "description":"A deliberately carved dormant ward rune.",
    "location":"On one intact central face stone.",
    "palette":"Deep violet with a small lavender core.",
    "expression":"Light remains inside the carved channels.",
    "limits":["No detached glow or flowing ornament."]
  }],
  "registration":"centered",
  "size":{"width":64,"height":32},
  "deploy":{"pathTemplate":"terrain/rune-wall.png"}
}`

	p, err := decodeAndValidateV6(t, v6MinimalPack(object))

	require.NoError(t, err)
	require.Len(t, *p.Objects[0].MagicSources, 1)
	require.Equal(t, "ward-rune", (*p.Objects[0].MagicSources)[0].ID)
}

// A direct migration error prevents an old pack from being interpreted under
// the new causal-art contract before provider billing begins.
func TestValidateV6RejectsV5WithMigrationMessage(t *testing.T) {
	p := &pack.Pack{Version: 5}

	err := pack.Validate(t.TempDir(), p)

	require.ErrorContains(t, err, "sprites.json v5 is unsupported; migrate the pack to v6")
}

// Coupled water phases and shoreline joins must carry independent stable part
// facts while retaining one review and deployment owner.
func TestDecodeV6AcceptsAtomicStaticSetParts(t *testing.T) {
	object := `{
  "id":"drowned-water-cycle-01",
  "kind":"static-set",
  "family":"ruin",
  "description":"One restrained seamless dark-water material cycle.",
  "magicSources":[],
  "renderMode":"opaque-tile",
  "registration":"canvas",
  "parts":[
    {
      "id":"phase-00",
      "role":"First water animation phase.",
      "description":"The shared water material at its first reflection state.",
      "size":{"width":64,"height":64},
      "deploy":{"pathTemplate":"terrain/water/phase-00.png"}
    },
    {
      "id":"phase-01",
      "role":"Second water animation phase.",
      "description":"The same material with connected reflections advanced slightly.",
      "size":{"width":64,"height":64},
      "deploy":{"pathTemplate":"terrain/water/phase-01.png"}
    }
  ]
}`

	p, err := decodeAndValidateV6(t, v6MinimalPack(object))

	require.NoError(t, err)
	require.Equal(t, pack.KindStaticSet, p.Objects[0].Kind)
	require.Len(t, p.Objects[0].Parts, 2)
}

func TestDecodeV6AcceptsCanvasRegisteredTransparentOverlaySet(t *testing.T) {
	object := `{
  "id":"weather-overlay-kit",
  "kind":"static-set",
  "family":"ruin",
  "description":"Transparent weather overlays.",
  "magicSources":[],
  "renderMode":"transparent-overlay",
  "registration":"canvas",
  "parts":[
    {
      "id":"rain-a",
      "role":"First registered weather overlay.",
      "description":"Sparse rain streak overlay.",
      "size":{"width":32,"height":32},
      "deploy":{"pathTemplate":"weather/rain-a.png"}
    },
    {
      "id":"rain-b",
      "role":"Second registered weather overlay.",
      "description":"Sparse rain streak overlay.",
      "size":{"width":32,"height":32},
      "deploy":{"pathTemplate":"weather/rain-b.png"}
    }
  ]
}`

	p, err := decodeAndValidateV6(t, v6MinimalPack(object))

	require.NoError(t, err)
	require.Equal(t, pack.RenderModeTransparentOverlay, p.Objects[0].RenderMode)
}

func TestDecodeV6AcceptsCanvasRegisteredMaterialSwatchSet(t *testing.T) {
	object := `{
  "id":"shore-materials",
  "kind":"static-set",
  "family":"ruin",
  "description":"Full-bleed shoreline material swatches.",
  "magicSources":[],
  "renderMode":"material-swatch",
  "registration":"canvas",
  "parts":[
    {
      "id":"wet-earth",
      "role":"Wet edge material.",
      "description":"Opaque wet earth swatch.",
      "size":{"width":128,"height":128},
      "deploy":{"pathTemplate":"shore/wet-earth.png"}
    },
    {
      "id":"moss",
      "role":"Ground edge material.",
      "description":"Opaque moss swatch.",
      "size":{"width":128,"height":128},
      "deploy":{"pathTemplate":"shore/moss.png"}
    }
  ]
}`

	p, err := decodeAndValidateV6(t, v6MinimalPack(object))

	require.NoError(t, err)
	require.Equal(t, pack.RenderModeMaterialSwatch, p.Objects[0].RenderMode)
}

func TestValidateV6RejectsMaterialSwatchOutsideStaticSet(t *testing.T) {
	object := `{
  "id":"shore-material",
  "kind":"static",
  "family":"ruin",
  "description":"One material swatch.",
  "magicSources":[],
  "renderMode":"material-swatch",
  "registration":"canvas",
  "size":{"width":128,"height":128},
  "deploy":{"pathTemplate":"shore/wet-earth.png"}
}`

	_, err := decodeAndValidateV6(t, v6MinimalPack(object))

	require.ErrorContains(t, err, `renderMode "material-swatch" requires kind "static-set"`)
}

func TestValidateV6RequiresCanvasRegistrationForTransparentOverlay(t *testing.T) {
	object := `{
  "id":"weather-overlay-kit",
  "kind":"static-set",
  "family":"ruin",
  "description":"Transparent weather overlays.",
  "magicSources":[],
  "renderMode":"transparent-overlay",
  "registration":"centered",
  "parts":[
    {
      "id":"rain-a",
      "role":"First registered weather overlay.",
      "description":"Sparse rain streak overlay.",
      "size":{"width":32,"height":32},
      "deploy":{"pathTemplate":"weather/rain-a.png"}
    },
    {
      "id":"rain-b",
      "role":"Second registered weather overlay.",
      "description":"Sparse rain streak overlay.",
      "size":{"width":32,"height":32},
      "deploy":{"pathTemplate":"weather/rain-b.png"}
    }
  ]
}`

	_, err := decodeAndValidateV6(t, v6MinimalPack(object))

	require.ErrorContains(t, err, `transparent-overlay object "weather-overlay-kit" registration must be "canvas"`)
}

func TestDecodeV6RejectsMalformedStaticSets(t *testing.T) {
	valid := `{
  "id":"wall-kit",
  "kind":"static-set",
  "family":"ruin",
  "description":"Coupled mundane wall parts.",
  "magicSources":[],
  "registration":"centered",
  "parts":[
    {"id":"wall","role":"Straight wall.","description":"Worked stone wall.","size":{"width":32,"height":32},"deploy":{"pathTemplate":"terrain/wall/wall.png"}},
    {"id":"corner","role":"Outer corner.","description":"Worked stone corner.","size":{"width":32,"height":32},"deploy":{"pathTemplate":"terrain/wall/corner.png"}}
  ]
}`
	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{name: "one part", old: `,
    {"id":"corner","role":"Outer corner.","description":"Worked stone corner.","size":{"width":32,"height":32},"deploy":{"pathTemplate":"terrain/wall/corner.png"}}`, new: "", wantErr: "must contain at least two parts"},
		{name: "duplicate part", old: `"id":"corner"`, new: `"id":"wall"`, wantErr: `duplicate part id "wall"`},
		{name: "missing role", old: `"role":"Straight wall."`, new: `"role":""`, wantErr: "role is required"},
		{name: "object size", old: `"registration":"centered",`, new: `"registration":"centered","size":{"width":32,"height":32},`, wantErr: "must not define object size"},
		{name: "object deploy", old: `"registration":"centered",`, new: `"registration":"centered","deploy":{"pathTemplate":"terrain/shore.png"},`, wantErr: "must not define object deploy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeAndValidateV6(t, v6MinimalPack(strings.Replace(valid, tt.old, tt.new, 1)))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func v6MinimalPack(object string) string {
	return `{
  "version":6,
  "style":{
    "id":"test-style",
    "description":"Original test style.",
    "principles":["Readable connected clusters."],
    "palette":{"maxColors":32,"colorSpace":"linear-srgb","alpha":"binary","dithering":"none"},
    "contrastHierarchy":["magic","units","terrain"],
    "units":{"common":["Readable."],"archetypes":{"test":{"description":"Test.","scaleClass":"reference-stable","rules":["Readable."]}}},
    "terrain":{"common":["Readable."],"families":{"ruin":{"description":"Masonry.","rules":["Broad blocks."]}}},
    "forbidden":["No copied motifs."],
    "reference":{"id":"test-style-reference","path":"references/style/test.png","description":"Approved guide."}
  },
  "styleGuide":{
    "description":"Original board.",
    "size":{"width":1536,"height":1024},
    "inputs":[{"id":"style-input","role":"style","path":"style.png","description":"Original evidence."}],
    "deploy":{"path":"references/style/test.png"}
  },
  "objects":[` + object + `]
}`
}

func decodeAndValidateV6(t *testing.T, data string) (*pack.Pack, error) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "style.png"),
		testkit.PNG(t, 16, 16),
		0o644,
	))
	p, err := pack.Decode(strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err = pack.Validate(dir, p); err != nil {
		return nil, err
	}
	return p, nil
}
