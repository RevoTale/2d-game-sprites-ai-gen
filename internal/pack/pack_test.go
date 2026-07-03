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
	_, err := pack.Decode(strings.NewReader(`{"version":1,"objects":[],"type":"animated"}`))

	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestValidateRejectsDuplicateExplicitFrameIDs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "THEME.md"), []byte("theme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(`{
  "version": 1,
  "objects": [
    {
      "id": "duelist",
      "description": "A duelist.",
      "size": { "width": 16, "height": 16 },
      "animations": [
        {
          "id": "attack",
          "description": "Attack.",
          "frames": [
            { "id": "hit", "description": "One." },
            { "id": "hit", "description": "Two." }
          ]
        }
      ]
    }
  ]
}`), 0o644))

	_, _, err := pack.Load(dir)

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
