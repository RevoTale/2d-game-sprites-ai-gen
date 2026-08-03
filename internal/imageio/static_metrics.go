package imageio

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

const smallColorClusterMaximumPixels = 4

// StaticEvidence records deterministic texture evidence. It deliberately does
// not decide whether repetition or material flow is artistically acceptable.
type StaticEvidence struct {
	OpaqueRatio                float64
	HorizontalEdgeDelta        float64
	VerticalEdgeDelta          float64
	MaximumHorizontalEdgeDelta float64
	MaximumVerticalEdgeDelta   float64
	SmallClusterRatio          float64
	LuminanceRange             float64
}

// MeasureStaticEvidence records visual-review evidence without deciding
// whether a material, seam, or contrast treatment is artistically acceptable.
func MeasureStaticEvidence(path string) (StaticEvidence, error) {
	file, err := os.Open(path)
	if err != nil {
		return StaticEvidence{}, err
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		return StaticEvidence{}, err
	}
	bounds := decoded.Bounds()
	pixels := bounds.Dx() * bounds.Dy()
	if pixels == 0 {
		return StaticEvidence{}, nil
	}
	opaque := 0
	minimumLuminance, maximumLuminance := 1.0, 0.0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			value := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA)
			if value.A == 255 {
				opaque++
			}
			luminance := (0.2126*float64(value.R) + 0.7152*float64(value.G) + 0.0722*float64(value.B)) / 255
			minimumLuminance = min(minimumLuminance, luminance)
			maximumLuminance = max(maximumLuminance, luminance)
		}
	}
	horizontalEdgeDelta, maximumHorizontalEdgeDelta := oppositeEdgeEvidence(decoded, true)
	verticalEdgeDelta, maximumVerticalEdgeDelta := oppositeEdgeEvidence(decoded, false)
	return StaticEvidence{
		OpaqueRatio:                float64(opaque) / float64(pixels),
		HorizontalEdgeDelta:        horizontalEdgeDelta,
		VerticalEdgeDelta:          verticalEdgeDelta,
		MaximumHorizontalEdgeDelta: maximumHorizontalEdgeDelta,
		MaximumVerticalEdgeDelta:   maximumVerticalEdgeDelta,
		SmallClusterRatio:          smallColorClusterRatio(decoded),
		LuminanceRange:             maximumLuminance - minimumLuminance,
	}, nil
}

func oppositeEdgeEvidence(img image.Image, horizontal bool) (float64, float64) {
	bounds := img.Bounds()
	count := bounds.Dy()
	if !horizontal {
		count = bounds.Dx()
	}
	if count == 0 {
		return 0, 0
	}
	var total, maximum float64
	for index := 0; index < count; index++ {
		var left, right color.NRGBA
		if horizontal {
			y := bounds.Min.Y + index
			left = color.NRGBAModel.Convert(img.At(bounds.Min.X, y)).(color.NRGBA)
			right = color.NRGBAModel.Convert(img.At(bounds.Max.X-1, y)).(color.NRGBA)
		} else {
			x := bounds.Min.X + index
			left = color.NRGBAModel.Convert(img.At(x, bounds.Min.Y)).(color.NRGBA)
			right = color.NRGBAModel.Convert(img.At(x, bounds.Max.Y-1)).(color.NRGBA)
		}
		delta := (absChannel(left.R, right.R) +
			absChannel(left.G, right.G) +
			absChannel(left.B, right.B) +
			absChannel(left.A, right.A)) / (4 * 255)
		total += delta
		maximum = max(maximum, delta)
	}
	return total / float64(count), maximum
}

func smallColorClusterRatio(img image.Image) float64 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return 0
	}
	visited := make([]bool, width*height)
	opaquePixels, smallPixels := 0, 0
	queue := make([]image.Point, 0, 64)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			index := (y-bounds.Min.Y)*width + x - bounds.Min.X
			value := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if value.A < 128 || visited[index] {
				continue
			}
			queue = append(queue[:0], image.Pt(x, y))
			visited[index] = true
			clusterPixels := 0
			for len(queue) != 0 {
				point := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				clusterPixels++
				for _, next := range [...]image.Point{
					{X: point.X - 1, Y: point.Y},
					{X: point.X + 1, Y: point.Y},
					{X: point.X, Y: point.Y - 1},
					{X: point.X, Y: point.Y + 1},
				} {
					if !next.In(bounds) {
						continue
					}
					nextIndex := (next.Y-bounds.Min.Y)*width + next.X - bounds.Min.X
					if visited[nextIndex] || color.NRGBAModel.Convert(img.At(next.X, next.Y)).(color.NRGBA) != value {
						continue
					}
					visited[nextIndex] = true
					queue = append(queue, next)
				}
			}
			opaquePixels += clusterPixels
			if clusterPixels <= smallColorClusterMaximumPixels {
				smallPixels += clusterPixels
			}
		}
	}
	if opaquePixels == 0 {
		return 0
	}
	return float64(smallPixels) / float64(opaquePixels)
}

func absChannel(left, right uint8) float64 {
	if left >= right {
		return float64(left - right)
	}
	return float64(right - left)
}
