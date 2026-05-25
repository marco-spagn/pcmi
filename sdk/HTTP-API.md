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
