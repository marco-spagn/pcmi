# gRPC vs HTTP API surface

PCMI exposes a **dual transport** for core memory operations. Many operational and product features remain **HTTP-only** by design (SSE, webhooks, admin, graph links, compaction).

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

### Store parity (v1.25.0)

gRPC `Store` / `BatchStore` accept the same store fields as REST JSON, including:

- `tags`, `embedding_model`, `embedding_space`, `source_agent_id`
- `encrypt_content`, `expires_at_rfc3339`
- **`embedding`** — optional client-supplied vector (`repeated float`); must match DB dimension (1536). If omitted, the worker may backfill when `OPENAI_API_KEY` is configured.

Responses return `id`, `status`, `version`, optional `superseded_id` (embeddings are not echoed back, same as REST).

## HTTP-only endpoints

These routes have **no gRPC RPC** today. Use HTTP (or extend proto in a future release if needed).

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/memories/compact` | Trim superseded versions for one path |
| POST | `/v1/memories/refine` | Trigger distillation for a path prefix |
| POST | `/v1/memories/links` | Create cross-memory link |
| GET | `/v1/memories/links` | List links |
| GET | `/v1/stats` | Tenant dashboard stats |
| GET | `/v1/events` | **SSE** event stream |
| GET | `/v1/events/schemas` | Event schema registry |
| POST | `/v1/events` | Ingest universal events |
| POST | `/v1/webhooks` | Register webhook |
| GET | `/v1/webhooks` | List webhooks |
| GET | `/v1/webhooks/dead-letter` | Failed deliveries |
| POST | `/v1/admin/tenants` | Create tenant (admin) |
| GET | `/v1/admin/tenants` | List tenants |
| GET/POST | `/v1/admin/api-keys` (+ rotate) | API key management |
| POST | `/v1/embeddings/migrate` | Queue re-embedding by prefix |
| POST | `/v1/memories/rollback` | Version rollback |
| POST | `/v1/memories/summarize` | LLM summarize under prefix |
| GET | `/v1/memories/history` | All versions for a path |
| GET | `/v1/memories/*` | Get memory by path |
| GET | `/v1/lineage/memory` | Memory lineage |
| GET | `/v1/lineage/distilled/:id` | Distilled lineage |
| GET | `/v1/distilled` | List distilled knowledge |
| GET | `/v1/audit` | Audit log |
| POST | `/v1/memories/export`, `/import` | Bulk export/import |
| GET | `/metrics` | Prometheus (not under `/v1`) |

## When to use which

- **gRPC**: high-throughput agents, batch store/retrieve, streaming retrieve, same auth via `api_key` metadata or field.
- **HTTP**: everything else (admin, webhooks, SSE, compaction, refine, links, stats, OpenAPI/SDK ergonomics).

See also: `docs/openapi.yaml`, `proto/pcmi/v1/memory.proto`, `docs/CODEBASE.md`.
