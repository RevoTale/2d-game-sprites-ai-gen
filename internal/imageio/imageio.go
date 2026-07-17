// Package imageio validates, normalizes, copies, and assembles PNG image artifacts.
package imageio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const defaultPaletteSize = 32

type PaletteColor struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}

type Metrics struct {
	SilhouetteOverlap   float64 `json:"silhouetteOverlap"`
	EdgeAgreement       float64 `json:"edgeAgreement"`
	OccupiedAreaDelta   float64 `json:"occupiedAreaDelta"`
	OccupiedBoundsDelta float64 `json:"occupiedBoundsDelta"`
	CenterDistance      float64 `json:"centerDistance"`
	BaselineDelta       float64 `json:"baselineDelta"`
	PaletteDistance     float64 `json:"paletteDistance"`
	Components          int     `json:"components"`
	SecondaryComponents int     `json:"secondaryComponents,omitempty"`
	EdgeGuardOccupied   bool    `json:"edgeGuardOccupied"`
	Score               float64 `json:"score"`
}

func WriteNormalizedPNG(path string, data []byte, width, height int) error {
	_, err := WriteNormalizedPNGWithPalette(path, data, width, height, nil)
	return err
}

func WriteNormalizedPNGWithPalette(path string, data []byte, width, height int, locked []PaletteColor) ([]PaletteColor, error) {
	return WriteNormalizedPNGWithOptions(path, data, width, height, locked, false)
}

func WriteNormalizedPNGWithOptions(path string, data []byte, width, height int, locked []PaletteColor, removeBackground bool) ([]PaletteColor, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("normalized png size must be positive, got %dx%d", width, height)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}
	if removeBackground {
		img = removeEdgeBackground(img)
	}
	normalized, err := normalizeImage(img, width, height)
	if err != nil {
		return nil, err
	}
	palette := locked
	if len(palette) == 0 {
		palette = extractPalette(normalized, defaultPaletteSize)
	}
	normalized = applyPalette(normalized, palette)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(file, normalized); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return palette, nil
}

func HasTransparency(path string) (bool, error) {
	img, err := decodeNRGBA(path)
	if err != nil {
		return false, err
	}
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if img.NRGBAAt(x, y).A < 255 {
				return true, nil
			}
		}
	}
	return false, nil
}

func removeEdgeBackground(source image.Image) *image.NRGBA {
	bounds := source.Bounds()
	img := image.NewNRGBA(bounds)
	stddraw.Draw(img, bounds, source, bounds.Min, stddraw.Src)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.NRGBAAt(x, y).A < 255 {
				return img
			}
		}
	}
	corners := [...]color.NRGBA{
		img.NRGBAAt(bounds.Min.X, bounds.Min.Y),
		img.NRGBAAt(bounds.Max.X-1, bounds.Min.Y),
		img.NRGBAAt(bounds.Min.X, bounds.Max.Y-1),
		img.NRGBAAt(bounds.Max.X-1, bounds.Max.Y-1),
	}
	var red, green, blue int
	for _, corner := range corners {
		red += int(corner.R)
		green += int(corner.G)
		blue += int(corner.B)
	}
	background := color.NRGBA{R: uint8(red / len(corners)), G: uint8(green / len(corners)), B: uint8(blue / len(corners)), A: 255}
	for _, corner := range corners {
		if colorDelta(corner, background) > 72 {
			return img
		}
	}
	seen := make([]bool, bounds.Dx()*bounds.Dy())
	queue := make([]image.Point, 0, 2*bounds.Dx()+2*bounds.Dy())
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		queue = append(queue, image.Pt(x, bounds.Min.Y), image.Pt(x, bounds.Max.Y-1))
	}
	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y++ {
		queue = append(queue, image.Pt(bounds.Min.X, y), image.Pt(bounds.Max.X-1, y))
	}
	for len(queue) > 0 {
		point := queue[0]
		queue = queue[1:]
		index := (point.Y-bounds.Min.Y)*bounds.Dx() + point.X - bounds.Min.X
		if seen[index] {
			continue
		}
		seen[index] = true
		if colorDelta(img.NRGBAAt(point.X, point.Y), background) > 72 {
			continue
		}
		img.SetNRGBA(point.X, point.Y, color.NRGBA{})
		for _, next := range [...]image.Point{{X: point.X - 1, Y: point.Y}, {X: point.X + 1, Y: point.Y}, {X: point.X, Y: point.Y - 1}, {X: point.X, Y: point.Y + 1}} {
			if next.In(bounds) {
				queue = append(queue, next)
			}
		}
	}
	return img
}

