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
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
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

func TestLoadRejectsLegacyVersion10RunWithoutModifyingIt(t *testing.T) {
	outputDir := t.TempDir()
	path := generate.ManifestPath(outputDir, "old-run")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	original := []byte("{\n  \"version\": 10,\n  \"runId\": \"old-run\"\n}\n")
	require.NoError(t, os.WriteFile(path, original, 0o644))

	_, err := generate.Load(outputDir, "old-run")

	require.ErrorContains(t, err, "unsupported manifest v10")
	require.ErrorContains(t, err, "manifest v12")
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
	require.Equal(t, image.Pt(1664, 1280), gen.requests[1].Size)
	require.Equal(t, image.Pt(1664, 1280), gen.requests[2].Size)
	require.Contains(t, gen.requests[0].Prompt, "# CLI Protocol")
	require.Contains(t, gen.requests[0].Prompt, "# Evidence Authority")
	require.Contains(t, gen.requests[0].Prompt, "Locks override conflicting direction-reference elements")
	require.Contains(t, gen.requests[0].Prompt, "# Sprite Facts")
	require.Contains(t, gen.requests[0].Prompt, "# Ordered Poses")
	require.Contains(t, gen.requests[0].Prompt, "cannot override the CLI Protocol")
	require.Contains(t, gen.requests[0].Prompt, "anchors are not clipping cells")
	require.Contains(t, gen.requests[0].Prompt, "may cross a midpoint")
	require.Contains(t, gen.requests[0].Prompt, "look toward screen-right/east")
	require.Contains(t, gen.requests[0].Prompt, "complete forward weapon")
	require.Contains(t, gen.requests[0].Prompt, "own identity, materials, colors, features")
	require.Contains(t, gen.requests[0].Prompt, "style guide owns shape language")
	require.Contains(t, gen.requests[0].Prompt, "not colors or proportions")
	require.Contains(t, gen.requests[0].Prompt, "# Supernatural sources")
	require.Contains(t, gen.requests[0].Prompt, "None declared. Do not invent glow")
	require.Contains(t, gen.requests[1].Prompt, "# CLI Protocol")
	require.Contains(t, gen.requests[1].Prompt, "# Evidence Authority")
	require.Contains(t, gen.requests[1].Prompt, "# Sprite Facts")
	require.Contains(t, gen.requests[1].Prompt, "# Ordered Poses")
	require.Contains(t, gen.requests[1].Prompt, "# Supernatural sources")
	require.Contains(t, gen.requests[1].Prompt, "None declared. Do not invent glow")
	require.Contains(t, gen.requests[1].Prompt, "cannot override the CLI Protocol")
	require.Contains(t, gen.requests[1].Prompt, "Image 1 is the sole colored authority")
	require.Contains(t, gen.requests[1].Prompt, "Logical anchors are approximate")
	require.NotContains(t, gen.requests[1].Prompt, "Image 2")
	require.Contains(t, gen.requests[1].Prompt, "every frame looks and acts toward screen-right/east")
	require.Contains(t, gen.requests[1].Prompt, "wide attached weapon")
	require.Contains(t, gen.requests[1].Prompt, "Never crop, mirror, independently fit")
	require.Contains(t, gen.requests[1].Prompt, "fixed-size shapes")
	require.Contains(
		t,
		gen.requests[1].Prompt,
		"arrange backswing, contact, and follow-through extents around that fixed root",
	)
	require.Contains(
		t,
		gen.requests[1].Prompt,
		"fit unchanged inside the same final native 384x384 rectangle",
	)
	require.Contains(t, gen.requests[1].Prompt, "one frame-00 registration origin")
	require.Contains(t, gen.requests[1].Prompt, "Keep the grounded body pivot fixed at that origin")
	require.NotContains(t, gen.requests[1].Prompt, "Preserve intentional body and foot displacement")
	require.Contains(t, gen.requests[1].Prompt, "Stage wide motion diagonally or in depth")
	require.Contains(t, gen.requests[1].Prompt, "Never solve fit by shortening equipment")
	require.Contains(t, gen.requests[2].Prompt, "one compact arc inside the shared final-frame rectangle")
	require.Contains(t, gen.requests[2].Prompt, "Down/front projection")
	require.Contains(t, gen.requests[2].Prompt, "toward the screen-bottom foreground")
	require.Contains(t, gen.requests[2].Prompt, "Up/back projection")
	require.Contains(t, gen.requests[2].Prompt, "toward screen-top depth")
	require.Contains(t, gen.requests[2].Prompt, "Right/side projection")
	require.Contains(t, gen.requests[2].Prompt, "toward screen-right")
	require.Contains(t, gen.requests[2].Prompt, "never a straight screen-horizontal maximum-width pose")
	require.Contains(t, gen.requests[2].Prompt, "A slash remains an angled arc")
	require.Contains(t, gen.requests[2].Prompt, "a thrust uses depth foreshortening")
	require.Contains(t, gen.requests[2].Prompt, "Screen-horizontal width is not attack strength")
	require.Contains(t, gen.requests[1].Prompt, "including behind a backswing")
	require.Contains(t, gen.requests[1].Prompt, "Preserve exact material colors, saturation, and contrast")
	require.Contains(
		t,
		gen.requests[1].Prompt,
		"No floating sparks, motes, embers, droplets, aura fragments, or isolated glow pixels",
	)
	require.Contains(
		t,
		gen.requests[1].Prompt,
		"Every visible magic highlight must stay physically connected to the unit body or attached equipment",
	)
	require.Contains(
		t,
		gen.requests[1].Prompt,
		"Never paint bloom, halo, aura, soft light, semi-transparent glow, or colored lighting into the chroma background",
	)
	require.Contains(
		t,
		gen.requests[1].Prompt,
		"Show magic brightness only with opaque hard-edged connected pixel clusters",
	)
	require.Less(t, len(gen.requests[0].Prompt), 3_000)
	require.Less(t, len(gen.requests[1].Prompt), 4_500)
	directionInputs := 0
	for _, input := range gen.requests[0].Inputs {
		if !strings.HasPrefix(input.ID, "direction-reference-") {
			continue
		}
		directionInputs++
		require.Equal(t, conditioning.RolePose, input.Role)
		require.Equal(t, "configured-direction-geometry", input.Authority)
	}
	require.Equal(t, 3, directionInputs)

	manifest, err := generate.Load(filepath.Join(dir, p.OutputDir), "run")
	require.NoError(t, err)
	recordedPrompt, err := os.ReadFile(manifest.Intermediates["character-master:relic-knight"].Artifacts.PromptPath)
	require.NoError(t, err)
	require.Equal(t, gen.requests[0].Prompt, string(recordedPrompt))
	require.Contains(t, gen.requests[0].Prompt, "# Image References")
	require.Equal(t, generate.StatusReady, manifest.Intermediates["character-master:relic-knight"].Status)
	require.Equal(t, generate.StatusReady, manifest.Intermediates["animation-board:relic-knight:walk"].Status)
	require.Equal(t, generate.StatusReady, manifest.Intermediates["animation-board:relic-knight:attack"].Status)
	require.Equal(t, generate.StatusAwaitingReview, manifest.Units["unit:relic-knight"].Status)
	require.Len(t, manifest.Units["unit:relic-knight"].TargetIDs, 24)
	require.Equal(t, generate.AnimatedAssemblyVersion, manifest.Units["unit:relic-knight"].AssemblyVersion)
	require.NotNil(t, manifest.Units["unit:relic-knight"].Profile)
	require.Len(t, manifest.Units["unit:relic-knight"].Transform.DirectionAnchors, 3)
	require.FileExists(t, manifest.Units["unit:relic-knight"].Artifacts.CanonicalProfilePath)
	require.FileExists(t, manifest.Units["unit:relic-knight"].Artifacts.CanonicalProfileOverlayPath)
	require.FileExists(t, manifest.Units["unit:relic-knight"].Artifacts.NativePreviewPath)
	require.FileExists(t, manifest.Units["unit:relic-knight"].Artifacts.PortraitPreviewPath)
	portraitSize, err := imageio.PNGDimensions(manifest.Units["unit:relic-knight"].Artifacts.PortraitPreviewPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(96, 96), portraitSize)
	comparisonSize, err := imageio.PNGDimensions(manifest.Units["unit:relic-knight"].Artifacts.IdentityComparisonPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(3568, 1192), comparisonSize)
	completeUnitSize, err := imageio.PNGDimensions(manifest.Units["unit:relic-knight"].Artifacts.CompleteUnitSheetPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(9216, 384), completeUnitSize)
	for _, targetID := range manifest.Units["unit:relic-knight"].TargetIDs {
		require.Equal(t, generate.StatusAwaitingReview, manifest.Targets[targetID].Status)
		require.Equal(
			t,
			"reference-derived-board-calibrated-subject",
			manifest.Targets[targetID].Normalization.ScaleAlgorithm,
		)
		require.FileExists(t, manifest.Targets[targetID].NormalizedPath)
		dimensions, dimensionErr := imageio.PNGDimensions(manifest.Targets[targetID].NormalizedPath)
		require.NoError(t, dimensionErr)
		require.Equal(t, image.Pt(384, 384), dimensions)
	}
	for _, animationID := range []string{"walk", "attack"} {
		board := manifest.Intermediates["animation-board:relic-knight:"+animationID]
		require.NotNil(t, board.ScaleCalibration)
		require.Equal(t, imageio.SemanticScaleCalibrationVersion, board.ScaleCalibration.Version)
		require.Len(t, board.ScaleCalibration.DirectionScales, 3)
		require.Len(t, board.ScaleCalibration.DirectionPivotOffsets, 3)
		require.Len(t, board.ScaleCalibration.PoseMeasurements, 12)
		for _, measurement := range board.ScaleCalibration.PoseMeasurements {
			require.Positive(t, measurement.ForegroundPixels)
			require.Positive(t, measurement.SourceWidth)
			require.Positive(t, measurement.SourceHeight)
			require.Positive(t, measurement.CanonicalWidth)
			require.Positive(t, measurement.CanonicalHeight)
		}
		require.FileExists(t, board.Artifacts.ScaleCalibrationPath)
	}
}

