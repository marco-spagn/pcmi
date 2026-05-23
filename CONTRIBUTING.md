# Contributing to PCMI

Thank you for helping improve PCMI. This guide explains how to set up a development environment, run tests, and submit changes that match project conventions.

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Ways to contribute](#ways-to-contribute)
- [Development setup](#development-setup)
- [Running tests](#running-tests)
- [Code style](#code-style)
- [Pull requests](#pull-requests)
- [Versioning and releases](#versioning-and-releases)
- [Database migrations](#database-migrations)
- [Protobuf / gRPC](#protobuf--grpc)
- [Documentation](#documentation)

## Code of conduct

Be respectful and constructive. Security issues must **not** be reported in public issues — see [SECURITY.md](SECURITY.md).

## Ways to contribute

- Bug reports with reproduction steps and PCMI version (`GET /v1/health` → `version`)
- Documentation fixes and examples
- Tests that lock in real behavior (not trivial assertions)
- Features discussed in an issue first for large API or schema changes

## Development setup

### Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | **1.25+** | Matches `go.mod` |
| Docker | recent | Full stack via Compose |
| golangci-lint | v2 | `make lint` |
| Optional | `helm`, `kubeconform` | Structural deploy tests |

### Clone and configure

```bash
git clone https://github.com/marco-spagn/pcmi.git
cd pcmi
cp .env.example .env
```

### Run the stack (Docker)

```bash
docker compose up -d --build
curl -s http://localhost:8000/v1/health
```

Dev API key after migrations: **`testkey123`** (admin).

### Run API on the host (optional)

```bash
make infra-deps-up          # Postgres + Redis only
go run ./cmd/api
go run ./cmd/worker         # separate terminal
```

Set `DATABASE_URL` and `REDIS_ADDR` for localhost (see `.env.example`).

## Running tests

| Command | Purpose |
|---------|---------|
| `make test` | Unit tests (`go test ./...`) |
| `make test-race` | Race detector on core packages |
| `make test-cover` / `make cover-check` | Coverage report + local gate |
| `make lint` | golangci-lint |
| `make test-integration-bufconn` | gRPC in-process (Postgres **:5432** only; no **:50051**) |
| `make test-integration-live` | gRPC over TCP — needs API on **:50051** (`make infra-up` or `go run ./cmd/api`); sets `GRPC_TEST_API_KEY=testkey123` by default |
| `make test-integration` | bufconn, then live (both) |
| `make test-streams-integration` | Redis Streams in `internal/event` (miniredis; no Docker) |
| `make test-integration-handler` | Handler integration (`-tags=integration`) |
| `make infra-up` / `make infra-wait` | Compose stack: PG **:5432**, Redis **:6379**, HTTP **:8000**, gRPC **:50051** |
| `make sdk-smoke` | Python + TypeScript HTTP smokes |
| `make deploy-structural-test` | CI YAML, Compose, OpenAPI, Helm, proto sanity |
| `make test-all-local` | Broad local CI parity (see `scripts/test_all_local.sh`) |
| `make test-full-real` | Full host validation: CI + optional OpenAI E2E + importance/sessions/dedup smokes + MCP — [docs/local-ci.md](docs/local-ci.md) |
| `make ci-like-github` | CI parity without feature smokes (alias `make test-all`) |
| `make free-dev-ports` / `make act-preflight` | Free `:5432` / `:6379` before compose or `act` |
| `make admin-list-keys` | List tenants/API keys from Postgres (hash prefix only) |
| `make test-streams-integration` | Redis Streams event bus |
| `make test-circuit-breaker` | Embedding circuit breaker |
| `make test-ratelimit-integration` | Distributed Redis rate limiter |
| `make test-idempotency` | `X-Idempotency-Key` on store |
| `make test-key-lifecycle` | Admin API key rotate/revoke |
| `make test-retrieval-scoring` | Importance + temporal decay |
| `make test-sessions-integration` | Agent sessions handler |
| `make test-dedup` | Content-hash dedup at ingest |
| `make smoke-importance` / `make smoke-sessions` / `make smoke-dedup` | curl E2E (API on `:8000`) |
| `make build-mcp` / `make test-mcp-unit` / `make test-mcp-smoke` | MCP stdio server |
| `make distillation-e2e` | Full distillation scenario (needs `OPENAI_API_KEY`) |

**GitHub CI** runs when the commit message includes **`CI_start`**, or on `workflow_dispatch`:

```bash
git commit -m "fix: your change CI_start"
# or
gh workflow run CI
```

See [docs/local-ci.md](docs/local-ci.md) and [docs/integration-testing.md](docs/integration-testing.md).

**gRPC live locally:** `make infra-up && make test-integration-live`. **GitHub:** live gRPC runs only in the `integration-smoke` job (with `CI_start`); the `go` job skips live tests when `GRPC_TEST_API_KEY` is unset. SSE httptest can be slow — use `PCMI_SKIP_SSE_HTTPTEST=1` if needed.

## Code style

- Follow existing patterns in the package you edit — see [docs/CODEBASE.md](docs/CODEBASE.md).
- **Do not** call `os.Getenv` in production code outside `internal/config` (enforced by `internal/config/getenv_audit_test.go`).
- Register specific HTTP routes before wildcards in `memory_handler.go`.
- Keep handlers thin; business logic belongs in `internal/service` and `internal/repository`.

## Pull requests

1. Fork and branch from `main` (`feature/…` or `fix/…`).
2. Run at least `make test` and `make lint` before pushing.
3. For API or schema changes, update `docs/openapi.yaml`, protos, SDK mapping in `sdk/HTTP-API.md`, and [docs/grpc-vs-http.md](docs/grpc-vs-http.md) when applicable.
4. Add or update tests for behavior you change.
5. Update [CHANGELOG.md](CHANGELOG.md) under `[Unreleased]` (Keep a Changelog format).
6. Describe **why** in the PR body, not only **what**.

## Versioning and releases

The public API version is defined in one place:

```go
// internal/version/version.go
const Tag = "v1.33.0"
const Semver = "1.33.0"
```

When bumping a release, also update:

- `docs/openapi.yaml` → `info.version`
- `docs/INDEX.md` and `docs/roadmap.md` (current version section)
- Helm `deploy/helm/pcmi/Chart.yaml` `appVersion` if shipping a tagged release
- CI smoke env defaults if they pin an exact tag

Semantic versioning applies to the **HTTP/gRPC API contract**, not individual SDK package versions.

## Database migrations

- Add numbered SQL files under `migrations/` (never edit applied migrations in production).
- Mount new files in `docker-compose.yml` and document in [docs/MIGRATIONS.md](docs/MIGRATIONS.md).
- Run `make deploy-structural-test` to verify Compose references exist.

## Protobuf / gRPC

- Sources: `proto/pcmi/v1/*.proto`
- Generated Go: `internal/grpc/pcmiv1/` (regenerate with your project's `protoc` workflow; keep stubs in sync with proto changes).
- Register new services in `internal/grpc/server.go` and add integration tests under `internal/grpc/`.

## Documentation

- User-facing guides: `docs/` (indexed in [docs/INDEX.md](docs/INDEX.md))
- README: high-level entry point in English
- Diagrams: prefer **Mermaid** in Markdown (renders on GitHub); legacy SVG links in INDEX are optional

If you add an endpoint, update OpenAPI, USAGE, grpc-vs-http matrix, and SDK HTTP-API table when SDKs should expose it.

---

Questions? Open a [Discussion](https://github.com/marco-spagn/pcmi/discussions) or issue with the `question` label.
