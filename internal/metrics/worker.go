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
