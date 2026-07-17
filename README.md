# 2D Game Sprites AI Generator

Schema-driven CLI for generating reusable 2D sprite assets with AI.

Use this when you want static sprites or animated frames generated from a project-local `THEME.md` and `sprites.json`.
Static targets are generated independently. Animated objects use an approved combined directional seed board, then one
complete generated image per animation/variant row. Frames are extracted only at fixed coordinates after the complete
row passes strict mechanical validation.

## Quick Start

```bash
task validate
task test

sprites-ai-gen init
sprites-ai-gen validate --pack .
sprites-ai-gen generate --pack . --run auto --object blood-duelist
sprites-ai-gen review --pack . --run 2026-07-03-m0847 --object blood-duelist --candidate 01 --status accepted
sprites-ai-gen generate --pack . --run 2026-07-03-m0847 --object blood-duelist
sprites-ai-gen status --pack . --run 2026-07-03-m0847
sprites-ai-gen review --pack . --run 2026-07-03-m0847 --object blood-duelist --status accepted
sprites-ai-gen deploy --pack . --run 2026-07-03-m0847 --dry-run
sprites-ai-gen deploy --pack . --run 2026-07-03-m0847
sprites-ai-gen git-guard --pack .
```

## Command Flow

### Prepare a Pack

```bash
sprites-ai-gen init
sprites-ai-gen validate --pack .
```

`init` creates starter `THEME.md`, `sprites.json`, `.env.example`, and ignore rules. `validate` checks the pack schema, references, target expansion, deploy paths, and managed output structure before any image generation starts.

### Generate Drafts

```bash
sprites-ai-gen generate --pack . --run auto
sprites-ai-gen status --pack . --run 2026-07-03-m0847
```

`generate` creates or resumes a manifest-V5 run. The first animated invocation creates three combined directional
seed-board candidates and stops for seed review. After one seed candidate is accepted, rerunning the command generates
one complete-row candidate per attempt, validates the whole selected row, and extracts normalized frames at fixed cell
coordinates. Static targets use the same one-candidate-per-attempt flow.

Configure OpenAI in the process environment or pack `.env`:

```text
SPRITES_AI_GEN_PROVIDER=openai
OPENAI_API_KEY=...
```

OpenAI is the supported real provider. The default model is `gpt-image-2` and can be overridden through environment
configuration. Animated results use ordered references and masks but still require explicit visual QA before deployment.
Use `--fake` only for deterministic plumbing checks and tests.

Provider canvases are reduced to the configured target size with deterministic area sampling, a maximum 32-color
locked palette, hard alpha, and no dithering. Automated checks reject malformed geometry; visual QA still decides
identity, anatomy, facing, equipment side, projectile absence, and game-scale readability.

### Review the Images

```bash
sprites-ai-gen review --pack . --run 2026-07-03-m0847 --object blood-duelist --candidate 01 --status accepted
sprites-ai-gen review --pack . --run 2026-07-03-m0847 --object blood-duelist --animation attack --variant direction=right --status accepted
```

Every successful generation attempt writes raw evidence, a normalized board, extracted frames, metrics, a
nearest-neighbor contact sheet, a looping GIF, prompts, references, hashes, lineage, and QA notes. Seed review uses
`--candidate` is seed-only; row review applies to the complete selected row. Rejected scopes require `--reason`.

### Preview and Deploy

```bash
sprites-ai-gen deploy --pack . --run 2026-07-03-m0847 --dry-run
sprites-ai-gen deploy --pack . --run 2026-07-03-m0847
```

`deploy --dry-run` executes the same validation and shows which files would be replaced, skipped, or blocked. Static
targets deploy independently. Animated action-direction rows deploy only when every frame is accepted,
production-eligible, shares current seed/row lineage, and production has not changed since generation. Files are staged
before replacement and the invocation rolls back if replacement or manifest persistence fails.

### Clean Local Attempts

```bash
sprites-ai-gen git-guard --pack .
sprites-ai-gen prune --pack . --run 2026-07-03-m0847 --only-raw
```

`git-guard` fails if generated PNG artifacts under the pack output directory are tracked or staged. `prune --only-raw`
removes provider raw images after prompt, QA, metrics, normalized candidates, selected output, and lineage are preserved.

## Scoped Generation

Generate one object:

```bash
sprites-ai-gen generate --pack . --run auto --object blood-duelist
```

Generate one animation direction:

```bash
sprites-ai-gen generate --pack . --run auto \
  --object blood-duelist \
  --animation attack \
  --variant direction=right
```

Generate one exact frame:

```bash
sprites-ai-gen generate --pack . --run auto \
  --object blood-duelist \
  --animation attack \
  --variant direction=right \
  --frame contact
```

Accepted and deployed targets are skipped. Rejected scopes require `--force` for a new paid attempt; prior evidence is retained.

A scoped request first creates the object-wide directional seed board when needed. Selecting one frame requires a
complete current production row, exposes only that cell through a mask, and resets review/lineage for the complete row.
Forcing a row creates a fresh complete row while reusing its accepted seed. Object-wide force regenerates the seed board
and invalidates dependent rows. Earlier attempts remain recorded.

## Minimal Pack

```json
{
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
            { "id": "contact", "description": "Forward thrust contact frame." }
          ]
        }
      ],
      "deploy": {
        "pathTemplate": "units/{object}__{animation}__{variant.direction}__{frame}.png"
      }
    }
  ]
}
```

## Documentation

- [Workflow](docs/workflow.md): run lifecycle, output layout, statuses, review, deploy, and pruning.
- [Sprite Pack Schema](docs/sprites-json.md): `sprites.json` fields, variants, frames, references, and deploy templates.
- [Animated Consistency](docs/consistency.md): seed approval, complete-row validation, extraction, and candidate QA.
- [Architecture](docs/architecture.md): package responsibilities and maintainer-facing boundaries.
