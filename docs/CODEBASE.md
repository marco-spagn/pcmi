# PCMI Codebase Guide

Orientation document for readers and contributors: what each area does, deliberate dependencies, and conventions.

## Entry points

| Path | Role |
|------|------|
| `cmd/api/main.go` | Pool `database.NewPools(DATABASE_URL, DATABASE_READ_URL)`, readiness on primary, middleware on primary, `handler.SetupMemoryRoutes(app, primary, readReplica)`; gRPC and admin on primary. |
| `cmd/worker/main.go` | Worker: embedding (circuit breaker), distillation, pruning, consolidation, expiry, consume Redis Streams/pubsub. |
| `cmd/mcp/main.go` | MCP stdio server → HTTP API (`PCMI_BASE_URL`, `PCMI_API_KEY`). |
| `cmd/pcmi-admin/main.go` | CLI ops: `list` tenants/API keys (`make admin-list-keys`). |

**API middleware order** (Fiber: the first `Use` registered is outermost): `otelfiber` (tracing, skips `/metrics`, `/health`, `/v1/health`, `/ready`, `/v1/ready`) → `metrics` (no-op) → `APIKeyMiddleware` → `RateLimitMiddleware` → `AuditMiddleware`. Unauthenticated probes are defined in `middleware.IsUnauthenticatedProbe`: `/health`, `/v1/health`, `/metrics`, `/ready`, `/v1/ready`.

**Route registration**: in `memory_handler.go` specific routes (`/memories/history`, batch, lineage under `/lineage/*`) must go **before** the wildcard `GET /memories/*` to prevent Fiber from capturing segments as path names.

## `internal/handler`

Fiber HTTP handlers; read tenant from `middleware.TenantContextKey`, call repository or service.

- `memory_handler.go` — main memory routing, retrieve, history, refine, links, stats, get-by-path wildcard, **`POST /memories/compact`**.
- `batch_handler.go`, `admin_handler.go` — batch API and admin tenant/keys.
- `events_handler.go` — SSE stream, event ingest, schema list.
- `lineage_handler.go` — `/v1/lineage/memory`, `/v1/lineage/distilled/:id` (outside `/memories/*`).
- `refine_handler.go` — `POST /v1/memories/refine` publishes to Redis for the worker.
- `links_handler.go`, `stats_handler.go` — link graph and tenant statistics.
- `webhook_handler.go`, `summarize_handler.go`, `embedding_migrate_handler.go`, `distilled_handler.go`, `audit_handler.go`, `history_handler.go` — as named.
- `ready.go` — `GET /ready`, `/v1/ready`: ping DB + Redis for readiness (503 if a dependency is down).

## `internal/service`

Reusable application logic from both REST and gRPC:

- `memory_service.go` — store/retrieve, Redis event publication after store.
- `batch_service.go`, `admin_service.go`, `event_service.go`, `summarize_service.go`.

Do not import UI frameworks here: only models, repositories, embedding.

## `internal/repository`

PostgreSQL data access (`pgxpool`). Pattern: set tenant via `set_tenant_context` from middleware before RLS queries. `NewMemoryRepository(writePool, readPool)` routes SELECTs to `readPool` when `DATABASE_READ_URL` is set (replica); transactions and `GetHistoricalVersion` use the primary.

- `memory_repository.go` — append-only store, retrieve with temporal filters, agent, embedding space, **tags** (`tagFilters` in `retrieve_sql.go`); write vs read pool as above.
- `retrieve_sql.go` — shared SQL clauses (`temporalClause`, `scopeFilters`, `tagFilters`).
- `history.go`, `rollback.go`, `get_by_path.go` — versions, rollback, single path get.
- `export_import.go` — tenant-scoped export/import.
- `lineage.go` — join memory versions + `distilled_knowledge` for lineage.
- `links.go` — `memory_links` table.
- `stats.go` — aggregates for `/v1/stats`.
- `admin_repository.go`, `audit_repository.go`, `event_repository.go` — admin, audit, event ingest.

## `internal/worker`

Async processes; share DB and Redis with the API.

