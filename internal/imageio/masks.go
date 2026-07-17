package imageio

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// WriteMotionMask creates an OpenAI-compatible edit mask. Stable pixels are
// opaque; changed silhouette and color regions are transparent and dilated so
// articulated limbs and equipment have room to move.
func WriteMotionMask(anchorPath, posePath, outputPath string) error {
	anchor, err := decodeNRGBA(anchorPath)
	if err != nil {
		return err
	}
	pose, err := decodeNRGBA(posePath)
	if err != nil {
		return err
	}
	if anchor.Bounds() != pose.Bounds() {
		return fmt.Errorf("anchor %dx%d and pose %dx%d dimensions differ", anchor.Bounds().Dx(), anchor.Bounds().Dy(), pose.Bounds().Dx(), pose.Bounds().Dy())
	}
	bounds := anchor.Bounds()
	changed := make([]bool, bounds.Dx()*bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			left, right := anchor.NRGBAAt(x, y), pose.NRGBAAt(x, y)
			colorDelta := absInt(int(left.R)-int(right.R)) + absInt(int(left.G)-int(right.G)) + absInt(int(left.B)-int(right.B))
			if (left.A > 0) != (right.A > 0) || (left.A > 0 && right.A > 0 && colorDelta > 72) {
				changed[(y-bounds.Min.Y)*bounds.Dx()+x-bounds.Min.X] = true
			}
		}
	}
	radius := max(1, bounds.Dx()/32)
	mask := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			moving := false
			for dy := -radius; dy <= radius && !moving; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					point := image.Pt(x+dx, y+dy)
					if point.In(bounds) && changed[(point.Y-bounds.Min.Y)*bounds.Dx()+point.X-bounds.Min.X] {
						moving = true
						break
					}
				}
			}
			if !moving {
				mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	return writePNG(outputPath, mask)
}

// WriteEditableSubjectMask preserves transparent padding and exposes the pose
// silhouette for edit-based directional-anchor generation.
func WriteEditableSubjectMask(posePath, outputPath string) error {
	pose, err := decodeNRGBA(posePath)
	if err != nil {
		return err
	}
	mask := image.NewNRGBA(pose.Bounds())
	for y := pose.Bounds().Min.Y; y < pose.Bounds().Max.Y; y++ {
		for x := pose.Bounds().Min.X; x < pose.Bounds().Max.X; x++ {
			if pose.NRGBAAt(x, y).A == 0 {
				mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	return writePNG(outputPath, mask)
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
		file.Close()
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
