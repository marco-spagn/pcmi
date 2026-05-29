// Package handler registers Fiber routes for the PCMI REST API: memories, retrieve, events (SSE),
// webhooks, admin, batch, lineage, links, stats, and refine. Tenants and roles come from middleware;
// heavy business logic is delegated to internal/service and internal/repository.
//
// Convention: more specific GET routes (e.g. /lineage/*, /memories/history) must be
// registered before the wildcard GET /memories/* to prevent Fiber from treating "lineage" as a path.
package handler
