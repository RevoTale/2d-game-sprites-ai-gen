package output_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/output"
	"github.com/stretchr/testify/require"
)

func TestValidateAcceptsManifestV1V2V3AndOpaqueLegacyFiles(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"runs/v1/manifest.json",
		"runs/v1/intermediates/knight/identity/attempts/001/raw-candidate.png",
		"runs/v2/targets/knight__walk__right__00/attempts/001/candidates/01/metrics.json",
		"runs/v2/intermediates/knight/directions/direction-right/palette.json",
		"runs/v3/intermediates/knight/identity/board.png",
		"runs/v3/intermediates/knight/variants/direction-right/anchor/attempts/001/mask.png",
		"runs/v3/intermediates/knight/variants/direction-right/anchor/attempts/001/candidates/01/metrics.json",
		"runs/v3/intermediates/knight/animations/walk/direction-right/pose-board.png",
		"runs/v3/intermediates/knight/animations/walk/direction-right/motion-study/normalized.png",
		"runs/v3/intermediates/knight/animations/walk/direction-right/motion-study/attempts/001/candidates/01/raw-candidate.png",
		"runs/v3/contact-sheets/intermediates/motion-study-knight.png",
		"legacy/arbitrary/old-draft/prompt.md",
	}
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("fixture"), 0o644))
	}

	require.NoError(t, output.Validate(root))
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
