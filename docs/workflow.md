# Workflow

Animated generation is unit-atomic:

1. Validate V4 configuration and direction reference PNGs.
2. Derive one canonical subject profile from neutral direction references.
3. Generate and recover a canonical multi-direction character master.
4. Generate one semantic board per animation across all configured directions.
5. Calibrate each animation/direction from frame `00` to its matching master
   direction, recover complete ordered poses, apply the canonical visible scale
   and palette, and create review artifacts.
6. Accept or reject the complete unit.
7. Preview and deploy every frame atomically.
8. Optionally refine tracked production PNGs with a painter.

Draft paths:

```text
output/runs/<run-id>/
  manifest.json
  intermediates/<object>/character-master/
  intermediates/<object>/animations/<animation>/
  targets/<target-id>/
  units/<object>/review/
```

Manifest V10 stores the canonical profile, one `character-master`, one `animation-board` per animation,
one derived `unit`, frame deployment evidence, hashes, lineage, review, and
deployment facts. Older manifests remain untouched and unsupported.

Each animation request sends one opaque flat-chroma semantic board prefilled
with the matching recovered master direction at every logical anchor. It sends
no mask and does not upload a separate master, style/identity reference,
production direction reference, legacy animation frame, or neighboring board.

The character master is matched to the configured reference profile. Each
independent animation board derives one source-coordinate correction per
direction from frame `00`; the correction applies unchanged to all frames of
that direction. This preserves the same canonical visible scale across calls.
Complete action extents are clipping evidence, not scale input; unsafe poses
are rejected instead of shrinking the unit.

A rejected animated run is immutable. Regenerate with a fresh object-wide
`--run auto`; there is no animation, direction, row, or frame retry. An
interrupted non-rejected run may resume from its first incomplete stage.

Static targets retain their existing independent workflow.
