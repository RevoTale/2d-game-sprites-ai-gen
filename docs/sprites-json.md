# `sprites.json` V3

V3 is provider-neutral. It stores character appearance, proportions,
equipment, directions, animation/frame intent, source evidence, sizes, and
deployment templates. It does not store provider prompts, canvas, chroma,
semantic anchors, masks, pose recovery, model settings, sequencing,
normalization, review, threshold, candidate, attempt, lineage, or run state.

This is an ownership boundary, not just a schema convenience. Sprite
descriptions may provide visual facts and motion semantics, but they must not
duplicate or override generator protocol. Generator code owns board geometry,
semantic anchors, provider-input preparation, pose ownership, validation,
normalization, and lifecycle.
Keep this file strict JSON; do not add comments or migrate it to JSONC.

Every animated object has exactly one `direction` variant. Each direction value
has one required reference:

```json
{
  "version": 3,
  "objects": [{
    "id": "relic-knight",
    "description": "Heavy blue-and-gold knight.",
    "identityLocks": ["The swept gold helmet crest remains unchanged."],
    "size": {"width": 160, "height": 160},
    "variants": [{
      "id": "direction",
      "description": "Authored battlefield direction.",
      "values": [{
        "id": "right",
        "description": "Right-facing side view.",
        "reference": {
          "path": "../source/frames/units/relic-knight/walk/right/00.png",
          "description": "Current right-facing identity reference."
        }
      }]
    }],
    "animations": [{
      "id": "walk",
      "description": "Grounded walk.",
      "frames": [{"id": "00", "description": "Neutral step."}]
    }]
  }]
}
```

Direction references are pack-relative PNGs, must exist, and must match the
object frame size. Their evidence IDs are derived from object and direction.
Additional animated variant axes are unsupported. Static objects and generic
style/identity references retain their existing shape.

V1 and V2 packs fail with a V3 migration message.
