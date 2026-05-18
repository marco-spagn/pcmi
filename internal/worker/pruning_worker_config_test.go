package worker

import (
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestNewPruningWorker_defaults(t *testing.T) {
	w := NewPruningWorker(nil, nil)
	if w.retentionDays != 30 {
		t.Fatalf("retentionDays=%d", w.retentionDays)
	}
	if w.interval != 6*time.Hour {
		t.Fatalf("interval=%v", w.interval)
	}
}

func TestNewPruningWorker_configOverrides(t *testing.T) {
	w := NewPruningWorker(nil, &config.Config{
		PruneRetentionDays: 90,
		PruneIntervalSecs:  120,
	})
	if w.retentionDays != 90 || w.interval != 120*time.Second {
		t.Fatalf("retention=%d interval=%v", w.retentionDays, w.interval)
	}
}

func TestNewPruningWorker_invalidConfigUsesDefaults(t *testing.T) {
	w := NewPruningWorker(nil, &config.Config{
		PruneRetentionDays: 0,
		PruneIntervalSecs:  0,
	})
	if w.retentionDays != 30 {
		t.Fatalf("retentionDays=%d", w.retentionDays)
	}
	if w.interval != 6*time.Hour {
		t.Fatalf("interval=%v", w.interval)
	}
}
