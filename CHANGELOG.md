# Changelog

All notable changes to PCMI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The "Tag" constant in `internal/version/version.go` is the source of truth for
the public API version exposed by `/v1/version` and the gRPC `Version` RPC.

## [Unreleased]

### Added

- **SSO / OIDC authentication** (`internal/middleware/oidc.go`): clients can authenticate with an `Authorization: Bearer <jwt>` from any OpenID Connect provider (Keycloak, Auth0, Microsoft Entra, Okta, Google, …) instead of an `X-API-Key`. Vendor-neutral — the issuer's discovery document + JWKS verify each token's signature, issuer, audience, and expiry. Claims are mapped onto PCMI's model: a configurable role claim (string or array) resolves to `admin`/`user`/`readonly` with precedence admin > user > readonly, and a tenant claim must reference an existing tenant (no auto-provisioning) before the same `set_tenant_context` RLS activation as the API-key path runs. Additive and opt-in: enabled only when `OIDC_ISSUER` is set; requests without a bearer token still use `X-API-Key`. Config: `OIDC_ISSUER`, `OIDC_AUDIENCE`, `OIDC_ROLE_CLAIM`, `OIDC_TENANT_CLAIM`, `OIDC_ADMIN_ROLE`, `OIDC_WRITE_ROLE`, `OIDC_READONLY_ROLE`. See [SECURITY.md § SSO / OIDC](SECURITY.md).
- **Backup / restore / DR tooling** (`scripts/backup/`): `pcmi_backup.sh` wraps `pg_dump` into a compressed, timestamped custom-format archive; `pcmi_restore.sh` wraps `pg_restore` and **refuses to overwrite a populated database unless `FORCE=1`**. Makefile targets `backup`, `restore`, `backup-restore-test`. A `docs/runbooks/backup-restore.md` runbook covers scheduled backups, restore, backup verification, DR order-of-operations, and PITR guidance. A CI job (`backup-restore`) runs the full **seed → backup → wipe → restore → assert** cycle on every PR so a regression that breaks restore fails the build. Redis holds only transient state and is not backed up.

- **Entity extraction Phase A**: migration `022_extraction_profiles.sql`; `EXTRACTION_ENABLED` worker/API flag; tenant profiles (`GET/PUT/DELETE /v1/extraction-profiles/{id}`); LLM slot extraction into `metadata.pcmi_extract` (`GET/POST /v1/memories/extraction/{memory_id}`). See [cognitive-graph-entities.md](docs/cognitive-graph-entities.md).

- **Entity extraction Phase B**: migration `023_entity_graph.sql`; promoted slots become `:Entity` vertices with `:mentions` edges from `:Memory` when extraction validates and AGE is available; `GET /v1/graph/entities/memory`, `GET /v1/graph/entities/related` (by `kind`+`key` or shared entities via `memory_id`). See [cognitive-graph-entities.md](docs/cognitive-graph-entities.md).

- **LLM link proposals Phase C**: migration `024_graph_link_proposals.sql`; `LINK_PROPOSALS_ENABLED` flag; async/manual LLM proposals after extraction; `GET /v1/graph/link-proposals`, `POST /v1/graph/link-proposals/generate/{memory_id}`, `POST .../{id}/accept|reject` materializes reviewed edges to `memory_links`. See [cognitive-graph-entities.md](docs/cognitive-graph-entities.md).

- **Entity registry Phase D**: migrations `025_entity_registry.sql`, `026_entity_graph_same_as.sql`; canonical entity registry + alias table + evolution snapshots (generic across all extraction profiles); `ENTITY_ALIAS_PROPOSALS_ENABLED`; `GET /v1/entities/registry`, `GET /v1/entities/registry/{kind}/{canonical_key}`, `POST /v1/entities/registry/aliases`, `GET/POST /v1/graph/entity-alias-proposals/*`. Re-extraction reconciles `mentions` and appends snapshots. See [cognitive-graph-entities.md](docs/cognitive-graph-entities.md).

- **Entity extraction race fix**: async worker no longer overwrites a successful sync extraction (`status: ok`) with a later `validation_failed` from a duplicate LLM call.

### Fixed

- **CI: Go toolchain security patch** — bumped the `toolchain` directive in `go.mod` from `go1.25.12` to `go1.25.14`, clearing the standard-library advisories (`crypto/tls`, `html/template`, `net/url`, `net/http`) that `govulncheck` flagged on `main`. Same-minor patch bump; no API or language-version change (`go 1.25.0` unchanged).
- **CI: advisory linters no longer break the pipeline** — the `nilaway` and `gosec` steps in the `golangci-lint` job are now `continue-on-error: true`. Both are advisory (findings already non-blocking via `|| true`), but a transient failure of `go install ...@latest` was failing the whole job.
- **`GET /v1/graph/related` bidirectional traversal**: default `direction=both` follows `memory_links` in either direction so incoming cross-campaign correlations and leaf nodes (e.g. postmortems) appear in graph exploration. Use `direction=out` for legacy outgoing-only behaviour; `direction=in` for incoming-only.

