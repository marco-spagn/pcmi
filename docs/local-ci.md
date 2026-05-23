# Running CI locally — without GitHub minutes

When you've burned through your GitHub Actions credits (or just want to iterate
faster), you can run **the exact same workflow jobs** locally with
[`nektos/act`](https://github.com/nektos/act). `act` parses `.github/workflows/`
and replays each job inside a Docker container that mimics
`ubuntu-latest`.

---

## One-time setup

```bash
brew install act              # macOS
# or:
# scoop install act-cli      # Windows
# https://github.com/nektos/act#installation for other platforms
```

You need a working Docker daemon (Docker Desktop, OrbStack, colima…).

The repository ships `.actrc` with sensible defaults:
- medium-size runner image (~1 GB on first pull)
- `--reuse` to cache containers across runs
- `--container-architecture linux/amd64` (works on both Intel and Apple Silicon)

No further configuration needed.

---

## Day-to-day commands

| Goal | Command |
|---|---|
| **All tests / full CI on host** (auto-frees `:5432` / `:6379`; alias `make test-all`) | `make ci-like-github` or `./scripts/ci_like_github.sh` |
| Broad local suite (compose + HTTP/gRPC smoke; auto-frees ports) | `make test-all-local` |
| See what jobs exist | `make act-list` |
| Free `:5432` / `:6379` manually (compose down + stop act service containers) | `make free-dev-ports` or `make act-preflight` |
| Run **everything** (`act` + host `trivy` + host smoke, ≈ 20–30 min) | `make act-all` |
| Run just the linter | `make act-lint` |
| Run unit tests + race detector | `make act-test` |
| Run `govulncheck` supply-chain scan | `make act-vuln` |
| Run Trivy image scan (API + worker) | `make act-trivy` |
| Run integration smoke (compose + **host** bins, same scripts as CI) | `make act-integration-smoke` |
| Run a job by name | `make act-job JOB=integration-smoke` |
| Generate coverage profile only | `make test-cover` |
| Enforce coverage thresholds (after `test-cover`) | `make cover-check` |
| Print per-function coverage report | `make cover-report` |
| gRPC live integration (API on **:50051**) | `make test-integration-live` (after `make infra-up`) |
| gRPC in-process (Postgres only, no **:50051**) | `make test-integration-bufconn` |
| Redis Streams bus (`internal/event`) | `make test-streams-integration` (miniredis in-process; no Docker) |

Behind the scenes:

```bash
# These are exactly what the Makefile runs.
act --list              # list jobs
act push                # run the full push-triggered pipeline
act -j golangci-lint    # run a single job
act -j go               # run "Go build, vet & test"
act -j security         # run govulncheck
act -j trivy-images     # run trivy on both Docker images
```

---

## What works locally vs only on GitHub

| Job | Runs in `act` | Notes |
|---|---|---|
| `golangci-lint` | ✅ | Identical to GitHub run |
| `go` (build, vet, race test, coverage) | ✅ | Identical |
| `security` (govulncheck) | ✅ | Identical |
| `trivy-images` | ⚠️ stub in `act` + `make act-trivy` | `aquasecurity/trivy-action` + its `setup-trivy` pre-steps are not fully emulated; the workflow steps are skipped when `ACT=true`, then `make act-trivy` runs `aquasec/trivy:latest` with the same severity / exit-code policy. |
| `integration-smoke` | ⚠️ stub in `act` + `make act-integration-smoke` | `act` runs the job container on `host` network while service containers stay on a bridge — background API + health checks flake. When `ACT=true`, only a notice step runs; `make act-integration-smoke` runs `scripts/act_integration_smoke_host.sh` (compose + local `go build` + same bash/Go/SDK scripts as CI). On **GitHub-hosted** runners nothing changes (`ACT` is unset). |
| `integration-e2e` (OpenAI) | ⚠️ | Needs `OPENAI_API_KEY`. Pass with `act -j integration-e2e -s OPENAI_API_KEY=$OPENAI_API_KEY` |

---

## Host integration tests (gRPC live, streams)

These targets are **not** run by `make act-test` or the `go` job’s default `go test ./internal/...` alone. Use them when you change gRPC handlers, streaming, or Redis Streams wiring.

### Ports

| Port | Service | Needed for |
|------|---------|------------|
| `5432` | PostgreSQL | `test-integration-bufconn`, `test-integration-live`, `test-integration-handler`, CI `go` job |
| `6379` | Redis | Full stack (`infra-up`, `act-integration-smoke`); **not** required for bufconn-only gRPC tests |
| `50051` | gRPC (`pcmi-api`) | `make test-integration-live`, `integration-smoke` gRPC step on GitHub |
| `8000` | HTTP REST | `make sdk-smoke`, `make infra-smoke`, `scripts/ci_integration_smoke.sh` |

### `make test-integration-live` (TCP gRPC)

Dials a **real** API on `GRPC_HOST` (default `localhost:50051`). The Makefile sets `GRPC_TEST_API_KEY=testkey123` (dev seed from migrations), so tests **do not skip** — if nothing listens on `:50051`, the run fails at dial/RPC time.

Recommended sequence:

```bash
make infra-up          # postgres :5432, redis :6379, api :8000/:50051, worker
# infra-up already waits on GET /v1/ready; optional:
make infra-wait        # or: curl -sf http://localhost:8000/v1/ready

make test-integration-live
```

Alternatives:

- **Deps only + API on host:** `make infra-deps-up` then `go run ./cmd/api` in another terminal, then `make test-integration-live`.
- **In-process only (no :50051):** `make test-integration-bufconn` — same package, bufconn server + miniredis; needs migrated Postgres only.
- **Both:** `make test-integration` (bufconn, then live).

### `make test-streams-integration`

Runs `go test -tags=integration -run TestStream ./internal/event/...` with **miniredis** in-process. No Postgres, Redis, or `:50051` required. Optional before PRs that touch `internal/event` Streams backend; not part of `act-integration-smoke` today.

### What GitHub enforces (with `CI_start`)

| Check | Live gRPC on `:50051` |
|-------|------------------------|
| Job `go` | **No** — `GRPC_TEST_API_KEY` unset; live tests in `internal/grpc` are **skipped** |
| Job `integration-smoke` | **Yes** — builds API on host, then `go test -tags=integration ./internal/grpc/...` with `GRPC_TEST_API_KEY=testkey123` |
| Local `make act-lint && make act-test` | **No** — same as `go` job (Postgres service only) |
| Local `make act-integration-smoke` | **Yes** — same scripts as `integration-smoke` |

Full pipeline on GitHub still requires **`CI_start`** in the commit message (or `workflow_dispatch`). See [CONTRIBUTING.md](../CONTRIBUTING.md).

More detail: [integration-testing.md](integration-testing.md), [USAGE.md](USAGE.md).

---

## Skipping the slow steps

If you only want to verify your code change passes lint + unit tests before
pushing:

```bash
make act-lint && make act-test
```

This is sub-3 minutes on a warm cache and catches 90% of CI failures without
spending any GitHub minutes.

---

## Pre-commit hook (optional)

To block commits that would fail CI:

```bash
cat > .git/hooks/pre-push <<'EOF'
#!/usr/bin/env bash
set -e
make act-lint
make act-test
EOF
chmod +x .git/hooks/pre-push
```

---

## Troubleshooting

**`exec format error` on Apple Silicon**
Edit `.actrc` and remove the `--container-architecture linux/amd64` line, then
let `act` use the native arm64 image.

**Slow first run**
The runner image is ~1 GB. After the first download `--reuse` keeps the
container around so subsequent runs start in < 5 seconds.

**Want to skip the cache and rebuild from scratch**
```bash
act -j go --rebuild
```

**`go test -tags=integration ./internal/handler/...` hangs ~10 minutes then FAIL**

The package times out on `TestIntegrationHTTP_EventStreamMemoryStored` (SSE over
`httptest` + Fiber `adaptor`). Use:

```bash
PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -count=1 ./internal/handler/...
```

`make ci-like-github` sets this in Phase F; `newIntegrationHTTPApp` sets it by
default unless `PCMI_FORCE_SSE_HTTPTEST=1`. Full write-up:
[integration-testing.md](integration-testing.md).

**`Bind for 0.0.0.0:5432 failed: port is already allocated`**
`act` always starts the `integration-smoke` service containers (Postgres/Redis)
on the host **before** any step runs, even when the job is a no-op under `ACT`.
The usual fix is automatic: `make ci-like-github`, `make act-all`, `make act-test`,
`make infra-up`, and `make test-all-local` all run `scripts/free_dev_ports.sh`
first (compose down + stop any Docker container publishing `:5432` / `:6379`,
including leftover `act-*` service containers). Manual cleanup: `make free-dev-ports`.
To keep Compose running, use `SKIP_ACT_PORT_CLEANUP=1` — then you must free
those ports yourself before jobs that bind them.

---

## Coverage gate

CI now blocks merges that regress test coverage. The gate runs as the last step
of the `go` job in `.github/workflows/ci.yml`:

```yaml
- name: Enforce coverage thresholds
  env:
    COVERAGE_MIN_TOTAL: "22"
    COVERAGE_PKG_FLOORS: "config:70,event:70,eventschema:85,metrics:70"
  run: scripts/ci_coverage_check.sh
```

Run the same check locally before pushing:

```bash
make test-cover     # writes coverage.out (race + atomic mode)
make cover-check    # exits non-zero if any floor is missed
make cover-report   # human-friendly per-function summary
```

`scripts/ci_coverage_check.sh` parses `coverage.out` itself — no `go tool cover`
dependency — and supports the same `COVERAGE_MIN_TOTAL` / `COVERAGE_PKG_FLOORS`
overrides as CI. When a PR lifts a package's coverage materially, **bump the
floor in the same PR** so future regressions are caught.
