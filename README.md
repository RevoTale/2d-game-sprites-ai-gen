# 2D Game Sprites AI Generator

Schema-driven CLI for generating reusable 2D sprite assets with AI.

The generator reads `THEME.md` for shared style direction and `sprites.json` for objects, variants, animations, frames, references, output, and deploy paths. Each expanded target is generated as its own image; the CLI never crops AI-generated sheets into frames.

## Features

- Static sprite and animated frame generation from one schema.
- Generic `variants` for directions, skins, growth stages, materials, or any other axis.
- Variant cross-product expansion for full target coverage.
- Independent image generation for every expanded target.
- Reference image declarations at broad or specific levels.
- Strict validation for pack schema, references, target paths, and managed output.
- Resumable runs with generated, accepted, rejected, and deployed states.
- Review sheets assembled from normalized target images.
- Bulk accept/reject review workflow with required rejection reasons.
- Deploy plans that show exactly what will be replaced and what will stay unchanged.
- Partial deploy by default, with `--complete` for strict full-scope deploys.
- Optional pruning of raw generated attempts after review metadata is preserved.
- Deterministic fake provider for tests and OpenAI as the first real provider.

## Quick Start

```bash
task validate
task test

sprites-ai-gen init
sprites-ai-gen validate --pack .
sprites-ai-gen generate --pack . --run auto --object blood-duelist
sprites-ai-gen sheet --pack . --run 2026-07-03-m0847 --object blood-duelist
sprites-ai-gen review --pack . --run 2026-07-03-m0847 --object blood-duelist --status accepted
sprites-ai-gen deploy-plan --pack . --run 2026-07-03-m0847
sprites-ai-gen deploy --pack . --run 2026-07-03-m0847
```

## Basic Workflow

1. Write `THEME.md` and `sprites.json`, or start with `sprites-ai-gen init`.
2. Run `sprites-ai-gen validate --pack .`.
3. Generate one object, variant, animation, frame, or the whole pack.
4. Check run progress with `sprites-ai-gen status`.
5. Build review sheets with `sprites-ai-gen sheet`.
6. Mark generated targets with `sprites-ai-gen review`.
7. Preview changes with `sprites-ai-gen deploy-plan`.
8. Deploy accepted targets with `sprites-ai-gen deploy`.
9. Prune raw attempts with `sprites-ai-gen prune --only-raw` when they are no longer needed.

Generated images are drafts until reviewed. Rejected targets require a reason so later regeneration knows what to fix. Accepted targets can be accepted in bulk after visual review.

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