func TestAnimatedUnitPaletteComesOnlyFromCanonicalMaster(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recoloringProvider{}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	manifest, err := generate.Load(filepath.Join(dir, p.OutputDir), "run")
	require.NoError(t, err)
	target := manifest.Targets["relic-knight__walk__direction-down__00"]
	require.Contains(t, target.Palette, imageio.PaletteColor{R: 200, G: 40, B: 40})
	require.NotContains(t, target.Palette, imageio.PaletteColor{R: 20, G: 220, B: 20})
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

func TestOutdatedAwaitingReviewUnitReassemblesWithoutProviderCalls(t *testing.T) {
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
	manifest.Units["unit:relic-knight"].AssemblyVersion = 0
	manifest.Units["unit:relic-knight"].Profile.Version = 1
	manifest.Units["unit:relic-knight"].Profile.ReferenceCanvases = nil
	for _, boardID := range manifest.Units["unit:relic-knight"].AnimationBoardIDs {
		manifest.Intermediates[boardID].ScaleCalibration = nil
		manifest.Intermediates[boardID].Artifacts.ScaleCalibrationPath = ""
	}
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	resumed := &recordingProvider{}
	result, err := generate.Run(context.Background(), all, resumed, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Empty(t, resumed.requests)
	require.Zero(t, result.Generated)
	reloaded, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.AnimatedAssemblyVersion, reloaded.Units["unit:relic-knight"].AssemblyVersion)
	require.Equal(
		t,
		imageio.CanonicalSubjectProfileVersion,
		reloaded.Units["unit:relic-knight"].Profile.Version,
	)
	require.Equal(
		t,
		[]image.Point{{X: 384, Y: 384}, {X: 384, Y: 384}, {X: 384, Y: 384}},
		reloaded.Units["unit:relic-knight"].Profile.ReferenceCanvases,
	)
	for _, boardID := range reloaded.Units["unit:relic-knight"].AnimationBoardIDs {
		require.NotNil(t, reloaded.Intermediates[boardID].ScaleCalibration)
		require.FileExists(t, reloaded.Intermediates[boardID].Artifacts.ScaleCalibrationPath)
	}
}

func TestOutdatedAwaitingReviewUnitRejectsIncompleteReferenceLineage(t *testing.T) {
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
	unit := manifest.Units["unit:relic-knight"]
	unit.AssemblyVersion = 0
	unit.Profile.Version = 1
	unit.Profile.ReferenceCanvases = nil
	unit.Profile.ReferenceHashes = unit.Profile.ReferenceHashes[:2]
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	resumed := &recordingProvider{}
	result, err := generate.Run(context.Background(), all, resumed, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Empty(t, resumed.requests)
	require.Zero(t, result.Generated)
	reloaded, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusRejected, reloaded.Units["unit:relic-knight"].Status)
	require.Contains(
		t,
		reloaded.Units["unit:relic-knight"].HardRejections,
		"invalid_canonical_subject_profile: canonical profile reference lineage is incomplete",
	)
}

func TestRejectedReassemblyClearsStaleReviewableTargets(t *testing.T) {
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
	unit := manifest.Units["unit:relic-knight"]
	unit.AssemblyVersion = 0
	attack := manifest.Intermediates["animation-board:relic-knight:attack"]
	attack.Poses[0].Bounds = image.Rect(-400, -200, 400, 0)
	attack.Poses[0].Pivot = image.Point{}
	require.NoError(t, generate.Save(outputDir, "run", manifest))

	resumed := &recordingProvider{}
	_, err = generate.Run(context.Background(), all, resumed, generate.Options{
		OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Empty(t, resumed.requests)
	reloaded, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Equal(t, generate.StatusRejected, reloaded.Units["unit:relic-knight"].Status)
	require.NotEmpty(t, reloaded.Units["unit:relic-knight"].HardRejections)
	require.Contains(
		t,
		reloaded.Units["unit:relic-knight"].HardRejections[0],
		"unsafe_canonical_pose_extent",
	)
	require.Empty(t, reloaded.Units["unit:relic-knight"].Artifacts.CompleteUnitSheetPath)
	for _, targetID := range reloaded.Units["unit:relic-knight"].TargetIDs {
		target := reloaded.Targets[targetID]
		require.Equal(t, generate.StatusRejected, target.Status)
		require.Empty(t, target.NormalizedPath)
		require.False(t, target.ProductionEligible)
		require.NotEmpty(t, target.HardRejections)
	}
}

func TestIndependentAnimationBoardScaleIsCalibratedToMaster(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	gen := &smallerAnimationProvider{}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: outputDir,
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "relic-knight"},
	})

	require.NoError(t, err)
	require.Len(t, gen.requests, 3)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	board := manifest.Intermediates["animation-board:relic-knight:walk"]
	require.NotNil(t, board.ScaleCalibration)
	for _, ratio := range board.ScaleCalibration.SourceRatios {
		require.Greater(t, ratio, 1.5)
	}
	masterBounds, err := imageio.ForegroundBounds(filepath.Join(
		outputDir,
		"runs",
		"run",
		"units",
		"relic-knight",
		"review",
		"master-directions",
		"down.png",
	))
	require.NoError(t, err)
	frameBounds, err := imageio.ForegroundBounds(
		manifest.Targets["relic-knight__walk__direction-down__00"].NormalizedPath,
	)
	require.NoError(t, err)
	require.InDelta(t, masterBounds.Dx(), frameBounds.Dx(), 1)
	require.InDelta(t, masterBounds.Dy(), frameBounds.Dy(), 1)
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
	interrupted, loadErr := generate.Load(outputDir, "run")
	require.NoError(t, loadErr)
	require.Equal(
		t,
		1,
		generate.ProviderCallsRemaining(
			interrupted,
			targets.FilterTargets(all, targets.Filter{Object: "relic-knight"}),
		),
	)

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

	require.ErrorContains(t, err, "rejected animated runs are immutable in V13")
	require.Empty(t, resumed.requests)
}

