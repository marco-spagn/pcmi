# PCMI evolution roadmap

## v1.50.0 (current)

- **AI framework examples** (PCMI-016): LangChain, LlamaIndex, AutoGen AgentChat, CrewAI — minimal HTTP integration under `examples/`.
- API version `v1.50.0`.

## v1.48.0

- **API versioning policy**: [API-VERSIONING.md](API-VERSIONING.md), `cliff.toml`, Release workflow on `v*` tags.
- API version `v1.48.0`.

## v1.47.0

- **Cursor pagination** (PCMI-014): keyset `cursor` / `next_cursor` / `has_more` on list endpoints.
- API version `v1.47.0`.

## v1.33.0

- **gRPC Admin + Metrics**: `AdminService` (tenant/API keys) and `MetricsService` (`Scrape`, `StreamScrape`) on the gRPC plane; HTTP `GET /metrics` and admin routes remain for Prometheus and browsers.
- **Admin UI**: embedded dashboard at `GET /v1/admin/ui`.
- **Cursor pagination**: keyset cursors on path-only retrieve (`cursor`, `next_cursor`, `has_more`).
- **gRPC TLS**: shared cert/key config with HTTP when `PCMI_TLS_CERT` / `PCMI_TLS_KEY` are set.
- **CodeQL** workflow and deploy structural tests expanded.
- API version `v1.33.0`.

## v1.31.0

- **Distillation E2E**: `scripts/run_pcmi_distillation_test.sh` + `generate_soc_incidents_enterprise_v2.py`; report fix (`/v1/distilled` → `entries`); batch distill **LIMIT 100**; `.gitignore` for `.venv_e2e/` and `.pcmi_test_out/`.
- API version `v1.31.0`.

## v1.30.0

- **Documentation**: README with usage guides, Mermaid diagrams, [docs/INDEX.md](INDEX.md), [USAGE.md](USAGE.md), [DATA-MODEL.md](DATA-MODEL.md), [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md); `architecture.md` updated.
- API version `v1.30.0` (documentation only; behavior unchanged).

## v1.29.0

- **gRPC operational parity**: refine, links, stats, events (ingest + schemas + **StreamEvents**), webhooks, migrate embeddings, rollback, summarize, history, lineage, distilled, audit, export/import. Admin and `/metrics` remain HTTP-only.
- API version `v1.29.0`.

## v1.28.0

- **gRPC operational**: `GetMemory` (GET `/v1/memories/*`) and `Compact` (POST `/v1/memories/compact`).
- **SDK admin**: `create_tenant`, `list_api_keys`, `create_api_key`, `rotate_api_key` (Python + TypeScript).
- **CI/DX**: `make sdk-smoke`, `scripts/ci_sdk_smoke.sh` in integration-smoke; read-only admin smoke.
- API version `v1.28.0`.

## v1.27.0

- **HTTP-only SDK**: `sdk/HTTP-API.md` — endpoint-to-method mapping table for Python/TypeScript; clients aligned (full store/retrieve, `compact`, webhooks, `migrate_embeddings`, `list_links`, …).
- **SDK polish**: TypeScript `StoreOptions` / `RetrieveOptions` (`embedding`, `tags`, `encrypt_content`, `expires_at`, `tags_match`); Python `async with`, same missing fields and methods.
- API version `v1.27.0`.

## v1.26.0

- **gRPC retrieve response parity ↔ REST**: `RetrieveEntry` exposes `tenant_id`, `metadata_json`, `tags`, `embedding_model` / `embedding_space`, RFC3339 timestamps (`valid_from`, `valid_to`, `created_at`), `source_agent_id` / `source_event_id`, `content_encrypted`, `embedding` (if present). Applies to `Retrieve`, `BatchRetrieve`, `RetrieveStream`.
- API version `v1.26.0`.

## v1.25.0

- **Vector embedding on gRPC store**: `StoreRequest` / `BatchStoreItem` with `repeated float embedding` (parity with REST `embedding`); worker backfill if absent.
- **HTTP-only API documentation**: `docs/grpc-vs-http.md` (compact, refine, links, stats, admin, webhooks, SSE, …).
- API version `v1.25.0`.

## v1.24.0

