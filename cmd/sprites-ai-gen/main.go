package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

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
		p, _, all, err := loadTargetsFromCommon(common)
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
		fmt.Printf("sprite pack is valid: animated_objects=%d static_objects=%d targets=%d\n", len(animatedObjects), len(staticObjects), len(all))
		return nil
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

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	packDir := fs.String("pack", ".", "sprite pack directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files := map[string]string{
		"THEME.md":     "# Theme\n\nHigh-detail pixel art with clean readable silhouettes.\n",
		".env.example": "# OpenAI is the supported real image provider.\nSPRITES_AI_GEN_PROVIDER=openai\nOPENAI_API_KEY=\n",
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
	return writeStarterDirectionReference(*packDir)
}

func writeStarterDirectionReference(packDir string) error {
	path := filepath.Join(packDir, "references", "current-right.png")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	for y := 36; y < 148; y++ {
		for x := 52; x < 108; x++ {
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
	providerName := fs.String("provider", "", "real provider: openai; auto-detects from provider env when omitted")
	fake := fs.Bool("fake", false, "use deterministic fake provider for tests and plumbing checks")
	force := fs.Bool("force", false, "retry explicitly rejected targets")
	allObjects := fs.Bool("all", false, "generate every object after reporting the exact provider-call count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	if *allObjects && common.object != "" {
		return errors.New("--all cannot be combined with --object")
	}
	if !*allObjects && common.object == "" && !*fake {
		return errors.New("real-provider generation requires --object <id>")
	}
	if err := rejectAnimatedPartialSelector(all, common.filter(), "generate"); err != nil {
		return err
	}
	if *force && selectsAnimated(all, common.filter()) {
		return errors.New("animated generation is complete-unit only in V9; rejected runs require a new --run auto run")
	}
	if *allObjects && selectsAnimated(all, common.filter()) {
		return errors.New("animated generation is complete-unit only in V9; select exactly one --object")
	}
	env, err := generateEnvironment(common.packDir)
	if err != nil {
		return err
	}
	gen, err := provider.Select(provider.SelectionOptions{ExplicitName: *providerName, Fake: *fake, Env: env})
	if err != nil {
		return err
	}
	all = resolveReferencePaths(all, common.packDir)
	if *allObjects {
		fmt.Printf("provider_calls_planned: %d\n", plannedProviderCalls(all, common.filter()))
	}
	out := pack.OutputDir(p)
	if err := output.Validate(filepath.Join(common.packDir, out)); err != nil {
		return err
	}
	deployDir := p.DeployDir
	if deployDir != "" && !filepath.IsAbs(deployDir) {
		deployDir = filepath.Join(common.packDir, deployDir)
	}
	result, err := generate.Run(ctx, all, gen, generate.Options{
		OutputDir: filepath.Join(common.packDir, out),
		DeployDir: deployDir,
		RunID:     common.runID,
		Filter:    common.filter(),
		Force:     *force,
		Progress: func(event generate.ProgressEvent) {
			fmt.Println(formatGenerationProgress(event))
		},
	})
	if err != nil {
		return err
	}
	fmt.Printf("run_id: %s\ngenerated: %d\nskipped: %d\nawaiting_review: %d\n", result.RunID, result.Generated, result.Skipped, result.AwaitingReview)
	manifest, err := generate.Load(filepath.Join(common.packDir, out), result.RunID)
	if err != nil {
		return err
	}
	return statusreport.Print(os.Stdout, manifest, all, common.filter())
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
		variants := append([]targets.VariantSelection(nil), out[i].Variants...)
		for variantIndex := range variants {
			if variants[variantIndex].ReferencePath != "" && !filepath.IsAbs(variants[variantIndex].ReferencePath) {
				variants[variantIndex].ReferencePath = filepath.Join(packDir, variants[variantIndex].ReferencePath)
			}
		}
		out[i].Variants = variants
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
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	if err := rejectAnimatedPartialSelector(all, common.filter(), "status"); err != nil {
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
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	if err := rejectAnimatedPartialSelector(all, common.filter(), "review"); err != nil {
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
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	if err := rejectAnimatedPartialSelector(all, common.filter(), "deploy"); err != nil {
		return err
	}
	deployDir := p.DeployDir
	if deployDir == "" {
		return errors.New("deployDir is required")
	}
	if !filepath.IsAbs(deployDir) {
		deployDir = filepath.Join(common.packDir, deployDir)
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
	p, _, all, err := loadTargetsFromCommon(common)
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
	p, _, _, err := loadTargetsFromCommon(common)
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
	packDir   string
	runID     string
	object    string
	animation string
	frame     string
	variants  multiFlag
}

func commonFlags(name string) (*flag.FlagSet, *commonOptions) {
	common := &commonOptions{runID: "auto"}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&common.packDir, "pack", ".", "sprite pack directory")
	fs.StringVar(&common.runID, "run", "auto", "run id")
	fs.StringVar(&common.object, "object", "", "object id filter")
	fs.StringVar(&common.animation, "animation", "", "animation id filter")
	fs.StringVar(&common.frame, "frame", "", "frame id filter")
	fs.Var(&common.variants, "variant", "variant filter as axis=value")
	return fs, common
}

func rejectRemovedFlags(args []string) error {
	removed := map[string]string{
		"--output":        "paths come from sprites.json",
		"--deploy-dir":    "paths come from sprites.json",
		"--allow-partial": "pending targets are ignored and animated units are reviewed atomically",
		"--complete":      "deployment always selects complete accepted groups",
		"--candidate":     "each attempt has exactly one candidate and candidate selection is automatic",
		"--stage":         "V9 reviews the complete unit without an intermediate stage",
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

func rejectAnimatedPartialSelector(all []targets.Target, filter targets.Filter, command string) error {
	if !selectsAnimated(all, filter) {
		return nil
	}
	if filter.Animation != "" || filter.Frame != "" || len(filter.Variants) != 0 {
		return fmt.Errorf("animated %s is complete-unit only in V9; select only --object", command)
	}
	return nil
}

func selectsAnimated(all []targets.Target, filter targets.Filter) bool {
	for _, target := range all {
		if target.AnimationID != "" && (filter.Object == "" || target.ObjectID == filter.Object) {
			return true
		}
	}
	return false
}

func plannedProviderCalls(all []targets.Target, filter targets.Filter) int {
	statics := map[string]bool{}
	animations := map[string]map[string]bool{}
	for _, target := range targets.FilterTargets(all, filter) {
		if target.AnimationID == "" {
			statics[target.ID] = true
			continue
		}
		if animations[target.ObjectID] == nil {
			animations[target.ObjectID] = map[string]bool{}
		}
		animations[target.ObjectID][target.AnimationID] = true
	}
	total := len(statics)
	for _, objectAnimations := range animations {
		total += 1 + len(objectAnimations)
	}
	return total
}

func requireConcreteRunID(runID string) error {
	if runID == "" || runID == "auto" {
		return errors.New("--run requires an existing run id")
	}
	return nil
}

func (c *commonOptions) filter() targets.Filter {
	variants := map[string]string{}
	for _, item := range c.variants {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			variants[parts[0]] = parts[1]
		}
	}
	return targets.Filter{Object: c.object, Animation: c.animation, Frame: c.frame, Variants: variants}
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("variant %q must use axis=value", value)
	}
	axis, normalized := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[0])+"="+strings.TrimSpace(parts[1])
	for _, existing := range *m {
		if strings.HasPrefix(existing, axis+"=") {
			return fmt.Errorf("variant axis %q was specified more than once", axis)
		}
	}
	*m = append(*m, normalized)
	return nil
}

func loadTargetsFromCommon(common *commonOptions) (*pack.Pack, string, []targets.Target, error) {
	p, theme, err := pack.Load(common.packDir)
	if err != nil {
		return nil, "", nil, err
	}
	all, err := targets.Expand(p, theme)
	if err != nil {
		return nil, "", nil, err
	}
	return p, theme, all, nil
}

const starterSpritesJSON = `{
  "version": 3,
  "outputDir": "output",
  "deployDir": "deploy",
  "objects": [
    {
      "id": "blood-duelist",
      "description": "Elegant demonic duelist with red coat, horned silhouette, and thin rapier.",
      "identityLocks": [
        "The horned silhouette, red coat, and thin rapier remain identical in every direction and frame.",
        "The rapier remains in the same hand."
      ],
      "size": { "width": 160, "height": 160 },
      "variants": [
        {
          "id": "direction",
          "description": "Battlefield facing direction.",
          "values": [
            { "id": "right", "description": "Side view facing right.", "reference": { "path": "references/current-right.png", "description": "Current right-facing identity reference." } }
          ]
        }
      ],
      "animations": [
        {
          "id": "attack",
          "description": "Rapier attack animation with readable body motion.",
          "frames": [
            { "description": "Ready stance." },
            { "description": "Windup." },
            { "id": "contact", "description": "Forward thrust contact frame." },
            { "description": "Recovery." }
          ]
        }
      ],
      "deploy": {
        "pathTemplate": "units/{object}__{animation}__{variant.direction}__{frame}.png"
      }
    }
  ]
}
`
