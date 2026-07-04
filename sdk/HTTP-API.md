# HTTP API ↔ SDK method mapping

Official **Python and TypeScript SDKs use HTTP only** (not gRPC). This table maps REST routes to SDK methods. The same operations are also available on gRPC — see [`../docs/grpc-vs-http.md`](../docs/grpc-vs-http.md).

OpenAPI schemas: [`../docs/openapi.yaml`](../docs/openapi.yaml).

## SDK method coverage

| HTTP | Python (`PCMIClient`) | TypeScript (`PCMIClient`) |
|------|----------------------|---------------------------|
| `POST /v1/memories` | `store` | `store` |
| `POST /v1/retrieve` | `retrieve` | `retrieve` |
| `POST /v1/memories/batch` | `batch_store` | `batchStore` |
| `POST /v1/retrieve/batch` | `batch_retrieve` | `batchRetrieve` |
| `POST /v1/memories/compact` | `compact` | `compact` |
| `POST /v1/memories/refine` | `refine` | `refine` |
| `POST /v1/memories/links` | `create_link` | `createLink` |
| `GET /v1/memories/links` | `list_links` | `listLinks` |
| `GET /v1/stats` | `tenant_stats` | `getStats` |
| `GET /v1/events` (SSE) | `subscribe` | `subscribe` |
| `GET /v1/events/schemas` | `list_event_schemas` | `listEventSchemas` |
| `POST /v1/events` | `ingest_event` | `ingestEvent` |
| `POST /v1/webhooks` | `register_webhook` | `registerWebhook` |
| `GET /v1/webhooks` | `list_webhooks` | `listWebhooks` |
| `GET /v1/webhooks/dead-letter` | `list_webhook_dead_letter` | `listWebhookDeadLetter` |
| `POST /v1/embeddings/migrate` | `migrate_embeddings` | `migrateEmbeddings` |
| `POST /v1/memories/rollback` | `rollback` | `rollback` |
| `POST /v1/memories/summarize` | `summarize` | `summarize` |
| `GET /v1/memories/history` | `get_history` | `getHistory` |
| `GET /v1/memories/{path}` | `get_memory` | `getMemory` |
| `GET /v1/lineage/memory` | `memory_lineage` | `memoryLineage` |
| `GET /v1/lineage/distilled/{id}` | `distilled_lineage` | `distilledLineage` |
| `GET /v1/distilled` | `list_distilled` | `listDistilled` |
| `GET /v1/audit` | `list_audit` | `listAudit` |
| `POST /v1/memories/export` | `export_memories` | `exportMemories` |
| `POST /v1/memories/import` | `import_memories` | `importMemories` |
| `POST /v1/sessions` | `create_session` | — |
| `DELETE /v1/sessions/{id}` | `end_session` | — |
| `POST /v1/sessions/{id}/memories` | `store_session_memory` | — |
| `GET /v1/sessions/{id}/memories` | `list_session_memories` | — |
| `POST /v1/sessions/{id}/promote` | `promote_session` | — |
| `GET /v1/admin/tenants` | `list_tenants` | `listTenants` |
| `POST /v1/admin/tenants` | `create_tenant` | `createTenant` |
| `GET /v1/admin/api-keys` | `list_api_keys` | `listApiKeys` |
| `POST /v1/admin/api-keys` | `create_api_key` | `createApiKey` |
| `POST /v1/admin/api-keys/:id/rotate` | `rotate_api_key` | `rotateApiKey` |

### Experimental — Cognitive Graph (no SDK wrapper yet)

Requires Apache AGE (`docker compose --profile graph`). See [`../docs/cognitive-graph.md`](../docs/cognitive-graph.md).