- **Session promote path collision** (`internal/repository/session_repository.go`): `POST /v1/sessions/:id/promote` re-paths working-memory rows in place. When a target long-term path already held a current memory, the `UPDATE` violated the `uq_memory_entries_open_version` unique index (added in the versioning-race fix), aborting the **entire** promotion with an opaque DB error (and, before that index, silently created two "current" rows at the same path). Promote now detects an occupied target — including two session rows that map to the same path — skips those items instead of failing or overwriting, and reports them in a new `skipped` field on the promote response.

### Security

- **Secret indirection — keep raw secrets out of the environment** (`internal/config/secret.go`): sensitive settings were read straight from the process environment (`os.Getenv`), so `PCMI_ENCRYPTION_KEY`, API keys, the DSN password, and the metrics token were visible in `docker inspect`, `/proc/<pid>/environ`, and orchestrator env views — a standard pentest finding. `config.Load()` now resolves each secret through `resolveSecret`, which supports the `<NAME>_FILE` convention (Docker/Kubernetes secrets, Vault Agent) plus inline `file:` and `env:` schemes, falling back to the literal value for backward compatibility. A named-but-unreadable file logs a warning and is treated as unset so `Config.Validate` fails loudly instead of running with a wrong/empty secret. Covers `PCMI_ENCRYPTION_KEY`, `DATABASE_URL`, `DATABASE_READ_URL`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GROK_API_KEY`, `DEEPSEEK_API_KEY`, `METRICS_SCRAPE_TOKEN`, `ADMIN_API_KEY`, `PCMI_API_KEY`. No behavior change when raw env values are used. See [SECURITY.md § Secrets handling](SECURITY.md).

- **Expiry worker poison-row DoS** (`internal/worker/expiry.go`): the expiry sweep cast `(metadata->>'ttl_seconds')::int` for every current row whose free-form `metadata` JSONB contains a `ttl_seconds` key. Because metadata is stored verbatim from client input, one authenticated tenant storing a non-integer or out-of-range value (`"abc"`, `1.5`, `true`, a 12-digit number) made that cast abort the **entire** UPDATE — so memories stopped expiring for **every** tenant until the offending row was removed (a cross-tenant availability bug, accidentally triggerable with a JSON float). The TTL branch now guards the cast with a `CASE` (Postgres does not guarantee `AND` evaluation order) and a `^[0-9]{1,9}$` pattern that admits only plain int4-range integers; malformed/oversized TTLs are ignored rather than fatal. The first-class `expires_at` path is unchanged.

- **RLS enforcement accuracy (docs)**: clarified that PostgreSQL Row-Level Security is **not enforced in the default single-role setup** — the application connects as the table-owning role (`pcmi`) and no table uses `FORCE ROW LEVEL SECURITY`, so Postgres bypasses RLS for the owner. Tenant isolation is enforced by explicit `tenant_id` query scoping; RLS ships as opt-in defense-in-depth. `docs/DATA-MODEL.md#tenant-isolation` now documents the two-layer model and the steps to make RLS actively enforce (dedicated non-owner role, `FORCE`, per-transaction context). README RLS claims updated to point at this. No code/schema change.

- **Cypher passthrough tenant-scope bypass** (`internal/graph/graph.go`): `POST /v1/graph/cypher` auto-injects the tenant filter as a single `WHERE` before the *first* `RETURN`. A query using `UNION` (e.g. `MATCH (m:Memory) RETURN m.id UNION MATCH (m:Memory) RETURN m.id`) kept an unscoped second sub-query that compiled cleanly and returned **every tenant's** nodes — a cross-tenant data-disclosure vector in the experimental Cognitive Graph. `validateCypherQuery` now rejects `UNION`/`WITH`/`UNWIND`/`FOREACH` (token-based, so identifiers that merely contain those substrings still pass), alongside the existing write-keyword blocklist. EXPERIMENTAL feature.

- **Import entry cap** (`internal/service/memory_service.go`): `POST /v1/memories/import` and gRPC `ImportMemories` iterated entries with no limit, while sibling batch endpoints cap at 50/20. In `skip` mode each entry drives a `GetByPath` lookup plus a `Store` transaction, so one authenticated request could fan out into tens of thousands of DB round-trips (asymmetric resource exhaustion). Import now rejects more than 1000 entries per request with HTTP 400 / gRPC `InvalidArgument`, fail-fast before any database work; clients with larger datasets paginate.

- **Webhook SSRF egress filter** (`internal/webhook/ssrf.go`): webhook target URLs were stored and called with no validation, letting an authenticated tenant point a webhook at internal services or the cloud metadata endpoint (`169.254.169.254`) — a server-side request forgery / credential-theft vector. Registration (HTTP `POST /v1/webhooks` and gRPC `RegisterWebhook`) now rejects non-http(s) URLs and literal private/loopback/link-local/ULA/metadata IP addresses, and the delivery client refuses **any** private/internal address **at dial time** — which also covers hostnames that resolve to such addresses, DNS rebinding, and redirect-to-internal. Secure by default; set `WEBHOOK_ALLOW_PRIVATE_TARGETS=true` to allow trusted internal receivers.
 

### Fixed — data integrity

- **UTF-8-safe content truncation** (`internal/worker/consolidation.go`, `internal/service/summarize_service.go`): both clamped text with a byte slice (`content[:16000]`, `combined[:maxLen]`) that can land in the middle of a multi-byte rune. The consolidation path then persists the result, so truncating non-ASCII content on a rune boundary produced an invalid UTF-8 byte sequence that PostgreSQL rejects on `INSERT` — the consolidation run failed permanently for that prefix. Both call sites now use a `truncateUTF8` helper that drops the partial trailing rune, guaranteeing valid UTF-8 (the summarize path likewise no longer emits a garbled trailing character).

