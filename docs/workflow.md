# Workflow

Animated generation is unit-atomic:

1. Validate V3 configuration and direction reference PNGs.
2. Generate and recover a canonical multi-direction character master.
3. Generate one semantic board per animation across all configured directions.
4. Recover complete ordered poses, apply one unit-wide transform and palette,
   and create review artifacts.
5. Accept or reject the complete unit.
6. Preview and deploy every frame atomically.
7. Optionally refine tracked production PNGs with a painter.

Draft paths:

```text
output/runs/<run-id>/
  manifest.json
  intermediates/<object>/character-master/
  intermediates/<object>/animations/<animation>/
  targets/<target-id>/
  units/<object>/review/
```

Manifest V9 stores one `character-master`, one `animation-board` per animation,
one derived `unit`, frame deployment evidence, hashes, lineage, review, and
deployment facts. Older manifests remain untouched and unsupported.

Each animation request sends one opaque flat-chroma semantic board prefilled
with the matching recovered master direction at every logical anchor. It sends
no mask and does not upload a separate master, style/identity reference,
production direction reference, legacy animation frame, or neighboring board.

A rejected animated run is immutable. Regenerate with a fresh object-wide
`--run auto`; there is no animation, direction, row, or frame retry. An
interrupted non-rejected run may resume from its first incomplete stage.

Static targets retain their existing independent workflow.
