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

## Expected Flow

The CLI is built around a draft lifecycle:

1. `init` creates a minimal sprite pack.
2. `validate` proves `THEME.md`, `sprites.json`, references, target expansion, and path templates are valid.
3. `generate` creates or resumes a run and generates only matched pending targets. Each expanded target is generated as an independent image.
4. `sheet` assembles review sheets from normalized target images. Sheets are review artifacts only.
5. `review` records human or agent visual QA decisions. Accepted targets can be bulk reviewed; rejected targets require a reason.
6. `deploy-plan` previews what accepted targets would replace and what non-accepted targets would leave unchanged.
7. `deploy` copies only accepted target images. Partial deploy is the default; `--complete` requires every target in scope to be accepted.
8. `prune --only-raw` can remove raw attempt files after metadata is preserved.

## Run Output

Managed output lives under `output/runs/<run-id>/` by default:

```text
output/
  runs/
    2026-07-03-m0847/
      manifest.json
      targets/
        blood-duelist__attack__direction-right__contact/
          prompt.md
          qa.md
          normalized.png
          attempts/
            001/
              raw-candidate.png
      contact-sheets/
        blood-duelist.png
```

`raw-candidate.png` is provider evidence. `normalized.png` is the deployment candidate. Only accepted normalized target images are copied by `deploy`.

## Status Lifecycle

Targets move through explicit states:

```text
pending -> generated -> accepted -> deployed
                    \-> rejected -> generated
```

Accepted and deployed targets are protected from regeneration unless `--force` is used. When `--force` creates a new attempt, old attempt and review history remains in `manifest.json`.

## Package Map

- `internal/pack` parses and validates user-authored pack files.
- `internal/targets` expands objects, variants, animations, and frames into deterministic target IDs.
- `internal/generate` owns run manifests, resumable generation, and artifact writes.
- `internal/review` records accept/reject QA decisions.
- `internal/deploy` previews and copies accepted targets.
- `internal/imageio` handles PNG validation, copying, and sheet assembly.
- `internal/provider` isolates AI provider implementations behind one interface.

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
