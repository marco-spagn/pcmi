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
| See what jobs exist | `make act-list` |
| Free `:5432` / `:6379` before act (stops project `docker compose`) | `make act-preflight` |
| Run **everything** (`act` + host `trivy` + host smoke, ≈ 20–30 min) | `make act-all` |
| Run just the linter | `make act-lint` |
| Run unit tests + race detector | `make act-test` |
| Run `govulncheck` supply-chain scan | `make act-vuln` |
| Run Trivy image scan (API + worker) | `make act-trivy` |
| Run integration smoke (compose + **host** bins, same scripts as CI) | `make act-integration-smoke` |
| **Full CI parity on host** (lint/vuln/helm opt., `go test -race -tags=integration`, coverage gate, then smoke) | `make ci-like-github` or `./scripts/ci_like_github.sh` |
| Run a job by name | `make act-job JOB=integration-smoke` |
| Generate coverage profile only | `make test-cover` |
| Enforce coverage thresholds (after `test-cover`) | `make cover-check` |
| Print per-function coverage report | `make cover-report` |

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
If you just ran `make act-integration-smoke` or `docker compose up`, stop the
stack first: `docker compose down` (or run `make act-all`, which calls
`make act-preflight` to do that automatically). To keep Compose running, use
`SKIP_ACT_PORT_CLEANUP=1 make act-all` — then you must free `5432`/`6379`
yourself, or avoid running jobs that need those service ports.

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
