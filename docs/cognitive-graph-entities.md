# Cognitive Graph — entity layer (design proposal)

> **Status: Phase A–C implemented** — tenant extraction profiles, `:Entity` vertices,
> and an LLM link proposal review queue. Accepting a proposal materializes
> `memory_links`; nothing is auto-applied without review.

## Problem statement

Today, cross-incident questions like *“show every alert involving `10.0.4.22`”*
or *“which campaigns share the same affected user?”* require either:

1. Full-text / metadata filters you wrote by hand at ingest time, or
2. Traversing **memory-to-memory** links you already created.

The SOC demo dataset puts `src_ip`, `dst_host`, `affected_user`, etc. in CSV
columns and JSON **metadata**, but the AGE graph does **not** model them as
nodes. The reviewer observation is correct: the live graph is mostly **paths and
typed links between memories**.

## Goal

For tenant **T** (e.g. customer **Acme**), given **N** ingested records (SOC
incidents today; CRM cases, lab notes, CTI findings tomorrow):

1. **Decompose** each record into a **tenant-defined attribute schema** (slots).
2. **Materialize** high-value attributes as **entity vertices** in AGE (optional
   but recommended for traversal).
3. **Propose** memory↔entity and entity↔entity edges via an LLM pipeline —
   **never silently overwrite** human/agent-authored links.
4. **Evolve** safely: new memory versions re-run extraction; entity identity and
   links update with audit + confidence, aligned with PCMI append-only versioning.

## What already exists (reuse, do not duplicate)

| Capability | Location | Reuse for entity layer |
|------------|----------|------------------------|
| Memory store + versioning + `as_of` | `memory_entries`, lineage API | Source of truth; re-extract on new version |
| Typed memory↔memory links | `memory_links` + AGE sync (019) | Keep; add **proposed_by=llm** in metadata |
| Distillation / refine | worker, `POST /v1/memories/refine` | Batch synthesis across paths — complementary |
| Summarize (LLM) | `POST /v1/memories/summarize` | Pattern for LLM calls + truncation |
| Tenant isolation | `tenant_id` everywhere | Entity vertices scoped the same way |
| Graph traversal | `/v1/graph/related`, `/chain`, Cypher | Extend with `:Entity` label queries |

## Core concepts

### 1. Domain profile (per tenant, not global SOC)

Attributes are **not** hard-coded to SIEM fields. Each tenant defines a
**domain profile** — a JSON schema of **slots** the extractor must fill.

Example profile id: `soc.siem.v1` (Acme SOC) vs `crm.support.v1` (another tenant).

```json
{
  "profile_id": "soc.siem.v1",
  "version": 1,
  "required_slots": [
    {"name": "record_kind", "type": "enum", "values": ["incident", "alert", "evidence"]},
    {"name": "severity", "type": "enum", "values": ["P1", "P2", "P3", "P4"]},
    {"name": "src_ip", "type": "ip", "nullable": true},
    {"name": "dst_host", "type": "hostname", "nullable": true},
    {"name": "affected_user", "type": "string", "nullable": true},
    {"name": "threat_actor", "type": "string", "nullable": true},
    {"name": "mitre_technique", "type": "string", "nullable": true}
  ],
  "entity_promotion": {
    "src_ip": {"vertex_label": "IPAddress", "normalize": "trim"},
    "dst_host": {"vertex_label": "Asset", "normalize": "lower"},
    "affected_user": {"vertex_label": "User", "normalize": "lower"},
    "threat_actor": {"vertex_label": "ThreatActor", "normalize": "alias_table"}
  }
}
```

**“Always present”** means: every slot exists in the output object (`null` if
unknown), not that every slot has a non-null value. Validation is schema-based,
not prose-based.

### 2. Extraction record (per memory version)

Stored alongside the memory (suggested: `memory_entries.metadata.pcmi_extract`
or dedicated table `memory_extractions` in a future migration).

