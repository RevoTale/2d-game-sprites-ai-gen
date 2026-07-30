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
- MUST recover animated poses from mechanically validated semantic boards using ordered logical anchors, foreground
  ownership, and complete connected pose groups. Nominal anchor regions are not clipping boundaries.
- MUST reject missing, merged, overlapping, ambiguous, order-reversed, or real-canvas-clipped poses. Never crop harder,
  clear edge pixels, discard unowned foreground, or repair malformed anatomy after generation.
- MUST treat review/contact sheets as artifacts assembled from validated boards and extracted target images.
- MUST make `deploy` copy only accepted target images.
- MUST make `deploy --dry-run` use the production deployment validation path and show exactly what would be replaced,
  skipped, or blocked without writing.
- MUST keep typed style, identity, pose, and layout inputs distinct through target expansion, provider requests, and
  manifest evidence. Providers must fail before billing when they cannot honor a required role.
- MUST generate one canonical multi-direction character master, then one board per animation across all configured
  directions. The master is generated automatically and reviewed only as part of the complete unit.
- MUST use exactly one configured current-production reference per direction for master generation. Animation requests
  receive exactly one colored image: an opaque flat-chroma semantic board prefilled with the matching recovered
  canonical-master direction at every logical anchor. Do not send a provider mask. The master, style/object references, configured
  direction references, other production frames, and neighboring animation boards remain unsent review evidence.
- MUST never chain adjacent generated animations or frames. Invalid masters and animation boards block later calls and
  extraction instead of silently falling back to weaker conditioning.
- MUST keep `sprites.json` V4 provider-neutral. Registration mode is a visual geometry fact; provider canvas, masks,
  candidate handling, normalization, reviews, and
  lineage remain internal generator behavior.
- MUST generate exactly one candidate per master, animation-board, or static attempt.
  Mechanically or manually rejected animated runs are immutable and require a fresh run.
  A board must pass all mechanical hard checks before pose recovery; anatomy, equipment-side, projectile,
  cadence, motion, and gameplay-scale quality remain mandatory visual QA.
- MUST distinguish extraction safety from visual pose similarity. Missing cores, ambiguous component ownership, merged
  or overlapping groups, non-monotonic order, canvas-edge contact, and unsafe production-frame fit are blocking.
  Nominal anchor-midpoint crossing is allowed. Identity, silhouette, scale, centering, baseline, palette, and cadence
  differences are review evidence only.
- MUST use deterministic area reduction, a locked maximum-32-color palette, linear-sRGB matching, hard alpha, and no
  dithering for pixel normalization. Catmull-Rom and free-color interpolation are forbidden for sprite candidates.
- MUST build an animated unit's locked palette from its canonical character master only. Animation boards may add
  motion but must not add authoritative colors.
- MUST reject malformed candidates instead of cropping anatomy, clearing guard bands, or deleting foreground pieces.
- MUST make animated review and deployment complete-unit atomic. Every frame must be accepted, production-eligible,
  share the current master/animation lineages, and retain its generation-start production hash before any file is
  replaced.
- MUST preserve each configured animated target's native dimensions through
  normalization, review artifacts, and deployment. Do not apply a fixed review
  enlargement or silently accept a normalized PNG whose dimensions differ
  from the current target; this prevents older draft resolutions from entering
  a migrated pack.
- MUST keep the fake provider behind the explicit `--fake` CLI flag only. Do not allow fake generation through
  `--provider`, `SPRITES_AI_GEN_PROVIDER`, or provider auto-detection.
- MUST support OpenAI as the only real provider unless the user explicitly approves another integration. Select it from
  `--provider openai`, `SPRITES_AI_GEN_PROVIDER=openai`, or `OPENAI_API_KEY`. Validated manifest-V10 unit output is
  production-eligible only after complete-unit visual QA and acceptance.
- MUST request explicit `quality=high` for every OpenAI image generation and edit call. GPT Image provider settings
  remain generator-owned and must not enter `sprites.json`.
- MUST use deterministic semantic anchors without visible guides, panels, or masks. Current three-direction/four-frame
  boards use a 1536x1152 canvas and 384px anchor spacing; master canvases are at least 1024x1024.
- MUST derive one canonical subject profile from configured neutral direction references before animated generation.
  The profile owns one unit scale, the actual reference canvas, and one preferred automatically measured anchor per
  configured direction. When a pack explicitly accepts an exact smaller predecessor canvas, center it by the even
  canvas delta without foreground scaling and reject incomplete or changed reference lineage. After all
  poses are recovered, project that preferred anchor by the minimum distance into the shared feasible interval for the
  direction. Every frame of a direction reuses the resulting anchor. Animation extents must never reduce the scale or
  cause per-frame recentering.
- MUST register every animated frame with the profile-derived scale, its direction anchor, and the shared palette.
  Per-frame fitting is forbidden; a complete pose that cannot fit safely rejects the unit.
- MUST support only manifest V10 for run commands and save it atomically using temporary-file write, file sync, rename,
  and directory sync.
  Older run directories stay untouched and return a clear instruction to start a new run.
- MUST write review artifacts automatically after successful generation. `sheet` and `deploy-plan` are not CLI commands.
- MUST keep API credentials, full prompts, image bytes, and signed output URLs out of progress and error logs.
- MUST make tests describe the exact behavior under test in their names and assertions.
- MUST document package responsibility and invariants in package comments or nearby README files.

# Working Agreements
- Keep workflow docs short and command-oriented.
- Explain schema tradeoffs beside the schema, not only in chat.
