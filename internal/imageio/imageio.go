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

const (
	opaqueTileMicroClusterMaximumPixels = 8
	opaqueTileDespecklePasses           = 4
)

// CompositePaletteSize preserves several per-asset ramps in one style board.
const CompositePaletteSize = 128

const (
	backgroundExactDelta  = 72
	backgroundSpillDelta  = 270
	backgroundSpillExcess = 24
	backgroundSpillPasses = 6
)

type PaletteColor struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}

type Metrics struct {
	SilhouetteOverlap          float64 `json:"silhouetteOverlap"`
	EdgeAgreement              float64 `json:"edgeAgreement"`
	OccupiedAreaDelta          float64 `json:"occupiedAreaDelta"`
	OccupiedBoundsDelta        float64 `json:"occupiedBoundsDelta"`
	CenterDistance             float64 `json:"centerDistance"`
	BaselineDelta              float64 `json:"baselineDelta"`
	PaletteDistance            float64 `json:"paletteDistance"`
	Components                 int     `json:"components"`
	SecondaryComponents        int     `json:"secondaryComponents,omitempty"`
	EdgeGuardOccupied          bool    `json:"edgeGuardOccupied"`
	CellEdgeOccupied           bool    `json:"cellEdgeOccupied"`
	BackdropLike               bool    `json:"backdropLike"`
	Score                      float64 `json:"score"`
	OpaqueRatio                float64 `json:"opaqueRatio,omitempty"`
	HorizontalEdgeDelta        float64 `json:"horizontalEdgeDelta,omitempty"`
	VerticalEdgeDelta          float64 `json:"verticalEdgeDelta,omitempty"`
	MaximumHorizontalEdgeDelta float64 `json:"maximumHorizontalEdgeDelta,omitempty"`
	MaximumVerticalEdgeDelta   float64 `json:"maximumVerticalEdgeDelta,omitempty"`
	SmallClusterRatio          float64 `json:"smallClusterRatio,omitempty"`
	LuminanceRange             float64 `json:"luminanceRange,omitempty"`
}

func WriteNormalizedPNG(path string, data []byte, width, height int) error {
	_, err := WriteNormalizedPNGWithPalette(path, data, width, height, nil)
	return err
}

func WriteNormalizedPNGWithPalette(path string, data []byte, width, height int, locked []PaletteColor) ([]PaletteColor, error) {
	return WriteNormalizedPNGWithOptions(path, data, width, height, locked, false)
}

func WriteNormalizedPNGWithOptions(path string, data []byte, width, height int, locked []PaletteColor, removeBackground bool) ([]PaletteColor, error) {
	return writeNormalizedPNG(
		path,
		data,
		width,
		height,
		locked,
		removeBackground,
		false,
		defaultPaletteSize,
		false,
	)
}

// WriteNormalizedOpaqueTilePNG applies the ordinary production palette and
// then merges tiny exact-color islands into their dominant neighboring
// material. Connectivity wraps across opposite edges so cleanup cannot create
// a seam in a tile that is meant to repeat.
func WriteNormalizedOpaqueTilePNG(path string, data []byte, width, height int, locked []PaletteColor) ([]PaletteColor, error) {
	return writeNormalizedPNG(
		path,
		data,
		width,
		height,
		locked,
		false,
		false,
		defaultPaletteSize,
		true,
	)
}

// WriteNormalizedIsolatedPNG converts an isolated provider composition into
// its declared native canvas. Provider background is coordinate reserve, not
// subject scale: remove it, retain the complete foreground bounds, reduce that
// foreground without stretching, and apply only semantic registration.
func WriteNormalizedIsolatedPNG(
	path string,
	data []byte,
	width, height int,
	locked []PaletteColor,
	registration SubjectRegistrationMode,
) ([]PaletteColor, error) {
	return writeNormalizedIsolatedPNG(
		path,
		data,
		width,
		height,
		locked,
		registration,
		false,
		true,
		false,
	)
}

// WriteNormalizedTransparentIsolatedPNG fits an already background-free crop
// into its declared canvas. Semantic board recovery owns background removal,
// so edge-touching pixels in the crop are real subject pixels.
func WriteNormalizedTransparentIsolatedPNG(
	path string,
	data []byte,
	width, height int,
	locked []PaletteColor,
	registration SubjectRegistrationMode,
) ([]PaletteColor, error) {
	return writeNormalizedIsolatedPNG(
		path,
		data,
		width,
		height,
		locked,
		registration,
		false,
		false,
		false,
	)
}

