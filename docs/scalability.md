# PCMI Scalability (v1.14)

## Horizontal scaling

| Component | Scale strategy |
|-----------|----------------|
| **API** | Stateless; scale behind load balancer. Share Postgres + Redis. Enable `/metrics` scraping per pod. |
| **Worker** | Multiple replicas consume Redis pub/sub; distillation/consolidation use DB row locks implicitly via single-writer patterns per prefix—expect some duplicate work at scale; safe due to append-only store. |
| **Postgres** | Primary bottleneck for write-heavy workloads. Partition by `tenant_id` at very large scale; index `path`, `content_tsv`, `embedding` already present. |
| **Redis** | Fan-out only; not durable source of truth. |

## Retrieval performance

- **Hybrid (v1.14)**: Vector + `pcmi_bm25_rank` (normalized `ts_rank_cd`) with `websearch_to_tsquery` for keyword leg.
- **Batch retrieve**: Up to **20** queries per request to amortize HTTP overhead.
- **Export**: Cap **5000** rows per export; use path prefixes to shard migrations.

## Connection pools

- API and worker expose pool stats on health endpoints. Tune `pgxpool` max conns per pod so `replicas × max_conns` stays below Postgres `max_connections`.

## gRPC

- Prefer gRPC for high-throughput agents (lower overhead than REST JSON). Same MemoryService layer as HTTP.

## Observability

- Prometheus: `pcmi_http_requests_total`, `pcmi_http_request_duration_seconds`, `pcmi_memory_stores_total`, `pcmi_memory_retrieves_total`.
- Audit log API for per-tenant API usage forensics.

## Multi-tenant admin

- Admin APIs (`/v1/admin/*`) use `SECURITY DEFINER` functions for tenant CRUD and key rotation—limit admin role keys and rotate via `POST /v1/admin/api-keys/:id/rotate`.
