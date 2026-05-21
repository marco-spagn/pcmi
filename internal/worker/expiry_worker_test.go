package worker

import (
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestNewExpiryWorker_defaultInterval(t *testing.T) {
	cfg := config.Load()
	w := NewExpiryWorker(nil, cfg)
	want := time.Duration(cfg.ExpiryIntervalSecs) * time.Second
	if w.interval != want {
		t.Fatalf("interval = %v want %v", w.interval, want)
	}
}

func TestNewExpiryWorker_intervalFromConfig(t *testing.T) {
	w := NewExpiryWorker(nil, &config.Config{ExpiryIntervalSecs: 42})
	if w.interval != 42*time.Second {
		t.Fatalf("interval = %v", w.interval)
	}
}

func TestNewExpiryWorker_nilConfigUsesDefault(t *testing.T) {
	w := NewExpiryWorker(nil, nil)
	if w.interval != time.Hour {
		t.Fatalf("expected 1h default, got %v", w.interval)
	}
}

func TestNewExpiryWorker_nonPositiveFallsBack(t *testing.T) {
	w := NewExpiryWorker(nil, &config.Config{ExpiryIntervalSecs: 0})
	if w.interval != time.Hour {
		t.Fatalf("expected 1h default, got %v", w.interval)
	}
}
