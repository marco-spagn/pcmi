# Retrieval Pipeline

Hybrid retrieval in PCMI runs as a single SQL query in the repository layer (`internal/repository/memory_repository.go`), combining structural filtering, semantic ANN, lexical BM25, and weighted score fusion.

## Pipeline (Figure 7 — readable)

```mermaid
flowchart TB
  subgraph INPUT["Input"]
    direction TB
    I1["Query + path_prefix + limit + weights"]
    I2["tenant_id injected by middleware; optional"]
    I3["as_of, tags, source_agent_id, embedding_space"]
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

  subgraph S4["Stage 4 — Fusion + temporal rank"]
    direction TB
    S4A["score = W_s × (1 - cos_dist) + W_l × bm25"]
    S4B["default W_s = 0.55, W_l = 0.45"]
    S4C["ORDER BY score DESC LIMIT k"]
    S4D["filter valid_to IS NULL (or as_of clause)"]
  end

  subgraph OUTPUT["Output"]
    direction TB
    O1["[]MemoryEntry ranked and deduplicated"]
    O2["weights configurable per-query; optional"]
    O3["DATABASE_READ_URL replica for heavy reads"]
  end

  subgraph REDUCTION["Data reduction"]
    direction TB
    R1["N records"]
    R2["N' filtered"]
    R3["k top-ranked"]
  end

  INPUT --> S1
  S1 --> S2
  S2 --> S3
  S3 --> S4
  S4 --> OUTPUT

  R1 --> R2
  R2 --> R3

  S1 -.-> R2
  S4 -.-> R3

  classDef input fill:#e8e0f4,stroke:#7c6aad,color:#1a1a1a
  classDef stage1 fill:#d6ebf7,stroke:#4a90b8,color:#1a1a1a
  classDef stage2 fill:#d9f0d9,stroke:#5a9e5a,color:#1a1a1a
  classDef stage3 fill:#fde8cc,stroke:#c4843a,color:#1a1a1a
  classDef stage4 fill:#f8d7da,stroke:#b85450,color:#1a1a1a
  classDef output fill:#d9f0d9,stroke:#5a9e5a,color:#1a1a1a
  classDef reduction fill:#f5f5f5,stroke:#888888,color:#1a1a1a

  class INPUT,I1,I2,I3 input
  class S1,S1A,S1B,S1C stage1
  class S2,S2A,S2B stage2
  class S3,S3A,S3B,S3C stage3
  class S4,S4A,S4B,S4C,S4D stage4
  class OUTPUT,O1,O2,O3 output
  class REDUCTION,R1,R2,R3 reduction
```

## Stages

| Stage | Mechanism | Role |
|-------|-----------|------|
| 1 | `ltree` prefix (`path <@ $prefix`) | Tenant-scoped structural filter; shrinks N → N' |
| 2 | pgvector HNSW ANN | Semantic similarity via cosine distance |
| 3 | `pcmi_bm25_rank` + `websearch_to_tsquery` | Lexical match for exact tokens (CVE IDs, tickets) |
| 4 | Weighted fusion + temporal clause | `0.55 × semantic + 0.45 × BM25`; `valid_to IS NULL` or `as_of` |

## Related

- Implementation: `internal/repository/memory_repository.go`, `internal/repository/retrieve_sql.go`
- Read replica: [federation-read-replicas.md](federation-read-replicas.md)
- Performance notes: [scalability.md](scalability.md)
