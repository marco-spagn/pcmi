package embedding

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

func TestCircuitBreaker_OpenGeneration_IncrementsOnReopen(t *testing.T) {
	t.Parallel()
	inner := &stubProvider{fn: func(context.Context, string) ([]float32, error) {
		return nil, errors.New("down")
	}}
	cfg := testCircuitBreakerConfig()
	cfg.BaseOpenTimeout = 30 * time.Millisecond
	cfg.MaxOpenTimeout = 200 * time.Millisecond
	cb := WrapWithCircuitBreaker(inner, cfg).(*CircuitBreakerProvider)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, _ = cb.Generate(ctx, "x")
	}
	if cb.OpenGeneration() != 1 {
		t.Fatalf("openGen=%d want 1", cb.OpenGeneration())
	}

	time.Sleep(cfg.BaseOpenTimeout + 40*time.Millisecond)
	_, _ = cb.Generate(ctx, "x") // half-open fail -> reopen

	if cb.OpenGeneration() < 2 {
		t.Fatalf("openGen=%d want >=2 after half-open failure", cb.OpenGeneration())
	}
}

func TestCircuitBreaker_CustomOpenHold_BlocksBeforeGobreakerHalfOpen(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	inner := &stubProvider{fn: func(context.Context, string) ([]float32, error) {
		calls.Add(1)
		return nil, errors.New("fail")
	}}
	cfg := testCircuitBreakerConfig()
	cfg.MaxConsecutiveFailures = 2
	cfg.BaseOpenTimeout = 5 * time.Millisecond
	cfg.MaxOpenTimeout = 500 * time.Millisecond
	cb := WrapWithCircuitBreaker(inner, cfg).(*CircuitBreakerProvider)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_, _ = cb.Generate(ctx, "x")
	}
	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("state=%v", cb.State())
	}

	// Immediately after open: custom hold should fast-fail without calling inner again.
	before := calls.Load()
	for range 5 {
		_, err := cb.Generate(ctx, "x")
		if !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("err=%v", err)
		}
	}
	if calls.Load() != before {
		t.Fatalf("inner called during custom open hold: before=%d after=%d", before, calls.Load())
	}
}

func TestCircuitBreaker_NilInner_ReturnsNil(t *testing.T) {
	t.Parallel()
	if WrapWithCircuitBreaker(nil, DefaultCircuitBreakerConfig()) != nil {
		t.Fatal("expected nil wrapper for nil inner")
	}
}

func TestOpenBackoffDuration_NegativeBaseUsesDefault(t *testing.T) {
	t.Parallel()
	d := OpenBackoffDuration(0, -1, -1)
	if d <= 0 {
		t.Fatalf("expected positive duration, got %v", d)
	}
}
