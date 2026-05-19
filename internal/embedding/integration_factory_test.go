//go:build integration

package embedding

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

// TestIntegration_NewFromConfig_EnvMatrix verifies provider selection against
// config.Load() after t.Setenv — same path cmd/api and cmd/worker use at startup.
func TestIntegration_NewFromConfig_EnvMatrix(t *testing.T) {
	cases := []struct {
		name    string
		apiKey  string
		baseURL string
		wantNil bool
	}{
		{name: "disabled", apiKey: "", baseURL: "", wantNil: true},
		{name: "upstream", apiKey: "sk-x", baseURL: "", wantNil: false},
		{name: "azure", apiKey: "az", baseURL: "https://res.openai.azure.com", wantNil: false},
		{name: "compatible", apiKey: "local", baseURL: "https://embed.internal/v1", wantNil: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", tc.apiKey)
			t.Setenv("OPENAI_BASE_URL", tc.baseURL)
			cfg := config.Load()
			p, err := NewFromConfig(cfg)
			if err != nil {
				t.Fatalf("NewFromConfig: %v", err)
			}
			if tc.wantNil && p != nil {
				t.Fatal("expected nil provider")
			}
			if !tc.wantNil && p == nil {
				t.Fatal("expected non-nil provider")
			}
		})
	}
}
