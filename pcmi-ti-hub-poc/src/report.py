"""Render a Markdown CTI brief from the PoC run results.

Pure function (no I/O, no network) so it is unit-testable: it takes the results
collected across the run phases and returns a Markdown string. `run_poc.py` writes
it to `reports/cti_brief_<timestamp>.md`.
"""
from __future__ import annotations

from typing import Any


def _fmt_vendors(vendors: list[str]) -> str:
    return ", ".join(vendors)


def build_markdown_brief(
    *,
    dataset: str,
    generated_at: str,
    api: str,
    reports_meta: list[dict[str, Any]],
    ingest: dict[str, Any],
    embeddings: dict[str, Any],
    correlate: dict[str, Any],
    temporal: dict[str, Any],
    graph: dict[str, Any],
    enrich: dict[str, Any],
    session: dict[str, Any],
    distillation: dict[str, Any] | None = None,
    llm_distillation: dict[str, Any] | None = None,
    observability: dict[str, Any] | None = None,
) -> str:
    lines: list[str] = []
    a = lines.append

    a(f"# Cross-Vendor CTI Brief — {dataset}")
    a("")
    a(f"*Generated {generated_at} · source: PCMI memory layer at {api}*")
    a("")
    a("> Produced automatically by the PCMI × TI Mindmap HUB PoC. PCMI is the "
      "persistent memory layer; correlations below were surfaced by its hybrid "
      "retrieval (LLM embeddings + BM25) and Cognitive Graph, not hand-authored.")
    a("")

    # Executive summary (LLM)
    llm = (correlate.get("llm_summary") or "").strip()
    if llm:
        a("## Executive summary (LLM)")
        a("")
        a(llm)
        a("")

    # Ingest overview
    a("## Corpus")
    a("")
    a(f"- **{ingest.get('total_memories')} memories**, **{ingest['links']['created']} typed graph links**, "
      f"across **{len(reports_meta)} vendor reports**")
    a(f"- LLM embeddings: `{embeddings.get('model')}` "
      f"({'ready' if embeddings.get('ready') else 'not ready'})")
    a("")
    a("| Vendor | Report | Date | Findings |")
    a("|--------|--------|------|----------|")
    for r in reports_meta:
        a(f"| {r['vendor']} | {r['source']} | {r.get('report_date') or '?'} | {r['node_count']} |")
    a("")

    # Pre-ingest retrieval
    a("## Prior-context recall (pre-ingest)")
    a("")
    a("Before ingesting each report, PCMI was queried for what it already knew:")
    a("")
    for p in ingest.get("pre_ingest", []):
        a(f"- Report {p['report_index']} ({p['vendor']}): **{p['prior_memories']}** prior memories already held")
    a("")

    # Cross-vendor correlations
    a("## Cross-vendor correlations")
    a("")
    confirmed = correlate.get("confirmed_actor_correlations", [])
    if confirmed:
        a("### Same actor, different vendor code-names (semantic match, alias-confirmed)")
        a("")
        for c in confirmed:
            a(f"- **{c['a']['name']}** ({c['a']['vendor']}) ≡ **{c['b']['name']}** ({c['b']['vendor']}) "
              f"— shared identity `{', '.join(c['shared_aliases'])}`, semantic score {c['score']}")
        a("")
        a("> Keyword search cannot make this link — the vendors never use the same "
          "code-name in prose. PCMI's embedding retrieval bridges them; structured "
          "alias metadata confirms the identity.")
        a("")
    ttp = correlate.get("ttp_correlations", [])
    if ttp:
        a("### Shared TTPs documented by multiple vendors")
        a("")
        a("| TTP | Vendors | Memories |")
        a("|-----|---------|----------|")
        for t in ttp:
            a(f"| {t['label']} | {t['n_vendors']} ({_fmt_vendors(t['vendors'])}) | {t['n_memories']} |")
        a("")

    # Deterministic LLM-style distillation
    if distillation:
        a("## Deterministic LLM-style distillation → reusable knowledge")
        a("")
        a(
            "This phase is **not** an external LLM call. It uses deterministic heuristics "
            "over sparse alias/TTP signals to model what an analyst might ask an LLM to synthesize."
        )
        a("")
        a(
            f"PCMI generated **{distillation.get('discovered')}** cross-report discovery candidate(s) "
            f"from sparse signals and persisted **{distillation.get('persisted')}** as reusable memory "
            f"under `root.cti.distillation`."
        )
        a("")
        a("| Discovery | Signals | Confidence | Persisted memory |")
        a("|-----------|---------|------------|------------------|")
        for c in (distillation.get("candidates") or [])[:8]:
            signal_text = "; ".join(f"{s['kind']}={s['value']}" for s in c.get("signals", []))
            a(
                f"| `{c.get('relation_type')}`: {c.get('from_name')} ↔ {c.get('to_name')} "
                f"| {signal_text} | {c.get('confidence')} | `{c.get('memory_path')}` |"
            )
        a("")
        a(
            f"Graph persistence: **{distillation.get('links_added')}** typed link(s) added, "
            f"**{distillation.get('links_validated')}** existing link(s) validated/idempotent."
        )
        a("")

    # Real LLM inference distillation
    if llm_distillation and not llm_distillation.get("skipped"):
        mode = llm_distillation.get("mode") or "unknown"
        model = llm_distillation.get("model") or "n/a"
        a("## LLM inference distillation → reusable knowledge")
        a("")
        a(
            f"This phase **calls an external LLM** ({mode} mode, model `{model}`) with raw report text "
            "plus PCMI retrieval context. Candidates are schema-validated and rejected unless evidence "
            "quotes are grounded in the input context."
        )
        a("")
        a(
            f"Accepted **{llm_distillation.get('discovered')}** candidate(s); persisted "
            f"**{llm_distillation.get('persisted')}** under `root.cti.distillation.llm.*`. "
            f"Rejected: **{len(llm_distillation.get('rejected') or [])}**."
        )
        a("")
        a("| Discovery | Signals | Confidence | Persisted memory |")
        a("|-----------|---------|------------|------------------|")
        for c in (llm_distillation.get("candidates") or [])[:8]:
            signal_text = "; ".join(f"{s['kind']}={s['value']}" for s in c.get("signals", []))
            a(
                f"| `{c.get('relation_type')}`: {c.get('from_name')} ↔ {c.get('to_name')} "
                f"| {signal_text} | {c.get('confidence')} | `{c.get('memory_path')}` |"
            )
        a("")
        a(
            f"Graph persistence: **{llm_distillation.get('links_added')}** typed link(s) added, "
            f"**{llm_distillation.get('links_validated')}** existing link(s) validated/idempotent."
        )
        a("")

    # Temporal
    if temporal.get("demonstrated"):
        a("## Temporal evolution (append-only versioning)")
        a("")
        a(f"The actor profile at `{temporal.get('path')}` advanced from "
          f"v{temporal.get('version_before')} to v{temporal.get('version_now')} after a follow-up report. "
          f"An `as_of` query before the update still returns v{temporal.get('as_of_version')} — the prior "
          "assessment is retained, not overwritten.")
        a("")

    # Graph
    a("## Cognitive Graph")
    a("")
    start = graph.get("start") or {}
    a(f"- Traversal from **{start.get('name')}** ({start.get('vendor')}) reached a "
      f"{graph.get('subgraph_nodes')}-node subgraph; shortest cross-vendor chain "
      f"{ (graph.get('connected_chain') or {}).get('hops') } hops.")
    a(f"- Graph vertices (linked memories): {graph.get('graph_vertices')}")
    a(f"- **Knowledge accumulation**: {enrich.get('validated')} discovered correlation(s) "
      f"validated against the analyst-curated graph, {enrich.get('added')} new cross-vendor "
      "edge(s) added by PCMI.")
    a("")

    # Investigation session
    if session.get("promoted"):
        a("## Analyst investigation (agent session)")
        a("")
        a(f"An investigation session on **{session.get('subject')}** accumulated "
          f"{session.get('working_memories')} working-memory notes, then promoted "
          f"{session.get('promoted')} to long-term at `{session.get('target_prefix')}`.")
        a("")

    # Provenance & observability
    if observability:
        a("## Provenance & observability")
        a("")
        a(f"- Every write emitted a live event on PCMI's stream "
          f"(`{observability.get('events_observed')}` observed during this run: "
          f"{', '.join(t for t in (observability.get('event_types') or []) if t) or 'n/a'}).")
        a(f"- Every API request was audited: `{observability.get('audit_total')}` "
          "entries in the tenant audit log — full provenance for regulated CTI workflows.")
        a("")

    a("---")
    a("")
    a("*No operational IOCs. Actor names, malware families, CVEs and MITRE IDs are real; "
      "correlations are PCMI-derived.*")
    a("")
    a("*PCMI complements TI Mindmap HUB's STIX Constellation — it ingests the HUB's "
      "STIX 2.1 output and adds versioned `as_of` history, agent working-memory and "
      "enterprise controls; it does not reimplement the HUB's cross-report graph.*")
    a("")
    return "\n".join(lines)
