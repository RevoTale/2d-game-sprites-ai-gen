package generate

import (
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
)

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
