# PCMI — indice documentazione

Mappa di tutta la documentazione del repository (API **v1.31.0**).

## Iniziare

| Documento | Per chi | Contenuto |
|-----------|---------|-----------|
| [README.md](../README.md) | Tutti | Panoramica, diagrammi, **come usare PCMI** (HTTP, gRPC, SDK) |
| [USAGE.md](USAGE.md) | Sviluppatori agenti | Guida operativa passo-passo |
| [architecture.md](architecture.md) | Architetti | Componenti, flussi, deployment |
| [DATA-MODEL.md](DATA-MODEL.md) | Backend / DBA | Schema logico, versioning, RLS |

## API e client

| Documento | Contenuto |
|-----------|-----------|
| [openapi.yaml](openapi.yaml) | Contratto REST OpenAPI 3 |
| [grpc-vs-http.md](grpc-vs-http.md) | Tabella RPC gRPC ↔ route HTTP |
| [../sdk/README.md](../sdk/README.md) | SDK Python e TypeScript |
| [../sdk/HTTP-API.md](../sdk/HTTP-API.md) | Mapping endpoint → metodi SDK |
| [../proto/pcmi/v1/memory.proto](../proto/pcmi/v1/memory.proto) | Definizione gRPC |

## Test locali

| Documento / script | Contenuto |
|--------------------|-----------|
| [integration-testing.md](integration-testing.md) | `go test -tags=integration`, **SSE httptest** che può bloccare ~10m, `PCMI_SKIP_SSE_HTTPTEST` |
| [local-ci.md](local-ci.md) | `act`, `make ci-like-github`, job CI |
| [../scripts/run_pcmi_distillation_test.sh](../scripts/run_pcmi_distillation_test.sh) | E2E ingest SOC + refine Redis + verifica distilled (`make distillation-e2e`) |
| [../scripts/generate_soc_incidents_enterprise_v2.py](../scripts/generate_soc_incidents_enterprise_v2.py) | Generatore incidenti sintetici per il test sopra |

## Pipeline e operazioni

| Documento | Contenuto |
|-----------|-----------|
| [retrieval-pipeline.md](retrieval-pipeline.md) | Retrieve ibrido (ltree + BM25 + vettoriale) |
| [memory-compaction.md](memory-compaction.md) | Compaction per path vs pruning |
| [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md) | Worker, Redis, SSE, webhook |
| [MIGRATIONS.md](MIGRATIONS.md) | Migration SQL in ordine |

## Produzione

| Documento | Contenuto |
|-----------|-----------|
| [failure-modes.md](failure-modes.md) | Guasti e mitigazioni |
| [scalability.md](scalability.md) | Scaling e limiti |
| [federation-read-replicas.md](federation-read-replicas.md) | `DATABASE_READ_URL` |
| [CODEBASE.md](CODEBASE.md) | Mappa pacchetti Go |

## Esempi e roadmap

| Documento | Contenuto |
|-----------|-----------|
| [../examples/README.md](../examples/README.md) | Celery, Temporal |
| [roadmap.md](roadmap.md) | Evoluzione per release |
| [high-lev-arch.md](high-lev-arch.md) | Visione architetturale (lungo) |

## Diagrammi statici (SVG)

| File | Argomento |
|------|-----------|
| [high-lev-architecture.svg](high-lev-architecture.svg) | Architettura alto livello |
| [distillation-memory.svg](distillation-memory.svg) | Distillazione |
| [temporal-versioning.svg](temporal-versioning.svg) | Versioning temporale |
| [save-data_seq_diagram.svg](save-data_seq_diagram.svg) | Sequenza store |

I diagrammi **Mermaid** aggiornati sono nel README e in `architecture.md` / `USAGE.md` (render su GitHub).
