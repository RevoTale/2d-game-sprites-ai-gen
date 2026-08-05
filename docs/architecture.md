# Architecture

- `internal/pack`: strict V6 decode, unknown-field rejection, validation, and
  safe paths.
- `internal/targets`: deterministic expansion, including atomic semantic
  static-set parts, and object/batch selection.
- `internal/provider`: OpenAI HTTP integration plus an injected test double.
- `internal/imageio`: semantic layouts/recovery, canonical profiles,
  normalization, palettes, and review artifacts.
- `internal/generate`: manifest V12, style guide, master/animation/static-set
  generation, density evidence, and resumability.
- `internal/review`: manual complete-unit or static-target decisions.
- `internal/deploy`: staging, production-hash checks, rollback, and atomic
  replacement.
- `cmd/sprites-ai-gen`: flags and wiring only.

Manifest V12 records logical and intrinsic size plus source density. Static-set
parts share one attempt, provider board, palette, lineage, review owner, and
deployment group. Static-set recovery measures every part before writing, then
uses one deterministic non-upscaling transform selected by the most constrained
safe production canvas. This preserves common material scale while allowing
transparent reserve; independent part fitting is forbidden. Board geometry,
exact `2x` canvases, normalization, and QA stay in code; JSON contains only
material, semantic role, logical size, and deploy facts.

Isolated statics retain their independent alpha-bounds fit. Animated units use
their separate canonical master/profile and direction-owned scale/anchor flow;
static-set transforms are not available to animation code, and action extent
continues to reject rather than shrink a unit.

JSON provides visual facts. Generator code provides immutable protocol. An
animation request receives one opaque chroma semantic layout containing the
current-run master at every logical anchor; it receives no mask or separate
identity/reference image. Semantic recovery owns structural extraction, while
humans own visual quality. Logical anchor spacing stays independent from the
provider canvas: animation boards include real outer reserve so edge poses are
not conditioned against a clipping boundary.
