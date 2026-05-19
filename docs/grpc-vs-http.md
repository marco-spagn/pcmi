# gRPC vs HTTP API surface

PCMI exposes a **dual transport** for memory, operational, admin, and metrics APIs. Choose based on client capabilities and workload shape — not feature availability alone.

| Transport | Best for |
|-----------|----------|
| **gRPC** | Agents, batch workloads, streaming retrieve/events, admin automation without browsers |
| **HTTP** | OpenAPI/SDK ergonomics, SSE in browsers, Prometheus scrape at `GET /metrics`, embedded **admin UI** |

Protos: [`proto/pcmi/v1/`](../proto/pcmi/v1/) · REST contract: [`openapi.yaml`](openapi.yaml)

---

## gRPC (`pcmi.v1.MemoryService`)

| RPC | REST equivalent |
|-----|-----------------|
| `Store` | `POST /v1/memories` |
| `BatchStore` | `POST /v1/memories/batch` |
| `Retrieve` | `POST /v1/retrieve` |
| `BatchRetrieve` | `POST /v1/retrieve/batch` |
| `RetrieveStream` | same as retrieve (streamed entries) |
| `Health` | `GET /v1/health` |
| `Ready` | `GET /v1/ready` |
| `GetMemory` | `GET /v1/memories/{path}` |
| `Compact` | `POST /v1/memories/compact` |
| `Refine` | `POST /v1/memories/refine` |
| `CreateLink` | `POST /v1/memories/links` |
| `ListLinks` | `GET /v1/memories/links` |
| `GetStats` | `GET /v1/stats` |
| `IngestEvent` | `POST /v1/events` |
| `ListEventSchemas` | `GET /v1/events/schemas` |
| `StreamEvents` | `GET /v1/events` (**SSE** → gRPC server stream) |
| `RegisterWebhook` | `POST /v1/webhooks` |
| `ListWebhooks` | `GET /v1/webhooks` |
| `ListWebhookDeadLetter` | `GET /v1/webhooks/dead-letter` |
| `MigrateEmbeddings` | `POST /v1/embeddings/migrate` |
| `Rollback` | `POST /v1/memories/rollback` |
| `Summarize` | `POST /v1/memories/summarize` |
| `GetHistory` | `GET /v1/memories/history` |
| `GetMemoryLineage` | `GET /v1/lineage/memory` |
| `GetDistilledLineage` | `GET /v1/lineage/distilled/:id` |
| `ListDistilled` | `GET /v1/distilled` |
| `ListAudit` | `GET /v1/audit` |
| `ExportMemories` | `POST /v1/memories/export` |
| `ImportMemories` | `POST /v1/memories/import` |

### Store parity (v1.25.0+)

gRPC `Store` / `BatchStore` accept the same store fields as REST JSON, including:

- `tags`, `embedding_model`, `embedding_space`, `source_agent_id`
- `encrypt_content`, `expires_at_rfc3339`
- **`embedding`** — optional client-supplied vector (`repeated float`); must match DB dimension (1536). If omitted, the worker may backfill when `OPENAI_API_KEY` is configured.

Responses return `id`, `status`, `version`, optional `superseded_id` (embeddings are not echoed back, same as REST).

### Retrieve parity (v1.26.0+)

`Retrieve`, `BatchRetrieve`, and `RetrieveStream` return `RetrieveEntry` fields aligned with REST `MemoryEntry`: metadata, tags, model/space labels, temporal fields, agent/event IDs, `content_encrypted`, and optional `embedding` vector when stored.

Path-only retrieve (no `query`, no vector search) supports opaque **keyset cursors** (`cursor`, `next_cursor`, `has_more`) on HTTP and gRPC.

### Operational parity (v1.29.0+)

Unary RPCs cover refine, links, stats, events ingest, webhooks, embedding migration, rollback, summarize, history, lineage, distilled list, audit, export/import. Complex JSON shapes use `JSONResponse.json` (UTF-8 JSON object) where noted in proto.

`StreamEvents` streams `StreamEventMsg` messages (`type` + `payload_json`) — equivalent to SSE `data:` frames.

---

## gRPC (`pcmi.v1.AdminService`)

Mirrors HTTP `/v1/admin/*` (admin API key required). Registered alongside `MemoryService` in `internal/grpc/server.go`.

| RPC | REST equivalent |
|-----|-----------------|
| `CreateTenant` | `POST /v1/admin/tenants` |
| `ListTenants` | `GET /v1/admin/tenants` |
| `CreateAPIKey` | `POST /v1/admin/api-keys` |
| `RotateAPIKey` | `POST /v1/admin/api-keys/{id}/rotate` |
| `ListAPIKeys` | `GET /v1/admin/api-keys` |

Official Python/TypeScript SDKs still call these routes over **HTTP** — see [`sdk/HTTP-API.md`](../sdk/HTTP-API.md).

---

## gRPC (`pcmi.v1.MetricsService`)

| RPC | REST / ops equivalent |
|-----|------------------------|
| `Scrape` | `GET /metrics` (Prometheus text) |
| `StreamScrape` | chunked scrape stream |
| `GetMetric` | *Unimplemented* — use `Scrape` or HTTP Prometheus |

For standard Prometheus polling, **`GET /metrics`** on the HTTP port remains the usual choice.

---

## HTTP-only (no gRPC equivalent)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/admin/ui` | Embedded HTML admin dashboard (browser) |

All other admin and memory operations listed above are available on **both** transports unless noted.

---

## SDK coverage

| Client | Transports |
|--------|------------|
| Python / TypeScript SDK | HTTP only — full route map in [`sdk/HTTP-API.md`](../sdk/HTTP-API.md) |
| Generated gRPC stubs | `MemoryService`, `AdminService`, `MetricsService` |

See also: [`docs/openapi.yaml`](openapi.yaml), [`docs/CODEBASE.md`](CODEBASE.md).
