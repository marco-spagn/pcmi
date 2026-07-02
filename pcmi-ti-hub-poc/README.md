# PCMI × TI Mindmap HUB — Proof of Concept

Demonstrates **[PCMI](../README.md)** as a **complementary** memory layer for
AI-powered threat intelligence — the kind of workflow
[TI Mindmap HUB](https://medium.com/ti-mindmap-hub-research/introducing-ti-mindmap-hub-an-open-research-platform-for-ai-powered-threat-intelligence-4592faddf96c)
runs. The PoC ingests TI Mindmap HUB's **STIX 2.1** output (`TI_HUB_MODE=live`) and
adds what the HUB's *STIX Constellation* does **not**: versioned `as_of` history,
agent working-memory, and enterprise controls. It does **not** claim to add the
cross-report correlation / knowledge graph the HUB already ships — see
[Relationship to TI Mindmap HUB](#relationship-to-ti-mindmap-hub-complementary-not-a-reimplementation)
for the honest breakdown.

It ingests **real 2025–2026 vendor CTI** — Mandiant M-Trends 2026, CrowdStrike
GTR 2026, Unit 42 GIRR 2026, Microsoft Threat Intelligence — into PCMI memories
with typed links in the **Cognitive Graph** (Apache AGE), and exercises PCMI
end-to-end across these phases:

1. **Pre-ingest retrieval** — before processing a new vendor report, query PCMI
   for prior context on the same actors/TTPs.
2. **Longitudinal graph** — typed cross-report links, walked with multi-hop
   traversal, shortest-path chains, and read-only Cypher.
3. **Cross-vendor correlation** — the same actor/TTP documented by different
   vendors, found automatically. The headline: PCMI's **LLM-embedding semantic
   retrieval** links CrowdStrike's **“COZY BEAR”** to Microsoft's **“Midnight
   Blizzard”** (both APT29) — a correlation keyword search cannot make because the
   two vendors never use the same code-name in prose.
4. **Deterministic LLM-style distillation (Phase 3c)** — *not* an external LLM call.
   Heuristic synthesis scans sparse signals across ingested reports (aliases,
   attribution, targets, tools, platforms and TTPs), creates an evidence object
   with confidence/explanation, then writes it back to PCMI as reusable memory
   under `root.cti.distillation` plus compatible graph links.
5. **LLM inference distillation (Phase 3d)** — *real* external LLM call when
   `OPENAI_API_KEY` is set (fixture offline otherwise). Sends raw report text +
   PCMI retrieval context to the model, validates JSON candidates (schema +
   evidence grounding), and persists accepted correlations under
   `root.cti.distillation.llm.*`. Demo examples include:
   - **PROMPTSTEAL ↔ Forest Blizzard** as `malware_linked_to_actor` via
     `APT28/FROZENLAKE` alias evidence.
   - **PRESSURE CHOLLIMA ↔ Sapphire Sleet** as `campaign_actor_overlap` via
     DPRK/North Korea attribution, cryptocurrency targeting and macOS execution
     signals. This is deliberately *not* claimed as a confirmed same actor.
   - **COZY BEAR ↔ Midnight Blizzard** as `alias_equivalent` via APT29/Cozy Bear.
6. **Temporal memory (`as_of`)** — a follow-up report updates an actor profile;
   PCMI's append-only versioning answers *“what did we know before the update?”*
   as a query parameter (the prior version is retained, not overwritten).
7. **Knowledge accumulation** — the correlations PCMI discovers are written back
   into the graph as typed `related` edges: ones the analyst had already curated
   are *validated* (independently rediscovered), and new ones are *added*, so the
   memory graph grows from its own analysis.
8. **Observability & audit** — the run subscribes to PCMI's live **SSE event
   stream** (`memory.stored` / `memory.updated`) and reads the per-request
   **audit log** (`/v1/audit`): every mutation is streamable for a SOC dashboard,
   every request is auditable for compliance.
9. **Analyst investigation (agent sessions)** — an investigation session
   accumulates ephemeral working memory, then *promotes* the dossier to long-term
   (`/v1/sessions/*`).
10. **CTI brief artifact** — the whole run is assembled into a Markdown brief on
    disk (`reports/cti_brief_<ts>.md`).

```
CTI dataset (40 findings, 25 links, 4 vendors)
        │  ti_hub_client → pseudo-STIX reports (grouped by vendor)
        ▼
  stix_to_pcmi ──▶  POST /v1/memories        (SDO  → memory, ltree path, tags, metadata, importance)
               └─▶  POST /v1/memories/links  (SRO  → typed edge memory.<id> → memory.<id>)
        │
        ▼
  PCMI + Apache AGE  ◀── worker computes OpenAI embeddings (semantic retrieval)
        │
        ├─ retrieve (hybrid: BM25 + semantic + importance + decay)  → pre-ingest + correlation
        ├─ graph/related, graph/chain                                → longitudinal traversal
        └─ memories/summarize (LLM)                                  → cross-vendor threat brief
```

## Relationship to TI Mindmap HUB (complementary, not a reimplementation)

TI Mindmap HUB is **not** missing memory or a graph. It ships a Neo4j
**"STIX Constellation"** — a cross-report graph that *deduplicates entities into
canonical nodes* — with alias-aware search (`kg_search`), cross-report links
(`kg_cross_report`) and per-entity timelines (`kg_timeline`), exposed via **25 MCP
tools** alongside STIX 2.1 bundles, IOC/CVE search and weekly briefings. So this
PoC does **not** claim to add the cross-report correlation or knowledge graph the
HUB "lacks" — the HUB has them.

PCMI's role is **complementary**, on axes the HUB does not cover, and it
**integrates with** the HUB's output rather than duplicating the Constellation:

| TI Mindmap HUB already provides | PCMI adds on top |
|---|---|
| STIX 2.1 bundles, IOC/CVE search, weekly briefings (25 MCP tools) | **ingests** those STIX 2.1 bundles — `TI_HUB_MODE=live` |
| STIX Constellation: canonical entities, alias `kg_search`, `kg_cross_report`, `kg_timeline` | **versioned append-only `as_of`** — reconstruct *what was known at time T*, not just *when* an entity appeared |
| Multi-agent (Autogen) pipeline that produces the outputs | **agent working-memory + promote** for that pipeline (sessions) — a layer *beneath* the output graph |
| Open research platform | **enterprise controls**: multi-tenant, RBAC, audit log, encryption, idempotency |

Honest reading of the phases below: the cross-vendor **correlation** and
**knowledge-graph** phases overlap with the Constellation (and can be seen as
*validating* it); PCMI's genuinely **differentiated** value is temporal `as_of`
(Phase 6), agent working-memory (Phase 9), observability/audit (Phase 8), and being
a framework/domain-agnostic memory substrate.

Sources: [MCP tools](https://docs.ti-mindmap-hub.com/mcp/server/) ·
[STIX 2.1 data model](https://docs.ti-mindmap-hub.com/concepts/data-model/).

## Layout

| File | Role |
|------|------|
| `run_poc.py` | Async orchestrator — phases 0 preflight → 8 summary/checklist |
| `src/pcmi_client.py` | Async httpx wrapper over the PCMI REST API |
| `src/ti_hub_client.py` | Connector: `demo` (curated dataset) or `live` (real STIX 2.1 bundles) → reports/relationships |
| `src/stix_ingest.py` | Parses real **STIX 2.1 bundles** (the format TI Mindmap HUB emits) into the pipeline's shapes |
| `scripts/make_stix_bundles.py` | Transcodes the curated dataset into STIX 2.1 bundles for `TI_HUB_MODE=live` |
| `examples/tihub_stix/` | STIX 2.1 bundles for live mode + `hub_native/` (TI Mindmap HUB's own published example) |
| `src/stix_to_pcmi.py` | Maps SDOs → memories and SROs → typed links (path sanitising, link-type mapping) |
| `src/correlator.py` | Cross-vendor correlation: semantic actor pass (alias-confirmed) + lexical TTP pass |
| `src/distillation.py` | Deterministic LLM-style sparse-signal discovery + reusable evidence memory persistence |
| `src/llm_distillation.py` | Real LLM inference distillation (OpenAI or fixture) + validation + persistence under `root.cti.distillation.llm.*` |
| `examples/llm_distillation_fixture.json` | Recorded LLM JSON response for offline CI/tests (no API key) |
| `src/enrichment.py` | Persists discovered correlations back into the graph (validated vs added) |
| `src/report.py` | Renders the Markdown CTI brief (pure function) |
| `scripts/run_e2e.sh` | One-command cold start (infra → migrations → API+worker → PoC) |
| `tests/test_logic.py` | Pure-logic unit tests (no network): `python3 -m unittest discover -s tests` |
| `reports/` | Generated `cti_brief_<ts>.md` artifacts (git-ignored) |
| `examples/vendor_reports_cti_dataset.json` | The CTI dataset (40 nodes, 25 links) |
| `BUGS.md` | PCMI findings observed from the outside (no PCMI source changed) |

## Run it

### One command (recommended)

Brings up the AGE Postgres + Redis, applies migrations 020/021 (which
`docker-compose.yml` does not mount — see `BUGS.md`), starts the PCMI API +
worker with the OpenAI key from the repo `.env` (for LLM embeddings), then runs
the PoC:

```bash
cd pcmi-ti-hub-poc
scripts/run_e2e.sh --fresh      # true cold start (wipes the DB volume)
scripts/run_e2e.sh down         # stop API/worker + compose when done
```

### Live STIX 2.1 mode (real TI Mindmap HUB output format)

`TI_HUB_MODE=live` ingests **real STIX 2.1 bundles** from `examples/tihub_stix/`
(the exact format TI Mindmap HUB emits) instead of the curated JSON — proving the
integration path rather than a thematic demo. The bundles there are the curated CTI
transcoded into STIX 2.1 (`scripts/make_stix_bundles.py`); TI Mindmap HUB's own
published example lives in `examples/tihub_stix/hub_native/` and is verified by the
test suite. Point `TIHUB_STIX_DIR` at your own exported bundles to ingest them.

```bash
TI_HUB_MODE=live scripts/run_e2e.sh --fresh          # cold start, live STIX ingest
# or against a running stack:
TI_HUB_MODE=live TIHUB_STIX_DIR=/path/to/stix python3 run_poc.py
```

Live mode passes the same 17-point checklist as demo (4 reports, 40 memories,
25 links) — same pipeline, real STIX 2.1 in.

### Against an already-running PCMI

```bash
pip install -r requirements.txt
export PCMI_BASE_URL=http://localhost:8000
export PCMI_API_KEY=testkey123          # dev seed key (migration 003, admin role)
export TI_HUB_MODE=demo
python3 run_poc.py
```

**Requires** the AGE-enabled Postgres (`docker compose --profile graph up
postgres-age`), migrations 001–021 applied, and the API + **worker** running.
Semantic actor correlation needs `OPENAI_API_KEY` set on the API/worker
(`EMBEDDING_MODEL=text-embedding-3-small`); without it the run still passes but
the confirmed cross-vendor actor link falls back to lexical (limited).

### Environment knobs

| Var | Default | Meaning |
|-----|---------|---------|
| `PCMI_BASE_URL` | `http://localhost:8000` | PCMI API base URL |
| `PCMI_API_KEY` | `testkey123` | API key (`X-API-Key`) |
| `TI_HUB_MODE` | `demo` | `demo` reads the bundled dataset (no HUB key needed) |
| `POC_EMBED_WAIT_SECS` | `240` | Max wait for the embedding worker to vectorise memories |
| `POC_SEMANTIC_MIN_SCORE` | `0.30` | Min hybrid score for a semantic actor match |
| `POC_LLM_SUMMARY` | `1` | Set `0` to skip the LLM `/summarize` brief |
| `POC_LLM_DISTILLATION` | `1` | Set `0` to skip Phase 3d (LLM inference distillation) |
| `POC_LLM_MODEL` | `gpt-4o-mini` | OpenAI chat model for Phase 3d (live mode only) |

## What "good" looks like

```
Phase 2c  ✓ LLM embeddings live — "Midnight Blizzard" semantically bridged to "COZY BEAR (APT-29)"
Phase 3   ✔ [Microsoft] Midnight Blizzard ≡ [CrowdStrike] COZY BEAR (APT-29)
              semantic score≈0.50   shared identity: apt29, cozy bear
          ⚑ 8 shared TTPs across ≥2 vendors (OAuth, ransomware, zero-day, supply chain, …)
Phase 3c  ◆ malware_linked_to_actor: PROMPTSTEAL ↔ Forest Blizzard via apt28 (deterministic)
          ◆ campaign_actor_overlap: PRESSURE CHOLLIMA ↔ Sapphire Sleet via DPRK/crypto/macOS
          ✓ evidence memories persisted under root.cti.distillation with graph links
Phase 3d  ◆ [FIXTURE or LIVE] alias_equivalent: Midnight Blizzard ↔ COZY BEAR
          ◆ malware_linked_to_actor: PROMPTSTEAL ↔ Forest Blizzard
          ◆ campaign_actor_overlap: PRESSURE CHOLLIMA ↔ Sapphire Sleet (moderate confidence)
          ✓ validated LLM candidates persisted under root.cti.distillation.llm.*
Phase 4   graph_chain 2 hops cross-vendor; Cypher → 28 graph vertices
Phase 6   as_of time-travel: v1 (before follow-up) vs v2 (now), prior version retained
Phase 7   1 correlation validated (rediscovered) + new cross-vendor edges added
Phase 8   live SSE events observed (memory.stored/updated) + audit log recorded
Phase 9   investigation session: 4 working notes → promoted to long-term
Phase 10  reports/cti_brief_<ts>.md written
CHECKLIST ALL 15 CHECKS PASSED ✓  (40 memories, 25 links, 4 vendors)
```

## Tests

Pure-logic unit tests (link-type mapping, ltree sanitising, alias normalisation,
lexical matching, deterministic + LLM inference distillation, persistence idempotency,
date parsing, dataset shaping) — no PCMI required:

```bash
python3 -m unittest discover -s tests
```

## Design notes

* **Graph link identity.** Links use `from_path`/`to_path` = `memory.<id>` (the
  integer PCMI returns on store), not the memory's ltree path — that is the vertex
  identity the AGE sync trigger and `/v1/graph/*` expect (learned from
  `examples/soc-incident-graph/load_to_pcmi.py`). Links are flushed **after** all
  reports are stored because a relationship can span two vendors' reports.
* **Link types.** STIX verbs are mapped onto the five PCMI types
  (`uses/targets/exploits → causal`, `delivers/drops → temporal`,
  `attributed-to/indicates → supports`, `variant-of/related-to → related`); native
  PCMI types pass through. This dataset's links (`supports`, `related`) are already
  native.
* **Live STIX 2.1 ingestion.** `src/stix_ingest.py` parses real STIX 2.1 bundles
  (SDOs: threat-actor/campaign/malware/indicator/attack-pattern/vulnerability/note;
  SROs: `relationship`), keyed by STIX `id` so relationships resolve across bundles.
  Our transcoded bundles carry the source metadata in STIX custom `x_` properties so
  live mode matches demo richness; real HUB bundles simply omit those and fall back
  to STIX-native fields.
* **Honest attribution.** Semantic retrieval is used for **recall** (bridging
  code-names); any "vendor X documents Y" claim is re-confirmed **lexically**, and
  PCMI-derived `…consolidated` memories are excluded from vendor counts.
* **Precision vs. recall.** Confirmed actor correlations require both semantic
  proximity **and** a shared alias in structured metadata; semantically-close but
  differently-attributed actors are reported separately as *candidates*.
* **Honest labelling.** Phase 3c is deterministic (metadata alias `derived_type:
  llm_style_distillation`); Phase 3d is real LLM inference (`derived_type:
  llm_inference_distillation`, paths under `root.cti.distillation.llm.*`). Without
  `OPENAI_API_KEY`, Phase 3d uses the bundled fixture — still validates and
  persists, but is not a live model call.
* **Distillation link typing.** PCMI's public graph link enum is preserved
  (`related`/`supports`). CTI-specific relation types such as
  `alias_equivalent`, `malware_linked_to_actor` and `campaign_actor_overlap` are
  stored in the reusable evidence memory and link metadata so the demo remains
  compatible with the current API/schema.
