// Package imageio handles PNG-oriented filesystem operations.
//
// It area-reduces provider canvases, locks deterministic palettes without
// dithering, hardens alpha, builds edit masks, scores candidate geometry, copies
// files, deterministic reference boards, and review sheets. Generated animation
// rows are extracted only at fixed coordinates after whole-board validation.
// Seed-board validation keeps structural failures separate from human-review
// warnings; this package never searches for or repairs malformed cell boundaries.
package imageio
