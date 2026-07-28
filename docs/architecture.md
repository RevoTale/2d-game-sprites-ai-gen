# Architecture

- `internal/pack`: V3 parsing and reference validation.
- `internal/targets`: deterministic expansion and deployment paths.
- `internal/provider`: OpenAI and deterministic fake provider.
- `internal/imageio`: semantic layouts, foreground ownership, complete-pose
  recovery, unit-wide registration, structural checks, palette, review sheets,
  and GIFs.
- `internal/generate`: manifest V9, animated planning, master/animation provider
  preparation, prompts, resumable calls, lineage, static generation, and unit
  artifact assembly.
- `internal/review`: complete-unit or static-target decisions.
- `internal/deploy`: dry-run planning, stale-hash protection, staging, rollback,
  and unit/static atomic replacement.
- `internal/status`: stage, artifacts, blockers, provenance, and next commands.

Animated state has one `character-master`, one `animation-board` per configured
animation, one derived `unit`, and frame targets. Intermediates are `pending`,
`ready`, or `rejected`; only units and static targets are manually reviewed.
The character master and animation boards are non-deployable evidence.
Logical anchors preserve order without acting as clipping boundaries.
Foreground components must resolve into complete, separated, ordered poses.

`sprites.json` owns provider-neutral sprite facts: identity, proportions,
equipment, directions, animation/frame intent, references, sizes, and deploy
templates. Generator code owns provider canvas geometry, opaque chroma,
semantic anchors, ownership, attempts, validation, normalization, review state,
and deployment safety. Animation provider requests send one opaque semantic
board prefilled from the recovered master and no mask; the separate master
remains unsent lineage evidence.

Manifests use temporary-file write, file sync, rename, and directory sync.
Generation can resume after interruption without repeating recorded provider
calls. Deployment stages every unit frame before touching production and rolls
back replacements if manifest persistence fails.
