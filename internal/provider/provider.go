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
	"image/draw"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
)

type Capabilities struct {
	References bool
	Progress   bool
}

type Request struct {
	Prompt           string
	Size             image.Point
	Inputs           []conditioning.Input
	CandidateOrdinal int
	Progress         func(current, total int)
}

type Result struct {
	PNG      []byte
	Metadata map[string]string
}

type Provider interface {
	Generate(ctx context.Context, req Request) (Result, error)
	Capabilities() Capabilities
}

type Fake struct {
	CapabilitiesValue Capabilities
}

func (f Fake) Capabilities() Capabilities {
	if f.CapabilitiesValue == (Capabilities{}) {
		return Capabilities{References: true, Progress: true}
	}
	return f.CapabilitiesValue
}

func (f Fake) Generate(_ context.Context, req Request) (Result, error) {
	if req.Size.X <= 0 || req.Size.Y <= 0 {
		return Result{}, errors.New("fake provider requires positive size")
	}
	for _, role := range []conditioning.Role{conditioning.RoleIdentity, conditioning.RolePose} {
		for _, input := range req.Inputs {
			if input.Role != role {
				continue
			}
			data, err := os.ReadFile(input.Path)
			if err != nil {
				return Result{}, err
			}
			config, err := png.DecodeConfig(bytes.NewReader(data))
			if err == nil && config.Width == req.Size.X && config.Height == req.Size.Y {
				decoded, decodeErr := png.Decode(bytes.NewReader(data))
				if decodeErr == nil && imageHasForeground(decoded) {
					return Result{PNG: data, Metadata: map[string]string{"provider": "fake", "source": "canvas-reference"}}, nil
				}
			}
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, req.Size.X, req.Size.Y))
	fill := color.NRGBA{R: 120, G: 80, B: 180, A: 255}
	if strings.Contains(strings.ToLower(req.Prompt), "full-bleed opaque") {
		draw.Draw(img, img.Bounds(), &image.Uniform{C: fill}, image.Point{}, draw.Src)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return Result{}, err
		}
		return Result{
			PNG:      buf.Bytes(),
			Metadata: map[string]string{"provider": "fake", "source": "opaque-output"},
		}, nil
	}
	marginX := max(2, req.Size.X/8)
	marginY := max(2, req.Size.Y/8)
	for y := marginY; y < req.Size.Y-marginY; y++ {
		for x := marginX; x < req.Size.X-marginX; x++ {
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

func imageHasForeground(img image.Image) bool {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A != 0 {
				return true
			}
		}
	}
	return false
}

type OpenAI struct {
	APIKey  string
	Model   string
	Client  *http.Client
	Timeout time.Duration
}

const (
	openAIMinSquareSide  = 1024
	openAISizeMultiple   = 16
	openAIMaxEdge        = 3840
	openAIMinPixels      = 655360
	openAIMaxPixels      = 8294400
	openAIMaxAspect      = 3
	defaultOpenAITimeout = 10 * time.Minute
	defaultOpenAIQuality = "high"
)

type openAIImageRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	Size         string `json:"size"`
	OutputFormat string `json:"output_format"`
	Background   string `json:"background"`
	Quality      string `json:"quality"`
}

func (o OpenAI) Capabilities() Capabilities {
	return Capabilities{References: true}
}

func (o OpenAI) Generate(ctx context.Context, req Request) (Result, error) {
	apiKey := o.APIKey
	if apiKey == "" {
		apiKey = os.Getenv(EnvOpenAIAPIKey)
	}
	if apiKey == "" {
		return Result{}, fmt.Errorf("%s is required", EnvOpenAIAPIKey)
	}
	model := o.Model
	if model == "" {
		model = "gpt-image-2"
	}
	client := openAIHTTPClient(o.Client, o.Timeout)
	providerSize, err := openAIProviderSize(req.Size)
	if err != nil {
		return Result{}, err
	}
	if len(req.Inputs) > 0 {
		return o.generateEdit(ctx, client, apiKey, model, req, providerSize)
	}
	return o.generateFromPrompt(ctx, client, apiKey, model, req, providerSize)
}

func openAIHTTPClient(base *http.Client, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultOpenAITimeout
	}
	if base == nil {
		return &http.Client{Timeout: timeout}
	}
	if base.Timeout > 0 {
		return base
	}
	copy := *base
	copy.Timeout = timeout
	return &copy
}

func (o OpenAI) generateFromPrompt(ctx context.Context, client *http.Client, apiKey, model string, req Request, providerSize image.Point) (Result, error) {
	body := openAIImageRequest{
		Model: model, Prompt: req.Prompt,
		Size:         fmt.Sprintf("%dx%d", providerSize.X, providerSize.Y),
		OutputFormat: "png", Background: "opaque",
		Quality: defaultOpenAIQuality,
	}
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
		return Result{}, openAIHTTPError("generation", resp)
	}
	pngBytes, err := decodeImageResponse(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if err := validateProviderPNG(pngBytes, providerSize); err != nil {
		return Result{}, err
	}
	return Result{PNG: pngBytes, Metadata: map[string]string{
		"provider": "openai", "model": model,
		"providerSize": fmt.Sprintf("%dx%d", providerSize.X, providerSize.Y),
		"quality":      defaultOpenAIQuality,
		"endpoint":     "generations",
	}}, nil
}

