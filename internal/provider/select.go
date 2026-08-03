package provider

import (
	"fmt"
	"os"
	"strings"
)

const (
	EnvOpenAIAPIKey = "OPENAI_API_KEY"
	EnvOpenAIModel  = "SPRITES_AI_GEN_OPENAI_MODEL"
)

// OpenAIFromEnvironment is the only production provider selection path.
// Automated tests inject Fake directly at their owning boundary.
func OpenAIFromEnvironment(env map[string]string) (Provider, error) {
	apiKey := strings.TrimSpace(environmentValue(env, EnvOpenAIAPIKey))
	if apiKey == "" {
		return nil, fmt.Errorf("%s is required for paid generation", EnvOpenAIAPIKey)
	}
	return OpenAI{
		APIKey: apiKey,
		Model:  strings.TrimSpace(environmentValue(env, EnvOpenAIModel)),
	}, nil
}

func environmentValue(env map[string]string, key string) string {
	if env != nil {
		return env[key]
	}
	return os.Getenv(key)
}
