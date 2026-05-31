package metrics

import (
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry is the dedicated Prometheus registry for PCMI.
var Registry = prometheus.NewRegistry()

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

var (
	memoryStoresTotal = promauto.With(Registry).NewCounter(
		prometheus.CounterOpts{
			Name: "pcmi_memory_stores_total",
			Help: "Total memory store operations",
		},
	)
	memoryRetrievesTotal = promauto.With(Registry).NewCounter(
		prometheus.CounterOpts{
			Name: "pcmi_memory_retrieves_total",
			Help: "Total memory retrieve operations",
		},
	)
)

// Middleware is a no-op placeholder; HTTP RED metrics removed to avoid duplicate-sample gather errors.
func Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error { return c.Next() }
}

var (
	graphTraversalTotal = promauto.With(Registry).NewCounter(
		prometheus.CounterOpts{
			Name: "pcmi_graph_traversal_total",
			Help: "Total graph traversal operations executed",
		},
	)
	graphTraversalDuration = promauto.With(Registry).NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pcmi_graph_traversal_duration_seconds",
			Help:    "Graph traversal duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func IncStore()     { memoryStoresTotal.Inc() }
func IncRetrieve() { memoryRetrievesTotal.Inc() }

// IncGraphTraversal increments the graph traversal counter.
func IncGraphTraversal() { graphTraversalTotal.Inc() }

// ObserveGraphTraversal records the duration of a graph traversal.
func ObserveGraphTraversal(seconds float64) { graphTraversalDuration.Observe(seconds) }
