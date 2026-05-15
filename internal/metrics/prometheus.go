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

func IncStore()     { memoryStoresTotal.Inc() }
func IncRetrieve() { memoryRetrievesTotal.Inc() }