func TestManifestV7IsUnsupported(t *testing.T) {
	outputDir := t.TempDir()
	runDir := filepath.Join(outputDir, "runs", "old")
	require.NoError(t, os.MkdirAll(runDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runDir, "manifest.json"), []byte(`{"version":7,"runId":"old","targets":{}}`), 0o644))

	_, err := generate.Load(outputDir, "old")

	require.ErrorContains(t, err, "unsupported manifest v7")
	require.ErrorContains(t, err, "manifest v12")
}

func TestBatchGenerationRecordsOneFailureAndContinuesUnrelatedObjects(t *testing.T) {
	dir := testkit.WriteFullUnitPack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, pack.OutputDir(p))
	deployDir := filepath.Join(dir, p.DeployDir)
	gen := &recordingProvider{failAt: 1}

	result, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir:       outputDir,
		DeployDir:       deployDir,
		RunID:           "batch",
		ContinueOnError: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Failed)
	require.Len(t, gen.requests, 4)
	manifest, err := generate.Load(outputDir, "batch")
	require.NoError(t, err)
	require.Len(t, manifest.Failures, 1)
	require.Equal(t, "grass", manifest.Failures[0].ObjectID)
	require.True(t, manifest.Failures[0].Ambiguous)
	require.Equal(t, generate.StatusAwaitingReview, manifest.Units["unit:relic-knight"].Status)
}