- `embedding.go` — embedding generation after events (if OpenAI is configured).
- `distillation.go` — job on path prefix; publishes `knowledge.distilled`; dedup on sources (`distillation_helpers.go`, `distillation_version.go`).
- `consolidation.go` — merge related memories under a prefix.
- `pruning.go` — calls SQL function `prune_superseded_memories`.
- `expiry.go` — periodically calls `expire_memory_entries()` for TTL.

**Redis events** consumed in `cmd/worker/main.go`: `memory.stored`, `memory.updated`, `memory.refine.requested` — every handler is wrapped in an OpenTelemetry span (`redis.memory_event`) if OTLP is configured.

## `internal/event`

- `backend.go` — `EVENT_BACKEND`: `streams` (XADD `pcmi:events`) vs `pubsub` (`memory_events`).
- `stream.go` / `redis.go` — publish/consume, consumer group `pcmi-workers`, DLQ.
- `schema.go` — event type constants.

## `internal/ratelimit`

Distributed Redis limiter when `RATE_LIMIT_BACKEND=redis` (key per tenant+API key).

## `internal/embedding`

`CircuitBreakerProvider` (gobreaker) wraps OpenAI and other providers; metrics `pcmi_embedding_*`.

## `internal/middleware`

- `apikey.go` — resolves key → tenant, role; `set_tenant_context`.
- `public.go` — `IsUnauthenticatedProbe`: single list of GET paths exempt from key, rate limit, and audit.
- `ratelimit.go` — limiter per API key (`RATE_LIMIT_BACKEND=memory|redis`); probes exempt.
- `idempotency.go` — cache `POST /v1/memories` for `X-Idempotency-Key` (24h).
- `metrics_auth.go` — Bearer on `GET /metrics` if `METRICS_SCRAPE_TOKEN` is set.
- `audit.go` — writes `audit_log`.
- `admin.go`, `rbac.go`, `tenant.go` — admin roles and write constraints.

## `internal/version`

Constants `Tag` (`vX.Y.Z`) and `Semver` (`X.Y.Z` for OpenAPI) — single source of truth for health API, gRPC, worker, and CI smoke updates.

## `internal/metrics`

Dedicated Prometheus registry (`metrics.Registry`) for the **API**, counters `pcmi_memory_stores_total` / `pcmi_memory_retrieves_total`. Separate registry **`WorkerRegistry`** for **pcmi-worker** (`cmd/worker`), with `pcmi_worker_redis_events_total{event_type=…}` and Go/process collectors. HTTP RED middleware was removed to avoid duplicate gather errors under high-traffic scrapes. **OpenTelemetry** HTTP server metrics (histograms from `otelfiber`) are separate and require an OTLP collector to aggregate on the backend side.

## `internal/telemetry`

Optional OTLP/HTTP tracer initialization: `telemetry.Init(ctx, defaultServiceName)` from **`cmd/api`** (`pcmi-api`) and **`cmd/worker`** (`pcmi-worker`); `OTEL_SERVICE_NAME` takes priority over the default. Global W3C propagators; without an OTLP endpoint, tracer is **noop**. Variables: `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`.

## `internal/grpc`

gRPC server: `MemoryService`, `AdminService`, `MetricsService` (see `internal/grpc/server.go`). Auth via metadata `x-api-key` or proto request fields. Core + operational RPCs (refine, links, stats, events stream, webhooks, …) — see `docs/grpc-vs-http.md`. **HTTP-only:** embedded admin UI (`GET /v1/admin/ui`). Prometheus scrape: `GET /metrics` (HTTP) or `MetricsService.Scrape` (gRPC). Writes reject `readonly` role (`PermissionDenied`). Integration tests: `go test -tags=integration ./internal/grpc/...` with API+gRPC running.

## `internal/webhook`

HTTP delivery to registered URLs, retry, dead-letter.

## `internal/eventschema`

Payload validation registry for universal event ingest.

## `internal/crypto`

Field-level encryption for `content` at-rest when requested (`encrypt_content` / `metadata.sensitive`).

## `internal/embedding`

Embedding provider (OpenAI); interface used by service/worker.

