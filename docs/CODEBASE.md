# Guida al codice PCMI

Documento di orientamento per chi legge o modifica il repository: cosa fa ogni area, dipendenze consapevoli e convenzioni.

## Entry point

| Percorso | Ruolo |
|----------|--------|
| `cmd/api/main.go` | Pool `database.NewPools(DATABASE_URL, DATABASE_READ_URL)`, readiness sul primario, middleware su primario, `handler.SetupMemoryRoutes(app, primary, readReplica)`; gRPC e admin sul primario. |
| `cmd/worker/main.go` | Worker: embedding (se c’è `OPENAI_API_KEY`), distillation, pruning, consolidation, expiry, subscribe Redis. |

**Ordine middleware API** (Fiber: il primo `Use` registrato è il più esterno): `otelfiber` (tracing, salta `/metrics`, `/health`, `/v1/health`, `/ready`, `/v1/ready`) → `metrics` (no-op) → `APIKeyMiddleware` → `RateLimitMiddleware` → `AuditMiddleware`. Le probe senza chiave sono definite in `middleware.IsUnauthenticatedProbe`: `/health`, `/v1/health`, `/metrics`, `/ready`, `/v1/ready`.

**Registrazione route**: in `memory_handler.go` le route specifiche (`/memories/history`, batch, lineage sotto `/lineage/*`) vanno **prima** del wildcard `GET /memories/*` per evitare che Fiber catturi segmenti come nomi di path.

## `internal/handler`

HTTP handlers Fiber; leggono tenant da `middleware.TenantContextKey`, chiamano repository o service.

- `memory_handler.go` — routing principale memorie, retrieve, history, refine, links, stats, get-by-path wildcard, **`POST /memories/compact`**.
- `batch_handler.go`, `admin_handler.go` — batch API e admin tenant/chiavi.
- `events_handler.go` — SSE stream, ingest eventi, lista schemi.
- `lineage_handler.go` — `/v1/lineage/memory`, `/v1/lineage/distilled/:id` (fuori da `/memories/*`).
- `refine_handler.go` — `POST /v1/memories/refine` pubblica su Redis per il worker.
- `links_handler.go`, `stats_handler.go` — grafo link e statistiche tenant.
- `webhook_handler.go`, `summarize_handler.go`, `embedding_migrate_handler.go`, `distilled_handler.go`, `audit_handler.go`, `history_handler.go` — come da nome.
- `ready.go` — `GET /ready`, `/v1/ready`: ping DB + Redis per readiness (503 se una dipendenza è giù).

## `internal/service`

Logica applicativa riutilizzabile da REST e gRPC:

- `memory_service.go` — store/retrieve, pubblicazione eventi Redis dopo store.
- `batch_service.go`, `admin_service.go`, `event_service.go`, `summarize_service.go`.

Non importare framework UI qui: solo modelli, repository, embedding.

## `internal/repository`

Accesso dati PostgreSQL (`pgxpool`). Pattern: impostare tenant via `set_tenant_context` dal middleware prima delle query RLS. `NewMemoryRepository(writePool, readPool)` instrada le SELECT su `readPool` quando `DATABASE_READ_URL` è impostato (replica); transazioni e `GetHistoricalVersion` usano il primario.

- `memory_repository.go` — store append-only, retrieve con filtri temporali, agent, embedding space, **tag** (`tagFilters` in `retrieve_sql.go`); write vs read pool come sopra.
- `retrieve_sql.go` — clausole SQL condivise (`temporalClause`, `scopeFilters`, `tagFilters`).
- `history.go`, `rollback.go`, `get_by_path.go` — versioni, rollback, get singolo path.
- `export_import.go` — export/import tenant-scoped.
- `lineage.go` — join versioni memoria + `distilled_knowledge` per lineage.
- `links.go` — tabella `memory_links`.
- `stats.go` — aggregati per `/v1/stats`.
- `admin_repository.go`, `audit_repository.go`, `event_repository.go` — admin, audit, eventi ingest.

## `internal/worker`

Processi asincroni; condividono DB e Redis con l’API.

- `embedding.go` — generazione embedding dopo eventi (se OpenAI configurato).
- `distillation.go` — job su prefisso path; pubblica `knowledge.distilled`; dedup su sorgenti (`distillation_helpers.go`, `distillation_version.go`).
- `consolidation.go` — merge ricordi correlati sotto prefisso.
- `pruning.go` — chiamata a funzione SQL `prune_superseded_memories`.
- `expiry.go` — chiama periodicamente `expire_memory_entries()` per TTL.

**Eventi Redis** consumati in `cmd/worker/main.go`: `memory.stored`, `memory.updated`, `memory.refine.requested` — ogni gestione è wrappata in uno span OpenTelemetry (`redis.memory_event`) se OTLP è configurato.

## `internal/event`

- `redis.go` — pub/sub canale `memory_events`; opzionale notifica webhook.
- `schema.go` — costanti tipi evento.
- `bus.go` — bus in-process dove usato.

## `internal/middleware`

- `apikey.go` — risolve chiave → tenant, ruolo; `set_tenant_context`.
- `public.go` — `IsUnauthenticatedProbe`: elenco unico path GET esenti da chiave, rate limit e audit.
- `ratelimit.go` — limiter per API key; probe in `IsUnauthenticatedProbe` esenti dal limit.
- `audit.go` — scrittura `audit_log`.
- `admin.go`, `rbac.go`, `tenant.go` — ruoli admin e vincoli scrittura.

