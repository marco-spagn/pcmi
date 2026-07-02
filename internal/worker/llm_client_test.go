package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

// ─── factory ──────────────────────────────────────────────────────────────────

func TestNewLLMClient_defaultIsOpenAI(t *testing.T) {
	c, err := NewLLMClient(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*openAILLMClient); !ok {
		t.Fatalf("expected *openAILLMClient, got %T", c)
	}
}

func TestNewLLMClient_openai(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "openai", OpenAIAPIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	cl := c.(*openAILLMClient)
	if !cl.IsConfigured() {
		t.Fatal("expected configured")
	}
}

func TestNewLLMClient_openai_noKey(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	if c.IsConfigured() {
		t.Fatal("should not be configured without key")
	}
}

func TestNewLLMClient_grok(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "grok", GrokAPIKey: "xai-test"})
	if err != nil {
		t.Fatal(err)
	}
	cl := c.(*openAILLMClient)
	if !cl.IsConfigured() {
		t.Fatal("expected configured")
	}
}

func TestNewLLMClient_xai_alias(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "xai", GrokAPIKey: "xai-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*openAILLMClient); !ok {
		t.Fatalf("expected *openAILLMClient, got %T", c)
	}
}

func TestNewLLMClient_anthropic(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "anthropic", AnthropicAPIKey: "ant-test"})
	if err != nil {
		t.Fatal(err)
	}
	cl := c.(*anthropicLLMClient)
	if !cl.IsConfigured() {
		t.Fatal("expected configured")
	}
}

func TestNewLLMClient_claude_alias(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "claude", AnthropicAPIKey: "ant-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*anthropicLLMClient); !ok {
		t.Fatalf("expected *anthropicLLMClient, got %T", c)
	}
}

func TestNewLLMClient_deepseek(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "deepseek", DeepSeekAPIKey: "ds-test"})
	if err != nil {
		t.Fatal(err)
	}
	cl := c.(*openAILLMClient)
	if !cl.IsConfigured() {
		t.Fatal("expected configured")
	}
}

func TestNewLLMClient_unsupportedProvider(t *testing.T) {
	_, err := NewLLMClient(&config.Config{LLMProvider: "ollama"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNewLLMClient_nil_config(t *testing.T) {
	c, err := NewLLMClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewLLMClient_customModel(t *testing.T) {
	c, err := NewLLMClient(&config.Config{LLMProvider: "grok", GrokAPIKey: "k", DistillationModel: "grok-2"})
	if err != nil {
		t.Fatal(err)
	}
	if c.(*openAILLMClient).modelName != "grok-2" {
		t.Fatalf("model=%q", c.(*openAILLMClient).modelName)
	}
}

// ─── default model names ──────────────────────────────────────────────────────

func TestDefaultModels(t *testing.T) {
	cases := []struct {
		provider string
		key      string
		want     string
	}{
		{"openai", "sk", defaultModelOpenAI},
		{"grok", "xai", defaultModelGrok},
		{"anthropic", "ant", defaultModelAnthropic},
		{"deepseek", "ds", defaultModelDeepSeek},
	}
	for _, tc := range cases {
		cfg := &config.Config{LLMProvider: tc.provider}
		switch tc.provider {
		case "openai":
			cfg.OpenAIAPIKey = tc.key
		case "grok":
			cfg.GrokAPIKey = tc.key
		case "anthropic":
			cfg.AnthropicAPIKey = tc.key
		case "deepseek":
			cfg.DeepSeekAPIKey = tc.key
		}
		c, err := NewLLMClient(cfg)
		if err != nil {
			t.Fatalf("%s: %v", tc.provider, err)
		}
		var model string
		switch v := c.(type) {
		case *openAILLMClient:
			model = v.modelName
		case *anthropicLLMClient:
			model = v.modelName
		}
		if model != tc.want {
			t.Errorf("%s: model=%q want=%q", tc.provider, model, tc.want)
		}
	}
}

// ─── openAILLMClient unit tests ───────────────────────────────────────────────

func TestOpenAILLMClient_IsConfigured(t *testing.T) {
	if newOpenAIClient("", "m", "").IsConfigured() {
		t.Fatal("empty key should not be configured")
	}
	if !newOpenAIClient("sk-x", "m", "").IsConfigured() {
		t.Fatal("non-empty key should be configured")
	}
}

// ─── anthropicLLMClient unit tests ───────────────────────────────────────────

func TestAnthropicLLMClient_IsConfigured(t *testing.T) {
	if newAnthropicClient("", "m").IsConfigured() {
		t.Fatal("empty key should not be configured")
	}
	if !newAnthropicClient("ant-x", "m").IsConfigured() {
		t.Fatal("non-empty key should be configured")
	}
}

func TestAnthropicLLMClient_Complete_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []anthropicContentBlock{{Type: "text", Text: `{"summary":"s","insights":["i1"]}`}},
		})
	}))
	defer srv.Close()

	c := &anthropicLLMClient{
		apiKey:     "test-key",
		modelName:  "claude-test",
		httpClient: srv.Client(),
	}
	// Point at test server by overriding the URL via a custom transport.
	c.httpClient = &http.Client{
		Transport: &prefixRewriter{base: srv.URL, inner: http.DefaultTransport},
	}

	text, err := c.Complete(context.Background(), "system", []string{"user msg"})
	if err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Fatal("expected non-empty response")
	}
}

