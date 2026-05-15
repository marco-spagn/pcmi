# PCMI evolution roadmap

## v1.15.0 (current)

- Explicit distillation trigger (`POST /v1/memories/refine`)
- Memory lineage and distilled-to-source traceability
- Cross-memory links graph
- Tenant stats dashboard API
- Tag-based retrieval filters
- TTL (`expires_at`) with append-only expiry

## Near term

- Celery/Temporal adapter samples (external orchestrators calling PCMI HTTP)
- Cross-tenant federation read replicas
- Memory compaction policies beyond pruning
- gRPC batch + streaming retrieve
- OpenTelemetry traces alongside Prometheus metrics

## Long term

- Federated multi-region memory shards
- Policy engine for data residency
- On-device embedding spaces with server-side index merge
