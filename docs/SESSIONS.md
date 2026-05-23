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

## Tests

```bash
make test-sessions-integration   # requires DATABASE_URL + migration 016
```
