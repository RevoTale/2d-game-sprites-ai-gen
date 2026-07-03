# Architecture

This document describes package responsibilities for maintainers.

## Package Map

- `cmd/sprites-ai-gen`: CLI wiring only.
- `internal/pack`: parses and validates user-authored pack files.
- `internal/targets`: expands objects, variants, animations, and frames into deterministic target IDs.
- `internal/generate`: owns run manifests, resumable generation, and artifact writes.
- `internal/review`: records accept/reject QA decisions.
- `internal/deploy`: previews and copies accepted targets.
- `internal/imageio`: handles PNG validation, copying, and sheet assembly.
- `internal/provider`: isolates AI provider implementations behind one interface.
- `internal/testkit`: provides schema-level fixtures for tests.

## Boundaries

`pack` owns schema validation. Other packages should receive an already validated pack.

`targets` owns expansion and prompt construction. Generation code should not duplicate variant or frame expansion rules.

`generate` owns run state and writes target artifacts. Review and deploy code should update run state through manifest records instead of inventing parallel metadata.

`review` owns visual QA decisions. Provider generation does not imply acceptance.

`deploy` owns previewing and copying accepted target images. It must not infer, repair, or crop images.

`provider` owns external AI API calls. The fake provider is the default test provider and must stay deterministic.
