package imageio

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
)

// CanonicalTransform records the deterministic scale and placement applied to
// one generated pose. It is evidence for review and lineage, not input policy.
type CanonicalTransform struct {
	Scale    float64 `json:"scale"`
	Baseline int     `json:"baseline"`
	CenterX  int     `json:"centerX"`
	OffsetX  int     `json:"offsetX,omitempty"`
	OffsetY  int     `json:"offsetY,omitempty"`
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func roundUp(value, multiple int) int {
	return (value + multiple - 1) / multiple * multiple
}

func neighboringPixels(point image.Point) [8]image.Point {
	return [8]image.Point{
		{X: point.X - 1, Y: point.Y - 1},
		{X: point.X, Y: point.Y - 1},
		{X: point.X + 1, Y: point.Y - 1},
		{X: point.X - 1, Y: point.Y},
		{X: point.X + 1, Y: point.Y},
		{X: point.X - 1, Y: point.Y + 1},
		{X: point.X, Y: point.Y + 1},
		{X: point.X + 1, Y: point.Y + 1},
	}
}
