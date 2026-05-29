// Package service contains the domain logic used by both the HTTP API and gRPC:
// store/retrieve memories, batch, admin, summarize, event ingest. Depends on repository
// and internal/embedding for vectors; publishes Redis events after store via internal/event.
package service
