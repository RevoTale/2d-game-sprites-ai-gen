package generate

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

func resolvePoseGuide(target targets.Target, deployDir string) (string, error) {
	if path, err := existingDeployPath(target, deployDir); err != nil {
		return "", err
	} else if path != "" {
		return path, nil
	}
	for index := len(target.Inputs) - 1; index >= 0; index-- {
		if target.Inputs[index].Role == conditioning.RolePose {
			if _, err := os.Stat(target.Inputs[index].Path); err != nil {
				return "", fmt.Errorf("animated target %q pose reference %q: %w", target.ID, target.Inputs[index].Path, err)
			}
			return target.Inputs[index].Path, nil
		}
	}
	return "", fmt.Errorf("animated target %q requires an existing deployed frame or variant, animation, or frame reference as its pose guide", target.ID)
}

func existingDeployPath(target targets.Target, deployDir string) (string, error) {
	if deployDir == "" {
		return "", nil
	}
	path, err := targets.DeployPath(deployDir, target)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return "", nil
}

func filterInputs(inputs []conditioning.Input, roles ...conditioning.Role) []conditioning.Input {
	allowed := map[conditioning.Role]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	var out []conditioning.Input
	for _, input := range inputs {
		if allowed[input.Role] {
			out = append(out, input)
		}
	}
	return out
}

func directionKey(target targets.Target) string {
	var key strings.Builder
	key.WriteString(target.ObjectID)
	for _, variant := range target.Variants {
		key.WriteByte(0)
		key.WriteString(variant.AxisID)
		key.WriteByte('=')
		key.WriteString(variant.ValueID)
	}
	return key.String()
}

func safeVariantKey(target targets.Target) string {
	parts := make([]string, 0, len(target.Variants))
	for _, variant := range target.Variants {
		parts = append(parts, variant.AxisID+"-"+variant.ValueID)
	}
	if len(parts) == 0 {
		return "default"
	}
	return strings.Join(parts, "__")
}
