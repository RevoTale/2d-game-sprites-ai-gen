// Package generate owns draft run state and provider-backed target generation.
//
// A manifest-v9 run stores one canonical character master, one semantic board
// per animation, complete-pose recovery evidence, a complete-unit
// review/deployment aggregate, and immutable lineage. Older manifests may
// remain on disk but are unsupported by commands.
package generate