## `internal/version`

Costanti `Tag` (`vX.Y.Z`) e `Semver` (`X.Y.Z` per OpenAPI) — unico punto per health API, gRPC, worker e aggiornamento smoke CI.

## `internal/metrics`

Registry Prometheus dedicato (`metrics.Registry`) per **API**, contatori `pcmi_memory_stores_total` / `pcmi_memory_retrieves_total`. Registry separato **`WorkerRegistry`** per **pcmi-worker** (`cmd/worker`), con `pcmi_worker_redis_events_total{event_type=…}` e collector Go/process. Il middleware HTTP RED è stato rimosso per evitare errori di gather duplicati in scrape ad alto traffico. Le metriche HTTP server di **OpenTelemetry** (histogram da `otelfiber`) sono separate e richiedono un collector OTLP se si vogliono aggregare lato backend.

## `internal/telemetry`

Inizializzazione tracer OTLP/HTTP opzionale: `telemetry.Init(ctx, defaultServiceName)` da **`cmd/api`** (`pcmi-api`) e **`cmd/worker`** (`pcmi-worker`); `OTEL_SERVICE_NAME` ha priorità sul default. Propagatori W3C globali; senza endpoint OTLP, tracer **noop**. Variabili: `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`.

## `internal/grpc`

Server gRPC che riusa `MemoryService`; autenticazione via metadata `x-api-key` o campo richiesta proto. RPC core + operational (refine, links, stats, events stream, webhooks, …) — vedi `docs/grpc-vs-http.md`. **Admin** e **/metrics** restano HTTP-only. Scritture rifiutano ruolo `readonly` (`PermissionDenied`). Test integrazione: `go test -tags=integration ./internal/grpc/...` con API+gRPC in esecuzione.

## `internal/webhook`

Delivery HTTP verso URL registrati, retry, dead-letter.

## `internal/eventschema`

Registry validazione payload per ingest eventi universali.

## `internal/crypto`

Cifratura campo `content` at-rest quando richiesto (`encrypt_content` / `metadata.sensitive`).

## `internal/embedding`

Provider embedding (OpenAI); interfaccia usata da service/worker.

## `internal/model`

Struct JSON per API e persistenza; nessuna logica.

## `internal/database`

- `db.go` / `pools.go` — `New(url)` per un singolo pool; `NewPools(primaryURL, readReplicaURL)` per primario + replica di lettura opzionale (`ReadOrPrimary`, `Close`).

## `migrations`

Ordine **lessicografico** in `docker-compose` e in CI: `001`, `002`, … Non rinumerare file già applicati in produzione; aggiungere sempre `0NN_...sql` successivi. Dettaglio in `docs/MIGRATIONS.md`.

## `sdk/`

Client HTTP thin; vedere `sdk/README.md`, mapping API solo-HTTP in `sdk/HTTP-API.md`, trasporti in `docs/grpc-vs-http.md`.

## `scripts/`

Smoke/E2E per CI: un unico `scripts/ci_integration_smoke.sh` (job **integration-smoke**), più `test_pcmi.sh` / `ci_e2e_sse_dedup.sh` / `ci_e2e_finale.sh` quando c’è OpenAI (E2E compose). Locale: `scripts/local_smoke_orchestration.sh`.

## `examples/`

Celery e Temporal minimi che chiamano l’API HTTP; vedi `examples/README.md`.

## Test

- `make test` — unit tests (`go test ./...`)
- `make lint` — golangci-lint v2 (install with `make install-lint`; config requires `version: "2"` in `.golangci.yml`)
- `make test-integration` — gRPC integration tests (`go test -tags=integration ./internal/grpc/...`; API+gRPC+Postgres running)
- `make sdk-smoke` — Python + TypeScript HTTP SDK smoke (`scripts/ci_sdk_smoke.sh`; API on :8000)
- CI: `integration-smoke` (API+worker locali + Postgres/Redis servizi), `integration-e2e` (compose + OpenAI se secret)

## Convezioni utili in futuro

1. **Nuove route memoria**: aggiungere sotto `/v1` in `SetupMemoryRoutes` **prima** del wildcard `/memories/*` se il path rischia conflitto.
2. **Nuove migration**: includere il file in `docker-compose.yml` sotto `postgres.volumes` e in ogni path che applica migrazioni manualmente.
3. **Eventi worker**: estendere `internal/event/schema.go` e sottoscrittore in `cmd/worker/main.go`.
4. **Versione API**: costanti in `internal/version` (`Tag` / `Semver`); smoke CI `PCMI_EXPECT_VERSION`. Dopo modifiche a `proto/pcmi/v1/memory.proto`: `protoc --proto_path=proto --go_out=. --go_opt=module=github.com/marco-spagn/pcmi --go-grpc_out=. --go-grpc_opt=module=github.com/marco-spagn/pcmi pcmi/v1/memory.proto`.

## Riferimenti

- API HTTP: `docs/openapi.yaml`
- gRPC vs HTTP: `docs/grpc-vs-http.md`
- Architettura: `docs/architecture.md`
- Migrations: `docs/MIGRATIONS.md`
- Failure / scale: `docs/failure-modes.md`, `docs/scalability.md`
- Retrieval: `docs/retrieval-pipeline.md`
