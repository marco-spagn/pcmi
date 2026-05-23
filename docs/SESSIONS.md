# Agent sessions and working memory

Sessions bound an agent run to a **working memory** scope. Rows in `memory_entries` carry
`metadata.session_id` and `metadata.memory_scope=working` until promoted to long-term storage.

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/sessions` | Create session → `{ id, status: "active", ... }` |
| `POST` | `/v1/sessions/{id}/memories` | Store working memory (path under `sessions.{id}`) |
| `GET` | `/v1/sessions/{id}/memories` | List session memories; `include_long_term=true` appends global rows **after** session rows |
| `POST` | `/v1/sessions/{id}/promote` | Move working rows to `target_prefix` (default `root`) and clear `session_id` |
| `DELETE` | `/v1/sessions/{id}` | End session (`ended_at`) |

## Schema

- `migrations/016_sessions.sql` — `agent_sessions` table + partial index on `metadata->>'session_id'`.
- RLS on `agent_sessions` matches other tenant-scoped tables.

## Esempio curl

```bash
export PCMI_BASE_URL=http://localhost:8000
export PCMI_API_KEY=testkey123

# Crea sessione
SID=$(curl -s -X POST "$PCMI_BASE_URL/v1/sessions" \
  -H "X-API-Key: $PCMI_API_KEY" -H "Content-Type: application/json" \
  -d '{"agent_id":"demo-agent"}' | jq -r '.id')

# Working memory nella sessione
curl -s -X POST "$PCMI_BASE_URL/v1/sessions/$SID/memories" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path":"note","content":"contesto temporaneo"}'

# Promuovi verso memoria globale (prefisso default root)
curl -s -X POST "$PCMI_BASE_URL/v1/sessions/$SID/promote" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"target_prefix":"root.demo"}'

# Chiudi sessione
curl -s -X DELETE "$PCMI_BASE_URL/v1/sessions/$SID" \
  -H "X-API-Key: $PCMI_API_KEY"
```

## Tests

```bash
make infra-up
make smoke-sessions              # script: scripts/smoke_sessions.sh
make test-sessions-integration   # requires DATABASE_URL + migration 016
```

Incluso in **`make test-full-real`** (Phase 3 + 4). Vedi [local-ci.md](local-ci.md).