// WriteNativeScaleTransparentIsolatedPNG preserves the shared provider-board
// scale and only registers the recovered crop inside its production canvas.
// Oversized subjects reject instead of being independently reduced.
func WriteNativeScaleTransparentIsolatedPNG(
	path string,
	data []byte,
	width, height int,
	locked []PaletteColor,
	registration SubjectRegistrationMode,
) ([]PaletteColor, error) {
	return writeNormalizedIsolatedPNG(
		path,
		data,
		width,
		height,
		locked,
		registration,
		false,
		false,
		true,
	)
}

// WriteIsolatedReviewPreviewPNG writes an exact-size review artifact from the
// visible subject rather than the source canvas. It mirrors the production
// portrait packer's alpha-bounds fitting while allowing display-only upscale.
func WriteIsolatedReviewPreviewPNG(
	path string,
	data []byte,
	width, height int,
	locked []PaletteColor,
	registration SubjectRegistrationMode,
) ([]PaletteColor, error) {
	return writeNormalizedIsolatedPNG(
		path,
		data,
		width,
		height,
		locked,
		registration,
		true,
		true,
		false,
	)
}

func writeNormalizedIsolatedPNG(
	path string,
	data []byte,
	width, height int,
	locked []PaletteColor,
	registration SubjectRegistrationMode,
	allowUpscale bool,
	removeBackground bool,
	preserveScale bool,
) ([]PaletteColor, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("normalized png size must be positive, got %dx%d", width, height)
	}
	if registration != SubjectRegistrationCentered &&
		registration != SubjectRegistrationGrounded {
		return nil, fmt.Errorf("unsupported isolated registration %q", registration)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}
	foreground := image.NewNRGBA(decoded.Bounds())
	stddraw.Draw(
		foreground,
		foreground.Bounds(),
		decoded,
		decoded.Bounds().Min,
		stddraw.Src,
	)
	if removeBackground {
		foreground = removeEdgeBackground(decoded)
	}
	foregroundBounds, err := alphaBounds(foreground)
	if err != nil {
		return nil, fmt.Errorf("isolated foreground: %w", err)
	}
	guard := max(1, min(width, height)/32)
	safe := image.Rect(guard, guard, width-guard, height-guard)
	if safe.Empty() {
		return nil, fmt.Errorf("isolated target %dx%d has no safe foreground rectangle", width, height)
	}
	fittedWidth, fittedHeight := foregroundBounds.Dx(), foregroundBounds.Dy()
	if preserveScale {
		if fittedWidth > safe.Dx() || fittedHeight > safe.Dy() {
			return nil, fmt.Errorf(
				"isolated foreground %dx%d exceeds native-scale safe target %dx%d",
				fittedWidth,
				fittedHeight,
				safe.Dx(),
				safe.Dy(),
			)
		}
	} else {
		scale := math.Min(
			float64(safe.Dx())/float64(foregroundBounds.Dx()),
			float64(safe.Dy())/float64(foregroundBounds.Dy()),
		)
		if scale > 1 && !allowUpscale {
			return nil, fmt.Errorf(
				"isolated foreground %dx%d requires upscaling to fit target %dx%d",
				foregroundBounds.Dx(),
				foregroundBounds.Dy(),
				width,
				height,
			)
		}
		fitted := aspectFitRect(
			foregroundBounds.Dx(),
			foregroundBounds.Dy(),
			safe.Dx(),
			safe.Dy(),
		)
		fittedWidth, fittedHeight = fitted.Dx(), fitted.Dy()
	}
	left := safe.Min.X + (safe.Dx()-fittedWidth)/2
	top := safe.Min.Y + (safe.Dy()-fittedHeight)/2
	if registration == SubjectRegistrationGrounded {
		top = safe.Max.Y - fittedHeight
	}
	normalized := image.NewNRGBA(image.Rect(0, 0, width, height))
	areaScale(
		normalized,
		image.Rect(left, top, left+fittedWidth, top+fittedHeight),
		foreground,
		foregroundBounds,
	)
	palette := locked
	if len(palette) == 0 {
		palette = extractPalette(normalized, defaultPaletteSize)
	}
	normalized = applyPalette(normalized, palette)
	if err := writePNG(path, normalized); err != nil {
		return nil, err
	}
	return palette, nil
}

// WriteNormalizedCompositePNG preserves several independent material ramps in
// a comparison board. The 32-color production limit applies to each eventual
// sprite, not to a composite reference representing several different assets.
func WriteNormalizedCompositePNG(
	path string,
	data []byte,
	width, height int,
) ([]PaletteColor, error) {
	return writeNormalizedPNG(
		path,
		data,
		width,
		height,
		nil,
		false,
		false,
		CompositePaletteSize,
		false,
	)
}

