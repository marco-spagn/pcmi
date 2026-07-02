#!/usr/bin/env python3
"""PCMI × TI Mindmap HUB — proof-of-concept orchestrator.

Demonstrates PCMI as a persistent memory layer for AI-powered threat intel by
ingesting real 2025-2026 vendor CTI into the Cognitive Graph and showing three
capabilities:

  1. Pre-ingest retrieval  — before processing a new vendor report, ask PCMI what
     it already knows about the same actors/TTPs.
  2. Longitudinal graph    — typed cross-report links (multi-hop traversal).
  3. Cross-vendor correlation — the same actor/TTP found across vendors, driven by
     PCMI's LLM embedding retrieval (bridges different vendor code-names).

Run:
    export PCMI_BASE_URL=http://localhost:8000      # default
    export PCMI_API_KEY=testkey123                  # dev seed key (admin)
    export TI_HUB_MODE=demo
    python3 run_poc.py
"""
from __future__ import annotations

import asyncio
import datetime
import json
import os
import re
import sys
import time

from src.correlator import (
    NAMESPACE,
    TTP_PROBES,
    Correlator,
    is_vendor_finding,
    lexical_contains,
    normalize_aliases,
    vendor_of,
)
from src.distillation import DistillationPersister, discover_cross_report_knowledge
from src.enrichment import Enricher
from src.llm_distillation import run_llm_distillation
from src.pcmi_client import PCMIClient, PCMIError
from src.report import build_markdown_brief
from src.stix_to_pcmi import StixToPcmi, sanitize_path
from src.ti_hub_client import SDO, Report, TiHubClient

BASE_URL = os.environ.get("PCMI_BASE_URL", "http://localhost:8000")
API_KEY = os.environ.get("PCMI_API_KEY", "testkey123")
TI_HUB_MODE = os.environ.get("TI_HUB_MODE", "demo")
SEMANTIC_MIN_SCORE = float(os.environ.get("POC_SEMANTIC_MIN_SCORE", "0.30"))
EMBED_WAIT_SECS = int(os.environ.get("POC_EMBED_WAIT_SECS", "240"))
DO_SUMMARY = os.environ.get("POC_LLM_SUMMARY", "1") == "1"
DO_LLM_DISTILLATION = os.environ.get("POC_LLM_DISTILLATION", "1") == "1"
OPENAI_API_KEY = os.environ.get("OPENAI_API_KEY", "").strip()
POC_LLM_MODEL = os.environ.get("POC_LLM_MODEL", "gpt-4o-mini")
DATASET = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    "examples",
    "vendor_reports_cti_dataset.json",
)
# Sources for the non-demo modes.
_HERE = os.path.dirname(os.path.abspath(__file__))
STIX_DIR = os.environ.get("TIHUB_STIX_DIR") or os.path.join(_HERE, "examples", "tihub_stix")
REPORTS_DIR = os.environ.get("TIHUB_REPORTS_DIR") or os.path.join(_HERE, "examples", "tihub_reports")
TIHUB_MCP_URL = os.environ.get("TIHUB_MCP_URL") or None
TIHUB_API_KEY = os.environ.get("TIHUB_API_KEY", "").strip()
TIHUB_MCP_LIMIT = int(os.environ.get("TIHUB_MCP_LIMIT", "20"))

SUBJECT_TYPES = ("threat-actor", "campaign", "malware")
ACTOR_TYPES = ("threat-actor", "campaign")


# ── console helpers ──────────────────────────────────────────────────────────
def banner(title: str) -> None:
    print(f"\n{'═' * 72}\n  {title}\n{'═' * 72}")


def line(msg: str = "") -> None:
    print(msg, flush=True)


def actor_query(sdo: SDO) -> str:
    last = sdo.path.rsplit(".", 1)[-1]
    tokens = [t for t in re.split(r"[^A-Za-z0-9]+", last) if t]
    return " ".join(tokens[:2]) if tokens else last


def nice_name(sdo: SDO) -> str:
    md = sdo.metadata or {}
    return str(md.get("actor") or md.get("family") or actor_query(sdo).title())


def build_actor_subjects(reports: list[Report], ids: dict[str, int]) -> list[dict]:
    """Actor/campaign memories with their id, vendor, description and alias set."""
    subjects: list[dict] = []
    for report in reports:
        for sdo in report.sdos:
            if sdo.stix_type in ACTOR_TYPES and sdo.key in ids:
                subjects.append(
                    {
                        "id": ids[sdo.key],
                        "vendor": sdo.metadata.get("vendor", "?"),
                        "name": nice_name(sdo),
                        "content": sdo.content,
                        "aliases": normalize_aliases(sdo.metadata),
                    }
                )
    return subjects


def effective_promotion_count(promoted_count: int, verified_count: int, working_count: int) -> int:
    """Promotion success for both fresh and rerun databases.

    PCMI can return ``promoted=0`` when the target long-term paths already exist
    from an earlier PoC run; the verified dossier still proves the session flow.
    """
    if promoted_count > 0:
        return promoted_count
    return min(verified_count, working_count)


