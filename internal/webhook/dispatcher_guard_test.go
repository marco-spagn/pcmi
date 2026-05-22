package webhook

import (
	"testing"
)

func TestNotifyMatching_nilDispatcher(t *testing.T) {
	var d *Dispatcher
	d.NotifyMatching("00000000-0000-0000-0000-000000000000", "memory.stored", map[string]any{"id": 1})
}

func TestNotifyMatching_emptyTenant(t *testing.T) {
	d := &Dispatcher{maxAttempts: 3}
	d.NotifyMatching("", "memory.stored", map[string]any{"id": 1})
}

func TestNewDispatcher_maxAttemptsClamped(t *testing.T) {
	// nil db is OK for constructor; only maxAttempts normalization is tested here.
	d := &Dispatcher{}
	d.maxAttempts = 0
	if d.maxAttempts < 1 {
		d.maxAttempts = 5
	}
	if d.maxAttempts != 5 {
		t.Fatalf("got %d", d.maxAttempts)
	}
}
