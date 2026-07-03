# 2D Game Sprites AI Generator

Schema-driven CLI for generating reusable 2D sprite assets with AI.

The generator reads:

- `THEME.md` for shared style direction.
- `sprites.json` for objects, variants, animations, frames, references, output, and deploy paths.

Each expanded target is generated as its own image. The CLI never crops AI-generated sheets into frames.

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

## Review Flow

Generated images are drafts. Use `review` to accept or reject them before deploy.

Rejected targets require a reason so later regeneration knows what to fix. Accepted targets can be accepted in bulk after visual review.

## Sprite Pack Shape

`type` is intentionally absent. Objects with `animations` produce animated frame targets; objects without animations produce static targets. `variants` are generic axes, so `direction`, `skin`, `growth`, and similar dimensions all use the same schema.

```json
{
  "version": 1,
  "outputDir": "output",
  "deployDir": "../game/assets/source",
  "references": [
    {
      "path": "references/style-anchor.png",
      "description": "Global pixel-art style anchor.",
      "required": true
    }
  ],
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
            { "id": "down", "description": "Facing toward camera." },
            { "id": "up", "description": "Facing away from camera." },
            { "id": "right", "description": "Side view facing right." }
          ]
        }
      ],
      "animations": [
        {
          "id": "attack",
          "description": "Rapier attack animation with readable body motion and no projectile pixels.",
          "frames": [
            { "description": "Ready stance." },
            { "description": "Windup, torso rotates back." },
            { "id": "contact", "description": "Forward thrust contact frame." },
            { "description": "Recovery back to guard." }
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