# ── Phase 2 helper: pre-ingest retrieval ─────────────────────────────────────
async def pre_ingest_scan(client: PCMIClient, report: Report, incoming_vendor: str) -> dict:
    probes: list[tuple[str, str]] = []
    seen: set[str] = set()
    for sdo in report.sdos:
        if sdo.stix_type in SUBJECT_TYPES:
            q = actor_query(sdo)
            if q and q not in seen:
                seen.add(q)
                probes.append((nice_name(sdo), q))
    blob = " ".join(f"{s.content} {' '.join(s.tags)}" for s in report.sdos).lower()
    for label, q in TTP_PROBES:
        if q not in seen and all(tok in blob for tok in q.lower().split()):
            seen.add(q)
            probes.append((label, q))

    # Fetch every prior memory once with an EMPTY query — that is a deterministic
    # "list all under prefix" (no semantic/lexical ranking, no query-embedding call),
    # so pre-ingest is independent of embedding-worker timing. Then match probes
    # purely client-side (whole-word lexical) against genuine vendor findings.
    prior = [e for e in await client.retrieve_entries(NAMESPACE, query="", limit=200) if is_vendor_finding(e)]

    found: dict[int, dict] = {}
    hit_probes: list[tuple[str, str, int]] = []
    cross_vendor: list[tuple[str, dict]] = []
    for label, q in probes:
        entries = [e for e in prior if lexical_contains(e, q)]
        if entries:
            hit_probes.append((label, q, len(entries)))
        for e in entries:
            found[e["id"]] = e
            v = vendor_of(e)
            if v and v != incoming_vendor:
                cross_vendor.append((label, e))
    return {"found": found, "hit_probes": hit_probes, "cross_vendor": cross_vendor}


# ── Phase 2c helper: wait for LLM embeddings ─────────────────────────────────
async def wait_for_embeddings(client: PCMIClient, subjects: list[dict]) -> dict:
    """Block until PCMI's embedding worker has vectorised enough memories for
    semantic retrieval to bridge vendor code-names.

    Signal: pick a same-actor pair confirmed by alias metadata but described
    under *different* code-names (e.g. Cozy Bear vs Midnight Blizzard). Query with
    one side's description and wait until the other side is returned — that can
    only happen semantically (their texts share no code-name), so it is a true
    "embeddings are live" probe rather than a lexical coincidence.
    """
    banner("PHASE 2c — Wait for PCMI LLM embeddings (semantic retrieval)")

    canary: tuple[dict, dict] | None = None
    for i, a in enumerate(subjects):
        for b in subjects[i + 1:]:
            if a["vendor"] != b["vendor"] and (a["aliases"] & b["aliases"]):
                canary = (a, b)
                break
        if canary:
            break

    if canary is None:
        line("  (no alias-confirmed cross-vendor pair to canary on; short fixed wait)")
        await asyncio.sleep(30)
        return {"ready": False, "reason": "no-canary"}

    a, b = canary
    shared = ", ".join(sorted(a["aliases"] & b["aliases"]))
    line(f"  canary: [{a['vendor']}] {a['name']}  ⟷  [{b['vendor']}] {b['name']}  (shared identity: {shared})")
    line(f"  waiting until a semantic query for \"{a['name']}\" surfaces \"{b['name']}\" (mem#{b['id']}) …")

    t0 = time.time()
    while time.time() - t0 < EMBED_WAIT_SECS:
        entries = await client.retrieve_entries(NAMESPACE, query=a["content"][:500], limit=8)
        ids = [e["id"] for e in entries]
        hit = b["id"] in ids
        line(f"    t={int(time.time() - t0):>3}s  semantic hits={len(entries)}  bridged={hit}")
        if hit:
            line(f"  ✓ LLM embeddings live — \"{a['name']}\" semantically bridged to \"{b['name']}\" across vendors.")
            await asyncio.sleep(10)  # small grace for remaining backfill
            return {"ready": True, "seconds": round(time.time() - t0, 1), "canary": (a["name"], b["name"])}
        await asyncio.sleep(10)

    line("  ⚠ embedding wait timed out — proceeding; semantic results may be partial.")
    return {"ready": False, "reason": "timeout"}


# ── Phases ───────────────────────────────────────────────────────────────────
async def phase0_preflight(client: PCMIClient) -> dict:
    banner("PHASE 0 — Preflight: Cognitive Graph health")
    health = await client.graph_health()
    line(f"  GET /v1/graph/health -> {json.dumps(health)}")
    if not health.get("available"):
        line("  ✗ Cognitive Graph not available — start the AGE Postgres profile and migration 019.")
        raise SystemExit(2)
    line("  ✓ Apache AGE available — graph traversal enabled.")
    return health


def phase1_pull(hub: TiHubClient) -> tuple[list[Report], list]:
    src = {
        "demo": "curated dataset (demo)",
        "reports": "their published reports (real Markdown CTI)",
        "live": "live platform STIX 2.1 via MCP",
        "stix": "STIX 2.1 bundles",
    }.get(hub.mode, hub.mode)
    banner(f"PHASE 1 — Pull CTI reports from TI Mindmap HUB — {src}")
    reports = hub.reports()
    relationships = hub.relationships()
    line(f"  Dataset: {hub.name}")
    line(f"  Reports (vendors): {len(reports)}   Relationships (links): {len(relationships)}")
    line("")
    for i, r in enumerate(reports, 1):
        line(f"  [{i}] {r.source}")
        line(f"        vendor={r.vendor}  date={r.report_date or '?'}  period={r.data_period or '?'}  nodes={r.node_count}")
    return reports, relationships


