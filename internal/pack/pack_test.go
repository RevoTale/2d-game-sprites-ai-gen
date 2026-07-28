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

func TestDecodeRejectsUnknownFields(t *testing.T) {
	_, err := pack.Decode(strings.NewReader(`{"version":3,"objects":[],"type":"animated"}`))

	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestValidateRejectsV2WithMigrationMessage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("theme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{"version":2,"objects":[]}`), 0o644))

	_, _, err := pack.Load(dir)

	require.ErrorContains(t, err, "sprites.json v2 is unsupported; migrate the pack to v3")
}

func TestValidateRejectsReferenceWithoutIDOrRole(t *testing.T) {
	dir := testkit.WritePack(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "style.png"), testkit.PNG(t, 16, 16), 0o644))
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"objects": [`, `"references": [{"path":"style.png","description":"Style."}], "objects": [`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, "pack reference id")
}

func TestValidateRejectsDuplicateReferenceIDsAcrossScopes(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"id":"identity"`, `"id":"style"`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, `duplicate reference id "style"`)
}

func TestValidateRejectsReferenceRoleThatDoesNotMatchOwner(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"role":"style"`, `"role":"identity"`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, `pack reference "style" role must be "style"`)
}

func TestValidateRequiresIdentityLocksForAnimatedObjects(t *testing.T) {
	dir := testkit.WritePack(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"identityLocks": ["Gold horned silhouette.", "Sword remains in the right hand."],`, ``, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, `animated object "blood-duelist" identityLocks must contain at least one visual lock`)
}

