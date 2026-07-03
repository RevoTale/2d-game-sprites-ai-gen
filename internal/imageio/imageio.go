// Package imageio validates, normalizes, copies, and assembles PNG image artifacts.
package imageio

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func WriteNormalizedPNG(path string, data []byte, width, height int) error {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		return fmt.Errorf("png dimensions are %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
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
		draw.Draw(dst, image.Rect(x, 0, x+bounds.Dx(), bounds.Dy()), img, bounds.Min, draw.Src)
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
