package envfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/envfile"
	"github.com/stretchr/testify/require"
)

func TestReadParsesSimpleDotenvEntriesWithoutShellExpansion(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte(`
# comment
APP_MODE=production
OPENAI_API_KEY="test-key"
RAW='$OPENAI_API_KEY'
`), 0o600))

	values, err := envfile.Read(path)

	require.NoError(t, err)
	require.Equal(t, "production", values["APP_MODE"])
	require.Equal(t, "test-key", values["OPENAI_API_KEY"])
	require.Equal(t, "$OPENAI_API_KEY", values["RAW"])
}

func TestMergeLetsProcessEnvironmentOverrideDotenvFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte("OPENAI_API_KEY=file-key\n"), 0o600))

	values, err := envfile.Merge([]string{"OPENAI_API_KEY=process-key"}, path)

	require.NoError(t, err)
	require.Equal(t, "process-key", values["OPENAI_API_KEY"])
}
