# HTTP-only PCMI APIs (SDK guide)

Core memory **store/retrieve** is available on **gRPC** and **HTTP**. Everything in this document is **HTTP-only** — use the official SDKs or `docs/openapi.yaml`.

Transport overview: [`../docs/grpc-vs-http.md`](../docs/grpc-vs-http.md).

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
| `GET /v1/admin/tenants` | `list_tenants` | `listTenants` |

Admin key creation (`POST /v1/admin/tenants`, API keys) requires **admin** role — call via HTTP/OpenAPI; not wrapped in thin SDKs yet.

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

```typescript
const client = new PCMIClient(baseUrl, apiKey);
await client.store("root.note", "hello", {}, { tags: ["demo"], embeddingModel: "unspecified" });
await client.compact("root.note", { keepSuperseded: 10 });
client.subscribe((ev) => console.log(ev.type), { types: ["memory.stored"] });
```

For gRPC agents, use `proto/pcmi/v1/memory.proto` and generated stubs — SDKs remain HTTP-first.