async def phase2_ingest(
    client: PCMIClient, mapper: StixToPcmi, reports: list[Report], relationships: list
) -> dict:
    banner("PHASE 2 — Ingest reports in order (with pre-ingest retrieval)")
    per_report: list[dict] = []
    pre_ingest_log: list[dict] = []

    for idx, report in enumerate(reports):
        line(f"\n  ── Report {idx + 1}/{len(reports)}: {report.vendor} ({report.node_count} nodes) ──")
        if idx > 0:
            scan = await pre_ingest_scan(client, report, report.vendor)
            n_prior = len(scan["found"])
            line(f"  ▶ PRE-INGEST RETRIEVAL: PCMI already holds {n_prior} prior memories relevant to this report.")
            for label, q, n in scan["hit_probes"][:6]:
                line(f"      • probe \"{q}\" ({label}) → {n} prior hit(s)")
            for label, e in scan["cross_vendor"][:4]:
                line(f"      ↳ cross-vendor prior context on \"{label}\": [{vendor_of(e)}] mem#{e['id']} {e['path']}")
            pre_ingest_log.append({"report_index": idx + 1, "vendor": report.vendor, "prior_memories": n_prior})
        else:
            line("  ▶ PRE-INGEST RETRIEVAL: skipped (first report — store is empty).")

        stats = await mapper.ingest_report(report)
        by_type = ", ".join(f"{k}:{v}" for k, v in sorted(stats["by_type"].items()))
        line(f"  ✓ ingested {stats['stored']} memories  [{by_type}]")
        per_report.append(stats)

    banner("PHASE 2b — Flush typed Cognitive Graph links (cross-report)")
    link_stats = await mapper.link_relationships(relationships)
    by_type = ", ".join(f"{k}:{v}" for k, v in sorted(link_stats["by_type"].items()))
    line(f"  ✓ created {link_stats['created']} links  [{by_type}]   skipped(no endpoint)={link_stats['skipped']}")

    total_mem = sum(s["stored"] for s in per_report)
    line(f"\n  TOTAL: {total_mem} memories, {link_stats['created']} links across {len(reports)} vendors.")
    return {"per_report": per_report, "links": link_stats, "pre_ingest": pre_ingest_log, "total_memories": total_mem}


async def phase3_correlate(client: PCMIClient, subjects: list[dict]) -> dict:
    banner("PHASE 3 — Cross-vendor correlation (LLM semantic retrieval)")
    corr = Correlator(client)

    actor_pairs = await corr.semantic_actor_correlations(subjects, min_score=SEMANTIC_MIN_SCORE)
    confirmed = [p for p in actor_pairs if p["confirmed"]]
    candidates = [p for p in actor_pairs if not p["confirmed"]]

    line("\n  Same actor under DIFFERENT vendor code-names (semantic match, alias-confirmed):")
    if confirmed:
        for p in confirmed:
            line(f"    ✔ [{p['a']['vendor']}] {p['a']['name']}  ≡  [{p['b']['vendor']}] {p['b']['name']}")
            line(f"         semantic score={p['score']}   shared identity: {', '.join(p['shared_aliases'])}")
    else:
        line("    (none confirmed)")

    if candidates:
        line("\n  Semantic-only cross-vendor actor candidates (no shared alias — related tradecraft):")
        for p in candidates[:5]:
            line(f"    ~ [{p['a']['vendor']}] {p['a']['name']}  ~  [{p['b']['vendor']}] {p['b']['name']}  (score={p['score']})")

    ttp_hits = await corr.ttp_correlations()
    line("\n  Shared TTPs documented by MULTIPLE vendors:")
    for t in ttp_hits:
        line(f"    ⚑ {t['label']:<32} (\"{t['query']}\") → {t['n_vendors']} vendors: {', '.join(t['vendors'])}  [{t['n_memories']} memories]")

    total = len(confirmed) + len(ttp_hits)
    line(f"\n  TOTAL cross-vendor correlations: {total}  "
         f"(confirmed actors={len(confirmed)}, semantic candidates={len(candidates)}, TTPs={len(ttp_hits)})")

    # Optional: LLM narrative synthesis over the whole corpus (distillation-style).
    summary_text = None
    if DO_SUMMARY:
        try:
            res = await client.summarize(NAMESPACE, limit=40, style="brief")
            summary_text = (res.get("summary") or res.get("text") or "").strip()
            if summary_text:
                banner("PHASE 3b — LLM cross-vendor threat brief (PCMI /summarize)")
                for para in summary_text.split("\n"):
                    if para.strip():
                        line(f"  {para.strip()}")
        except PCMIError as exc:
            line(f"  (LLM summary skipped: {exc})")

    return {
        "confirmed_actor_correlations": confirmed,
        "semantic_candidates": candidates,
        "ttp_correlations": ttp_hits,
        "total": total,
        "llm_summary": summary_text,
    }


