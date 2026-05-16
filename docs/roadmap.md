# PCMI evolution roadmap

## v1.18.0 (current)

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

## Near term

- Memory compaction policies beyond pruning
- gRPC batch + streaming retrieve
- OpenTelemetry traces alongside Prometheus metrics

## Long term

- Federated multi-region memory shards
- Policy engine for data residency
- On-device embedding spaces with server-side index merge
