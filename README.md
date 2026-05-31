# PCMI — Persistent Cognitive Memory Infrastructure

[![CI](https://github.com/marco-spagn/pcmi/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marco-spagn/pcmi/actions/workflows/ci.yml)
[![CodeQL](https://github.com/marco-spagn/pcmi/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/marco-spagn/pcmi/actions/workflows/codeql.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/marco-spagn/pcmi/main/badges/coverage.json)](badges/coverage.json)
[![Release](https://img.shields.io/github/v/release/marco-spagn/pcmi)](https://github.com/marco-spagn/pcmi/releases/latest)
[![Container](https://img.shields.io/badge/ghcr.io-pcmi-blue?logo=docker)](https://ghcr.io/marco-spagn/pcmi)
[![Go](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![API](https://img.shields.io/badge/API-v1.49.0-22c55e)](internal/version/version.go)

**Durable, multi-tenant memory for AI agents** — outside the agent runtime, with HTTP and gRPC APIs, hybrid retrieval, background workers, and enterprise controls (RLS, RBAC, audit, observability).

Agents are ephemeral. Organizational memory should not be.

## Cognitive Graph Explorer *(experimental)*

Explore SOC incidents and linked memories as a **typed property graph** on [Apache AGE](https://github.com/apache/age) — multi-hop traversal, shortest causal chains, and a browser UI at **`/v1/graph/ui`**.

**Demo (~90s)** — kill-chain expansion, Tree/Radial layouts, memory inspector, Find Chain, edge semantics, clusters, timeline.

> GitHub does not render `<video>` in README files. **Click the preview below** to open the full MP4 in GitHub’s built-in player (play / pause / seek / fullscreen).

<p align="center">
  <a href="https://github.com/marco-spagn/pcmi/blob/feat/pcmi-cognitive-graph-v3-spike/docs/assets/graph-ui-demo.mp4">
    <img
      src="docs/assets/graph-ui-demo.gif"
      alt="Animated preview — click to play the full 90 second Cognitive Graph Explorer walkthrough with controls"
      width="100%"
    />
  </a>
</p>

<p align="center">
  <a href="https://github.com/marco-spagn/pcmi/blob/feat/pcmi-cognitive-graph-v3-spike/docs/assets/graph-ui-demo.mp4">
    <img src="https://img.shields.io/badge/▶_Play_full_video_(90s,_with_controls)-2dd4bf?style=for-the-badge&labelColor=0a0f1e" alt="Play full demo video on GitHub" />
  </a>
  &nbsp;
  <a href="https://github.com/marco-spagn/pcmi/releases/download/graph-ui-demo/graph-ui-demo.mp4">
    <img src="https://img.shields.io/badge/Download-MP4-38bdf8?style=for-the-badge&labelColor=0a0f1e" alt="Download MP4" />
  </a>
</p>

**From a git clone:** open `docs/assets/graph-ui-demo.mp4` in any browser or video player.

<p align="center">
  <a href="http://localhost:8000/v1/graph/ui"><code>/v1/graph/ui</code></a> (local)
  &nbsp;·&nbsp;
  <a href="docs/cognitive-graph.md#graph-ui--demo-video">Graph UI guide</a>
  &nbsp;·&nbsp;
  <code>make graph-ui</code> to load the SOC dataset
</p>

---

## Table of contents

- [Cognitive Graph Explorer](#cognitive-graph-explorer-experimental)
- [Why PCMI](#why-pcmi)
- [Features](#features)
- [Quickstart (2 minutes)](#quickstart-2-minutes)
  - [Docker](#docker)
- [Usage examples](#usage-examples)
- [Cognitive Graph — details](#cognitive-graph--details)
- [Architecture](#architecture)
- [APIs and clients](#apis-and-clients)
- [Documentation](#documentation)
- [Repository layout](#repository-layout)
- [Development](#development)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

---

## Why PCMI

Production agents are replaced, upgraded, and sharded across teams. Without a shared memory layer:

- Knowledge stays trapped in prompts, vector indexes, or vendor-specific chat history.
- Deployments and model swaps force expensive re-ingestion.
- Auditors cannot answer *what the system knew at decision time*.

PCMI centralizes **versioned, path-scoped memories** in PostgreSQL, with optional embeddings, distillation, events, and webhooks — consumable from any agent framework or LLM provider.

```mermaid
flowchart LR
  subgraph clients [Clients]
    Agent[Agents / Apps]
  end
  subgraph pcmi [PCMI]
    API[HTTP API :8000]
    GRPC[gRPC :50051]
    W[Worker]
  end
  DB[(PostgreSQL + pgvector)]
  Redis[(Redis)]
  Agent --> API
  Agent --> GRPC
  API --> DB
  API --> Redis
  GRPC --> DB
  W --> DB
  W --> Redis
```

---

## Features

| Area | Capabilities |
|------|----------------|
| **Memory** | Hierarchical `ltree` paths, append-only versioning, tags, TTL, optional field encryption |
| **Retrieve** | Hybrid ranking: BM25 + semantic + **importance** + **temporal decay**; `as_of` reads; keyset cursors |
| **Sessions** | Agent sessions + working memory; promote to long-term (`/v1/sessions/*`) — [docs/SESSIONS.md](docs/SESSIONS.md) |
| **Dedup** | Content-hash dedup at ingest (`none` / `skip` / `link` / `merge`) — env, tenant, or `X-Dedup-Mode` |
| **Workers** | Embedding (circuit breaker), distillation, consolidation, pruning, compaction, expiry |
| **Events** | Redis **Streams** by default (`EVENT_BACKEND=streams`); legacy pub/sub; SSE + gRPC streams |
| **Integration** | Webhooks with **HMAC** (`timestamp.body`), idempotent store (`X-Idempotency-Key`), MCP stdio server |
| **Graph** *(experimental)* | Typed `memory_links` on **Apache AGE** — `/v1/graph/related`, `/chain`, `/cypher`; browser UI at [`/v1/graph/ui`](docs/cognitive-graph.md#graph-ui--demo-video) |
| **Security** | API-key RBAC + **rotation/lifecycle** (admin), PostgreSQL RLS, optional metrics Bearer token |
| **Rate limit** | Per-key limits; **`RATE_LIMIT_BACKEND=redis`** for multi-instance API |
| **Ops** | Prometheus metrics, OpenTelemetry, Helm chart, health/readiness probes |
| **Admin** | Tenant/API-key CRUD + rotate/revoke (HTTP + gRPC), embedded UI at `GET /v1/admin/ui` |
| **Lists** | Keyset pagination (`limit`, `cursor`, `after_id` where supported) on audit, history, distilled, webhooks, distillation, admin — see [docs/USAGE.md](docs/USAGE.md#cursor-based-pagination-on-list-endpoints-pcmi-014) |

Current API version: see `version` on [`GET /v1/health`](docs/openapi.yaml) (source of truth: [`internal/version/version.go`](internal/version/version.go)).

---



| Service | Port | Purpose |
|---------|------|---------|
| HTTP API | `8000` | REST + SSE + admin UI |
| gRPC | `50051` | High-throughput memory + ops |
| PostgreSQL | `5432` | Primary store |
| Redis | `6379` | Events + worker coordination |

### Docker

Pre-built multi-arch images (`linux/amd64`, `linux/arm64`) are published to the GitHub Container Registry on every release and on every push to `main`:

```bash
# Latest stable release
docker pull ghcr.io/marco-spagn/pcmi:latest

# Specific version
docker pull ghcr.io/marco-spagn/pcmi:v1.49.0

# Tip of main (continuous delivery)
docker pull ghcr.io/marco-spagn/pcmi:main

# Run with your own postgres + redis
docker run --rm -p 8000:8000 \
  -e DATABASE_URL="postgres://user:pass@host:5432/db?sslmode=disable" \
  -e REDIS_ADDR="redis:6379" \
  ghcr.io/marco-spagn/pcmi:latest
```

---

## Usage examples

### HTTP (curl)

```bash
export PCMI_BASE_URL=http://localhost:8000
export PCMI_API_KEY=testkey123

# Store
curl -s -X POST "$PCMI_BASE_URL/v1/memories" \
  -H "Content-Type: application/json" -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path":"root.demo.note","content":"Hello PCMI","tags":["demo"],"embedding_model":"unspecified"}'

# Retrieve
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

Full operational guide: **[docs/USAGE.md](docs/USAGE.md)** · SDK reference: **[sdk/README.md](sdk/README.md)**

---

## Cognitive Graph — details

The **demo video is at the top of this README**. Below: how to run the UI locally and what each control maps to on the API.

PCMI models linked memories as a graph: **nodes** = memories, **edges** = typed `memory_links` (`causal`, `temporal`, `contradicts`, `supports`, `related`). With AGE enabled, `GET /v1/graph/related` and `GET /v1/graph/chain` power the explorer.

### Try it locally

| Step | Command / URL |
|------|----------------|
| 1. Start stack + load SOC dataset | `make graph-ui` (or `bash scripts/e2e/launch_graph_ui.sh`) |
| 2. Open the UI | [http://localhost:8000/v1/graph/ui](http://localhost:8000/v1/graph/ui) |
| 3. API key | Default dev key: `testkey123` (see `.env.example`) |
| 4. First exploration | Memory ID **14**, depth **5**, link types **causal + temporal** — Conti kill chain |

The green **AGE ready** badge means `GET /v1/graph/health` reports `available: true`. If it is yellow, point the API at `postgres-age` (port **5433**) — see [docs/cognitive-graph.md](docs/cognitive-graph.md#how-to-enable).

### What the UI does

| Control | Backed by | Purpose |
|---------|-----------|---------|
| **Explore** | `GET /v1/graph/related` | Expand N hops from a root memory; edge labels show `link_type`, node colors show hop depth |
| **Find Chain** | `GET /v1/graph/chain` | Shortest causal path between root (Memory ID field) and a selected node — highlighted in gold |
| **Force / Tree / Radial** | vis-network layouts | Force = natural clusters; Tree = stage-by-stage; Radial = root-centered briefing |
| **Inspector** | `GET /v1/graph/memories` cache | Path, alert content, tags, severity / MITRE metadata on node click |
| **Clusters** | Path-prefix grouping | Collapse alert storms (many duplicates on the same subnet) |
| **Timeline** | Memory timestamps | Time axis; click a dot to focus the node in the graph |

**Suggested memory IDs** (after the SOC dataset load), curl examples, and edge-type semantics: **[docs/cognitive-graph.md § Graph UI](docs/cognitive-graph.md#graph-ui--demo-video)**.

**Regenerate the demo video** (requires Chrome + ffmpeg):

```bash
docker compose --profile graph -f docker-compose.yml -f docker-compose.record-graph.yml up -d api
cd scripts/e2e && npm install playwright@1.49.1 && node record_graph_ui_demo.mjs
# → docs/assets/graph-ui-demo.mp4
```

> **Status:** experimental v2.0 spike — API and schema may change. See [docs/cognitive-graph.md](docs/cognitive-graph.md) and [docs/roadmap.md](docs/roadmap.md).

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

Deeper design: **[docs/architecture.md](docs/architecture.md)** · Data model: **[docs/DATA-MODEL.md](docs/DATA-MODEL.md)** · Workers & events: **[docs/WORKERS-AND-EVENTS.md](docs/WORKERS-AND-EVENTS.md)**

---

## APIs and clients

| Surface | When to use |
|---------|-------------|
| **HTTP REST** | OpenAPI tooling, browsers, SSE, Prometheus scrape at `GET /metrics`, admin UI |
| **gRPC** | Agents, batch workloads, streaming retrieve/events; `MemoryService`, `AdminService`, `MetricsService` |
| **SDKs** | Python & TypeScript thin HTTP clients — see [sdk/HTTP-API.md](sdk/HTTP-API.md) |
| **MCP** | stdio server for Cursor / Claude — see [docs/MCP.md](docs/MCP.md) |

| Resource | Location |
|----------|----------|
| OpenAPI 3 | [docs/openapi.yaml](docs/openapi.yaml) |
| MCP server | [docs/MCP.md](docs/MCP.md) |
| gRPC protos | [proto/pcmi/v1/](proto/pcmi/v1/) |
| gRPC ↔ HTTP matrix | [docs/grpc-vs-http.md](docs/grpc-vs-http.md) |

**Note:** Official SDKs speak HTTP only. Use gRPC stubs for maximum throughput or streaming.

---

## Documentation

**Full index:** [docs/INDEX.md](docs/INDEX.md)

| Document | Description |
|----------|-------------|
| [docs/USAGE.md](docs/USAGE.md) | End-to-end usage (HTTP, gRPC, env, paths) |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | Schema, versioning, RLS |
| [docs/retrieval-pipeline.md](docs/retrieval-pipeline.md) | Hybrid retrieve pipeline |
| [docs/WORKERS-AND-EVENTS.md](docs/WORKERS-AND-EVENTS.md) | Background jobs, Redis, webhooks |
| [docs/CODEBASE.md](docs/CODEBASE.md) | Go package map for contributors |
| [docs/integration-testing.md](docs/integration-testing.md) | Integration test tags and SSE notes |
| [docs/local-ci.md](docs/local-ci.md) | Reproduce CI locally |
| [docs/distillation-tests.md](docs/distillation-tests.md) | Distillation E2E harness |
| [docs/SESSIONS.md](docs/SESSIONS.md) | Agent sessions and working memory |
| [docs/cognitive-graph.md](docs/cognitive-graph.md) | Cognitive Graph (AGE), SOC dataset, Graph UI + demo video |
| [docs/MCP.md](docs/MCP.md) | MCP stdio server for Cursor / Claude |
| [deploy/helm/README.md](deploy/helm/README.md) | Kubernetes / Helm deployment |
| [CHANGELOG.md](CHANGELOG.md) | Release history |

Optional technical report (PDF build): [docs/papers/](docs/papers/).

---

## Repository layout

| Path | Description |
|------|-------------|
| [`cmd/api`](cmd/api) | HTTP + gRPC server, `/metrics`, admin UI |
| [`cmd/mcp`](cmd/mcp) | MCP stdio server for AI agents (`pcmi-mcp`) |
| [`cmd/worker`](cmd/worker) | Embedding, distillation, pruning, expiry |
| [`internal/`](internal/) | Domain logic (handler, service, repository, worker, grpc) |
| [`proto/`](proto/) | Protobuf definitions |
| [`migrations/`](migrations/) | SQL schema (`001`–`012`) |
| [`sdk/`](sdk/) | Python & TypeScript HTTP clients |
| [`examples/`](examples/) | Celery, Temporal, LangChain, LlamaIndex, AutoGen, CrewAI samples |
| [`deploy/helm/`](deploy/helm/) | Primary Kubernetes packaging |
| [`deploy/k8s/`](deploy/k8s/) | Static manifests (non-Helm) |
| [`k8s/`](k8s/) | **Deprecated** — use `deploy/helm/` |
| [`scripts/`](scripts/) | CI smoke, distillation E2E, coverage |
| [`.github/workflows/`](.github/workflows/) | CI, CodeQL |

Container images: `Dockerfile.api`, `Dockerfile.worker` (root `Dockerfile` is legacy).

---

## Development

```bash
# Unit tests
make test

# Lint (golangci-lint v2)
make lint

# gRPC integration: in-process (bufconn) or live TCP on :50051
make test-integration-bufconn   # Postgres only
make infra-up && make test-integration-live   # full stack + dial :50051
make test-integration           # both

# SDK smoke (Python + TypeScript)
make sdk-smoke

# Full CI parity on host (~15–45+ min; auto-frees :5432 / :6379)
make ci-like-github
# alias: make test-all

# Broad local suite (compose + smoke; also auto-frees ports)
make test-all-local
# Faster: make test-all-local-quick

# Synthetic data (JSONL only, any preset)
make synth-list
make synth-generate PRESET=finance SYNTH_NUM=500 SYNTH_SEED=42

# Distillation end-to-end (requires OPENAI_API_KEY in .env)
make distillation-e2e
make distillation-e2e PRESET=advertising SYNTH_NUM=200 SYNTH_SEED=1

# Feature smokes (API on :8000 after make infra-up)
make smoke-importance   # PCMI-009 retrieve ranking
make smoke-sessions     # PCMI-010 sessions curl E2E
make smoke-dedup        # PCMI-011 ingest dedup

# Full local validation: CI parity + optional OpenAI E2E + smokes + MCP
make test-full-real

# Dev ops
make admin-list-keys    # list tenants/keys from Postgres (hash prefix only)
make free-dev-ports     # free :5432 / :6379 before compose or act
```

| Target | What it validates |
|--------|-------------------|
| `make test-streams-integration` | Redis Streams bus (`EVENT_BACKEND=streams`) |
| `make test-circuit-breaker` | Embedding circuit breaker + worker fast-fail |
| `make test-ratelimit-integration` | `RATE_LIMIT_BACKEND=redis` (miniredis) |
| `make test-idempotency` | `X-Idempotency-Key` middleware + repository |
| `make test-key-lifecycle` | Admin rotate/revoke API keys |
| `make test-retrieval-scoring` | Importance + temporal decay SQL |
| `make test-sessions-integration` | Sessions handler (Postgres + migration 016) |
| `make test-dedup` | Content-hash dedup at ingest |
| `make test-integration-live` | gRPC TCP on `:50051` (after `make infra-up`) |
| `make test-mcp-unit` / `make test-mcp-smoke` | MCP stdio server |

**CI on GitHub:** workflows run on every push/PR (or via `gh workflow run CI`). The `go` job runs integration tests against Postgres only (live gRPC skipped); **`integration-smoke`** starts the API and runs gRPC on `:50051`. See [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/local-ci.md](docs/local-ci.md).

**Coverage:** the badge reads [`badges/coverage.json`](badges/coverage.json) on `main` (CI commits it on `main` when coverage changes; see [docs/github-branch-protection.md](docs/github-branch-protection.md) for ruleset bypass). CI enforces a minimum total in [`.github/workflows/ci.yml`](.github/workflows/ci.yml) (`COVERAGE_MIN_TOTAL`, currently **39%**). Local `make cover-check` defaults to a lower threshold for fast iteration.


## Quickstart (2 minutes)

```bash
git clone https://github.com/marco-spagn/pcmi.git && cd pcmi
bash scripts/quickstart.sh
```

The script checks dependencies, starts the full Docker Compose stack, stores sample memories across three scenarios (SOC alerts, trading signals, DevOps incidents), runs a hybrid retrieval query, triggers distillation, and prints a summary — all in under 3 minutes.

<!-- TODO: record and add docs/quickstart-demo.gif once the script is stable -->
![PCMI Quickstart](docs/quickstart-demo.gif)

**Requirements:** [Docker](https://docs.docker.com/get-docker/) and Docker Compose. For local Go development: **Go 1.25+**.

After migrations, the dev seed API key is **`testkey123`** (admin role). See [`.env.example`](.env.example) for all configuration options.

---

## Contributing

We welcome issues and pull requests. Please read **[CONTRIBUTING.md](CONTRIBUTING.md)** before opening a PR (setup, tests, versioning, migrations, proto conventions).

---

## Security

Report vulnerabilities **privately** — do not open public issues for security bugs. See **[SECURITY.md](SECURITY.md)** for disclosure process and SLAs.

---

## License

[Apache License 2.0](LICENSE) — Copyright 2026 Marco Spagnuolo & PCMI Team.
