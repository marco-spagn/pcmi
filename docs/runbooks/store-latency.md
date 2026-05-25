# Runbook: Store P99 Latency High

## Symptoms
- Alert `PCMISLOStorep99High` fires: P99 store latency > 45ms for 5+ minutes.
- Users report slow memory save operations.
- `pcmi_http_request_duration_seconds_bucket{handler="store"}` shows elevated counts in upper buckets.

## Probable causes
1. **PgBouncer pool exhausted** — all 20 pool connections busy; new requests queue before acquiring a connection.
2. **Postgres index bloat** — `memory_entries` ltree or tenant_id indexes have high dead-tuple ratio after bulk deletes/expiry.
3. **Embedding worker contention** — store handler blocks waiting for pgvector write if the embedding pipeline is behind.
4. **High GC pressure on the API pod** — Go GC pauses spiking under sustained load due to large allocations per request.
5. **Slow disk I/O on the DB host** — WAL write latency elevated, seen as slow `INSERT` durations in `pg_stat_activity`.

## Investigation steps

Check current P99 over the last 15 minutes:
```promql
histogram_quantile(0.99,
  sum by (le) (rate(pcmi_http_request_duration_seconds_bucket{handler="store"}[15m]))
)
```

Check PgBouncer pool utilisation:
```promql
pcmi_pgbouncer_pool_used / pcmi_pgbouncer_pool_size
```

Identify slow queries in Postgres:
```sql
SELECT pid, now() - pg_stat_activity.query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active' AND query_start < now() - interval '1 second'
ORDER BY duration DESC;
```

Check table bloat:
```sql
SELECT schemaname, tablename, n_dead_tup, n_live_tup,
       round(100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0), 1) AS dead_pct
FROM pg_stat_user_tables
WHERE tablename = 'memory_entries';
```

Check Go GC pause metrics (if exposed via OTEL):
```promql
go_gc_duration_seconds{quantile="0.99"}
```

## Remediation steps
1. **Pool exhausted**: increase `PGBOUNCER_POOL_SIZE` in `deploy/helm/pcmi/values.yaml` from 20 → 30, apply with `helm upgrade`.
2. **Index bloat**: run `VACUUM ANALYZE memory_entries;` (online, no lock). Schedule autovacuum more aggressively if recurrent.
3. **Embedding contention**: scale the embedding worker horizontally or reduce `EMBEDDING_CONCURRENCY`.
4. **GC pressure**: reduce `GOMAXPROCS` or add a second API pod to distribute load.
5. **Disk I/O**: check cloud provider metrics for IOPS throttling; consider upgrading the DB storage tier.

## Escalation path
1. On-call engineer: verify PgBouncer pool and restart if stuck connections found.
2. DB admin: vacuum / index rebuild if bloat > 30%.
3. Platform team: if P99 remains > 45ms after remediation, page the infrastructure squad for DB tier upgrade review.
