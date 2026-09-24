package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/sashabaranov/go-openai"

	"github.com/marco-spagn/pcmi/internal/usage"
)

type usageCapture struct {
	mu  sync.Mutex
	evs []usage.Event
}

func (c *usageCapture) Record(ev usage.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evs = append(c.evs, ev)
}

func (c *usageCapture) find(tenant string) *usage.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.evs {
		if c.evs[i].TenantID == tenant {
			return &c.evs[i]
		}
	}
	return nil
}

func installCapture(t *testing.T) *usageCapture {
	t.Helper()
	c := &usageCapture{}
	prev := usage.SetRecorder(c)
	t.Cleanup(func() { usage.SetRecorder(prev) })
	return c
}

func TestAnthropicLLMClient_reportsUsage(t *testing.T) {
	capt := installCapture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":321,"output_tokens":45}}`))
	}))
	defer srv.Close()
	c := &anthropicLLMClient{apiKey: "k", modelName: "claude-x",
		httpClient: &http.Client{Transport: &prefixRewriter{base: srv.URL, inner: http.DefaultTransport}}}
	ctx := usage.WithScope(context.Background(), "tenant-anthropic", usage.OpDistillation)
	if _, err := c.Complete(ctx, "", []string{"x"}); err != nil {
		t.Fatal(err)
	}
	ev := capt.find("tenant-anthropic")
	if ev == nil || ev.Provider != "anthropic" || ev.Model != "claude-x" || ev.InputTokens != 321 || ev.OutputTokens != 45 || ev.Operation != usage.OpDistillation {
		t.Fatalf("event %+v", ev)
	}
}

func TestOpenAICompatibleClients_reportUsage(t *testing.T) {
	capt := installCapture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "{}"}}},
			Usage:   openai.Usage{PromptTokens: 11, CompletionTokens: 7},
		})
	}))
	defer srv.Close()
	for _, tc := range []struct{ tenant, provider string }{
		{"tenant-oa", "openai"}, {"tenant-grok", "grok"}, {"tenant-ds", "deepseek"},
	} {
		c := newOpenAIClient("k", "m1", srv.URL+"/v1")
		c.provider = tc.provider
		if _, err := c.Complete(usage.WithScope(context.Background(), tc.tenant, usage.OpExtraction), "sys", []string{"u"}); err != nil {
			t.Fatal(err)
		}
		ev := capt.find(tc.tenant)
		if ev == nil || ev.Provider != tc.provider || ev.InputTokens != 11 || ev.OutputTokens != 7 {
			t.Fatalf("%s: event %+v", tc.provider, ev)
		}
	}
	if newGrokClient("k", "m").provider != "grok" || newDeepSeekClient("k", "m").provider != "deepseek" || newOpenAIClient("k", "m", "").provider != "openai" {
		t.Fatal("provider labels")
	}
}
