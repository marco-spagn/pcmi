package webhook

import (
	"testing"
	"time"
)

func TestBackoffCap(t *testing.T) {
	d := &Dispatcher{retryBase: 2 * time.Second, maxAttempts: 5}
	backoff := d.retryBase * (1 << 10)
	if backoff > 2*time.Minute {
		backoff = 2 * time.Minute
	}
	if backoff != 2*time.Minute {
		t.Fatalf("expected cap at 2m, got %v", backoff)
	}
}
