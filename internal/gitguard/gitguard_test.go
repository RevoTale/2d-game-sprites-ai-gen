package gitguard_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/gitguard"
	"github.com/stretchr/testify/require"
)

func TestCheckReportsTrackedAndStagedGeneratedPNGs(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	output := filepath.Join(repo, "pack", "output")
	tracked := filepath.Join(output, "runs", "old", "tracked.png")
	staged := filepath.Join(output, "runs", "new", "staged.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(tracked), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(staged), 0o755))
	require.NoError(t, os.WriteFile(tracked, testPNG(t), 0o644))
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "track old artifact")
	require.NoError(t, os.WriteFile(staged, testPNG(t), 0o644))
	runGit(t, repo, "add", ".")

	offenders, err := gitguard.Check(output)

	require.NoError(t, err)
	require.Contains(t, offenders, "pack/output/runs/new/staged.png")
	require.Contains(t, offenders, "pack/output/runs/old/tracked.png")
}

func TestCheckAcceptsRelativeGeneratedRoot(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	output := filepath.Join(repo, "pack", "output")
	require.NoError(t, os.MkdirAll(output, 0o755))
	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repo))
	t.Cleanup(func() { require.NoError(t, os.Chdir(oldwd)) })

	offenders, err := gitguard.Check(filepath.Join("pack", "output"))

	require.NoError(t, err)
	require.Empty(t, offenders)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 100, G: 20, B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
