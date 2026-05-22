// Package embedding — factory.go (PR #6).
//
// Provider-agnostic factory. Returns a concrete embedding.Provider chosen
// from *config.Config, so call sites no longer hard-code OpenAI:
//
//	provider, err := embedding.NewFromConfig(cfg)
//	if err != nil { ... }
//	emb, err := provider.Generate(ctx, "some text")
//
// Provider selection is driven entirely by Config — the call site never
// sees the underlying SDK type. This unblocks Azure OpenAI, llama.cpp-style
// local servers, and any future OpenAI-compatible endpoint without churn
// in worker/, service/, or cmd/.
//
// Provider matrix:
//
//	cfg.OpenAIAPIKey == ""                                 → Disabled (nil Provider, nil error — caller decides)
//	cfg.OpenAIBaseURL == "" && OpenAIAPIKey != ""          → OpenAI (default upstream)
//	cfg.OpenAIBaseURL contains ".openai.azure.com"         → Azure OpenAI (api-version pinned)
//	cfg.OpenAIBaseURL set otherwise (custom HTTPS endpoint) → OpenAI-compatible (e.g. llama.cpp server)
package embedding

import (
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/marco-spagn/pcmi/internal/config"
)

// NewFromConfig returns a ready-to-use Provider, or (nil, nil) when no
// OpenAI API key is configured — in that case the embedding worker is
// intentionally disabled. Returns an error only on misconfiguration
// (e.g. an Azure base URL without an api-key).
//
// Selection order:
//  1. Azure OpenAI when BaseURL host ends in ".openai.azure.com"
//  2. OpenAI-compatible local server when BaseURL is set but not Azure
//  3. Upstream OpenAI when only the API key is set
//  4. Disabled when no API key at all
func NewFromConfig(cfg *config.Config) (Provider, error) {
	if cfg == nil {
		return nil, nil
	}
	apiKey := strings.TrimSpace(cfg.OpenAIAPIKey)
	baseURL := strings.TrimSpace(cfg.OpenAIBaseURL)
	model := strings.TrimSpace(cfg.EmbeddingModel)

	if apiKey == "" {
		return nil, nil
	}

	var (
		prov Provider
		err  error
	)
	switch {
	case isAzureEndpoint(baseURL):
		prov, err = newAzureProvider(apiKey, baseURL, model)
	case baseURL != "":
		prov = newCompatibleProvider(apiKey, baseURL, model)
	default:
		prov = NewOpenAIProvider(apiKey, model)
	}
	if err != nil {
		return nil, err
	}
	return WrapWithCircuitBreaker(prov, DefaultCircuitBreakerConfig()), nil
}

// isAzureEndpoint guards the Azure detection: the BaseURL must end with
// the Azure-managed host suffix. We accept either a bare host or a full
// URL (https://<resource>.openai.azure.com).
func isAzureEndpoint(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	// Strip scheme and any trailing path/query so we only match the
	// hostname.
	s := strings.TrimPrefix(baseURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return strings.HasSuffix(strings.ToLower(s), ".openai.azure.com")
}

// newAzureProvider builds a Provider against Azure OpenAI. Azure exposes
// the same Chat/Embeddings shape but uses a different auth scheme
// (api-key header) and requires the api-version query parameter on every
// call. The go-openai SDK already handles both when we use the Azure
// config helper.
func newAzureProvider(apiKey, baseURL, model string) (Provider, error) {
	cfg := openai.DefaultAzureConfig(apiKey, baseURL)
	// Azure deployment names == model names by convention in PCMI; ops
	// teams that diverge can override the model field per tenant in
	// PR#7's per-tenant config (out of scope for PR #6).
	return NewOpenAIProviderWithConfig(cfg, model), nil
}

// newCompatibleProvider points the go-openai client at any
// OpenAI-compatible HTTPS endpoint (llama.cpp server, vLLM, TGI, …).
// Auth and request shape match upstream OpenAI 1:1; the only thing that
// changes is the base URL.
func newCompatibleProvider(apiKey, baseURL, model string) Provider {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = strings.TrimRight(baseURL, "/")
	return NewOpenAIProviderWithConfig(cfg, model)
}
