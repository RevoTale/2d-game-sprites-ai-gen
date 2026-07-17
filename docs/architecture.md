# Architecture

This document describes package responsibilities for maintainers.

## Package Map

- `cmd/sprites-ai-gen`: CLI wiring only.
- `internal/pack`: parses and validates user-authored pack files.
- `internal/targets`: expands objects, variants, animations, and frames into deterministic target IDs.
- `internal/conditioning`: defines typed style, identity, pose, and mask inputs shared across expansion and providers.
- `internal/generate`: owns manifest-V5 runs, deterministic layout/row planning, production pose discovery, approval
  gates, force/resume semantics, candidate selection, fixed-cell extraction, and artifact writes.
- `internal/review`: records accept/reject QA decisions.
- `internal/status`: renders scoped manifest state, review artifacts, and executable next actions without broadening
  selectors.
- `internal/deploy`: validates, stages, and atomically copies accepted static targets or complete animated rows; dry-run
  uses the same validation path.
- `internal/imageio`: handles deterministic pixel normalization, palettes, masks, candidate metrics, copying, and sheets.
- `internal/output`: enforces the generator-owned run layout while preserving the opaque migrated legacy tree.
- `internal/envfile`: reads simple dotenv files for CLI configuration without shell expansion.
- `internal/provider`: isolates AI provider implementations behind one interface.
- `internal/testkit`: provides schema-level fixtures for tests.

## Boundaries

`pack` owns schema validation. Other packages should receive an already validated pack.

`targets` owns expansion and prompt construction. Generation code should not duplicate variant or frame expansion rules.

`generate` owns run state and writes target artifacts. It derives combined directional seed boards and complete
animation rows from array-order metadata and resolved production poses without adding fields to the pack schema. A row
becomes a target source only after fixed-layout validation passes. Review and deploy update manifest records instead of
parallel metadata.

`review` owns visual QA decisions. Provider generation does not imply acceptance.

`deploy` owns previewing and copying accepted target images. Animated rows are its atomic unit. It must not infer,
repair, crop, or select images.

`envfile` owns dotenv parsing. It must not execute shell syntax or override process environment values.

`provider` owns external AI API calls, typed capability reporting, and provider selection. OpenAI API mechanics stay
here. The fake provider is deterministic and available only through `--fake`.
