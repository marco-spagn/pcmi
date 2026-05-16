# PCMI evolution roadmap

## v1.27.0 (current)

- **SDK HTTP-only**: `sdk/HTTP-API.md` — tabella mapping endpoint → metodi Python/TypeScript; client allineati (store/retrieve completi, `compact`, webhooks, `migrate_embeddings`, `list_links`, …).
- **Polish SDK**: TypeScript `StoreOptions` / `RetrieveOptions` (`embedding`, `tags`, `encrypt_content`, `expires_at`, `tags_match`); Python `async with`, stessi campi e metodi mancanti.
- Versione API `v1.27.0`.

## v1.26.0

- **Parità risposta retrieve gRPC ↔ REST**: `RetrieveEntry` espone `tenant_id`, `metadata_json`, `tags`, `embedding_model` / `embedding_space`, timestamp RFC3339 (`valid_from`, `valid_to`, `created_at`), `source_agent_id` / `source_event_id`, `content_encrypted`, `embedding` (se presente). Vale per `Retrieve`, `BatchRetrieve`, `RetrieveStream`.
- Versione API `v1.26.0`.

## v1.25.0

- **Embedding vettoriale su gRPC store**: `StoreRequest` / `BatchStoreItem` con `repeated float embedding` (parità con REST `embedding`); worker backfill se assente.
- **Documentazione API solo-HTTP**: `docs/grpc-vs-http.md` (compact, refine, links, stats, admin, webhooks, SSE, …).
- Versione API `v1.25.0`.

## v1.24.0

- **Parità store gRPC ↔ REST**: `StoreRequest` / `BatchStoreItem` con `tags`, `embedding_model`, `embedding_space`, `source_agent_id`, `encrypt_content`, `expires_at_rfc3339`; `StoreResponse` con `superseded_id` opzionale. Test integrazione gRPC (`go test -tags=integration`) e smoke store+retrieve con tag.
- Versione API `v1.24.0`.

## v1.23.0

- **Osservabilità worker**: `cmd/worker` inizializza `telemetry.Init` (OTLP/HTTP opzionale, default `service.name=pcmi-worker`), shutdown ordinato; span **consumer** per ogni messaggio Redis `memory_events`; **`GET :8081/metrics`** Prometheus (`WorkerRegistry`, `pcmi_worker_redis_events_total`). `telemetry.Init` accetta un `defaultServiceName` (`pcmi-api` vs `pcmi-worker`).
- Versione API `v1.23.0`.

## v1.22.0

- **Parità retrieve gRPC ↔ REST**: `RetrieveRequest` e `BatchRetrieveQuery` espongono gli stessi filtri del body JSON REST: `tags`, `tags_match`, `as_of_rfc3339` (RFC3339/RFC3339Nano), `source_agent_id`, `embedding_space`. Valori `as_of` non validi → `InvalidArgument`.
- Versione API `v1.22.0`.

## v1.21.0

- **gRPC batch store**: `BatchStore` RPC (stessi limiti di `POST /v1/memories/batch`, max 50 item). Chiavi **readonly** ricevono `PermissionDenied` su `Store` e `BatchStore`, allineato a REST.
- **Versione centralizzata**: `internal/version` (`Tag` / `Semver`) per health REST/gRPC, worker e smoke CI.
- Versione API `v1.21.0`.

## v1.20.0

- **gRPC retrieve**: `BatchRetrieve` (fino a 20 query, stessi limiti di `POST /v1/retrieve/batch`) e `RetrieveStream` (header con `total` poi un messaggio per entry, stessi dati di `Retrieve`).
- **OpenTelemetry**: propagazione W3C (`tracecontext` + `baggage`); export OTLP/HTTP quando `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` o `OTEL_EXPORTER_OTLP_ENDPOINT` è valorizzato; altrimenti tracer noop. **Prometheus** resta su `GET /metrics` (registry dedicato). HTTP instrumentato con `otelfiber`; gRPC con `otelgrpc` (`grpc.StatsHandler`).
- Versione API `v1.20.0`.

## v1.19.0

- **Compaction per path**: `POST /v1/memories/compact` + SQL `compact_memory_path_history` — rimuove versioni **chiuse** (`valid_to` valorizzato) oltre le ultime `keep_superseded` per un path; la riga corrente resta intatta. Complementare al **pruning** globale per età (`prune_superseded_memories`).
- **CI**: smoke HTTP unificato in `scripts/ci_integration_smoke.sh`; job `go` e `golangci-lint` in parallelo; E2E compose alleggerito (niente duplicati `ci_e2e_v1_14` / `v1_15` / `ci_e2e_embedding` già coperti dallo smoke).
- Versione API `v1.19.0`.

## v1.18.0

- **Esempi orchestratori**: `examples/celery` e `examples/temporal` — task/activity HTTP verso PCMI.
- **Read replica opzionale**: `DATABASE_READ_URL` instrada SELECT pesanti (retrieve, stats, lineage, …) su una streaming replica; federazione multi-tenant invariata (stesso cluster PG). Dettaglio in `docs/federation-read-replicas.md`.
- Versione API `v1.18.0` allineata su health REST/gRPC, worker e CI.

## v1.17.0

- **Readiness**: `GET /ready`, `GET /v1/ready` (HTTP) e RPC `pcmi.v1.MemoryService/Ready` — ping PostgreSQL + Redis; 503 / `not_ready` se una dipendenza non risponde. Kubernetes sample usa `/v1/ready` per `readinessProbe`.
- Versione API `v1.17.0` allineata su health REST/gRPC, worker e CI.

## v1.16.0

- Documentazione centralizzata: `docs/CODEBASE.md`, `docs/MIGRATIONS.md`, Godoc (`internal/*/doc.go`), `sdk/README.md`
- Indice documentazione in README e allineamento versione API `v1.16.0`

## v1.15.0

- Explicit distillation trigger (`POST /v1/memories/refine`)
- Memory lineage and distilled-to-source traceability
- Cross-memory links graph
- Tenant stats dashboard API
- Tag-based retrieval filters
- TTL (`expires_at`) with append-only expiry

## Long term

- Federated multi-region memory shards
- Policy engine for data residency
- On-device embedding spaces with server-side index merge
