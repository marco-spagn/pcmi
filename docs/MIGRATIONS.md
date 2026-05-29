# PostgreSQL database migrations

Migrations are SQL files in `migrations/`. Locally with Docker Compose they are mounted in `/docker-entrypoint-initdb.d/` and executed **once only** at the first creation of the Postgres volume.

## Application order

Must match the order in `docker-compose.yml` (postgres volumes list) and CI scripts that run `for f in migrations/*.sql`.

| File | Main content |
|------|-------------|
| `001_init.sql` | Extensions (`ltree`, `vector`, `pg_trgm`), base tables `tenants`, `memory_entries`, `distilled_knowledge`, `events`, indexes.
| `002_multi_tenant.sql` | RLS, `set_tenant_context` function, tenant policies on relevant tables.
| `003_rbac_api_keys.sql` | `api_keys` table, roles, default dev key.
| `004_audit_encryption.sql` | `audit_log`, encryption support where schema requires it.
| `005_worker_helpers.sql` | Worker support functions/triggers (pruning, etc.).
| `006_fts_hybrid.sql` | `content_tsv`, hybrid FTS + vector functions.
| `007_v1_11_embedding_space.sql` | `embedding_space` columns and related indexes.
| `008_v1_12_distilled_webhooks_encrypt.sql` | Webhook registry, `distilled_knowledge` improvements, content encryption.
| `009_v1_13_event_schemas_webhook_dlq.sql` | Event schema registry, webhook DLQ.
| `010_v1_14_consolidation_bm25_admin.sql` | Consolidation runs, BM25 helper, admin extensions.
| `011_v1_15_links_expiry_tags.sql` | `memory_links`, `expires_at`, GIN index on `tags`, `expire_memory_entries()`. |
| `012_v1_19_memory_compaction.sql` | `compact_memory_path_history` — removes excess closed versions for a single path. |
| `013_idempotency.sql` | `idempotency_cache` table for `X-Idempotency-Key` on `POST /v1/memories` (24h TTL). |
| `014_key_lifecycle.sql` | API key rotation, grace period, `last_used_ip`. |
| `015_importance_decay.sql` | `importance`, `access_count`, `last_accessed_at` on `memory_entries`; `tenant_memory_config`. |
| `016_sessions.sql` | `agent_sessions`; index on `memory_entries(metadata->>'session_id')` for working memory. |
| `017_dedup.sql` | `content_hash` on `memory_entries`; partial index for current versions (ingest dedup). |
| `018_distillation_policy.sql` | `distillation_policies`, `distillation_runs` — policy engine for automatic distillation. |

## Adding a new migration

1. Create `NNN_description.sql` with `NNN` greater than the last existing one.
2. Update `docker-compose.yml` (volume `.../migrations/NNN...:/docker-entrypoint-initdb.d/NNN...`).
3. If the Postgres volume already exists, apply the same file manually (Compose does not re-run init on a full volume).

## Automatic runner (`cmd/migrate`)

For clusters or already-initialized volumes, use the **`pcmi-migrate`** binary (idempotent, `schema_migrations` table):

```bash
export DATABASE_URL=postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable
go run ./cmd/migrate
# or, from the legacy image:
./pcmi-migrate
```

In Helm, enable `migrations.enabled=true` to run migrations as an API init-container before startup. Requires the image with `/pcmi-migrate` and the `migrations/` directory included (legacy Dockerfile or dedicated build).

## Extension dependencies

Required before application code: `ltree`, `vector`, `pg_trgm`. The `pgvector/pgvector` image provides them.
