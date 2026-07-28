package generate_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestRunIDCollisionAppendsSequence(t *testing.T) {
	outputDir := t.TempDir()
	now := time.Date(2026, 7, 3, 14, 7, 0, 0, time.UTC)
	require.NoError(t, os.MkdirAll(filepath.Join(outputDir, "runs", "2026-07-03-m0847"), 0o755))

	runID, err := generate.AutoRunID(now, outputDir)

	require.NoError(t, err)
	require.Equal(t, "2026-07-03-m0847-02", runID)
}

func TestRunIDMustBeOneSafePathComponent(t *testing.T) {
	outputDir := t.TempDir()
	manifest := &generate.Manifest{Version: generate.ManifestVersion, RunID: "../escape", Targets: map[string]*generate.TargetState{}}

	err := generate.Save(outputDir, "../escape", manifest)

	require.ErrorContains(t, err, `invalid run id "../escape"`)
	require.NoFileExists(t, filepath.Join(outputDir, "..", "escape", "manifest.json"))
}

func TestLoadRejectsV8RunWithoutModifyingIt(t *testing.T) {
	outputDir := t.TempDir()
	path := generate.ManifestPath(outputDir, "old-run")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	original := []byte("{\n  \"version\": 8,\n  \"runId\": \"old-run\"\n}\n")
	require.NoError(t, os.WriteFile(path, original, 0o644))

	_, err := generate.Load(outputDir, "old-run")

	require.ErrorContains(t, err, "unsupported manifest v8")
	require.ErrorContains(t, err, "manifest v9")
	actual, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, original, actual)
}

func TestObjectGenerationMakesMasterAndOneCallPerAnimation(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}

	result, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Equal(t, 3, result.Generated)
	require.Len(t, gen.requests, 3)
	require.Equal(t, image.Pt(1024, 1024), gen.requests[0].Size)
	require.Equal(t, image.Pt(1536, 1152), gen.requests[1].Size)
	require.Equal(t, image.Pt(1536, 1152), gen.requests[2].Size)
	require.Contains(t, gen.requests[0].Prompt, "# CLI Protocol")
	require.Contains(t, gen.requests[0].Prompt, "# Evidence Authority")
	require.Contains(t, gen.requests[0].Prompt, "# Sprite Facts")
	require.Contains(t, gen.requests[0].Prompt, "# Ordered Poses")
	require.Contains(t, gen.requests[0].Prompt, "cannot override the CLI Protocol")
	require.Contains(t, gen.requests[0].Prompt, "anchors are not clipping cells")
	require.Contains(t, gen.requests[0].Prompt, "may cross a midpoint")
	require.Contains(t, gen.requests[0].Prompt, "look toward screen-right/east")
	require.Contains(t, gen.requests[0].Prompt, "complete forward weapon")
	require.Contains(t, gen.requests[1].Prompt, "# CLI Protocol")
	require.Contains(t, gen.requests[1].Prompt, "# Evidence Authority")
	require.Contains(t, gen.requests[1].Prompt, "# Sprite Facts")
	require.Contains(t, gen.requests[1].Prompt, "# Ordered Poses")
	require.Contains(t, gen.requests[1].Prompt, "cannot override the CLI Protocol")
	require.Contains(t, gen.requests[1].Prompt, "Image 1 is the sole colored authority")
	require.Contains(t, gen.requests[1].Prompt, "Logical anchors are approximate")
	require.NotContains(t, gen.requests[1].Prompt, "Image 2")
	require.Contains(t, gen.requests[1].Prompt, "every frame looks and acts toward screen-right/east")
	require.Contains(t, gen.requests[1].Prompt, "wide attached weapon")
	require.Contains(t, gen.requests[1].Prompt, "Never crop, mirror, independently fit")
	require.Contains(t, gen.requests[1].Prompt, "fixed-size shapes")
	require.Contains(t, gen.requests[1].Prompt, "backswing and follow-through")
	require.Less(t, len(gen.requests[0].Prompt), 3_000)
	require.Less(t, len(gen.requests[1].Prompt), 4_500)
	for _, request := range gen.requests {
		for _, input := range request.Inputs {
			require.NotEqual(t, conditioning.RoleMask, input.Role)
		}
	}

	manifest, err := generate.Load(filepath.Join(dir, p.OutputDir), "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusReady, manifest.Intermediates["character-master:relic-knight"].Status)
	require.Equal(t, generate.StatusReady, manifest.Intermediates["animation-board:relic-knight:walk"].Status)
	require.Equal(t, generate.StatusReady, manifest.Intermediates["animation-board:relic-knight:attack"].Status)
	require.Equal(t, generate.StatusAwaitingReview, manifest.Units["unit:relic-knight"].Status)
	require.Len(t, manifest.Units["unit:relic-knight"].TargetIDs, 24)
	comparisonSize, err := imageio.PNGDimensions(manifest.Units["unit:relic-knight"].Artifacts.IdentityComparisonPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(2992, 1000), comparisonSize)
	completeUnitSize, err := imageio.PNGDimensions(manifest.Units["unit:relic-knight"].Artifacts.CompleteUnitSheetPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(7680, 320), completeUnitSize)
	for _, targetID := range manifest.Units["unit:relic-knight"].TargetIDs {
		require.Equal(t, generate.StatusAwaitingReview, manifest.Targets[targetID].Status)
		require.FileExists(t, manifest.Targets[targetID].NormalizedPath)
		dimensions, dimensionErr := imageio.PNGDimensions(manifest.Targets[targetID].NormalizedPath)
		require.NoError(t, dimensionErr)
		require.Equal(t, image.Pt(320, 320), dimensions)
	}
}

