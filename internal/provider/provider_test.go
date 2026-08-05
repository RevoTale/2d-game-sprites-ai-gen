package provider_test

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
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestOpenAIUsesSquareProviderCanvasForSmallTargetSprites(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload struct {
			Size       string `json:"size"`
			Background string `json:"background"`
			Quality    string `json:"quality"`
		}
		require.NoError(t, json.Unmarshal(body, &payload))
		require.Equal(t, "1120x1120", payload.Size)
		require.Equal(t, "opaque", payload.Background)
		require.Equal(t, "high", payload.Quality)
		return openAIImageResponse(t, 1120, 1120), nil
	})
	openAI := provider.OpenAI{
		APIKey: "test",
		Client: &http.Client{Transport: transport},
	}

	result, err := openAI.Generate(context.Background(), provider.Request{
		Prompt: "single 160x160 sprite",
		Size:   image.Pt(160, 160),
	})

	require.NoError(t, err)
	require.Equal(t, "1120x1120", result.Metadata["providerSize"])
	require.Equal(t, "high", result.Metadata["quality"])
	require.NotEmpty(t, result.PNG)
}

func TestOpenAIRequestTimeoutBoundsStalledTransport(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	openAI := provider.OpenAI{
		APIKey:  "test",
		Timeout: 10 * time.Millisecond,
		Client:  &http.Client{Transport: transport},
	}

	_, err := openAI.Generate(context.Background(), provider.Request{Prompt: "sprite", Size: image.Pt(1024, 1024)})

	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded), "unexpected timeout error: %v", err)
}

func TestFakePrefersIdentityCanvasOverEditablePoseCanvas(t *testing.T) {
	dir := t.TempDir()
	posePath := filepath.Join(dir, "target.png")
	identityPath := filepath.Join(dir, "identity.png")
	pose := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	pose.SetNRGBA(1, 1, color.NRGBA{R: 220, A: 255})
	var posePNG bytes.Buffer
	require.NoError(t, png.Encode(&posePNG, pose))
	require.NoError(t, os.WriteFile(posePath, posePNG.Bytes(), 0o644))
	require.NoError(t, os.WriteFile(identityPath, testPNG(t, 8, 8), 0o644))

	result, err := (provider.Fake{}).Generate(context.Background(), provider.Request{
		Size: image.Pt(8, 8),
		Inputs: []conditioning.Input{
			{Role: conditioning.RolePose, Path: posePath},
			{Role: conditioning.RoleIdentity, Path: identityPath},
		},
	})

	require.NoError(t, err)
	decoded, err := png.Decode(bytes.NewReader(result.PNG))
	require.NoError(t, err)
	require.Equal(t, color.NRGBA{R: 120, G: 40, B: 160, A: 255}, color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA))
}

func TestOpenAIUsesEditsEndpointWhenReferencesArePresent(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "identity.png")
	posePath := filepath.Join(dir, "pose.png")
	referencePath := filepath.Join(dir, "style.png")
	require.NoError(t, os.WriteFile(identityPath, testPNG(t, 8, 8), 0o644))
	require.NoError(t, os.WriteFile(posePath, testPNG(t, 8, 8), 0o644))
	require.NoError(t, os.WriteFile(referencePath, testPNG(t, 8, 8), 0o644))
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/images/edits", req.URL.Path)
		require.True(t, strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data"))
		form, err := multipartReader(req)
		require.NoError(t, err)
		require.Equal(t, []string{"gpt-image-2"}, form.Value["model"])
		require.Equal(t, []string{"1120x1120"}, form.Value["size"])
		require.Equal(t, []string{"png"}, form.Value["output_format"])
		require.Equal(t, []string{"opaque"}, form.Value["background"])
		require.Equal(t, []string{"high"}, form.Value["quality"])
		require.Len(t, form.File["image[]"], 3)
		require.Equal(t, "identity.png", form.File["image[]"][0].Filename)
		require.Equal(t, "pose.png", form.File["image[]"][1].Filename)
		require.Equal(t, "style.png", form.File["image[]"][2].Filename)
		require.Empty(t, form.File["mask"])
		require.Equal(t, "image/png", form.File["image[]"][0].Header.Get("Content-Type"))
		require.Contains(t, form.Value["prompt"][0], "single 160x160 sprite")
		return openAIImageResponse(t, 1120, 1120), nil
	})
	openAI := provider.OpenAI{
		APIKey: "test",
		Client: &http.Client{Transport: transport},
	}

	result, err := openAI.Generate(context.Background(), provider.Request{
		Prompt: "single 160x160 sprite",
		Size:   image.Pt(160, 160),
		Inputs: []conditioning.Input{
			{Role: conditioning.RoleIdentity, Path: identityPath, Description: "Directional identity.", Required: true},
			{Role: conditioning.RolePose, Path: posePath, Description: "Exact pose.", Required: true},
			{Role: conditioning.RoleStyle, Path: referencePath, Description: "Style anchor.", Required: true},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "edits", result.Metadata["endpoint"])
	require.Equal(t, "high", result.Metadata["quality"])
	require.NotEmpty(t, result.PNG)
}

func TestOpenAISendsEditMaskAsDedicatedMultipartField(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "layout.png")
	maskPath := filepath.Join(dir, "mask.png")
	require.NoError(t, os.WriteFile(inputPath, testPNG(t, 1024, 1024), 0o644))
	mask := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for y := 256; y < 768; y++ {
		for x := 256; x < 768; x++ {
			mask.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	var encodedMask bytes.Buffer
	require.NoError(t, png.Encode(&encodedMask, mask))
	require.NoError(t, os.WriteFile(maskPath, encodedMask.Bytes(), 0o644))
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		form, err := multipartReader(req)
		require.NoError(t, err)
		require.Len(t, form.File["image[]"], 1)
		require.Len(t, form.File["mask"], 1)
		require.Equal(t, "mask.png", form.File["mask"][0].Filename)
		require.Equal(t, "image/png", form.File["mask"][0].Header.Get("Content-Type"))
		return openAIImageResponse(t, 1024, 1024), nil
	})

	_, err := (provider.OpenAI{
		APIKey: "test", Client: &http.Client{Transport: transport},
	}).Generate(context.Background(), provider.Request{
		Prompt: "complete masked board", Size: image.Pt(1024, 1024),
		Inputs:   []conditioning.Input{{Role: conditioning.RolePose, Path: inputPath}},
		MaskPath: maskPath,
	})

	require.NoError(t, err)
}

