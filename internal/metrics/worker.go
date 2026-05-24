package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// WorkerRegistry is the Prometheus registry for pcmi-worker (separate from API Registry).
var WorkerRegistry = prometheus.NewRegistry()

func init() {
	WorkerRegistry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

var workerRedisEvents = promauto.With(WorkerRegistry).NewCounterVec(
	prometheus.CounterOpts{
		Name: "pcmi_worker_redis_events_total",
		Help: "Redis memory_events messages handled by pcmi-worker",
	},
	[]string{"event_type"},
)

// IncWorkerRedisEvent increments the counter for one consumed Redis event type.
func IncWorkerRedisEvent(eventType string) {
	if eventType == "" {
		eventType = "unknown"
	}
	workerRedisEvents.WithLabelValues(eventType).Inc()
}

// ── Distillation metrics ──────────────────────────────────────────────────────

var (
	// distillationDuration tracks how long each LLM distillation job takes.
	distillationDuration = promauto.With(WorkerRegistry).NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pcmi_distillation_duration_seconds",
			Help:    "Duration of a single distillation LLM job in seconds",
			Buckets: prometheus.DefBuckets, // 0.005 … 10s
		},
	)

	// distillationTotal counts completed jobs by outcome.
	distillationTotal = promauto.With(WorkerRegistry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "pcmi_distillation_total",
			Help: "Total distillation jobs by status (ok|error|duplicate|skipped)",
		},
		[]string{"status"},
	)

	// distillationSourcesPerJob tracks how many raw memories went into each job.
	distillationSourcesPerJob = promauto.With(WorkerRegistry).NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pcmi_distillation_sources_per_job",
			Help:    "Number of raw memory entries fed into each distillation job",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 200},
		},
	)

	// distillationActiveJobs is the number of LLM jobs currently running.
	distillationActiveJobs = promauto.With(WorkerRegistry).NewGauge(
		prometheus.GaugeOpts{
			Name: "pcmi_distillation_active_jobs",
			Help: "Number of distillation LLM jobs currently executing",
		},
	)

	// distillationQueuedJobs is the number of goroutines waiting for a semaphore slot.
	distillationQueuedJobs = promauto.With(WorkerRegistry).NewGauge(
		prometheus.GaugeOpts{
			Name: "pcmi_distillation_queued_jobs",
			Help: "Number of distillation jobs queued and waiting for a concurrency slot",
		},
	)
)

// ObserveDistillationJob records duration and status of one completed job.
func ObserveDistillationJob(durationSecs float64, status string) {
	distillationDuration.Observe(durationSecs)
	distillationTotal.WithLabelValues(status).Inc()
}

// ObserveDistillationSources records the number of sources used by one job.
func ObserveDistillationSources(n int) {
	distillationSourcesPerJob.Observe(float64(n))
}

// IncDistillationActive increments the active-jobs gauge.
func IncDistillationActive() { distillationActiveJobs.Inc() }

// DecDistillationActive decrements the active-jobs gauge.
func DecDistillationActive() { distillationActiveJobs.Dec() }

// IncDistillationQueued increments the queued-jobs gauge.
func IncDistillationQueued() { distillationQueuedJobs.Inc() }

// DecDistillationQueued decrements the queued-jobs gauge.
func DecDistillationQueued() { distillationQueuedJobs.Dec() }

// ── Webhook delivery metrics ──────────────────────────────────────────────────

var (
	webhookDeadLetterTotal = promauto.With(WorkerRegistry).NewCounter(
		prometheus.CounterOpts{
			Name: "pcmi_webhook_dead_letter_total",
			Help: "Webhook deliveries moved to dead-letter after exhausting retries",
		},
	)
	webhookPendingOldestAge = promauto.With(WorkerRegistry).NewGauge(
		prometheus.GaugeOpts{
			Name: "pcmi_webhook_pending_oldest_age_seconds",
			Help: "Age in seconds of the oldest pending webhook delivery",
		},
	)
)

// IncWebhookDeadLetter increments when a delivery enters dead-letter status.
func IncWebhookDeadLetter() { webhookDeadLetterTotal.Inc() }

// SetWebhookPendingOldestAge sets the age gauge (0 when no pending deliveries).
func SetWebhookPendingOldestAge(seconds float64) { webhookPendingOldestAge.Set(seconds) }

// ── Embedding circuit breaker metrics ───────────────────────────────────────────

const (
	EmbeddingCircuitClosed   = "closed"
	EmbeddingCircuitOpen     = "open"
	EmbeddingCircuitHalfOpen = "half_open"

	EmbeddingResultSuccess      = "success"
	EmbeddingResultError        = "error"
	EmbeddingResultCircuitOpen  = "circuit_open"
	EmbeddingResultRateLimited  = "rate_limited"
)

var (
	embeddingCircuitState = promauto.With(WorkerRegistry).NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pcmi_embedding_circuit_state",
			Help: "Embedding circuit breaker state (1 = active state)",
		},
		[]string{"state"},
	)
	embeddingRequestsTotal = promauto.With(WorkerRegistry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "pcmi_embedding_requests_total",
			Help: "Embedding provider requests by result",
		},
		[]string{"result"},
	)
	embeddingLatency = promauto.With(WorkerRegistry).NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pcmi_embedding_latency_seconds",
			Help:    "Embedding provider request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func init() {
	for _, s := range []string{EmbeddingCircuitClosed, EmbeddingCircuitOpen, EmbeddingCircuitHalfOpen} {
		embeddingCircuitState.WithLabelValues(s).Set(0)
	}
}

// SetEmbeddingCircuitState marks exactly one circuit state gauge as 1.
func SetEmbeddingCircuitState(state string) {
	for _, s := range []string{EmbeddingCircuitClosed, EmbeddingCircuitOpen, EmbeddingCircuitHalfOpen} {
		v := 0.0
		if s == state {
			v = 1
		}
		embeddingCircuitState.WithLabelValues(s).Set(v)
	}
}

// IncEmbeddingRequest increments the embedding request counter for one result label.
func IncEmbeddingRequest(result string) {
	embeddingRequestsTotal.WithLabelValues(result).Inc()
}

// ObserveEmbeddingLatency records one embedding call duration.
func ObserveEmbeddingLatency(seconds float64) {
	embeddingLatency.Observe(seconds)
}