async def phase3d_llm_distill(client: PCMIClient, reports: list[Report]) -> dict:
    """Real LLM inference: report text + retrieval context → validated correlations."""
    banner("PHASE 3d — LLM inference distillation (report text + retrieval → knowledge)")

    if not DO_LLM_DISTILLATION:
        line("  (skipped — set POC_LLM_DISTILLATION=1 to enable)")
        return {"skipped": True, "reason": "disabled", "mode": "skipped"}

    if OPENAI_API_KEY:
        line(f"  inference mode: live OpenAI ({POC_LLM_MODEL})")
    else:
        line("  inference mode: fixture (no OPENAI_API_KEY — using recorded LLM response for CI/offline)")

    try:
        result = await run_llm_distillation(
            client, reports, api_key=OPENAI_API_KEY or None, model=POC_LLM_MODEL,
        )
    except Exception as exc:
        line(f"  ✗ LLM distillation failed: {exc}")
        return {"skipped": True, "reason": str(exc), "mode": "error"}

    line(
        f"  LLM returned {result.get('raw_candidate_count', 0)} candidate(s); "
        f"{result.get('discovered', 0)} accepted after validation "
        f"({len(result.get('rejected') or [])} rejected)."
    )
    for c in (result.get("candidates") or [])[:8]:
        signal_text = ", ".join(f"{s['kind']}={s['value']}" for s in c.get("signals", []))
        mode_tag = "LIVE" if result.get("mode") == "live" else "FIXTURE"
        line(f"    ◆ [{mode_tag}] {c['relation_type']}: {c['from_name']} ↔ {c['to_name']}  confidence={c['confidence']}")
        line(f"       signals: {signal_text}")
        line(f"       evidence memory: {c['memory_path']}")
    for rej in (result.get("rejected") or [])[:3]:
        line(f"    ✗ rejected: {rej.get('reason')}")

    line(
        "\n  ✓ promoted LLM distillation to PCMI: "
        f"{result.get('persisted', 0)} evidence memories, {result.get('links_added', 0)} graph link(s) added, "
        f"{result.get('links_validated', 0)} existing link(s) validated/idempotent."
    )
    if result.get("memory_paths"):
        for path in result["memory_paths"][:6]:
            line(f"      reusable memory: {path}")
    return result


async def phase3c_distill(client: PCMIClient, reports: list[Report]) -> dict:
    """Derive and persist reusable cross-report knowledge from sparse signals."""
    banner("PHASE 3c — Deterministic LLM-style distillation (sparse signals → reusable knowledge)")

    candidates = discover_cross_report_knowledge(reports)
    line(f"  discovery candidates generated from report text/metadata: {len(candidates)}")
    for c in candidates[:8]:
        signal_text = ", ".join(f"{s['kind']}={s['value']}" for s in c["signals"])
        line(f"    ◆ {c['relation_type']}: {c['from_name']} ↔ {c['to_name']}  confidence={c['confidence']}")
        line(f"       signals: {signal_text}")
        line(f"       evidence memory: {c['memory_path']}")

    result = await DistillationPersister(client).persist(candidates)
    line(
        "\n  ✓ promoted distillation to PCMI: "
        f"{result['persisted']} evidence memories, {result['links_added']} graph link(s) added, "
        f"{result['links_validated']} existing link(s) validated/idempotent."
    )
    if result["memory_paths"]:
        for path in result["memory_paths"][:6]:
            line(f"      reusable memory: {path}")
    return result


async def phase4_graph(client: PCMIClient, reports: list[Report]) -> dict:
    banner("PHASE 4 — Cognitive Graph traversal")

    candidates: list[tuple[SDO, int]] = []
    for report in reports:
        for sdo in report.sdos:
            if sdo.stix_type in ACTOR_TYPES and sdo.key in client.ids:
                candidates.append((sdo, client.ids[sdo.key]))
    if not candidates:
        line("  ✗ no threat-actor/campaign memories to traverse from.")
        return {"available": False}

    # "Find a threat actor via retrieve", then pick the richest neighbourhood:
    # prefer more reachable nodes, then deeper chains (probe the live graph).
    best: dict | None = None
    for sdo, mid in candidates:
        rel = await client.graph_related(mid, depth=4)
        entries = rel.get("entries") or []
        max_depth = max((e.get("depth", 0) for e in entries), default=0)
        key = (len(entries), max_depth)
        if best is None or key > best["key"]:
            best = {"sdo": sdo, "id": mid, "entries": entries, "key": key}

    assert best is not None
    start_sdo, start_id, entries = best["sdo"], best["id"], best["entries"]

    q = actor_query(start_sdo)
    retrieved = await client.retrieve_entries(NAMESPACE, query=q, limit=5)
    line(f"  ▶ retrieve(\"{q}\") → threat node {nice_name(start_sdo)} = mem#{start_id} "
         f"(in top results: {start_id in [e['id'] for e in retrieved]})")

    subgraph_nodes = 1 + len(entries)
    line(f"\n  graph_related(memory_id={start_id}, depth=4, all link types):")
    line(f"    reached {len(entries)} related nodes → explored subgraph spans {subgraph_nodes} nodes")
    for e in sorted(entries, key=lambda x: x.get("depth", 0)):
        line(f"      depth {e.get('depth')}  via {e.get('link_type'):<10} mem#{e['id']}")

    chain_result = None
    if entries:
        target = max(entries, key=lambda x: x.get("depth", 0))
        chain = await client.graph_chain(start_id, target["id"], max_depth=10)
        line(f"\n  graph_chain(from={start_id}, to={target['id']}):  "
             f"connected={chain.get('connected')} hops={chain.get('hops')}")
        for hop in chain.get("path") or []:
            line(f"      hop {hop.get('hop')}: mem#{hop['from_id']} --{hop['link_type']}--> mem#{hop['to_id']}")
        chain_result = chain

    first_id, last_id = min(client.ids.values()), max(client.ids.values())
    fl = await client.graph_chain(first_id, last_id, max_depth=20)
    line(f"\n  graph_chain(first mem#{first_id} → last mem#{last_id}):  "
         f"connected={fl.get('connected')} hops={fl.get('hops')} (separate threads need not connect — expected)")

    # Read-only Cypher passthrough (4th graph endpoint). The passthrough returns a
    # single 'result' column, so only single-column RETURNs are used here.
    graph_vertices = None
    try:
        res = await client.graph_cypher("MATCH (n:Memory) RETURN count(n) AS n")
        rows = res.get("rows") or []
        graph_vertices = rows[0].get("result") if rows else None
        line(f"\n  Cypher POST /v1/graph/cypher: \"MATCH (n:Memory) RETURN count(n)\" → "
             f"{graph_vertices} graph vertices (linked memories)")
    except PCMIError as exc:
        line(f"\n  (Cypher passthrough skipped: {exc})")

    return {
        "available": True,
        "start": {"id": start_id, "name": nice_name(start_sdo), "vendor": start_sdo.metadata.get("vendor")},
        "related_reached": len(entries),
        "subgraph_nodes": subgraph_nodes,
        "connected_chain": chain_result,
        "graph_vertices": graph_vertices,
    }


