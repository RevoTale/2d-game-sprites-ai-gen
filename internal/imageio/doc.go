// Package imageio handles PNG-oriented filesystem operations.
//
// It removes flat backgrounds, recovers complete poses from semantic boards,
// applies unit-wide registration, area-reduces provider canvases, locks
// deterministic palettes without dithering, hardens alpha, and builds evidence.
// Reusable static helpers also provide fixed boards and edit masks. Structural
// failures stay separate from human-review warnings; foreground is never
// cropped, erased, or independently fitted to hide malformed output.
package imageio
