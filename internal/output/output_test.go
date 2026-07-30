package output_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/output"
	"github.com/stretchr/testify/require"
)

func TestValidateAcceptsV10LayoutAndPreservesUnsupportedRuns(t *testing.T) {
	root := t.TempDir()
	oldManifest := filepath.Join(root, "runs", "v9", "manifest.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(oldManifest), 0o755))
	require.NoError(t, os.WriteFile(oldManifest, []byte(`{"version":9}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "runs", "v9", "intermediates", "knight", "animations"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "runs", "v9", "intermediates", "knight", "animations", "arbitrary-old-file.png"), []byte("fixture"), 0o644))
	paths := []string{
		"runs/v10/manifest.json",
		"runs/v10/targets/grass/attempts/001/evidence.json",
		"runs/v10/targets/grass/attempts/001/candidates/01/metrics.json",
		"runs/v10/intermediates/knight/character-master/canonical-profile.json",
		"runs/v10/intermediates/knight/character-master/canonical-profile-overlay.png",
		"runs/v10/intermediates/knight/character-master/current-directional-references.png",
		"runs/v10/intermediates/knight/character-master/provider/direction-references/right.png",
		"runs/v10/intermediates/knight/character-master/attempts/001/evidence.json",
		"runs/v10/intermediates/knight/character-master/attempts/001/candidates/01/raw-candidate.png",
		"runs/v10/intermediates/knight/character-master/recovered/00.png",
		"runs/v10/intermediates/knight/character-master/review/ownership.png",
		"runs/v10/intermediates/knight/animations/walk/master-comparison-guide.png",
		"runs/v10/intermediates/knight/animations/walk/scale-calibration.json",
		"runs/v10/intermediates/knight/animations/walk/recovered/00.png",
		"runs/v10/intermediates/knight/animations/walk/review/recovered-poses.png",
		"runs/v10/units/knight/review/complete-unit.png",
		"runs/v10/units/knight/review/master-directions/right.png",
		"runs/v10/units/knight/review/gifs/walk-right.gif",
		"legacy/arbitrary/old-draft/prompt.md",
	}
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		content := []byte("fixture")
		if relative == "runs/v10/manifest.json" {
			content = []byte(`{"version":10}`)
		}
		require.NoError(t, os.WriteFile(path, content, 0o644))
	}

	require.NoError(t, output.Validate(root))
}

func TestValidateRejectsLegacyLayoutInsideV10Run(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "runs", "v10", "manifest.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifest), 0o755))
	require.NoError(t, os.WriteFile(manifest, []byte(`{"version":10}`), 0o644))
	path := filepath.Join(root, "runs", "v10", "intermediates", "knight", "direction-seeds", "normalized.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("fixture"), 0o644))

	err := output.Validate(root)

	require.ErrorContains(t, err, "direction-seeds/normalized.png")
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