// WriteReviewPreviewPNG writes an exact-size review artifact. Unlike
// production normalization, it may enlarge a small source because the result
// is display evidence only and can never become a deployment candidate.
func WriteReviewPreviewPNG(
	path string,
	data []byte,
	width, height int,
	locked []PaletteColor,
) ([]PaletteColor, error) {
	return writeNormalizedPNG(
		path,
		data,
		width,
		height,
		locked,
		false,
		true,
		defaultPaletteSize,
		false,
	)
}

func writeNormalizedPNG(
	path string,
	data []byte,
	width, height int,
	locked []PaletteColor,
	removeBackground, allowUpscale bool,
	paletteSize int,
	despeckleOpaqueTile bool,
) ([]PaletteColor, error) {
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
	normalized, err := normalizeImage(img, width, height, allowUpscale)
	if err != nil {
		return nil, err
	}
	palette := locked
	if len(palette) == 0 {
		palette = extractPalette(normalized, paletteSize)
	}
	normalized = applyPalette(normalized, palette)
	if despeckleOpaqueTile {
		mergeSmallOpaqueColorClustersToroidal(
			normalized,
			opaqueTileMicroClusterMaximumPixels,
			opaqueTileDespecklePasses,
		)
	}
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

func mergeSmallOpaqueColorClustersToroidal(img *image.NRGBA, maximumPixels, passes int) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 || maximumPixels < 1 || passes < 1 {
		return
	}
	directions := [...]image.Point{{X: -1}, {X: 1}, {Y: -1}, {Y: 1}}
	for pass := 0; pass < passes; pass++ {
		visited := make([]bool, width*height)
		replacements := make(map[image.Point]color.NRGBA)
		queue := make([]image.Point, 0, maximumPixels+1)
		cluster := make([]image.Point, 0, maximumPixels+1)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				index := (y-bounds.Min.Y)*width + x - bounds.Min.X
				if visited[index] {
					continue
				}
				value := img.NRGBAAt(x, y)
				visited[index] = true
				queue = append(queue[:0], image.Pt(x, y))
				cluster = cluster[:0]
				for len(queue) != 0 {
					point := queue[len(queue)-1]
					queue = queue[:len(queue)-1]
					cluster = append(cluster, point)
					for _, direction := range directions {
						next := wrapTilePoint(point.Add(direction), bounds)
						nextIndex := (next.Y-bounds.Min.Y)*width + next.X - bounds.Min.X
						if visited[nextIndex] || img.NRGBAAt(next.X, next.Y) != value {
							continue
						}
						visited[nextIndex] = true
						queue = append(queue, next)
					}
				}
				if value.A == 0 || len(cluster) > maximumPixels {
					continue
				}
				replacement, ok := dominantOpaqueBoundaryColor(img, cluster, value, directions)
				if !ok {
					continue
				}
				for _, point := range cluster {
					replacements[point] = replacement
				}
			}
		}
		if len(replacements) == 0 {
			return
		}
		for point, replacement := range replacements {
			img.SetNRGBA(point.X, point.Y, replacement)
		}
	}
}

func dominantOpaqueBoundaryColor(
	img *image.NRGBA,
	cluster []image.Point,
	value color.NRGBA,
	directions [4]image.Point,
) (color.NRGBA, bool) {
	counts := make(map[uint32]int)
	bounds := img.Bounds()
	for _, point := range cluster {
		for _, direction := range directions {
			neighbor := img.NRGBAAt(
				wrapTilePoint(point.Add(direction), bounds).X,
				wrapTilePoint(point.Add(direction), bounds).Y,
			)
			if neighbor.A == 0 || neighbor == value {
				continue
			}
			key := uint32(neighbor.R)<<16 | uint32(neighbor.G)<<8 | uint32(neighbor.B)
			counts[key]++
		}
	}
	bestKey, bestCount := uint32(0), 0
	for key, count := range counts {
		if count > bestCount || count == bestCount && key < bestKey {
			bestKey, bestCount = key, count
		}
	}
	if bestCount == 0 {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: uint8(bestKey >> 16), G: uint8(bestKey >> 8), B: uint8(bestKey), A: 255}, true
}

func wrapTilePoint(point image.Point, bounds image.Rectangle) image.Point {
	width, height := bounds.Dx(), bounds.Dy()
	x := (point.X-bounds.Min.X+width)%width + bounds.Min.X
	y := (point.Y-bounds.Min.Y+height)%height + bounds.Min.Y
	return image.Pt(x, y)
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

// HasRemovableEdgeBackground reports whether an isolated provider result
// already contains transparent reserve or has one flat edge background that
// the production chroma removal can actually clear.
func HasRemovableEdgeBackground(data []byte) (bool, error) {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("decode png: %w", err)
	}
	if imageHasTransparency(decoded) {
		return true, nil
	}
	return imageHasTransparency(removeEdgeBackground(decoded)), nil
}

