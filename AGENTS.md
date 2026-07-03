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
    generate/
    imageio/
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
- MUST never crop AI-generated sheets into frames; every target must be generated independently.
- MUST treat review/contact sheets as artifacts assembled from target images.
- MUST make `deploy` copy only accepted target images.
- MUST make `deploy-plan` and `deploy --dry-run` show exactly what would be replaced and what would remain unchanged.
- MUST fail required references when the provider cannot use image references.
- MUST make tests describe the exact behavior under test in their names and assertions.
- MUST document package responsibility and invariants in package comments or nearby README files.

# Working Agreements
- Keep workflow docs short and command-oriented.
- Explain schema tradeoffs beside the schema, not only in chat.
