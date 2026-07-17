package generate_test

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/output"
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

func TestGenerateFailsMissingAnimatedPoseBeforeCreatingRunOrCallingProvider(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "blood-duelist"}})

	require.ErrorContains(t, err, "requires an existing deployed frame")
	require.Empty(t, gen.requests)
	require.NoFileExists(t, generate.ManifestPath(outputDir, "run"))
}

func TestCapabilityPreflightRejectsAnimatedRequestsBeforeProviderCall(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &capabilityProvider{capabilities: provider.Capabilities{Masks: true}}
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})

	require.ErrorContains(t, err, "reference and mask support")
	require.Zero(t, gen.calls)
	require.NoFileExists(t, generate.ManifestPath(outputDir, "run"))
}

func TestCapabilityPreflightRejectsOptionalStaticReferencesBeforeProviderCall(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &capabilityProvider{}
	outputDir := filepath.Join(dir, p.OutputDir)

	_, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})

	require.ErrorContains(t, err, "uses image references")
	require.Zero(t, gen.calls)
	require.NoFileExists(t, generate.ManifestPath(outputDir, "run"))
}

func TestE2EFirstAnimatedGenerationCreatesCombinedSeedBoardAndWaitsForReview(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}
	filter := rightAttack()

	result, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: filepath.Join(dir, p.OutputDir), RunID: "run", Filter: filter})

	require.NoError(t, err)
	require.Zero(t, result.Generated)
	require.Equal(t, 2, result.AwaitingReview)
	require.Len(t, gen.requests, 3)
	for _, request := range gen.requests {
		require.Equal(t, conditioning.RolePose, request.Inputs[0].Role)
		require.Contains(t, request.Prompt, "Combined Directional Seed Board")
		seen := map[struct {
			path string
			role conditioning.Role
		}]bool{}
		for _, input := range request.Inputs {
			key := struct {
				path string
				role conditioning.Role
			}{path: input.Path, role: input.Role}
			require.False(t, seen[key], "seed request contains duplicate input %s", input.Path)
			seen[key] = true
		}
	}
	manifest, err := generate.Load(filepath.Join(dir, p.OutputDir), "run")
	require.NoError(t, err)
	require.Equal(t, 5, manifest.Version)
	seed := manifest.Intermediates["direction-seed-board:blood-duelist"]
	require.Equal(t, generate.StatusAwaitingReview, seed.Status)
	require.Equal(t, 2, seed.Layout.Count)
	require.Nil(t, seed.Review)
	require.FileExists(t, seed.Artifacts.PromptPath)
	require.FileExists(t, seed.Artifacts.QAPath)
	require.FileExists(t, seed.Artifacts.CandidateSheetPath)
	sheetFile, err := os.Open(seed.Artifacts.CandidateSheetPath)
	require.NoError(t, err)
	sheet, err := png.Decode(sheetFile)
	require.NoError(t, err)
	require.NoError(t, sheetFile.Close())
	require.Equal(t, image.Rect(0, 0, 3072, 1064), sheet.Bounds())
	require.Empty(t, manifest.Intermediates["animation-row:blood-duelist__direction-right__animation-attack"])
}

func TestE2ENoSelectorGeneratesStaticTargetsAndSeedsButNoRowsBeforeSeedApproval(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}
	outputDir := filepath.Join(dir, p.OutputDir)

	result, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run"})

	require.NoError(t, err)
	require.Equal(t, 1, result.Generated)
	require.Equal(t, 4, result.AwaitingReview)
	require.Len(t, gen.requests, 4, "one static request plus three directional-seed candidates")
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	static := manifest.Targets["grass"]
	require.Equal(t, generate.StatusAwaitingReview, static.Status)
	require.FileExists(t, static.Artifacts.PromptPath)
	require.FileExists(t, static.Artifacts.QAPath)
	require.Equal(t, generate.StatusAwaitingReview, manifest.Intermediates["direction-seed-board:blood-duelist"].Status)
	for _, intermediate := range manifest.Intermediates {
		require.NotEqual(t, "animation-row", intermediate.Kind)
	}
}

func TestE2EAcceptedSeedResumesCompleteRowGenerationAndFixedCellExtraction(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	gen := &recordingProvider{}
	require.NoError(t, generateSeed(t, all, gen, outputDir))

	result, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})

	require.NoError(t, err)
	require.Equal(t, 2, result.Generated)
	require.Len(t, gen.requests, 4)
	for _, request := range gen.requests[3:] {
		require.Contains(t, request.Prompt, "Complete Animation Row")
		require.Equal(t, []conditioning.Role{conditioning.RolePose, conditioning.RoleIdentity, conditioning.RolePose, conditioning.RoleMask}, inputRoles(request.Inputs)[:4])
		require.NotContains(t, inputRoles(request.Inputs), conditioning.RoleStyle)
	}
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	row := findIntermediate(t, manifest, "animation-row")
	require.Len(t, row.Attempts, 1)
	require.Len(t, row.Attempts[0].Candidates, 1)
	require.FileExists(t, row.Artifacts.PromptPath)
	require.FileExists(t, row.Artifacts.QAPath)
	require.FileExists(t, row.Artifacts.CandidateSheetPath)
	require.FileExists(t, row.Artifacts.ContactSheetPath)
	require.FileExists(t, row.Artifacts.AnimationGIFPath)
	require.Len(t, row.Artifacts.FramePaths, 2)
	for _, id := range []string{"blood-duelist__attack__direction-right__00", "blood-duelist__attack__direction-right__contact"} {
		state := manifest.Targets[id]
		require.Equal(t, generate.StatusAwaitingReview, state.Status)
		require.Equal(t, "validated_row_extraction", state.CapabilityMode)
		require.True(t, state.ProductionEligible)
		require.Equal(t, row.Lineage, state.RowLineage)
		require.FileExists(t, state.NormalizedPath)
	}
}