```json
{
  "profile_id": "soc.siem.v1",
  "profile_version": 1,
  "memory_id": 4242,
  "memory_version": 3,
  "extracted_at": "2026-07-02T10:00:00Z",
  "model": "gpt-4.1-mini",
  "confidence": 0.86,
  "slots": {
    "record_kind": "incident",
    "severity": "P2",
    "src_ip": "10.0.4.22",
    "dst_host": "MAIL-684",
    "affected_user": "stefano.costa",
    "threat_actor": "Akira",
    "mitre_technique": "T1567.002"
  },
  "evidence_spans": [
    {"slot": "src_ip", "quote": "origine 10.0.4.22", "start": 42, "end": 54}
  ]
}
```

Grounding (`evidence_spans`) is required for SOC/audit use cases; optional for
low-risk domains.

### 3. Entity vertices (AGE)

Promoted slots become vertices, e.g.:

```cypher
(:Entity {kind: "IPAddress", key: "10.0.4.22", tenant_id: "..."})
(:Entity {kind: "Asset", key: "mail-684", tenant_id: "..."})
(:Memory {id: "memory.4242", tenant_id: "..."})-[:mentions {slot: "src_ip", confidence: 0.86}]->(:Entity ...)
```

**Canonical key** (`key`) is normalized per profile rules — this is lightweight
entity resolution, not a full STIX Constellation.

Cross-memory correlation becomes:

```cypher
MATCH (m1:Memory)-[:mentions]->(e:Entity {kind: "IPAddress", key: "10.0.4.22"})
MATCH (m2:Memory)-[:mentions]->(e)
RETURN m1.id, m2.id
```

### 4. LLM responsibilities (bounded)

| Task | LLM? | Auto-apply? |
|------|------|-------------|
| Fill profile slots from `content` + existing metadata | Yes | Yes → `metadata.pcmi_extract` |
| Promote slots to `:Entity` vertices | No (deterministic) | Yes, when extraction validates |
| Propose `memory_links` (causal/temporal/…) | Yes | **No** — write to `graph_link_proposals` or `memory_links.metadata.proposed=true` |
| Propose `entity↔entity` alias edges | Yes | **No** until confirmed |
| Supersede extraction on new memory version | Worker job | Yes — new row, old kept for audit |

The graph engine **reads** edges; it does **not** infer dispositions or MITRE
labels without an explicit extraction + promotion step (same principle as today).

### 5. Memory evolution

When `POST /v1/memories` creates version **v+1** at the same path:

1. Worker enqueues `memory.extract` (new event type).
2. LLM re-runs against profile **v** (or tenant’s current profile).
3. New extraction row references `(memory_id, memory_version)`.
4. Entity `mentions` edges are **reconciled** (add new, mark stale old with
   `valid_to` on edge properties or separate `entity_mentions` table).
5. Optional: distillation/refine consumes extractions instead of raw prose.

This aligns “evolving memory” with PCMI’s append-only model — not mutating history.

## Example walkthrough (tenant Acme, SOC profile)

```text
Tenant: acme-corp (uuid)
Profile: soc.siem.v1
Paths: root.clients.acme.soc.2026.inc_*

INC-001 stored → extract → src_ip=10.0.4.22, user=stefano.costa, actor=Akira
INC-002 stored → extract → src_ip=10.0.4.22, user=stefano.costa, actor=Akira
LLM proposal: INC-001 -[related {reason: shared src_ip + user}]-> INC-002
Analyst/agent: POST /v1/memories/links (accept) OR reject

Graph query: "all incidents touching stefano.costa" → traverse User entity
```

Same profile machinery works for non-SOC:

```text
Profile: crm.support.v1
Slots: customer_id, product, sentiment, ticket_priority, ...
```

## API sketch (future)

| Method | Path | Purpose |
|--------|------|---------|
| PUT | `/v1/tenants/{id}/extraction-profiles/{profile_id}` | Admin: define slots + promotion |
| GET | `/v1/memories/{id}/extraction` | Read latest (or `?version=`) extraction |
| POST | `/v1/memories/{id}/extract` | Force re-extract (admin/write) |
| GET | `/v1/graph/entities/related` | Traverse from entity key or memory via entities |
| GET | `/v1/graph/link-proposals` | List pending LLM link proposals |
| POST | `/v1/graph/link-proposals/{id}/accept` | Materialize to `memory_links` |