func (o OpenAI) generateEdit(ctx context.Context, client *http.Client, apiKey, model string, req Request, providerSize image.Point) (Result, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	// GPT Image 2 always processes references at high fidelity and rejects an
	// input_fidelity override. Input order is therefore the provider contract.
	if err := writer.WriteField("model", model); err != nil {
		return Result{}, err
	}
	if err := writer.WriteField("prompt", req.Prompt); err != nil {
		return Result{}, err
	}
	if err := writer.WriteField("size", fmt.Sprintf("%dx%d", providerSize.X, providerSize.Y)); err != nil {
		return Result{}, err
	}
	if err := writer.WriteField("output_format", "png"); err != nil {
		return Result{}, err
	}
	if err := writer.WriteField("background", "opaque"); err != nil {
		return Result{}, err
	}
	if err := writer.WriteField("quality", defaultOpenAIQuality); err != nil {
		return Result{}, err
	}
	for i := range req.Inputs {
		input := &req.Inputs[i]
		if err := addInputImage(writer, "image[]", *input); err != nil {
			return Result{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/edits", &body)
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, openAIHTTPError("edit", resp)
	}
	pngBytes, err := decodeImageResponse(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if err := validateProviderPNG(pngBytes, providerSize); err != nil {
		return Result{}, err
	}
	return Result{PNG: pngBytes, Metadata: map[string]string{
		"provider": "openai", "model": model,
		"providerSize": fmt.Sprintf("%dx%d", providerSize.X, providerSize.Y),
		"quality":      defaultOpenAIQuality,
		"endpoint":     "edits",
	}}, nil
}

func validateProviderPNG(data []byte, expected image.Point) error {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode provider PNG: %w", err)
	}
	actual := decoded.Bounds().Size()
	if actual != expected {
		return fmt.Errorf("provider returned %dx%d, expected %dx%d", actual.X, actual.Y, expected.X, expected.Y)
	}
	return nil
}

func addInputImage(writer *multipart.Writer, field string, input conditioning.Input) error {
	file, err := os.Open(input.Path)
	if err != nil {
		return fmt.Errorf("open generation input %q: %w", input.Path, err)
	}
	defer file.Close()
	contentType, err := referenceContentType(file)
	if err != nil {
		return err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     field,
		"filename": filepath.Base(input.Path),
	}))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy generation input %q: %w", input.Path, err)
	}
	return nil
}

func referenceContentType(file *os.File) (string, error) {
	var header [512]byte
	n, err := file.Read(header[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read reference %q header: %w", file.Name(), err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind reference %q: %w", file.Name(), err)
	}
	if detected := http.DetectContentType(header[:n]); isOpenAIReferenceMIME(detected) {
		return detected, nil
	}
	if fromExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(file.Name()))); isOpenAIReferenceMIME(fromExt) {
		return fromExt, nil
	}
	return "", fmt.Errorf("reference %q must be PNG, JPEG, or WebP", file.Name())
}

func isOpenAIReferenceMIME(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func decodeImageResponse(body io.Reader) ([]byte, error) {
	var decoded struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) == 0 || decoded.Data[0].B64JSON == "" {
		return nil, errors.New("openai image generation returned no image")
	}
	pngBytes, err := base64.StdEncoding.DecodeString(decoded.Data[0].B64JSON)
	if err != nil {
		return nil, err
	}
	return pngBytes, nil
}

func openAIHTTPError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var payload struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)
	category := payload.Error.Code
	if category == "" {
		category = payload.Error.Type
	}
	if category != "" {
		return fmt.Errorf("openai image %s failed: %s (%s)", operation, resp.Status, category)
	}
	return fmt.Errorf("openai image %s failed: %s", operation, resp.Status)
}

func openAIProviderSize(target image.Point) (image.Point, error) {
	if target.X <= 0 || target.Y <= 0 {
		return image.Point{}, errors.New("openai provider requires positive target size")
	}
	rounded := image.Pt(roundUpToMultiple(target.X, openAISizeMultiple), roundUpToMultiple(target.Y, openAISizeMultiple))
	if validOpenAIProviderSize(rounded) {
		return rounded, nil
	}
	side := max(target.X, target.Y)
	if side < openAIMinSquareSide {
		for multiplier := 1; ; multiplier++ {
			candidate := side * multiplier
			if candidate < openAIMinSquareSide || candidate%openAISizeMultiple != 0 {
				continue
			}
			if candidate <= openAIMaxEdge && candidate*candidate <= openAIMaxPixels {
				side = candidate
				break
			}
			side = openAIMinSquareSide
			break
		}
	}
	side = roundUpToMultiple(side, openAISizeMultiple)
	if side > openAIMaxEdge || side*side > openAIMaxPixels {
		return image.Point{}, fmt.Errorf("target %dx%d cannot satisfy OpenAI image size constraints", target.X, target.Y)
	}
	return image.Pt(side, side), nil
}

func validOpenAIProviderSize(size image.Point) bool {
	if size.X > openAIMaxEdge || size.Y > openAIMaxEdge {
		return false
	}
	pixels := size.X * size.Y
	if pixels < openAIMinPixels || pixels > openAIMaxPixels {
		return false
	}
	short, long := min(size.X, size.Y), max(size.X, size.Y)
	return long <= short*openAIMaxAspect
}

func roundUpToMultiple(value, multiple int) int {
	if value%multiple == 0 {
		return value
	}
	return value + multiple - value%multiple
}
