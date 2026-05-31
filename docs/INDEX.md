# PCMI documentation index

Central map of repository documentation. **API version: v1.48.0** ([`internal/version/version.go`](../internal/version/version.go)).

## Getting started

| Document | Audience | Content |
|----------|----------|---------|
| [README.md](../README.md) | Everyone | Overview, quick start, architecture, links |
| [USAGE.md](USAGE.md) | Integrators | Step-by-step HTTP, gRPC, SDK, env, paths |
| [architecture.md](architecture.md) | Architects | Components, flows, deployment |
| [DATA-MODEL.md](DATA-MODEL.md) | Backend / DBA | Schema, versioning, RLS |
| [cognitive-graph.md](cognitive-graph.md) | Backend / ops | AGE graph traversal, Graph UI, [who classifies data](cognitive-graph.md#how-data-enters-pcmi--who-classifies-what) (not SOC-specific), [demo video](assets/graph-ui-demo.mp4) |

## API and clients

| Document | Content |
|----------|---------|
| [openapi.yaml](openapi.yaml) | REST contract (OpenAPI 3) |
| [grpc-vs-http.md](grpc-vs-http.md) | gRPC RPC ↔ HTTP route matrix |
| [../sdk/README.md](../sdk/README.md) | Python & TypeScript HTTP SDKs |
| [../sdk/HTTP-API.md](../sdk/HTTP-API.md) | Endpoint → SDK method mapping |
| [MCP.md](MCP.md) | MCP stdio server for AI agents (`cmd/mcp`) |
| [../proto/pcmi/v1/memory.proto](../proto/pcmi/v1/memory.proto) | Core gRPC memory API |
| [../proto/pcmi/v1/admin.proto](../proto/pcmi/v1/admin.proto) | Admin gRPC API |
| [../proto/pcmi/v1/metrics.proto](../proto/pcmi/v1/metrics.proto) | Metrics gRPC API |

## Sprint 1–2 features (operational)

| Topic | Document / command |
|-------|------------------|
| Redis Streams (`EVENT_BACKEND`) | [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md), `make test-streams-integration` |
| Embedding circuit breaker | [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md), `make test-circuit-breaker` |
| Distributed rate limit | [USAGE.md](USAGE.md) (`RATE_LIMIT_BACKEND=redis`), `make test-ratelimit-integration` |
| Metrics scrape auth | [USAGE.md](USAGE.md) (`METRICS_SCRAPE_TOKEN`) |
| Store idempotency | [USAGE.md](USAGE.md) (`X-Idempotency-Key`), `make test-idempotency` |
| Webhook HMAC | [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md) |
| API key lifecycle | [USAGE.md](USAGE.md), `make admin-list-keys`, `make test-key-lifecycle` |
| Importance + decay | [retrieval-pipeline.md](retrieval-pipeline.md), `make smoke-importance` |
| Agent sessions | [SESSIONS.md](SESSIONS.md), `make smoke-sessions` |
| Ingest dedup | [USAGE.md](USAGE.md) (`DEDUP_MODE`), `make smoke-dedup` |
| MCP server | [MCP.md](MCP.md), `make test-mcp-unit` |
| Full local validation | [local-ci.md](local-ci.md) (`make test-full-real`) |

## Testing

| Document / script | Content |
|-------------------|---------|
| [integration-testing.md](integration-testing.md) | `-tags=integration`, SSE httptest notes |
| [local-ci.md](local-ci.md) | `make ci-like-github`, `make test-full-real`, CI jobs |
| [distillation-tests.md](distillation-tests.md) | E2E distillation harness (`make distillation-e2e`) |
| `scripts/smoke_importance_retrieve.sh` | Manual importance retrieve ranking |
| `scripts/smoke_sessions.sh` | Sessions curl E2E |
| `scripts/smoke_dedup.sh` | Dedup ingest curl E2E |
| [../scripts/pcmi_synth/README.md](../scripts/pcmi_synth/README.md) | Synthetic data CLI (presets, seed, size, optional LLM) |
| [../scripts/distill_e2e.sh](../scripts/distill_e2e.sh) | Simple distillation E2E wrapper |
| [../scripts/run_pcmi_distillation_test.sh](../scripts/run_pcmi_distillation_test.sh) | Full orchestrator (Docker + ingest + refine) |
| [../scripts/e2e/README.md](../scripts/e2e/README.md) | Manual / CI E2E shell scripts |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Contributor setup and PR checklist |
| [API-VERSIONING.md](API-VERSIONING.md) | API SemVer, releases, git-cliff, tags |

## Pipeline and operations

| Document | Content |
|----------|---------|
| [retrieval-pipeline.md](retrieval-pipeline.md) | Hybrid retrieve (ltree + BM25 + vector) |
| [SESSIONS.md](SESSIONS.md) | Agent sessions and working memory (v1.43) |
| [memory-compaction.md](memory-compaction.md) | Compaction vs pruning |
| [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md) | Workers, Redis, SSE, webhooks |
| [MIGRATIONS.md](MIGRATIONS.md) | SQL migrations in order |
| [failure-modes.md](failure-modes.md) | Failures and mitigations |
| [scalability.md](scalability.md) | Scaling and limits |
| [federation-read-replicas.md](federation-read-replicas.md) | `DATABASE_READ_URL` |
| [CODEBASE.md](CODEBASE.md) | Go package map |

## Examples and roadmap

| Document | Content |
|----------|---------|
| [../examples/README.md](../examples/README.md) | Celery, Temporal, LangChain, LlamaIndex, AutoGen, CrewAI |
| [roadmap.md](roadmap.md) | Release evolution |
| [high-lev-arch.md](high-lev-arch.md) | Long-form architecture vision |

## Diagrams

| Resource | Topic |
|----------|--------|
| Mermaid in [README.md](../README.md), [architecture.md](architecture.md), [USAGE.md](USAGE.md) | Architecture & flows (preferred; renders on GitHub) |
| [save-data_seq_diagram.md](save-data_seq_diagram.md) | Store sequence (Mermaid source) |
| [architecture_v1.4.md](architecture_v1.4.md) | **Legacy** — superseded by [architecture.md](architecture.md) |

## Technical report (optional)

| Path | Content |
|------|---------|
| [papers/PCMI_Technical_Report_v1.33.md](papers/PCMI_Technical_Report_v1.33.md) | Long-form architecture report |
| [papers/build_report.py](papers/build_report.py) | PDF/PPTX build scripts (WeasyPrint) |

## Deployment

| Document | Content |
|----------|---------|
| [../deploy/helm/README.md](../deploy/helm/README.md) | Helm chart |
| [../k8s/README.md](../k8s/README.md) | Deprecated root `k8s/` — use Helm |
