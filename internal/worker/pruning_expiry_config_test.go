package worker

import (
	"testing"
	"time"
)

func TestNewPruningWorker_envOverride(t *testing.T) {
	t.Setenv("PRUNE_RETENTION_DAYS", "42")
	t.Setenv("PRUNE_INTERVAL_SECS", "120")
	w := NewPruningWorker(nil)
	if w.retentionDays != 42 {
		t.Fatalf("retention %d", w.retentionDays)
	}
	if w.interval != 120*time.Second {
		t.Fatalf("interval %s", w.interval)
	}
}

func TestNewExpiryWorker_envOverride(t *testing.T) {
	t.Setenv("EXPIRY_INTERVAL_SECS", "99")
	w := NewExpiryWorker(nil)
	if w.interval != 99*time.Second {
		t.Fatalf("interval %s", w.interval)
	}
}
