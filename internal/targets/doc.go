// Package targets turns a validated Pack into concrete generation targets.
//
// Expansion is deterministic: variant axes are combined as a cross product,
// animation frame order follows the frames array order, and missing frame IDs
// are derived from zero-padded array indexes. This package also builds the
// prompt text used by providers, so generation code does not need to understand
// pack structure details.
package targets
