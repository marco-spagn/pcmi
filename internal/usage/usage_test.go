package usage

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// counterValue reads a Prometheus counter without the testutil package.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := c.Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}

// captureRecorder collects events for one tenant (tests share the global recorder).
type captureRecorder struct {
	mu     sync.Mutex
	events []Event
}

func (c *captureRecorder) Record(ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *captureRecorder) forTenant(tenant string) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []Event
	for _, e := range c.events {
		if e.TenantID == tenant {
			out = append(out, e)
		}
	}
	return out
}

func TestScope(t *testing.T) {
	if tid, op := ScopeFrom(context.Background()); tid != "" || op != "" {
		t.Fatal("empty context should have no scope")
	}
	//nolint:staticcheck // nil context is handled explicitly
	if tid, op := ScopeFrom(nil); tid != "" || op != "" {
		t.Fatal("nil context should have no scope")
	}
	ctx := WithScope(context.Background(), "t1", OpDistillation)
	if tid, op := ScopeFrom(ctx); tid != "t1" || op != OpDistillation {
		t.Fatalf("got %s %s", tid, op)
	}
}

func TestReport_recordsAndCounts(t *testing.T) {
	rec := &captureRecorder{}
	prev := SetRecorder(rec)
	t.Cleanup(func() { SetRecorder(prev) })

	before := counterValue(t, tokensTotal.WithLabelValues("openai", "m-report", OpSummarize, "input"))
	ctx := WithScope(context.Background(), "tenant-report", OpSummarize)
	Report(ctx, "openai", "m-report", 100, 20)
	Report(ctx, "openai", "m-report", -5, -1)

	evs := rec.forTenant("tenant-report")
	if len(evs) != 2 || evs[0].InputTokens != 100 || evs[0].OutputTokens != 20 || evs[0].Operation != OpSummarize {
		t.Fatalf("events: %+v", evs)
	}
	if evs[1].InputTokens != 0 || evs[1].OutputTokens != 0 {
		t.Fatalf("negative tokens not clamped: %+v", evs[1])
	}
	if evs[0].At.IsZero() {
		t.Fatal("At not set")
	}
	if got := counterValue(t, tokensTotal.WithLabelValues("openai", "m-report", OpSummarize, "input")) - before; got != 100 {
		t.Fatalf("input tokens counter delta %v", got)
	}
	if got := counterValue(t, requestsTotal.WithLabelValues("openai", "m-report", OpSummarize)); got < 2 {
		t.Fatalf("requests counter %v", got)
	}
}

func TestReport_labelsFallbackAndTruncate(t *testing.T) {
	rec := &captureRecorder{}
	prev := SetRecorder(rec)
	t.Cleanup(func() { SetRecorder(prev) })

	long := strings.Repeat("x", 200)
	Report(WithScope(context.Background(), "tenant-labels", ""), "", long, 1, 0)
	evs := rec.forTenant("tenant-labels")
	if len(evs) != 1 || evs[0].Operation != OpUnknown || evs[0].Provider != "unknown" || len(evs[0].Model) != maxLabelValueSize {
		t.Fatalf("events: %+v", evs)
	}
}

func TestReport_noRecorderStillCounts(t *testing.T) {
	prev := SetRecorder(nil)
	t.Cleanup(func() { SetRecorder(prev) })
	before := counterValue(t, requestsTotal.WithLabelValues("p-norec", "m", OpUnknown))
	Report(context.Background(), "p-norec", "m", 1, 1)
	if counterValue(t, requestsTotal.WithLabelValues("p-norec", "m", OpUnknown))-before != 1 {
		t.Fatal("counter not incremented without recorder")
	}
}