func TestStaticGenerationSendsOnlyConfiguredJSONEvidence(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	for index := range all {
		if all[index].ObjectID == "grass" {
			all[index].RenderMode = pack.RenderModeOpaqueTile
		}
	}
	productionPath := filepath.Join(dir, p.DeployDir, "terrain", "grass.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(productionPath), 0o755))
	require.NoError(t, os.WriteFile(productionPath, testkit.PNG(t, 16, 16), 0o644))
	gen := &recordingProvider{}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "grass"},
	})

	require.NoError(t, err)
	require.Len(t, gen.requests, 1)
	require.Len(t, gen.requests[0].Inputs, 1)
	require.Equal(t, conditioning.RoleStyle, gen.requests[0].Inputs[0].Role)
	require.NotEqual(t, productionPath, gen.requests[0].Inputs[0].Path)
	manifest, err := generate.Load(filepath.Join(dir, p.OutputDir), "run")
	require.NoError(t, err)
	state := manifest.Targets["grass"]
	require.NotNil(t, state)
	require.NotEmpty(t, state.Artifacts.TiledPreviewPath)
	require.FileExists(t, state.Artifacts.TiledPreviewPath)
}

func TestStaticGenerationRecordsExactTwoTimesSourceDensity(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{
		OutputDir: outputDir,
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "grass"},
	})

	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	state := manifest.Targets["grass"]
	require.Equal(t, pack.Size{Width: 16, Height: 16}, state.LogicalSize)
	require.Equal(t, pack.Size{Width: 32, Height: 32}, state.IntrinsicSize)
	require.Equal(t, 2, state.SourceDensity)
	dimensions, err := imageio.PNGDimensions(state.NormalizedPath)
	require.NoError(t, err)
	require.Equal(t, image.Pt(32, 32), dimensions)
}

