package webhook

import "testing"

func TestMaxAttemptsFromEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("WEBHOOK_MAX_ATTEMPTS", "")
		if n := maxAttemptsFromEnv(); n != defaultMaxAttempts {
			t.Fatalf("got %d want %d", n, defaultMaxAttempts)
		}
	})
	t.Run("valid override", func(t *testing.T) {
		t.Setenv("WEBHOOK_MAX_ATTEMPTS", "12")
		if n := maxAttemptsFromEnv(); n != 12 {
			t.Fatalf("got %d", n)
		}
	})
	t.Run("zero falls back", func(t *testing.T) {
		t.Setenv("WEBHOOK_MAX_ATTEMPTS", "0")
		if n := maxAttemptsFromEnv(); n != defaultMaxAttempts {
			t.Fatalf("got %d want default", n)
		}
	})
	t.Run("invalid falls back", func(t *testing.T) {
		t.Setenv("WEBHOOK_MAX_ATTEMPTS", "nope")
		if n := maxAttemptsFromEnv(); n != defaultMaxAttempts {
			t.Fatalf("got %d want default", n)
		}
	})
}
