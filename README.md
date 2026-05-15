# PCMI – Persistent Cognitive Memory Infrastructure

Memory lives **outside** agents. Agents are ephemeral; this layer is persistent, runtime-agnostic, and reachable over HTTP.

## Features

- Hierarchical paths (`ltree`), JSONB metadata, pgvector embeddings  
- Append-only rows with `valid_from` / `valid_to`; re-store on the same path soft-closes the prior version and bumps `version`  
- Hybrid retrieval: `ltree` scope + PostgreSQL FTS (`ts_rank_cd`) + optional semantic ranking when `OPENAI_API_KEY` is set  
- Point-in-time retrieve via `as_of` (temporal slice, not only `valid_to IS NULL`)  
- RBAC via `X-API-Key` (`readonly` cannot POST memories), audit log API (`GET /v1/audit`), multi-tenant RLS  
- Temporal rollback (`POST /v1/memories/rollback`) restores a prior version as a new append-only row  
- Historical reconstruction (`GET /v1/memories/history?path=...`) lists all versions for a path  
- Universal event ingestion (`POST /v1/events`) persists agent/runtime events and fans out over Redis/SSE  
- Cross-agent memory scopes (`source_agent_id` on store/retrieve) and multi embedding-space labels (`embedding_space`)  
- Per-API-key rate limiting (configurable via `RATE_LIMIT_RPM`; disable with `RATE_LIMIT_DISABLED`)  
- Redis event fan-out (`memory.stored`, `memory.updated`, `knowledge.distilled`) and worker-driven embedding + distillation  
- SSE event stream at `GET /v1/events` (SDK `subscribe()`)  
- Distilled knowledge **versioning** (`version` on `GET /v1/distilled`; new distillations bump version per path)  
- **Webhook** delivery on Redis events (`POST/GET /v1/webhooks`)  
- **Encrypted** memory content (`encrypt_content` or `metadata.sensitive`) with `PCMI_ENCRYPTION_KEY`  
- Embedding **migration** API (`POST /v1/embeddings/migrate`) queues re-embedding by path prefix  
- Background **pruning** of superseded rows (`PRUNE_RETENTION_DAYS`)  
- Kubernetes samples under `deploy/k8s/`  
- **Event schema registry** (`GET /v1/events/schemas`) with strict payload validation on ingest  
- **Webhook retry + dead-letter** queue (`GET /v1/webhooks/dead-letter`) with exponential backoff  
- **Memory summarization** (`POST /v1/memories/summarize`) — extractive by default, LLM when `OPENAI_API_KEY` is set  
- API/worker **connection pool metrics** on health endpoints  
- **gRPC API** (`GRPC_PORT`, default 50051) — Store, Retrieve, Health alongside REST (shared service layer)  
- **Consolidation worker** — merges ≥3 related memories under a path prefix into `{prefix}.consolidated`  
- **BM25-tuned hybrid** — `pcmi_bm25_rank` + `websearch_to_tsquery` for improved keyword leg  
- **Multi-tenant admin API** — `GET/POST /v1/admin/tenants`, API key create/rotate (`admin` role)  
- **GET /v1/memories/{path}** — single memory with optional `version` / `as_of`  
- **Batch** — `POST /v1/memories/batch`, `POST /v1/retrieve/batch`  
- **Export/import** — tenant-scoped JSON migration (`POST /v1/memories/export`, `/import`)  
- **Prometheus** — `GET /metrics` (no auth)  
- Ops docs: `docs/failure-modes.md`, `docs/scalability.md`  
- **Refine API** — `POST /v1/memories/refine` queues distillation for a path prefix (SDK `refine()`)  
- **Memory lineage** — `GET /v1/memories/lineage`, `GET /v1/distilled/{id}/lineage`  
- **Memory links** — graph edges between paths (`POST/GET /v1/memories/links`)  
- **Tenant stats** — `GET /v1/stats` (counts, expiring-soon)  
- **Tag filters** on retrieve (`tags`, `tags_match`: `any` | `all`)  
- **TTL expiry** — `expires_at` on store; background expiry worker soft-closes rows  , `docs/architecture.md`  
- **Refine API** (`POST /v1/memories/refine`) — queue distillation for a path prefix (SDK `refine()`)  
- **Lineage** — `GET /v1/memories/lineage?path=...`, `GET /v1/distilled/{id}/lineage`  
- **Memory links** — `POST/GET /v1/memories/links` between paths  
- **Tenant stats** — `GET /v1/stats` (counts, expiring soon)  
- **Tag filters** on retrieve (`tags`, `tags_match=all|any`)  
- **Memory TTL** — optional `expires_at` on store; expiry worker soft-closes expired rows  

## Quick start

```bash
cd pcmi
cp .env.example .env
# optional: set OPENAI_API_KEY for embeddings + semantic retrieve
docker compose up -d --build

# Liveness (no API key)
curl -s http://localhost:8000/health

# Store (default dev key from migration 003)
curl -s -X POST http://localhost:8000/v1/memories \
  -H "Content-Type: application/json" \
  -H "X-API-Key: testkey123" \
  -d '{"path":"root.test.demo","content":"Hello PCMI","metadata":{"source":"readme"},"embedding_model":"text-embedding-3-small"}'

curl -s -X POST http://localhost:8000/v1/retrieve \
  -H "Content-Type: application/json" \
  -H "X-API-Key: testkey123" \
  -d '{"path_prefix":"root.test","query":"","limit":10}'

curl -s "http://localhost:8000/v1/distilled?path_prefix=root.test" \
  -H "X-API-Key: testkey123"

# Live events (SSE; optional ?types=memory.stored,knowledge.distilled)
curl -sN http://localhost:8000/v1/events \
  -H "X-API-Key: testkey123" \
  -H "Accept: text/event-stream"
```

OpenAPI: `docs/openapi.yaml`

## Layout

- `cmd/api` — Fiber HTTP API  
- `cmd/worker` — embeddings + distillation + Redis subscriber  
- `migrations` — Postgres schema (run via Docker init or your migrator)  
- `sdk/python`, `sdk/typescript` — thin clients  

## License

See `LICENSE`.
