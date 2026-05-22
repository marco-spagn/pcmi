package embedding

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

// PR #6 — tests for NewFromConfig. Provider selection is config-driven;
// these tests pin the matrix down.

func TestNewFromConfig_NilConfig(t *testing.T) {
	p, err := NewFromConfig(nil)
	if err != nil {
		t.Fatalf("nil config: unexpected error: %v", err)
	}
	if p != nil {
		t.Fatal("nil config must produce a nil Provider (embedding disabled)")
	}
}

func TestNewFromConfig_NoAPIKey(t *testing.T) {
	cfg := &config.Config{OpenAIAPIKey: "", OpenAIBaseURL: "https://api.openai.com"}
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("no key: unexpected error: %v", err)
	}
	if p != nil {
		t.Fatal("missing OPENAI_API_KEY must disable embedding (nil Provider)")
	}
}

func TestNewFromConfig_DefaultOpenAI(t *testing.T) {
	cfg := &config.Config{
		OpenAIAPIKey:   "sk-test",
		OpenAIBaseURL:  "",
		EmbeddingModel: "text-embedding-3-small",
	}
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("upstream OpenAI: unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("upstream OpenAI must yield a non-nil Provider when key is set")
	}
	cb, ok := p.(*CircuitBreakerProvider)
	if !ok {
		t.Fatalf("expected *CircuitBreakerProvider, got %T", p)
	}
	op, ok := cb.inner.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected OpenAI inner, got %T", cb.inner)
	}
	if op.model != "text-embedding-3-small" {
		t.Errorf("model: got %q, want text-embedding-3-small", op.model)
	}
}

func TestNewFromConfig_AzureEndpoint(t *testing.T) {
	cfg := &config.Config{
		OpenAIAPIKey:  "az-test-key",
		OpenAIBaseURL: "https://my-corp.openai.azure.com",
	}
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("Azure: unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("Azure endpoint must yield a non-nil Provider")
	}
}

func TestNewFromConfig_AzureEndpointWithPath(t *testing.T) {
	// Some operators pass the full Azure resource URL including
	// /openai/... — the host detection must still match.
	cfg := &config.Config{
		OpenAIAPIKey:  "az-key",
		OpenAIBaseURL: "https://my-corp.openai.azure.com/openai/deployments/pcmi",
	}
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("Azure w/ path: unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("Azure w/ path must yield non-nil Provider")
	}
}

func TestNewFromConfig_OpenAICompatibleLocal(t *testing.T) {
	// llama.cpp / vLLM / TGI all serve at an OpenAI-compatible HTTPS
	// endpoint. NewFromConfig must route to the compatible-mode provider.
	cfg := &config.Config{
		OpenAIAPIKey:  "anything", // local servers usually ignore the key
		OpenAIBaseURL: "https://llamacpp.internal:8080/v1",
	}
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("local-compatible: unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("local-compatible: expected non-nil Provider")
	}
}

func TestIsAzureEndpoint(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"https://api.openai.com", false},
		{"https://api.openai.com/v1", false},
		{"https://my-corp.openai.azure.com", true},
		{"https://my-corp.openai.azure.com/openai/deployments", true},
		{"http://my-corp.openai.azure.com", true},                 // scheme-insensitive
		{"my-corp.openai.azure.com", true},                        // no scheme
		{"https://my-corp.openai.azure.com?api-version=2024-02-15", true},
		{"https://llamacpp.internal:8080/v1", false},
		{"https://api.openai.com/v1?foo=openai.azure.com", false}, // suffix match only on host
	}
	for _, tc := range cases {
		if got := isAzureEndpoint(tc.in); got != tc.want {
			t.Errorf("isAzureEndpoint(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