func imageHasTransparency(img image.Image) bool {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A < 255 {
				return true
			}
		}
	}
	return false
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
		if colorDelta(corner, background) > backgroundExactDelta {
			return img
		}
	}
	// Clear the exact chroma key everywhere, including enclosed negative space
	// between limbs and equipment. The provider is explicitly required to use
	// a background color that is distinct from the subject.
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if colorDelta(img.NRGBAAt(x, y), background) <= backgroundExactDelta {
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	// Generated pixel art can contain a narrow antialiased chroma fringe even
	// around an otherwise flat background. Peel only background-hued pixels
	// touching already transparent background; unrelated interior colors stay.
	for pass := 0; pass < backgroundSpillPasses; pass++ {
		var clear []image.Point
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				value := img.NRGBAAt(x, y)
				if value.A == 0 || !isChromaSpill(value, background) || !touchesTransparency(img, image.Pt(x, y)) {
					continue
				}
				clear = append(clear, image.Pt(x, y))
			}
		}
		if len(clear) == 0 {
			break
		}
		for _, point := range clear {
			img.SetNRGBA(point.X, point.Y, color.NRGBA{})
		}
	}
	return img
}

func isChromaSpill(value, background color.NRGBA) bool {
	return colorDelta(value, background) <= backgroundSpillDelta &&
		chromaChannelExcess(value, background) >= backgroundSpillExcess
}

func chromaChannelExcess(value, background color.NRGBA) int {
	key := [...]int{int(background.R), int(background.G), int(background.B)}
	pixel := [...]int{int(value.R), int(value.G), int(value.B)}
	minimum, maximum := key[0], key[0]
	for _, channel := range key[1:] {
		minimum = min(minimum, channel)
		maximum = max(maximum, channel)
	}
	if maximum-minimum < backgroundExactDelta {
		return 0
	}
	keyThreshold := minimum + (maximum-minimum)/2
	minimumKey, maximumOther := 255, 0
	keyChannels, otherChannels := 0, 0
	for index, channel := range key {
		if channel >= keyThreshold {
			minimumKey = min(minimumKey, pixel[index])
			keyChannels++
			continue
		}
		maximumOther = max(maximumOther, pixel[index])
		otherChannels++
	}
	if keyChannels == 0 || otherChannels == 0 {
		return 0
	}
	return minimumKey - maximumOther
}

func touchesTransparency(img *image.NRGBA, point image.Point) bool {
	for y := point.Y - 1; y <= point.Y+1; y++ {
		for x := point.X - 1; x <= point.X+1; x++ {
			neighbor := image.Pt(x, y)
			if neighbor == point || !neighbor.In(img.Bounds()) {
				continue
			}
			if img.NRGBAAt(x, y).A == 0 {
				return true
			}
		}
	}
	return false
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

// SharedPaletteFromPNGs derives one deterministic palette from every supplied
// image so a complete unit uses the same colors across all animation boards.
func SharedPaletteFromPNGs(paths []string, limit int) ([]PaletteColor, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("shared palette requires at least one PNG")
	}
	counts := make(map[uint32]int)
	for _, path := range paths {
		img, err := decodeNRGBA(path)
		if err != nil {
			return nil, err
		}
		accumulatePaletteCounts(counts, img)
	}
	if limit <= 0 {
		limit = defaultPaletteSize
	}
	return paletteFromCounts(counts, limit), nil
}

// WriteDensityReducedPNG writes the exact logical-size review form of an
// intrinsic-density source. It scales the complete canvas deterministically;
// it never refits or recenters foreground content.
func WriteDensityReducedPNG(sourcePath, outputPath string, density int) error {
	if density < 1 {
		return fmt.Errorf("source density must be positive")
	}
	source, err := decodeNRGBA(sourcePath)
	if err != nil {
		return err
	}
	if source.Bounds().Dx()%density != 0 || source.Bounds().Dy()%density != 0 {
		return fmt.Errorf("source dimensions are not divisible by density %d", density)
	}
	destination := image.NewNRGBA(image.Rect(
		0,
		0,
		source.Bounds().Dx()/density,
		source.Bounds().Dy()/density,
	))
	areaScale(destination, destination.Bounds(), source, source.Bounds())
	return writePNG(outputPath, destination)
}

func normalizeImage(
	img image.Image,
	width, height int,
	allowUpscale bool,
) (*image.NRGBA, error) {
	bounds := img.Bounds()
	if !allowUpscale && (bounds.Dx() < width || bounds.Dy() < height) {
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
	accumulatePaletteCounts(counts, img)
	return paletteFromCounts(counts, limit)
}

func accumulatePaletteCounts(counts map[uint32]int, img *image.NRGBA) {
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
}

func paletteFromCounts(counts map[uint32]int, limit int) []PaletteColor {
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
