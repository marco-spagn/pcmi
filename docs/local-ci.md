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
| Run **everything** (`act` + host `trivy` + host smoke, ≈ 20–30 min) | `make act-all` |
| Run just the linter | `make act-lint` |
| Run unit tests + race detector | `make act-test` |
| Run `govulncheck` supply-chain scan | `make act-vuln` |
| Run Trivy image scan (API + worker) | `make act-trivy` |
| Run integration smoke (compose + **host** bins, same scripts as CI) | `make act-integration-smoke` |
| Run a job by name | `make act-job JOB=integration-smoke` |

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

**Trivy hits Docker Hub rate limits**
Pass an auth token: `act -j trivy-images -s DOCKER_TOKEN=$DOCKER_TOKEN`.
