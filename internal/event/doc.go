// Package event defines Redis/SSE event types and implements publish/subscribe on Redis:
// Redis pub/sub (memory_events) or Redis Streams (pcmi:events, default via EVENT_BACKEND=streams).
// Optional webhook notification is wired up by cmd/api after InitRedis.
package event