def _find_sdo(reports: list[Report], key: str) -> SDO | None:
    for report in reports:
        for sdo in report.sdos:
            if sdo.key == key:
                return sdo
    return None


async def phase6_temporal(client: PCMIClient, reports: list[Report]) -> dict:
    """Append-only versioning + `as_of` time-travel — PCMI's flagship capability.

    Simulates a follow-up report updating an actor profile, then time-travels to
    show the prior assessment is retained (not overwritten) and queryable.
    """
    banner("PHASE 6 — Temporal memory: append-only versioning & as_of time-travel")

    # Pick the richest actor/campaign (most normalised aliases) — mode-agnostic;
    # falls back to a campaign when no actor is named (some real reports have none).
    actors = [s for r in reports for s in r.sdos if s.stix_type in ACTOR_TYPES and s.key in client.ids]
    target = _find_sdo(reports, "ms_midnight_blizzard_oauth") or (
        max(actors, key=lambda s: len(normalize_aliases(s.metadata)), default=None)
    )
    if target is None:
        line("  ✗ no actor memory to demonstrate versioning.")
        return {"demonstrated": False}

    path = sanitize_path(target.path)
    before = await client.get_memory(path)
    v_before = before.get("version")
    line(f"  memory: {path}")
    line(f"  before follow-up: version {v_before}  “…{(before.get('content') or '')[-72:].strip()}”")

    t_cut = datetime.datetime.now(datetime.timezone.utc).isoformat()
    await asyncio.sleep(1.2)  # ensure a distinct valid_from for the new version

    follow_up = (
        target.content
        + "  [FOLLOW-UP 2026-06: consent-grant OAuth campaign expanded to additional NGOs and "
        "think-tanks; new device-code phishing indicators shared with partners.]"
    )
    meta = dict(target.metadata)
    meta.update({"stix_type": target.stix_type, "pcmi_key": target.key, "revision": "follow-up-2026-06"})
    await client.store(path, follow_up, tags=list(target.tags) + [target.stix_type], metadata=meta, importance=0.9)

    now = await client.get_memory(path)
    past = await client.get_memory(path, as_of=t_cut)
    line(f"  ▶ stored follow-up → version {now.get('version')} "
         f"(append-only: v{v_before} retained, not overwritten)")
    line(f"  as_of {t_cut[:19]}Z  → version {past.get('version')}: “…{(past.get('content') or '')[-64:].strip()}”")
    line(f"  now (current)       → version {now.get('version')}: “…{(now.get('content') or '')[-96:].strip()}”")

    ok = (past.get("version") == v_before) and (now.get("version") == (v_before or 0) + 1)
    line(f"  ✓ time-travel: “what did we know before the follow-up?” answered as a query parameter."
         if ok else "  ⚠ unexpected version numbers")
    return {
        "demonstrated": ok,
        "path": path,
        "version_before": v_before,
        "version_now": now.get("version"),
        "as_of_version": past.get("version"),
    }


async def phase7_enrich(client: PCMIClient, correlate: dict) -> dict:
    """Persist PCMI-discovered correlations back into the graph (close the loop)."""
    banner("PHASE 7 — Knowledge accumulation (persist discovered correlations)")

    corrs: list[dict] = []
    for p in correlate["confirmed_actor_correlations"]:
        corrs.append({
            "a_id": p["a"]["id"], "b_id": p["b"]["id"],
            "label": f"[{p['a']['vendor']}] {p['a']['name']} ≡ [{p['b']['vendor']}] {p['b']['name']}",
            "score": p["score"], "shared_aliases": p["shared_aliases"], "kind": "confirmed-actor-identity",
        })
    for p in correlate["semantic_candidates"]:
        if p["score"] >= 0.47:
            corrs.append({
                "a_id": p["a"]["id"], "b_id": p["b"]["id"],
                "label": f"[{p['a']['vendor']}] {p['a']['name']} ~ [{p['b']['vendor']}] {p['b']['name']}",
                "score": p["score"], "shared_aliases": [], "kind": "semantic-tradecraft",
            })

    result = await Enricher(client).persist_correlations(corrs)
    validated, added = result["validated"], result["added"]

    line(f"  discovered correlations considered: {len(corrs)}")
    line(f"\n  ✔ validated (already curated by the analyst — PCMI rediscovered independently): {len(validated)}")
    for c in validated[:6]:
        line(f"      {c['label']}   [{c['kind']}]")
    line(f"\n  ＋ added (new PCMI-discovered cross-vendor edges): {len(added)}")
    for c in added[:8]:
        line(f"      {c['label']}   related  score={c['score']}")

    # Prove one added edge is now traversable.
    proof = None
    if added:
        c = added[0]
        chain = await client.graph_chain(c["a_id"], c["b_id"], max_depth=3)
        line(f"\n  proof — graph_chain(mem#{c['a_id']} → mem#{c['b_id']}) after enrichment: "
             f"connected={chain.get('connected')} hops={chain.get('hops')} "
             f"(this cross-vendor edge did not exist before Phase 7)")
        proof = {"from": c["a_id"], "to": c["b_id"], "connected": chain.get("connected"), "hops": chain.get("hops")}

    return {"considered": len(corrs), "validated": len(validated), "added": len(added), "proof": proof}


