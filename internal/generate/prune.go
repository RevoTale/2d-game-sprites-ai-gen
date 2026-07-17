package generate

import (
	"os"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

// PruneRaw removes provider raw images while retaining normalized candidates,
// scoring evidence, selected output, and manifest lineage.
func PruneRaw(outputDir, runID string, selected []targets.Target) (int, error) {
	manifest, err := Load(outputDir, runID)
	if err != nil {
		return 0, err
	}
	wanted := make(map[string]bool, len(selected))
	for _, target := range selected {
		wanted[target.ID] = true
	}
	removed := 0
	for targetID, state := range manifest.Targets {
		if !wanted[targetID] {
			continue
		}
		count, err := pruneAttempts(state.Attempts)
		if err != nil {
			return removed, err
		}
		removed += count
	}
	for _, intermediate := range manifest.Intermediates {
		if !intermediateMatches(intermediate, wanted) {
			continue
		}
		count, err := pruneAttempts(intermediate.Attempts)
		if err != nil {
			return removed, err
		}
		removed += count
	}
	return removed, Save(outputDir, runID, manifest)
}

func intermediateMatches(intermediate *IntermediateState, wanted map[string]bool) bool {
	for _, targetID := range intermediate.TargetIDs {
		if wanted[targetID] {
			return true
		}
	}
	return len(intermediate.TargetIDs) == 0
}

func pruneAttempts(attempts []Attempt) (int, error) {
	removed := 0
	for attemptIndex := range attempts {
		if attempts[attemptIndex].RawPath != "" {
			count, err := removeIfPresent(attempts[attemptIndex].RawPath)
			if err != nil {
				return removed, err
			}
			removed += count
			attempts[attemptIndex].RawPath = ""
		}
		for candidateIndex := range attempts[attemptIndex].Candidates {
			path := attempts[attemptIndex].Candidates[candidateIndex].RawPath
			if path == "" {
				continue
			}
			count, err := removeIfPresent(path)
			if err != nil {
				return removed, err
			}
			removed += count
			attempts[attemptIndex].Candidates[candidateIndex].RawPath = ""
		}
	}
	return removed, nil
}

func removeIfPresent(path string) (int, error) {
	if err := os.Remove(path); err == nil {
		return 1, nil
	} else if os.IsNotExist(err) {
		return 0, nil
	} else {
		return 0, err
	}
}