func TestStaticSetGenerationUsesOneSharedProviderAttempt(t *testing.T) {
	dir := testkit.WriteStaticSetPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &testkit.StaticSetProvider{}
	outputDir := filepath.Join(dir, p.OutputDir)

	result, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: outputDir,
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "fortification"},
	})

	require.NoError(t, err)
	require.Len(t, gen.Requests, 1)
	require.Len(t, gen.Requests[0].Inputs, 2)
	require.Equal(t, "static-set-layout", gen.Requests[0].Inputs[0].ID)
	require.Equal(t, conditioning.RolePose, gen.Requests[0].Inputs[0].Role)
	require.Equal(t, "cli-protocol", gen.Requests[0].Inputs[0].Authority)
	require.Contains(t, gen.Requests[0].Prompt, "Never merge, touch, or connect")
	require.FileExists(t, gen.Requests[0].Inputs[0].Path)
	require.FileExists(t, gen.Requests[0].MaskPath)
	assertBoardIsFullyOpaque(t, gen.Requests[0].Inputs[0].Path)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	set := manifest.Intermediates["static-set:fortification"]
	require.NotNil(t, set)
	require.FileExists(t, set.EditSourcePath)
	require.Equal(t, gen.Requests[0].MaskPath, set.EditMaskPath)
	maskEvidence := set.Attempts[0].References[len(set.Attempts[0].References)-1]
	require.Equal(t, "edit-mask", maskEvidence.ID)
	require.Equal(t, "mask", maskEvidence.Role)
	require.Equal(t, "cli-protocol", maskEvidence.Authority)
	require.Equal(t, gen.Requests[0].MaskPath, maskEvidence.SentPath)
	require.True(t, maskEvidence.SentToProvider)
	assertTransparentLayoutPixelsUseOneChroma(
		t,
		set.EditSourcePath,
		gen.Requests[0].Inputs[0].Path,
	)
	require.Equal(t, 2, result.AwaitingReview, "set=%+v targets=%+v", set, manifest.Targets)
	require.Equal(t, generate.StatusAwaitingReview, set.Status)
	require.Len(t, set.Attempts, 1)
	require.Len(t, set.Attempts[0].Candidates, 1)
	require.NotEmpty(t, set.Lineage)
	require.NotNil(t, set.StaticSetScale)
	require.FileExists(t, set.Artifacts.MasterSheetPath)
	require.FileExists(t, set.Artifacts.ContactSheetPath)
	require.DirExists(t, set.Artifacts.RuntimeOverrideRoot)
	var sharedPalette []imageio.PaletteColor
	for _, target := range all {
		state := manifest.Targets[target.ID]
		require.Equal(t, generate.StatusAwaitingReview, state.Status)
		require.Equal(t, []string{set.ID}, state.Dependencies)
		require.Equal(t, set.Lineage, state.SourceCandidate)
		require.FileExists(t, state.NormalizedPath)
		require.Equal(t, pack.Size{
			Width: target.Size.Width * 2, Height: target.Size.Height * 2,
		}, state.IntrinsicSize)
		require.Equal(t, 2, state.SourceDensity)
		require.NotNil(t, state.Normalization)
		require.Equal(t, "canvas-class-static-set-alpha-fit-v1", state.Normalization.ScaleAlgorithm)
		require.Equal(t, set.StaticSetScale.ScaleForPart(target.ID), state.Normalization.Scale)
		require.FileExists(t, state.Artifacts.BattlefieldPreviewPath)
		overridePath := filepath.Join(set.Artifacts.RuntimeOverrideRoot, target.ID+".png")
		require.FileExists(t, overridePath)
		normalized, err := os.ReadFile(state.NormalizedPath)
		require.NoError(t, err)
		override, err := os.ReadFile(overridePath)
		require.NoError(t, err)
		require.Equal(t, normalized, override)
		if sharedPalette == nil {
			sharedPalette = state.Palette
		} else {
			require.Equal(t, sharedPalette, state.Palette)
		}
	}
}

