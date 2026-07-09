# 2D Game Sprites AI Generator

Schema-driven CLI for generating reusable 2D sprite assets with AI.

Use this when you want static sprites or animated frames generated from a project-local `THEME.md` and `sprites.json`. Every target image is generated independently. Review sheets are assembled from generated targets; the CLI never crops AI-generated sheets into source frames.

## Quick Start

```bash
task validate
task test

sprites-ai-gen init
sprites-ai-gen validate --pack .
sprites-ai-gen generate --pack . --run auto --object blood-duelist
sprites-ai-gen status --pack . --run 2026-07-03-m0847
sprites-ai-gen sheet --pack . --run 2026-07-03-m0847 --object blood-duelist
sprites-ai-gen review --pack . --run 2026-07-03-m0847 --object blood-duelist --status accepted
sprites-ai-gen deploy-plan --pack . --run 2026-07-03-m0847
sprites-ai-gen deploy --pack . --run 2026-07-03-m0847
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

`generate` creates or resumes a run and fills only missing matched targets. Static sprites, generic variants, animations, and frames all expand from `sprites.json`. Providers may return larger raw canvases; the CLI aspect-fits each target into the exact configured sprite size. `status` shows what is still pending, generated, accepted, rejected, deployed, or missing local artifacts.

### Review the Images

```bash
sprites-ai-gen sheet --pack . --run 2026-07-03-m0847 --object blood-duelist
sprites-ai-gen review --pack . --run 2026-07-03-m0847 --object blood-duelist --status accepted
```

`sheet` assembles review sheets from normalized target images only. The CLI never treats a sheet as source frames. `review` records visual QA decisions; rejected targets require `--reason`, while accepted targets can be bulk-marked after review.

### Preview and Deploy

```bash
sprites-ai-gen deploy-plan --pack . --run 2026-07-03-m0847
sprites-ai-gen deploy --pack . --run 2026-07-03-m0847
```

`deploy-plan` shows exactly which accepted targets would replace files and which non-accepted targets would stay unchanged. `deploy` copies accepted images only. Partial deploy is the default; add `--complete` when every scoped target must be accepted before anything is copied.

### Clean Local Attempts

```bash
sprites-ai-gen prune --pack . --run 2026-07-03-m0847 --only-raw
```

`prune --only-raw` removes raw generated attempts after prompt, QA, and manifest metadata are preserved.

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

Accepted and deployed targets are protected from regeneration. Use `--force` when you intentionally want a new attempt while keeping the old attempt and review history.

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
- [Architecture](docs/architecture.md): package responsibilities and maintainer-facing boundaries.
