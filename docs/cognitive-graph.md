# Cognitive Graph (v3.0 Spike)

> **Status: EXPERIMENTAL** — This is a v3.0 spike. The API and schema may change
> significantly before the full release. See [roadmap.md](roadmap.md) for the
> planned v3.0 timeline.

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

The default `docker-compose.yml` uses `pgvector/pgvector:pg16` which does not
include AGE.  A separate `postgres-age` service is available under the `graph`
profile:

```bash
docker compose --profile graph up postgres-age
```

Point the API at the AGE instance:

```bash
export DATABASE_URL=postgres://pcmi:pcmi@localhost:5433/pcmi
```

### 2. Install AGE locally (alternative)

```bash
docker run --name pcmi-pg-age \
  -e POSTGRES_DB=pcmi \
  -e POSTGRES_USER=pcmi \
  -e POSTGRES_PASSWORD=pcmi \
  -p 5433:5432 \
  apache/age:latest
```

### 3. Apply migration 019

```bash
psql "$DATABASE_URL" -f migrations/019_cognitive_graph_age.sql
```

The migration wraps everything in a `DO $$ ... EXCEPTION WHEN ... END $$` block,
so it **degrades gracefully** if AGE is not installed — it emits a `NOTICE` and
continues without error.

### 4. Verify

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

| Method | Path                   | Description                                 |
|--------|------------------------|---------------------------------------------|
| GET    | `/v1/graph/health`     | Returns `{"available": bool, "extension": "apache-age"}` |
| GET    | `/v1/graph/related`    | Graph traversal from a memory node          |

### Query parameters for `/v1/graph/related`

| Parameter    | Default | Description                                         |
|--------------|---------|-----------------------------------------------------|
| `memory_id`  | —       | **Required.** `memory_entries.id` of the start node |
| `depth`      | `3`     | Max hop depth (1–10)                                |
| `link_types` | all     | Comma-separated subset of link type constants       |

Returns `501 Not Implemented` when AGE is not installed, with a `hint` field
pointing to this document.

## Current limitations (spike)

- Vertex IDs are stored as string paths (`memory.{id}`), not direct FK references.
- The `MERGE` on relationships uses dynamic Cypher executed via `EXECUTE format(...)`,
  which has a small overhead per link insert.
- No pagination on graph results.
- No write endpoint — links must be created via `POST /v1/links` or `GraphClient.CreateLink`.
- The trigger is `AFTER INSERT` only; `ON CONFLICT DO UPDATE` (upsert) does not
  re-fire the trigger, so `CreateLink` calls `sync_memory_link_to_graph` explicitly.

## What v3.0 will add

- Full Cypher passthrough endpoint: `POST /v1/graph/cypher` with tenant scoping.
- **Contradiction detection**: surface `contradicts` chains automatically during
  memory ingestion via a background worker event.
- **Causal chain reconstruction**: `GET /v1/graph/chain?from=X&to=Y` returning
  the shortest causal path.
- AGE bundled in the default Docker image (replacing `pgvector/pgvector:pg16`).
- Pagination and streaming for large graph results.
- Metrics: `pcmi_graph_traversal_total`, `pcmi_graph_traversal_duration_seconds`.