func colorDelta(left, right color.NRGBA) int {
	return absInt(int(left.R)-int(right.R)) + absInt(int(left.G)-int(right.G)) + absInt(int(left.B)-int(right.B))
}

func WritePalette(path string, palette []PaletteColor) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(palette, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func PaletteFromPNG(path string, limit int) ([]PaletteColor, error) {
	img, err := decodeNRGBA(path)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultPaletteSize
	}
	return extractPalette(img, limit), nil
}

func normalizeImage(img image.Image, width, height int) (*image.NRGBA, error) {
	bounds := img.Bounds()
	if bounds.Dx() < width || bounds.Dy() < height {
		return nil, fmt.Errorf("png dimensions are %dx%d, smaller than target %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	dstRect := aspectFitRect(bounds.Dx(), bounds.Dy(), width, height)
	areaScale(dst, dstRect, img, bounds)
	return dst, nil
}

func areaScale(dst *image.NRGBA, dstRect image.Rectangle, src image.Image, srcRect image.Rectangle) {
	for y := dstRect.Min.Y; y < dstRect.Max.Y; y++ {
		sy0 := srcRect.Min.Y + (y-dstRect.Min.Y)*srcRect.Dy()/dstRect.Dy()
		sy1 := srcRect.Min.Y + (y-dstRect.Min.Y+1)*srcRect.Dy()/dstRect.Dy()
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := dstRect.Min.X; x < dstRect.Max.X; x++ {
			sx0 := srcRect.Min.X + (x-dstRect.Min.X)*srcRect.Dx()/dstRect.Dx()
			sx1 := srcRect.Min.X + (x-dstRect.Min.X+1)*srcRect.Dx()/dstRect.Dx()
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var red, green, blue, alpha, count uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					c := color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA)
					red += uint64(c.R) * uint64(c.A)
					green += uint64(c.G) * uint64(c.A)
					blue += uint64(c.B) * uint64(c.A)
					alpha += uint64(c.A)
					count++
				}
			}
			out := color.NRGBA{}
			if count > 0 && alpha/count >= 128 {
				out.A = 255
				out.R = uint8(red / alpha)
				out.G = uint8(green / alpha)
				out.B = uint8(blue / alpha)
			}
			dst.SetNRGBA(x, y, out)
		}
	}
}

type weightedColor struct {
	color PaletteColor
	count int
}

func extractPalette(img *image.NRGBA, limit int) []PaletteColor {
	counts := make(map[uint32]int)
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			key := uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
			counts[key]++
		}
	}
	colors := make([]weightedColor, 0, len(counts))
	for key, count := range counts {
		colors = append(colors, weightedColor{color: PaletteColor{R: uint8(key >> 16), G: uint8(key >> 8), B: uint8(key)}, count: count})
	}
	sort.Slice(colors, func(i, j int) bool { return colorKey(colors[i].color) < colorKey(colors[j].color) })
	if len(colors) <= limit {
		palette := make([]PaletteColor, len(colors))
		for i := range colors {
			palette[i] = colors[i].color
		}
		return palette
	}
	boxes := [][]weightedColor{colors}
	for len(boxes) < limit {
		index := splittableBox(boxes)
		if index < 0 {
			break
		}
		left, right := splitColorBox(boxes[index])
		boxes[index] = left
		boxes = append(boxes, right)
	}
	palette := make([]PaletteColor, 0, len(boxes))
	for _, box := range boxes {
		palette = append(palette, averageColor(box))
	}
	sort.Slice(palette, func(i, j int) bool { return colorKey(palette[i]) < colorKey(palette[j]) })
	return palette
}