func TestOpaqueTileStaticSetGeneratesSeamAndLoopReviewArtifacts(t *testing.T) {
	dir := testkit.WriteWaterCyclePack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &testkit.StaticSetProvider{}
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: outputDir,
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "water-cycle"},
	})

	require.NoError(t, err)
	require.Len(t, gen.Requests, 1)
	require.Equal(t, image.Pt(1024, 1024), gen.Requests[0].Size)
	require.Contains(t, gen.Requests[0].Prompt, "fill its complete declared tile rectangle")
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	set := manifest.Intermediates["static-set:water-cycle"]
	require.Equal(t, generate.StatusAwaitingReview, set.Status)
	require.Equal(t, 320, set.SemanticLayout.AnchorSpacing)
	require.FileExists(t, filepath.Join(outputDir, "runs", "run", "static-sets", "water-cycle", "review", "loop.gif"))
	for _, partID := range []string{"phase-00", "phase-01", "phase-02"} {
		require.FileExists(t, filepath.Join(
			outputDir,
			"runs",
			"run",
			"static-sets",
			"water-cycle",
			"review",
			"repeats",
			partID+"-3x3.png",
		))
	}
	for _, target := range all {
		state := manifest.Targets[target.ID]
		require.Equal(t, "full-bleed-opaque-static-set-v1", state.Normalization.ScaleAlgorithm)
		assertBoardIsFullyOpaque(t, state.NormalizedPath)
	}
}

func TestMaterialSwatchStaticSetUsesFullBleedMirroredRepeatContract(t *testing.T) {
	dir := testkit.WriteWaterCyclePack(t)
	p, all := testkit.LoadTargets(t, dir)
	for index := range all {
		all[index].RenderMode = pack.RenderModeMaterialSwatch
	}
	gen := &testkit.StaticSetProvider{FillCanvas: true}
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: outputDir,
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "water-cycle"},
	})

	require.NoError(t, err)
	require.Len(t, gen.Requests, 1)
	require.Contains(t, gen.Requests[0].Prompt, "full-bleed opaque material swatches")
	require.Contains(t, gen.Requests[0].Prompt, "exactly 3 separate editable rectangles")
	require.Contains(t, gen.Requests[0].Prompt, "never filling the complete provider canvas")
	require.NotContains(t, gen.Requests[0].Prompt, "tile seamlessly")
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	for _, target := range all {
		state := manifest.Targets[target.ID]
		require.Equal(t, "full-bleed-material-swatch-v1", state.Normalization.ScaleAlgorithm)
		assertBoardIsFullyOpaque(t, state.NormalizedPath)
	}
}

func TestStaticSetNormalizationVersionReprocessesWithoutProviderCall(t *testing.T) {
	dir := testkit.WriteStaticSetPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &testkit.StaticSetProvider{}
	opts := generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "fortification"},
	}

	_, err := generate.Run(context.Background(), all, gen, opts)
	require.NoError(t, err)
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	require.NoError(t, err)
	set := manifest.Intermediates["static-set:fortification"]
	set.StaticSetScale = nil
	for _, target := range all {
		manifest.Targets[target.ID].Normalization.ScaleAlgorithm = "native-provider-board-scale"
	}
	require.NoError(t, generate.Save(opts.OutputDir, opts.RunID, manifest))

	resume := &testkit.StaticSetProvider{}
	result, err := generate.Run(context.Background(), all, resume, opts)

	require.NoError(t, err)
	require.Empty(t, resume.Requests)
	require.Equal(t, 2, result.AwaitingReview)
	reprocessed, err := generate.Load(opts.OutputDir, opts.RunID)
	require.NoError(t, err)
	require.NotNil(t, reprocessed.Intermediates["static-set:fortification"].StaticSetScale)
	for _, target := range all {
		require.Equal(
			t,
			"canvas-class-static-set-alpha-fit-v1",
			reprocessed.Targets[target.ID].Normalization.ScaleAlgorithm,
		)
	}
}

func TestStaticSetGenerationResumeDoesNotRepeatProviderCall(t *testing.T) {
	dir := testkit.WriteStaticSetPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &testkit.StaticSetProvider{}
	opts := generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "fortification"},
	}

	_, err := generate.Run(context.Background(), all, gen, opts)
	require.NoError(t, err)
	_, err = generate.Run(context.Background(), all, gen, opts)

	require.NoError(t, err)
	require.Len(t, gen.Requests, 1)
}

func TestStaticSetMechanicalCandidateReprocessesWithoutProviderCall(t *testing.T) {
	dir := testkit.WriteStaticSetPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &testkit.StaticSetProvider{}
	opts := generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "fortification"},
	}

	_, err := generate.Run(context.Background(), all, gen, opts)
	require.NoError(t, err)
	require.Len(t, gen.Requests, 1)
	manifest, err := generate.Load(opts.OutputDir, opts.RunID)
	require.NoError(t, err)
	set := manifest.Intermediates["static-set:fortification"]
	require.Len(t, set.Attempts, 1)
	require.Len(t, set.Attempts[0].Candidates, 1)
	set.Status = generate.StatusRejected
	set.Lineage = ""
	set.NormalizedPath = ""
	set.HardRejections = []string{"obsolete mechanical rejection"}
	set.Attempts[0].SelectedCandidate = ""
	set.Attempts[0].Candidates[0].QualityVersion = 0
	set.Attempts[0].Candidates[0].HardRejections = []string{"obsolete mechanical rejection"}
	for _, target := range all {
		state := manifest.Targets[target.ID]
		state.Status = generate.StatusRejected
		state.NormalizedPath = ""
		state.HardRejections = []string{"obsolete mechanical rejection"}
	}
	require.NoError(t, generate.Save(opts.OutputDir, opts.RunID, manifest))

	resume := &testkit.StaticSetProvider{}
	result, err := generate.Run(context.Background(), all, resume, opts)

	require.NoError(t, err)
	require.Empty(t, resume.Requests)
	require.Equal(t, 2, result.AwaitingReview)
	reprocessed, err := generate.Load(opts.OutputDir, opts.RunID)
	require.NoError(t, err)
	require.Equal(
		t,
		generate.StatusAwaitingReview,
		reprocessed.Intermediates["static-set:fortification"].Status,
	)
}

