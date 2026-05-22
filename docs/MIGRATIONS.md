# Migrazioni database PostgreSQL

Le migrazioni sono file SQL in `migrations/`. In locale con Docker Compose sono montate in `/docker-entrypoint-initdb.d/` ed eseguite **una sola volta** alla prima creazione del volume Postgres.

## Ordine di applicazione

Deve coincidere con l’ordine in `docker-compose.yml` (lista volumi postgres) e con gli script CI che fanno `for f in migrations/*.sql`.

| File | Contenuto principale |
|------|----------------------|
| `001_init.sql` | Estensioni (`ltree`, `vector`, `pg_trgm`), tabelle base `tenants`, `memory_entries`, `distilled_knowledge`, `events`, indici.
| `002_multi_tenant.sql` | RLS, funzione `set_tenant_context`, policy tenant su tabelle rilevanti.
| `003_rbac_api_keys.sql` | Tabella `api_keys`, ruoli, chiave di sviluppo default.
| `004_audit_encryption.sql` | `audit_log`, supporto cifratura dove previsto dallo schema.
| `005_worker_helpers.sql` | Funzioni/trigger di supporto per worker (pruning, ecc.).
| `006_fts_hybrid.sql` | `content_tsv`, funzioni hybrid FTS + vector.
| `007_v1_11_embedding_space.sql` | Colonne `embedding_space` e indici correlati.
| `008_v1_12_distilled_webhooks_encrypt.sql` | Webhook registry, miglioramenti `distilled_knowledge`, cifratura contenuti.
| `009_v1_13_event_schemas_webhook_dlq.sql` | Registro schemi eventi, DLQ webhook.
| `010_v1_14_consolidation_bm25_admin.sql` | Consolidation runs, BM25 helper, estensioni admin.
| `011_v1_15_links_expiry_tags.sql` | `memory_links`, `expires_at`, indice GIN su `tags`, `expire_memory_entries()`. |
| `012_v1_19_memory_compaction.sql` | `compact_memory_path_history` — rimuove versioni chiuse in eccesso per un singolo path. |
| `013_idempotency.sql` | Tabella `idempotency_cache` per `X-Idempotency-Key` su `POST /v1/memories` (TTL 24h). |

## Aggiungere una nuova migrazione

1. Creare `NNN_descrizione.sql` con `NNN` > ultimo esistente.
2. Aggiornare `docker-compose.yml` (volume `.../migrations/NNN...:/docker-entrypoint-initdb.d/NNN...`).
3. Se il volume Postgres esiste già, applicare manualmente lo stesso file (Compose non riesegue init su volume pieno).

## Dipendenze estensioni

Richieste prima del codice applicativo: `ltree`, `vector`, `pg_trgm`. L’immagine `pgvector/pgvector` le fornisce.
