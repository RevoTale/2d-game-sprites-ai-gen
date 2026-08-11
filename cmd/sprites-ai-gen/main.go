package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/catalog"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/deploy"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/envfile"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/gitguard"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/output"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/review"
	statusreport "github.com/RevoTale/2d-game-sprites-ai-gen/internal/status"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

var productionProvider = provider.OpenAIFromEnvironment

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}
	if err := rejectRemovedFlags(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "validate":
		fs, common := commonFlags("validate")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		p, all, err := loadTargetsFromCommon(common)
		if err != nil {
			return err
		}
		if err := output.Validate(filepath.Join(common.packDir, pack.OutputDir(p))); err != nil {
			return err
		}
		animatedObjects := make(map[string]struct{})
		staticObjects := make(map[string]struct{})
		for _, target := range all {
			if target.AnimationID == "" {
				staticObjects[target.ObjectID] = struct{}{}
				continue
			}
			animatedObjects[target.ObjectID] = struct{}{}
		}
		guideState := "approved"
		if _, statErr := os.Stat(filepath.Join(common.packDir, p.Style.Reference.Path)); errors.Is(statErr, os.ErrNotExist) {
			guideState = "missing_bootstrap_required"
		} else if statErr != nil {
			return fmt.Errorf("inspect style guide: %w", statErr)
		}
		fmt.Printf(
			"sprite pack is valid: animated_objects=%d static_objects=%d targets=%d style_guide=%s\n",
			len(animatedObjects),
			len(staticObjects),
			len(all),
			guideState,
		)
		return nil
	case "catalog":
		return runCatalog(args[1:])
	case "generate":
		return runGenerate(ctx, args[1:])
	case "status":
		return runStatus(args[1:])
	case "review":
		return runReview(args[1:])
	case "sheet":
		return errors.New("command sheet was removed; review artifacts are generated automatically")
	case "deploy-plan":
		return errors.New("command deploy-plan was removed; use deploy --dry-run")
	case "deploy":
		return runDeploy(args[1:])
	case "prune":
		return runPrune(args[1:])
	case "git-guard":
		return runGitGuard(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCatalog(args []string) error {
	fs := flag.NewFlagSet("catalog", flag.ContinueOnError)
	packDir := fs.String("pack", ".", "sprite pack directory")
	usageRoot := fs.String("usage-root", "", "optional directory containing map JSON packages")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("catalog does not accept positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	p, err := pack.Load(*packDir)
	if err != nil {
		return err
	}
	all, err := targets.Expand(p)
	if err != nil {
		return err
	}
	deployDir := p.DeployDir
	if deployDir == "" {
		return errors.New("deployDir is required for catalog")
	}
	if !filepath.IsAbs(deployDir) {
		deployDir = filepath.Join(*packDir, deployDir)
	}
	resolvedUsageRoot := *usageRoot
	if resolvedUsageRoot != "" && !filepath.IsAbs(resolvedUsageRoot) {
		resolvedUsageRoot = filepath.Join(*packDir, resolvedUsageRoot)
	}
	result, err := catalog.Build(p, all, catalog.Options{
		PackDir:   *packDir,
		DeployDir: deployDir,
		UsageRoot: resolvedUsageRoot,
		OutputDir: filepath.Join(*packDir, pack.OutputDir(p), "catalog"),
	})
	if err != nil {
		return err
	}
	fmt.Printf("catalog_entries: %d\nindex: %s\nmetadata: %s\n", result.Entries, result.IndexPath, result.MetadataPath)
	return nil
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	packDir := fs.String("pack", ".", "sprite pack directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files := map[string]string{
		".env.example": "# OpenAI is the only production image provider.\nOPENAI_API_KEY=\nSPRITES_AI_GEN_OPENAI_MODEL=gpt-image-2\n",
		".gitignore":   ".env\n.env.*\n!.env.example\noutput/\n",
		"sprites.json": starterSpritesJSON,
	}
	for name, content := range files {
		path := filepath.Join(*packDir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return writeStarterStyleInput(*packDir)
}

func writeStarterStyleInput(packDir string) error {
	path := filepath.Join(packDir, "references", "original-style-input.png")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	for y := 32; y < 128; y++ {
		for x := 32; x < 128; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 100, G: 80, B: 160, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data.Bytes(), 0o644)
}

func runGenerate(ctx context.Context, args []string) error {
	fs, common := commonFlags("generate")
	allObjects := fs.Bool("all", false, "generate every object after reporting the exact provider-call count")
	reprocessOnly := fs.Bool(
		"reprocess-only",
		false,
		"rebuild artifacts from an existing raw candidate without allowing provider calls",
	)
	var excluded stringListFlag
	fs.Var(&excluded, "exclude-object", "object id to exclude from --all; repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	all, deployDir, err := scopedTargets(common, p, all)
	if err != nil {
		return err
	}
	if common.styleGuide && (*allObjects || len(excluded) != 0) {
		return errors.New("--style-guide cannot be combined with --all or --exclude-object")
	}
	if *allObjects && common.object != "" {
		return errors.New("--all cannot be combined with --object")
	}
	if len(excluded) != 0 && !*allObjects {
		return errors.New("--exclude-object is valid only with --all")
	}
	if !common.styleGuide && !*allObjects && common.object == "" {
		return errors.New("paid generation requires --object <id> or explicit --all")
	}
	if *reprocessOnly {
		if common.runID == "" || common.runID == "auto" {
			return errors.New("--reprocess-only requires an existing concrete --run id")
		}
		if _, loadErr := generate.Load(filepath.Join(common.packDir, pack.OutputDir(p)), common.runID); loadErr != nil {
			return fmt.Errorf("load --reprocess-only run: %w", loadErr)
		}
	}
	filter := common.filter()
	if *allObjects {
		filter = targets.Filter{Exclude: map[string]bool{}}
		known := map[string]bool{}
		for _, id := range targets.ObjectIDs(all) {
			known[id] = true
		}
		for _, id := range excluded {
			if !known[id] {
				return fmt.Errorf("--exclude-object %q does not match a configured object", id)
			}
			filter.Exclude[id] = true
		}
	}
	if _, err := targets.Select(all, filter); err != nil {
		return fmt.Errorf("select generation scope: %w", err)
	}
	out := pack.OutputDir(p)
	if *allObjects {
		selected := targets.FilterTargets(all, filter)
		var manifest *generate.Manifest
		if common.runID != "" && common.runID != "auto" {
			manifest, err = generate.Load(filepath.Join(common.packDir, out), common.runID)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		fmt.Printf(
			"provider_calls_planned: %d\n",
			generate.ProviderCallsRemaining(manifest, selected),
		)
	}
	if !common.styleGuide {
		if err := requireApprovedStyleGuide(common.packDir, p); err != nil {
			return err
		}
	}
	var gen provider.Provider
	if *reprocessOnly {
		gen = reprocessOnlyProvider{}
	} else {
		env, envErr := generateEnvironment(common.packDir)
		if envErr != nil {
			return envErr
		}
		gen, err = productionProvider(env)
		if err != nil {
			return err
		}
	}
	all = resolveReferencePaths(all, common.packDir)
	if err := output.Validate(filepath.Join(common.packDir, out)); err != nil {
		return err
	}
	configSHA256, err := pathSHA256(filepath.Join(common.packDir, "sprites.json"))
	if err != nil {
		return err
	}
	styleGuideSHA256 := ""
	if !common.styleGuide {
		styleGuideSHA256, err = pathSHA256(filepath.Join(common.packDir, p.Style.Reference.Path))
		if err != nil {
			return err
		}
	}
	result, err := generate.Run(ctx, all, gen, generate.Options{
		OutputDir:        filepath.Join(common.packDir, out),
		DeployDir:        deployDir,
		RunID:            common.runID,
		Filter:           filter,
		ConfigSHA256:     configSHA256,
		StyleGuideSHA256: styleGuideSHA256,
		ContinueOnError:  *allObjects,
		Progress: func(event generate.ProgressEvent) {
			fmt.Println(formatGenerationProgress(event))
		},
	})
	if err != nil {
		return err
	}
	fmt.Printf(
		"run_id: %s\ngenerated: %d\nskipped: %d\nfailed: %d\nawaiting_review: %d\n",
		result.RunID,
		result.Generated,
		result.Skipped,
		result.Failed,
		result.AwaitingReview,
	)
	manifest, err := generate.Load(filepath.Join(common.packDir, out), result.RunID)
	if err != nil {
		return err
	}
	return statusreport.Print(os.Stdout, manifest, all, filter)
}

type reprocessOnlyProvider struct{}

func (reprocessOnlyProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{References: true, Masks: true}
}

func (reprocessOnlyProvider) Generate(context.Context, provider.Request) (provider.Result, error) {
	return provider.Result{}, errors.New(
		"--reprocess-only cannot call OpenAI; the selected run has no reusable raw candidate",
	)
}

func pathSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func requireApprovedStyleGuide(packDir string, p *pack.Pack) error {
	path := filepath.Join(packDir, p.Style.Reference.Path)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(
				"approved style guide %q is missing; generate, review, and deploy it with --style-guide before asset generation",
				p.Style.Reference.Path,
			)
		}
		return fmt.Errorf("inspect approved style guide %q: %w", p.Style.Reference.Path, err)
	}
	return nil
}

func formatGenerationProgress(event generate.ProgressEvent) string {
	switch event.Stage {
	case generate.ProgressRunStarted:
		return fmt.Sprintf("run: %s", event.RunID)
	case generate.ProgressIntermediateGenerating:
		return fmt.Sprintf("progress: intermediate %s generating", event.TargetID)
	case generate.ProgressIntermediateReady:
		return fmt.Sprintf("progress: intermediate %s ready", event.TargetID)
	case generate.ProgressTargetGenerating:
		return fmt.Sprintf("progress: target %d/%d %s generating", event.Current, event.Total, event.TargetID)
	case generate.ProgressCandidateGenerating:
		if event.Total == 0 {
			return fmt.Sprintf("progress: intermediate %s candidate %d/%d generating", event.TargetID, event.Candidate, event.Candidates)
		}
		return fmt.Sprintf("progress: target %d/%d %s candidate %d/%d generating", event.Current, event.Total, event.TargetID, event.Candidate, event.Candidates)
	case generate.ProgressProviderProgress:
		if event.Total == 0 {
			return fmt.Sprintf("progress: intermediate %s candidate %d/%d provider poll %d", event.TargetID, event.Candidate, event.Candidates, event.ProviderCurrent)
		}
		return fmt.Sprintf("progress: target %d/%d %s candidate %d/%d provider poll %d", event.Current, event.Total, event.TargetID, event.Candidate, event.Candidates, event.ProviderCurrent)
	case generate.ProgressCandidateReady:
		if event.Total == 0 {
			return fmt.Sprintf("progress: intermediate %s candidate %d/%d ready", event.TargetID, event.Candidate, event.Candidates)
		}
		return fmt.Sprintf("progress: target %d/%d %s candidate %d/%d ready", event.Current, event.Total, event.TargetID, event.Candidate, event.Candidates)
	case generate.ProgressTargetReady:
		return fmt.Sprintf("progress: target %d/%d %s ready", event.Current, event.Total, event.TargetID)
	case generate.ProgressTargetSkipped:
		return fmt.Sprintf("progress: target %d/%d %s skipped", event.Current, event.Total, event.TargetID)
	case generate.ProgressRunCompleted:
		return "progress: run complete"
	default:
		return "progress: unknown stage"
	}
}

func generateEnvironment(packDir string) (map[string]string, error) {
	return envfile.Merge(os.Environ(), filepath.Join(packDir, ".env"))
}

func resolveReferencePaths(all []targets.Target, packDir string) []targets.Target {
	out := append([]targets.Target(nil), all...)
	for i := range out {
		inputs := append([]conditioning.Input(nil), out[i].Inputs...)
		for inputIndex := range inputs {
			if inputs[inputIndex].Path != "" && !filepath.IsAbs(inputs[inputIndex].Path) {
				inputs[inputIndex].Path = filepath.Join(packDir, inputs[inputIndex].Path)
			}
			if inputs[inputIndex].SourcePath != "" && !filepath.IsAbs(inputs[inputIndex].SourcePath) {
				inputs[inputIndex].SourcePath = filepath.Join(packDir, inputs[inputIndex].SourcePath)
			}
		}
		out[i].Inputs = inputs
		if out[i].DirectionRefPath != "" && !filepath.IsAbs(out[i].DirectionRefPath) {
			out[i].DirectionRefPath = filepath.Join(packDir, out[i].DirectionRefPath)
		}
	}
	return out
}

func runStatus(args []string) error {
	fs, common := commonFlags("status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireConcreteRunID(common.runID); err != nil {
		return err
	}
	p, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	all, _, err = scopedTargets(common, p, all)
	if err != nil {
		return err
	}
	manifest, err := generate.Load(filepath.Join(common.packDir, pack.OutputDir(p)), common.runID)
	if err != nil {
		return err
	}
	return statusreport.Print(os.Stdout, manifest, all, common.filter())
}

func runReview(args []string) error {
	fs, common := commonFlags("review")
	status := fs.String("status", "", "accepted or rejected")
	reason := fs.String("reason", "", "review reason")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireConcreteRunID(common.runID); err != nil {
		return err
	}
	p, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	all, _, err = scopedTargets(common, p, all)
	if err != nil {
		return err
	}
	outputDir := filepath.Join(common.packDir, pack.OutputDir(p))
	result, err := review.Apply(all, review.Options{OutputDir: outputDir, RunID: common.runID, Filter: common.filter(), Status: *status, Reason: *reason})
	fmt.Printf("reviewed: %d\nskipped_pending: %d\n", result.Reviewed, result.SkippedPending)
	for _, warning := range result.Warnings {
		fmt.Printf("warning: %s\n", warning)
	}
	if err != nil {
		return err
	}
	manifest, err := generate.Load(outputDir, common.runID)
	if err != nil {
		return err
	}
	return statusreport.Print(os.Stdout, manifest, all, common.filter())
}

func runDeploy(args []string) error {
	fs, common := commonFlags("deploy")
	dryRun := fs.Bool("dry-run", false, "show deploy plan without copying")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireConcreteRunID(common.runID); err != nil {
		return err
	}
	p, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	all, deployDir, err := scopedTargets(common, p, all)
	if err != nil {
		return err
	}
	if deployDir == "" {
		return errors.New("deployDir is required")
	}
	opts := deploy.Options{OutputDir: filepath.Join(common.packDir, pack.OutputDir(p)), RunID: common.runID, DeployDir: deployDir, Filter: common.filter()}
	if *dryRun {
		plan, err := deploy.BuildPlan(all, opts)
		fmt.Printf("run_id: %s\nstate: dry_run\n", common.runID)
		fmt.Print(deploy.FormatPlan(plan))
		return err
	}
	plan, err := deploy.Execute(all, opts)
	fmt.Print(deploy.FormatPlan(plan))
	if err != nil {
		return err
	}
	manifest, err := generate.Load(opts.OutputDir, common.runID)
	if err != nil {
		return err
	}
	return statusreport.Print(os.Stdout, manifest, all, common.filter())
}

func runPrune(args []string) error {
	fs, common := commonFlags("prune")
	onlyRaw := fs.Bool("only-raw", false, "remove raw attempts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireConcreteRunID(common.runID); err != nil {
		return err
	}
	p, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	all, _, err = scopedTargets(common, p, all)
	if err != nil {
		return err
	}
	if !*onlyRaw {
		return errors.New("only --only-raw pruning is supported")
	}
	outputDir := filepath.Join(common.packDir, pack.OutputDir(p))
	selected, err := targets.Select(all, common.filter())
	if err != nil {
		return err
	}
	removed, err := generate.PruneRaw(outputDir, common.runID, selected)
	if err != nil {
		return err
	}
	fmt.Printf("pruned raw images: %d\n", removed)
	manifest, err := generate.Load(outputDir, common.runID)
	if err != nil {
		return err
	}
	return statusreport.Print(os.Stdout, manifest, all, common.filter())
}

func runGitGuard(args []string) error {
	fs, common := commonFlags("git-guard")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	outputDir := filepath.Join(common.packDir, pack.OutputDir(p))
	offenders, err := gitguard.Check(outputDir)
	if err != nil {
		return err
	}
	if len(offenders) == 0 {
		fmt.Println("no generated PNG artifacts are tracked or staged")
		return nil
	}
	for _, path := range offenders {
		fmt.Println(path)
	}
	return fmt.Errorf("%d generated PNG artifacts are tracked or staged", len(offenders))
}

type commonOptions struct {
	packDir    string
	runID      string
	object     string
	styleGuide bool
}

func commonFlags(name string) (*flag.FlagSet, *commonOptions) {
	common := &commonOptions{runID: "auto"}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&common.packDir, "pack", ".", "sprite pack directory")
	fs.StringVar(&common.runID, "run", "auto", "run id")
	fs.StringVar(&common.object, "object", "", "object id filter")
	fs.BoolVar(&common.styleGuide, "style-guide", false, "select the configured original style guide")
	return fs, common
}

func rejectRemovedFlags(args []string) error {
	removed := map[string]string{
		"--output":        "paths come from sprites.json",
		"--deploy-dir":    "paths come from sprites.json",
		"--allow-partial": "pending targets are ignored and animated units are reviewed atomically",
		"--complete":      "deployment always selects complete accepted groups",
		"--candidate":     "each attempt has exactly one candidate and candidate selection is automatic",
		"--stage":         "V13 reviews the complete unit without an intermediate stage",
		"--provider":      "OpenAI is the only production provider",
		"--fake":          "the fake provider is private to automated tests",
		"--force":         "rejected assets require a fresh V13 run",
		"--animation":     "V13 animated generation, review, and deployment are complete-unit atomic",
		"--variant":       "V13 uses configured directions and complete-unit atomicity",
		"--frame":         "V13 does not support partial animated generation",
	}
	for _, arg := range args {
		name := arg
		if index := strings.IndexByte(name, '='); index >= 0 {
			name = name[:index]
		}
		if replacement, ok := removed[name]; ok {
			return fmt.Errorf("flag %s was removed; %s", name, replacement)
		}
	}
	return nil
}

func plannedProviderCalls(all []targets.Target, filter targets.Filter) int {
	return generate.ProviderCallsRemaining(nil, targets.FilterTargets(all, filter))
}

func requireConcreteRunID(runID string) error {
	if runID == "" || runID == "auto" {
		return errors.New("--run requires an existing run id")
	}
	return nil
}

func (c *commonOptions) filter() targets.Filter {
	if c.styleGuide {
		return targets.Filter{Object: targets.StyleGuideTargetID}
	}
	return targets.Filter{Object: c.object}
}

type stringListFlag []string

func (m *stringListFlag) String() string { return strings.Join(*m, ",") }
func (m *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("object id must not be empty")
	}
	for _, existing := range *m {
		if existing == value {
			return fmt.Errorf("object %q was specified more than once", value)
		}
	}
	*m = append(*m, value)
	return nil
}

func loadTargetsFromCommon(common *commonOptions) (*pack.Pack, []targets.Target, error) {
	p, err := pack.Load(common.packDir)
	if err != nil {
		return nil, nil, err
	}
	all, err := targets.Expand(p)
	if err != nil {
		return nil, nil, err
	}
	return p, all, nil
}

func scopedTargets(
	common *commonOptions,
	p *pack.Pack,
	all []targets.Target,
) ([]targets.Target, string, error) {
	if common.styleGuide {
		if common.object != "" {
			return nil, "", errors.New("--style-guide cannot be combined with --object")
		}
		return []targets.Target{targets.StyleGuideTarget(p)}, common.packDir, nil
	}
	deployDir := p.DeployDir
	if deployDir != "" && !filepath.IsAbs(deployDir) {
		deployDir = filepath.Join(common.packDir, deployDir)
	}
	return all, deployDir, nil
}

const starterSpritesJSON = `{
  "version": 6,
  "outputDir": "output",
  "deployDir": "deploy",
  "style": {
    "id": "compact-dark-fantasy-tactics",
    "description": "Original compact dark-fantasy tactics-RPG pixel art with broad silhouettes, strong dark contours, and restrained clustered shading.",
    "principles": [
      "Prefer broad readable pixel clusters over high-frequency noise.",
      "Keep focal features readable at battlefield and portrait scale."
    ],
    "palette": {
      "maxColors": 32,
      "colorSpace": "linear-srgb",
      "alpha": "binary",
      "dithering": "none"
    },
    "contrastHierarchy": ["magic-effects", "units", "structures", "terrain"],
    "units": {
      "common": ["Use compact broad silhouettes and stable grounded registration."],
      "archetypes": {
        "compact-humanoid": {
          "description": "Compact readable tactics-RPG humanoid.",
          "scaleClass": "standard-humanoid",
          "rules": ["Use a large focal head, broad shoulders, and short grounded legs."]
        }
      }
    },
    "terrain": {
      "common": ["Use broad connected materials and reserve high contrast for gameplay subjects."],
      "families": {
        "ground": {
          "description": "Quiet seamless battlefield ground.",
          "rules": ["Keep large mid-dark material regions and rare bright accents."]
        }
      }
    },
    "forbidden": [
      "No copied proprietary characters, terrain, UI, compositions, or motifs.",
      "No blur, antialiasing, labels, platforms, or baked shadows."
    ],
    "reference": {
      "id": "compact-dark-fantasy-style-v1",
      "path": "references/style/compact-dark-fantasy-style-v1.png",
      "description": "Approved original project style guide."
    }
  },
  "styleGuide": {
    "description": "One original style board showing a compact armored humanoid, compact caster, agile humanoid, broad monster, quiet ground with blocky ruin, and restrained crystal magic.",
    "size": {"width": 1536, "height": 1024},
    "inputs": [{
      "id": "original-style-input",
      "role": "style",
      "path": "references/original-style-input.png",
      "description": "Original project-owned visual evidence."
    }],
    "deploy": {"path": "references/style/compact-dark-fantasy-style-v1.png"}
  },
  "objects": [
    {
      "id": "ground-example",
      "kind": "static",
      "family": "ground",
      "renderMode": "opaque-tile",
      "registration": "canvas",
      "description": "Quiet seamless dark battlefield ground.",
      "magicSources": [],
      "size": {"width": 256, "height": 256},
      "deploy": {"pathTemplate": "terrain/ground-example.png"}
    }
  ]
}
`
