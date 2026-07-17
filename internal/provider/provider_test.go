package provider_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
		}
		require.NoError(t, json.Unmarshal(body, &payload))
		require.Equal(t, "1120x1120", payload.Size)
		require.Equal(t, "opaque", payload.Background)
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
	require.NotEmpty(t, result.PNG)
}

func TestOpenAIUsesEditsEndpointWhenReferencesArePresent(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "identity.png")
	posePath := filepath.Join(dir, "pose.png")
	referencePath := filepath.Join(dir, "style.png")
	maskPath := filepath.Join(t.TempDir(), "mask.png")
	require.NoError(t, os.WriteFile(identityPath, testPNG(t, 8, 8), 0o644))
	require.NoError(t, os.WriteFile(posePath, testPNG(t, 8, 8), 0o644))
	require.NoError(t, os.WriteFile(referencePath, testPNG(t, 8, 8), 0o644))
	require.NoError(t, os.WriteFile(maskPath, testPNG(t, 8, 8), 0o644))
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/images/edits", req.URL.Path)
		require.True(t, strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data"))
		form, err := multipartReader(req)
		require.NoError(t, err)
		require.Equal(t, []string{"gpt-image-2"}, form.Value["model"])
		require.Equal(t, []string{"1120x1120"}, form.Value["size"])
		require.Equal(t, []string{"png"}, form.Value["output_format"])
		require.Equal(t, []string{"opaque"}, form.Value["background"])
		require.Len(t, form.File["image[]"], 3)
		require.Equal(t, "identity.png", form.File["image[]"][0].Filename)
		require.Equal(t, "pose.png", form.File["image[]"][1].Filename)
		require.Equal(t, "style.png", form.File["image[]"][2].Filename)
		require.Len(t, form.File["mask"], 1)
		require.Equal(t, "image/png", form.File["image[]"][0].Header.Get("Content-Type"))
		require.Contains(t, form.Value["prompt"][0], "single 160x160 sprite")
		require.Contains(t, form.Value["prompt"][0], "Style anchor.")
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
			{Role: conditioning.RoleMask, Path: maskPath, Description: "Motion mask.", Required: true},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "edits", result.Metadata["endpoint"])
	require.NotEmpty(t, result.PNG)
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

func TestOpenAIReportsReferenceAndMaskCapabilities(t *testing.T) {
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

func TestSelectProviderUsesFakeOnlyWhenFakeFlagIsSet(t *testing.T) {
	selected, err := provider.Select(provider.SelectionOptions{Fake: true})

	require.NoError(t, err)
	require.IsType(t, provider.Fake{}, selected)
}

func TestSelectProviderRejectsFakeNameFromExplicitProvider(t *testing.T) {
	_, err := provider.Select(provider.SelectionOptions{ExplicitName: "fake"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "--fake")
}

func TestSelectProviderRejectsFakeNameFromEnvironment(t *testing.T) {
	_, err := provider.Select(provider.SelectionOptions{
		Env: map[string]string{
			provider.EnvProvider: "fake",
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "--fake")
}

func TestSelectProviderDetectsOpenAIFromAPIKey(t *testing.T) {
	selected, err := provider.Select(provider.SelectionOptions{
		Env: map[string]string{
			provider.EnvOpenAIAPIKey: "test-key",
		},
	})

	require.NoError(t, err)
	require.IsType(t, provider.OpenAI{}, selected)
}

func TestSelectProviderUsesOpenAIProviderEnvironmentWhenAPIKeyIsPresent(t *testing.T) {
	selected, err := provider.Select(provider.SelectionOptions{
		Env: map[string]string{
			provider.EnvProvider:     "openai",
			provider.EnvOpenAIAPIKey: "test-key",
		},
	})

	require.NoError(t, err)
	require.IsType(t, provider.OpenAI{}, selected)
}

func TestSelectProviderUsesEnvironmentModelOverride(t *testing.T) {
	selected, err := provider.Select(provider.SelectionOptions{Env: map[string]string{
		provider.EnvProvider:     "openai",
		provider.EnvOpenAIAPIKey: "test-key",
		provider.EnvOpenAIModel:  "gpt-image-custom",
	}})

	require.NoError(t, err)
	require.Equal(t, "gpt-image-custom", selected.(provider.OpenAI).Model)
}

func TestSelectProviderRequiresOpenAIAPIKeyWhenOpenAIIsExplicit(t *testing.T) {
	_, err := provider.Select(provider.SelectionOptions{ExplicitName: "openai", Env: map[string]string{}})

	require.Error(t, err)
	require.Contains(t, err.Error(), provider.EnvOpenAIAPIKey)
}

func TestSelectProviderRejectsFakeFlagWithExplicitProvider(t *testing.T) {
	_, err := provider.Select(provider.SelectionOptions{Fake: true, ExplicitName: "openai"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "--fake cannot be used with --provider")
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
