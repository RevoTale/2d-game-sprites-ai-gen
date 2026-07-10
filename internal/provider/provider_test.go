package provider_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestOpenAIUsesSquareProviderCanvasForSmallTargetSprites(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload struct {
			Size string `json:"size"`
		}
		require.NoError(t, json.Unmarshal(body, &payload))
		require.Equal(t, "1024x1024", payload.Size)
		return openAIImageResponse(t, 1024, 1024), nil
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
	require.Equal(t, "1024x1024", result.Metadata["providerSize"])
	require.NotEmpty(t, result.PNG)
}

func TestOpenAIUsesEditsEndpointWhenReferencesArePresent(t *testing.T) {
	referencePath := filepath.Join(t.TempDir(), "style.png")
	require.NoError(t, os.WriteFile(referencePath, testPNG(t, 8, 8), 0o644))
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/images/edits", req.URL.Path)
		require.True(t, strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data"))
		form, err := multipartReader(req)
		require.NoError(t, err)
		require.Equal(t, []string{"gpt-image-2"}, form.Value["model"])
		require.Equal(t, []string{"1024x1024"}, form.Value["size"])
		require.Equal(t, []string{"png"}, form.Value["output_format"])
		require.Len(t, form.File["image[]"], 1)
		require.Contains(t, form.Value["prompt"][0], "single 160x160 sprite")
		require.Contains(t, form.Value["prompt"][0], "Style anchor.")
		return openAIImageResponse(t, 1024, 1024), nil
	})
	openAI := provider.OpenAI{
		APIKey: "test",
		Client: &http.Client{Transport: transport},
	}

	result, err := openAI.Generate(context.Background(), provider.Request{
		Prompt: "single 160x160 sprite",
		Size:   image.Pt(160, 160),
		References: []provider.Reference{{
			Path:        referencePath,
			Description: "Style anchor.",
			Required:    true,
		}},
	})

	require.NoError(t, err)
	require.Equal(t, "edits", result.Metadata["endpoint"])
	require.NotEmpty(t, result.PNG)
}

func TestOpenAISupportsReferenceImages(t *testing.T) {
	require.True(t, provider.OpenAI{}.SupportsReferences())
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
