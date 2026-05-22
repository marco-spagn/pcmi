package event

import (
	"strings"
	"sync"
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

var (
	backendMu           sync.RWMutex
	configuredBackend   = BackendStreams
)

// SetEventBackend selects streams or pub/sub (from config.Load().EventBackend).
func SetEventBackend(backend string) {
	backendMu.Lock()
	defer backendMu.Unlock()
	switch strings.TrimSpace(strings.ToLower(backend)) {
	case BackendPubSub:
		configuredBackend = BackendPubSub
	default:
		configuredBackend = BackendStreams
	}
}

// EventBackend returns the active event transport (default: streams).
func EventBackend() string {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return configuredBackend
}

func useStreams() bool { return EventBackend() == BackendStreams }