func TestGenerateRecordsSeedCandidateOrdinalsWithoutProviderSeedClaims(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	gen := &recordingProvider{}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: filepath.Join(dir, p.OutputDir), RunID: "run", Filter: rightAttack()})

	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, []int{gen.requests[0].CandidateOrdinal, gen.requests[1].CandidateOrdinal, gen.requests[2].CandidateOrdinal})
}

func TestE2ERejectedFrameRepairRequiresForceAndResetsEveryFrameReview(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	deployDir := filepath.Join(dir, p.DeployDir)
	for _, frameID := range []string{"00", "contact"} {
		path := filepath.Join(deployDir, "units", "blood-duelist__attack__right__"+frameID+".png")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, testkit.PNGWithMargin(t, 16, 16, 3), 0o644))
	}
	require.NoError(t, generateSeed(t, all, &recordingProvider{}, outputDir))
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	row := findIntermediate(t, manifest, "animation-row")
	row.Status = generate.StatusRejected
	for _, state := range manifest.Targets {
		if state.AnimationRowID != "" {
			state.Status = generate.StatusRejected
			state.Review = &generate.ReviewRecord{Status: generate.StatusRejected, Reason: "repair this cell", ReviewedAt: time.Now().UTC().Format(time.RFC3339)}
		}
	}
	require.NoError(t, generate.Save(outputDir, "run", manifest))
	frame := targets.Filter{Object: "blood-duelist", Animation: "attack", Frame: "contact", Variants: map[string]string{"direction": "right"}}
	gen := &recordingProvider{}

	result, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: frame})

	require.NoError(t, err)
	require.Empty(t, gen.requests)
	require.Equal(t, 2, result.Skipped)

	_, err = generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, DeployDir: deployDir, RunID: "run", Filter: frame, Force: true})

	require.NoError(t, err)
	manifest, err = generate.Load(outputDir, "run")
	require.NoError(t, err)
	row = findIntermediate(t, manifest, "animation-row")
	require.Len(t, row.Attempts, 2)
	require.Equal(t, "row", row.Attempts[1].Kind)
	for _, state := range manifest.Targets {
		if state.AnimationRowID == row.ID {
			require.Equal(t, generate.StatusAwaitingReview, state.Status)
			require.Nil(t, state.Review)
			require.NotNil(t, state.Production)
			require.True(t, state.Production.Exists)
			require.NotEmpty(t, state.Production.SHA256)
		}
	}
}

func TestFrameRepairFailsBeforeProviderWhenProductionRowIsIncomplete(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	require.NoError(t, generateSeed(t, all, &recordingProvider{}, outputDir))
	gen := &recordingProvider{}
	frame := targets.Filter{Object: "blood-duelist", Animation: "attack", Frame: "contact", Variants: map[string]string{"direction": "right"}}

	_, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, DeployDir: filepath.Join(dir, p.DeployDir), RunID: "run", Filter: frame})

	require.ErrorContains(t, err, "complete current production row")
	require.Empty(t, gen.requests)
}

func TestE2ELegacyManifestsRemainOnDiskButCannotResumeGeneration(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			dir := testkit.WritePack(t)
			p, all := testkit.LoadTargets(t, dir)
			outputDir := filepath.Join(dir, p.OutputDir)
			legacy := &generate.Manifest{Version: version, RunID: "legacy", Targets: map[string]*generate.TargetState{"grass": {ID: "grass", Status: generate.StatusPending}}}
			require.NoError(t, generate.Save(outputDir, "legacy", legacy))
			_, err := generate.Load(outputDir, "legacy")
			require.ErrorContains(t, err, "unsupported manifest")
			require.ErrorContains(t, err, "start a new run")

			_, err = generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "legacy", Filter: targets.Filter{Object: "grass"}})
			require.ErrorContains(t, err, "unsupported manifest")
		})
	}
}

func TestE2EStaticTargetStillGeneratesWithoutAnimatedIntermediates(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	gen := &recordingProvider{}

	result, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run", Filter: targets.Filter{Object: "grass"}})

	require.NoError(t, err)
	require.Equal(t, 1, result.Generated)
	require.Len(t, gen.requests, 1)
	require.Equal(t, image.Pt(1024, 1024), gen.requests[0].Size)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	require.Empty(t, manifest.Intermediates)
	require.Equal(t, "static", manifest.Targets["grass"].CapabilityMode)
}