- **Concurrent-write versioning race** (`internal/repository/memory_repository.go`, `migrations/020_memory_open_version_unique.sql`): two simultaneous `POST /v1/memories` to the same `(tenant_id, path)` could each leave a row with `valid_to IS NULL` (two "current" versions with regressed/duplicate version numbers), making `GetByPath` and `as_of` reads non-deterministic. Migration 020 heals any existing duplicates and adds a partial unique index `uq_memory_entries_open_version`; `Store` now retries (bounded) when the losing transaction collides, recomputing the correct next version.

### Added — Cognitive Graph spike (experimental)

- **`migrations/019_cognitive_graph_age.sql`**: optional Apache AGE setup (`pcmi_memory_graph`, `sync_memory_link_to_graph`, trigger on `memory_links` insert/update); skipped when AGE is absent.
- **`GET /v1/graph/health`**: AGE availability probe (no auth).
- **`GET /v1/graph/related`**: multi-hop traversal over `memory_links` (501 when AGE absent).
- **`GET /v1/graph/ui`**: browser **Cognitive Graph Explorer** (traversal, chain highlight, inspector, layouts, clusters, timeline).
- **Demo video**: [docs/assets/graph-ui-demo.mp4](docs/assets/graph-ui-demo.mp4); regenerate with `scripts/e2e/record_graph_ui_demo.mjs`.
- **`internal/graph`**, **`docs/cognitive-graph.md`**, optional `docker compose --profile graph` `postgres-age` service; `make graph-ui` one-command SOC dataset + UI.
- **Cognitive Graph test matrix**: deterministic dataset and `make test-cognitive-graph-matrix` E2E script covering all link types, mixed paths, filters, pagination, cycles, self-loops, isolated nodes, Cypher passthrough, and invalid requests.
- **Cognitive Graph realistic dataset**: 1200-node SOC/incident-response JSON graph with campaigns, alerts, evidence, hypotheses, postmortems, false positives, duplicates, all link types, and generator/validator targets.
- **Cognitive Graph integration coverage**: `make test-cognitive-graph-matrix` now also exercises multi-tenant isolation, duplicate-path deduplication, numeric pagination boundaries, stale AGE edge cleanup, partial AGE setup, self-loop chains, realistic load smoke, and SOC loader structural checks.

### Added — audit fixes (10 missing features)

- **`cmd/migrate`**: idempotent migration runner (`schema_migrations` tracking); built into the legacy Dockerfile as `/pcmi-migrate`; Helm init-container when `migrations.enabled=true`.
- **Helm**: `networkpolicy.yaml` (API + worker isolation), `worker-hpa.yaml` (custom metric `pcmi_distillation_queued_jobs`), `values.yaml` keys `networkPolicy`, `migrations`, `worker.autoscaling`.
- **Prometheus**: webhook dead-letter alert rules in `deploy/prometheus/alerts.yaml`.
- **Embedding**: `ValidateModelDimension()` at startup — fails fast when `EMBEDDING_MODEL` native dimension ≠ DB `VECTOR(1536)`.
- **Python SDK**: session lifecycle (`create_session`, `end_session`, `store_session_memory`, `list_session_memories`, `promote_session`).

### Fixed

- **Cognitive Graph**: Cypher tenant scoping now preserves the original `WHERE` body before `RETURN`, escapes tenant literals, and applies `link_types` filters to every edge in a multi-hop path instead of only the first edge.
- **Cognitive Graph**: graph availability now requires the specific `pcmi_memory_graph`; Cypher passthrough scopes every `:Memory` alias and rejects dollar-quote delimiters, semicolon-separated statements, SQL write/control keywords, and dangerous keywords split by tabs/newlines.
- **Cognitive Graph**: related traversal now tenant-scopes target nodes, deduplicates memories reachable through multiple paths, and paginates over numerically sorted memory IDs instead of AGE's lexicographic `memory.<id>` strings.
- **Cognitive Graph**: AGE sync trigger now removes stale graph edges when `memory_links` rows are deleted or re-keyed, with integration coverage for graph drift cleanup.
- **Graph datasets**: SOC loader checkpoints are tied to the current CSV fingerprint, partial `--limit` loads now create links between loaded endpoints even when matching link rows appear later in the CSV, and link creation retries handle API 429 rate limits.
- **SDK docs**: README, SDK guides, and quickstart commands now point to the published PyPI `pcmi` and npm `@marco-spagn/pcmi-sdk` packages instead of stale pending/local-only install text.
- **Worker expiry**: the expiry worker now honours the first-class `expires_at` column (set via the `expires_at` API/gRPC field and indexed by migration 011), not only the legacy `metadata.ttl_seconds`. Memories stored with an explicit `expires_at` previously never expired despite the documented behaviour in `docs/WORKERS-AND-EVENTS.md`.
- **Worker contradiction detection**: the recent-memory scan queried a non-existent `text_content` column and used an invalid `path !~ '*.consolidated.*'` predicate, so the query errored on every run and the feature (enabled by default) silently did nothing. It now selects `content`, excludes consolidated entries with a valid ltree filter, and only compares current (`valid_to IS NULL`) rows.
- **Worker distillation**: 3-minute job timeout; `distillation_runs` rows marked `completed` after success.
- **Import**: ltree path validation per entry; generic `store failed` instead of raw DB errors.
- **Rate limit**: startup warning when using in-memory backend with multiple API replicas.

