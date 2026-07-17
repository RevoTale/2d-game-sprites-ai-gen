# Animated Consistency

## State Flow

```text
selected animated rows
  -> three object-wide directional-seed candidates
  -> human accepts one seed
  -> one candidate per selected animation/direction row
  -> complete-row mechanical validation and normalization
  -> automatic contact sheet, GIF, metrics, and extracted frames
  -> human accepts or rejects the complete row
  -> row-atomic deployment
```

One accepted directional seed is required per animated object/run. Broad pack style and object identity references stop
after seed generation. Row requests receive the accepted directional seed, normalized production pose evidence, specific
configured references, and a mask when one frame was selected for repair.

## Fixed Geometry

Provider images are 1024x1024 with a 32px outer margin, 16px true inter-cell gutters, and an inner cell guard of
`max(8px, cellWidth/32)`.

- Directional seed: centered 2x2 grid with one unused trailing cell.
- Four-frame row: centered horizontal four-cell grid.
- Extraction: fixed coordinates only after complete-board validation.

Seed prompts include the object description and every configured variant description, name every expected row/column
coordinate in row-major order, and explicitly identify every unused cell. Animation-row prompts include the object,
selected variant, and animation descriptions, then map every ordered frame ID and description to its exact left-to-right
row/column coordinate; a masked repair also names the sole editable frame cell. This keeps `sprites.json` descriptors
authoritative and prevents facing or frame-order requirements from relying on implication.
Provider masks are editing guidance, not a hard geometry guarantee, so generated pixels outside the expected cells still
fail mechanical validation. See the [OpenAI image-editing guide](https://developers.openai.com/api/docs/guides/image-generation#edit-an-image-using-a-mask).

References are cropped to alpha bounds before guide placement. Seed directions share body scale and baseline. After
seed acceptance, production pose evidence is aligned to that direction's canonical seed scale/baseline. Legacy frames
guide motion and cadence; they do not define hard silhouette truth.

## Hard Checks And Review Evidence

Reject candidates for wrong canvas dimensions; foreground in margins, gutters, trailing cells, or guards; missing
subjects; clipping or cross-cell foreground; a second primary component at least 60% of the largest; non-flat or
non-removable background; invalid lineage; or inability to fit the final cell at canonical seed scale without cropping.

Record legacy silhouette overlap, occupied-bounds, center/baseline, secondary disconnected equipment, palette distance,
and cadence as warnings/evidence. Mechanical validation does not decide anatomy, identity, equipment side, projectile
absence, motion quality, or battlefield readability.

Directional-seed candidate sheets label each board with its candidate ID and mechanical `VALID` or `INVALID` status.
`status` lists eligible candidates, invalid candidates, and the mechanically preferred candidate. Mechanical preference
only chooses structurally usable evidence; it never replaces manual visual review. For every generated review scope,
`status` also prints the saved prompt, QA note, candidate/contact sheets, GIF, and every extracted frame path that exists,
so configured cell meanings and native outputs are available from the same command as the next action. Seed and row
review decisions rewrite the owning QA note with the accepted/rejected status and reason; status never points reviewers
at a stale pre-review QA result.

## Normalization

Normalize each accepted seed direction to the canonical 160x160 body scale and baseline, then apply that same
direction-specific transform to every row frame. Preserve deterministic area reduction, hard alpha, a maximum 32-color
locked palette, linear-sRGB matching, and no dithering. Reject instead of shrinking, cropping, clearing, or deleting
malformed foreground.

## Repair And Lineage

Selecting one frame requires a complete production row. The production row is the edit source and the mask exposes only
the selected cell. Generation, review, extraction, and deployment remain complete-row operations, and every review and
lineage record in the row is reset.

Manifest V5 stores only current seed/row/candidate/review/lineage/reference-hash/deployment evidence. Older run versions
remain on disk but are unsupported. Animated deployment requires a complete accepted row with current seed/row lineage
and unchanged generation-start production hashes.
