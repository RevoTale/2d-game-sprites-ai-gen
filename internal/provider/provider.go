// Package provider defines image generation providers.
package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
)

type Reference struct {
	Path        string
	Description string
	Required    bool
}

type Request struct {
	Prompt     string
	Size       image.Point
	References []Reference
}

type Result struct {
	PNG      []byte
	Metadata map[string]string
}

type Provider interface {
	Generate(ctx context.Context, req Request) (Result, error)
	SupportsReferences() bool
}

type Fake struct {
	ReferenceSupport bool
}

func (f Fake) SupportsReferences() bool { return f.ReferenceSupport }

func (f Fake) Generate(_ context.Context, req Request) (Result, error) {
	if req.Size.X <= 0 || req.Size.Y <= 0 {
		return Result{}, errors.New("fake provider requires positive size")
	}
	img := image.NewNRGBA(image.Rect(0, 0, req.Size.X, req.Size.Y))
	fill := color.NRGBA{R: 120, G: 80, B: 180, A: 255}
	for y := 0; y < req.Size.Y; y++ {
		for x := 0; x < req.Size.X; x++ {
			if x == y || x+y == req.Size.X-1 {
				img.SetNRGBA(x, y, color.NRGBA{R: 240, G: 230, B: 120, A: 255})
				continue
			}
			img.SetNRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Result{}, err
	}
	return Result{PNG: buf.Bytes(), Metadata: map[string]string{"provider": "fake"}}, nil
}

type OpenAI struct {
	APIKey string
	Model  string
	Client *http.Client
}

const (
	openAIMinSquareSide = 1024
	openAISizeMultiple  = 16
	openAIMaxEdge       = 3840
	openAIMaxPixels     = 8294400
)

type openAIImageRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	Size         string `json:"size"`
	OutputFormat string `json:"output_format"`
}

func (o OpenAI) SupportsReferences() bool {
	// V1 uses the Image API generation endpoint. Reference-image editing can be added behind
	// the same provider interface without changing pack or target contracts.
	return false
}

func (o OpenAI) Generate(ctx context.Context, req Request) (Result, error) {
	apiKey := o.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return Result{}, errors.New("OPENAI_API_KEY is required")
	}
	if len(req.References) > 0 {
		return Result{}, errors.New("openai generation provider does not support reference images yet")
	}
	model := o.Model
	if model == "" {
		model = "gpt-image-2"
	}
	client := o.Client
	if client == nil {
		client = http.DefaultClient
	}
	providerSize, err := openAIProviderSize(req.Size)
	if err != nil {
		return Result{}, err
	}
	body := openAIImageRequest{Model: model, Prompt: req.Prompt, Size: fmt.Sprintf("%dx%d", providerSize.X, providerSize.Y), OutputFormat: "png"}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", bytes.NewReader(encoded))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("openai image generation failed: %s", resp.Status)
	}
	var decoded struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Result{}, err
	}
	if len(decoded.Data) == 0 || decoded.Data[0].B64JSON == "" {
		return Result{}, errors.New("openai image generation returned no image")
	}
	pngBytes, err := base64.StdEncoding.DecodeString(decoded.Data[0].B64JSON)
	if err != nil {
		return Result{}, err
	}
	return Result{PNG: pngBytes, Metadata: map[string]string{"provider": "openai", "model": model, "providerSize": fmt.Sprintf("%dx%d", providerSize.X, providerSize.Y)}}, nil
}

func openAIProviderSize(target image.Point) (image.Point, error) {
	if target.X <= 0 || target.Y <= 0 {
		return image.Point{}, errors.New("openai provider requires positive target size")
	}
	side := max(target.X, target.Y)
	if side < openAIMinSquareSide {
		side = openAIMinSquareSide
	}
	side = roundUpToMultiple(side, openAISizeMultiple)
	if side > openAIMaxEdge || side*side > openAIMaxPixels {
		return image.Point{}, fmt.Errorf("target %dx%d needs provider canvas %dx%d, which exceeds OpenAI image size constraints", target.X, target.Y, side, side)
	}
	return image.Pt(side, side), nil
}

func roundUpToMultiple(value, multiple int) int {
	if value%multiple == 0 {
		return value
	}
	return value + multiple - value%multiple
}
