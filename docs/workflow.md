# Workflow

## Lifecycle

1. `init` creates a minimal pack.
2. `validate` checks schema, references, deploy paths, expansion, and managed output structure.
3. The first animated `generate` creates or resumes a manifest-V5 run and produces three combined directional seed boards.
4. `status` reports eligible, invalid, and mechanically preferred seed candidates; the labeled candidate sheet remains
   subject to mandatory visual review.
5. `review --candidate <id> --status accepted` approves one mechanically eligible complete seed board.
6. Repeating `generate` produces one complete animation-row candidate with every frame assigned to an exact fixed
   column, validates the row, extracts frames, and writes review artifacts automatically.
7. Final `review` records accepted or rejected complete-row/static QA.
8. `deploy --dry-run` optionally reports rows or static targets that would be replaced, skipped, or blocked.
9. `deploy` stages accepted files and replaces them atomically by animated row or static target.
10. `git-guard` rejects tracked or staged draft PNGs; `prune --only-raw` removes raw provider evidence only.

OpenAI is selected through `SPRITES_AI_GEN_PROVIDER=openai` or `OPENAI_API_KEY`. The fake provider is available only
through `--fake` for deterministic tests.

## Run Output

```text
output/runs/<run-id>/
  manifest.json
  intermediates/<object>/direction-seeds/
    source-board.png
    normalized.png
    edit-source.png
    seeds/<variant>.png
    attempts/<attempt>/candidates/<candidate>/
  intermediates/<object>/animations/<animation>/<variant>/row/
    pose-board.png
    normalized.png
    edit-source.png
    attempts/<attempt>/candidates/<candidate>/
  targets/<target-id>/
    qa.md
    normalized.png
    palette.json
  contact-sheets/
  animations/
```

Manifest V5 records current seed/row attempts, candidate evidence, fixed layouts, reference and production hashes,
lineage, reviews, and deployment evidence. V1-V4 run directories remain untouched but commands require a new run.

## Review And Deploy

Directional seeds, complete rows, and static targets follow:

```text
pending -> awaiting_review -> accepted -> deployed
                           \-> rejected -> awaiting_review (with --force)
```

Rejected reviews require a reason. Acceptance without a reason records a default manual-review note. A frame repair
regenerates and resets the complete row. Static targets deploy independently. Animated targets deploy only as complete
accepted rows whose frames share current seed/row lineage and generation-start production hashes. Pending, rejected,
stale, and already-deployed groups remain unchanged.

Directional-seed acceptance requires a mechanically eligible candidate ID. When every seed candidate is invalid,
`status` reports an object-wide seed rejection command; rejection requires a reason but no candidate. The rejected seed
can then be regenerated only with the exact scoped `generate --force` command reported by `status`.

`prune --only-raw` preserves accepted seeds, selected row correction canvases, extracted targets, and lineage.
