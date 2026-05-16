# PCMI Architecture

Persistent Cognitive Memory Infrastructure (PCMI) separates **ephemeral agents** from **durable memory**.

## Principles

- **Runtime agnostic**: HTTP/gRPC APIs only; no LangChain/LangGraph coupling in core.
- **Append-only cognition**: memories version via `valid_from` / `valid_to`; rollback creates new rows.
- **Hierarchical paths**: PostgreSQL `ltree` for scoped retrieval and domain isolation.
- **Hybrid retrieval**: structural prefix + BM25 (`tsvector`) + optional pgvector semantic rank.
- **Event-driven workers**: Redis pub/sub fans out store/update/refine events to embedding, distillation, consolidation, expiry, and webhooks.

## Components

| Component | Role |
|-----------|------|
| `cmd/api` | Fiber REST API, SSE `/v1/events`, Prometheus `/metrics`, optional gRPC, optional OpenTelemetry OTLP traces |
| `cmd/worker` | Embeddings, distillation, pruning, consolidation, TTL; health + **Prometheus `/metrics`** on :8081 |
| PostgreSQL | `memory_entries`, `distilled_knowledge`, `memory_links`, RLS per tenant |
| Redis | `memory_events` channel for cross-process fan-out |
| SDKs | Python / TypeScript thin clients (`store`, `retrieve`, `refine`, `subscribe`) |

## Data flow

```text
Agent → POST /v1/memories → API → Postgres (append version)
                              ↓
                         Redis memory.stored
                              ↓
                    Worker: embed + distill + consolidate
                              ↓
                         Redis knowledge.distilled → SSE / webhooks
```

## v1.15 additions

- **POST /v1/memories/refine** — queue distillation for a path prefix (`memory.refine.requested`).
- **GET /v1/lineage/memory** — version history + derived distilled rows for a path.
- **GET /v1/lineage/distilled/{id}** — trace distilled knowledge to source memories.
- **POST/GET /v1/memories/links** — graph edges between paths (`related`, custom types).
- **GET /v1/stats** — tenant counters (active, superseded, distilled, links, expiring soon).
- **Tag filters** on retrieve (`tags`, `tags_match=all|any`).
- **TTL** `expires_at` on store + `expire_memory_entries()` worker.

## Security

- Multi-tenant RLS via `set_tenant_context(uuid)` per request.
- API keys hashed (SHA-256); roles: `readonly`, `write`, `admin`.
- Optional field encryption (`PCMI_ENCRYPTION_KEY`) for sensitive content.

## Deployment

- Docker Compose for local/full stack.
- Kubernetes samples under `deploy/k8s/` (API, worker, config, secrets).
- CI: `golangci-lint`, unit tests, integration-smoke (Postgres + Redis), optional OpenAI E2E.

See also: `docs/failure-modes.md`, `docs/scalability.md`, `docs/openapi.yaml`, **`docs/CODEBASE.md`** (mappa del codice), **`docs/MIGRATIONS.md`** (SQL).
