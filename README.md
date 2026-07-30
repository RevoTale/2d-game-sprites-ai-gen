# 2D Game Sprites AI Generator

Provider-neutral sprite-pack CLI with an OpenAI image backend and deterministic
fake-provider tests.

## Animated V10 workflow

```text
direction references → canonical subject profile → character master
→ semantic animation boards → complete-pose recovery → canonical registration
→ complete-unit review → atomic deployment
```

```bash
sprites-ai-gen validate --pack .
sprites-ai-gen generate --pack . --run auto --object blood-duelist
sprites-ai-gen status --pack . --run <run-id> --object blood-duelist
sprites-ai-gen review --pack . --run <run-id> --object blood-duelist --status accepted
sprites-ai-gen deploy --pack . --run <run-id> --object blood-duelist --dry-run
sprites-ai-gen deploy --pack . --run <run-id> --object blood-duelist
```

An animated run makes one canonical character-master request and one request
per configured animation. Logical anchors preserve direction and frame order,
but they are not clipping boundaries. The generator recovers complete foreground
poses, applies one reference-derived unit transform and palette, then presents the complete
unit for review and atomic deployment.

Independent provider calls may render subjects at different source-coordinate
scales. Frame `00` calibrates each animation/direction to the matching master
view. One calibrated multiplier applies to the complete direction row, while
one shared output anchor and canonical visible scale remain unit-wide
invariants. Calibration evidence is written to `scale-calibration.json`.

Neutral direction references are the only scale authority. Weapons, wings,
tails, and other animation extents may use the complete native frame, but they
cannot shrink the unit. A pose that does not fit the canonical transform is
rejected.

Each animation request has exactly one colored input: an opaque flat-chroma
semantic board prefilled with the matching recovered master direction at each
anchor. It has no provider mask. No separate master or production reference is
uploaded for animation generation.

There is no seed review, row/frame generation, row/frame review, or partial
animated retry or deployment. A rejected animated run is immutable; start a
fresh `--run auto`. Static assets remain independently generated, reviewed, and
deployed.

Real animated generation requires exactly one `--object`. Every provider
request produces one candidate. Interrupted runs resume completed stages without
duplicating calls.

See [sprites.json](docs/sprites-json.md), [workflow](docs/workflow.md),
[consistency](docs/consistency.md), and [architecture](docs/architecture.md).
