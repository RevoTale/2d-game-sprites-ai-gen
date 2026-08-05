package catalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/catalog"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestBuildWritesDeterministicStaticReviewCatalog(t *testing.T) {
	packDir := testkit.WritePack(t)
	p, err := pack.Load(packDir)
	require.NoError(t, err)
	all, err := targets.Expand(p)
	require.NoError(t, err)
	deployDir := filepath.Join(packDir, p.DeployDir)
	grass := targets.FilterTargets(all, targets.Filter{Object: "grass"})[0]
	grassPath, err := targets.DeployPath(deployDir, grass)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(grassPath), 0o755))
	require.NoError(t, os.WriteFile(grassPath, testkit.PNG(t, 32, 32), 0o644))
	usageRoot := filepath.Join(packDir, "maps")
	require.NoError(t, os.MkdirAll(usageRoot, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(usageRoot, "example.json"),
		[]byte(`{"placements":[{"assetId":"grass"}]}`),
		0o644,
	))
	out := filepath.Join(packDir, "output", "catalog")
	opts := catalog.Options{PackDir: packDir, DeployDir: deployDir, UsageRoot: usageRoot, OutputDir: out}

	first, err := catalog.Build(p, all, opts)
	require.NoError(t, err)
	firstJSON, err := os.ReadFile(first.MetadataPath)
	require.NoError(t, err)
	firstHTML, err := os.ReadFile(first.IndexPath)
	require.NoError(t, err)
	second, err := catalog.Build(p, all, opts)
	require.NoError(t, err)
	secondJSON, err := os.ReadFile(second.MetadataPath)
	require.NoError(t, err)
	secondHTML, err := os.ReadFile(second.IndexPath)
	require.NoError(t, err)

	require.Equal(t, firstJSON, secondJSON)
	require.Equal(t, firstHTML, secondHTML)
	require.Equal(t, 1, second.Entries)
	var metadata catalog.Metadata
	require.NoError(t, json.Unmarshal(secondJSON, &metadata))
	require.Equal(t, "grass", metadata.Entries[0].ID)
	require.Equal(t, catalog.StatusPresent, metadata.Entries[0].Status)
	require.Equal(t, pack.Size{Width: 32, Height: 32}, metadata.Entries[0].IntrinsicSize)
	require.Equal(t, pack.Size{Width: 16, Height: 16}, metadata.Entries[0].LogicalSize)
	require.Equal(t, "2x", metadata.Entries[0].Density)
	require.Equal(t, []string{"example.json"}, metadata.Entries[0].MapUsage)
	require.FileExists(t, filepath.Join(out, filepath.FromSlash(metadata.Entries[0].PreviewPath)))
	require.Contains(t, string(secondHTML), "Native source")
	require.Contains(t, string(secondHTML), "Logical game size")
	require.Contains(t, string(secondHTML), "Enlarged inspection")
}

func TestBuildLabelsMissingPlannedAssets(t *testing.T) {
	packDir := testkit.WritePack(t)
	p, err := pack.Load(packDir)
	require.NoError(t, err)
	all, err := targets.Expand(p)
	require.NoError(t, err)
	out := filepath.Join(packDir, "output", "catalog")

	result, err := catalog.Build(p, all, catalog.Options{
		PackDir:   packDir,
		DeployDir: filepath.Join(packDir, p.DeployDir),
		OutputDir: out,
	})

	require.NoError(t, err)
	data, err := os.ReadFile(result.MetadataPath)
	require.NoError(t, err)
	var metadata catalog.Metadata
	require.NoError(t, json.Unmarshal(data, &metadata))
	require.Equal(t, catalog.StatusPlaceholder, metadata.Entries[0].Status)
	require.Empty(t, metadata.Entries[0].PreviewPath)
}
