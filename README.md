# PCMI — Persistent Cognitive Memory Infrastructure

[![CI](https://github.com/marco-spagn/pcmi/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marco-spagn/pcmi/actions/workflows/ci.yml)
[![CodeQL](https://github.com/marco-spagn/pcmi/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/marco-spagn/pcmi/actions/workflows/codeql.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/marco-spagn/pcmi/main/badges/coverage.json)](badges/coverage.json)
[![Release](https://img.shields.io/github/v/release/marco-spagn/pcmi)](https://github.com/marco-spagn/pcmi/releases/latest)
[![Container](https://img.shields.io/badge/ghcr.io-pcmi-blue?logo=docker)](https://ghcr.io/marco-spagn/pcmi)
[![Go](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![API](https://img.shields.io/badge/API-v1.51.0-22c55e)](internal/version/version.go)
[![PyPI](https://img.shields.io/pypi/v/pcmi?logo=python&label=PyPI)](https://pypi.org/project/pcmi/)
[![npm](https://img.shields.io/npm/v/%40marco-spagn%2Fpcmi-sdk?logo=npm&label=npm)](https://www.npmjs.com/package/@marco-spagn/pcmi-sdk)

<br/>

> **Agents are ephemeral. Organizational memory should not be.**

**PCMI** is a **production-grade, multi-tenant memory layer for AI agents** — a standalone service that lives outside your agent runtime. It gives every agent in your org a single source of truth for what was known, when it was known, and why it mattered. Think of it as **PostgreSQL for agent cognition**: append-only, versioned, queryable at any point in time.

---

## Table of Contents

- [The Problem](#the-problem)
- [The Solution](#the-solution)
- [Cognitive Graph](#cognitive-graph)
- [Why Another Memory Layer?](#why-another-memory-layer)
- [Quickstart (5 minutes)](#quickstart-5-minutes)
- [Architecture](#architecture)
- [Benchmarks & Comparison](#benchmarks--comparison)
- [Project Deliverables](#project-deliverables)
- [Integration Examples](#integration-examples)
- [Features](#features)
- [APIs & Clients](#apis--clients)
- [Documentation](#documentation)
- [Repository Layout](#repository-layout)
- [Development](#development)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

---

## The Problem

**AI agents are scaling faster than the infrastructure to support them.**

In production today, agent memory looks like this:

| Approach | What breaks |
|----------|-------------|
| **Stuffed prompts** | Context windows overflow. Knowledge is lost between runs. Costs explode at scale. |
| **Vector DB alone** | Semantic search on embeddings loses structure, time, and provenance. You can't ask *"what did we know on January 12th?"* |
| **Framework-locked memory** | LangChain memory, CrewAI memory, AutoGen memory — each agent has its own silo. Cross-team knowledge is trapped. |
| **Chat history** | Vendor-locked, unstructured, not queryable. Auditors can't trace decisions. |
| **DIY Postgres** | Every team re-invents versioning, multi-tenancy, RBAC, events, distillation. Months of engineering for a commodity need. |

The result: **agents that forget, teams that can't share knowledge, and organizations that can't audit AI decisions.**

> If your database had the memory model of a typical AI agent, you'd fire your DBA.

---

## The Solution

PCMI is a **single, self-hosted memory service** that any agent — regardless of framework, language, or LLM provider — reads from and writes to.

```mermaid
flowchart TB
  subgraph Agents["Any Agent / Framework"]
    direction LR
    A1[LangChain]
    A2[CrewAI]
    A3[AutoGen]
    A4[Custom]
    A5[LlamaIndex]
  end

  subgraph PCMI["PCMI Memory Layer"]
    direction LR
    HTTP[HTTP REST<br/>:8000]
    GRPC[gRPC<br/>:50051]
    MCP[MCP stdio<br/>Cursor / Claude]
  end

  subgraph Store["Storage & Processing"]
    direction LR
    PG[(PostgreSQL<br/>+ pgvector<br/>+ ltree)]
    RD[(Redis<br/>Streams)]
    W[Background<br/>Workers]
  end

  Agents --> HTTP
  Agents --> GRPC
  Agents --> MCP
  HTTP --> PG
  GRPC --> PG
  HTTP --> RD
  GRPC --> RD
  RD --> W
  W --> PG
```

### What PCMI gives you that raw databases don't

| Capability | Why it matters |
|------------|----------------|
| **Append-only versioning** | Every write creates a new version; the old one is closed. Query `as_of` any timestamp. Full lineage. |
| **Hybrid retrieval** | BM25 (keyword) + cosine (semantic) + importance (0–1) + temporal decay — fused in a single SQL query with configurable weights. |
| **`ltree` path hierarchy** | `root.team.project.context` — natural scoping for teams, projects, tenants. No flat namespace collisions. |
| **Sessions + working memory** | Ephemeral context during an agent run. Promote to long-term when it matters. |
| **Automated distillation** | LLM-powered summarization of memory clusters into concise knowledge — configurable policies, worker-driven. |
| **Events everywhere** | Redis Streams, SSE, gRPC streams, webhooks (HMAC-signed). Every mutation is observable. |
| **Enterprise controls** | RLS per tenant, RBAC per API key, key rotation/revocation, column encryption, audit log, rate limiting. |

---

## Cognitive Graph

> **Experimental (v2.0 spike).** Opt-in. The API and schema may change before the full release.

Beyond direct path links, PCMI ships a **graph traversal layer** over `memory_links` powered by [Apache AGE](https://github.com/apache/age) (Cypher on PostgreSQL). Memories become nodes; links become **typed, weighted edges**. You can walk multi-hop chains over memory that evolves over time (*"which memories are causally related to this one within 3 hops?"*), reconstruct the **shortest path** between two memories, and run **read-only Cypher** — all automatically scoped per tenant. It is fully **opt-in**: without AGE the graph endpoints return `501` and the rest of PCMI runs unchanged.

[![Cognitive Graph Explorer](docs/assets/graph-ui-demo.gif)](docs/cognitive-graph.md)

<sub>↑ ~18s silent preview loop — see the full ~90s walkthrough in [docs/cognitive-graph.md](docs/cognitive-graph.md#graph-ui--demo-video).</sub>

```mermaid
mindmap
  root((Cognitive Graph))
    Apache AGE
      Cypher on PostgreSQL
      opt-in / graceful 501
    5 link types
      causal
      temporal
      contradicts
      supports
      related
    Traversal
      multi-hop /related
      shortest path /chain
      keyset pagination
    Cypher passthrough
      MATCH only
      auto tenant scope
    Graph Explorer UI
      vis-network
      force / tree / radial
      inspector + Find Chain
```

**Enable it:**

```bash
# 1. Start the AGE-enabled Postgres (one image bundling pgvector + Apache AGE)
docker compose --profile graph up postgres-age

# 2. Point the API at it and apply the graph migration
export DATABASE_URL=postgres://pcmi:pcmi@localhost:5433/pcmi
psql "$DATABASE_URL" -f migrations/019_cognitive_graph_age.sql

# 3. Health check (unauthenticated probe, like /v1/ready)
curl http://localhost:8000/v1/graph/health
# {"available":true,"extension":"apache-age"}
```

| Endpoint | Description |
|----------|-------------|
| `GET /v1/graph/related` | Multi-hop traversal from a memory (`memory_id`, `depth`, `link_types`, keyset pagination) |
| `GET /v1/graph/chain` | Shortest path between two memories (`from`, `to`, `max_depth`) |
| `POST /v1/graph/cypher` | Read-only Cypher passthrough (`MATCH` only, write keywords rejected, tenant scope injected) |
| `GET /v1/graph/ui` | Browser Graph Explorer (vis-network: force/tree/radial layouts, node inspector, Find Chain) |

Link types: `causal`, `temporal`, `contradicts`, `supports`, `related`. Edges are mirrored into AGE by a PostgreSQL trigger (`trg_memory_links_sync_graph`) on `memory_links`. Full reference, Cypher examples, and the SOC demo dataset: **[docs/cognitive-graph.md](docs/cognitive-graph.md)**.

---

## Why Another Memory Layer?

This is the honest question. Mem0, Zep, LangGraph Checkpointer, and a dozen other projects exist. Why build PCMI?

### The honest answer

Every existing solution made **one of three tradeoffs we couldn't accept:**

1. **Framework lock-in.** LangGraph's `MemorySaver` only works inside LangGraph. Mem0's SDK is Python-only. Zep's open-source version lags years behind their cloud. We needed something that any agent — from a 50-line Python script to a Go microservice to a Cursor MCP tool — could use with the same API.

2. **No audit trail.** Most memory layers store "the latest state" — not *how we got there*. For regulated industries (finance, healthcare, security), you need to answer *"what did the system know at 14:32 UTC on March 3rd?"* PCMI's append-only versioning makes this a query parameter (`as_of`), not a forensic investigation.

3. **Cloud-only or single-tenant.** Mem0 and Zep are primarily cloud services. Self-hosting is an afterthought. PCMI runs on your own Postgres — the same one your app already uses. No data leaves your VPC.

### What PCMI is NOT

| PCMI is NOT | Instead, use |
|-------------|-------------|
| A vector database | Pinecone, Weaviate, Qdrant |
| An agent framework | LangGraph, CrewAI, AutoGen |
| A chat history store | Your LLM provider's built-in solution |
| A knowledge graph | Neo4j, Amazon Neptune |
| A workflow engine | Temporal, Celery, Prefect |

**PCMI is the memory layer that sits BETWEEN your agent and your database.** It handles versioning, retrieval ranking, eventing, and lifecycle management so your agent code doesn't have to.

---

## Quickstart (5 minutes)

```bash
git clone https://github.com/marco-spagn/pcmi.git && cd pcmi
bash scripts/quickstart.sh
```

**What happens:**

```
┌──────────────────────────────────────────────────────────┐
│ Step 1 — Docker Compose spins up:                        │
│   • PostgreSQL 16 + pgvector + ltree                     │
│   • PCMI API (:8000 HTTP + :50051 gRPC)                  │
│   • PCMI Worker (embedding, distillation, pruning)       │
│   • Redis 7 (event streams)                              │
│                                                          │
│ Step 2 — The script stores 6 sample memories:            │
│   • 3 SOC alerts (brute-force, lateral movement, DLP)    │
│   • 2 trading signals (BTC breakout, ETH oversold)       │
│   • 1 DevOps incident (DB CPU spike)                     │
│                                                          │
│ Step 3 — Hybrid retrieval query across root.security.*   │
│ Step 4 — Distillation runs (LLM-powered summarization)   │
│                                                          │
│ Default API key: testkey123 (admin role)                 │
└──────────────────────────────────────────────────────────┘
```

**Alternative: Docker only (no script)**

```bash
# Pull and run directly
docker run --rm -p 8000:8000 \
  -e DATABASE_URL="postgres://user:pass@host:5432/db?sslmode=disable" \
  -e REDIS_ADDR="redis:6379" \
  ghcr.io/marco-spagn/pcmi:latest
```

Pre-built multi-arch images (`linux/amd64`, `linux/arm64`) on every release:

```bash
docker pull ghcr.io/marco-spagn/pcmi:latest    # stable
docker pull ghcr.io/marco-spagn/pcmi:v1.51.0    # pinned
docker pull ghcr.io/marco-spagn/pcmi:main       # bleeding edge
```

### Your first API calls

```bash
export PCMI_URL=http://localhost:8000
export PCMI_KEY=testkey123

# Store a memory
curl -s -X POST "$PCMI_URL/v1/memories" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $PCMI_KEY" \
  -d '{"path":"root.demo.note","content":"The new caching layer reduced p99 latency from 340ms to 12ms","tags":["engineering","perf"]}'

# Retrieve with hybrid ranking
curl -s -X POST "$PCMI_URL/v1/retrieve" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $PCMI_KEY" \
  -d '{"path_prefix":"root.demo","query":"caching latency","limit":5}' | jq .

# Stream events in real time
curl -sN "$PCMI_URL/v1/events" \
  -H "X-API-Key: $PCMI_KEY" \
  -H "Accept: text/event-stream"
```

**Next:** [docs/USAGE.md](docs/USAGE.md) — full API reference · [docs/SESSIONS.md](docs/SESSIONS.md) — agent sessions

---

## Architecture

```mermaid
sequenceDiagram
  autonumber
  participant A as Agent / App
  participant API as PCMI API<br/>(Go, Fiber + gRPC)
  participant PG as PostgreSQL 16<br/>+ pgvector + ltree
  participant R as Redis 7<br/>Streams
  participant W as PCMI Worker<br/>(Go)

  A->>API: POST /v1/memories
  API->>PG: INSERT memory version N<br/>close version N-1
  API->>PG: INSERT audit_log
  API->>R: XADD pcmi:events<br/>memory.stored
  API-->>A: 201 {id, version, path}

  R-->>W: XREADGROUP pcmi:events
  W->>PG: UPDATE embedding (async)
  W->>W: Distillation check<br/>(policy-driven)
  W->>PG: INSERT distilled_knowledge
  W->>R: XADD pcmi:events<br/>knowledge.distilled
```

### Data flow at a glance

| Component | Language | Port | Responsibility |
|-----------|----------|------|----------------|
| **API Server** | Go (Fiber) | `:8000` HTTP, `:50051` gRPC | REST, gRPC, SSE, Prometheus `/metrics`, Admin UI, MCP stdio |
| **Worker** | Go | `:8081` health | Embedding (circuit breaker), distillation, pruning, consolidation, expiry |
| **PostgreSQL 16** | — | `:5432` | Primary store: memories, embeddings (pgvector), paths (ltree), sessions, audit, webhooks, RLS |
| **Redis 7** | — | `:6379` | Event bus (Streams), worker coordination, rate-limit backend |
| **Apache AGE** *(opt-in)* | — | `:5433` | Cognitive Graph: typed memory_links, multi-hop traversal, Cypher queries |

### Hybrid retrieval pipeline

```
GET /v1/retrieve
      │
      ▼
┌─────────────────────────────────────────────────────┐
│ Single SQL query, 5 fused signals:                  │
│                                                     │
│  score = W_semantic · cosine_similarity(query, emb) │
│        + W_lexical  · ts_rank_cd(tsv, websearch)    │
│        + W_import   · importance                    │
│        + W_temporal · exp(-λ · age_hours)           │
│                                                     │
│  Default weights: 0.40 / 0.30 / 0.15 / 0.15         │
│  Per-tenant configurable via tenant_memory_config   │
│  Per-request overrides for ad-hoc tuning            │
└─────────────────────────────────────────────────────┘
```

Deep dives: [docs/architecture.md](docs/architecture.md) · [docs/DATA-MODEL.md](docs/DATA-MODEL.md) · [docs/retrieval-pipeline.md](docs/retrieval-pipeline.md) · [docs/WORKERS-AND-EVENTS.md](docs/WORKERS-AND-EVENTS.md)

---

## Benchmarks & Comparison

### Performance (k6 load test — 50 VUs sustained)

| Metric | SLO | Typical (local) |
|--------|-----|-----------------|
| **Store** P99 latency | < 50ms | 12–28ms |
| **Retrieve** P99 latency | < 100ms | 18–45ms |
| **HTTP error rate** | < 1% | < 0.1% |
| **Throughput** at 50 VUs | — | ~850 req/s (store) / ~1,200 req/s (retrieve) |

Run it yourself: `k6 run scripts/load/k6_store_retrieve.js`

### Go micro-benchmarks (internal)

```bash
make bench-retrieval  # BenchmarkHybridScore — ~4.2 µs/op scoring
make bench            # Worker, model, crypto — full suite
```

### Comparison: PCMI vs Mem0 vs Zep vs LangGraph

| Dimension | **PCMI** | **Mem0** | **Zep** | **LangGraph Store** |
|-----------|----------|----------|---------|---------------------|
| **Deployment** | Self-hosted, Docker/K8s/Helm | Cloud (primary), self-host (OSS) | Cloud (primary), stale OSS | In-process (Python/JS) |
| **Database** | PostgreSQL + pgvector + ltree | SQLite / Postgres | Postgres + pgvector | In-memory, SQLite, or Postgres |
| **API surface** | HTTP REST + gRPC + MCP | HTTP REST (Python SDK) | HTTP REST | LangGraph Python API only |
| **SDK languages** | Python, TypeScript, Go | Python only | Python, TypeScript, Go | Python, JavaScript |
| **Multi-tenant** | ✅ Native (tenant-scoped queries; RLS policies, [opt-in enforcement](docs/DATA-MODEL.md#tenant-isolation)) | ❌ Per-user (no org isolation) | ✅ User/group model | ❌ Not designed for multi-tenant |
| **Append-only versioning** | ✅ `as_of` temporal queries | ❌ Latest state only | ❌ Latest state only | ✅ Checkpoint versioning |
| **Retrieval ranking** | BM25 + semantic + importance + decay | Semantic only | Semantic + graph + BM25 | None (state fetch) |
| **Session working memory** | ✅ Promote to long-term | ❌ | ✅ Fact memory | ✅ State checkpointing |
| **Automated distillation** | ✅ LLM-powered, policy-driven | ❌ | ❌ | ❌ |
| **Event streaming** | Redis Streams + SSE + gRPC + webhooks | Basic callbacks | Webhooks | None (framework-level) |
| **RBAC / API keys** | ✅ Admin / User (read-write) / ReadOnly + rotation | ❌ API key only | ❌ API key only | ❌ |
| **Audit log** | ✅ Full per-request audit | ❌ | Paid tier | ❌ |
| **Rate limiting** | ✅ Redis-backed, per-key | ❌ | Paid tier | ❌ |
| **Encryption at rest** | ✅ Column-level AES | ❌ | Paid tier | ❌ |
| **Idempotency** | ✅ `X-Idempotency-Key` | ❌ | ❌ | ❌ |
| **Graph querying** | ✅ Apache AGE (typed links, Cypher) | ❌ | ✅ Knowledge graph | ❌ |
| **Cognitive Graph UI** | ✅ `/v1/graph/ui` | ❌ | ❌ | ❌ |
| **MCP server** | ✅ stdio for Cursor/Claude | ❌ | ❌ | ❌ |
| **OpenAPI spec** | ✅ Complete, versioned | Partial | Partial | N/A |
| **Kubernetes support** | ✅ Helm chart + Kustomize | ❌ | Basic | N/A |
| **Language** | Go (high perf, low resource) | Python | Python/Go | Python |
| **License** | Apache 2.0 | Apache 2.0 | Apache 2.0 | MIT |

### When to use what

| Your need | Best choice | Why |
|-----------|-------------|-----|
| Quick personal memory for GPT wrappers | **Mem0** | Simple Python SDK, cloud-hosted, fast to start |
| Chatbot with user/session memory | **Zep** | Good user/session model, graph + vector |
| State management inside LangGraph | **LangGraph Store** | Native integration, zero setup |
| **Multi-agent org with audit requirements** | **PCMI** | Multi-tenant, versioned, enterprise controls, any framework |
| **Regulated industry (fintech, healthcare, security)** | **PCMI** | Audit log, RLS, encryption, `as_of` queries |
| **High-throughput, language-agnostic** | **PCMI** | gRPC + Go SDK, 3+ language clients |

---

## Project Deliverables

PCMI ships as a complete, production-ready stack — not just a library.

### Core infrastructure

| Deliverable | Location | Description |
|-------------|----------|-------------|
| **PostgreSQL schema** | [`migrations/`](migrations/) | 20 migration files: tenants, memories (ltree + pgvector), RBAC, sessions, dedup, distillation, cognitive graph |
| **OpenAPI 3.0 spec** | [`docs/openapi.yaml`](docs/openapi.yaml) | Complete REST API specification, versioned at v1.51.0 |
| **Protobuf definitions** | [`proto/pcmi/v1/`](proto/pcmi/v1/) | gRPC: `MemoryService` (45 RPCs), `AdminService`, `MetricsService` |
| **Docker Compose** | [`docker-compose.yml`](docker-compose.yml) | Full stack: API, Worker, PostgreSQL+pgvector, Redis, optional AGE |
| **Dockerfiles** | `Dockerfile`, `Dockerfile.api`, `Dockerfile.worker` | Multi-arch images published to ghcr.io |
| **Helm chart** | [`deploy/helm/pcmi/`](deploy/helm/) | Production Kubernetes: HPA, PDB, NetworkPolicy, TLS, migration init-container |
| **K8s manifests (Kustomize)** | [`deploy/k8s/`](deploy/k8s/) | Base + dev/prod overlays: API, Worker, ConfigMap, HPA, PDB |

### Client SDKs

| Language | Package | Runtime | Transport |
|----------|---------|---------|-----------|
| **Python** | `pcmi` (PyPI) | Python 3.10+ | `httpx` async |
| **TypeScript** | `@marco-spagn/pcmi-sdk` (npm) | Node / Browser | Native `fetch` |
| **Go** | `github.com/marco-spagn/pcmi/sdk/go/pcmi` | Go 1.25+ | `net/http` stdlib |

All SDKs cover: store, retrieve, batch, sessions, events (SSE), webhooks, admin, links, stats, lineage, import/export, embeddings migration.

Full SDK reference: [sdk/README.md](sdk/README.md) · [sdk/HTTP-API.md](sdk/HTTP-API.md)

### Event system

| Event type | Schema | Trigger |
|-----------|--------|---------|
| `memory.stored` | strict: id, tenant_id, path, version | Each `POST /v1/memories` |
| `memory.updated` | strict: +superseded_id | Each version update |
| `knowledge.distilled` | strict: tenant_id, path, distilled_id | Worker distillation complete |
| `memory.refine.requested` | loose | Manual refine trigger |
| `agent.step.completed` | strict: step | Agent checkpoints |
| `tool.call.executed` | strict: tool | Tool invocation |
| `workflow.finished` | loose | Workflow completion |
| `reasoning.generated` | loose | Chain-of-thought output |
| `contradiction.detected` | strict: from_path, to_path, confidence | Contradiction analysis |

Transport: Redis Streams → SSE / gRPC streams / webhooks (HMAC-SHA256 `timestamp.body`)

---

## Integration Examples

### LangChain

```bash
cd examples/langchain
```

```python
from pcmi_tools import PCMI_TOOLS
from langchain.agents import create_react_agent

agent = create_react_agent(llm, PCMI_TOOLS, prompt)
# Agent now has: pcmi_store, pcmi_retrieve, pcmi_session_remember
```

→ [`examples/langchain/`](examples/langchain/) — store, retrieve, session working memory

### CrewAI

```bash
cd examples/crewai
```

```python
from pcmi_tools import PCMI_TOOLS
from crewai import Agent

analyst = Agent(
    role="Security Analyst",
    tools=PCMI_TOOLS,
    # Agent now has: pcmi_store, pcmi_retrieve
)
```

→ [`examples/crewai/`](examples/crewai/) — `@tool` decorated store/retrieve

### LangGraph

```python
from pcmi import PCMIClient
from langgraph.graph import StateGraph

pcmi = PCMIClient("http://localhost:8000", "testkey123")

async def store_to_pcmi(state: dict) -> dict:
    """Checkpoint agent state into PCMI after each node."""
    await pcmi.store(
        path=f"root.langgraph.{state['thread_id']}",
        content=json.dumps(state),
        metadata={"workflow": "research", "node": state.get("current_node")}
    )
    return state

async def retrieve_context(state: dict) -> dict:
    """Pull relevant memories before LLM call."""
    result = await pcmi.retrieve(
        path_prefix=f"root.langgraph.{state['thread_id']}",
        query=state.get("query", ""),
        limit=5
    )
    state["context_memories"] = result["entries"]
    return state

workflow = StateGraph(AgentState)
workflow.add_node("retrieve", retrieve_context)
workflow.add_node("act", my_agent_node)
workflow.add_node("persist", store_to_pcmi)
workflow.add_edge("retrieve", "act")
workflow.add_edge("act", "persist")
```

### AutoGen

```bash
cd examples/autogen
```

```python
from pcmi_tools import build_pcmi_tools
from autogen_agentchat.agents import AssistantAgent

tools = build_pcmi_tools()
agent = AssistantAgent("researcher", tools=tools)
```

→ [`examples/autogen/`](examples/autogen/) — AgentChat `FunctionTool` wrappers

### LlamaIndex

```bash
cd examples/llamaindex
```

```python
from pcmi_tools import PCMI_TOOLS
from llama_index.core.agent import FunctionAgent

agent = FunctionAgent.from_tools(PCMI_TOOLS, llm=llm)
```

→ [`examples/llamaindex/`](examples/llamaindex/) — `FunctionTool` store/retrieve

### Temporal.io (durable execution)

```python
# Worker: async PCMI calls as Temporal activities
@activity.defn
async def pcmi_store_activity(path: str, content: str) -> dict:
    return await store(path, content)

@activity.defn
async def pcmi_retrieve_activity(path_prefix: str, query: str) -> dict:
    return await retrieve(path_prefix, query)
```

→ [`examples/temporal/`](examples/temporal/) — workflows + activities + worker

### Celery (task queue)

```bash
cd examples/celery
```

```python
from pcmi_tasks import pcmi_store, pcmi_retrieve

pcmi_store.delay("root.celery.task", "async store result")
result = pcmi_retrieve.delay("root.celery", "", 10).get()
```

→ [`examples/celery/`](examples/celery/) — async store/retrieve via `httpx`

### Custom agent (raw HTTP)

```python
import httpx

async def my_agent_memory(action: str, **kwargs):
    """Drop-in memory for any custom agent loop."""
    async with httpx.AsyncClient(base_url="http://localhost:8000") as client:
        headers = {"X-API-Key": "testkey123", "Content-Type": "application/json"}

        if action == "remember":
            r = await client.post("/v1/memories", json=kwargs, headers=headers)
            return r.json()

        if action == "recall":
            r = await client.post("/v1/retrieve", json=kwargs, headers=headers)
            return r.json()

        if action == "listen":
            async with client.stream("GET", "/v1/events", headers=headers) as r:
                async for line in r.aiter_lines():
                    if line.startswith("data:"):
                        yield json.loads(line[5:])
```

### MCP (Model Context Protocol) — Cursor / Claude Desktop

```json
{
  "mcpServers": {
    "pcmi": {
      "command": "pcmi-mcp",
      "env": {
        "PCMI_BASE_URL": "http://localhost:8000",
        "PCMI_API_KEY": "testkey123"
      }
    }
  }
}
```

→ [docs/MCP.md](docs/MCP.md) — stdio server for AI code assistants

---

## Features

| Area | Capabilities |
|------|-------------|
| **Memory** | Hierarchical `ltree` paths, append-only versioning, tags, TTL, importance (0–1), optional field encryption |
| **Retrieve** | Hybrid ranking: BM25 + semantic + importance + temporal decay; `as_of` temporal reads; keyset cursors |
| **Sessions** | Agent sessions + working memory; promote to long-term (`/v1/sessions/*`) — [docs/SESSIONS.md](docs/SESSIONS.md) |
| **Dedup** | Content-hash dedup at ingest (`none` / `skip` / `link` / `merge`) — env, tenant, or `X-Dedup-Mode` header |
| **Workers** | Embedding with circuit breaker, distillation (LLM summarization), consolidation, pruning, compaction, expiry |
| **Events** | Redis **Streams** by default (`EVENT_BACKEND=streams`); legacy pub/sub; SSE + gRPC streams; webhooks with HMAC |
| **Webhooks** | HMAC-SHA256 (`timestamp.body`), retry with dead-letter queue, per-endpoint event type filtering |
| **Graph** *(experimental)* | Typed `memory_links` synced to **Apache AGE** — multi-hop traversal, shortest path, 5 link types (`causal`/`temporal`/`contradicts`/`supports`/`related`), Cypher passthrough (`MATCH` only), Graph Explorer UI at `/v1/graph/ui` — [docs](docs/cognitive-graph.md) |
| **Security** | API-key RBAC + rotation/lifecycle, tenant-scoped queries with [opt-in PostgreSQL RLS](docs/DATA-MODEL.md#tenant-isolation), column encryption, optional metrics Bearer token |
| **Rate limit** | Per-key limits; `RATE_LIMIT_BACKEND=redis` for multi-instance API |
| **Idempotency** | `X-Idempotency-Key` with 24h cache per tenant |
| **Ops** | Prometheus metrics (`/metrics`), OpenTelemetry, health/readiness probes, Helm chart, Kustomize overlays |
| **Admin** | Tenant/API-key CRUD + rotate/revoke, embedded UI at `GET /v1/admin/ui`, gRPC admin service |
| **Pagination** | Keyset cursors (`limit`, `cursor`, `after_id`) on all list endpoints |

---

## APIs & Clients

| Surface | When to use |
|---------|-------------|
| **HTTP REST** | OpenAPI tooling, browsers, SSE, Prometheus scrape, admin UI |
| **gRPC** | Agents, batch workloads, streaming retrieve/events; 45 RPCs across `MemoryService`, `AdminService`, `MetricsService` |
| **Python SDK** | `pip install pcmi` ([PyPI](https://pypi.org/project/pcmi/)) — async `httpx` client |
| **TypeScript SDK** | `npm install @marco-spagn/pcmi-sdk` ([npm](https://www.npmjs.com/package/@marco-spagn/pcmi-sdk)) — Node/browser `fetch` |
| **Go SDK** | `go get github.com/marco-spagn/pcmi/sdk/go/pcmi` — stdlib `net/http` |
| **MCP** | stdio server for Cursor / Claude Code — [docs/MCP.md](docs/MCP.md) |

| Reference | Location |
|-----------|----------|
| OpenAPI 3 | [docs/openapi.yaml](docs/openapi.yaml) |
| gRPC protos | [proto/pcmi/v1/](proto/pcmi/v1/) |
| gRPC ↔ HTTP matrix | [docs/grpc-vs-http.md](docs/grpc-vs-http.md) |
| SDK reference | [sdk/README.md](sdk/README.md) |

---

## Documentation

**Full index:** [docs/INDEX.md](docs/INDEX.md)

| Document | Topic |
|----------|-------|
| [docs/USAGE.md](docs/USAGE.md) | End-to-end usage (HTTP, gRPC, env, paths, pagination) |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | Schema design, versioning, RLS |
| [docs/architecture.md](docs/architecture.md) | System design, components, data flow |
| [docs/retrieval-pipeline.md](docs/retrieval-pipeline.md) | Hybrid scoring: BM25 + semantic + importance + decay |
| [docs/WORKERS-AND-EVENTS.md](docs/WORKERS-AND-EVENTS.md) | Background jobs, Redis Streams, webhooks, HMAC |
| [docs/SESSIONS.md](docs/SESSIONS.md) | Agent sessions + working memory + promote |
| [docs/cognitive-graph.md](docs/cognitive-graph.md) | Apache AGE integration, graph UI, Cypher |
| [docs/MCP.md](docs/MCP.md) | MCP stdio server for Cursor / Claude |
| [docs/CODEBASE.md](docs/CODEBASE.md) | Go package map for contributors |
| [docs/integration-testing.md](docs/integration-testing.md) | Integration test tags and patterns |
| [docs/local-ci.md](docs/local-ci.md) | Reproduce CI locally |
| [docs/distillation-tests.md](docs/distillation-tests.md) | Distillation E2E harness |
| [deploy/helm/README.md](deploy/helm/README.md) | Kubernetes / Helm deployment |
| [CHANGELOG.md](CHANGELOG.md) | Release history |

---

## Repository Layout

```
pcmi/
├── cmd/
│   ├── api/            # HTTP + gRPC server, /metrics, admin UI
│   ├── worker/         # Embedding, distillation, pruning, expiry
│   └── mcp/            # MCP stdio server (pcmi-mcp)
├── internal/           # Go domain logic (handler, service, repository, worker, grpc, graph)
├── proto/pcmi/v1/      # Protobuf definitions (MemoryService, AdminService, MetricsService)
├── migrations/         # 20 SQL migrations (001 → 020)
├── sdk/
│   ├── python/         # Python SDK (pcmi on PyPI)
│   ├── typescript/     # TypeScript SDK (npm package)
│   └── go/             # Go SDK
├── examples/
│   ├── langchain/      # LangChain integration
│   ├── crewai/         # CrewAI integration
│   ├── autogen/        # AutoGen integration
│   ├── llamaindex/     # LlamaIndex integration
│   ├── temporal/       # Temporal.io workflows
│   ├── celery/         # Celery tasks
│   └── soc-incident-graph/  # SOC example dataset
├── deploy/
│   ├── helm/pcmi/      # Production Helm chart
│   └── k8s/            # Kustomize base + dev/prod overlays
├── docs/               # Documentation hub
├── scripts/            # CI, E2E, load tests, quickstart
├── docker-compose.yml  # Full local stack
├── Dockerfile           # Multi-arch release image
└── Makefile            # 60+ targets
```

---

## Development

```bash
# Quick iteration cycle
make dev-up          # Start dependencies (postgres + redis)
make dev-api         # Run API locally
make test            # Unit tests
make lint            # golangci-lint v2

# Integration tests
make test-integration-bufconn   # gRPC on bufconn (Postgres only)
make infra-up && make test-integration-live  # Full stack + TCP gRPC
make test-integration           # Both

# Feature-specific tests
make test-streams-integration    # Redis Streams
make test-ratelimit-integration  # Redis rate limiting
make test-idempotency            # X-Idempotency-Key
make test-retrieval-scoring      # Importance + temporal decay
make test-sessions-integration   # Agent sessions
make test-dedup                  # Content dedup
make test-key-lifecycle          # API key rotation/revoke

# SDK smoke tests
make sdk-smoke                   # Python + TypeScript

# Full CI parity (~15-45 min)
make ci-like-github

# Benchmarks
make bench-retrieval             # Hybrid score micro-benchmark
make bench                       # Worker, model, crypto benchmarks
```

---

## Contributing

Issues and PRs are welcome. Read **[CONTRIBUTING.md](CONTRIBUTING.md)** for setup, test conventions, migration rules, and proto guidelines.

---

## Security

Report vulnerabilities **privately** — see **[SECURITY.md](SECURITY.md)** for disclosure process and response SLAs.

---

## License

[Apache License 2.0](LICENSE) — Copyright 2026 Marco Spagnuolo & PCMI Team.

---

<p align="center">
  <em>Built with Go, PostgreSQL, and the conviction that agents deserve better memory.</em>
</p>
