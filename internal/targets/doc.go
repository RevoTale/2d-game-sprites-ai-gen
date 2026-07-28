// Package targets turns a validated Pack into concrete generation targets.
//
// Expansion is deterministic: variant axes are combined as a cross product,
// animation and frame order follow their array order, and missing frame IDs are
// derived from zero-padded array indexes. This package also builds target
// prompts, infers typed input roles from schema ownership, and preserves array
// order metadata for character-master and animation-board planning.
package targets
