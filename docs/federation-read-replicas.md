# Multi-tenant federation and PostgreSQL read replicas

PCMI is **multi-tenant** on a single database: every row is tied to `tenant_id` and RLS policies depend on `set_tenant_context`, set by middleware after `X-API-Key` validation.

## Operational goal

Under **read-heavy** workloads (many agents or orchestrators reading memory in parallel), you can add a PostgreSQL **streaming replica** and route read-only queries to the replica, keeping **writes, transactions, and readiness probes** on the primary.

Environment variable on the API:

- **`DATABASE_URL`** — connection to the **primary** (required).
- **`DATABASE_READ_URL`** — connection to the **read replica** (optional). If absent, everything stays on the primary.

## What is routed

| Path | Pool |
|------|------|
| `Store`, batch store, import, rollback, event ingest, admin, audit insert, webhook, link **create** | Primary |
| `Retrieve`, export query, history, stats, lineage, link **list**, `GetByPath` "current" | Replica if configured |
| `GetHistoricalVersion` (version / `as_of`, also used in rollback) | **Primary** (consistency with writes) |

gRPC **Retrieve** inherits the same `MemoryService` → uses the replica for SELECTs like HTTP.

## Limitations and best practices

1. **Replication lag**: immediately after a `POST /v1/memories`, a `POST /v1/retrieve` on the replica may not yet see the row. For "read-your-writes", repeat the read on the primary (no dedicated header exists; recommended pattern: brief retry or accept eventual consistency).
2. **RLS**: the replica must repeat the same policies as the primary (same schema and application role); in Postgres, hot standbys apply the same policies to replicated rows.
3. **Readiness**: `GET /v1/ready` continues to ping the **primary** and Redis; the replica's mere state does not block the probe (if the replica is down, consider separate monitoring or a custom health check).
4. **Worker**: the worker process uses only the primary (`DATABASE_URL`), to avoid write/distillation jobs on lagged copies.

## Kubernetes (sketch)

- Internal service to the primary for `DATABASE_URL`.
- Separate service (or URI) for the replica in `DATABASE_READ_URL` in the API `ConfigMap`.
- Do not expose the replica as a public endpoint unless necessary.

## Orchestration examples

To start jobs that call PCMI via HTTP from Celery or Temporal, see [`examples/README.md`](../examples/README.md).