### Changed — CI/CD refactor

- **Workflow** (`.github/workflows/ci.yml`): documented job DAG, per-job `timeout-minutes`, concurrency cancel-in-progress, failure log artifacts for smoke/E2E.
- **Version drift**: smoke health checks use `scripts/ci/resolve_version.sh` / `pcmi-resolve-version` action (reads `internal/version/version.go`) instead of hardcoded tags in YAML.
- **Parity**: `scripts/ci/phases/` (A–G) shared with `scripts/ci_like_github.sh`; Postgres migrate, Trivy, API worker lifecycle, and coverage env centralized under `scripts/ci/`.
- **Docs**: [docs/local-ci.md](docs/local-ci.md) matrix, CONTRIBUTING and PR template aligned.
- **Coverage badge**: `go` job commits `badges/coverage.json` to `main` when changed (`[skip ci]`, `paths-ignore: badges/**`). Branch protection must allow **GitHub Actions** bypass (or optional `BADGE_UPDATE_TOKEN` PAT) — see [docs/github-branch-protection.md](docs/github-branch-protection.md).

## [1.49.0] — 2026-05-24

### Added — AI framework examples (PCMI-016)

- **`examples/langchain`**, **`examples/llamaindex`**, **`examples/autogen`**, **`examples/crewai`**: minimal store / retrieve (and LangChain session working memory) via shared [`examples/pcmi_http.py`](examples/pcmi_http.py).
- Each directory includes `README.md`, `requirements.txt`, `main.py`, and `smoke_test.py` (structural by default; set `PCMI_SMOKE_LIVE=1` for live API checks).
- **Makefile**: `examples-smoke-structural`, `examples-smoke` (Docker infra + live smoke).
- **Tests**: `internal/deploy/examples_test.go` layout guard.

## [1.48.0] — 2026-05-24

### Added — Changelog & API versioning policy (PCMI-015)

