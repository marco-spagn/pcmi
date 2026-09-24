package handler

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/service"
)

func TestConfigureReranker(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"nil config", nil, false},
		{"disabled", &config.Config{OpenAIAPIKey: "sk"}, false},
		{"enabled without key", &config.Config{RerankEnabled: true}, false},
		{"enabled bad provider", &config.Config{RerankEnabled: true, LLMProvider: "nope"}, false},
		{"enabled openai", &config.Config{RerankEnabled: true, OpenAIAPIKey: "sk", RerankModel: "gpt-x", RerankTimeoutMs: 100}, true},
		{"enabled anthropic", &config.Config{RerankEnabled: true, LLMProvider: "anthropic", AnthropicAPIKey: "k"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewMemoryService(nil, nil)
			ConfigureReranker(svc, tc.cfg)
			if svc.RerankEnabled() != tc.want {
				t.Fatalf("RerankEnabled=%v want %v", svc.RerankEnabled(), tc.want)
			}
		})
	}
	ConfigureReranker(nil, &config.Config{RerankEnabled: true}) // nil service is a no-op
}
