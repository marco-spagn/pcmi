# PCMI Usage Guide

How to connect agents, services, and orchestrators to **Persistent Cognitive Memory Infrastructure**.

## Prerequisites

1. **PostgreSQL** with extensions `ltree` and `pgvector` (see `docker-compose.yml`).
2. **Redis** for events and workers.
3. **API key** tenant (`X-API-Key` header or `api_key` gRPC field) — in dev: `testkey123` (migration `003_rbac_api_keys.sql`, role `admin`).
4. Optional: **`OPENAI_API_KEY`** for automatic embeddings, semantic retrieval, and LLM summarization.

```bash
cp .env.example .env
docker compose up -d --build
curl -s http://localhost:8000/v1/health
```

## Choosing a transport

```mermaid
flowchart LR
  subgraph clients [Client]
    A[Agent / App]
  end
  subgraph transports [Transports]
    H[HTTP REST + SSE]
    G[gRPC]
  end
  subgraph api [pcmi-api]
    F[Fiber :8000]
    GR[gRPC :50051]
  end
  A --> H --> F
  A --> G --> GR
  F --> PG[(PostgreSQL)]
  GR --> PG
  F --> R[(Redis)]
```

| Need | Recommended |
|------|-------------|
| Browser, curl, OpenAPI | **HTTP** |
| Throughput, batch, stream retrieve/events | **gRPC** |
| Official Python/TS SDKs | **HTTP** (wrappers in `sdk/`) |
| Tenant bootstrap / admin UI | **HTTP** (`/v1/admin/*`, `GET /v1/admin/ui`) or **gRPC** `AdminService` |
| List tenants/keys in dev (without curl) | **`make admin-list-keys`** (CLI `cmd/pcmi-admin`) |
| Prometheus metrics | **HTTP** `GET /metrics` or **gRPC** `MetricsService.Scrape` (see authentication below) |

RPC detail: [grpc-vs-http.md](grpc-vs-http.md).

### MCP (agents in Cursor / Claude)

stdio server **`pcmi-mcp`** (`cmd/mcp`): tools `pcmi_store`, `pcmi_retrieve`, … — see **[MCP.md](MCP.md)**.

```bash
make build-mcp
make test-mcp-smoke    # JSON-RPC handshake
```

---

## HTTP — basic operations

### Authentication

Header on every request (except `/health`, `/metrics`, `/ready`):

```http
X-API-Key: testkey123
Content-Type: application/json
```

Roles: `readonly` (read-only), `write`, `admin` (tenant/key management).

### Admin — API key lifecycle (role `admin`)

| Action | HTTP |
|--------|------|
| Create tenant | `POST /v1/admin/tenants` |
| List tenants | `GET /v1/admin/tenants` |
| Create key | `POST /v1/admin/api-keys` |
| List keys | `GET /v1/admin/api-keys` |
| Rotate | `POST /v1/admin/api-keys/{id}/rotate` → new secret (returned once in response) |
| Revoke | `DELETE /v1/admin/api-keys/{id}` |

gRPC equivalent: `AdminService` (`CreateAPIKey`, `RotateAPIKey`, …). In dev, without curl admin:

```bash
make admin-list-keys
# Filter by tenant: go run ./cmd/pcmi-admin list --tenant default
```

Integration test: `make test-key-lifecycle`.

### Cursor-based pagination on list endpoints (PCMI-014)

Endpoints that return long lists use **keyset pagination** (not `OFFSET`). Common query parameters:

| Parameter | Description |
|-----------|-------------|
| `limit` | Rows per page, **1–200** (default depends on endpoint, e.g. `50` on audit/history, `100` on `GET /v1/admin/tenants`) |
| `cursor` | Opaque string from `next_cursor` in the previous response |
| `after_id` | Legacy alias: builds a cursor when `cursor` is absent. **Do not** use together with `cursor` (400) |

Common response fields: `limit`, `next_cursor` (empty if end of list), `has_more`.

| Endpoint | Array key | `total` | Notes |
|----------|-----------|---------|-------|
| `GET /v1/audit` | `entries` | **Global** count (with `since` filter) | `offset` always returned as `0` (deprecated) |
| `GET /v1/admin/tenants` | `tenants` | **Global** tenant count | `after_id` not supported — use `cursor` only |
| `GET /v1/admin/api-keys` | `api_keys` | absent | `after_id` not supported |
| `GET /v1/memories/history` | `entries` | Rows **in this page** | `path` required |
| `GET /v1/distilled` | `entries` | Rows **in this page** | `path_prefix` required |
| `GET /v1/distillation/policies` | `policies` | absent | |
| `GET /v1/distillation/runs` | `runs` | absent | optional `policy_id` |
| `GET /v1/webhooks`, `GET /v1/webhooks/dead-letter` | `entries` | absent | webhooks: `cursor` only |
| `GET /v1/memories/links` | `entries` | absent | filters `from_path`, `to_path`, `link_type` |

