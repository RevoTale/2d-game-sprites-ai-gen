# Consistency and normalization

The canonical master is the sole colored appearance authority for animation
requests. Current production art contributes exactly one configured reference
per direction to master generation; no other production animation frame is
uploaded. Direction references define silhouette, proportions, palette,
equipment, pixel density, outline weight, and shading. Broad style references
are secondary.

The master uses a deterministic near-square semantic layout on a minimum
1024x1024 opaque flat-chroma canvas. Animation boards use logical direction rows
and ordered frame columns. Current three-direction/four-frame boards are
1536x1152 with 384px anchor spacing. Anchors communicate order and approximate
placement; they are not raster clipping boundaries. Provider masks, visible
guides, panels, labels, and slot backgrounds are absent.

Each animation request sends one colored image: the opaque semantic board
prefilled with the matching recovered master direction at every anchor. The
separate master, style/object references, configured production references,
other production frames, and neighboring animation boards remain unsent
lineage or review evidence.

After deterministic border-connected chroma removal, recovery:

1. identifies the expected primary body cores;
2. assigns them one-to-one to ordered logical anchors;
3. attaches every disconnected component to one unambiguous core;
4. forms separated complete pose groups;
5. verifies row and frame ordering; and
6. crops confirmed background only after ownership is proven.

Wide attached swords, shields, wings, tails, and cloth may cross nominal anchor
midpoints. Missing cores, ambiguous or unowned components, merged or overlapping
groups, non-monotonic ordering, real canvas-edge contact, invalid background,
or a pose that cannot fit the production frame reject the unit. Recovery never
deletes, clears, synthesizes, or independently shrinks foreground.

One unit-wide transform is derived from the widest and tallest envelope across
all recovered master and animation poses. It uses one scale, body pivot, and
baseline for every final frame so moving equipment does not recenter the body.
The target body scale is reduced once for the complete unit when its envelope
requires more reserve; individual poses are never fitted. Provider scale drift
remains visible to reviewers instead of being hidden by per-frame fitting. One
deterministic maximum-32-color palette is built from the complete unit and
applied with linear-sRGB matching, hard alpha, area reduction, and no dithering.

Mechanical checks decide only safe recovery and registration. Identity,
anatomy, equipment side and size, silhouette, occupied scale, cadence, motion,
and battlefield readability remain manual-review decisions.

Review artifacts include current direction references, raw and recovered
master evidence, raw semantic boards, ownership overlays, recovered pose
sheets, normalized animation sheets, the complete-unit sheet, native-size GIFs,
identity comparisons, prompts, evidence, metrics, hashes, lineage, and QA
notes. Normalized frames and the complete-unit sheet remain at the configured
target size; review assembly does not add an implicit enlargement.