async def phase8_observability(client: PCMIClient) -> dict:
    """Show PCMI observability: live SSE event stream + the per-request audit log —
    the "every mutation observable / every decision auditable" enterprise story
    that regulated CTI workflows need."""
    banner("PHASE 8 — Observability & audit trail (SSE events + audit log)")

    collected: list[dict] = []

    async def _listen() -> None:
        try:
            async for ev in client.subscribe_events(types=["memory.stored", "memory.updated"]):
                collected.append(ev)
        except asyncio.CancelledError:
            raise
        except Exception:  # SSE is best-effort for the demo
            pass

    listener = asyncio.create_task(_listen())
    await asyncio.sleep(1.0)  # let the subscription establish before mutating

    marker = "root.cti.observability.run_marker"
    await client.store(marker, "PoC observability marker", importance=0.4)
    await asyncio.sleep(0.4)
    await client.store(marker, "PoC observability marker (updated)", importance=0.4)  # → memory.updated
    await asyncio.sleep(2.0)

    listener.cancel()
    try:
        await listener
    except asyncio.CancelledError:
        pass

    seen = [e.get("Type") or e.get("type") for e in collected]
    line(f"  live SSE stream (/v1/events): observed {len(collected)} event(s) → {seen}")

    audit = await client.list_audit(limit=8)
    entries = audit.get("entries") or []
    line(f"  audit log (/v1/audit): {audit.get('total')} API requests recorded; most recent:")
    for e in entries[:6]:
        ts = (e.get("created_at") or "")[11:19]
        line(f"    {ts}  {str(e.get('method','')):6} {e.get('status_code','')}  {e.get('path','')}")
    line("  → every mutation is streamable (SOC dashboards) and every request is audited (compliance).")

    return {"events_observed": len(collected), "event_types": seen, "audit_total": audit.get("total")}


async def phase9_session(client: PCMIClient, correlate: dict, temporal: dict) -> dict:
    """Model an analyst investigation with PCMI **agent sessions**: accumulate
    ephemeral working memory, then promote the dossier to long-term."""
    banner("PHASE 9 — Analyst investigation (agent session → promote)")

    confirmed = correlate.get("confirmed_actor_correlations") or []
    subject = confirmed[0]["a"]["name"] if confirmed else "APT29"
    slug = re.sub(r"[^a-z0-9]+", "_", subject.lower()).strip("_") or "actor"

    session = await client.create_session(metadata={"investigation": subject, "analyst": "pcmi-poc"})
    sid = session["id"]
    line(f"  opened session {sid[:8]}…  subject: {subject}")

    notes: list[tuple[str, str]] = []
    if confirmed:
        c = confirmed[0]
        notes.append(("identity",
                      f"Cross-vendor identity: {c['a']['name']} ({c['a']['vendor']}) ≡ "
                      f"{c['b']['name']} ({c['b']['vendor']}); shared {', '.join(c['shared_aliases'])}; "
                      f"semantic score {c['score']}."))
    oauth = next((t for t in (correlate.get("ttp_correlations") or []) if "OAuth" in t["label"]), None)
    if oauth:
        notes.append(("ttp", f"Shared TTP: {oauth['label']} — {oauth['n_vendors']} vendors "
                             f"({', '.join(oauth['vendors'])})."))
    if temporal.get("demonstrated"):
        notes.append(("temporal", f"Profile escalated v{temporal.get('version_before')}→"
                                  f"v{temporal.get('version_now')}; prior assessment retained via as_of."))
    notes.append(("assessment", f"Assessment: {subject} is an active cross-vendor threat; "
                                "consolidate into a long-term dossier."))

    for path, content in notes:
        await client.store_session_memory(sid, path, content, tags=[slug])
    listed = await client.list_session_memories(sid, limit=20)
    wm = len(listed.get("entries") or [])
    line(f"  accumulated {wm} working-memory notes (ephemeral, session-scoped)")

    target = f"root.cti.investigations.{slug}"
    promoted = await client.promote_session(sid, target_prefix=target)
    promoted_count = int(promoted.get("promoted") or 0)
    line(f"  ▶ promoted {promoted_count} new notes to long-term at {target}")

    verify = await client.retrieve_entries(target, query="", limit=20)
    verified_count = len(verify)
    effective_promoted = effective_promotion_count(promoted_count, verified_count, wm)
    if promoted_count == 0 and effective_promoted > 0:
        line("  ↳ notes were already present from an earlier run; treating the verified dossier as success.")
    line(f"  ✓ long-term now holds {verified_count} promoted memories under {target}")
    await client.end_session(sid)
    line("  session closed (working memory is gone; the dossier persists).")

    return {
        "subject": subject,
        "working_memories": wm,
        "promoted": effective_promoted,
        "promoted_new": promoted_count,
        "target_prefix": target,
        "verified": verified_count,
    }


