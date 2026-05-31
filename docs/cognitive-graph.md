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

---

## SOC Incident Dataset (1000 nodes + 1333 links)

For realistic demos, the repo includes a **deterministic SOC incident dataset** that
models a real case-management queue. Every node is a triaged alert with coherent
entities, realistic dispositions, and full MITRE ATT&CK mapping.

### One-command launch

```bash
# Everything: start AGE infra → generate dataset → load → open UI
make graph-ui

# Full stack (API + Worker) instead of just DB + Redis
make graph-ui FULL_STACK=1

# Custom size
make graph-ui DATASET_SIZE=5000

# Or run the script directly
bash scripts/e2e/launch_graph_ui.sh
```

### Files

| File | Description |
|------|-------------|
| `generate_soc_dataset.py` | Generator (deterministic, seed 1337). `python3 generate_soc_dataset.py 5000` |
| `soc_incidents_nodes.csv` | 1000 alert/incident nodes (33 typed columns) |
| `soc_incidents_links.csv` | 1333 typed edges (causal 29%, temporal 29%, related 20%, supports 13%, contradicts 10%) |
| `load_to_pcmi.py` | Batch loader (resumable, `id_map.json` checkpoint) |
| `validate.py` | Integrity + coherence validator |
| `CASI_SPECIFICI.md` | Walkthrough of 5 real-world SOC patterns in the data |
| `DATA_DICTIONARY.md` | Full 33-column reference |

### Manual load

```bash
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123

# Smoke test (first 100)
python3 load_to_pcmi.py --limit 100

# Full load (Ctrl+C safe — resumable)
python3 load_to_pcmi.py --batch 50 --link-workers 16
```

---

## Graph UI — demo video

