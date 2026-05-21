package webhook

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestWebhookMaxAttemptsFromConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("WEBHOOK_MAX_ATTEMPTS", "")
		cfg := config.Load()
		d := NewDispatcher(nil, cfg.WebhookMaxAttempts)
		if d.maxAttempts != 5 {
			t.Fatalf("got %d want 5", d.maxAttempts)
		}
	})
	t.Run("valid override", func(t *testing.T) {
		t.Setenv("WEBHOOK_MAX_ATTEMPTS", "12")
		cfg := config.Load()
		d := NewDispatcher(nil, cfg.WebhookMaxAttempts)
		if d.maxAttempts != 12 {
			t.Fatalf("got %d", d.maxAttempts)
		}
	})
	t.Run("zero falls back in constructor", func(t *testing.T) {
		d := NewDispatcher(nil, 0)
		if d.maxAttempts != 5 {
			t.Fatalf("got %d want default", d.maxAttempts)
		}
	})
	t.Run("invalid env uses config default", func(t *testing.T) {
		t.Setenv("WEBHOOK_MAX_ATTEMPTS", "nope")
		cfg := config.Load()
		d := NewDispatcher(nil, cfg.WebhookMaxAttempts)
		if d.maxAttempts != 5 {
			t.Fatalf("got %d want default", d.maxAttempts)
		}
	})
}