OpenAPI + SDK updates follow the usual four-place rule when implemented.

## Implementation phases

### Phase A — Metadata-only (low risk) ✅

- Domain profiles in `extraction_profiles` table.
- Worker: on `memory.stored` / `memory.updated`, call LLM → validate JSON against profile → write
  `metadata.pcmi_extract` (when `EXTRACTION_ENABLED=true`).
- HTTP: `GET/PUT/DELETE /v1/extraction-profiles/*`, `GET/POST /v1/memories/extraction/:id`.
- No AGE schema change; retrieve filters on `metadata->'pcmi_extract'->'slots'`.

### Phase B — Entity vertices in AGE ✅

- Migration `023_entity_graph.sql`: `:Entity` label, `reconcile_entity_mentions_for_memory`
  SQL helper (mirror 019 graceful-degradation pattern).
- Worker/API: after successful extraction, promoted slots sync to AGE via
  `entity_promotion` rules in the profile.
- `GET /v1/graph/entities/memory?memory_id=`, `GET /v1/graph/entities/related`
  (`kind`+`key` or `memory_id` for shared-entity correlation).

### Phase C — LLM link proposals + review ✅

- Migration `024_graph_link_proposals.sql`: pending proposal queue with partial unique index.
- Worker/API: after successful extraction, LLM proposes `memory_links` candidates
  (requires `LINK_PROPOSALS_ENABLED=true`, AGE, and entity-correlated memories).
- `GET /v1/graph/link-proposals`, `POST .../generate/:memory_id`,
  `POST .../:id/accept`, `POST .../:id/reject` — accept materializes to `memory_links`
  with `metadata.proposed_by=llm`.

## Risks and mitigations

| Risk | Mitigation |
|------|------------|
| LLM hallucinates IPs/users | JSON schema + regex validators per slot type; evidence spans; confidence threshold |
| “Always present” interpreted as always non-null | Document `nullable`; validate keys exist, allow `null` |
| Duplicate entities (`APT29` vs `Cozy Bear`) | Optional alias table per tenant; Phase C LLM proposals, human confirm |
| Graph drift vs SQL | Same trigger/worker reconciliation tests as 019 stale-edge cleanup |
| Cost / latency | Async worker only; batch extract; cache by `(content_hash, profile_version)` |
| Cross-domain tenants | Profile per path prefix or tenant default; never one global SOC schema |

## Honest comparison

| | Current spike | This proposal | Neo4j / STIX Constellation |
|--|---------------|---------------|----------------------------|
| Node identity | `memory.<id>` | `memory.<id>` + `:Entity` | Canonical STIX objects |
| Who creates links | Client API | Client + **approved** LLM proposals | Pipeline + dedup rules |
| Entity resolution | None | Normalized keys + optional aliases | Full graph merge |
| Temporal “as_of” | Memory versions | Memory versions + extraction versions | Varies |

PCMI should **not** claim parity with a dedicated knowledge graph. It should
claim: **versioned memory + typed links + optional extracted entities**, with
LLM as **assistant**, not oracle.

## Related docs

- [cognitive-graph.md](cognitive-graph.md) — current AGE spike (memory graph)
- [DATA-MODEL.md](DATA-MODEL.md) — versioning, RLS, metadata
- [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md) — worker event pipeline
- [examples/soc-incident-graph/data-dictionary.md](../examples/soc-incident-graph/data-dictionary.md) — SOC column reference for `soc.siem.v1` profile

## Open questions

1. Store extractions in `metadata` vs dedicated table (query performance)?
2. Should entity promotion be synchronous on store or worker-only?
3. gRPC parity timing for extract + entity graph endpoints?
4. MCP tools: `extract_memory`, `list_link_proposals`, `accept_link_proposal`?
