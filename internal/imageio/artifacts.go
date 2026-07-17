package imageio

import (
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
)

const candidateReviewHeaderHeight = 40

var reviewGlyphs = map[rune][7]uint8{
	'0': {14, 17, 19, 21, 25, 17, 14},
	'1': {4, 12, 4, 4, 4, 4, 14},
	'2': {14, 17, 1, 2, 4, 8, 31},
	'3': {30, 1, 1, 14, 1, 1, 30},
	'4': {2, 6, 10, 18, 31, 2, 2},
	'5': {31, 16, 16, 30, 1, 1, 30},
	'6': {14, 16, 16, 30, 17, 17, 14},
	'7': {31, 1, 2, 4, 8, 8, 8},
	'8': {14, 17, 17, 14, 17, 17, 14},
	'9': {14, 17, 17, 15, 1, 1, 14},
	'A': {14, 17, 17, 31, 17, 17, 17},
	'C': {14, 17, 16, 16, 16, 17, 14},
	'D': {30, 17, 17, 17, 17, 17, 30},
	'E': {31, 16, 16, 30, 16, 16, 31},
	'I': {31, 4, 4, 4, 4, 4, 31},
	'L': {16, 16, 16, 16, 16, 16, 31},
	'N': {17, 25, 21, 19, 17, 17, 17},
	'T': {31, 4, 4, 4, 4, 4, 4},
	'V': {17, 17, 17, 17, 17, 10, 4},
}

// CandidateReviewTile describes one fixed-board candidate and its mechanical
// eligibility. Visual quality remains a human decision.
type CandidateReviewTile struct {
	ID    string
	Path  string
	Valid bool
}

func decodeNRGBA(path string) (*image.NRGBA, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		return nil, err
	}
	dst := image.NewNRGBA(img.Bounds())
	draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
	return dst, nil
}

func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func AssembleHorizontalSheet(paths []string, out string) error {
	if len(paths) == 0 {
		return fmt.Errorf("sheet requires at least one source image")
	}
	var images []image.Image
	totalWidth, height := 0, 0
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			return err
		}
		bounds := img.Bounds()
		if height != 0 && bounds.Dy() != height {
			return fmt.Errorf("sheet source %q height mismatch", path)
		}
		height = bounds.Dy()
		totalWidth += bounds.Dx()
		images = append(images, img)
	}
	destination := image.NewNRGBA(image.Rect(0, 0, totalWidth, height))
	x := 0
	for _, img := range images {
		bounds := img.Bounds()
		draw.Draw(destination, image.Rect(x, 0, x+bounds.Dx(), bounds.Dy()), img, bounds.Min, draw.Src)
		x += bounds.Dx()
	}
	return writePNG(out, destination)
}

// WriteNearestNeighborContactSheet enlarges native sprite frames without
// interpolation so pixel clusters remain readable during visual review.
func WriteNearestNeighborContactSheet(paths []string, out string, scale int) error {
	if scale < 1 {
		return fmt.Errorf("contact sheet scale must be positive")
	}
	images, size, err := decodeSameSize(paths)
	if err != nil {
		return err
	}
	destination := image.NewNRGBA(image.Rect(0, 0, size.X*scale*len(images), size.Y*scale))
	for index, source := range images {
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				value := color.NRGBAModel.Convert(source.At(source.Bounds().Min.X+x, source.Bounds().Min.Y+y)).(color.NRGBA)
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						destination.SetNRGBA((index*size.X+x)*scale+dx, y*scale+dy, value)
					}
				}
			}
		}
	}
	return writePNG(out, destination)
}

// WriteCandidateReviewSheet preserves each candidate board pixel-for-pixel
// below a labeled validity header so reviewers cannot confuse visual appeal
// with mechanical eligibility.
func WriteCandidateReviewSheet(tiles []CandidateReviewTile, out string) error {
	if len(tiles) == 0 {
		return fmt.Errorf("candidate review sheet requires at least one tile")
	}
	paths := make([]string, len(tiles))
	for index, tile := range tiles {
		paths[index] = tile.Path
	}
	images, size, err := decodeSameSize(paths)
	if err != nil {
		return err
	}
	destination := image.NewNRGBA(image.Rect(0, 0, size.X*len(images), size.Y+candidateReviewHeaderHeight))
	for index, source := range images {
		left := index * size.X
		validity, header := "INVALID", color.NRGBA{R: 165, G: 36, B: 36, A: 255}
		if tiles[index].Valid {
			validity, header = "VALID", color.NRGBA{R: 30, G: 122, B: 70, A: 255}
		}
		draw.Draw(destination, image.Rect(left, 0, left+size.X, candidateReviewHeaderHeight), &image.Uniform{C: header}, image.Point{}, draw.Src)
		drawPixelLabel(destination, left+12, 9, "CANDIDATE "+tiles[index].ID+" "+validity)
		draw.Draw(destination, image.Rect(left, candidateReviewHeaderHeight, left+size.X, candidateReviewHeaderHeight+size.Y), source, source.Bounds().Min, draw.Src)
	}
	return writePNG(out, destination)
}

func drawPixelLabel(destination *image.NRGBA, left, top int, label string) {
	const scale = 3
	white := &image.Uniform{C: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
	for _, character := range label {
		glyph, ok := reviewGlyphs[character]
		if ok {
			for row, bits := range glyph {
				for column := 0; column < 5; column++ {
					if bits&(1<<uint(4-column)) == 0 {
						continue
					}
					x, y := left+column*scale, top+row*scale
					draw.Draw(destination, image.Rect(x, y, x+scale, y+scale), white, image.Point{}, draw.Src)
				}
			}
		}
		left += 6 * scale
	}
}

// WriteLoopingGIF writes native-size review animation without changing source
// PNGs. Delay is expressed in GIF centiseconds.
func WriteLoopingGIF(paths []string, out string, delay int) error {
	if delay < 1 {
		return fmt.Errorf("GIF delay must be positive")
	}
	images, size, err := decodeSameSize(paths)
	if err != nil {
		return err
	}
	colors := make(color.Palette, 0, 256)
	colors = append(colors, color.NRGBA{})
	colors = append(colors, palette.Plan9[:255]...)
	animation := gif.GIF{LoopCount: 0}
	for _, source := range images {
		frame := image.NewPaletted(image.Rect(0, 0, size.X, size.Y), colors)
		draw.Draw(frame, frame.Bounds(), source, source.Bounds().Min, draw.Src)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, delay)
		animation.Disposal = append(animation.Disposal, gif.DisposalBackground)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := gif.EncodeAll(file, &animation); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func decodeSameSize(paths []string) ([]image.Image, image.Point, error) {
	if len(paths) == 0 {
		return nil, image.Point{}, fmt.Errorf("review artifact requires at least one source image")
	}
	images := make([]image.Image, 0, len(paths))
	var size image.Point
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, image.Point{}, err
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			return nil, image.Point{}, err
		}
		if size == (image.Point{}) {
			size = img.Bounds().Size()
		} else if img.Bounds().Size() != size {
			return nil, image.Point{}, fmt.Errorf("review artifact source %q dimensions differ", path)
		}
		images = append(images, img)
	}
	return images, size, nil
}
