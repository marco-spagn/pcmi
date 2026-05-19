package worker

import (
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestNewPruningWorker_configOverride(t *testing.T) {
	cfg := &config.Config{PruneRetentionDays: 7, PruneIntervalSecs: 60}
	w := NewPruningWorker(nil, cfg)
	if w.retentionDays != 7 {
		t.Fatalf("retention=%d", w.retentionDays)
	}
	if w.interval != time.Minute {
		t.Fatalf("interval=%v", w.interval)
	}
}

func TestNewExpiryWorker_configOverride(t *testing.T) {
	w := NewExpiryWorker(nil, &config.Config{ExpiryIntervalSecs: 90})
	if w.interval != 90*time.Second {
		t.Fatalf("interval=%v", w.interval)
	}
}