func TestAcceptedStaticTargetIsSkippedEvenWithForce(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "grass"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	manifest.Targets["grass"].Status = generate.StatusAccepted
	require.NoError(t, generate.Save(outputDir, "run", manifest))
	gen := &recordingProvider{}

	result, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Force: true})

	require.NoError(t, err)
	require.Equal(t, 1, result.Skipped)
	require.Empty(t, gen.requests)
}

func TestRejectedStaticTargetRequiresForceForAnotherAttempt(t *testing.T) {
	dir := testkit.WritePack(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	filter := targets.Filter{Object: "grass"}
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	manifest.Targets["grass"].Status = generate.StatusRejected
	require.NoError(t, generate.Save(outputDir, "run", manifest))
	withoutForce := &recordingProvider{}
	_, err = generate.Run(context.Background(), all, withoutForce, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter})
	require.NoError(t, err)
	require.Empty(t, withoutForce.requests)
	withForce := &recordingProvider{}

	_, err = generate.Run(context.Background(), all, withForce, generate.Options{OutputDir: outputDir, RunID: "run", Filter: filter, Force: true})

	require.NoError(t, err)
	require.Len(t, withForce.requests, 1)
}

func TestPruneRawRemovesSeedAndRowRawsButPreservesSelectedCorrectionSources(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	require.NoError(t, generateSeed(t, all, &recordingProvider{}, outputDir))
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})
	require.NoError(t, err)

	removed, err := generate.PruneRaw(outputDir, "run", targets.FilterTargets(all, rightAttack()))

	require.NoError(t, err)
	require.Equal(t, 4, removed)
	manifest, err := generate.Load(outputDir, "run")
	require.NoError(t, err)
	for _, intermediate := range manifest.Intermediates {
		require.FileExists(t, intermediate.NormalizedPath)
		require.FileExists(t, intermediate.EditSourcePath)
	}
}

func TestE2EGeneratedOutputPassesStrictManagedLayoutValidation(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	require.NoError(t, generateSeed(t, all, &recordingProvider{}, outputDir))
	_, err := generate.Run(context.Background(), all, provider.Fake{}, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})
	require.NoError(t, err)

	require.NoError(t, output.Validate(outputDir))
}

func TestE2EInterruptedSeedGenerationResumesOnlyMissingCandidates(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	p, all := testkit.LoadTargets(t, dir)
	outputDir := filepath.Join(dir, p.OutputDir)
	failing := &failAfterProvider{failAt: 2}
	_, err := generate.Run(context.Background(), all, failing, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})
	require.Error(t, err)
	resumed := &recordingProvider{}

	result, err := generate.Run(context.Background(), all, resumed, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})

	require.NoError(t, err)
	require.Equal(t, 2, result.AwaitingReview)
	require.Len(t, resumed.requests, 2)
}

func generateSeed(t *testing.T, all []targets.Target, gen provider.Provider, outputDir string) error {
	t.Helper()
	_, err := generate.Run(context.Background(), all, gen, generate.Options{OutputDir: outputDir, RunID: "run", Filter: rightAttack()})
	if err != nil {
		return err
	}
	return generate.SelectSeedCandidate(all, outputDir, "run", "blood-duelist", "01", generate.StatusAccepted, "Approved fixture seed.")
}

func rightAttack() targets.Filter {
	return targets.Filter{Object: "blood-duelist", Animation: "attack", Variants: map[string]string{"direction": "right"}}
}

type recordingProvider struct {
	requests []provider.Request
}

func (*recordingProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true, Masks: true}
}

func (p *recordingProvider) Generate(ctx context.Context, request provider.Request) (provider.Result, error) {
	copyRequest := request
	copyRequest.Inputs = append([]conditioning.Input(nil), request.Inputs...)
	p.requests = append(p.requests, copyRequest)
	return provider.Fake{}.Generate(ctx, request)
}

type failAfterProvider struct {
	calls  int
	failAt int
}

type capabilityProvider struct {
	capabilities provider.Capabilities
	calls        int
}

func (p *capabilityProvider) Capabilities() provider.Capabilities { return p.capabilities }

func (p *capabilityProvider) Generate(context.Context, provider.Request) (provider.Result, error) {
	p.calls++
	return provider.Result{}, errors.New("provider must not be called")
}

func (*failAfterProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true, Masks: true}
}

func (p *failAfterProvider) Generate(ctx context.Context, request provider.Request) (provider.Result, error) {
	p.calls++
	if p.calls == p.failAt {
		return provider.Result{}, errors.New("interrupted")
	}
	return provider.Fake{}.Generate(ctx, request)
}

func inputRoles(inputs []conditioning.Input) []conditioning.Role {
	roles := make([]conditioning.Role, len(inputs))
	for index, input := range inputs {
		roles[index] = input.Role
	}
	return roles
}

func findIntermediate(t *testing.T, manifest *generate.Manifest, kind string) *generate.IntermediateState {
	t.Helper()
	for _, state := range manifest.Intermediates {
		if state.Kind == kind {
			return state
		}
	}
	require.FailNow(t, "intermediate not found", kind)
	return nil
}
