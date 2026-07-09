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
	"net/http"
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
