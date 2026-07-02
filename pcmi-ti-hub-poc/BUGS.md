# PCMI findings observed while building this PoC

The PoC does **not** modify PCMI source. These are issues/gotchas found while
driving PCMI from the outside, with the workaround the PoC uses.

---

## 1. `docker-compose.yml` initdb mount list is stale (misses migrations 020 & 021)  — *bug*

**What.** `postgres` and `postgres-age` mount migrations into
`/docker-entrypoint-initdb.d/` one file at a time, but the list stops at
`019_cognitive_graph_age.sql`. The repo ships two later migrations:

| Migration | Adds |
|-----------|------|
| `020_memory_open_version_unique.sql` | `UNIQUE INDEX uq_memory_entries_open_version` — one open version per `(tenant_id, path)`; closes a concurrent-store data-integrity race |
| `021_link_type_check.sql` | `CHECK (link_type IN (causal, temporal, contradicts, supports, related, duplicate))` — the documented link-type enum |

Because they are not mounted, a fresh `docker compose --profile graph up`
initialises the DB **without** the open-version uniqueness guard and **without**
the link-type CHECK constraint. A containerized deployment therefore silently
runs with weaker integrity than the migrations intend, and
`POST /v1/memories/links` accepts arbitrary `link_type` strings (the CHECK is the
DB-level backstop; `model.NormalizeLinkType` still guards the HTTP path).

**Impact.** Medium — data-integrity constraints the project explicitly added are
absent in the default container setup. Only the API/host-run path that applies
migrations via the migrate runner gets them.

**Fix suggestion.** Add the two files to both services' `volumes:` lists in
`docker-compose.yml` (and keep the list in sync with `migrations/` going
forward — a CI check comparing the directory to the mount list would prevent
recurrence).

**Status — fixed in [PR #169](https://github.com/marco-spagn/pcmi/pull/169)** (open, awaiting review):
mounts `020`/`021` in both services, **and** adds a regression guard
`TestEveryMigrationMountedInInitdbServices` in `internal/deploy/` that fails CI if
any `migrations/*.sql` is not mounted into both initdb services. Verified the
guard fails on the pre-fix compose and passes after.

**PoC workaround (still in place, harmless once #169 merges).** `scripts/run_e2e.sh`
applies `020` and `021` via `psql` after the container is healthy — idempotent, so
it is a no-op when compose already applied them.

---

## 2. `POST /v1/graph/cypher` passthrough only handles single-column `RETURN`  — *bug (experimental feature)*

The Cypher passthrough wraps the AGE call as `... AS (result agtype)` (a single
output column). Any query whose `RETURN` projects **more than one column** fails:

```
{"query":"MATCH (n:Memory)-[r]->(m) RETURN type(r), count(*)"}
→ ERROR: return row and column definition list do not match (SQLSTATE 42804)
```

and an `ORDER BY <alias>` over an aggregate also breaks
(`could not find rte for deg`, SQLSTATE 42703). Only single-column projections
work (e.g. `MATCH (n:Memory) RETURN count(n)` → `{"columns":["result"],"rows":[{"result":28}]}`
— note the `AS` alias is ignored, the column is always `result`). This makes the
endpoint unusable for the graph-analytics queries the docs imply. Likely fix:
derive the AS-column list from the query's `RETURN` arity, or document the
single-column restriction. EXPERIMENTAL spike feature.

**PoC impact.** The PoC only issues single-column Cypher (`count(n)`); richer
graph analytics use `/v1/graph/related` instead.

---

## 3. `RetrieveRequest` in `docs/openapi.yaml` omits `tags` / `tags_match`  — *doc gap*

`POST /v1/retrieve` accepts and honours `tags` + `tags_match` (verified:
`{"path_prefix":"root.cti.vendor_reports","tags":["threat-actor"],"tags_match":"any"}`
returns only tagged entries), and the Python SDK sends them, but the
`RetrieveRequest` schema in the OpenAPI spec does not list either field. Minor —
spec should document the tag filter.

---

## Notes (expected behaviour, not bugs — flagged for API consumers)

* **Consolidation worker is always on.** `cmd/worker` starts
  `ConsolidationWorker` unconditionally (no env switch). On each `memory.stored`
  event it may write derived `…consolidated` memories that carry **no `vendor`
  metadata**. Any analytics that count "which vendor said X" via `/v1/retrieve`
  must exclude these derived rows — this PoC filters to entries carrying both
  `metadata.vendor` and `metadata.stix_type` (`correlator.is_vendor_finding`).

* **`/v1/retrieve` query semantics change with embeddings.** With no embedding
  provider a non-empty `query` is a hard lexical filter (no match ⇒
  `entries: null`). With `OPENAI_API_KEY` set, it becomes a fused semantic + BM25
  ranker that returns up to `limit` nearest neighbours **regardless of lexical
  overlap**. Good for recall (this is what bridges vendor code-names), but "who
  documented TTP Y" must be re-confirmed lexically — see
  `correlator.ttp_correlations`.
