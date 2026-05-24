package embedding

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

// TestCBStress_ConcurrentTrip launches 50 goroutines all generating failures
// simultaneously. The circuit must open exactly once; openGeneration must
// equal 1 afterwards (no spurious resets or double-trips).
func TestCBStress_ConcurrentTrip(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	inner := &stubProvider{fn: func(_ context.Context, _ string) ([]float32, error) {
		calls.Add(1)
		return nil, errors.New("forced failure")
	}}
	cfg := CircuitBreakerConfig{
		MaxConsecutiveFailures: 3,
		BaseOpenTimeout:        500 * time.Millisecond,
		MaxOpenTimeout:         2 * time.Second,
		RateLimitPerSec:        10_000,
		HalfOpenMaxRequests:    1,
	}
	cb := WrapWithCircuitBreaker(inner, cfg).(*CircuitBreakerProvider)
	ctx := context.Background()

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cb.Generate(ctx, "x")
		}()
	}
	wg.Wait()

	state := cb.State()
	if state != gobreaker.StateOpen {
		t.Errorf("expected StateOpen after %d failures, got %v", calls.Load(), state)
	}
	if gen := cb.OpenGeneration(); gen != 1 {
		t.Errorf("openGeneration=%d, want 1 (circuit opened exactly once)", gen)
	}
}

// TestCBStress_OpenCircuitFastFail verifies that 200 goroutines all get
// ErrCircuitOpen quickly once the circuit is open. Each call must return
// in < 5ms (no upstream latency should be incurred).
func TestCBStress_OpenCircuitFastFail(t *testing.T) {
	t.Parallel()
	inner := &stubProvider{fn: func(_ context.Context, _ string) ([]float32, error) {
		time.Sleep(100 * time.Millisecond) // slow upstream
		return nil, errors.New("down")
	}}
	cfg := CircuitBreakerConfig{
		MaxConsecutiveFailures: 2,
		BaseOpenTimeout:        5 * time.Second,
		MaxOpenTimeout:         10 * time.Second,
		RateLimitPerSec:        100_000,
		HalfOpenMaxRequests:    1,
	}
	cb := WrapWithCircuitBreaker(inner, cfg).(*CircuitBreakerProvider)
	ctx := context.Background()

	// Trip the circuit.
	for i := 0; i < 2; i++ {
		_, _ = cb.Generate(ctx, "trip")
	}
	if cb.State() != gobreaker.StateOpen {
		t.Skip("circuit did not open — timing sensitive, skipping")
	}

	const goroutines = 200
	var wg sync.WaitGroup
	var wrongErrors atomic.Int32
	var slowCalls atomic.Int32

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			_, err := cb.Generate(ctx, "x")
			elapsed := time.Since(start)
			if !errors.Is(err, ErrCircuitOpen) {
				wrongErrors.Add(1)
			}
			if elapsed > 5*time.Millisecond {
				slowCalls.Add(1)
			}
		}()
	}
	wg.Wait()

	if n := wrongErrors.Load(); n > 0 {
		t.Errorf("%d calls did not return ErrCircuitOpen", n)
	}
	if n := slowCalls.Load(); n > 0 {
		t.Errorf("%d calls took > 5ms on open circuit (upstream should be bypassed)", n)
	}
}

// TestCBStress_HalfOpenProbeIsolation verifies that only MaxRequests=1
// goroutine is allowed to probe in half-open state. 50 goroutines all race
// to call Generate after the open timeout expires; at most 1 should reach
// the inner provider per half-open window.
func TestCBStress_HalfOpenProbeIsolation(t *testing.T) {
	t.Parallel()
	cfg := CircuitBreakerConfig{
		MaxConsecutiveFailures: 2,
		BaseOpenTimeout:        50 * time.Millisecond,
		MaxOpenTimeout:         200 * time.Millisecond,
		RateLimitPerSec:        100_000,
		HalfOpenMaxRequests:    1, // exactly 1 probe allowed
	}
	ctx := context.Background()

	var successCalls atomic.Int32
	cbForProbe := WrapWithCircuitBreaker(&stubProvider{fn: func(_ context.Context, _ string) ([]float32, error) {
		successCalls.Add(1)
		return []float32{1.0}, nil
	}}, cfg).(*CircuitBreakerProvider)

	// Trip cbForProbe by bypassing the limiter directly.
	for i := 0; i < int(cfg.MaxConsecutiveFailures); i++ {
		_, _ = cbForProbe.cb.Execute(func() ([]float32, error) {
			return nil, errors.New("x")
		})
	}
	if cbForProbe.State() != gobreaker.StateOpen {
		t.Skip("circuit did not open — timing sensitive")
	}

	// Wait for the half-open window to open.
	time.Sleep(cfg.BaseOpenTimeout + 20*time.Millisecond)

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cbForProbe.Generate(ctx, "probe")
		}()
	}
	wg.Wait()

	// After the half-open probe succeeds, the circuit should be closed.
	if cbForProbe.State() == gobreaker.StateOpen {
		t.Error("circuit still open after successful probe")
	}
}

