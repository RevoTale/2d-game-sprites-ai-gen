# Sprite Pack Schema

A sprite pack contains `THEME.md` and `sprites.json`.

`THEME.md` is shared art direction. `sprites.json` defines what should be generated, how targets are expanded, and where accepted images deploy.

## Core Rules

- `type` is intentionally absent.
- Objects with `animations` produce animated frame targets.
- Objects without `animations` produce static targets.
- `variants` are generic axes such as `direction`, `skin`, `growth`, or `material`.
- Variant axes are recombined as a cross product.
- `frames[]` order is animation order.
- `frames[].description` is required.
- `frames[].id` is optional; missing IDs derive from array indexes as `00`, `01`, `02`, and so on.
- Target IDs and default deploy filenames use `__` as the delimiter.

## Fields

```json
{
  "version": 1,
  "outputDir": "output",
  "deployDir": "deploy",
  "references": [],
  "objects": []
}
```

- `version`: schema version. V1 requires `1`.
- `outputDir`: optional generated run directory. Defaults to `output`.
- `deployDir`: optional destination for accepted deployed images.
- `references`: optional broad reference images applied to all targets.
- `objects`: required sprite objects.

## Object Example

```json
{
  "id": "blood-duelist",
  "description": "Elegant demonic duelist with red coat, horned silhouette, and thin rapier.",
  "size": { "width": 160, "height": 160 },
  "references": [
    {
      "path": "references/blood-duelist-design.png",
      "description": "Character identity reference.",
      "required": true
    }
  ],
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
```

## Reference Images

References can be declared at broad or specific levels. The generator merges references from broader to more specific scopes.

Required references fail generation when the provider cannot use image inputs. This prevents the CLI from silently ignoring important visual guidance.

## Deploy Templates

Deploy templates are relative paths. Path traversal and absolute paths are rejected.

Supported placeholders:

- `{target}`: full expanded target ID, including variants, animation, and frame when present.
- `{object}`
- `{animation}`
- `{frame}`
- `{variant.<axis>}`

Default static template:

```text
sprites/{target}.png
```

Static targets use `{target}` by default so variant combinations deploy to distinct files.

Default animated unit-style template:

```text
units/{object}__{animation}__{variant.direction}__{frame}.png
```

Override `deploy.pathTemplate` when an animated object does not use a `direction` variant or needs a project-specific layout.
