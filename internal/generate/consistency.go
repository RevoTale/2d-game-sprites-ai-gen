package generate

import (
	"errors"
	"os"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

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
