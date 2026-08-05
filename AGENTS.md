# Generator policy

- Use Taskfile. Keep `cmd/` as wiring and reusable behavior in focused
  `internal/` packages.
- Prefer the Go standard library and keep generated output ignored.
- Support strict `sprites.json` V6 and manifest V12 only. V6 requires every
  object to declare bounded supernatural causes explicitly; an empty list means
  the object is mundane.
- Treat JSON visual facts and immutable CLI protocol as separate authorities.
  Do not load theme files or per-asset descriptors.
- OpenAI is the only public real provider. Keep the fake provider private to
  automated tests. Never log credentials, signed URLs, image bytes, or full
  secrets.
- Generate exactly one candidate per stage. Animated units use one
  multi-direction master and one semantic board per animation.
- Animation inputs contain only the master-derived colored layout. Never send
  production animation frames, unrelated evidence, a separate master image, or
  masks.
- Recover complete poses structurally. Reject missing, merged, ambiguous,
  order-reversed, overlapping, or canvas-clipped subjects; never erase, crop,
  synthesize, shrink, or independently recenter foreground.
- Apply deterministic area reduction, master-locked maximum-32-color palette,
  linear-sRGB matching, binary alpha, and no dithering.
- Keep one canonical body scale and one anchor per direction. Action extent
  never changes scale.
- Save manifests atomically with file and directory sync.
- Review/deploy animated units as complete-unit atomic groups and statics as
  target-atomic groups. Protect generation-start production hashes and roll
  back failed deployment.
- Write prompts, evidence, raw/recovered/normalized output, metrics, sheets,
  GIFs, previews, lineage, and QA artifacts automatically.
- Tests use only the injected fake provider.
