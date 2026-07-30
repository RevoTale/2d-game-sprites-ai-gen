# Consistency and normalization

The canonical master is the sole colored appearance authority for animation
requests. Current production art contributes exactly one configured reference
per direction to master generation; no other production animation frame is
uploaded. Object descriptions, identity locks, and optional object identity
references define appearance, materials, and colors. Direction references
define facing, neutral-pose topology, equipment side and geometry, proportions,
occupied scale, and registration; legacy direction-specific recoloring is not
authoritative. Broad style references are secondary.

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

One canonical visible scale and one preferred anchor per configured direction
are derived from the neutral reference frames and generated character master.
When a pack intentionally moves to a larger target canvas, an exact approved
predecessor reference is centered by its even canvas delta. The foreground and
visual mass remain unchanged; only its anchor receives the centering offset.
Reference hashes and source canvas dimensions are persisted in the canonical
profile, and incomplete or changed lineage blocks offline reassembly.
Separate provider boards are independent source coordinate systems. Frame `00`
of each animation/direction is compared with the matching master direction by
foreground visual mass; the resulting board-local source correction applies to
every frame of that direction. It may cancel provider coordinate drift but may
not create visible per-animation or per-frame resizing.

Once all complete poses are recovered, the preferred anchor is projected by the
minimum distance into the interval shared by the master and every calibrated
pose set. Every animation frame of the direction reuses the resulting anchor,
so moving equipment does not create per-frame jitter. Animation extents never
reduce canonical visible scale; an oversized pose or empty feasible interval
rejects the unit. Real source mass, bounds, canonical dimensions, correction
ratios, and scales are persisted for review. One deterministic maximum-32-color
palette is built from the canonical master only and applied with linear-sRGB
matching, hard alpha, area reduction, and no dithering.

The OpenAI provider explicitly requests `quality=high` for generations and
edits. GPT Image has no temperature or deterministic seed control in this
workflow, so consistency comes from evidence hierarchy, shared requests,
canonical palette locking, and manual review.

Mechanical checks decide only safe recovery and registration. Identity,
anatomy, equipment side and size, silhouette, occupied scale, cadence, motion,
and battlefield readability remain manual-review decisions.

Review artifacts include current direction references, raw and recovered
master evidence, raw semantic boards, ownership overlays, recovered pose
sheets, normalized animation sheets, the complete-unit sheet, native-size GIFs,
identity comparisons, prompts, evidence, metrics, hashes, lineage, and QA
notes. Normalized frames and the complete-unit sheet remain at the configured
target size; review assembly does not add an implicit enlargement.
