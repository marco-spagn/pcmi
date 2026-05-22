// Package event definisce i tipi di evento Redis/SSE e implementa publish/subscribe su Redis
// Redis pub/sub (memory_events) o Redis Streams (pcmi:events, default via EVENT_BACKEND=streams).
// La notifica webhook opzionale viene collegata da cmd/api dopo InitRedis.
package event