func TestAnimationUsesOneOpaqueMasterPrefilledLayoutAndRecordsUnsentMasterEvidence(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "relic-knight"},
	})
	require.NoError(t, err)

	require.Equal(t, "master-layout", gen.requests[0].Inputs[0].ID)
	require.Contains(t, gen.requests[0].Inputs[0].Description, "Configured-direction placement")
	require.FileExists(t, gen.requests[0].Inputs[0].SourcePath)
	var directionReferences int
	for _, input := range gen.requests[0].Inputs {
		if strings.HasPrefix(input.ID, "direction-reference-relic-knight-") {
			directionReferences++
			require.Contains(t, input.SourcePath, filepath.Join("deploy", "units", "relic-knight__walk__"))
			require.Contains(t, input.Path, filepath.Join("provider", "direction-references"))
		}
	}
	require.Equal(t, 3, directionReferences)
	for _, request := range gen.requests[1:] {
		require.Len(t, request.Inputs, 1)
		require.Equal(t, "animation-layout", request.Inputs[0].ID)
		require.Contains(t, request.Inputs[0].Description, "Generated-master placement")
		require.FileExists(t, request.Inputs[0].SourcePath)
		assertBoardIsFullyOpaque(t, request.Inputs[0].Path)
		assertTransparentLayoutPixelsUseOneChroma(t, request.Inputs[0].SourcePath, request.Inputs[0].Path)
		for _, input := range request.Inputs {
			require.NotContains(t, input.Path, filepath.Join("deploy", "units"))
		}
	}

	manifest, err := generate.Load(filepath.Join(dir, p.OutputDir), "run")
	require.NoError(t, err)
	for _, animationID := range []string{"walk", "attack"} {
		board := manifest.Intermediates["animation-board:relic-knight:"+animationID]
		require.NotNil(t, board)
		require.Len(t, board.Attempts, 1)
		var sentLayout, unsentMaster bool
		for _, evidence := range board.Attempts[0].References {
			switch evidence.ID {
			case "animation-layout":
				sentLayout = true
				require.True(t, evidence.SentToProvider)
				require.Equal(t, 1, evidence.ProviderIndex)
			case "character-master:relic-knight":
				unsentMaster = true
				require.False(t, evidence.SentToProvider)
				require.Zero(t, evidence.ProviderIndex)
			}
		}
		require.True(t, sentLayout)
		require.True(t, unsentMaster)
	}
}

