package provider

import (
	"fmt"
	"os"
	"strings"
)

const (
	EnvProvider     = "SPRITES_AI_GEN_PROVIDER"
	EnvOpenAIAPIKey = "OPENAI_API_KEY"
	EnvOpenAIModel  = "SPRITES_AI_GEN_OPENAI_MODEL"

	NameOpenAI = "openai"
)

type SelectionOptions struct {
	ExplicitName string
	Fake         bool
	Env          map[string]string
}

func Select(opts SelectionOptions) (Provider, error) {
	if opts.Fake {
		if strings.TrimSpace(opts.ExplicitName) != "" {
			return nil, fmt.Errorf("--fake cannot be used with --provider")
		}
		return Fake{}, nil
	}
	if name := strings.TrimSpace(opts.ExplicitName); name != "" {
		return selectNamedProvider(name, opts)
	}
	if name := strings.TrimSpace(envValue(opts, EnvProvider)); name != "" {
		return selectNamedProvider(name, opts)
	}
	if apiKey := strings.TrimSpace(envValue(opts, EnvOpenAIAPIKey)); apiKey != "" {
		return OpenAI{APIKey: apiKey, Model: strings.TrimSpace(envValue(opts, EnvOpenAIModel))}, nil
	}
	return nil, fmt.Errorf("provider is required: pass --provider %s, set %s, configure %s, or pass --fake for deterministic tests", NameOpenAI, EnvProvider, EnvOpenAIAPIKey)
}

func selectNamedProvider(name string, opts SelectionOptions) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case NameOpenAI:
		apiKey := strings.TrimSpace(envValue(opts, EnvOpenAIAPIKey))
		if apiKey == "" {
			return nil, fmt.Errorf("%s is required for provider %q", EnvOpenAIAPIKey, NameOpenAI)
		}
		return OpenAI{APIKey: apiKey, Model: strings.TrimSpace(envValue(opts, EnvOpenAIModel))}, nil
	case "fake":
		return nil, fmt.Errorf("fake provider is only available through --fake")
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}

func envValue(opts SelectionOptions, key string) string {
	if opts.Env != nil {
		return opts.Env[key]
	}
	return os.Getenv(key)
}
