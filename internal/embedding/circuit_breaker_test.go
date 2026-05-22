package embedding

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/sony/gobreaker/v2"
)

type stubProvider struct {
	fn func(context.Context, string) ([]float32, error)
}

func (s *stubProvider) Generate(ctx context.Context, text string) ([]float32, error) {
	return s.fn(ctx, text)
}

func testCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxConsecutiveFailures: 3,
		BaseOpenTimeout:        80 * time.Millisecond,
		MaxOpenTimeout:         400 * time.Millisecond,
		RateLimitPerSec:        1000,
		HalfOpenMaxRequests:    1,
	}
}

func newTestCircuitBreaker(inner Provider) *CircuitBreakerProvider {
	p := WrapWithCircuitBreaker(inner, testCircuitBreakerConfig())
	return p.(*CircuitBreakerProvider)
}

func TestCircuitBreaker_ClosedState_PassesRequests(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	inner := &stubProvider{fn: func(context.Context, string) ([]float32, error) {
		calls.Add(1)
		return []float32{1, 2, 3}, nil
	}}
	cb := newTestCircuitBreaker(inner)

	vec, err := cb.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(vec) != 3 || calls.Load() != 1 {
		t.Fatalf("vec=%v calls=%d", vec, calls.Load())
	}
	if cb.State() != gobreaker.StateClosed {
		t.Fatalf("state=%v", cb.State())
	}
}

func TestCircuitBreaker_OpenState_FastFails(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	inner := &stubProvider{fn: func(context.Context, string) ([]float32, error) {
		calls.Add(1)
		return nil, errors.New("upstream down")
	}}
	cb := newTestCircuitBreaker(inner)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := cb.Generate(ctx, "x"); err == nil {
			t.Fatal("expected error")
		}
	}
	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("expected open, got %v", cb.State())
	}

	before := calls.Load()
	start := time.Now()
	_, err := cb.Generate(ctx, "x")
	elapsed := time.Since(start)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err=%v", err)
	}
	if calls.Load() != before {
		t.Fatalf("inner called while open: calls=%d", calls.Load())
	}
	if elapsed > 15*time.Millisecond {
		t.Fatalf("open-state call too slow: %v", elapsed)
	}
}

func TestCircuitBreaker_HalfOpen_RecoveryOnSuccess(t *testing.T) {
	t.Parallel()

	var fail atomic.Bool
	fail.Store(true)
	inner := &stubProvider{fn: func(context.Context, string) ([]float32, error) {
		if fail.Load() {
			return nil, errors.New("fail")
		}
		return []float32{0.5}, nil
	}}
	cfg := testCircuitBreakerConfig()
	cfg.BaseOpenTimeout = 40 * time.Millisecond
	cb := WrapWithCircuitBreaker(inner, cfg).(*CircuitBreakerProvider)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, _ = cb.Generate(ctx, "x")
	}
	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("expected open, got %v", cb.State())
	}

	time.Sleep(cfg.BaseOpenTimeout + 25*time.Millisecond)
	fail.Store(false)

	vec, err := cb.Generate(ctx, "x")
	if err != nil {
		t.Fatalf("recovery Generate: %v", err)
	}
	if len(vec) != 1 {
		t.Fatalf("vec=%v", vec)
	}
	if cb.State() != gobreaker.StateClosed {
		t.Fatalf("expected closed after success, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_ReOpensOnFailure(t *testing.T) {
	t.Parallel()

	inner := &stubProvider{fn: func(context.Context, string) ([]float32, error) {
		return nil, errors.New("still down")
	}}
	cfg := testCircuitBreakerConfig()
	cfg.BaseOpenTimeout = 40 * time.Millisecond
	cb := WrapWithCircuitBreaker(inner, cfg).(*CircuitBreakerProvider)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, _ = cb.Generate(ctx, "x")
	}
	time.Sleep(cfg.BaseOpenTimeout + 25*time.Millisecond)

	_, err := cb.Generate(ctx, "x")
	if err == nil {
		t.Fatal("expected error in half-open probe")
	}
	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("expected open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_ExponentialBackoff_Jitter(t *testing.T) {
	t.Parallel()

	base := 100 * time.Millisecond
	max := 2 * time.Second
	seen := make(map[time.Duration]struct{})
	for i := 0; i < 40; i++ {
		d := OpenBackoffDuration(2, base, max)
		if d < 300*time.Millisecond || d > 1300*time.Millisecond {
			t.Fatalf("attempt 2 backoff out of jitter range: %v", d)
		}
		seen[d] = struct{}{}
	}
	if len(seen) < 3 {
		t.Fatalf("expected jitter spread, got %d unique durations", len(seen))
	}

	d0 := OpenBackoffDuration(0, base, max)
	if d0 < 75*time.Millisecond || d0 > 125*time.Millisecond {
		t.Fatalf("attempt 0 backoff: %v", d0)
	}

	dCap := OpenBackoffDuration(20, base, max)
	if dCap > max {
		t.Fatalf("cap exceeded: %v", dCap)
	}
}

func TestOpenAIProvider_WrappedByCircuitBreakerFromFactory(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	cfg := openai.DefaultConfig("sk-test")
	cfg.BaseURL = srv.URL + "/v1"
	inner := NewOpenAIProviderWithConfig(cfg, "m")
	cb := WrapWithCircuitBreaker(inner, testCircuitBreakerConfig())

	for i := 0; i < 3; i++ {
		_, _ = cb.Generate(context.Background(), "x")
	}
	_, err := cb.Generate(context.Background(), "x")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected circuit open, got %v", err)
	}
}
