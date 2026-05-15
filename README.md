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