## `internal/model`

JSON structs for API and persistence; no business logic.

## `internal/database`

- `db.go` / `pools.go` — `New(url)` for a single pool; `NewPools(primaryURL, readReplicaURL)` for primary + optional read replica (`ReadOrPrimary`, `Close`).

## `migrations`

**Lexicographic** order in `docker-compose` and CI: `001`, `002`, … Do not renumber files already applied in production; always add successive `0NN_...sql` files. Detail in `docs/MIGRATIONS.md`.

## `sdk/`

Thin HTTP clients; see `sdk/README.md`, HTTP-only API mapping in `sdk/HTTP-API.md`, transports in `docs/grpc-vs-http.md`.

## `scripts/`

Smoke/E2E for CI: `scripts/ci_integration_smoke.sh` (job **integration-smoke**), `scripts/e2e/test_pcmi.sh` + `ci_e2e_sse_dedup.sh` / `ci_e2e_finale.sh` when OpenAI is available. Feature smokes: `scripts/smoke_importance_retrieve.sh`, `scripts/smoke_sessions.sh`, `scripts/smoke_dedup.sh`. Full host validation: `scripts/run_full_validation.sh` (`make test-full-real`). Port cleanup: `scripts/free_dev_ports.sh`. Distillation: `scripts/pcmi_synth/`, `scripts/distill_e2e.sh`, `scripts/run_pcmi_distillation_test.sh`. Legacy: `scripts/e2e/legacy/`.

## `examples/`

Celery, Temporal, and AI framework examples (LangChain, LlamaIndex, AutoGen, CrewAI) that call the HTTP API; see `examples/README.md`.

## Tests

- `make test` — unit tests (`go test ./...`)
- `make lint` — golangci-lint v2 (install with `make install-lint`; config requires `version: "2"` in `.golangci.yml`)
- `make test-integration` — gRPC integration tests (`go test -tags=integration ./internal/grpc/...`; API+gRPC+Postgres running)
- `make test-streams-integration`, `make test-circuit-breaker`, `make test-idempotency`, `make test-ratelimit-integration`, `make test-key-lifecycle`, `make test-retrieval-scoring`, `make test-sessions-integration`, `make test-dedup` — Sprint 1–2 focused packages
- `make test-full-real` — CI parity + optional OpenAI + feature smokes + MCP
- `make sdk-smoke` — Python + TypeScript HTTP SDK smoke (`scripts/ci_sdk_smoke.sh`; API on :8000)
- `make sdk-go-test` / `make sdk-go-smoke` — Go HTTP SDK (`sdk/go/`; smoke needs API on :8000)
- `make sdk-all` — Python/TS smoke + Go unit tests
- CI: `integration-smoke` (local API+worker + Postgres/Redis services), `integration-e2e` (compose + OpenAI if secret)

## Useful conventions for the future

1. **New memory routes**: add under `/v1` in `SetupMemoryRoutes` **before** the wildcard `/memories/*` if the path risks conflict.
2. **New migrations**: include the file in `docker-compose.yml` under `postgres.volumes` and in every path that applies migrations manually.
3. **Worker events**: extend `internal/event/schema.go` and subscriber in `cmd/worker/main.go`.
4. **API version**: constants in `internal/version` (`Tag` / `Semver`); CI smoke `PCMI_EXPECT_VERSION`. After changes to `proto/pcmi/v1/memory.proto`: `protoc --proto_path=proto --go_out=. --go_opt=module=github.com/marco-spagn/pcmi --go-grpc_out=. --go-grpc_opt=module=github.com/marco-spagn/pcmi pcmi/v1/memory.proto`.

## References

- Documentation index: `docs/INDEX.md`
- Usage guide: `docs/USAGE.md`
- HTTP API: `docs/openapi.yaml`
- gRPC vs HTTP: `docs/grpc-vs-http.md`
- Architecture: `docs/architecture.md`
- Migrations: `docs/MIGRATIONS.md`
- Failure / scale: `docs/failure-modes.md`, `docs/scalability.md`
- Retrieval: `docs/retrieval-pipeline.md`