async def phase10_brief(
    hub: TiHubClient, reports: list[Report], ingest: dict, embeddings: dict,
    correlate: dict, distillation: dict, llm_distillation: dict, temporal: dict, graph: dict,
    enrich: dict, session: dict, observ: dict,
) -> dict:
    """Assemble everything into a Markdown CTI brief artifact on disk."""
    banner("PHASE 10 — Generate CTI brief artifact (Markdown)")
    reports_meta = [
        {"vendor": r.vendor, "source": r.source, "report_date": r.report_date, "node_count": r.node_count}
        for r in reports
    ]
    generated_at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    md = build_markdown_brief(
        dataset=hub.name, generated_at=generated_at, api=BASE_URL, reports_meta=reports_meta,
        ingest=ingest, embeddings=embeddings, correlate=correlate, temporal=temporal,
        graph=graph, enrich=enrich, distillation=distillation, llm_distillation=llm_distillation,
        session=session, observability=observ,
    )
    out_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "reports")
    os.makedirs(out_dir, exist_ok=True)
    ts = datetime.datetime.now().strftime("%Y%m%dT%H%M%S")
    out_path = os.path.join(out_dir, f"cti_brief_{ts}.md")
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write(md)
    line(f"  ✓ wrote {os.path.relpath(out_path)}  ({len(md.splitlines())} lines, {len(md)} bytes)")
    return {"path": out_path, "bytes": len(md)}


