package output_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/output"
	"github.com/stretchr/testify/require"
)

func TestValidateAcceptsV12Layout(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"catalog/catalog.json",
		"catalog/index.html",
		"catalog/assets/terrain-details-stone-cluster-01.png",
		"runs/v12/manifest.json",
		"runs/v12/targets/grass/attempts/001/evidence.json",
		"runs/v12/targets/grass/attempts/001/candidates/01/metrics.json",
		"runs/v12/targets/style-guide/review/style-guide-96.png",
		"runs/v12/targets/grass/review/battlefield-preview-96.png",
		"runs/v12/targets/grass/review/tiled-repeat-3x3.png",
		"runs/v12/intermediates/knight/character-master/canonical-profile.json",
		"runs/v12/intermediates/knight/character-master/canonical-profile-overlay.png",
		"runs/v12/intermediates/knight/character-master/current-directional-references.png",
		"runs/v12/intermediates/knight/character-master/provider/direction-references/right.png",
		"runs/v12/intermediates/knight/character-master/attempts/001/evidence.json",
		"runs/v12/intermediates/knight/character-master/attempts/001/candidates/01/raw-candidate.png",
		"runs/v12/intermediates/knight/character-master/recovered/00.png",
		"runs/v12/intermediates/knight/character-master/review/ownership.png",
		"runs/v12/intermediates/knight/animations/walk/master-comparison-guide.png",
		"runs/v12/intermediates/knight/animations/walk/scale-calibration.json",
		"runs/v12/intermediates/knight/animations/walk/recovered/00.png",
		"runs/v12/intermediates/knight/animations/walk/review/recovered-poses.png",
		"runs/v12/units/knight/review/complete-unit.png",
		"runs/v12/units/knight/review/portrait-96.png",
		"runs/v12/units/knight/review/master-directions/right.png",
		"runs/v12/units/knight/review/gifs/walk-right.gif",
		"runs/v12/static-sets/fortification/prompt.md",
		"runs/v12/static-sets/fortification/recovered/00.png",
		"runs/v12/static-sets/fortification/review/ownership.png",
		"runs/v12/static-sets/water-cycle/review/loop.gif",
		"runs/v12/static-sets/water-cycle/review/repeats/phase-00-3x3.png",
		"runs/v12/static-sets/shore-kit/review/assembled-shores.png",
		"runs/v12/static-sets/fortification/review/runtime-overrides/fortification-part-wall.png",
		"runs/v12/static-sets/fortification/attempts/001/evidence.json",
		"runs/v12/static-sets/fortification/attempts/001/candidates/01/raw-candidate.png",
	}
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		content := []byte("fixture")
		if relative == "runs/v12/manifest.json" {
			content = []byte(`{"version":12}`)
		}
		require.NoError(t, os.WriteFile(path, content, 0o644))
	}

	require.NoError(t, output.Validate(root))
}

func TestValidateRejectsUnexpectedCatalogFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "catalog", "app.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("unexpected"), 0o644))

	err := output.Validate(root)

	require.ErrorContains(t, err, "catalog/app.js")
}

func TestValidateRejectsLegacyLayoutInsideV12Run(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "runs", "v12", "manifest.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifest), 0o755))
	require.NoError(t, os.WriteFile(manifest, []byte(`{"version":12}`), 0o644))
	path := filepath.Join(root, "runs", "v12", "intermediates", "knight", "direction-seeds", "normalized.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("fixture"), 0o644))

	err := output.Validate(root)

	require.ErrorContains(t, err, "direction-seeds/normalized.png")
}

func TestValidateRejectsRemovedLegacyTree(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy", "old-draft", "prompt.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("legacy"), 0o644))

	err := output.Validate(root)

	require.ErrorContains(t, err, "legacy/old-draft/prompt.md")
}

func TestValidateRejectsUnexpectedManagedRunFileWithExactPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "runs", "run", "targets", "knight", "notes.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("unexpected"), 0o644))

	err := output.Validate(root)

	require.Error(t, err)
	require.Contains(t, err.Error(), "runs/run/targets/knight/notes.txt")
}

func TestValidateRejectsNestedStaticSetRuntimeOverride(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(
		root,
		"runs",
		"run",
		"static-sets",
		"fortification",
		"review",
		"runtime-overrides",
		"nested",
		"wall.png",
	)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("unexpected"), 0o644))

	err := output.Validate(root)

	require.ErrorContains(t, err, "runtime-overrides/nested/wall.png")
}