Example (audit):

```bash
curl -s "${PCMI_BASE_URL}/v1/audit?limit=10" -H "X-API-Key: ${PCMI_API_KEY}" | jq '{total, has_more, next_cursor}'
# next page:
CUR=$(curl -s "${PCMI_BASE_URL}/v1/audit?limit=10" -H "X-API-Key: ${PCMI_API_KEY}" | jq -r .next_cursor)
curl -s "${PCMI_BASE_URL}/v1/audit?limit=10&cursor=${CUR}" -H "X-API-Key: ${PCMI_API_KEY}"
```

The CI smoke [`scripts/ci_integration_smoke.sh`](../scripts/ci_integration_smoke.sh) verifies `audit.total`, `admin/tenants.total`, and the presence of arrays on distilled; for dead-letter uses `entries | length` (not `total`).

OpenAPI contract: [openapi.yaml](openapi.yaml). Test: `make test-pagination`.

### Prometheus metrics (`GET /metrics`)

The endpoint does not use `X-API-Key`. In production set **`METRICS_SCRAPE_TOKEN`** on the API and configure Prometheus with the same secret:

| `METRICS_SCRAPE_TOKEN` | Behavior |
|------------------------|----------|
| not set | `GET /metrics` is open; at startup the API logs a **WARNING** |
| set | requires `Authorization: Bearer <token>` |

```bash
# Example scrape with token
export METRICS_SCRAPE_TOKEN="$(openssl rand -hex 32)"
curl -s -H "Authorization: Bearer ${METRICS_SCRAPE_TOKEN}" http://localhost:8000/metrics
```

Example `deploy/prometheus/prometheus.yml`: `authorization.credentials` or `bearer_token` aligned with the API token.

### Store and retrieve

```bash
export PCMI_BASE_URL=http://localhost:8000
export PCMI_API_KEY=testkey123

# Store
curl -s -X POST "$PCMI_BASE_URL/v1/memories" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path":"root.project.note","content":"Decision X","tags":["decision"],"embedding_model":"unspecified"}'

# Retrieve (hybrid: prefix + text + optional semantics)
curl -s -X POST "$PCMI_BASE_URL/v1/retrieve" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path_prefix":"root.project","query":"decision","limit":10,"tags":["decision"],"tags_match":"all"}'
```

### Live events (SSE)

```bash
curl -sN "$PCMI_BASE_URL/v1/events" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -H "Accept: text/event-stream" \
  -d '?types=memory.stored,memory.updated'
```

### Store idempotency (`X-Idempotency-Key`)

On `POST /v1/memories` you can send a UUID in the **`X-Idempotency-Key`** header. The first `200` response is cached for **24 hours** per tenant+key; retries return the same JSON with **`X-Idempotency-Replayed: true`**. Only successful responses are cached.

```bash
KEY=$(uuidgen)
curl -s -X POST "$PCMI_BASE_URL/v1/memories" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -H "X-Idempotency-Key: $KEY" \
  -d '{"path":"root.demo.idem","content":"once"}'
# Repeat the same request → same body, header X-Idempotency-Replayed: true
```

Test: `make test-idempotency`.

### Ingest dedup (`DEDUP_MODE`)

Avoids duplicates by **content hash** against the current version of the same path (PCMI-011). Precedence: body `dedup_mode` → header **`X-Dedup-Mode`** → `tenants.settings.dedup_mode` → env **`DEDUP_MODE`**.

| Mode | Behavior |
|------|----------|
| `none` | No dedup (default) |
| `skip` | `409` or response with `status: deduplicated`, `dedup_action: skipped` |
| `link` | New version linked to the existing source |
| `merge` | Updates the existing version |

```bash
export DEDUP_MODE=skip   # in .env or compose
make smoke-dedup         # curl E2E with API on :8000
```

Test: `make test-dedup`.

### Agent sessions

Working memory bound to a session; promotion to long-term memory. See **[SESSIONS.md](SESSIONS.md)** and `make smoke-sessions`.

### Advanced HTTP operations

| Action | Method and path |
|--------|----------------|
| Version history | `GET /v1/memories/history?path=...` |
| Rollback | `POST /v1/memories/rollback` |
| Refine (distillation) | `POST /v1/memories/refine` |
| Importance | `PATCH /v1/memories/{path}/importance` |
| Links between paths | `POST/GET /v1/memories/links` |
| Cognitive graph (AGE spike) | `GET /v1/graph/health`, `GET /v1/graph/related` — see [cognitive-graph.md](cognitive-graph.md) |
| Tenant stats | `GET /v1/stats` |
| Webhook | `POST/GET /v1/webhooks` |
| Compact path | `POST /v1/memories/compact` |
| Sessions | `POST/GET/DELETE /v1/sessions`, `POST .../promote` |

