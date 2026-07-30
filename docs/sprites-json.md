# `sprites.json` V4

V4 is provider-neutral. It stores character appearance, proportions,
equipment, directions, animation/frame intent, source evidence, sizes, and
one generic registration mode. It does not store provider prompts, canvas, chroma,
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
  "version": 4,
  "objects": [{
    "id": "relic-knight",
    "description": "Heavy blue-and-gold knight.",
    "identityLocks": ["The swept gold helmet crest remains unchanged."],
    "registration": {"mode": "grounded"},
    "size": {"width": 384, "height": 384},
    "variants": [{
      "id": "direction",
      "description": "Authored battlefield direction.",
      "values": [{
        "id": "right",
        "description": "Right-facing side view.",
        "reference": {
          "path": "../source/frames/units/relic-knight/walk/right/00.png",
          "description": "Current right-facing view-geometry and registration reference; colors are not authoritative."
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

Direction references are pack-relative PNGs and must exist. Normally they match
the object frame size. During an explicit canvas migration, the generator may
also accept the exact predecessor canvas when both axes are 64 pixels smaller;
it centers that reference with equal transparent padding without resampling its
foreground. No arbitrary smaller reference is valid. Canonical profiles record
the actual reference canvas so target-space anchors receive the same centering
translation. Their evidence IDs are derived from object and direction.
The generator sends them with the pose/view-geometry role; their descriptions
must not claim appearance or color authority. Additional animated variant axes
are unsupported. Static objects and generic style/identity references retain
their existing shape.

Registration modes are `grounded`, `centered`, and `canvas`. They do not carry
manual boxes, coordinates, action envelopes, scale numbers, or thresholds.
Animated objects require `grounded` or `centered`; opaque tiles use `canvas`.

V1 through V3 packs fail with a V4 migration message.
