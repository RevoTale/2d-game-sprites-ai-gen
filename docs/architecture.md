# Architecture

- `internal/pack`: strict V5 decode, unknown-field rejection, validation, and
  safe paths.
- `internal/targets`: deterministic expansion and object/batch selection.
- `internal/provider`: OpenAI HTTP integration plus an injected test double.
- `internal/imageio`: semantic layouts/recovery, canonical profiles,
  normalization, palettes, and review artifacts.
- `internal/generate`: manifest V11, style guide, master/animation/static
  generation, evidence, and resumability.
- `internal/review`: manual complete-unit or static-target decisions.
- `internal/deploy`: staging, production-hash checks, rollback, and atomic
  replacement.
- `cmd/sprites-ai-gen`: flags and wiring only.

JSON provides visual facts. Generator code provides immutable protocol. An
animation request receives one opaque chroma semantic layout containing the
current-run master at every logical anchor; it receives no mask or separate
identity/reference image. Semantic recovery owns structural extraction, while
humans own visual quality. Logical anchor spacing stays independent from the
provider canvas: animation boards include real outer reserve so edge poses are
not conditioned against a clipping boundary.
