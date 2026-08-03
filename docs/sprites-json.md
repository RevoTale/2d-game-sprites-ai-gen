# `sprites.json` V5

The strict JSON document is the only user-authored generation contract.
Unknown fields fail.

It owns global style/palette/contrast, style-guide inputs and destination, unit
archetypes and their semantic `scaleClass`, terrain families, object identity and continuity locks, explicit
directions, animation/frame semantics, render mode, dimensions, registration,
references, and deploy templates.

It never owns credentials, endpoints, provider lifecycle, canvas geometry,
chroma, numeric scale targets, semantic anchors, recovery thresholds, attempts, prompts,
reviews, hashes, lineage, manifests, or deployment state.

Animated objects require ordered `down`, `up`, and `right` directions and one
neutral PNG reference per direction. New output is `384x384`; an exact legacy
`320x320` reference is the only transition-size exception. Static objects
select one configured terrain family and remain at configured dimensions.

The style reference may be absent only while bootstrapping `--style-guide`.
Ordinary generation fails before provider construction until it exists.