func TestAnthropicLLMClient_Complete_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"auth_error","message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &anthropicLLMClient{
		apiKey:    "bad",
		modelName: "m",
		httpClient: &http.Client{
			Transport: &prefixRewriter{base: srv.URL, inner: http.DefaultTransport},
		},
	}
	_, err := c.Complete(context.Background(), "", []string{"x"})
	if err == nil {
		t.Fatal("expected error on HTTP 401")
	}
}

func TestAnthropicLLMClient_Complete_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Error: &struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{Type: "overloaded_error", Message: "overloaded"},
		})
	}))
	defer srv.Close()

	c := &anthropicLLMClient{
		apiKey:    "k",
		modelName: "m",
		httpClient: &http.Client{
			Transport: &prefixRewriter{base: srv.URL, inner: http.DefaultTransport},
		},
	}
	_, err := c.Complete(context.Background(), "", []string{"x"})
	if err == nil || !containsStr(err.Error(), "overloaded_error") {
		t.Fatalf("expected overloaded_error, got %v", err)
	}
}

func TestAnthropicLLMClient_Complete_noTextBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []anthropicContentBlock{{Type: "tool_use", Text: ""}},
		})
	}))
	defer srv.Close()

	c := &anthropicLLMClient{
		apiKey:    "k",
		modelName: "m",
		httpClient: &http.Client{
			Transport: &prefixRewriter{base: srv.URL, inner: http.DefaultTransport},
		},
	}
	_, err := c.Complete(context.Background(), "", []string{"x"})
	if err == nil {
		t.Fatal("expected error for no text block")
	}
}

func TestAnthropicLLMClient_Complete_cancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newAnthropicClient("key", "model")
	_, err := c.Complete(ctx, "", []string{"x"})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

// ─── DistillationWorker integration with LLMClient ───────────────────────────

func TestNewDistillationWorker_usesLLMClient(t *testing.T) {
	w := NewDistillationWorker(nil, &config.Config{LLMProvider: "openai"})
	if w.llm == nil {
		t.Fatal("expected llm to be set")
	}
	if w.llm.IsConfigured() {
		t.Fatal("should not be configured without api key")
	}
}

func TestNewDistillationWorker_anthropicProvider(t *testing.T) {
	w := NewDistillationWorker(nil, &config.Config{
		LLMProvider:     "anthropic",
		AnthropicAPIKey: "ant-key",
	})
	if _, ok := w.llm.(*anthropicLLMClient); !ok {
		t.Fatalf("expected anthropicLLMClient, got %T", w.llm)
	}
	if !w.llm.IsConfigured() {
		t.Fatal("expected configured")
	}
}

func TestNewDistillationWorker_invalidProvider_fallback(t *testing.T) {
	// Invalid provider logs a warning and falls back to unconfigured openai.
	w := NewDistillationWorker(nil, &config.Config{LLMProvider: "unknown-xyz"})
	if w.llm == nil {
		t.Fatal("expected non-nil llm after fallback")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// prefixRewriter rewrites requests to point at a test server base URL.
type prefixRewriter struct {
	base  string
	inner http.RoundTripper
}

func (p *prefixRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = "http"
	r2.URL.Host = r.URL.Host
	// Replace scheme+host with the test server.
	parsed, _ := http.NewRequest(r.Method, p.base+r.URL.Path, r.Body)
	parsed.Header = r.Header
	parsed.URL.RawQuery = r.URL.RawQuery
	return p.inner.RoundTrip(parsed)
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// mockLLMClient for use in other test files.
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) IsConfigured() bool { return m.err == nil || m.response != "" }
func (m *mockLLMClient) Complete(_ context.Context, _ string, _ []string) (string, error) {
	return m.response, m.err
}

var _ LLMClient = (*mockLLMClient)(nil)
var _ LLMClient = (*openAILLMClient)(nil)
var _ LLMClient = (*anthropicLLMClient)(nil)

// Ensure errors package is used.
var _ = errors.New
