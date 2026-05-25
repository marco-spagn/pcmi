# Runbook: Retrieve P99 Latency High

## Symptoms
- Alert `PCMISLORetrievep99High` fires: P99 retrieve latency > 90ms for 5+ minutes.
- Users report slow memory lookup responses.
- `pcmi_http_request_duration_seconds_bucket{handler="retrieve"}` shows elevated counts in upper buckets.

## Probable causes
1. **Vector similarity scan without index** — pgvector HNSW index missing or not being used; query falls back to sequential scan.
2. **Large corpus per tenant** — tenant has > 500k entries and a broad `path_prefix`, causing excessive ltree traversal.
3. **High `limit` value** — requests using `limit` well above 20 cause larger result sets to be ranked and serialised.
4. **Cold pgvector cache** — after a DB restart, HNSW index pages are evicted from shared_buffers, first queries are slow.
5. **Missing composite index** — `(tenant_id, path, valid_to)` index not present or not chosen by the planner.

## Investigation steps

Check current P99 over the last 15 minutes:
```promql
histogram_quantile(0.99,
  sum by (le) (rate(pcmi_http_request_duration_seconds_bucket{handler="retrieve"}[15m]))
)
```

Confirm index usage on retrieve query:
```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, content, metadata
FROM memory_entries
WHERE tenant_id = '<uuid>'::uuid
  AND path <@ '<prefix>'::ltree
  AND valid_to IS NULL
ORDER BY embedding <=> '<vector>'
LIMIT 20;
```

Check HNSW index presence:
```sql
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = 'memory_entries' AND indexdef ILIKE '%hnsw%';
```

Check row counts per tenant:
```sql
SELECT tenant_id, count(*) AS entries
FROM memory_entries
WHERE valid_to IS NULL
GROUP BY tenant_id
ORDER BY entries DESC
LIMIT 10;
```

Check slow query log:
```promql
histogram_quantile(0.99,
  sum by (le) (rate(pcmi_db_query_duration_seconds_bucket{query="retrieve"}[5m]))
)
```

## Remediation steps
1. **Vector index missing**: create HNSW index — `CREATE INDEX CONCURRENTLY ON memory_entries USING hnsw (embedding vector_cosine_ops);`
2. **Large corpus**: advise tenant to use more specific `path_prefix`; consider per-tenant corpus limits.
3. **High limit**: enforce `limit ≤ 20` at the API layer (already in spec); check if a client is violating this.
4. **Cold cache**: warm the index with a background query after DB restart; increase `shared_buffers` in Postgres config.
5. **Planner choice**: run `ANALYZE memory_entries;` to refresh statistics; add `pg_hint_plan` if planner consistently picks wrong index.

## Escalation path
1. On-call engineer: confirm HNSW index exists and query plan is correct.
2. DB admin: run `ANALYZE` and investigate planner statistics if index exists but is not used.
3. Platform team: if latency remains elevated after index work, escalate to DB tier upgrade (more RAM for shared_buffers).
