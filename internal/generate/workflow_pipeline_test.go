package generate

import (
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/imageio"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
	"github.com/stretchr/testify/require"
)

func TestSeedBoardPromptNamesEveryFixedCellAndExplicitlyForbidsTrailingCell(t *testing.T) {
	layout, err := imageio.CanvasGridLayout(3, 4, providerCanvasSize)
	require.NoError(t, err)
	plan := seedPlan{
		References: []targets.Target{
			{ObjectDesc: "knight", Variants: []targets.VariantSelection{{AxisID: "direction", ValueID: "down", Description: "Front/down view."}}},
			{ObjectDesc: "knight", Variants: []targets.VariantSelection{{AxisID: "direction", ValueID: "up", Description: "Back/up view; no front-facing face."}}},
			{ObjectDesc: "knight", Variants: []targets.VariantSelection{{AxisID: "direction", ValueID: "right", Description: "Right-facing side view that attacks toward canvas right."}}},
		},
		VariantKey: []string{"direction-down", "direction-up", "direction-right"},
	}

	prompt := seedBoardPrompt(plan, layout)

	require.Contains(t, prompt, "Expected cells use row-major order")
	require.Contains(t, prompt, "Treat every configured variant description below as a semantic requirement")
	require.Contains(t, prompt, "1. row 1, column 1 (top-left) — direction-down: knight")
	require.Contains(t, prompt, "Variant direction=down: Front/down view.")
	require.Contains(t, prompt, "2. row 1, column 2 (top-right) — direction-up: knight")
	require.Contains(t, prompt, "Variant direction=up: Back/up view; no front-facing face.")
	require.Contains(t, prompt, "3. row 2, column 1 (bottom-left) — direction-right: knight")
	require.Contains(t, prompt, "Variant direction=right: Right-facing side view that attacks toward canvas right.")
	require.Contains(t, prompt, "Unused cell: row 2, column 2 (bottom-right). It must remain completely empty")
}

func TestAnimationRowPromptMapsEveryFrameToItsExactFixedColumn(t *testing.T) {
	layout, err := imageio.CanvasGridLayout(4, 4, providerCanvasSize)
	require.NoError(t, err)
	plan := rowPlan{
		AnimationID: "walk",
		Targets: []targets.Target{
			{
				ObjectDesc:    "Bright knight with sword and shield.",
				AnimationDesc: "Four-beat walk with grounded weight transfer.",
				FrameID:       "00",
				FrameDesc:     "Ready stance.",
				Variants: []targets.VariantSelection{{
					AxisID: "direction", ValueID: "right", Description: "Right-facing side view that moves toward canvas right.",
				}},
			},
			{FrameID: "01", FrameDesc: "Forward step."},
			{FrameID: "02", FrameDesc: "Passing pose."},
			{FrameID: "03", FrameDesc: "Recovery stance."},
		},
	}

	prompt := animationRowPrompt(plan, layout, "02")

	require.Contains(t, prompt, "# Object requirement\nBright knight with sword and shield.")
	require.Contains(t, prompt, "# Variant requirement: direction=right\nRight-facing side view that moves toward canvas right.")
	require.Contains(t, prompt, "# Animation requirement: walk\nFour-beat walk with grounded weight transfer.")
	require.Contains(t, prompt, "Expected cells use left-to-right fixed order")
	require.Contains(t, prompt, "1. row 1, column 1 — 00: Ready stance.")
	require.Contains(t, prompt, "2. row 1, column 2 — 01: Forward step.")
	require.Contains(t, prompt, "3. row 1, column 3 — 02: Passing pose.")
	require.Contains(t, prompt, "4. row 1, column 4 — 03: Recovery stance.")
	require.Contains(t, prompt, "Correct only frame 02 inside row 1, column 3")
}

func TestCandidateEvidenceSeparatesEligibleInvalidAndMechanicallyPreferredCandidates(t *testing.T) {
	state := &IntermediateState{Attempts: []Attempt{{
		SelectedCandidate: "02",
		Candidates: []Candidate{
			{ID: "01", HardRejections: []string{"board_trailing_cell_occupied"}},
			{ID: "02"},
			{ID: "03"},
		},
	}}}

	evidence := CandidateEvidence(state)

	require.Equal(t, []string{"02", "03"}, evidence.Eligible)
	require.Equal(t, []string{"01"}, evidence.Invalid)
	require.Equal(t, "02", evidence.MechanicallyPreferred)
}

func TestCandidateReviewSummaryExplainsEligibilityRejectionsAndManualReview(t *testing.T) {
	state := &IntermediateState{Attempts: []Attempt{{
		SelectedCandidate: "02",
		Candidates: []Candidate{
			{ID: "01", HardRejections: []string{"board_trailing_cell_occupied", "cell_01_foreground_components_0"}},
			{ID: "02"},
			{ID: "03"},
		},
	}}}

	summary := CandidateReviewSummary(state)

	require.Equal(t, "Eligible candidates: 02, 03. Mechanically preferred candidate: 02. Invalid candidates: 01 (board_trailing_cell_occupied, cell_01_foreground_components_0). Manual visual review is still required.", summary)
}

func TestCandidateReviewSummaryRequiresRegenerationWhenEveryCandidateIsInvalid(t *testing.T) {
	state := &IntermediateState{Attempts: []Attempt{{
		Candidates: []Candidate{
			{ID: "01", HardRejections: []string{"board_trailing_cell_occupied"}},
			{ID: "02", HardRejections: []string{"cell_00_foreground_components_0"}},
		},
	}}}

	summary := CandidateReviewSummary(state)

	require.Equal(t, "Eligible candidates: none. Mechanically preferred candidate: none. Invalid candidates: 01 (board_trailing_cell_occupied), 02 (cell_00_foreground_components_0). No candidate can be reviewed; start a forced generation attempt after correcting the prompt or references.", summary)
}

func TestCandidateEvidenceReplacesStalePreferenceWithBestEligibleCandidate(t *testing.T) {
	state := &IntermediateState{Attempts: []Attempt{{
		SelectedCandidate: "01",
		Candidates: []Candidate{
			{ID: "01", Metrics: imageio.Metrics{Score: 100}, HardRejections: []string{"board_trailing_cell_occupied"}},
			{ID: "02", Metrics: imageio.Metrics{Score: 20}},
			{ID: "03", Metrics: imageio.Metrics{Score: 10}},
		},
	}}}

	evidence := CandidateEvidence(state)

	require.Equal(t, "02", evidence.MechanicallyPreferred)
}
