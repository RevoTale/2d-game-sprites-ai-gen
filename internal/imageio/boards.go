package imageio

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

var ErrProductionFrameClipping = errors.New("foreground would touch or cross a production frame edge")

// StudyMetrics is compact manifest evidence for a generated board. Semantic
// recovery owns structural validation; these values remain human-review data.
type StudyMetrics struct {
	Cells            []Metrics `json:"cells,omitempty"`
	Score            float64   `json:"score"`
	CadenceDelta     float64   `json:"cadenceDelta,omitempty"`
	MarginOccupied   bool      `json:"marginOccupied,omitempty"`
	GutterOccupied   bool      `json:"gutterOccupied,omitempty"`
	TrailingOccupied bool      `json:"trailingOccupied,omitempty"`
}

// BoardEvaluation separates structural failures from non-blocking review
// evidence.
type BoardEvaluation struct {
	Metrics          StudyMetrics
	BlockingFailures []string
	Warnings         []string
}

// PNGDimensions returns decoded PNG dimensions without loading all pixels.
func PNGDimensions(path string) (image.Point, error) {
	file, err := os.Open(path)
	if err != nil {
		return image.Point{}, err
	}
	defer file.Close()
	config, err := png.DecodeConfig(file)
	if err != nil {
		return image.Point{}, err
	}
	return image.Pt(config.Width, config.Height), nil
}

// WriteTransparentBoard removes an opaque edge-connected background while
// preserving the provider canvas dimensions used by semantic recovery.
func WriteTransparentBoard(path string, data []byte, width, height int) error {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode generated board: %w", err)
	}
	if decoded.Bounds().Dx() != width || decoded.Bounds().Dy() != height {
		return fmt.Errorf(
			"generated board is %dx%d, expected %dx%d",
			decoded.Bounds().Dx(),
			decoded.Bounds().Dy(),
			width,
			height,
		)
	}
	return writePNG(path, removeEdgeBackground(decoded))
}

// WriteEditableMaskedBoard treats a CLI-owned edit mask as the exact ownership
// boundary for full-bleed material parts. Provider masks are advisory, so raw
// pixels outside transparent mask regions must never merge neighboring parts.
func WriteEditableMaskedBoard(
	path string,
	data []byte,
	maskPath string,
	width, height int,
) error {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode generated board: %w", err)
	}
	expected := image.Rect(0, 0, width, height)
	if decoded.Bounds() != expected {
		return fmt.Errorf(
			"generated board is %dx%d, expected %dx%d",
			decoded.Bounds().Dx(),
			decoded.Bounds().Dy(),
			width,
			height,
		)
	}
	mask, err := decodeNRGBA(maskPath)
	if err != nil {
		return fmt.Errorf("decode editable ownership mask: %w", err)
	}
	if mask.Bounds() != expected {
		return fmt.Errorf(
			"editable ownership mask is %dx%d, expected %dx%d",
			mask.Bounds().Dx(),
			mask.Bounds().Dy(),
			width,
			height,
		)
	}
	result := image.NewNRGBA(expected)
	for y := expected.Min.Y; y < expected.Max.Y; y++ {
		for x := expected.Min.X; x < expected.Max.X; x++ {
			if mask.NRGBAAt(x, y).A != 0 {
				continue
			}
			result.SetNRGBA(
				x,
				y,
				color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA),
			)
		}
	}
	return writePNG(path, result)
}

// CanonicalFrameEdgePadding is the transparent final-frame gutter preserved
// without shrinking generated foreground.
func CanonicalFrameEdgePadding(_, _ int) int {
	return 1
}

// ValidateCanonicalFrame verifies the final encoded artifact.
func ValidateCanonicalFrame(path string, width, height int) error {
	frame, err := decodeNRGBA(path)
	if err != nil {
		return err
	}
	canvas := image.Rect(0, 0, width, height)
	if frame.Bounds() != canvas {
		return fmt.Errorf(
			"canonical frame dimensions are %dx%d, expected %dx%d",
			frame.Bounds().Dx(),
			frame.Bounds().Dy(),
			width,
			height,
		)
	}
	foreground, err := alphaBounds(frame)
	if err != nil {
		return err
	}
	safeCanvas := canvas.Inset(CanonicalFrameEdgePadding(width, height))
	if !foreground.In(safeCanvas) {
		return fmt.Errorf(
			"%w: final encoded foreground %v contacts fixed frame edge %v",
			ErrProductionFrameClipping,
			foreground,
			canvas,
		)
	}
	return nil
}

func alphaBounds(img *image.NRGBA) (image.Rectangle, error) {
	bounds := img.Bounds()
	result := image.Rectangle{Min: bounds.Max, Max: bounds.Min}
	found := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			found = true
			result.Min.X = min(result.Min.X, x)
			result.Min.Y = min(result.Min.Y, y)
			result.Max.X = max(result.Max.X, x+1)
			result.Max.Y = max(result.Max.Y, y+1)
		}
	}
	if !found {
		return image.Rectangle{}, errors.New("contains no foreground subject")
	}
	return result, nil
}

// ForegroundBounds returns exact non-transparent PNG bounds.
func ForegroundBounds(path string) (image.Rectangle, error) {
	img, err := decodeNRGBA(path)
	if err != nil {
		return image.Rectangle{}, err
	}
	return alphaBounds(img)
}
