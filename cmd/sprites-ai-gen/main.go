package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/deploy"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/generate"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/review"
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
	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "validate":
		fs, common := commonFlags("validate")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		_, _, _, err := loadTargetsFromCommon(common)
		if err != nil {
			return err
		}
		fmt.Println("sprite pack is valid")
		return nil
	case "generate":
		return runGenerate(ctx, args[1:])
	case "status":
		return runStatus(args[1:])
	case "review":
		return runReview(args[1:])
	case "sheet":
		return runSheet(args[1:])
	case "deploy-plan":
		return runDeploy(args[1:], true)
	case "deploy":
		return runDeploy(args[1:], false)
	case "prune":
		return runPrune(args[1:])
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
		".env.example": "OPENAI_API_KEY=\n",
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
	return nil
}

func runGenerate(ctx context.Context, args []string) error {
	fs, common := commonFlags("generate")
	providerName := fs.String("provider", "fake", "provider: fake or openai")
	force := fs.Bool("force", false, "regenerate accepted/deployed targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	gen := provider.Provider(provider.Fake{ReferenceSupport: true})
	if *providerName == "openai" {
		gen = provider.OpenAI{}
	}
	out := generate.OutputDir(p, common.outputOverride)
	result, err := generate.Run(ctx, all, gen, generate.Options{OutputDir: filepath.Join(common.packDir, out), RunID: common.runID, Filter: common.filter(), Force: *force})
	if err != nil {
		return err
	}
	fmt.Printf("run: %s\ngenerated: %d\nskipped: %d\n", result.RunID, result.Generated, result.Skipped)
	return nil
}

func runStatus(args []string) error {
	fs, common := commonFlags("status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, _, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	manifest, err := generate.Load(filepath.Join(common.packDir, generate.OutputDir(p, common.outputOverride)), common.runID)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, state := range manifest.Targets {
		counts[state.Status]++
	}
	for _, status := range []string{generate.StatusPending, generate.StatusGenerated, generate.StatusAccepted, generate.StatusRejected, generate.StatusDeployed} {
		fmt.Printf("%s: %d\n", status, counts[status])
	}
	return nil
}

func runReview(args []string) error {
	fs, common := commonFlags("review")
	status := fs.String("status", "", "accepted or rejected")
	reason := fs.String("reason", "", "review reason")
	allowPartial := fs.Bool("allow-partial", false, "allow pending matched targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	result, err := review.Apply(all, review.Options{OutputDir: filepath.Join(common.packDir, generate.OutputDir(p, common.outputOverride)), RunID: common.runID, Filter: common.filter(), Status: *status, Reason: *reason, AllowPartial: *allowPartial})
	fmt.Printf("reviewed: %d\nskipped_pending: %d\n", result.Reviewed, result.SkippedPending)
	return err
}

func runSheet(args []string) error {
	fs, common := commonFlags("sheet")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	outputDir := filepath.Join(common.packDir, generate.OutputDir(p, common.outputOverride))
	manifest, err := generate.Load(outputDir, common.runID)
	if err != nil {
		return err
	}
	selected := targets.FilterTargets(all, common.filter())
	var paths []string
	for _, target := range selected {
		state := manifest.Targets[target.ID]
		if state != nil && state.NormalizedPath != "" {
			paths = append(paths, state.NormalizedPath)
		}
	}
	name := "pack"
	if common.object != "" {
		name = common.object
	}
	out := filepath.Join(outputDir, "runs", common.runID, "contact-sheets", name+".png")
	if err := imageio.AssembleHorizontalSheet(paths, out); err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func runDeploy(args []string, planOnly bool) error {
	fs, common := commonFlags("deploy")
	dryRun := fs.Bool("dry-run", false, "show deploy plan without copying")
	complete := fs.Bool("complete", false, "require all selected targets accepted")
	deployDirOverride := fs.String("deploy-dir", "", "deploy directory override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	deployDir := p.DeployDir
	if *deployDirOverride != "" {
		deployDir = *deployDirOverride
	}
	if deployDir == "" {
		return errors.New("deployDir is required")
	}
	if !filepath.IsAbs(deployDir) {
		deployDir = filepath.Join(common.packDir, deployDir)
	}
	opts := deploy.Options{OutputDir: filepath.Join(common.packDir, generate.OutputDir(p, common.outputOverride)), RunID: common.runID, DeployDir: deployDir, Filter: common.filter(), Complete: *complete}
	if planOnly || *dryRun {
		plan, err := deploy.BuildPlan(all, opts)
		fmt.Print(deploy.FormatPlan(plan))
		return err
	}
	plan, err := deploy.Execute(all, opts)
	fmt.Print(deploy.FormatPlan(plan))
	return err
}

func runPrune(args []string) error {
	fs, common := commonFlags("prune")
	onlyRaw := fs.Bool("only-raw", false, "remove raw attempts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, all, err := loadTargetsFromCommon(common)
	if err != nil {
		return err
	}
	if !*onlyRaw {
		return errors.New("only --only-raw pruning is supported in v1")
	}
	outputDir := filepath.Join(common.packDir, generate.OutputDir(p, common.outputOverride))
	selected := targets.FilterTargets(all, common.filter())
	for _, target := range selected {
		if err := os.RemoveAll(filepath.Join(generate.TargetDir(outputDir, common.runID, target.ID), "attempts")); err != nil {
			return err
		}
	}
	return nil
}

type commonOptions struct {
	packDir        string
	runID          string
	outputOverride string
	object         string
	animation      string
	frame          string
	variants       multiFlag
}

func commonFlags(name string) (*flag.FlagSet, *commonOptions) {
	common := &commonOptions{runID: "auto"}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&common.packDir, "pack", ".", "sprite pack directory")
	fs.StringVar(&common.runID, "run", "auto", "run id")
	fs.StringVar(&common.outputOverride, "output", "", "output directory override")
	fs.StringVar(&common.object, "object", "", "object id filter")
	fs.StringVar(&common.animation, "animation", "", "animation id filter")
	fs.StringVar(&common.frame, "frame", "", "frame id filter")
	fs.Var(&common.variants, "variant", "variant filter as axis=value")
	return fs, common
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
	if !strings.Contains(value, "=") {
		return fmt.Errorf("variant %q must use axis=value", value)
	}
	*m = append(*m, value)
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
  "version": 1,
  "outputDir": "output",
  "deployDir": "deploy",
  "objects": [
    {
      "id": "blood-duelist",
      "description": "Elegant demonic duelist with red coat, horned silhouette, and thin rapier.",
      "size": { "width": 160, "height": 160 },
      "variants": [
        {
          "id": "direction",
          "description": "Battlefield facing direction.",
          "values": [
            { "id": "right", "description": "Side view facing right." }
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
