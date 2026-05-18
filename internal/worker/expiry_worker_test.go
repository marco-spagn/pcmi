package worker

import (
	"testing"
	"time"
)

func TestNewExpiryWorker_defaultInterval(t *testing.T) {
	t.Setenv("EXPIRY_INTERVAL_SECS", "")
	w := NewExpiryWorker(nil)
	if w.interval != 300*time.Second {
		t.Fatalf("default interval = %v", w.interval)
	}
}

func TestNewExpiryWorker_intervalFromEnv(t *testing.T) {
	t.Setenv("EXPIRY_INTERVAL_SECS", "42")
	w := NewExpiryWorker(nil)
	if w.interval != 42*time.Second {
		t.Fatalf("interval = %v", w.interval)
	}
}

func TestNewExpiryWorker_invalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("EXPIRY_INTERVAL_SECS", "not-a-number")
	w := NewExpiryWorker(nil)
	if w.interval != 300*time.Second {
		t.Fatalf("expected 300s default, got %v", w.interval)
	}
}

func TestNewExpiryWorker_nonPositiveEnvFallsBack(t *testing.T) {
	t.Setenv("EXPIRY_INTERVAL_SECS", "0")
	w := NewExpiryWorker(nil)
	if w.interval != 300*time.Second {
		t.Fatalf("expected 300s default, got %v", w.interval)
	}
}
