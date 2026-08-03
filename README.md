# 2D Game Sprites AI Generator

Reusable Go CLI for strict JSON-configured, OpenAI-backed sprite drafting.

## Contract

- `sprites.json` V5 owns style, objects, references, directions, animation
  semantics, dimensions, registration, and deployment paths.
- CLI code owns provider protocol, layouts, recovery, normalization, manifest
  V11, QA artifacts, review, and safe deployment.
- OpenAI is the only public provider; tests inject a private fake.
- One candidate is generated per stage.
- Animated units use one character master plus one board per animation, then
  review and deploy as a complete-unit.
- Static assets remain target-atomic.

## Commands

```bash
task validate
task test

go run ./cmd/sprites-ai-gen validate --pack <pack>
go run ./cmd/sprites-ai-gen generate --pack <pack> --run auto --style-guide
go run ./cmd/sprites-ai-gen generate --pack <pack> --run auto --object <id>
go run ./cmd/sprites-ai-gen generate --pack <pack> --run auto --all
go run ./cmd/sprites-ai-gen status --pack <pack> --run <run-id>
go run ./cmd/sprites-ai-gen review --pack <pack> --run <run-id> --object <id> --status accepted
go run ./cmd/sprites-ai-gen deploy --pack <pack> --run <run-id> --object <id> --dry-run
```

Applications should expose these through their own Docker/Taskfile wrappers.
Paid generation requires `OPENAI_API_KEY`; the default model is `gpt-image-2`
and `SPRITES_AI_GEN_OPENAI_MODEL` may override it.

See `docs/architecture.md`, `docs/sprites-json.md`, `docs/consistency.md`, and
`docs/workflow.md`.