The **Cognitive Graph Explorer** is a single-page app served at **`GET /v1/graph/ui`**
(same origin as the API). It calls `/v1/graph/related`, `/chain`, and `/graph/memories`
with your API key and renders the result with [vis-network](https://visjs.github.io/vis-network/docs/network/).

### Watch the walkthrough (~90s)

[![Click to play full video with controls](assets/graph-ui-demo.gif)](https://github.com/marco-spagn/pcmi/blob/feat/pcmi-cognitive-graph-v3-spike/docs/assets/graph-ui-demo.mp4)

**[▶ Play full video on GitHub (90s, with controls)](https://github.com/marco-spagn/pcmi/blob/feat/pcmi-cognitive-graph-v3-spike/docs/assets/graph-ui-demo.mp4)**

| | |
|--|--|
| **In repo** | [docs/assets/graph-ui-demo.mp4](assets/graph-ui-demo.mp4) — playable in GitHub’s file viewer and locally after `git clone` |
| **Release** | [graph-ui-demo.mp4](https://github.com/marco-spagn/pcmi/releases/download/graph-ui-demo/graph-ui-demo.mp4) |
| **Preview** | [graph-ui-demo.gif](assets/graph-ui-demo.gif) — animated excerpt for README (GitHub cannot inline `<video>`) |
| **Covers** | AGE health, kill-chain traversal (memory 14), Tree/Radial layouts, inspector, Find Chain, five link types, Royal campaign (35), supports fan-out, related cross-campaign links, clusters, timeline |
| **Regenerate** | `node scripts/e2e/record_graph_ui_demo.mjs` then `gh release upload graph-ui-demo docs/assets/graph-ui-demo.mp4 --clobber` |

### Open the UI

```bash
make graph-ui    # infra + SOC dataset + prints URL
# then browse:
open http://localhost:8000/v1/graph/ui
```

Enter your **X-API-Key**, set **Memory ID** + **Depth**, choose **link types**, and press **Explore**.
Click a node for the inspector; select a second node and press **Find Chain** for a shortest causal path.

---

## Graph UI — which Memory IDs to explore

After loading the SOC dataset, open `http://localhost:8000/v1/graph/ui` and
try these specific explorations. Each Memory ID corresponds to a position in the
CSV (deterministic insertion order, seed 1337).

### 1. Kill chain traversal (`/related`) — the main feature

| Memory ID | Depth | Link Types | What you see |
|-----------|-------|------------|--------------|
| **14** | 5 | `causal, temporal` | **CAMP000004 (Conti)** — 9-stage kill chain: resource_dev → phishing → web_attack → valid_accounts → malware_exec → persistence → priv_escalation → defense_evasion → postmortem. Severity climbs from P3→P1. Dwell time spans 10 days. |
| **35** | 5 | `causal, temporal` | **CAMP000007 (Royal)** — 10-stage chain: resource_dev → persistence → priv_escalation → lateral_movement → exfiltration. Each stage shares the same user/host/IP — coherent narrative across hops. |
| **113** | 5 | `causal, temporal, supports` | **CAMP000021 (TA505)** — 10-stage chain including postmortem `supports` edges that fan back to every stage in the campaign. |

**What to observe:** Each hop shows `depth`, `link_type`, and the memory ID.
The graph renders the full chain as a directed graph. Switch between **force**,
**tree**, and **radial** layouts to compare.

### 2. Shortest-path chain reconstruction (`/chain`)

| From | To | Max Depth | Link Types | What you see |
|------|----|-----------|------------|--------------|
| **14** | **22** | 10 | `causal` | 8-hop causal path through CAMP000004 (resource_dev → postmortem). Response: `connected: true, hops: 8, path: [{from_id:14, to_id:15, ...}, ...]`. |
| **35** | **44** | 10 | `causal` | 9-hop path through CAMP000007. Verify it's the shortest path with `hops` = exactly the stage count. |
| **22** | **14** | 10 | `causal` | Reverse direction → `connected: false` (edges are directed, no back-links). |

**What to observe:** The response includes every hop with `from_id`, `to_id`,
`link_type`, and `hop_index`. The UI renders the chain as a highlighted path
through the graph.

### 3. Contradiction edges (`contradicts`)

Look for a node with disposition `false_positive` that `contradicts` a `true_positive`
node in the same campaign. The dataset generates ~18% of campaigns with an initial
FP assessment that later contradicts the confirmed TP.

**In the UI:** Select a campaign root (e.g., ID 14), set depth 5, and check the
edge colors. **Red edges = contradicts**. Hover/click to see the rationale
("falso positivo iniziale vs attività confermata").

### 4. Cross-campaign links (`related`) — same threat actor

Nodes from different campaigns that share the same threat actor are linked via
`related` edges. For example, two campaigns attributed to the same APT group
will have cross-links.

**In the UI:** Select a node with high out-degree, depth 4, to see how the graph
connects across campaign boundaries. These appear as **gray dashed edges**.

### 5. Alert storm cluster

Alert storms generate 8-40 nodes with the same `/24` subnet, mostly `duplicate`
disposition. The duplicates are linked `related` → a representative node and
also `contradicts` the duplicate relationship.

**In the UI:** Look for dense clusters of nodes connected by `related` and
`contradicts` edges — these are alert storms. Use the **cluster view** to see
them grouped by path prefix.

### 6. Postmortem support fan-out

Campaign postmortem/synthesis nodes (typically the last node in a campaign)
have `supports` edges pointing back to **every** stage in the kill chain.

**In the UI:** Select the last node of CAMP000004 (ID ~22), depth 1,
link_types `supports` — you'll see it fan out to all 8 stages.

### 7. Isolated nodes

~30% of nodes have no links at all (standalone alerts). These are visible in the
**Memories browser** (left panel) but won't appear in traversals unless you
search for them by ID or path.

### Quick curl examples

```bash
API_KEY=testkey123 BASE=http://localhost:8000/v1

# Traversal: full CAMP000004 chain from ID 14
curl -s -H "X-API-Key: $API_KEY" \
  "$BASE/graph/related?memory_id=14&depth=5&link_types=causal,temporal" | jq '{count,total,entries:[.entries[]|{id,link_type,depth}]}'

# Chain: shortest path through CAMP000004
curl -s -H "X-API-Key: $API_KEY" \
  "$BASE/graph/chain?from=14&to=22&link_types=causal&max_depth=10" | jq '{connected,hops,path:[.path[]|{from_id,to_id,link_type}]}'

# List memories to find other IDs
curl -s -H "X-API-Key: $API_KEY" \
  "$BASE/graph/memories?limit=10" | jq '.entries[] | {id, path, preview: .preview[:80]}'

# Cypher passthrough (write role)
curl -s -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"query": "MATCH (n:Memory) RETURN n.id ORDER BY n.id LIMIT 10"}' \
  "$BASE/graph/cypher" | jq '.rows[:3]'
```
