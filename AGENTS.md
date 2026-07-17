# Overview
2D Game Sprites AI Generator is a reusable Go CLI for schema-driven AI sprite drafting.

# Project Structure
```text
/
  AGENTS.md
  README.md
  Taskfile.yml
  go.mod
  cmd/
    sprites-ai-gen/
  internal/
    deploy/
    conditioning/
    generate/
    imageio/
    output/
    pack/
    provider/
    review/
    targets/
    testkit/
```

# Strict Rules
- MUST use Taskfile as the repository workflow entrypoint.
- MUST keep CLI entrypoint packages under `cmd/` as wiring only.
- MUST keep reusable logic in focused `internal/` packages named by what they provide.
- MUST prefer the Go standard library; add dependencies only when they clearly reduce meaningful complexity.
- MUST keep generated outputs ignored by default.
- MUST allow animated frame extraction only from a mechanically validated complete row using fixed configured cell
  coordinates. Any occupied gutter, guard band, trailing cell, or cross-cell contamination rejects the whole row.
- MUST never infer cell boundaries, crop harder, clear edge pixels, or repair malformed anatomy after generation.
- MUST treat review/contact sheets as artifacts assembled from validated rows and extracted target images.
- MUST make `deploy` copy only accepted target images.
- MUST make `deploy --dry-run` use the production deployment validation path and show exactly what would be replaced,
  skipped, or blocked without writing.
- MUST keep typed style, identity, pose, and mask inputs distinct through target expansion, provider requests, and
  manifest evidence. Providers must fail before billing when they cannot honor a required role.
- MUST generate one combined directional seed board per animated object from existing/configured first-frame poses in
  schema order. Human seed approval is required before any dependent animation row is generated.
- MUST generate each animation/variant sequence as one complete row. The approved directional seed is identity
  guidance; the deterministic production pose board defines frame order, baseline, and motion.
- MUST stop broad pack style and object references after seed-board generation. Animation rows receive the editable row,
  approved directional seed, ordered production pose board, and one board mask.
- MUST never chain adjacent generated rows or frames. Invalid seed boards and rows block extraction instead of silently
  falling back to weaker conditioning.
- MUST keep `sprites.json` provider-neutral. Directional anchors, production-pose discovery, masks, candidate ranking,
  and lineage planning are internal generator behavior and must not require provider fields in the pack schema.
- MUST generate three candidates for a directional seed board and one candidate per complete animated-row/static attempt.
  A board must pass all mechanical hard checks before fixed-cell extraction; anatomy, equipment-side, projectile,
  cadence, motion, and gameplay-scale quality remain mandatory visual QA.
- MUST distinguish extraction safety from visual pose similarity. Occupied margins, gutters, trailing cells, cell guard
  bands, missing subjects, clipping, cross-cell foreground, and a second primary subject at least 60% of the largest are
  blocking. Legacy silhouette, scale, centering, baseline, palette, and cadence differences are review evidence only.
- MUST use deterministic area reduction, a locked maximum-32-color palette, linear-sRGB matching, hard alpha, and no
  dithering for pixel normalization. Catmull-Rom and free-color interpolation are forbidden for sprite candidates.
- MUST reject malformed candidates instead of cropping anatomy, clearing guard bands, or deleting foreground pieces.
- MUST make animated deployment action-and-variant-row atomic. Every frame in the row must be accepted,
  production-eligible, share the currently accepted seed/row lineages, and retain its generation-start production hash
  before any row file is replaced.
- MUST keep the fake provider behind the explicit `--fake` CLI flag only. Do not allow fake generation through
  `--provider`, `SPRITES_AI_GEN_PROVIDER`, or provider auto-detection.
- MUST support OpenAI as the only real provider unless the user explicitly approves another integration. Select it from
  `--provider openai`, `SPRITES_AI_GEN_PROVIDER=openai`, or `OPENAI_API_KEY`. Validated manifest-V5 row output is
  production-eligible only after whole-row visual QA and final-target acceptance.
- MUST use a fixed 1024x1024 provider canvas with 32px margins, 16px true gutters, centered fixed-layout boards, and an
  inner guard of `max(8px, cellWidth/32)`. Seed boards are 2x2 with one unused trailing cell; four-frame rows are a
  centered horizontal grid.
- MUST support only manifest V5 for run commands and save it atomically using temporary-file write, sync, and rename.
  Older run directories stay untouched and return a clear instruction to start a new run.
- MUST write review artifacts automatically after successful generation. `sheet` and `deploy-plan` are not CLI commands.
- MUST keep API credentials, full prompts, image bytes, and signed output URLs out of progress and error logs.
- MUST make tests describe the exact behavior under test in their names and assertions.
- MUST document package responsibility and invariants in package comments or nearby README files.

# Working Agreements
- Keep workflow docs short and command-oriented.
- Explain schema tradeoffs beside the schema, not only in chat.
