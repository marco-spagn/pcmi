# gRPC vs HTTP API surface

PCMI exposes a **dual transport** for memory and operational APIs. **Admin** tenant/API-key management and **Prometheus metrics** remain **HTTP-only**.

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

### Store parity (v1.25.0)

gRPC `Store` / `BatchStore` accept the same store fields as REST JSON, including:

- `tags`, `embedding_model`, `embedding_space`, `source_agent_id`
- `encrypt_content`, `expires_at_rfc3339`
- **`embedding`** — optional client-supplied vector (`repeated float`); must match DB dimension (1536). If omitted, the worker may backfill when `OPENAI_API_KEY` is configured.

Responses return `id`, `status`, `version`, optional `superseded_id` (embeddings are not echoed back, same as REST).

### Retrieve parity (v1.26.0)

`Retrieve`, `BatchRetrieve`, and `RetrieveStream` return `RetrieveEntry` fields aligned with REST `MemoryEntry`: metadata, tags, model/space labels, temporal fields, agent/event IDs, `content_encrypted`, and optional `embedding` vector when stored.

### Operational parity (v1.29.0)

Unary RPCs cover refine, links, stats, events ingest, webhooks, embedding migration, rollback, summarize, history, lineage, distilled list, audit, export/import. Complex JSON shapes use `JSONResponse.json` (UTF-8 JSON object) where noted in proto.

`StreamEvents` streams `StreamEventMsg` messages (`type` + `payload_json`) — equivalent to SSE `data:` frames.

## HTTP-only endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/admin/tenants` | Create tenant (admin) |
| GET | `/v1/admin/tenants` | List tenants |
| GET/POST | `/v1/admin/api-keys` (+ rotate) | API key management |
| GET | `/metrics` | Prometheus (not under `/v1`) |

**SDK coverage:** Python and TypeScript clients wrap HTTP routes in `sdk/HTTP-API.md` (including admin). gRPC clients use `proto/pcmi/v1/memory.proto` and generated stubs.

## When to use which

- **gRPC**: agents, batch workloads, streaming retrieve/events, full memory + ops surface without SSE/HTTP overhead.
- **HTTP**: admin bootstrap, Prometheus scraping, OpenAPI/SDK ergonomics, browser SSE if preferred over gRPC stream.

See also: `docs/openapi.yaml`, `proto/pcmi/v1/memory.proto`, `docs/CODEBASE.md`.
