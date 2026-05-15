# PCMI Failure Modes (v1.14)

This document describes how PCMI behaves when dependencies fail or limits are hit.

## PostgreSQL unavailable

- **API**: `POST /v1/memories`, `POST /v1/retrieve`, and related endpoints return **500** with an error message. Health may still respond if the process is up but DB ping fails on data paths.
- **Worker**: Embedding, distillation, consolidation, and pruning loops log errors and retry on the next tick; no data loss for already-committed rows.
- **Mitigation**: Run Postgres with HA (Patroni, cloud RDS Multi-AZ), connection pooling (PgBouncer), and alert on `pcmi_http_requests_total{status="500"}`.

## Redis unavailable

- **API**: Stores and retrieves against Postgres **continue to work**. Event publish (`memory.stored`) fails silently in logs; SSE subscribers and webhooks do not receive live fan-out.
- **Worker**: Distillation/consolidation triggers driven by Redis events stall until Redis returns; periodic consolidation ticker still processes `consolidation_runs` queue rows.
- **Mitigation**: Redis Sentinel or managed Redis with persistence; monitor Redis connectivity from API pods.

## OpenAI / embedding provider errors

- **Retrieve**: Semantic ranking falls back to FTS/BM25-only or structural listing when embedding generation fails.
- **Worker**: Embedding worker logs and skips rows; migration API marks rows for retry.
- **Mitigation**: Set quotas, use `embedding_space` labels for gradual model migration, cache embeddings on store when possible.

## Invalid API key or expired key

- Returns **401** for all authenticated routes. `/health`, `/v1/health`, and `/metrics` remain open.

## Read-only API key

- **403** on mutating routes (`POST` memories, batch, import, rollback, admin writes).

## Rate limiting

- When `RATE_LIMIT_RPM` is exceeded, returns **429**. Disable in dev with `RATE_LIMIT_DISABLED=true`.

## Webhook delivery failure

- Retries with exponential backoff; exhausted deliveries land in **dead-letter** (`GET /v1/webhooks/dead-letter`).

## gRPC

- Uses the same API key resolution as HTTP (`x-api-key` metadata or `api_key` field). Invalid keys return `Unauthenticated`. If `GRPC_PORT` is unset, gRPC listens on **50051** alongside REST.

## Consolidation

- Requires at least **3** active memories under a parent prefix. Failures mark `consolidation_runs.status=failed` with `error_message`; operators can inspect and re-trigger by inserting a new pending run.

## Tenant isolation (RLS)

- Every authenticated request sets `app.current_tenant` via `set_tenant_context`. Cross-tenant reads/writes are rejected at the database layer when RLS policies are enabled.
