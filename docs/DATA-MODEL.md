# Modello dati PCMI

Schema logico per memorie, versioni, tenant e grafo.

## Entità principali

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

## Versioning append-only

```mermaid
stateDiagram-v2
  [*] --> V1: Store path P (prima volta)
  V1 --> V1_closed: Store path P again
  V1_closed --> V2: nuova riga
  V2 --> V2_current: valid_to IS NULL
  note right of V1_closed: valid_to = NOW()
```

- **Nessun UPDATE** sul contenuto storico: ogni store crea una nuova riga.
- **Versione corrente**: `valid_to IS NULL`.
- **Retrieve con `as_of`**: slice temporale (non solo corrente).
- **Rollback**: nuova riga che ripristina contenuto storico.

## Path gerarchici (`ltree`)

Esempi:

| Path | Significato |
|------|-------------|
| `root` | Radice tenant |
| `root.acme` | Namespace progetto |
| `root.acme.sprint42.notes` | Note sprint |

`path_prefix` in retrieve = operatore `<@` (discendente o uguale).

## Embedding

| Campo | Ruolo |
|-------|--------|
| `embedding` | `vector(1536)` — pgvector |
| `embedding_model` | Etichetta modello (es. `text-embedding-3-small`) |
| `embedding_space` | Spazio logico per migrazioni multi-modello |

Store può inviare vettore client (REST `embedding`, gRPC `repeated float embedding`) oppure lasciare `NULL` per backfill worker.

## Sicurezza multi-tenant

```mermaid
sequenceDiagram
  participant C as Client
  participant API as pcmi-api
  participant PG as PostgreSQL
  C->>API: X-API-Key
  API->>API: SHA-256 → tenant_id + role
  API->>PG: set_tenant_context(tenant_id)
  API->>PG: query con RLS
```

RLS su `memory_entries`, `api_keys`, webhook, ecc.

## Tabelle correlate

| Tabella | Uso |
|---------|-----|
| `memory_entries` | Memorie versionate |
| `distilled_knowledge` | Sintesi distillate |
| `memory_links` | Grafo tra path |
| `webhook_endpoints` / `webhook_deliveries` | Notifiche HTTP |
| `audit_log` | Audit richieste API |

SQL: cartella `migrations/` — ordine in [MIGRATIONS.md](MIGRATIONS.md).