| HTTP | Notes |
|------|--------|
| `GET /v1/graph/health` | No auth; `{"available": bool, "extension": "apache-age"}` |
| `GET /v1/graph/related` | Read role; query `memory_id`, `depth`, `link_types`, `direction` (`both`|`out`|`in`); 501 when AGE absent |
| `GET /v1/graph/entities/memory` | Read role; list promoted entities for `memory_id`; 501 when AGE absent |
| `GET /v1/graph/entities/related` | Read role; correlate by `kind`+`key` or shared entities via `memory_id`; 501 when AGE absent |
| `GET /v1/graph/link-proposals` | List LLM link proposals (`status`, `source_memory_id`) |
| `POST /v1/graph/link-proposals/generate/{memory_id}` | Generate proposals (503 when `LINK_PROPOSALS_ENABLED=false`) |
| `POST /v1/graph/link-proposals/{id}/accept` | Materialize pending proposal to `memory_links` |
| `POST /v1/graph/link-proposals/{id}/reject` | Reject pending proposal |
| `GET /v1/entities/registry` | List canonical entities (`?kind=`, `?limit=`) — any extraction profile |
| `GET /v1/entities/registry/{kind}/{canonical_key}` | Entity detail + aliases + evolution snapshots |
| `POST /v1/entities/registry/aliases` | Register manual alias merge (write role) |
| `GET /v1/graph/entity-alias-proposals` | List pending entity alias merge proposals |
| `POST /v1/graph/entity-alias-proposals/generate/{memory_id}` | LLM alias proposals (503 when `ENTITY_ALIAS_PROPOSALS_ENABLED=false`) |
| `POST /v1/graph/entity-alias-proposals/{id}/accept` | Accept alias → registry + AGE `same_as` |
| `POST /v1/graph/entity-alias-proposals/{id}/reject` | Reject alias proposal |
| `GET /v1/extraction-profiles` | List tenant LLM extraction profiles (Phase A) |
| `PUT /v1/extraction-profiles/{profile_id}` | Upsert profile (`path_prefix`, `profile`, `enabled`) |
| `DELETE /v1/extraction-profiles/{profile_id}` | Remove profile |
| `GET /v1/memories/extraction/{memory_id}` | Read `metadata.pcmi_extract` for current memory version |
| `POST /v1/memories/extraction/{memory_id}` | Force LLM extraction (503 when `EXTRACTION_ENABLED=false`) |

Admin methods require **admin** role (`testkey123` in default migrations). CI runs read-only `admin_smoke` / `admin-smoke.mts`.

## List pagination (query params)

| Query | Meaning |
|-------|---------|
| `limit` | Page size 1–200 (endpoint default, often 50) |
| `cursor` | Opaque token from prior `next_cursor` |
| `after_id` | Legacy alias when `cursor` is empty (not on admin tenants/keys or webhooks) |

| Response field | Meaning |
|----------------|---------|
| `next_cursor` | Pass as `cursor` for the next page (empty when done) |
| `has_more` | `true` if another page exists |
| `total` | Present on audit (global count), admin tenants (global), history/distilled (rows in this page only) |

Go: `ListTenants` / `ListAPIKeys` set `cursor` in the query string. Python `list_audit` still sends `offset` for compatibility; the server ignores offset and returns `offset: 0`.

## Store / retrieve options (HTTP)

Both SDKs support parity fields on `store` / `retrieve`:

- **store**: `tags`, `embedding`, `embedding_model`, `embedding_space`, `source_agent_id`, `encrypt_content`, `expires_at` (RFC3339)
- **retrieve**: `tags`, `tags_match` (`any` \| `all`), `as_of`, `source_agent_id`, `embedding_space`

## Examples

### Python

```python
async with PCMIClient(base_url, api_key) as client:
    await client.store("root.note", "hello", tags=["demo"], embedding_model="unspecified")
    await client.compact("root.note", keep_superseded=10)
    async for ev in client.subscribe(types=["memory.stored"]):
        print(ev["type"])
```

### TypeScript

```bash
cd sdk/typescript && npm install && npm install -D tsx
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
npm run smoke
```

Use `smoke.mts` (not `tsx` heredocs) — stdin eval breaks named imports on Node 23+.

For gRPC agents, use `proto/pcmi/v1/memory.proto` and generated stubs — SDKs remain HTTP-first.