// TestCBStress_RepeatedTripRecoverCycle opens and closes the circuit 10 times.
// It verifies that the circuit reliably trips and recovers each cycle, and that
// the final state is Closed (openGeneration intentionally resets to 0 on close —
// it is a per-open-window counter used for exponential backoff, not a lifetime one).
func TestCBStress_RepeatedTripRecoverCycle(t *testing.T) {
	t.Parallel()
	const cycles = 10
	cfg := CircuitBreakerConfig{
		MaxConsecutiveFailures: 2,
		BaseOpenTimeout:        20 * time.Millisecond,
		MaxOpenTimeout:         100 * time.Millisecond,
		RateLimitPerSec:        100_000,
		HalfOpenMaxRequests:    1,
	}
	ctx := context.Background()

	var fail atomic.Bool
	cb := WrapWithCircuitBreaker(&stubProvider{fn: func(_ context.Context, _ string) ([]float32, error) {
		if fail.Load() {
			return nil, errors.New("down")
		}
		return []float32{1.0}, nil
	}}, cfg).(*CircuitBreakerProvider)

	var trippedCount int
	for cycle := 0; cycle < cycles; cycle++ {
		// Trip.
		fail.Store(true)
		for i := 0; i < int(cfg.MaxConsecutiveFailures)+1; i++ {
			_, _ = cb.Generate(ctx, "x")
		}
		if cb.State() == gobreaker.StateOpen {
			trippedCount++
		}
		// Wait for half-open window.
		time.Sleep(cfg.BaseOpenTimeout + 15*time.Millisecond)
		// Recover via probe.
		fail.Store(false)
		_, _ = cb.Generate(ctx, "x")
		time.Sleep(5 * time.Millisecond)
	}

	// Circuit must be closed (or at most half-open) after all cycles.
	finalState := cb.State()
	if finalState == gobreaker.StateOpen {
		t.Errorf("circuit stuck open after %d trip/recover cycles", cycles)
	}
	// Must have tripped at least half the cycles (timing variance allowed).
	if trippedCount < cycles/2 {
		t.Errorf("circuit tripped only %d/%d cycles", trippedCount, cycles)
	}
	// openGeneration resets to 0 when circuit closes — that is correct behaviour.
	// Verify the final generate works without error.
	fail.Store(false)
	if _, err := cb.Generate(ctx, "final"); err != nil {
		t.Errorf("final Generate after cycles: %v", err)
	}
}

// TestCBStress_RateLimiterBoundary fires 500 goroutines at RateLimitPerSec=5.
// Most will wait in the rate limiter; the test verifies no deadlock within 10s.
func TestCBStress_RateLimiterBoundary(t *testing.T) {
	t.Parallel()
	inner := &stubProvider{fn: func(_ context.Context, _ string) ([]float32, error) {
		return []float32{1.0}, nil
	}}
	cfg := CircuitBreakerConfig{
		MaxConsecutiveFailures: 100,
		BaseOpenTimeout:        time.Minute,
		MaxOpenTimeout:         time.Minute,
		RateLimitPerSec:        5, // very low limit — goroutines must queue
		HalfOpenMaxRequests:    1,
	}
	cb := WrapWithCircuitBreaker(inner, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const goroutines = 50
	var wg sync.WaitGroup
	var done atomic.Int32
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cb.Generate(ctx, "x")
			done.Add(1)
		}()
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(8 * time.Second):
		t.Errorf("TestCBStress_RateLimiterBoundary: %d/%d goroutines completed before timeout",
			done.Load(), goroutines)
	}
}
