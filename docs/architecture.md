# PCMI Architecture

Persistent Cognitive Memory Infrastructure (PCMI) separates **ephemeral agents** from **durable memory**.

> **Agents are ephemeral. Memory is persistent.**

## System context

```mermaid
C4Context
  title PCMI - System Context
  Person(agent, "AI Agent", "Ephemeral runtime")
  System(pcmi, "PCMI", "Memory API + workers")
  System_Ext(openai, "OpenAI", "Optional embeddings")
  Rel(agent, pcmi, "HTTP / gRPC")
  Rel(pcmi, openai, "Embeddings", "optional")
```

## Container view

```mermaid
flowchart TB
  subgraph clients [Clients]
    SDK[Python / TS SDK]
    ORCH[Celery / Temporal]
    AG[Custom agents]
  end
  subgraph pcmi_stack [PCMI stack]
    API[cmd/api :8000 / :50051]
    WK[cmd/worker :8081 metrics]
  end
  subgraph data [Data]
    PG[(PostgreSQL + ltree + pgvector)]
    RD[(Redis)]
  end
  clients --> API
  API --> PG
  API --> RD
  WK --> PG
  WK --> RD
```

## Principles

- **Runtime agnostic**: HTTP/gRPC; no framework lock-in in core.
- **Append-only cognition**: `valid_from` / `valid_to`; rollback = new row.
- **Hierarchical paths**: `ltree` scopes retrieval.
- **Hybrid retrieval**: prefix + BM25 + optional semantic (pgvector).
- **Event-driven workers**: Redis → embed, distill, prune, webhooks.

## Components

| Component | Role |
|-----------|------|
| `cmd/api` | Fiber REST, SSE, Prometheus `/metrics`, gRPC `MemoryService`, OTLP traces |
| `cmd/worker` | Embeddings, distillation, pruning, consolidation, expiry; `:8081/metrics` |
| PostgreSQL | Memories, distilled knowledge, links, audit, webhooks; RLS per tenant |
| Redis | Event bus: **Streams** `pcmi:events` (default) or pub/sub `memory_events` (`EVENT_BACKEND`) |
| SDKs | HTTP clients — see `sdk/` |

## Store flow

```mermaid
sequenceDiagram
  participant A as Agent
  participant API as pcmi-api
  participant DB as PostgreSQL
  participant R as Redis
  participant W as worker
  A->>API: Store(path, content)
  API->>DB: append row, close prior version
  API->>R: memory.stored / updated
  API-->>A: id, version
  R->>W: consume
  W->>DB: embedding / distill
  W->>R: knowledge.distilled
```

## API surfaces

| Surface | Port | Auth | Note |
|---------|------|------|------|
| HTTP REST | 8000 | `X-API-Key` | OpenAPI `docs/openapi.yaml` |
| gRPC | 50051 | `api_key` / metadata | `MemoryService`, `AdminService`, `MetricsService` |
| SSE events | 8000 | API key | `GET /v1/events` |
| Admin API | 8000 / 50051 | `admin` role | HTTP routes + gRPC `AdminService` |
| Admin UI | 8000 | `admin` role | `GET /v1/admin/ui` (browser, HTTP only) |
| Prometheus | 8000 | optional Bearer (`METRICS_SCRAPE_TOKEN`) | HTTP scrape or gRPC `MetricsService.Scrape` |

Full matrix: [grpc-vs-http.md](grpc-vs-http.md).

## Security

- RLS: `set_tenant_context(uuid)` per request.
- API keys: SHA-256 hash; roles `readonly`, `write`, `admin`.
- Optional encryption: `PCMI_ENCRYPTION_KEY`.

## Deployment

- **Local**: `docker compose up`
- **K8s**: `deploy/k8s/base/` + overlays — readiness `GET /v1/ready`, gRPC `Ready`
- **CI**: lint, unit, integration-smoke, SDK smoke, optional OpenAI E2E; runs on push/PR
- **Local full validation**: `make test-full-real` (see [local-ci.md](local-ci.md))

## Event bus (streams vs pub/sub)

```mermaid
flowchart LR
  API[api PublishEvent]
  subgraph redis [Redis]
    S[Stream pcmi:events]
    P[PubSub memory_events]
  end
  W[worker consumer]
  API -->|EVENT_BACKEND=streams| S
  API -->|EVENT_BACKEND=pubsub| P
  S --> W
  P --> W
```

## Related docs

- [USAGE.md](USAGE.md) — how to use PCMI
- [DATA-MODEL.md](DATA-MODEL.md) — schema and versioning
- [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md) — background jobs
- [failure-modes.md](failure-modes.md), [scalability.md](scalability.md)
- [CODEBASE.md](CODEBASE.md), [MIGRATIONS.md](MIGRATIONS.md)
- [INDEX.md](INDEX.md) — full doc index
