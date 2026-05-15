package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry is the dedicated Prometheus registry for PCMI (avoids DefaultRegisterer collisions).
var Registry = prometheus.NewRegistry()

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

var (
	httpRequestsTotal = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "pcmi_http_requests_total",
			Help: "Total HTTP requests by method, path pattern, and status",
		},
		[]string{"method", "path", "status"},
	)
	httpRequestDuration = promauto.With(Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pcmi_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
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

// Middleware records request counts and latency for Prometheus.
func Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		status := strconv.Itoa(c.Response().StatusCode())
		path := c.Route().Path
		if path == "" {
			path = c.Path()
		}
		httpRequestsTotal.WithLabelValues(c.Method(), path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Method(), path).Observe(time.Since(start).Seconds())
		return err
	}
}

func IncStore()    { memoryStoresTotal.Inc() }
func IncRetrieve() { memoryRetrievesTotal.Inc() }