func splittableBox(boxes [][]weightedColor) int {
	best, bestRange := -1, -1
	for i, box := range boxes {
		if len(box) < 2 {
			continue
		}
		red, green, blue := colorRanges(box)
		rangeValue := max(red, green, blue)
		if rangeValue > bestRange {
			best, bestRange = i, rangeValue
		}
	}
	return best
}

func splitColorBox(box []weightedColor) ([]weightedColor, []weightedColor) {
	red, green, blue := colorRanges(box)
	channel := 0
	if green > red && green >= blue {
		channel = 1
	} else if blue > red && blue > green {
		channel = 2
	}
	sort.SliceStable(box, func(i, j int) bool {
		left, right := channelValue(box[i].color, channel), channelValue(box[j].color, channel)
		if left == right {
			return colorKey(box[i].color) < colorKey(box[j].color)
		}
		return left < right
	})
	total := 0
	for _, value := range box {
		total += value.count
	}
	running, split := 0, 1
	for i := 0; i < len(box)-1; i++ {
		running += box[i].count
		if running*2 >= total {
			split = i + 1
			break
		}
	}
	return box[:split], box[split:]
}

func colorRanges(box []weightedColor) (int, int, int) {
	minR, minG, minB, maxR, maxG, maxB := 255, 255, 255, 0, 0, 0
	for _, value := range box {
		minR, minG, minB = min(minR, int(value.color.R)), min(minG, int(value.color.G)), min(minB, int(value.color.B))
		maxR, maxG, maxB = max(maxR, int(value.color.R)), max(maxG, int(value.color.G)), max(maxB, int(value.color.B))
	}
	return maxR - minR, maxG - minG, maxB - minB
}

func averageColor(box []weightedColor) PaletteColor {
	var red, green, blue, count int
	for _, value := range box {
		red += int(value.color.R) * value.count
		green += int(value.color.G) * value.count
		blue += int(value.color.B) * value.count
		count += value.count
	}
	return PaletteColor{R: uint8(red / count), G: uint8(green / count), B: uint8(blue / count)}
}

func applyPalette(img *image.NRGBA, palette []PaletteColor) *image.NRGBA {
	dst := image.NewNRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A < 128 || len(palette) == 0 {
				continue
			}
			nearest := palette[0]
			best := linearDistance(c, nearest)
			for _, candidate := range palette[1:] {
				distance := linearDistance(c, candidate)
				if distance < best {
					nearest, best = candidate, distance
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{R: nearest.R, G: nearest.G, B: nearest.B, A: 255})
		}
	}
	return dst
}

func linearDistance(c color.NRGBA, candidate PaletteColor) float64 {
	r := linearSRGB(c.R) - linearSRGB(candidate.R)
	g := linearSRGB(c.G) - linearSRGB(candidate.G)
	b := linearSRGB(c.B) - linearSRGB(candidate.B)
	return r*r + g*g + b*b
}

func linearSRGB(value uint8) float64 {
	v := float64(value) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func colorKey(c PaletteColor) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }

func channelValue(c PaletteColor, channel int) uint8 {
	switch channel {
	case 0:
		return c.R
	case 1:
		return c.G
	default:
		return c.B
	}
}

func aspectFitRect(srcW, srcH, dstW, dstH int) image.Rectangle {
	scale := min(float64(dstW)/float64(srcW), float64(dstH)/float64(srcH))
	scaledW := max(1, int(math.Round(float64(srcW)*scale)))
	scaledH := max(1, int(math.Round(float64(srcH)*scale)))
	if scaledW > dstW {
		scaledW = dstW
	}
	if scaledH > dstH {
		scaledH = dstH
	}
	x := (dstW - scaledW) / 2
	y := (dstH - scaledH) / 2
	return image.Rect(x, y, x+scaledW, y+scaledH)
}