func TestStaticSetMissingPartRejectsCompleteSet(t *testing.T) {
	dir := testkit.WriteStaticSetPack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &testkit.StaticSetProvider{PartCount: 1}
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, gen, generate.Options{
		OutputDir: outputDir,
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "fortification"},
	})

	require.NoError(t, err)
	require.Len(t, gen.Requests, 1)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	set := manifest.Intermediates["static-set:fortification"]
	require.Equal(t, generate.StatusRejected, set.Status)
	for _, target := range all {
		require.Equal(t, generate.StatusRejected, manifest.Targets[target.ID].Status)
		require.False(t, manifest.Targets[target.ID].ProductionEligible)
	}
}

func TestIsolatedStaticGenerationRejectsUnremovableFullCanvasBackdrop(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, corruptBackdropProvider{}, generate.Options{
		OutputDir: outputDir,
		DeployDir: filepath.Join(dir, p.DeployDir),
		RunID:     "run",
		Filter:    targets.Filter{Object: "grass"},
	})

	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	state := manifest.Targets["grass"]
	require.Equal(t, generate.StatusRejected, state.Status)
	require.Contains(t, state.HardRejections, "foreground_is_nonremovable_backdrop")
	require.False(t, state.ProductionEligible)
}

func TestStyleGuideSendsOnlyDeclaredBootstrapInputs(t *testing.T) {
	dir := testkit.WritePack(t)
	p, err := pack.Load(dir)
	require.NoError(t, err)
	target := targets.StyleGuideTarget(p)
	for index := range target.Inputs {
		target.Inputs[index].Path = filepath.Join(dir, target.Inputs[index].Path)
		target.Inputs[index].SourcePath = filepath.Join(dir, target.Inputs[index].SourcePath)
	}
	deployedGuide := filepath.Join(dir, p.StyleGuide.Deploy.Path)
	gen := &recordingProvider{}

	_, err = generate.Run(context.Background(), []targets.Target{target}, gen, generate.Options{
		OutputDir: filepath.Join(dir, p.OutputDir),
		DeployDir: dir,
		RunID:     "guide",
		Filter:    targets.Filter{Object: targets.StyleGuideTargetID},
	})

	require.NoError(t, err)
	require.Len(t, gen.requests, 1)
	require.Len(t, gen.requests[0].Inputs, len(p.StyleGuide.Inputs))
	for _, input := range gen.requests[0].Inputs {
		require.NotEqual(t, deployedGuide, input.Path)
	}
}

func TestStyleGuideReprocessesStaleNormalizationWithoutProviderCall(t *testing.T) {
	dir := testkit.WritePack(t)
	p, err := pack.Load(dir)
	require.NoError(t, err)
	target := targets.StyleGuideTarget(p)
	for index := range target.Inputs {
		target.Inputs[index].Path = filepath.Join(dir, target.Inputs[index].Path)
		target.Inputs[index].SourcePath = filepath.Join(
			dir,
			target.Inputs[index].SourcePath,
		)
	}
	outputDir := filepath.Join(dir, p.OutputDir)
	options := generate.Options{
		OutputDir: outputDir,
		DeployDir: dir,
		RunID:     "guide",
		Filter:    targets.Filter{Object: targets.StyleGuideTargetID},
	}
	initialProvider := &recordingProvider{}
	_, err = generate.Run(
		context.Background(),
		[]targets.Target{target},
		initialProvider,
		options,
	)
	require.NoError(t, err)
	require.Len(t, initialProvider.requests, 1)

	manifest, err := generate.Load(outputDir, "guide")
	require.NoError(t, err)
	state := manifest.Targets[targets.StyleGuideTargetID]
	require.NotEmpty(t, state.Attempts)
	require.NotEmpty(t, state.Attempts[0].Candidates)
	state.Attempts[0].Candidates[0].QualityVersion = 0
	require.NoError(t, generate.Save(outputDir, "guide", manifest))

	reprocessProvider := &recordingProvider{}
	_, err = generate.Run(
		context.Background(),
		[]targets.Target{target},
		reprocessProvider,
		options,
	)

	require.NoError(t, err)
	require.Empty(t, reprocessProvider.requests)
	reprocessed, err := generate.Load(outputDir, "guide")
	require.NoError(t, err)
	reprocessedState := reprocessed.Targets[targets.StyleGuideTargetID]
	require.Equal(
		t,
		imageio.CompositePaletteSize,
		reprocessedState.Normalization.MaximumColors,
	)
	require.Equal(
		t,
		"deterministic-composite-median-cut",
		reprocessedState.Normalization.PaletteMethod,
	)
	require.Greater(
		t,
		reprocessedState.Attempts[0].Candidates[0].QualityVersion,
		0,
	)
}

