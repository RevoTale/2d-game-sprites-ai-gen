package imageio

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
)

var opaqueChromaCandidates = [...]color.NRGBA{
	{R: 0, G: 255, B: 0, A: 255},
	{R: 255, G: 0, B: 255, A: 255},
	{R: 0, G: 255, B: 255, A: 255},
	{R: 255, G: 255, B: 0, A: 255},
	{R: 255, G: 0, B: 0, A: 255},
	{R: 0, G: 0, B: 255, A: 255},
}

// WriteOpaqueChromaCopies composites transparent provider evidence onto one
// shared, high-saturation chroma key chosen to be far from every subject
// color. Canonical transparent evidence remains unchanged at the input paths.
func WriteOpaqueChromaCopies(inputPaths, outputPaths []string) (color.NRGBA, error) {
	if len(inputPaths) == 0 || len(inputPaths) != len(outputPaths) {
		return color.NRGBA{}, fmt.Errorf("opaque chroma copies require matching non-empty inputs and outputs")
	}
	images := make([]*image.NRGBA, len(inputPaths))
	for index, path := range inputPaths {
		decoded, err := decodeNRGBA(path)
		if err != nil {
			return color.NRGBA{}, fmt.Errorf("decode chroma source %q: %w", path, err)
		}
		images[index] = decoded
	}
	background := mostDistinctChroma(images)
	for index, source := range images {
		opaque := image.NewNRGBA(source.Bounds())
		draw.Draw(opaque, opaque.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
		draw.Draw(opaque, opaque.Bounds(), source, source.Bounds().Min, draw.Over)
		if err := writePNG(outputPaths[index], opaque); err != nil {
			return color.NRGBA{}, err
		}
	}
	return background, nil
}

func mostDistinctChroma(images []*image.NRGBA) color.NRGBA {
	selected := opaqueChromaCandidates[0]
	selectedDistance := int64(-1)
	for _, candidate := range opaqueChromaCandidates {
		minimumDistance := int64(3 * 255 * 255)
		for _, source := range images {
			for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
				for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
					pixel := source.NRGBAAt(x, y)
					if pixel.A == 0 {
						continue
					}
					distance := squaredRGBDistance(candidate, pixel)
					if distance < minimumDistance {
						minimumDistance = distance
					}
				}
			}
		}
		if minimumDistance > selectedDistance {
			selected = candidate
			selectedDistance = minimumDistance
		}
	}
	return selected
}

func squaredRGBDistance(left, right color.NRGBA) int64 {
	red := int64(left.R) - int64(right.R)
	green := int64(left.G) - int64(right.G)
	blue := int64(left.B) - int64(right.B)
	return red*red + green*green + blue*blue
}
