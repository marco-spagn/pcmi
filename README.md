# PCMI — Persistent Cognitive Memory Infrastructure

[![CI](https://github.com/marco-spagn/pcmi/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marco-spagn/pcmi/actions/workflows/ci.yml)
[![CodeQL](https://github.com/marco-spagn/pcmi/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/marco-spagn/pcmi/actions/workflows/codeql.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/marco-spagn/pcmi/main/badges/coverage.json)](badges/coverage.json)
[![Release](https://img.shields.io/github/v/release/marco-spagn/pcmi)](https://github.com/marco-spagn/pcmi/releases/latest)
[![Container](https://img.shields.io/badge/ghcr.io-pcmi-blue?logo=docker)](https://ghcr.io/marco-spagn/pcmi)
[![Go](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**Durable, multi-tenant memory for AI agents.** Persistent storage outside the agent runtime, with hybrid retrieval, background workers, and enterprise controls.

Agents are ephemeral. Organizational memory should not be.

---

## What PCMI does

**Core memory.** Hierarchical `ltree` paths, append-only versioning, tags, TTL, optional field encryption. Content-hash deduplication at ingest.

**Hybrid retrieval.** BM25 + semantic + importance scoring + temporal decay. `as_of` point-in-time reads. Keyset cursor pagination.

**Agent sessions.** Working memory scoped to a session, promote to long-term. See [docs/SESSIONS.md](docs/SESSIONS.md).

**Background workers.** Embedding generation (with circuit breaker), LLM distillation, consolidation, pruning, expiry, contradiction detection.

**Event system.** Redis Streams (default) or pub/sub. SSE streaming and gRPC streams for real-time consumers. Webhooks with HMAC signing.

**Enterprise controls.** Multi-tenant with PostgreSQL Row-Level Security. API-key RBAC with rotation and lifecycle management. Rate limiting per key. Audit logging. Prometheus metrics and OpenTelemetry tracing.

**Graph (experimental).** Memory links with typed edges (causal, temporal, contradicts, supports, related). Apache AGE-powered Cypher traversal, shortest-path chain reconstruction, and interactive graph visualization UI at `/v1/graph/ui`. See [docs/cognitive-graph.md](docs/cognitive-graph.md).

---

## Quick examples

### Store and retrieve (curl)

```bash
export PCMI_BASE_URL=http://localhost:8000
export PCMI_API_KEY=testkey123

# Store a memory
curl -s -X POST "$PCMI_BASE_URL/v1/memories" \
  -H "Content-Type: application/json" -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path":"root.demo.note","content":"Hello PCMI","tags":["demo"]}'

# Retrieve under a path prefix
curl -s -X POST "$PCMI_BASE_URL/v1/retrieve" \
  -H "Content-Type: application/json" -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path_prefix":"root.demo","query":"","limit":10}'
```

### Python SDK

```bash
pip install -e sdk/python
```

```python
from pcmi import PCMIClient
import asyncio

async def main():
    async with PCMIClient("http://localhost:8000", "testkey123") as client:
        await client.store("root.agent.task", "completed step X", tags=["task"])
        result = await client.retrieve("root.agent", limit=5)
        print(result["total"])

asyncio.run(main())
```

### Real-time events (SSE)

```bash
curl -sN "$PCMI_BASE_URL/v1/events" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -H "Accept: text/event-stream"
```

Full guide: **[docs/USAGE.md](docs/USAGE.md)** · SDK reference: **[sdk/README.md](sdk/README.md)**

---

## Architecture

```mermaid
sequenceDiagram
  autonumber
  participant A as Agent
  participant API as pcmi-api
  participant PG as PostgreSQL
  participant R as Redis
  participant W as pcmi-worker
  A->>API: POST /v1/memories
  API->>PG: INSERT version N
  API->>R: publish event
  API-->>A: 200 id, version
  R->>W: memory.stored
  W->>PG: UPDATE embedding (async)
  W->>R: knowledge.distilled (optional)
```

Deeper design: **[docs/architecture.md](docs/architecture.md)** · Data model: **[docs/DATA-MODEL.md](docs/DATA-MODEL.md)** · Workers: **[docs/WORKERS-AND-EVENTS.md](docs/WORKERS-AND-EVENTS.md)**

---

## API surfaces

| Surface | Best for |
|---------|----------|
| **HTTP REST** | OpenAPI tooling, browsers, SSE, Prometheus, admin UI |
| **gRPC** | Agents, batch workloads, streaming retrieve/events |
| **Python SDK** | `pip install -e sdk/python` — async HTTP client |
| **TypeScript SDK** | `npm install` from `sdk/typescript` — typed HTTP client |
| **MCP** | stdio JSON-RPC for Cursor / Claude — `cmd/mcp` |

| Reference | Location |
|-----------|----------|
| OpenAPI 3 spec | [docs/openapi.yaml](docs/openapi.yaml) |
| gRPC protos | [proto/pcmi/v1/](proto/pcmi/v1/) |
| gRPC ↔ HTTP matrix | [docs/grpc-vs-http.md](docs/grpc-vs-http.md) |
| MCP setup | [docs/MCP.md](docs/MCP.md) |

---

## Development

```bash
# Essential
make test                          # unit tests
make lint                          # golangci-lint v2
make test-integration-bufconn      # gRPC integration (in-process)

# Full stack
make infra-up                      # docker compose up
make test-integration-live         # gRPC on :50051 (needs infra-up)
make test-integration              # both bufconn + live

# CI parity
make ci-like-github                # full CI pipeline locally
make test-all-local                # compose + smoke + E2E
make test-all-local-quick          # static/unit + lint only

# Feature-specific smokes (needs infra-up)
make smoke-importance              # retrieval ranking
make smoke-sessions                # agent sessions
make smoke-dedup                   # ingest dedup

# SDK
make sdk-smoke                     # Python + TypeScript

# Synthetic data
make synth-list                    # list presets
make synth-generate PRESET=finance SYNTH_NUM=500

# Distillation E2E (needs OPENAI_API_KEY)
make distillation-e2e
```

### Quickstart

```bash
git clone https://github.com/marco-spagn/pcmi.git && cd pcmi
bash scripts/quickstart.sh
```

The script starts a full Docker Compose stack, stores sample memories, runs retrieval, triggers distillation, and prints a summary — all in under 3 minutes.

**Requirements:** [Docker](https://docs.docker.com/get-docker/) and Docker Compose. For Go development: **Go 1.25+**.  
After the script finishes, the dev API key is **`testkey123`** (admin role). Config reference: [`.env.example`](.env.example).

| Service | Port | Purpose |
|---------|------|---------|
| HTTP API | `8000` | REST + SSE + admin UI + Prometheus `/metrics` |
| gRPC | `50051` | High-throughput memory and admin operations |
| PostgreSQL | `5432` | Primary store with `pgvector` |
| Redis | `6379` | Event bus (streams) and worker coordination |

### Docker images

Pre-built multi-arch images (`linux/amd64`, `linux/arm64`) on every release and every push to `main`:

```bash
docker pull ghcr.io/marco-spagn/pcmi:latest     # latest release
docker pull ghcr.io/marco-spagn/pcmi:v1.49.0     # specific version
docker pull ghcr.io/marco-spagn/pcmi:main         # tip of main

docker run --rm -p 8000:8000 \
  -e DATABASE_URL="postgres://user:pass@host:5432/db?sslmode=disable" \
  -e REDIS_ADDR="redis:6379" \
  ghcr.io/marco-spagn/pcmi:latest
```

See **[CONTRIBUTING.md](CONTRIBUTING.md)** for setup, conventions, and PR workflow. **[docs/local-ci.md](docs/local-ci.md)** covers reproducing CI locally.

---

## Documentation index

| Topic | Document |
|-------|----------|
| End-to-end usage (HTTP, gRPC, env, paths) | [docs/USAGE.md](docs/USAGE.md) |
| Schema, versioning, RLS | [docs/DATA-MODEL.md](docs/DATA-MODEL.md) |
| Hybrid retrieval pipeline | [docs/retrieval-pipeline.md](docs/retrieval-pipeline.md) |
| Background workers and events | [docs/WORKERS-AND-EVENTS.md](docs/WORKERS-AND-EVENTS.md) |
| Agent sessions and working memory | [docs/SESSIONS.md](docs/SESSIONS.md) |
| Cognitive Graph (AGE + Cypher) | [docs/cognitive-graph.md](docs/cognitive-graph.md) |
| Go package map | [docs/CODEBASE.md](docs/CODEBASE.md) |
| Integration testing | [docs/integration-testing.md](docs/integration-testing.md) |
| MCP server for Cursor / Claude | [docs/MCP.md](docs/MCP.md) |
| Kubernetes / Helm | [deploy/helm/README.md](deploy/helm/README.md) |
| Release history | [CHANGELOG.md](CHANGELOG.md) |
| Full doc index | [docs/INDEX.md](docs/INDEX.md) |

---

## Repository layout

| Path | What |
|------|------|
| [`cmd/api`](cmd/api) | HTTP + gRPC server entrypoint |
| [`cmd/worker`](cmd/worker) | Background worker entrypoint |
| [`cmd/mcp`](cmd/mcp) | MCP stdio server |
| [`internal/`](internal/) | Domain logic (handler, service, repository, worker, graph) |
| [`migrations/`](migrations/) | SQL migrations |
| [`proto/`](proto/) | Protobuf definitions |
| [`sdk/`](sdk/) | Python and TypeScript clients |
| [`deploy/helm/`](deploy/helm/) | Kubernetes Helm chart |
| [`scripts/`](scripts/) | CI, smoke tests, E2E, coverage |
| [`.github/workflows/`](.github/workflows/) | CI, CodeQL, release pipelines |

---

## Contributing

Issues and pull requests welcome. Read **[CONTRIBUTING.md](CONTRIBUTING.md)** before opening a PR.

---

## Security

Report vulnerabilities privately — do not open public issues. See **[SECURITY.md](SECURITY.md)** for the disclosure process.

---

## License

[Apache License 2.0](LICENSE) — Copyright 2026 Marco Spagnuolo & PCMI Team.