type recordingProvider struct {
	requests []provider.Request
	failAt   int
}

type corruptBackdropProvider struct{}

func (corruptBackdropProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true}
}

func (corruptBackdropProvider) Generate(_ context.Context, request provider.Request) (provider.Result, error) {
	img := image.NewNRGBA(image.Rect(0, 0, request.Size.X, request.Size.Y))
	for y := 0; y < request.Size.Y; y++ {
		for x := 0; x < request.Size.X; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x*17 + y*3) % 256),
				G: uint8((x*5 + y*19) % 256),
				B: uint8((x*11 + y*7) % 256),
				A: 255,
			})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return provider.Result{}, err
	}
	return provider.Result{PNG: encoded.Bytes()}, nil
}

func (p *recordingProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true}
}

func (p *recordingProvider) Generate(ctx context.Context, request provider.Request) (provider.Result, error) {
	p.requests = append(p.requests, request)
	if p.failAt != 0 && len(p.requests) == p.failAt {
		return provider.Result{}, errors.New("interrupted provider call")
	}
	return (provider.Fake{}).Generate(ctx, request)
}

type recoloringProvider struct {
	requests []provider.Request
}

func (p *recoloringProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true}
}

func (p *recoloringProvider) Generate(
	ctx context.Context,
	request provider.Request,
) (provider.Result, error) {
	p.requests = append(p.requests, request)
	result, err := (provider.Fake{}).Generate(ctx, request)
	if err != nil {
		return provider.Result{}, err
	}
	decoded, err := png.Decode(bytes.NewReader(result.PNG))
	if err != nil {
		return provider.Result{}, err
	}
	board := image.NewNRGBA(decoded.Bounds())
	background := color.NRGBAModel.Convert(decoded.At(
		decoded.Bounds().Min.X,
		decoded.Bounds().Min.Y,
	)).(color.NRGBA)
	foreground := color.NRGBA{R: 20, G: 220, B: 20, A: 255}
	if len(p.requests) == 1 {
		foreground = color.NRGBA{R: 200, G: 40, B: 40, A: 255}
	}
	for y := decoded.Bounds().Min.Y; y < decoded.Bounds().Max.Y; y++ {
		for x := decoded.Bounds().Min.X; x < decoded.Bounds().Max.X; x++ {
			value := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA)
			if value == background {
				board.SetNRGBA(x, y, background)
				continue
			}
			board.SetNRGBA(x, y, foreground)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, board); err != nil {
		return provider.Result{}, err
	}
	return provider.Result{PNG: encoded.Bytes(), Metadata: result.Metadata}, nil
}

type overflowingMasterProvider struct {
	requests []provider.Request
}

type smallerAnimationProvider struct {
	requests []provider.Request
}

func (p *smallerAnimationProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true}
}

func (p *smallerAnimationProvider) Generate(
	ctx context.Context,
	request provider.Request,
) (provider.Result, error) {
	p.requests = append(p.requests, request)
	if len(p.requests) == 1 {
		return (provider.Fake{}).Generate(ctx, request)
	}
	layout, err := imageio.SemanticAnimationLayout(3, 4)
	if err != nil {
		return provider.Result{}, err
	}
	board := image.NewNRGBA(image.Rect(0, 0, request.Size.X, request.Size.Y))
	background := color.NRGBA{R: 255, B: 255, A: 255}
	fillTestRect(board, board.Bounds(), background)
	foreground := color.NRGBA{R: 120, G: 80, B: 180, A: 255}
	for _, anchor := range layout.Anchors {
		bottom := anchor.Y + 106
		fillTestRect(
			board,
			image.Rect(anchor.X-60, bottom-120, anchor.X+60, bottom),
			foreground,
		)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, board); err != nil {
		return provider.Result{}, err
	}
	return provider.Result{
		PNG: encoded.Bytes(),
		Metadata: map[string]string{
			"provider": "smaller-animation-test",
		},
	}, nil
}

func (p *overflowingMasterProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true}
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
