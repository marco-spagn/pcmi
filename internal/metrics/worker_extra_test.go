package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// scrapeWorkerMetrics builds a test HTTP handler on /metrics for WorkerRegistry,
// sends a GET request, and returns the response body as a string.
func scrapeWorkerMetrics(t *testing.T, label string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(WorkerRegistry, promhttp.HandlerOpts{EnableOpenMetrics: false}))
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%s: status=%d", label, rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	return string(body)
}

func TestDistillationMetrics(t *testing.T) {
	ObserveDistillationJob(2.5, "ok")
	ObserveDistillationSources(10)
	IncDistillationActive()
	IncDistillationActive()
	DecDistillationActive()
	IncDistillationQueued()
	DecDistillationQueued()

	s := scrapeWorkerMetrics(t, "distillation")

	for _, want := range []string{
		"pcmi_distillation_duration_seconds",
		"pcmi_distillation_total",
		"pcmi_distillation_sources_per_job",
		"pcmi_distillation_active_jobs",
		"pcmi_distillation_queued_jobs",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing metric %q in worker registry", want)
		}
	}
}

func TestWebhookDeadLetterMetric(t *testing.T) {
	IncWebhookDeadLetter()
	SetWebhookPendingOldestAge(300)

	s := scrapeWorkerMetrics(t, "webhook")

	for _, want := range []string{
		"pcmi_webhook_dead_letter_total",
		"pcmi_webhook_pending_oldest_age_seconds",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing metric %q in worker registry", want)
		}
	}
}

func TestEmbeddingCircuitState(t *testing.T) {
	SetEmbeddingCircuitState(EmbeddingCircuitOpen)
	IncEmbeddingRequest(EmbeddingResultSuccess)
	ObserveEmbeddingLatency(0.5)

	s := scrapeWorkerMetrics(t, "embedding_circuit")

	for _, want := range []string{
		"pcmi_embedding_circuit_state",
		"pcmi_embedding_requests_total",
		"pcmi_embedding_latency_seconds",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing metric %q in worker registry", want)
		}
	}
}

func TestSetEmbeddingCircuitState_unknown(t *testing.T) {
	// An unknown state string should set all states to 0 (graceful default).
	SetEmbeddingCircuitState("unknown_state")

	s := scrapeWorkerMetrics(t, "embedding_unknown")

	if !strings.Contains(s, "pcmi_embedding_circuit_state") {
		t.Fatal("embedding circuit state metric missing")
	}
	// All three known states should appear with value 0.
	for _, state := range []string{EmbeddingCircuitClosed, EmbeddingCircuitOpen, EmbeddingCircuitHalfOpen} {
		if !strings.Contains(s, state) {
			t.Errorf("expected label %q to appear in circuit state metric", state)
		}
	}
}

func TestIncWorkerRedisEvent_empty(t *testing.T) {
	// Empty event type defaults to "unknown".
	IncWorkerRedisEvent("")

	s := scrapeWorkerMetrics(t, "redis_empty")

	if !strings.Contains(s, "pcmi_worker_redis_events_total") {
		t.Fatalf("missing redis events counter:\n%s", s[:min(600, len(s))])
	}
}

func TestIncWorkerRedisEvent_known(t *testing.T) {
	IncWorkerRedisEvent("memory.stored")

	s := scrapeWorkerMetrics(t, "redis_known")

	if !strings.Contains(s, "pcmi_worker_redis_events_total") {
		t.Fatalf("missing redis events counter")
	}
}