func TestValidateRequiresOneSizedReferencePerAnimatedDirection(t *testing.T) {
	tests := []struct {
		name    string
		replace string
		with    string
		want    string
	}{
		{
			name:    "missing",
			replace: `, "reference": {"path":"direction-right.png","description":"Current right-facing identity reference."}`,
			want:    `direction "right" reference is required`,
		},
		{
			name:    "unreadable",
			replace: `"path":"direction-right.png"`,
			with:    `"path":"missing.png"`,
			want:    `reference "missing.png"`,
		},
		{
			name:    "wrong size",
			replace: `"path":"direction-right.png"`,
			with:    `"path":"wrong-size.png"`,
			want:    `is 8x8, expected 16x16`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testkit.WritePack(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "wrong-size.png"), testkit.PNG(t, 8, 8), 0o644))
			path := filepath.Join(dir, "sprites.json")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			data = []byte(strings.Replace(string(data), tt.replace, tt.with, 1))
			require.NoError(t, os.WriteFile(path, data, 0o644))

			_, _, err = pack.Load(dir)

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateAcceptsDirectionReferencesWithinSiblingDeployDir(t *testing.T) {
	dir := testkit.WritePack(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"deployDir": "deploy"`, `"deployDir": "../source"`, 1))
	for _, direction := range []string{"down", "up", "right"} {
		name := "direction-" + direction + ".png"
		sourcePath := filepath.Join(filepath.Dir(dir), "source", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
		require.NoError(t, os.WriteFile(sourcePath, testkit.PNG(t, 16, 16), 0o644))
		data = []byte(strings.ReplaceAll(string(data), `"`+name+`"`, `"../source/`+name+`"`))
	}
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.NoError(t, err)
}

func TestValidateRejectsDirectionReferenceOutsidePackAndDeployDir(t *testing.T) {
	dir := testkit.WritePack(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	outside := filepath.Join(filepath.Dir(dir), "outside.png")
	require.NoError(t, os.WriteFile(outside, testkit.PNG(t, 16, 16), 0o644))
	data = []byte(strings.Replace(string(data), `"path":"direction-right.png"`, `"path":"../outside.png"`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, "path traversal outside the pack or deploy directory is not allowed")
}

func TestValidateRejectsAdditionalAnimatedVariantAxis(t *testing.T) {
	dir := testkit.WritePack(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"id": "direction"`, `"id": "pose"`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, `must define exactly one variant axis named "direction"`)
}

func TestValidateRejectsDuplicateDirectionReferencePaths(t *testing.T) {
	dir := testkit.WritePack(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"path":"direction-right.png"`, `"path":"direction-up.png"`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, `directions "right" and "up" use duplicate reference`)
}

func TestValidateReservesDerivedDirectionReferenceIDsPackWide(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"id":"style"`, `"id":"direction-reference-blood-duelist-right"`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.ErrorContains(t, err, `duplicate reference id "direction-reference-blood-duelist-right"`)
}

func TestValidateAcceptsOpaqueTileRenderModeForStaticObject(t *testing.T) {
	dir := testkit.WritePack(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"id": "grass",`, `"id": "grass",
      "renderMode": "opaque-tile",`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	p, _, err := pack.Load(dir)

	require.NoError(t, err)
	require.Equal(t, pack.RenderModeOpaqueTile, pack.EffectiveRenderMode(p.Objects[1]))
}

func TestValidateRejectsUnsupportedOrAnimatedOpaqueTileRenderMode(t *testing.T) {
	tests := []struct {
		name       string
		objectID   string
		renderMode string
		want       string
	}{
		{name: "unsupported", objectID: "grass", renderMode: "seamless", want: `renderMode "seamless" is unsupported`},
		{name: "animated", objectID: "blood-duelist", renderMode: pack.RenderModeOpaqueTile, want: `animated object "blood-duelist" renderMode "opaque-tile" is unsupported`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testkit.WritePack(t)
			path := filepath.Join(dir, "sprites.json")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			needle := `"id": "` + tt.objectID + `",`
			replacement := needle + "\n      \"renderMode\": \"" + tt.renderMode + "\","
			data = []byte(strings.Replace(string(data), needle, replacement, 1))
			require.NoError(t, os.WriteFile(path, data, 0o644))

			_, _, err = pack.Load(dir)

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateRejectsDuplicateExplicitFrameIDs(t *testing.T) {
	dir := testkit.WritePack(t)
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `{ "id": "contact", "description": "Hit." }`, `{ "id": "00", "description": "Hit." }`, 1))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, _, err = pack.Load(dir)

	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate frame id")
}

func TestLoadAcceptsFixturePack(t *testing.T) {
	dir := testkit.WritePack(t)

	p, theme, err := pack.Load(dir)

	require.NoError(t, err)
	require.Contains(t, theme, "smooth pixel art")
	require.Len(t, p.Objects, 2)
}

func TestValidateRejectsUnknownDeployTemplatePlaceholders(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("theme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 3,
  "objects": [
    {
      "id": "duelist",
      "description": "A duelist.",
      "size": { "width": 16, "height": 16 },
      "deploy": { "pathTemplate": "units/{object}__{unknown}.png" }
    }
  ]
}`), 0o644))

	_, _, err := pack.Load(dir)

	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown deploy placeholder")
}

func TestLoadAcceptsStaticObjectWithoutExplicitDeployTemplate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("theme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 3,
  "objects": [
    {
      "id": "grass",
      "description": "A grass tile.",
      "size": { "width": 16, "height": 16 }
    }
  ]
}`), 0o644))

	p, _, err := pack.Load(dir)

	require.NoError(t, err)
	require.Equal(t, "sprites/{target}.png", pack.DeployTemplate(p.Objects[0]))
}

func TestLoadAcceptsStaticVariantObjectWithoutExplicitDeployTemplate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("theme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 3,
  "objects": [
    {
      "id": "grass",
      "description": "A grass tile.",
      "size": { "width": 16, "height": 16 },
      "variants": [
        {
          "id": "season",
          "description": "Seasonal tile variant.",
          "values": [
            { "id": "summer", "description": "Summer grass." },
            { "id": "winter", "description": "Winter grass." }
          ]
        }
      ]
    }
  ]
}`), 0o644))

	p, _, err := pack.Load(dir)

	require.NoError(t, err)
	require.Equal(t, "sprites/{target}.png", pack.DeployTemplate(p.Objects[0]))
}
