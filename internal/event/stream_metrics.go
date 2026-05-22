package event

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/marco-spagn/pcmi/internal/metrics"
)

var (
	streamPending = promauto.With(metrics.WorkerRegistry).NewGauge(
		prometheus.GaugeOpts{
			Name: "pcmi_stream_pending_total",
			Help: "Pending Redis stream messages in the worker consumer group",
		},
	)
	streamAck = promauto.With(metrics.WorkerRegistry).NewCounter(
		prometheus.CounterOpts{
			Name: "pcmi_stream_ack_total",
			Help: "Redis stream messages acknowledged by pcmi-worker",
		},
	)
	streamDLQ = promauto.With(metrics.WorkerRegistry).NewCounter(
		prometheus.CounterOpts{
			Name: "pcmi_stream_dlq_total",
			Help: "Redis stream messages moved to the dead-letter stream",
		},
	)
)

// SetStreamPending updates the pending-message gauge.
func SetStreamPending(n int) { streamPending.Set(float64(n)) }

// IncStreamAck increments successful ACK counter.
func IncStreamAck() { streamAck.Inc() }

// IncStreamDLQ increments dead-letter counter.
func IncStreamDLQ() { streamDLQ.Inc() }