- **[docs/API-VERSIONING.md](docs/API-VERSIONING.md)**: SemVer rules for the HTTP/gRPC contract, release checklist, conventional commits, and client expectations.
- **`cliff.toml`**: [git-cliff](https://git-cliff.org/) configuration (Keep a Changelog groupings).
- **[`.github/workflows/release.yml`](.github/workflows/release.yml)**: on tag `vX.Y.Z`, verify tag matches `internal/version/version.go`, generate notes with git-cliff, publish GitHub Release.
- **Makefile**: `changelog-unreleased`, `changelog-tag TAG=vX.Y.Z`.
- **Tests**: `internal/deploy` guards for release workflow, `cliff.toml`, and API versioning doc.

## [1.47.0] — 2026-05-24

### Added — Cursor-based pagination (PCMI-014)

- **List endpoints** accept `limit` (1–200, endpoint-specific default), opaque `cursor`, and legacy `after_id` (mutually exclusive with `cursor`). Responses include `limit`, `next_cursor`, and `has_more`.
- **Handlers**: audit, memory links, webhooks (+ dead-letter), distilled, distillation policies/runs, admin tenants/API keys, memory history.
- **`total` backward compatibility**: full-table count on `GET /v1/audit` and `GET /v1/admin/tenants`; page row count on `GET /v1/memories/history` and `GET /v1/distilled`. Other paginated lists omit `total` — use `has_more` and `next_cursor`. `GET /v1/audit` still returns `offset: 0` (offset pagination removed).
- **`after_id` restrictions**: rejected together with `cursor`; not supported on admin tenant/key lists or webhook lists (use `cursor` from `next_cursor`).
- Makefile target: `test-pagination`.

## [1.46.0] — 2026-05-23

### Added — Go HTTP SDK (PCMI-013)

- **`sdk/go/pcmi/`**: Go 1.25 client for store, retrieve, sessions, admin, and SSE events.
- **`sdk/go/examples/basic`**: live smoke against a running API.
- Makefile: `sdk-go-test`, `sdk-go-smoke`, `sdk-all`.

## [1.45.0] — 2026-05-23

### Added — Automatic distillation policy engine (PCMI-012)

- Schema **`migrations/018_distillation_policy.sql`**: `distillation_policies`, `distillation_runs`.
- Worker policy engine: count threshold, optional max-age trigger, minimum interval between runs.
- API: `POST/GET /v1/distillation/policies`, `PATCH /v1/distillation/policies/{id}`, `GET /v1/distillation/runs`.
- Makefile: `test-distillation-policy`, `distillation-policy-e2e`.
- Env **`DISTILLATION_POLICY_DISABLED`**: skip policy engine (explicit refine / smoke only).

## [1.44.0] — 2026-05-23

### Added — Content-hash deduplication at ingest (PCMI-011)

- Schema **`migrations/017_dedup.sql`**: `content_hash` on `memory_entries`; partial index on current rows.
- Ingest modes **`DEDUP_MODE`**: `none` | `skip` | `link` | `merge` (env default, `tenants.settings.dedup_mode`, request `dedup_mode` or `X-Dedup-Mode`).
- Normalized SHA-256 hash (trim, lowercase, Unicode NFC) for duplicate detection.
- `link` creates `memory_links` with `link_type=duplicate` across paths; `merge` updates metadata/tags in place.
- Makefile: `test-dedup`.

## [1.43.0] — 2026-05-23

### Added — Session & working memory layer (PCMI-010)

- Schema **`migrations/016_sessions.sql`**: `agent_sessions`; partial index on
  `memory_entries(metadata->>'session_id')`.
- API: `POST /v1/sessions`, `POST/GET /v1/sessions/{id}/memories`,
  `POST /v1/sessions/{id}/promote`, `DELETE /v1/sessions/{id}`.
- Working memory rows carry `metadata.session_id` until promotion clears scope and
  rewrites paths under `target_prefix` (default `root`).
- Makefile: `test-sessions-integration`; docs in [SESSIONS.md](docs/SESSIONS.md).

## [1.42.0] — 2026-05-23

### Added — Memory importance scoring & temporal decay (PCMI-009)

- Schema **`migrations/015_importance_decay.sql`**: `importance`, `access_count`,
  `last_accessed_at` on `memory_entries`; per-tenant fusion weights in `tenant_memory_config`.
- Hybrid score: `W_s×cosine + W_l×bm25 + W_i×importance + W_r×exp(-ln(2)/halflife×age)`.
- API: `POST /v1/memories` `{ "importance" }`, `POST /v1/retrieve` `{ "decay_enabled" }`,
  `PATCH /v1/memories/{path}/importance`.
- Makefile: `test-retrieval-scoring`, `bench-retrieval`; docs updated in [retrieval-pipeline.md](docs/retrieval-pipeline.md).

## [1.41.0] — 2026-05-23

### Added — MCP server for AI agents (PCMI-008)

- **`cmd/mcp`**: stdio MCP server (`bin/pcmi-mcp`) with JSON-RPC 2.0 — five tools
  (`pcmi_store`, `pcmi_retrieve`, `pcmi_get_history`, `pcmi_list_paths`, `pcmi_create_link`)
  and two resources (`pcmi://memory/{path}`, `pcmi://stats`).
- **Env**: `PCMI_BASE_URL`, `PCMI_API_KEY`.
- **Makefile**: `build-mcp`, `install-mcp`, `test-mcp-unit`, `test-mcp-smoke`, `mcp-e2e`.
- **Docs**: [docs/MCP.md](docs/MCP.md), [cmd/mcp/README.md](cmd/mcp/README.md).

## [1.40.0] — 2026-05-23

### Added — API key rotation and lifecycle (PCMI-007)

- Schema **`migrations/014_key_lifecycle.sql`**: `rotated_to`, `rotation_grace_ends_at`,
  `last_used_ip` on `api_keys`; rotation creates a new key while the previous hash remains
  valid for a 24h grace period.
- Admin endpoints: **`POST /v1/admin/api-keys/{id}/rotate`**, **`DELETE /v1/admin/api-keys/{id}`**,
  enhanced **`GET /v1/admin/api-keys`** (lifecycle fields).
- Middleware and gRPC auth honor expiry, revocation, and rotation grace; **`last_used_at`** /
  **`last_used_ip`** updated on each successful request.
- Audit event **`api_key_rotation`** on rotate; Makefile target **`test-key-lifecycle`**.

## [1.39.0] — 2026-05-22

### Added — Webhook HMAC-SHA256 signatures (PCMI-006)

- Outbound webhook deliveries sign `timestamp + "." + body` with the endpoint secret;
  headers **`X-PCMI-Signature`**, **`X-PCMI-Timestamp`**, and **`X-PCMI-Delivery-ID`**.
- **`internal/crypto/hmac.go`** (`HMACSign` / `HMACVerify`) and **`internal/webhook/delivery.go`**.
- SDK helpers: Python **`verify_signature`**, TypeScript **`verifySignature`**.
- Documented in **`docs/WORKERS-AND-EVENTS.md`**.

## [1.38.0] — 2026-05-22

### Added — Idempotency keys on store (PCMI-005)

- **`X-Idempotency-Key`** (UUID) on `POST /v1/memories`: successful responses are cached
  per tenant for 24h; duplicates return the cached body with
  **`X-Idempotency-Replayed: true`**.
- Table **`idempotency_cache`** (`migrations/013_idempotency.sql`), middleware
  **`internal/middleware/idempotency.go`**, and expiry-worker purge of stale rows.
- Makefile target **`test-idempotency`**.

## [1.37.0] — 2026-05-22

### Added — Prometheus /metrics authentication (PCMI-004)

- **`METRICS_SCRAPE_TOKEN`**: when set, `GET /metrics` requires
  `Authorization: Bearer <token>`; when unset, `/metrics` stays open and the API
  logs a startup warning.
- **`middleware.MetricsScrapeAuth`**, example `deploy/prometheus/prometheus.yml`,
  and docs in `docs/USAGE.md`.

## [1.36.0] — 2026-05-22

### Added — Distributed rate limiting via Redis (PCMI-003)

- **`internal/ratelimit`**: sliding-window limiter using Redis `ZADD`/`ZCARD` with an
  atomic Lua script; shared counters across API replicas.
- **`RATE_LIMIT_BACKEND`**: `memory` (default, in-process Fiber limiter) or `redis`;
  `RATE_LIMIT_WINDOW_SECS` and `RATE_LIMIT_MAX_REQUESTS` tune the Redis window.
- **Fail-open**: Redis errors allow the request so availability is not tied to Redis health.
- **Makefile** target `test-ratelimit-integration`.

## [1.35.0] — 2026-05-22

### Added — Circuit breaker for OpenAI embedding (PCMI-002)

- **`internal/embedding`**: `CircuitBreakerProvider` wraps all providers from
  `NewFromConfig` with `github.com/sony/gobreaker/v2` and `golang.org/x/time/rate`.
- **Worker metrics**: `pcmi_embedding_circuit_state`, `pcmi_embedding_requests_total`,
  `pcmi_embedding_latency_seconds` on `metrics.WorkerRegistry`.
- **Makefile** target `test-circuit-breaker`; embedding worker logs via `slog`.

## [1.34.0] — 2026-05-22

### Added — Redis Streams durable event bus (PCMI-001)

- **Redis Streams** (`pcmi:events`) with `XADD` / `XREADGROUP` / `XACK` as the default
  event transport (`EVENT_BACKEND=streams`); legacy pub/sub on `memory_events` remains
  available via `EVENT_BACKEND=pubsub`.
- **`internal/event/stream.go`** — `StreamPublisher`, `StreamConsumer`, SSE/gRPC tail via
  non-grouped `XREAD`; **`stream_consumer.go`** — pending reclaim (`XCLAIM`) and DLQ
  (`pcmi:events:dlq`) after max deliveries.
- **Worker** consumes group `pcmi-workers` with pending recovery; **metrics**:
  `pcmi_stream_pending_total`, `pcmi_stream_ack_total`, `pcmi_stream_dlq_total`.
- **Makefile** target `test-streams-integration`; unit + integration tests for publish,
  consume, ACK, pending recovery, and load-balanced consumers.

### Added — Synthetic data CLI & E2E cleanup

- **`scripts/pcmi_synth/`** — unified synthetic memory generator: presets
  (`soc`, `finance`, `advertising`, `healthcare`, `custom`), `--num`, `--seed`,
  sharding aligned with `DISTILLATION_BATCH_SIZE`, optional `--llm` + `--domain`.
- **`scripts/distill_e2e.sh`** — simple wrapper for distillation E2E.
- **Makefile:** `synth-list`, `synth-generate`, `distill-smoke`; `distillation-e2e`
  accepts `PRESET`, `SYNTH_NUM`, `SYNTH_SEED`.
- **Moved** root `test_*.sh` → `scripts/e2e/` (compat stub `test_pcmi.sh` at repo root).
- **`run_pcmi_distillation_test.sh`** — `--preset`, `--llm`, `--domain`; uses `pcmi_synth`.

### Added — Security (PR #3)
- **gRPC in-process TLS.** `internal/grpc/server.go` now exposes
  `BuildServerOptions(*config.Config)`, which appends `grpc.Creds(...)` built
  from `cfg.TLSCertFile` / `cfg.TLSKeyFile` when both are set and readable.
  `Start(...)` uses it, so a single Config field now controls TLS on both
  the Fiber HTTP plane and the gRPC plane — matching the
  "gRPC senza TLS in-process" tech-debt entry.
  Misconfigurations (only one of cert/key set, file unreadable, malformed
  PEM) log a warning and fall back to plain TCP rather than deadlocking
  the gRPC plane.
- **CodeQL SAST workflow** (`.github/workflows/codeql.yml`) — matrix scan
  over Go, Python, JavaScript/TypeScript with the `security-and-quality`
  query pack, plus a weekly cron and PR/push triggers. Findings land in
  `Security → Code scanning alerts`. Closes the "SAST oltre govulncheck"
  tech-debt row.
- New unit tests `internal/grpc/tls_test.go`:
  - `TestBuildServerOptions_NoTLSWhenUnset` — default-path lock-in.
  - `TestBuildServerOptions_TLSEnabled` — generates an ECDSA self-signed
    cert+key in `t.TempDir()` and asserts the option list is length 2.
  - `TestBuildServerOptions_PartialTLSFallsBack` — only-cert / only-key.
  - `TestBuildServerOptions_BadCertFallsBack` — malformed PEM.
  - `TestBuildServerOptions_ReturnTypeIsServerOption` — type-safety
    paranoia for future grpc/v2 bumps.
- **TLS handshake + RPC:** `TestTLSHandshakeEndToEnd_HealthRPC` in
  `internal/grpc/tls_handshake_test.go` registers the standard gRPC health
  service and issues `Health.Check` after the channel reaches `READY`, so TLS
  misconfiguration cannot hide behind a handshake-only test.
- README: new `CodeQL` badge alongside CI / Coverage / Go / License / API.

### Fixed — Helm (`deploy/helm/pcmi`)
- **`values.schema.json`:** `otel.endpoint` must allow `""` (tracing off) or a
  non-empty URI. Requiring `format: uri` for the empty default broke
  `helm lint deploy/helm/pcmi --strict`.

### Added — Deploy / CI artifact tests
- **`TestCIWorkflowYAMLValid`** — `.github/workflows/ci.yml` parses; core jobs
  (`ci-gate`, `golangci-lint`, `security`, `helm-lint`, `go`) + PR permissions.
- **`TestAllGitHubWorkflowYAMLFilesParse`** — every `.github/workflows/*.yml`.
- **`TestDockerComposeYAMLValid`** — compose services `postgres`, `redis`, `api`, `worker`.
- **`TestComposeReferencesExistingMigrations`** — migration bind-mount paths exist.
- **`TestOpenAPIYAMLValid`** — `docs/openapi.yaml` is valid YAML + OpenAPI 3.x paths.
- **`TestCriticalShellScriptsSyntaxOK`** — `bash -n` on key `scripts/*.sh`.
- **`TestProtoMemoryServiceProtoExists`** — `proto/pcmi/v1/memory.proto` markers.
- `internal/deploy/codeql_workflow_test.go` — CodeQL workflow structure + SARIF permissions.
- `internal/deploy/helm_test.go` — Chart/values/schema + optional `helm` on PATH.
- **`scripts/test_all_local.sh`** — quick path adds golangci-lint, govulncheck,
  `./internal/deploy/...`, gRPC TLS subset, Helm/kubeconform, **`go mod verify`**,
  **`./internal/version/...`**.

### Fixed — Docs / OpenAPI
- **`docs/openapi.yaml`** — SSE route description no longer uses an unquoted `{ ... }`
  fragment (broke strict YAML parsers such as `yaml.v3`).

### Fixed — CodeQL pack config
- `.github/codeql/codeql-config.yml` no longer duplicates `queries:` — query
  packs stay on the workflow `init` step only, avoiding CLI conflicts on some
  runners.

### Fixed — CodeQL SARIF upload on repos without Code scanning
- `.github/workflows/codeql.yml`: SARIF `upload` defaults to `never` unless
  repository Actions variable `CODEQL_UPLOAD_SARIF` is set to `true`, so PR
  workflows no longer fail with “Code scanning is not enabled” before the
  feature is turned on in GitHub settings.

### Notes — PR #3
- No DB / API breaking changes. `PCMI_TLS_CERT` / `PCMI_TLS_KEY` were
  already in Config and `.env.example` for the HTTP server; PR #3 simply
  teaches the gRPC server to honour them too.
- Self-signed certs in tests use `crypto/ecdsa` + `crypto/x509` only,
  so no new modules join `go.sum`.

### Added — Configuration & Env (PR #2)
- `Config.OpenAIBaseURL` (env `OPENAI_BASE_URL`).
- `internal/config/getenv_audit_test.go`: regression test that walks
  `cmd/` and `internal/` (excluding `_test.go` and `internal/config/`)
  and fails the build if a direct `os.Getenv` call lands in production.
- `internal/config/config_pr2_test.go`: `.env.example` ↔ `config.go`
  drift guard.
- `internal/grpc/start_port_test.go`: `ResolveGRPCPort` table coverage.
- CI: dedicated audit step runs before the integration suite so rogue
  `os.Getenv` fails in 5 s instead of 5 min.
- `scripts/test_all_local.sh`: A5b runs the audit; A4 widened to
  `./internal/... ./cmd/...`.

### Changed — Configuration & Env (PR #2)
- `internal/grpc/server.go`: `Start(...)` reads `cfg.GRPCPort` via
  `ResolveGRPCPort` — `os.Getenv("GRPC_PORT")` removed from production.
- Default-value drift fixes in `config.Load()` so `.env.example`, Config,
  and consuming middleware now agree:
  - `REDIS_ADDR`: `redis:6379` → `localhost:6379`.
  - `RATE_LIMIT_RPM`: `60` → `120` (aligned with the middleware default).
  - `PRUNE_INTERVAL_SECS`: `3600` → `21600` (6 h).
  - `DISTILLATION_BATCH_SIZE` Validate range: `1–1000` → `1–200` (matches
    the runtime cap in `worker/distillation_helpers.go`).
- `cmd/api/main.go`: passes `cfg` to `grpcserver.Start`.

### Added — Admin UI, embedding providers, cursor contracts (PR #61)

- **`GET /v1/admin/ui`**: embedded HTML admin dashboard (`internal/handler/adminui/`) — health, tenants, API keys, observability pointers; requires admin API key.
- **`embedding.NewFromConfig`**: selects OpenAI vs Azure OpenAI vs OpenAI-compatible HTTP endpoints from `OPENAI_BASE_URL` / `OPENAI_API_KEY` / `EMBEDDING_MODEL` (`internal/embedding/factory.go` + tests).
- **Cursor pagination helpers** (`internal/model/cursor.go`): opaque keyset cursor encode/decode; optional `cursor` / `next_cursor` / `has_more` fields on memory retrieve DTOs (`internal/model/memory.go`).
- **`deploy/helm/IDE_NOTES.md`**: notes on Helm + YAML extension diagnostics.

### Added — Cursor pagination wiring + gRPC admin/metrics (follow-up)

- **Memory retrieve keyset pagination**: path-only `POST /v1/retrieve` (no `query`, no vector search) uses `(created_at, id)` ordering with opaque `next_cursor` / `has_more` (`internal/repository/memory_repository.go`, `internal/service/memory_service.go`).
- **Generated gRPC stubs**: `internal/grpc/pcmiv1/admin.pb.go`, `admin_grpc.pb.go`, `metrics.pb.go`, `metrics_grpc.pb.go`.
- **`AdminService` gRPC server** (`internal/grpc/admin_server.go`): mirrors HTTP `/v1/admin/*` tenant and API-key operations; requires admin API key.
- **`MetricsService` gRPC server** (`internal/grpc/metrics_server.go`): `Scrape` and `StreamScrape` over the Prometheus registry; `GetMetric` returns `Unimplemented` for now.

### Added — Quality & CI (PR #1)
- `scripts/ci_coverage_check.sh`: pure-bash/awk script that parses
  `coverage.out`, computes per-package + global statement coverage, and exits
  non-zero when configurable thresholds are not met. No `go tool cover`
  dependency.
- `make test-cover`, `make cover-check`, `make cover-report` Makefile targets
  wrapping the gate locally.
- New unit tests:
  - `internal/config/config_helpers_test.go` — direct coverage of the unexported
    `envOr` / `envInt` / `envBool` helpers, plus extra `Validate(...)` cases
    (admin key, encryption key, multi-error path) and `PruneInterval` /
    `ExpiryInterval` duration accessors.
  - `internal/event/schema_test.go` — locks down the public event-type
    constants (these are part of the webhook + gRPC stream contract) and
    round-trips a `UniversalEvent` through `encoding/json`, including the
    `omitempty` behaviour for `agent_id` / `correlation_id`.
- Coverage artifact uploaded by CI (`coverage.out` + `coverage-summary.txt` +
  `coverage-summary.md` + `coverage-badge.txt`) for 14 days.
- Coverage report rendered **inside every PR**:
  - written to `$GITHUB_STEP_SUMMARY` so reviewers see the per-package table
    when they click into the `go` check;
  - posted as a **sticky comment** on the PR via
    `marocchino/sticky-pull-request-comment@v2` (header `pcmi-coverage`,
    updated in-place on every push);
  - shields.io badge URL emitted in `coverage-badge.txt` and surfaced in the
    job summary, ready to be wired into the README on each release.
- README badges row (CI / Coverage / Go version / License / API version) at
  the top of `README.md`, plus an explanatory blurb that points reviewers at
  the gate definition in CI.

### Changed — Quality & CI (PR #1)
- `.github/workflows/ci.yml`: the workflow now grants `pull-requests: write`
  (needed for the sticky PR comment) and the `go` job runs the coverage gate
  after the per-function summary. Initial thresholds:
  - Global ≥ **22 %**
  - `config` ≥ 70 %, `event` ≥ 70 %, `eventschema` ≥ 85 %, `metrics` ≥ 70 %,
    `version` ≥ 80 %
  These floors are intentionally calibrated so the current `main` is green and
  any further work that drops coverage breaks the build. Subsequent PRs are
  expected to ratchet the floor up — never down.
- `docs/local-ci.md`: documented the coverage gate and the matching make
  targets in the "Day-to-day commands" table and a new "Coverage gate" section.

### Added — Dynamic coverage badge + extra tests (PR #1 follow-up)
- **Fully dynamic coverage badge** on the README. `scripts/ci_coverage_check.sh`
  now writes a shields.io endpoint JSON to `badges/coverage.json` (controlled
  by the new `COVERAGE_ENDPOINT_OUT` env var). On every push to `main`, the
  `go` CI job commits the regenerated file back to the branch with a
  `[skip ci]` marker, so the badge displayed at the top of `README.md` always
  reflects the latest measured coverage — no Codecov, Coveralls, gist or
  external service required.
- Workflow safeguards against badge-update infinite loops:
  - `on.push.paths-ignore: ['badges/**']` so the auto-commit cannot re-trigger
    the workflow;
  - `[skip ci]` token in the commit message as belt-and-braces protection;
  - `contents: write` granted only at the `go` job level (not workflow-wide).
- Coverage scope widened: `crypto`, `embedding`, `model`, `telemetry` added
  to `COVERAGE_PKGS` in `Makefile`. These packages already shipped tests but
  were not contributing to the global denominator before; folding them in
  gives the gate an honest signal.
- Additional unit tests (deterministic, no DB/Redis/network):
  - `internal/worker/distillation_env_extra_test.go` — boundary, whitespace,
    non-numeric, and out-of-range cases for the `DISTILLATION_BATCH_SIZE` /
    `DISTILLATION_CONCURRENCY` env parsers.
  - `internal/metrics/worker_unknown_test.go` — empty-label branch of
    `IncWorkerRedisEvent` (relabelled to `unknown`) and custom-label path.
  - `internal/handler/events_handler_extra_test.go` — whitespace-only filters,
    dedup, empty entries, non-string `tenant_id` payloads.
  - `internal/middleware/public_extra2_test.go` — systematic method × path
    matrix for `IsUnauthenticatedProbe`, plus negative cases.

### Notes
- No production code paths were touched in this PR — additions are
  test-only, CI-only, and documentation. Zero risk of runtime behaviour
  change; no database migrations required.
- The auto-commit step requires the workflow's default `GITHUB_TOKEN` to
  have `contents: write` on `main`. If branch protection is enabled, add the
  `pcmi-ci[bot]` author to the allowlist or use a separate deploy key —
  otherwise the badge update step will fail soft (`git push` rejected) and
  the badge will stay frozen at the last successful update.
