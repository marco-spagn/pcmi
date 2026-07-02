package worker

import (
	"fmt"
	"log"
	"strings"

	"github.com/marco-spagn/pcmi/internal/config"
)

// Default model names per provider when DISTILLATION_MODEL is not set.
const (
	defaultModelOpenAI   = "gpt-4o-mini"
	defaultModelGrok     = "grok-3-mini"
	defaultModelAnthropic = "claude-haiku-4-5-20251001"
	defaultModelDeepSeek = "deepseek-chat"
)

// NewLLMClient reads the provider selection and credentials from cfg and
// returns the appropriate LLMClient.  Returns an error only for unsupported
// provider names; a missing API key is not an error here — IsConfigured()
// will return false and the distillation worker will skip LLM calls at
// runtime, matching the existing "no OPENAI_API_KEY" behaviour.
func NewLLMClient(cfg *config.Config) (LLMClient, error) {
	if cfg == nil {
		return newOpenAIClient("", defaultModelOpenAI, ""), nil
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.LLMProvider))
	if provider == "" {
		provider = "openai"
	}

	model := strings.TrimSpace(cfg.DistillationModel)

	switch provider {
	case "openai":
		if model == "" {
			model = defaultModelOpenAI
		}
		if cfg.OpenAIAPIKey == "" {
			log.Println("⚠️  LLM_PROVIDER=openai but OPENAI_API_KEY is unset — distillation will be skipped")
		}
		return newOpenAIClient(cfg.OpenAIAPIKey, model, cfg.OpenAIBaseURL), nil

	case "grok", "xai":
		if model == "" {
			model = defaultModelGrok
		}
		if cfg.GrokAPIKey == "" {
			log.Println("⚠️  LLM_PROVIDER=grok but GROK_API_KEY is unset — distillation will be skipped")
		}
		return newGrokClient(cfg.GrokAPIKey, model), nil

	case "anthropic", "claude":
		if model == "" {
			model = defaultModelAnthropic
		}
		if cfg.AnthropicAPIKey == "" {
			log.Println("⚠️  LLM_PROVIDER=anthropic but ANTHROPIC_API_KEY is unset — distillation will be skipped")
		}
		return newAnthropicClient(cfg.AnthropicAPIKey, model), nil

	case "deepseek":
		if model == "" {
			model = defaultModelDeepSeek
		}
		if cfg.DeepSeekAPIKey == "" {
			log.Println("⚠️  LLM_PROVIDER=deepseek but DEEPSEEK_API_KEY is unset — distillation will be skipped")
		}
		return newDeepSeekClient(cfg.DeepSeekAPIKey, model), nil

	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER %q: must be openai|grok|anthropic|deepseek", provider)
	}
}
