# Integration testing — operational notes

Quick guide for `go test -tags=integration` on handler, service, repository, grpc, embedding, event (Streams).

---

## Prerequisites

- **Postgres** with migrations applied (`DATABASE_URL`, e.g. `postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable`).
- For HTTP handler tests: **Redis** not required on host — tests use **miniredis** in-process.
- For **gRPC live** (`make test-integration-live`): API listening on **`GRPC_HOST`** (default `localhost:50051`) and key `GRPC_TEST_API_KEY` (Makefile default: `testkey123`).

### Ports (summary)

| Port | Use |
|------|-----|
| `5432` | Postgres — bufconn, live, handler integration |
| `6379` | Redis — full stack (`make infra-up`); not needed for bufconn gRPC |
| `50051` | gRPC — only **live** tests and `integration-smoke` job |
| `8000` | HTTP — SDK smoke / `ci_integration_smoke.sh` |

---

## gRPC — bufconn vs live

| Target | Server | Postgres | :50051 | Notes |
|--------|--------|----------|--------|-------|
| `make test-integration-bufconn` | In-process (bufconn) | Yes | No | `-run '^TestIntegrationBufconn_'` |
| `make test-integration-live` | TCP on `GRPC_HOST` | Yes (env) | **Yes** | `-run '^TestGRPC\|^TestResolveTenantIntegration$'` |
| `make test-integration` | Both in sequence | Yes | Yes (live phase) | |

If `GRPC_TEST_API_KEY` **is not** set, live tests in `internal/grpc/integration_test.go` call `t.Skip`. The Makefile always sets `GRPC_TEST_API_KEY=testkey123` → without an API on `:50051` the run **fails** (is not skipped).

Recommended sequence for live:

```bash
make infra-up
make test-integration-live
# optional, after infra-up: make infra-wait   # GET /v1/ready on :8000
```

Deps only + API on host:

```bash
make infra-deps-up
go run ./cmd/api    # another terminal
make test-integration-live
```

**GitHub CI:** the `go` job runs `go test -tags=integration ./internal/...` **without** `GRPC_TEST_API_KEY` → live skipped. The **`integration-smoke`** job starts API/worker and runs `go test -tags=integration ./internal/grpc/...` with the key — parity with `make act-integration-smoke` locally. See [local-ci.md](local-ci.md).

---

## Redis Streams (`make test-streams-integration`)

Tests in `internal/event/` with tag `integration` and **miniredis** in-process (`TestStreamIntegration_*`). No Postgres or `:50051` required. Useful after changes to the Streams backend in `internal/event`.

```bash
make test-streams-integration
```

---

## Recommended command (handler + embedding)

```bash
export DATABASE_URL='postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable'

# PCMI_SKIP_SSE_HTTPTEST=1 is the default if you use newIntegrationHTTPApp; explicit when running go test manually:
PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -count=1 ./internal/handler/...

# With embedding in the same run:
PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -count=1 ./internal/handler/... ./internal/embedding/...
```

`make ci-like-github` already sets `PCMI_SKIP_SSE_HTTPTEST=1` in Phase F (`go test ./internal/...`).

---

## Known issue: `TestIntegrationHTTP_EventStreamMemoryStored` (SSE + httptest)

### Symptom

- `go test -tags=integration ./internal/handler/...` **appears blocked** for many minutes without output.
- After **~10 minutes** it fails with Go **package timeout** (default 10m), stack trace in `miniredis` / `netpoll` / `httptest`.
- Typical logs before timeout: `timeout waiting for SSE GET` or `httptest.Server blocked in Close after 5 seconds`.

### Root cause

The test `TestIntegrationHTTP_EventStreamMemoryStored` opens a **GET SSE** on `httptest.Server` wrapping the Fiber app via `adaptor.FiberApp`. With the **race detector** (and often without it), the stream connection can:

1. not complete headers within 30s (`startGate` timeout in the test);
2. leave `io.Copy` on the response body **blocked** after `cancel`;
3. cause `httptest.Server.Close` to **block** until package timeout.

On **GitHub Actions** the test is already `Skip` when `GITHUB_ACTIONS=true`. Locally the problem is the same.

### Real SSE coverage

No functional coverage is lost:

- **`scripts/ci_integration_smoke.sh`** — `curl` on `GET /v1/events` + `memory.stored` with a real HTTP server.
- CI job **`integration-smoke`** (after `go build` of API/worker on port 8000).

---

## Environment variables

| Variable | Default in `newIntegrationHTTPApp` | Effect |
|----------|------------------------------------|--------|
| `PCMI_SKIP_SSE_HTTPTEST` | `1` (unless you force SSE) | Skips `TestIntegrationHTTP_EventStreamMemoryStored` |
| `PCMI_FORCE_SSE_HTTPTEST` | — | If `1`, does **not** set the automatic skip; runs the SSE httptest (can take up to timeout) |
| `DATABASE_URL` | — | Required for handler/service/repository integration tests |
| `GITHUB_ACTIONS` | — | On GHA runner the SSE httptest is always skipped |

### Force the SSE httptest (debug)

```bash
PCMI_FORCE_SSE_HTTPTEST=1 PCMI_SKIP_SSE_HTTPTEST=0 \
  go test -tags=integration -count=1 -timeout 15m -v ./internal/handler/... \
  -run TestIntegrationHTTP_EventStreamMemoryStored
```

Use an explicit **timeout** (`-timeout 15m`) and expect possible failures or long waits.

---

## Other slow tests

- **`go test -race -tags=integration ./internal/...`** — can stay **silent** between packages for many minutes; normal on laptops.
- Mitigations: `PCMI_GO_TEST_P=1`, `CI_LIKE_NO_RACE=1` (local-only script `ci_like_github.sh`), `CI_LIKE_GO_VERBOSE=1`, `CI_LIKE_HEARTBEAT_SECS=120`.

See also [local-ci.md](local-ci.md) and `scripts/ci_like_github.sh --help`.

---

## Code references

- SSE test: `internal/handler/integration_http_e2e_test.go` — `TestIntegrationHTTP_EventStreamMemoryStored`
- HTTP integration helper: `newIntegrationHTTPApp` in the same file
- CI smoke: `scripts/ci_integration_smoke.sh` (section "SSE memory.stored")
