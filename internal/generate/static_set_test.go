package generate

import (
	"strings"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

func TestStaticSetPromptRequiresTransparentPartPerimeters(t *testing.T) {
	t.Parallel()

	shared := targets.Target{
		RenderMode: pack.RenderModeTransparentOverlay,
	}
	group := []targets.Target{
		{SetPartID: "moss-a", SetPartDesc: "One moss patch."},
		{SetPartID: "moss-b", SetPartDesc: "Another moss patch."},
	}

	prompt := staticSetPrompt(shared, group)

	if !strings.Contains(prompt, "uninterrupted chroma perimeter around all four edges of every part") {
		t.Fatal("transparent-overlay prompt does not require a removable perimeter for every part")
	}
}