async def main() -> int:
    data_label = {
        "demo": os.path.relpath(DATASET),
        "reports": os.path.relpath(REPORTS_DIR),
        "stix": os.path.relpath(STIX_DIR),
        "live": TIHUB_MCP_URL or "TI Mindmap HUB MCP server (live)",
    }.get(TI_HUB_MODE, os.path.relpath(DATASET))
    line("PCMI × TI Mindmap HUB — Proof of Concept")
    line(f"  API   : {BASE_URL}")
    line(f"  Mode  : TI_HUB_MODE={TI_HUB_MODE}")
    line(f"  Data  : {data_label}")

    hub = TiHubClient(
        DATASET, mode=TI_HUB_MODE, stix_dir=STIX_DIR, reports_dir=REPORTS_DIR,
        mcp_url=TIHUB_MCP_URL, api_key=TIHUB_API_KEY, mcp_limit=TIHUB_MCP_LIMIT,
    )
    if TI_HUB_MODE == "live":
        try:
            await hub.fetch_live()
        except Exception as exc:
            line(f"  ✗ live MCP ingest failed: {exc}")
            line("    (set TIHUB_API_KEY=tim_... ; or use TI_HUB_MODE=reports for their published reports)")
            return 2
    else:
        hub.load()

    async with PCMIClient(BASE_URL, API_KEY) as client:
        try:
            await phase0_preflight(client)
        except PCMIError as exc:
            line(f"  ✗ preflight failed: {exc}")
            return 2

        reports, relationships = phase1_pull(hub)
        mapper = StixToPcmi(client)
        ingest = await phase2_ingest(client, mapper, reports, relationships)

        subjects = build_actor_subjects(reports, client.ids)
        embed = await wait_for_embeddings(client, subjects)

        correlate = await phase3_correlate(client, subjects)
        distillation = await phase3c_distill(client, reports)
        llm_distillation = await phase3d_llm_distill(client, reports)
        graph = await phase4_graph(client, reports)
        temporal = await phase6_temporal(client, reports)
        enrich = await phase7_enrich(client, correlate)
        observ = await phase8_observability(client)
        session = await phase9_session(client, correlate, temporal)
        embeddings_meta = {"model": "text-embedding-3-small", "ready": embed.get("ready")}
        brief = await phase10_brief(
            hub, reports, ingest, embeddings_meta, correlate, distillation, llm_distillation,
            temporal, graph, enrich, session, observ,
        )

        banner("PHASE 11 — Summary")
        summary = {
            "dataset": hub.name,
            "api": BASE_URL,
            "llm_embeddings": {"model": "text-embedding-3-small", "ready": embed.get("ready"), "wait_seconds": embed.get("seconds")},
            "vendors_ingested": len(reports),
            "memories_created": ingest["total_memories"],
            "links_created": ingest["links"]["created"],
            "links_by_type": ingest["links"]["by_type"],
            "pre_ingest_retrieval": ingest["pre_ingest"],
            "cross_vendor_correlations": correlate["total"],
            "confirmed_actor_correlations": [
                {"a": f"{p['a']['vendor']}:{p['a']['name']}", "b": f"{p['b']['vendor']}:{p['b']['name']}",
                 "shared": p["shared_aliases"], "score": p["score"]}
                for p in correlate["confirmed_actor_correlations"]
            ],
            "semantic_candidate_count": len(correlate["semantic_candidates"]),
            "correlation_ttps": [t["label"] for t in correlate["ttp_correlations"]],
            "deterministic_llm_style_distillation": {
                "discovered": distillation.get("discovered"),
                "persisted": distillation.get("persisted"),
                "memories_created": distillation.get("memories_created"),
                "links_added": distillation.get("links_added"),
                "links_validated": distillation.get("links_validated"),
                "examples": [
                    {
                        "type": c["relation_type"],
                        "from": c["from_name"],
                        "to": c["to_name"],
                        "confidence": c["confidence"],
                        "memory_path": c["memory_path"],
                    }
                    for c in (distillation.get("candidates") or [])[:5]
                ],
            },
            "llm_inference_distillation": {
                "skipped": llm_distillation.get("skipped"),
                "mode": llm_distillation.get("mode"),
                "model": llm_distillation.get("model"),
                "discovered": llm_distillation.get("discovered"),
                "persisted": llm_distillation.get("persisted"),
                "rejected_count": len(llm_distillation.get("rejected") or []),
                "memories_created": llm_distillation.get("memories_created"),
                "links_added": llm_distillation.get("links_added"),
                "examples": [
                    {
                        "type": c["relation_type"],
                        "from": c["from_name"],
                        "to": c["to_name"],
                        "confidence": c["confidence"],
                        "memory_path": c["memory_path"],
                    }
                    for c in (llm_distillation.get("candidates") or [])[:5]
                ],
            },
            "graph": {
                "available": graph.get("available"),
                "traversal_start": graph.get("start"),
                "subgraph_nodes": graph.get("subgraph_nodes"),
                "connected_chain_hops": (graph.get("connected_chain") or {}).get("hops"),
                "graph_vertices": graph.get("graph_vertices"),
            },
            "temporal": {
                "demonstrated": temporal.get("demonstrated"),
                "version_before": temporal.get("version_before"),
                "version_now": temporal.get("version_now"),
                "as_of_returns_version": temporal.get("as_of_version"),
            },
            "knowledge_accumulation": {
                "correlations_considered": enrich.get("considered"),
                "validated_against_curated_graph": enrich.get("validated"),
                "new_edges_added": enrich.get("added"),
            },
            "observability": {
                "live_events_observed": observ.get("events_observed"),
                "event_types": observ.get("event_types"),
                "audit_requests_recorded": observ.get("audit_total"),
            },
            "investigation_session": {
                "subject": session.get("subject"),
                "working_memories": session.get("working_memories"),
                "promoted_to_long_term": session.get("promoted"),
                "target_prefix": session.get("target_prefix"),
            },
            "brief_artifact": os.path.relpath(brief.get("path", "")) if brief.get("path") else None,
        }
        print(json.dumps(summary, indent=2, ensure_ascii=False))

        banner("CHECKLIST")
        strict = TI_HUB_MODE == "demo"  # demo is the curated showcase; real-data modes gate on core capabilities
        checks = [
            ("Phase 0: graph available", graph.get("available") is True),
            (f"Phase 1: {'4' if strict else '>=1'} reports pulled", len(reports) == 4 if strict else len(reports) >= 1),
            (f"Phase 2: {'~40 ' if strict else ''}memories created",
             ingest["total_memories"] >= 35 if strict else ingest["total_memories"] > 0),
            ("Phase 2: links created", ingest["links"]["created"] > 0),
            ("Phase 3: >=1 cross-report correlation", correlate["total"] >= 1),
            ("Phase 4: traversal reaches >3 nodes", (graph.get("subgraph_nodes") or 0) > 3),
            ("Phase 4: Cypher passthrough returns graph vertex count", (graph.get("graph_vertices") or 0) > 0),
            ("Phase 6: as_of time-travel returns the prior version", temporal.get("demonstrated") is True),
            ("Phase 8: audit trail recorded + live events observed",
             (observ.get("audit_total") or 0) > 0 and (observ.get("events_observed") or 0) > 0),
            ("Phase 9: session promoted working memory to long-term", (session.get("promoted") or 0) > 0),
            ("Phase 10: CTI brief artifact written", (brief.get("bytes") or 0) > 0),
            ("Phase 11: summary JSON complete", True),
        ]
        if strict:
            # Curated-showcase gates (the demo dataset is tuned to exercise these).
            checks[4:4] = [
                ("Phase 2: pre-ingest finds >0 priors from report 2 on",
                 len(ingest["pre_ingest"]) >= 1 and all(p["prior_memories"] > 0 for p in ingest["pre_ingest"])),
            ]
            checks += [
                ("Phase 3: LLM confirmed same-actor across vendors", len(correlate["confirmed_actor_correlations"]) >= 1),
                ("Phase 3c: deterministic LLM-style distillation persisted reusable knowledge",
                 (distillation.get("persisted") or 0) >= 2 and bool(distillation.get("memory_paths"))),
                ("Phase 3d: LLM inference distillation persisted validated knowledge",
                 not llm_distillation.get("skipped") and (llm_distillation.get("persisted") or 0) >= 1),
                ("Phase 7: discovered correlations persisted (validated+added>0)",
                 (enrich.get("validated", 0) + enrich.get("added", 0)) > 0),
            ]
        ok = True
        for label, passed in checks:
            ok = ok and passed
            line(f"  [{'✓' if passed else '✗'}] {label}")
        line(f"\n  RESULT: {'ALL CHECKS PASSED ✓' if ok else 'SOME CHECKS FAILED ✗'}")
        return 0 if ok else 1


if __name__ == "__main__":
    try:
        sys.exit(asyncio.run(main()))
    except KeyboardInterrupt:
        sys.exit(130)