func TestOpenAIPreservesSupportedPortraitCanvasForFullUnitEdits(t *testing.T) {
	referencePath := filepath.Join(t.TempDir(), "unit-board.png")
	require.NoError(t, os.WriteFile(referencePath, testPNG(t, 8, 8), 0o644))
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		form, err := multipartReader(req)
		require.NoError(t, err)
		require.Equal(t, []string{"1024x1536"}, form.Value["size"])
		return openAIImageResponse(t, 1024, 1536), nil
	})
	openAI := provider.OpenAI{APIKey: "test", Client: &http.Client{Transport: transport}}

	result, err := openAI.Generate(context.Background(), provider.Request{
		Prompt: "full unit board",
		Size:   image.Pt(1024, 1536),
		Inputs: []conditioning.Input{{Role: conditioning.RoleIdentity, Path: referencePath}},
	})

	require.NoError(t, err)
	require.Equal(t, "1024x1536", result.Metadata["providerSize"])
}

func TestOpenAIReportsGenerationErrorBody(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/images/generations", req.URL.Path)
		return openAIErrorResponse(http.StatusBadRequest, `{"error":{"message":"unsupported generation size"}}`), nil
	})
	openAI := provider.OpenAI{
		APIKey: "test",
		Client: &http.Client{Transport: transport},
	}

	_, err := openAI.Generate(context.Background(), provider.Request{
		Prompt: "single 160x160 sprite",
		Size:   image.Pt(160, 160),
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "400 Bad Request")
	require.NotContains(t, err.Error(), "unsupported generation size")
}

func TestOpenAIReportsEditErrorBody(t *testing.T) {
	referencePath := filepath.Join(t.TempDir(), "style.png")
	require.NoError(t, os.WriteFile(referencePath, testPNG(t, 8, 8), 0o644))
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/images/edits", req.URL.Path)
		return openAIErrorResponse(http.StatusBadRequest, `{"error":{"message":"bad reference image field"}}`), nil
	})
	openAI := provider.OpenAI{
		APIKey: "test",
		Client: &http.Client{Transport: transport},
	}

	_, err := openAI.Generate(context.Background(), provider.Request{
		Prompt: "single 160x160 sprite",
		Size:   image.Pt(160, 160),
		Inputs: []conditioning.Input{{
			Role:        conditioning.RoleStyle,
			Path:        referencePath,
			Description: "Style anchor.",
			Required:    true,
		}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "400 Bad Request")
	require.NotContains(t, err.Error(), "bad reference image field")
}

func TestOpenAIReportsReferenceCapability(t *testing.T) {
	capabilities := provider.OpenAI{}.Capabilities()
	require.True(t, capabilities.References)
	require.True(t, capabilities.Masks)
}

func TestOpenAIRejectsResponseWithUnexpectedDimensions(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return openAIImageResponse(t, 1024, 1024), nil
	})
	openAI := provider.OpenAI{APIKey: "test", Client: &http.Client{Transport: transport}}

	_, err := openAI.Generate(context.Background(), provider.Request{Prompt: "sprite", Size: image.Pt(160, 160)})

	require.ErrorContains(t, err, "returned 1024x1024, expected 1120x1120")
}

func TestOpenAIFromEnvironmentRequiresAPIKey(t *testing.T) {
	_, err := provider.OpenAIFromEnvironment(map[string]string{})

	require.ErrorContains(t, err, "OPENAI_API_KEY is required")
}

func TestOpenAIFromEnvironmentCreatesOnlyOpenAIProvider(t *testing.T) {
	selected, err := provider.OpenAIFromEnvironment(map[string]string{
		provider.EnvOpenAIAPIKey: "test-key",
	})

	require.NoError(t, err)
	require.IsType(t, provider.OpenAI{}, selected)
}

func TestOpenAIFromEnvironmentUsesModelOverride(t *testing.T) {
	selected, err := provider.OpenAIFromEnvironment(map[string]string{
		provider.EnvOpenAIAPIKey: "test-key",
		provider.EnvOpenAIModel:  "gpt-image-custom",
	})

	require.NoError(t, err)
	require.Equal(t, "gpt-image-custom", selected.(provider.OpenAI).Model)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func openAIImageResponse(t *testing.T, width, height int) *http.Response {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 80, G: 120, B: 200, A: 255})
		}
	}
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	body := `{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(raw.Bytes()) + `"}]}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func openAIErrorResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func multipartReader(req *http.Request) (*multipart.Form, error) {
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		return nil, err
	}
	return req.MultipartForm, nil
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 120, G: 40, B: 160, A: 255})
		}
	}
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	return raw.Bytes()
}
