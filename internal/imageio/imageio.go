// Package imageio validates, normalizes, copies, and assembles PNG image artifacts.
package imageio

import (
	"bytes"
	"fmt"
	"image"
	stddraw "image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

func WriteNormalizedPNG(path string, data []byte, width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("normalized png size must be positive, got %dx%d", width, height)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}
	normalized, err := normalizeImage(img, width, height)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, normalized)
}

func normalizeImage(img image.Image, width, height int) (image.Image, error) {
	bounds := img.Bounds()
	if bounds.Dx() == width && bounds.Dy() == height {
		return img, nil
	}
	if bounds.Dx() < width || bounds.Dy() < height {
		return nil, fmt.Errorf("png dimensions are %dx%d, smaller than target %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	dstRect := aspectFitRect(bounds.Dx(), bounds.Dy(), width, height)
	xdraw.CatmullRom.Scale(dst, dstRect, img, bounds, xdraw.Src, nil)
	return dst, nil
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
	var imgs []image.Image
	totalW := 0
	height := 0
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
		totalW += bounds.Dx()
		imgs = append(imgs, img)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, totalW, height))
	x := 0
	for _, img := range imgs {
		bounds := img.Bounds()
		stddraw.Draw(dst, image.Rect(x, 0, x+bounds.Dx(), bounds.Dy()), img, bounds.Min, stddraw.Src)
		x += bounds.Dx()
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, dst)
}
