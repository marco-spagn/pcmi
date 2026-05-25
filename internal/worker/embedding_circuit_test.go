package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/metrics"
)

type slowFailProvider struct {
	delay time.Duration
	calls atomic.Int32
}

func (p *slowFailProvider) Generate(ctx context.Context, text string) ([]float32, error) {
	p.calls.Add(1)
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.delay):
		}
	}
	return nil, errors.New("openai down")
}

// TestEmbeddingWorker_OpenAIDown_DoesNotBlockWorkerLoop verifies the worker's
// continue-on-error path stays fast when the circuit is open (no slow upstream).
func TestEmbeddingWorker_OpenAIDown_DoesNotBlockWorkerLoop(t *testing.T) {
	t.Parallel()

	inner := &slowFailProvider{delay: 200 * time.Millisecond}
	cfg := embedding.CircuitBreakerConfig{
		MaxConsecutiveFailures: 2,
		BaseOpenTimeout:        50 * time.Millisecond,
		MaxOpenTimeout:         200 * time.Millisecond,
		RateLimitPerSec:        1000,
		HalfOpenMaxRequests:    1,
	}
	prov := embedding.WrapWithCircuitBreaker(inner, cfg)
	ctx := context.Background()

	// Trip the circuit (same as worker would on repeated failures).
	for i := 0; i < 2; i++ {
		_, _ = prov.Generate(ctx, "x")
	}

	// While still open (before half-open probe), calls must fast-fail without slow upstream.
	window := 25 * time.Millisecond
	start := time.Now()
	iterations := 0
	for time.Since(start) < window {
		callStart := time.Now()
		_, _ = prov.Generate(ctx, "x")
		if time.Since(callStart) > 15*time.Millisecond {
			t.Fatalf("iteration %d blocked too long while circuit open", iterations)
		}
		iterations++
	}
	if iterations < 3 {
		t.Fatalf("expected several fast iterations, got %d", iterations)
	}
	if inner.calls.Load() > 6 {
		t.Fatalf("too many upstream calls (%d); circuit should fast-fail", inner.calls.Load())
	}
}

func TestEmbeddingWorker_MetricsExposed_CircuitState(t *testing.T) {
	// Not parallel: reads the global WorkerRegistry gauge which is shared across
	// all tests in this binary. Running concurrently with OpenAIDown (which also
	// opens a circuit and later transitions to half-open) resets the "open" gauge
	// to 0 before this test can gather it.

	inner := &slowFailProvider{}
	cfg := embedding.CircuitBreakerConfig{
		MaxConsecutiveFailures: 2,
		BaseOpenTimeout:        50 * time.Millisecond,
		MaxOpenTimeout:         200 * time.Millisecond,
		RateLimitPerSec:        1000,
		HalfOpenMaxRequests:    1,
	}
	prov := embedding.WrapWithCircuitBreaker(inner, cfg)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_, _ = prov.Generate(ctx, "x")
	}
	_, _ = prov.Generate(ctx, "x")

	mfs, err := metrics.WorkerRegistry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var foundOpen bool
	for _, mf := range mfs {
		if mf.GetName() != "pcmi_embedding_circuit_state" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "state" && lp.GetValue() == metrics.EmbeddingCircuitOpen {
					if m.GetGauge().GetValue() == 1 {
						foundOpen = true
					}
				}
			}
		}
	}
	if !foundOpen {
		t.Fatal("pcmi_embedding_circuit_state{state=\"open\"} not set to 1")
	}
}
