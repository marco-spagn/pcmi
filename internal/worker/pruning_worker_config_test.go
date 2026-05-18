package worker

import (
	"testing"
	"time"
)

func TestNewPruningWorker_defaults(t *testing.T) {
	t.Setenv("PRUNE_RETENTION_DAYS", "")
	t.Setenv("PRUNE_INTERVAL_SECS", "")
	w := NewPruningWorker(nil)
	if w.retentionDays != 30 {
		t.Fatalf("retentionDays=%d", w.retentionDays)
	}
	if w.interval != 6*time.Hour {
		t.Fatalf("interval=%v", w.interval)
	}
}

func TestNewPruningWorker_envOverrides(t *testing.T) {
	t.Setenv("PRUNE_RETENTION_DAYS", "90")
	t.Setenv("PRUNE_INTERVAL_SECS", "120")
	w := NewPruningWorker(nil)
	if w.retentionDays != 90 || w.interval != 120*time.Second {
		t.Fatalf("retention=%d interval=%v", w.retentionDays, w.interval)
	}
}

func TestNewPruningWorker_invalidEnvUsesDefaults(t *testing.T) {
	t.Setenv("PRUNE_RETENTION_DAYS", "not-int")
	t.Setenv("PRUNE_INTERVAL_SECS", "0")
	w := NewPruningWorker(nil)
	if w.retentionDays != 30 {
		t.Fatalf("retentionDays=%d", w.retentionDays)
	}
	if w.interval != 6*time.Hour {
		t.Fatalf("interval=%v", w.interval)
	}
}
