// Package generate owns draft run state and provider-backed target generation.
//
// A manifest-v5 run stores combined directional seed boards, accepted seed
// lineage, complete animation-row attempts, fixed-cell extraction evidence, and
// immutable selected-row lineage. Manifest-v1 through manifest-v4 runs may
// remain on disk but are unsupported by commands.
package generate