- **gRPC store parity ↔ REST**: `StoreRequest` / `BatchStoreItem` with `tags`, `embedding_model`, `embedding_space`, `source_agent_id`, `encrypt_content`, `expires_at_rfc3339`; `StoreResponse` with optional `superseded_id`. gRPC integration tests (`go test -tags=integration`) and store+retrieve smoke with tags.
- API version `v1.24.0`.

## v1.23.0

- **Worker observability**: `cmd/worker` initializes `telemetry.Init` (optional OTLP/HTTP, default `service.name=pcmi-worker`), graceful shutdown; **consumer** span for every Redis `memory_events` message; **`GET :8081/metrics`** Prometheus (`WorkerRegistry`, `pcmi_worker_redis_events_total`). `telemetry.Init` accepts a `defaultServiceName` (`pcmi-api` vs `pcmi-worker`).
- API version `v1.23.0`.

## v1.22.0

- **gRPC retrieve parity ↔ REST**: `RetrieveRequest` and `BatchRetrieveQuery` expose the same filters as the REST JSON body: `tags`, `tags_match`, `as_of_rfc3339` (RFC3339/RFC3339Nano), `source_agent_id`, `embedding_space`. Invalid `as_of` values → `InvalidArgument`.
- API version `v1.22.0`.

## v1.21.0

- **gRPC batch store**: `BatchStore` RPC (same limits as `POST /v1/memories/batch`, max 50 items). **readonly** keys receive `PermissionDenied` on `Store` and `BatchStore`, aligned with REST.
- **Centralized version**: `internal/version` (`Tag` / `Semver`) for REST/gRPC health, worker, and CI smoke.
- API version `v1.21.0`.

## v1.20.0

- **gRPC retrieve**: `BatchRetrieve` (up to 20 queries, same limits as `POST /v1/retrieve/batch`) and `RetrieveStream` (header with `total` then one message per entry, same data as `Retrieve`).
- **OpenTelemetry**: W3C propagation (`tracecontext` + `baggage`); OTLP/HTTP export when `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` or `OTEL_EXPORTER_OTLP_ENDPOINT` is set; otherwise noop tracer. **Prometheus** remains on `GET /metrics` (dedicated registry). HTTP instrumented with `otelfiber`; gRPC with `otelgrpc` (`grpc.StatsHandler`).
- API version `v1.20.0`.

## v1.19.0

- **Per-path compaction**: `POST /v1/memories/compact` + SQL `compact_memory_path_history` — removes **closed** versions (`valid_to` set) beyond the last `keep_superseded` for a path; the current row is left untouched. Complements global age-based **pruning** (`prune_superseded_memories`).
- **CI**: unified HTTP smoke in `scripts/ci_integration_smoke.sh`; `go` and `golangci-lint` jobs in parallel; lighter E2E compose (no duplicates `ci_e2e_v1_14` / `v1_15` / `ci_e2e_embedding` already covered by smoke).
- API version `v1.19.0`.

## v1.18.0

- **Orchestrator examples**: `examples/celery` and `examples/temporal` — HTTP task/activity toward PCMI.
- **Optional read replica**: `DATABASE_READ_URL` routes heavy SELECTs (retrieve, stats, lineage, …) to a streaming replica; multi-tenant federation unchanged (same PG cluster). Detail in `docs/federation-read-replicas.md`.
- API version `v1.18.0` aligned on REST/gRPC health, worker, and CI.

## v1.17.0

- **Readiness**: `GET /ready`, `GET /v1/ready` (HTTP) and RPC `pcmi.v1.MemoryService/Ready` — ping PostgreSQL + Redis; 503 / `not_ready` if a dependency does not respond. Kubernetes sample uses `/v1/ready` for `readinessProbe`.
- API version `v1.17.0` aligned on REST/gRPC health, worker, and CI.

## v1.16.0

- Centralized documentation: `docs/CODEBASE.md`, `docs/MIGRATIONS.md`, Godoc (`internal/*/doc.go`), `sdk/README.md`
- Documentation index in README and API version alignment `v1.16.0`

## v1.15.0

- Explicit distillation trigger (`POST /v1/memories/refine`)
- Memory lineage and distilled-to-source traceability
- Cross-memory links graph
- Tenant stats dashboard API
- Tag-based retrieval filters
- TTL (`expires_at`) with append-only expiry

## Long term

- Federated multi-region memory shards
- Policy engine for data residency
- On-device embedding spaces with server-side index merge