Full contract: [openapi.yaml](openapi.yaml).

---

## gRPC — same backend

Host: `localhost:50051` (env `GRPC_PORT`). API key in message or `x-api-key` metadata.

```bash
# Health (no key required)
grpcurl -plaintext localhost:50051 pcmi.v1.MemoryService/Health

# Go integration test (tag integration):
#   bufconn — migrated Postgres only (:5432), in-process server, no :50051
#   live    — TCP dial on GRPC_HOST (:50051); requires pcmi-api listening
make infra-up                  # postgres :5432, redis :6379, api :8000 + :50051
make infra-wait                # optional if infra-up already waited for /v1/ready
make test-integration-bufconn
make test-integration-live     # fails if :50051 does not respond (Makefile sets GRPC_TEST_API_KEY)

# Redis Streams (miniredis in-process, without Docker):
make test-streams-integration

# Or manual equivalent:
DATABASE_URL=postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable \
  make test-integration-bufconn
GRPC_HOST=localhost:50051 GRPC_TEST_API_KEY=testkey123 \
  go test -tags=integration -count=1 ./internal/grpc -run '^TestGRPC|^TestResolveTenantIntegration$$'
```

On **GitHub**, gRPC live tests run in the **`integration-smoke`** job, not in the `go` job (where `GRPC_TEST_API_KEY` is not set and live tests are skipped). See [local-ci.md](local-ci.md) and [integration-testing.md](integration-testing.md).

Conceptual example (Go):

```go
conn, _ := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
client := pcmiv1.NewMemoryServiceClient(conn)
_, err := client.Store(ctx, &pcmiv1.StoreRequest{
    ApiKey: "testkey123", Path: "root.grpc.demo", Content: "hello",
    EmbeddingModel: "unspecified",
})
```

Available RPCs: store/retrieve/batch/stream, get memory, compact, refine, links, stats, events, webhooks, export/import, … — see [grpc-vs-http.md](grpc-vs-http.md).

---

## Official SDKs

### Python

```bash
cd sdk/python
python3 -m venv .venv && source .venv/bin/activate
pip install -e .
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
python smoke.py
```

```python
from pcmi import PCMIClient
import asyncio

async def main():
    async with PCMIClient("http://localhost:8000", "testkey123") as c:
        await c.store("root.sdk.demo", "content", tags=["demo"])
        r = await c.retrieve("root.sdk.demo", limit=5)
        print(r["total"])
        await c.refine("root.sdk")
        async for ev in c.subscribe(types=["memory.stored"]):
            print(ev["type"])
            break

asyncio.run(main())
```

### TypeScript

```bash
cd sdk/typescript
npm ci && npm run smoke
```

Use `smoke.mts` / `npm run smoke` — **not** heredoc `tsx` on Node 23 (see [sdk/README.md](../sdk/README.md)).

---

## Go integration tests (`-tags=integration`)

Require Postgres (`DATABASE_URL`). HTTP tests in `internal/handler` use miniredis in-process.

**Note — SSE:** `TestIntegrationHTTP_EventStreamMemoryStored` (httptest + Fiber SSE) can **block ~10 minutes** and fail the entire `handler` package on timeout. `newIntegrationHTTPApp` sets `PCMI_SKIP_SSE_HTTPTEST=1` by default; real SSE coverage is in `scripts/ci_integration_smoke.sh`.

```bash
export DATABASE_URL='postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable'
PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -count=1 ./internal/handler/...
```

Details, symptoms, and variables: **[integration-testing.md](integration-testing.md)**.

---

## Makefile (repo root)

