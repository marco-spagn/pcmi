# Retrieval Pipeline

Hybrid retrieval in PCMI runs as a single SQL query in the repository layer (`internal/repository/memory_repository.go`), combining structural filtering, semantic ANN, lexical BM25, importance weighting, optional temporal decay, and weighted score fusion.

## Pipeline (Figure 7 — readable)

```mermaid
flowchart TB
  subgraph INPUT["Input"]
    direction TB
    I1["Query + path_prefix + limit + weights"]
    I2["tenant_id injected by middleware; optional"]
    I3["as_of, tags, source_agent_id, embedding_space, decay_enabled"]
  end

  subgraph S1["Stage 1 — Structural filter (ltree)"]
    direction TB
    S1A["WHERE tenant_id = $t AND"]
    S1B["path &lt;@ $prefix::ltree"]
    S1C["Reduces candidate set N → N' ≪ N"]
  end

  subgraph S2["Stage 2 — Semantic ANN (pgvector HNSW)"]
    direction TB
    S2A["1 - (v_q &lt;=&gt; v_m)"]
    S2B["cosine distance score"]
  end

  subgraph S3["Stage 3 — Lexical BM25 (tsvector)"]
    direction TB
    S3A["pcmi_bm25_rank(content_tsv, query)"]
    S3B["via @@ websearch_to_tsquery"]
    S3C["Critical for CVE IDs, ticket numbers"]
  end

  subgraph S4["Stage 4 — Importance + temporal decay"]
    direction TB
    S4I["W_i × importance (0–1, default 0.5)"]
    S4R["W_r × exp(-ln(2)/halflife × age_days)"]
    S4D["age from last_accessed_at or created_at"]
  end

  subgraph S5["Stage 5 — Fusion + temporal rank"]
    direction TB
    S5A["score = W_s×cosine + W_l×bm25 + W_i×importance + W_r×decay"]
    S5B["defaults W_s=0.40 W_l=0.30 W_i=0.15 W_r=0.15"]
    S5C["ORDER BY score DESC LIMIT k"]
    S5D["filter valid_to IS NULL (or as_of clause)"]
  end

  subgraph OUTPUT["Output"]
    direction TB
    O1["[]MemoryEntry ranked; access_count++"]
    O2["per-tenant overrides in tenant_memory_config"]
    O3["DATABASE_READ_URL replica for heavy reads"]
  end

  INPUT --> S1
  S1 --> S2
  S2 --> S3
  S3 --> S4
  S4 --> S5
  S5 --> OUTPUT
```

## Stages

| Stage | Mechanism | Role |
|-------|-----------|------|
| 1 | `ltree` prefix (`path <@ $prefix`) | Tenant-scoped structural filter; shrinks N → N' |
| 2 | pgvector HNSW ANN | Semantic similarity via cosine distance |
| 3 | `pcmi_bm25_rank` + `websearch_to_tsquery` | Lexical match for exact tokens (CVE IDs, tickets) |
| 4 | Importance + recency | Stored `importance`; decay from memory age (halflife days) |
| 5 | Weighted fusion + temporal clause | Four-term score; `valid_to IS NULL` or `as_of` |

## Score formula (v1.42+)

```
score = W_s × cosine + W_l × bm25 + W_i × importance + W_r × exp(-ln(2)/halflife × age_days)
```

- Default weights sum to **1.0** (`0.40 / 0.30 / 0.15 / 0.15`).
- Per-tenant overrides: table `tenant_memory_config`.
- `POST /v1/retrieve` with `"decay_enabled": false` omits the recency term (`W_r = 0`).
- `POST /v1/memories` accepts `"importance"` in `[0,1]` (default `0.5`).
- `PATCH /v1/memories/{path}/importance` updates the current row.
- Ranked retrieves increment `access_count` and `last_accessed_at`.

## Retrieve request fields

| JSON field | Effect |
|------------|--------|
| `importance` (on store) | Weight in fusion (0–1, default 0.5) |
| `decay_enabled` | `false` sets recency weight `W_r = 0` for this query |
| `weights` | Optional override of `W_s`, `W_l`, `W_i`, `W_r` (must sum to 1.0) |

After a ranked retrieve, matching rows get `access_count++` and `last_accessed_at` updated (feeds decay on later queries).

## Related

- Implementation: `internal/repository/memory_repository.go`, `internal/repository/retrieve_sql.go`
- Tests: `make test-retrieval-scoring`, `make bench-retrieval`
- Manual smoke (API up): `make smoke-importance` → `scripts/smoke_importance_retrieve.sh`
- Full local suite: `make test-full-real` includes this smoke in Phase 3
- Read replica: [federation-read-replicas.md](federation-read-replicas.md)
- Performance notes: [scalability.md](scalability.md)
