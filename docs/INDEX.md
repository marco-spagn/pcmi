# PCMI documentation index

Central map of repository documentation. **API version: v1.42.0** ([`internal/version/version.go`](../internal/version/version.go)).

## Getting started

| Document | Audience | Content |
|----------|----------|---------|
| [README.md](../README.md) | Everyone | Overview, quick start, architecture, links |
| [USAGE.md](USAGE.md) | Integrators | Step-by-step HTTP, gRPC, SDK, env, paths |
| [architecture.md](architecture.md) | Architects | Components, flows, deployment |
| [DATA-MODEL.md](DATA-MODEL.md) | Backend / DBA | Schema, versioning, RLS |

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

## Testing

| Document / script | Content |
|-------------------|---------|
| [integration-testing.md](integration-testing.md) | `-tags=integration`, SSE httptest notes |
| [local-ci.md](local-ci.md) | `make ci-like-github`, CI jobs |
| [distillation-tests.md](distillation-tests.md) | E2E distillation harness (`make distillation-e2e`) |
| [../scripts/pcmi_synth/README.md](../scripts/pcmi_synth/README.md) | Synthetic data CLI (presets, seed, size, optional LLM) |
| [../scripts/distill_e2e.sh](../scripts/distill_e2e.sh) | Simple distillation E2E wrapper |
| [../scripts/run_pcmi_distillation_test.sh](../scripts/run_pcmi_distillation_test.sh) | Full orchestrator (Docker + ingest + refine) |
| [../scripts/e2e/README.md](../scripts/e2e/README.md) | Manual / CI E2E shell scripts |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Contributor setup and PR checklist |

## Pipeline and operations

| Document | Content |
|----------|---------|
| [retrieval-pipeline.md](retrieval-pipeline.md) | Hybrid retrieve (ltree + BM25 + vector) |
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
| [../examples/README.md](../examples/README.md) | Celery, Temporal |
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
