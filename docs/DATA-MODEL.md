# PCMI Data Model

Logical schema for memories, versions, tenants, and graph.

## Main entities

```mermaid
erDiagram
  tenants ||--o{ api_keys : has
  tenants ||--o{ memory_entries : owns
  tenants ||--o{ distilled_knowledge : owns
  tenants ||--o{ memory_links : owns
  memory_entries {
    bigint id PK
    uuid tenant_id FK
    ltree path
    text content
    jsonb metadata
    text[] tags
    vector embedding
    int version
    timestamptz valid_from
    timestamptz valid_to
    timestamptz expires_at
  }
  memory_links {
    bigint id PK
    ltree from_path
    ltree to_path
    text link_type
  }
```

## Append-only versioning

```mermaid
stateDiagram-v2
  [*] --> V1: Store path P (first time)
  V1 --> V1_closed: Store path P again
  V1_closed --> V2: new row
  V2 --> V2_current: valid_to IS NULL
  note right of V1_closed: valid_to = NOW()
```

- **No UPDATE** on historical content: every store creates a new row.
- **Current version**: `valid_to IS NULL`.
- **Retrieve with `as_of`**: temporal slice (not just current).
- **Rollback**: new row that restores historical content.

## Hierarchical paths (`ltree`)

Examples:

| Path | Meaning |
|------|---------|
| `root` | Tenant root |
| `root.acme` | Project namespace |
| `root.acme.sprint42.notes` | Sprint notes |

`path_prefix` in retrieve = operator `<@` (descendant or equal).

## Embedding

| Field | Role |
|-------|------|
| `embedding` | `vector(1536)` — pgvector |
| `embedding_model` | Model label (e.g. `text-embedding-3-small`) |
| `embedding_space` | Logical space for multi-model migrations |

Store can send a client vector (REST `embedding`, gRPC `repeated float embedding`) or leave `NULL` for worker backfill.

## Multi-tenant security

```mermaid
sequenceDiagram
  participant C as Client
  participant API as pcmi-api
  participant PG as PostgreSQL
  C->>API: X-API-Key
  API->>API: SHA-256 → tenant_id + role
  API->>PG: set_tenant_context(tenant_id)
  API->>PG: query with RLS
```

RLS on `memory_entries`, `api_keys`, webhooks, etc.

## Related tables

| Table | Use |
|-------|-----|
| `memory_entries` | Versioned memories |
| `distilled_knowledge` | Distilled summaries |
| `memory_links` | Typed edges between paths (`link_type` set by the client on create — see [cognitive-graph.md § How data enters PCMI](cognitive-graph.md#how-data-enters-pcmi--who-classifies-what)) |
| `webhook_endpoints` / `webhook_deliveries` | HTTP notifications |
| `audit_log` | API request audit |

SQL: `migrations/` directory — order in [MIGRATIONS.md](MIGRATIONS.md).