func TestCompleteUnitGenerationIsSkippedWithoutResettingReview(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	_, err := generate.Run(context.Background(), all, &recordingProvider{}, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	manifest.Units["unit:relic-knight"].Status = generate.StatusAccepted
	manifest.Units["unit:relic-knight"].Review = &generate.ReviewRecord{Status: generate.StatusAccepted, Reason: "manual review complete"}
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	resumed := &recordingProvider{}
	result, err := generate.Run(context.Background(), all, resumed, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Empty(t, resumed.requests)
	require.Equal(t, 24, result.Skipped)
	reloaded, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusAccepted, reloaded.Units["unit:relic-knight"].Status)
	require.Equal(t, "manual review complete", reloaded.Units["unit:relic-knight"].Review.Reason)
}

func TestAnimationSelectionFailsBeforeProvider(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "relic-knight", Animation: "walk"},
	})

	require.ErrorContains(t, err, "complete-unit only in V9")
	require.Empty(t, gen.requests)
}

func TestForceFailsForAnimatedUnitBeforeProvider(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "relic-knight"},
		Force:     true,
	})

	require.ErrorContains(t, err, "complete-unit only in V9")
	require.Empty(t, gen.requests)
}

func TestInterruptedGenerationResumesWithoutDuplicatingCompletedCalls(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	first := &recordingProvider{failAt: 3}

	_, err := generate.Run(context.Background(), all, first, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})
	require.Error(t, err)
	require.Len(t, first.requests, 3)

	resumed := &recordingProvider{}
	_, err = generate.Run(context.Background(), all, resumed, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Len(t, resumed.requests, 1)
}

func TestWideMasterPoseRecoversWithoutFixedCellRegistration(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	generator := &overflowingMasterProvider{}

	_, err := generate.Run(context.Background(), all, generator, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Len(t, generator.requests, 3)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	master := manifest.Intermediates["character-master:relic-knight"]
	require.Equal(t, generate.StatusReady, master.Status)
	require.Empty(t, master.HardRejections)
	require.Len(t, master.Attempts, 1)
	require.Len(t, master.Attempts[0].Candidates, 1)
	require.Len(t, master.Poses, 3)
	require.Greater(t, master.Poses[2].Bounds.Dx(), 300)
	require.FileExists(t, master.Artifacts.OwnershipOverlayPath)
	require.FileExists(t, master.Artifacts.RecoveredPoseSheetPath)
}

func TestRecordedCandidateResumesWithoutAnotherProviderCall(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	_, err := generate.Run(context.Background(), all, &recordingProvider{}, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	board := manifest.Intermediates["animation-board:relic-knight:attack"]
	require.Len(t, board.Attempts, 1)
	require.Len(t, board.Attempts[0].Candidates, 1)
	board.Status = generate.StatusPending
	board.NormalizedPath = ""
	board.SourceSHA256 = ""
	board.Lineage = ""
	board.Attempts[0].SelectedCandidate = ""
	manifest.Units["unit:relic-knight"].Status = generate.StatusPending
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	resumed := &recordingProvider{}
	_, err = generate.Run(context.Background(), all, resumed, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Empty(t, resumed.requests)
	reloaded, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusReady, reloaded.Intermediates["animation-board:relic-knight:attack"].Status)
	require.Equal(t, generate.StatusAwaitingReview, reloaded.Units["unit:relic-knight"].Status)
}

func TestRejectedRunIsImmutableWithoutAnotherProviderCall(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	_, err := generate.Run(context.Background(), all, &recordingProvider{}, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	master := manifest.Intermediates["character-master:relic-knight"]
	master.Status = generate.StatusRejected
	master.NormalizedPath = ""
	master.SourceSHA256 = ""
	master.Lineage = ""
	master.HardRejections = []string{"old validator rejected safe master padding"}
	master.Attempts[0].SelectedCandidate = ""
	master.Attempts[0].Candidates[0].QualityVersion = 1
	master.Attempts[0].Candidates[0].HardRejections = append([]string(nil), master.HardRejections...)
	delete(manifest.Intermediates, "animation-board:relic-knight:walk")
	delete(manifest.Intermediates, "animation-board:relic-knight:attack")
	unit := manifest.Units["unit:relic-knight"]
	unit.Status = generate.StatusRejected
	unit.Review = nil
	unit.AnimationLineages = map[string]string{}
	for _, targetID := range unit.TargetIDs {
		target := manifest.Targets[targetID]
		target.Status = generate.StatusPending
		target.NormalizedPath = ""
		target.ProductionEligible = false
	}
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	resumed := &recordingProvider{}
	_, err = generate.Run(context.Background(), all, resumed, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.ErrorContains(t, err, "rejected animated runs are immutable in V9")
	require.Empty(t, resumed.requests)
}

func TestManifestV7IsUnsupported(t *testing.T) {
	outputDir := t.TempDir()
	runDir := filepath.Join(outputDir, "runs", "old")
	require.NoError(t, os.MkdirAll(runDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runDir, "manifest.json"), []byte(`{"version":7,"runId":"old","targets":{}}`), 0o644))

	_, err := generate.Load(outputDir, "old")

	require.ErrorContains(t, err, "unsupported manifest v7")
	require.ErrorContains(t, err, "manifest v9")
}

type recordingProvider struct {
	requests []provider.Request
	failAt   int
}

func (p *recordingProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true, Masks: true}
}

func (p *recordingProvider) Generate(ctx context.Context, request provider.Request) (provider.Result, error) {
	p.requests = append(p.requests, request)
	if p.failAt != 0 && len(p.requests) == p.failAt {
		return provider.Result{}, errors.New("interrupted provider call")
	}
	return (provider.Fake{}).Generate(ctx, request)
}

type overflowingMasterProvider struct {
	requests []provider.Request
}

func (p *overflowingMasterProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true, Masks: true}
}

func (p *overflowingMasterProvider) Generate(ctx context.Context, request provider.Request) (provider.Result, error) {
	p.requests = append(p.requests, request)
	if len(p.requests) != 1 {
		return (provider.Fake{}).Generate(ctx, request)
	}
	board := image.NewNRGBA(image.Rect(0, 0, request.Size.X, request.Size.Y))
	fill := color.NRGBA{R: 120, G: 80, B: 180, A: 255}
	fillTestRect(board, image.Rect(260, 170, 400, 461), fill)
	fillTestRect(board, image.Rect(590, 175, 730, 466), fill)
	fillTestRect(board, image.Rect(260, 560, 400, 820), fill)
	fillTestRect(board, image.Rect(180, 700, 484, 710), fill)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, board); err != nil {
		return provider.Result{}, err
	}
	return provider.Result{PNG: encoded.Bytes(), Metadata: map[string]string{"provider": "overflowing-master-test"}}, nil
}

