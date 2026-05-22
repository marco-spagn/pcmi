package event

import (
	"os"
	"strings"
)

const (
	// StreamKey is the Redis stream for durable PCMI events.
	StreamKey = "pcmi:events"
	// DLQStreamKey holds events that exceeded max delivery attempts.
	DLQStreamKey = "pcmi:events:dlq"
	// WorkerConsumerGroup is the consumer group for pcmi-worker instances.
	WorkerConsumerGroup = "pcmi-workers"

	EnvEventBackend = "EVENT_BACKEND"
	BackendStreams  = "streams"
	BackendPubSub   = "pubsub"
)

// EventBackend returns the active event transport (default: streams).
func EventBackend() string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(EnvEventBackend)))
	switch v {
	case "", BackendStreams:
		return BackendStreams
	case BackendPubSub:
		return BackendPubSub
	default:
		return BackendStreams
	}
}

func useStreams() bool { return EventBackend() == BackendStreams }
