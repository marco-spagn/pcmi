# Cognitive Graph (v2.0 Spike)

> **Status: EXPERIMENTAL** — This is a v2.0 spike. The API and schema may change
> significantly before the full release. See [roadmap.md](roadmap.md) for the
> planned v2.0 timeline.

## What it is

The Cognitive Graph layer adds **graph traversal over `memory_links`** using
[Apache AGE](https://github.com/apache/age), a PostgreSQL extension that brings
Cypher query support to relational databases.

Instead of fetching only directly linked memories, you can answer questions like:

- *"Find all memories causally related to this one within 3 hops."*
- *"Which memories contradict each other through any chain of links?"*
- *"What is the temporal sequence leading to memory X?"*

Memories become nodes (`Memory` vertices) and `memory_links` become typed,
weighted edges in the `pcmi_memory_graph` AGE graph.

## How to enable

### 1. Start the AGE-enabled Postgres instance

The `docker/postgres-age/Dockerfile.postgres-age` builds a custom image on top of
`pgvector/pgvector:pg16` that bundles **both pgvector and Apache AGE** v1.5.0.
A `postgres-age` service is available under the `graph` profile:

```bash
docker compose --profile graph up postgres-age
```

This instance supports embedding AND graph features simultaneously — no
trade-offs. Point the API at the AGE instance:

```bash
export DATABASE_URL=postgres://pcmi:pcmi@localhost:5433/pcmi
```

### 2. Build the custom image (alternative)

```bash
docker build -f docker/postgres-age/Dockerfile.postgres-age -t pcmi-postgres-age .
docker run --name pcmi-pg-age \
  -e POSTGRES_DB=pcmi \
  -e POSTGRES_USER=pcmi \
  -e POSTGRES_PASSWORD=pcmi \
  -p 5433:5432 \
  pcmi-postgres-age
```

### 3. Apply migration 019

```bash
psql "$DATABASE_URL" -f migrations/019_cognitive_graph_age.sql
```

The migration wraps everything in a `DO $$ ... EXCEPTION WHEN ... END $$` block,
so it **degrades gracefully** if AGE is not installed — it emits a `NOTICE` and
continues without error.

### 4. Verify

`/v1/graph/health` is an unauthenticated probe (like `/v1/ready`). `/v1/graph/related`
requires a read-capable API key.

```bash
curl http://localhost:8000/v1/graph/health
# {"available":true,"extension":"apache-age"}
```

## Example Cypher query

Find all memories causally linked to memory with path `memory.42` within 2 hops:

```cypher
MATCH (m:Memory {id: 'memory.42'})-[r:causal*1..2]->(n:Memory)
RETURN n.id, type(r[0]), length(r)
```

Via the API:

```bash
curl "http://localhost:8000/v1/graph/related?memory_id=42&depth=2&link_types=causal"
```

Response:

```json
{
  "memory_id": 42,
  "depth": 2,
  "count": 3,
  "entries": [
    {"id": 7,  "link_type": "causal", "depth": 1},
    {"id": 15, "link_type": "causal", "depth": 1},
    {"id": 31, "link_type": "causal", "depth": 2}
  ]
}
```

## Link types and semantic meaning

| Constant            | Value          | Meaning                                                   |
|---------------------|----------------|-----------------------------------------------------------|
| `LinkTypeCausal`    | `causal`       | Memory A directly caused or enabled memory B              |
| `LinkTypeTemporal`  | `temporal`     | Memory A precedes B in time; B builds on A                |
| `LinkTypeContradicts` | `contradicts`| Memory A and B make incompatible claims                   |
| `LinkTypeSupports`  | `supports`     | Memory A provides evidence for the claim in B             |
| `LinkTypeRelated`   | `related`      | Weak semantic association; no specific causal direction   |

## Current API endpoints

| Method | Path                   | Auth        | Description                                              |
|--------|------------------------|-------------|----------------------------------------------------------|
| GET    | `/v1/graph/health`     | None        | Returns `{"available": bool, "extension": "apache-age"}` |
| GET    | `/v1/graph/related`    | Read        | Graph traversal from a memory node                       |
| GET    | `/v1/graph/chain`      | Read        | Shortest causal chain between two memories               |
| POST   | `/v1/graph/cypher`     | Write       | Passthrough for read-only Cypher queries                 |

Returns `501 Not Implemented` when AGE is not installed, with a `hint` field
pointing to this document.

### `GET /v1/graph/related`

| Parameter    | Default | Description                                                    |
|--------------|---------|----------------------------------------------------------------|
| `memory_id`  | —       | **Required.** `memory_entries.id` of the start node            |
| `depth`      | `3`     | Max hop depth (1–10)                                           |
| `link_types` | all     | Comma-separated subset of link type constants                  |
| `cursor`     | `0`     | Keyset pagination cursor (last memory ID from previous page)   |
| `limit`      | `50`    | Page size (1–200)                                              |

Response includes `total` (matching entries before pagination), `count` (entries
in this page), `next_cursor` (pass as `?cursor=` for the next page), and `limit`.
Keyset pagination via `memory_entries.id` is used internally — no `SKIP`/`OFFSET`
overhead.

### `GET /v1/graph/chain`

| Parameter    | Default  | Description                                                    |
|--------------|----------|----------------------------------------------------------------|
| `from`       | —        | **Required.** Source memory ID                                 |
| `to`         | —        | **Required.** Target memory ID                                 |
| `link_types` | all      | Comma-separated subset of link type constants                  |
| `max_depth`  | `10`     | Max number of hops to search (1–20)                            |

Response: `{"from_id": X, "to_id": Y, "connected": bool, "hops": N, "path": [...]}`.
When no path exists, `connected` is `false` and `path` is an empty array.

### `POST /v1/graph/cypher`

Requires write role. Request body:

```json
{"query": "MATCH (n:Memory) WHERE n.tenant_id = '...' RETURN n.id LIMIT 10"}
```

Only `MATCH` queries are allowed. Write keywords (`CREATE`, `DELETE`, `SET`,
`REMOVE`, `MERGE`, `DROP`, `CALL`, `LOAD`) are rejected. Tenant scoping is the
injected automatically — the `tenant_id` filter is added to the `WHERE` clause by extracting the `:Memory` node alias. Do NOT include `tenant_id` manually.

## Prometheus metrics

| Metric                                   | Type      | Description                          |
|------------------------------------------|-----------|--------------------------------------|
| `pcmi_graph_traversal_total`             | Counter   | Total graph traversal operations     |
| `pcmi_graph_traversal_duration_seconds`  | Histogram | Graph traversal duration (seconds)   |

Exposed at `GET /metrics` (requires `METRICS_SCRAPE_TOKEN`).

## Current limitations (spike)

- Vertex IDs are stored as string paths (`memory.{id}`), not direct FK references.
- The `MERGE` on relationships uses dynamic Cypher executed via `EXECUTE format(...)`,
  which has a small overhead per link insert.
- Graph queries have a configurable timeout (`GRAPH_QUERY_TIMEOUT_SECS`, default 30s)
  to prevent runaway traversals from blocking the connection pool.
- AGE sync to the graph is handled by a database trigger (`trg_memory_links_sync_graph`)
  on `memory_links`, which fires on INSERT and UPDATE (including `ON CONFLICT DO UPDATE`).

## What remains

- **LLM-based contradiction detection**: replace keyword heuristics with an
  LLM call for higher precision contradiction flagging.
- **Cypher query result streaming**: for very large result sets, stream rows
  incrementally rather than buffering the entire result in memory.

---

## How to test

### Unit tests (no database required)

```bash
# Graph client unit tests
go test ./internal/graph/ -v -count=1

# Graph handler tests (uses fake clients)
go test ./internal/handler/ -run "TestGraph|TestRegister.*Chain|TestRegister.*Cypher" -v -count=1

# Metrics registration
go test ./internal/metrics/ -v -count=1
```

### Integration / manual testing (requires AGE)

1. **Start AGE Postgres:**

   ```bash
   docker compose --profile graph up -d postgres-age
   ```

2. **Run the migration:**

   ```bash
   psql "postgres://pcmi:pcmi@localhost:5433/pcmi" \
     -f migrations/019_cognitive_graph_age.sql
   ```

3. **Start the API:**

   ```bash
   DATABASE_URL=postgres://pcmi:pcmi@localhost:5433/pcmi \
     go run ./cmd/api
   ```

4. **Health check:**

   ```bash
   curl http://localhost:8000/v1/graph/health
   # {"available":true,"extension":"apache-age"}
   ```

5. **Create some test data** — store a few memories and link them:

   ```bash
   API_KEY="your-api-key"
   BASE="http://localhost:8000/v1"

   # Store memories
   curl -s -H "Authorization: Bearer $API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"text":"The user reported a segfault in v2.3.1"}' \
     "$BASE/memories" | jq '.id'  # → 1

   curl -s -H "Authorization: Bearer $API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"text":"The segfault is caused by a null pointer in config.c:142"}' \
     "$BASE/memories" | jq '.id'  # → 2

   curl -s -H "Authorization: Bearer $API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"text":"Fixed null pointer check, deployed in v2.3.2"}' \
     "$BASE/memories" | jq '.id'  # → 3

   # Create causal links between them
   curl -s -H "Authorization: Bearer $API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"from_path":"memory.1","to_path":"memory.2","link_type":"causal"}' \
     "$BASE/memories/links"

   curl -s -H "Authorization: Bearer $API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"from_path":"memory.2","to_path":"memory.3","link_type":"causal"}' \
     "$BASE/memories/links"
   ```

6. **Test traversal (paginated):**

   ```bash
   curl -s -H "Authorization: Bearer $API_KEY" \
     "$BASE/graph/related?memory_id=1&depth=3&link_types=causal&limit=10" | jq .
   ```

   Expected: entries for memories 2 and 3, with depths 1 and 2.

7. **Test chain reconstruction:**

   ```bash
   curl -s -H "Authorization: Bearer $API_KEY" \
     "$BASE/graph/chain?from=1&to=3&link_types=causal&max_depth=5" | jq .
   ```

   Expected: `{"from_id":1,"to_id":3,"connected":true,"hops":2,"path":[...]}`

8. **Test Cypher passthrough:**

   ```bash
   curl -s -H "Authorization: Bearer $API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"query":"MATCH (n:Memory) RETURN n.id LIMIT 5"}' \
     "$BASE/graph/cypher" | jq .
   ```

   The Cypher endpoint requires a write-capable API key (non-readonly).

9. **Test graceful degradation (without AGE):**

   ```bash
   # Point at the default Postgres (no AGE) and restart the API.
   # Health reports available=false; /related, /chain, /cypher return 501.

   curl http://localhost:8000/v1/graph/health
   # {"available":false,"extension":"apache-age"}
   ```

### Full project test suite

```bash
# All unit and integration tests (no AGE required)
go test ./internal/... -count=1

# With coverage
go test ./internal/... -count=1 -coverprofile=coverage.out
go tool cover -func=coverage.out | grep graph
```