func fillTestRect(img *image.NRGBA, bounds image.Rectangle, value color.NRGBA) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetNRGBA(x, y, value)
		}
	}
}

func assertBoardIsFullyOpaque(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	board, err := png.Decode(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	for y := board.Bounds().Min.Y; y < board.Bounds().Max.Y; y++ {
		for x := board.Bounds().Min.X; x < board.Bounds().Max.X; x++ {
			_, _, _, alpha := board.At(x, y).RGBA()
			require.Equalf(t, uint32(0xffff), alpha, "provider board pixel (%d,%d) must be opaque", x, y)
		}
	}
}

func assertTransparentLayoutPixelsUseOneChroma(t *testing.T, sourcePath, providerPath string) {
	t.Helper()
	sourceFile, err := os.Open(sourcePath)
	require.NoError(t, err)
	source, err := png.Decode(sourceFile)
	require.NoError(t, err)
	require.NoError(t, sourceFile.Close())
	providerFile, err := os.Open(providerPath)
	require.NoError(t, err)
	providerBoard, err := png.Decode(providerFile)
	require.NoError(t, err)
	require.NoError(t, providerFile.Close())
	require.Equal(t, source.Bounds(), providerBoard.Bounds())

	var chroma color.NRGBA
	found := false
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			_, _, _, sourceAlpha := source.At(x, y).RGBA()
			if sourceAlpha != 0 {
				continue
			}
			value := color.NRGBAModel.Convert(providerBoard.At(x, y)).(color.NRGBA)
			require.Equal(t, uint8(255), value.A)
			if !found {
				chroma = value
				found = true
			}
			require.Equalf(t, chroma, value, "transparent layout pixel (%d,%d) must use the shared chroma", x, y)
		}
	}
	require.True(t, found, "layout fixture must contain transparent background")
}