| Target | Description |
|--------|-------------|
| `make test` | Go unit tests |
| `make lint` | golangci-lint v2 |
| `make test-integration` | Bufconn (migrated DB) + TCP tests on already-running API |
| `make test-integration-bufconn` | In-process tests only (Postgres + migrations) |
| `make test-integration-live` | gRPC TCP on `GRPC_HOST` (:50051); after `make infra-up` or API on host |
| `make test-streams-integration` | Redis Streams bus in `internal/event` (miniredis, without stack) |
| `make test-integration-handler` | HTTP handler tests (`-tags=integration`); sets `PCMI_SKIP_SSE_HTTPTEST=1` |
| `make infra-up` / `make infra-wait` | Compose stack + wait for `/v1/ready` (:8000) |
| `make admin-list-keys` | Tenant + API key table from Postgres (`DATABASE_URL`; shows hash prefix, not plaintext key). Filter: `go run ./cmd/pcmi-admin list --tenant default` |
| `make free-dev-ports` / `make act-preflight` | Free `:5432` / `:6379` (compose + `act-*` containers) |
| `make test-full-real` | Host CI + optional OpenAI E2E + importance/sessions/dedup smokes + MCP — [local-ci.md](local-ci.md) |
| `make smoke-importance` | Retrieve ranking with importance/decay (curl) |
| `make smoke-sessions` | Agent sessions curl E2E |
| `make smoke-dedup` | Ingest dedup curl E2E |
| `make test-streams-integration` | Redis Streams bus (`internal/event`) |
| `make test-circuit-breaker` | Embedding circuit breaker |
| `make test-ratelimit-integration` | Distributed Redis rate limiter |
| `make test-idempotency` | `X-Idempotency-Key` |
| `make test-key-lifecycle` | Admin rotate/revoke |
| `make test-retrieval-scoring` | Importance + decay in SQL retrieve |
| `make test-sessions-integration` | Sessions handler (`-tags=integration`) |
| `make test-dedup` | Dedup unit + handler |
| `make build-mcp` / `make test-mcp-unit` | MCP stdio server |
| `make act-integration-smoke` | CI job `integration-smoke`: compose PG/Redis + host binaries + `ci_integration_smoke.sh` + gRPC + SDK |
| `make ci-like-github` | Broad parity with CI workflow: lint/vuln/helm, test `-race -tags=integration`, coverage gate, then smoke |
| `make sdk-smoke` | Python + TS smoke (API on :8000) |

---

## Worker and what happens after store

After `POST /v1/memories`, the API publishes `memory.stored` / `memory.updated` on Redis. The **worker** (`cmd/worker`):

1. Generates **embedding** if missing and `OPENAI_API_KEY` is set.
2. May **distill** on events / refine.
3. **Prunes** old closed versions, **compacts** by path (API), **expires** on `expires_at`.

Diagram: [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md).

---

## Main environment variables

| Variable | Default | Effect |
|----------|---------|--------|
| `DATABASE_URL` | compose | Primary Postgres |
| `DATABASE_READ_URL` | — | Read replica |
| `REDIS_ADDR` | `localhost:6379` | Redis (events + rate limit) |
| `EVENT_BACKEND` | `streams` | `streams` = Redis Streams `pcmi:events`; `pubsub` = legacy channel `memory_events` |
| `API_PORT` | `8000` | HTTP |
| `GRPC_PORT` | `50051` | gRPC |
| `METRICS_SCRAPE_TOKEN` | — | If set, `GET /metrics` requires `Authorization: Bearer …` |
| `RATE_LIMIT_DISABLED` | `false` | Disables rate limiting (dev/CI) |
| `RATE_LIMIT_BACKEND` | `memory` | `memory` = in-process limiter; `redis` = shared counters across API replicas |
| `RATE_LIMIT_RPM` / `_READONLY` / `_WRITE` / `_ADMIN` | 120 / 200 / 100 / 30 | RPM per role (redis or memory backend) |
| `DEDUP_MODE` | `none` | Default ingest dedup if tenant/request does not specify |
| `OPENAI_API_KEY` | — | Embedding + semantic retrieve + LLM summarize/distill |
| `DISTILLATION_MODEL` / `DISTILLATION_BATCH_SIZE` | `gpt-4o-mini` / `10` | Worker distillation |
| `PCMI_ENCRYPTION_KEY` | — | Content encryption |
| `PCMI_BASE_URL` / `PCMI_API_KEY` | — | Only **`cmd/mcp`** (not api/worker) |

Full list: `.env.example`.

### Outgoing webhooks (HMAC verification)

At webhook registration set a `secret`. Every delivery POST includes:

- `X-PCMI-Timestamp` — epoch seconds (string)
- `X-PCMI-Signature` — `sha256={hex(HMAC-SHA256(secret, timestamp + "." + body))}`

Consumer-side verification with clock tolerance (~5 min). Detail: [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md).

---

## Recommended paths (`ltree`)

- Use hierarchical prefixes: `root.<tenant>.<project>.<topic>`.
- A **path** identifies a "memory line"; new versions share the same path.
- **Retrieve** with `path_prefix` returns the entire subtree.

See [DATA-MODEL.md](DATA-MODEL.md).

---

## Next steps

- [retrieval-pipeline.md](retrieval-pipeline.md) — how ranking works.
- [failure-modes.md](failure-modes.md) — resilience.
- [examples/README.md](../examples/README.md) — Celery / Temporal / LangChain / LlamaIndex / AutoGen / CrewAI.
