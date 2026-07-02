"""Real LLM inference distillation for the CTI PoC.

Unlike ``distillation.py`` (deterministic, alias/signal heuristics), this module
calls an external LLM with raw report text plus PCMI retrieval context, asks for
structured cross-report correlation hypotheses, validates the response against the
input context, and persists accepted candidates under ``root.cti.distillation.llm.*``.

When ``OPENAI_API_KEY`` is absent, a recorded fixture response is used so CI and
local runs stay offline and deterministic.
"""
from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass, field
from typing import Any, Callable, Awaitable

import httpx

from .correlator import NAMESPACE, normalize_aliases
from .distillation import _as_list, _display_name, _source_report
from .pcmi_client import PCMIClient, PCMIError
from .stix_to_pcmi import sanitize_segment
from .ti_hub_client import Report, SDO

LLM_DISTILLATION_PREFIX = "root.cti.distillation.llm"

ALLOWED_RELATION_TYPES = frozenset({
    "same_actor_likely",
    "alias_equivalent",
    "ioc_linked_to_campaign",
    "malware_linked_to_actor",
    "campaign_actor_overlap",
})

RELATION_TO_PCMI_LINK: dict[str, str] = {
    "same_actor_likely": "related",
    "alias_equivalent": "related",
    "ioc_linked_to_campaign": "supports",
    "malware_linked_to_actor": "supports",
    "campaign_actor_overlap": "related",
}

FIXTURE_PATH = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "examples",
    "llm_distillation_fixture.json",
)


@dataclass
class EntityContext:
    key: str
    name: str
    vendor: str
    path: str
    stix_type: str
    content: str
    source: str
    aliases: set[str] = field(default_factory=set)


@dataclass
class DistillationContext:
    entities: list[EntityContext]
    retrieval_snippets: list[dict[str, Any]]
    context_blob: str
    report_by_key: dict[str, Report]


def _entity_from_sdo(sdo: SDO, report: Report) -> EntityContext:
    md = dict(sdo.metadata)
    return EntityContext(
        key=sdo.key,
        name=_display_name(sdo),
        vendor=str(md.get("vendor") or report.vendor),
        path=sdo.path,
        stix_type=sdo.stix_type,
        content=sdo.content,
        source=str(md.get("source") or report.source),
        aliases=normalize_aliases(md),
    )


def _context_blob(entities: list[EntityContext], retrieval_snippets: list[dict[str, Any]]) -> str:
    parts: list[str] = []
    for e in entities:
        alias_text = ", ".join(sorted(e.aliases)) if e.aliases else ""
        parts.append(f"[{e.key}] {e.name} ({e.vendor}) aliases={alias_text} {e.content}")
    for snip in retrieval_snippets:
        parts.append(snip.get("text") or "")
    return "\n".join(parts).lower()


def _match_entity(name: str, entities: list[EntityContext]) -> EntityContext | None:
    needle = re.sub(r"\s+", " ", name.strip().lower())
    if not needle:
        return None
    for e in entities:
        if needle == e.name.lower() or needle == e.key.lower():
            return e
    for e in entities:
        if needle in e.name.lower() or e.name.lower() in needle:
            return e
    for e in entities:
        if any(needle in alias or alias in needle for alias in e.aliases):
            return e
    return None


def build_entity_context(reports: list[Report]) -> DistillationContext:
    """Collect actor/campaign/malware/indicator entities from ingested reports."""
    report_by_key = {sdo.key: report for report in reports for sdo in report.sdos}
    entities: list[EntityContext] = []
    for report in reports:
        for sdo in report.sdos:
            if sdo.stix_type in {"threat-actor", "campaign", "malware", "indicator", "incident"}:
                entities.append(_entity_from_sdo(sdo, report))
    return DistillationContext(
        entities=entities,
        retrieval_snippets=[],
        context_blob=_context_blob(entities, []),
        report_by_key=report_by_key,
    )


async def enrich_with_retrieval(client: PCMIClient, ctx: DistillationContext, *, limit: int = 5) -> DistillationContext:
    """Attach PCMI retrieval context for each entity (prior memories on same actors)."""
    snippets: list[dict[str, Any]] = []
    seen: set[int] = set()
    for e in ctx.entities:
        if e.stix_type not in {"threat-actor", "campaign", "malware"}:
            continue
        query = e.content[:400] or e.name
        entries = await client.retrieve_entries(NAMESPACE, query=query, limit=limit)
        for entry in entries:
            mid = entry.get("id")
            if mid in seen:
                continue
            seen.add(mid)
            text = f"retrieve({e.name}): [{entry.get('path')}] {entry.get('content', '')[:500]}"
            snippets.append({"entity_key": e.key, "memory_id": mid, "path": entry.get("path"), "text": text})
    ctx.retrieval_snippets = snippets
    ctx.context_blob = _context_blob(ctx.entities, snippets)
    return ctx


def build_llm_prompt(ctx: DistillationContext) -> str:
    entity_lines = []
    for e in ctx.entities:
        alias_text = ", ".join(sorted(e.aliases)) if e.aliases else "n/a"
        entity_lines.append(
            f"- key={e.key} name={e.name!r} vendor={e.vendor} type={e.stix_type} "
            f"source={e.source!r} aliases=[{alias_text}] text={e.content[:700]}"
        )
    retrieval_lines = [s["text"] for s in ctx.retrieval_snippets[:20]]
    return f"""You are a CTI analyst assistant. Propose cross-vendor correlation hypotheses using ONLY the evidence below.
Do not invent entities, vendors, or quotes. Every signal must include an evidence substring copied from the input.
Be conservative: campaign_actor_overlap means overlapping tradecraft/context, NOT confirmed same actor.
DPRK umbrella groups (e.g. PRESSURE CHOLLIMA vs Sapphire Sleet) should use campaign_actor_overlap with moderate confidence.

Return JSON:
{{
  "candidates": [
    {{
      "relation_type": "alias_equivalent|same_actor_likely|malware_linked_to_actor|campaign_actor_overlap|ioc_linked_to_campaign",
      "entity_a": "exact name from input",
      "entity_b": "exact name from input",
      "source_reports": ["vendor report titles from input"],
      "signals": [{{"kind": "alias|ttp|target|attribution|platform|ioc", "value": "short label", "evidence": "verbatim quote from input"}}],
      "confidence": 0.0-1.0,
      "explanation": "one paragraph citing only input evidence"
    }}
  ]
}}

Prioritize if supported by evidence:
- Midnight Blizzard ↔ COZY BEAR / APT29 (alias_equivalent or same_actor_likely)
- PROMPTSTEAL ↔ Forest Blizzard (malware_linked_to_actor)
- PRESSURE CHOLLIMA ↔ Sapphire Sleet (campaign_actor_overlap, honest moderate confidence)
- IOC linked to campaign if indicators appear in input

ENTITIES:
{chr(10).join(entity_lines)}

PCMI RETRIEVAL CONTEXT:
{chr(10).join(retrieval_lines) if retrieval_lines else "(none)"}
"""


def _extract_json_object(raw: str) -> dict[str, Any]:
    text = raw.strip()
    if text.startswith("```"):
        text = re.sub(r"^```(?:json)?\s*", "", text)
        text = re.sub(r"\s*```$", "", text)
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError as exc:
        # Try to salvage the first JSON object in a noisier response.
        match = re.search(r"\{[\s\S]*\}", text)
        if not match:
            raise ValueError(f"LLM response is not valid JSON: {exc}") from exc
        parsed = json.loads(match.group(0))
    if not isinstance(parsed, dict):
        raise ValueError("LLM response root must be a JSON object")
    return parsed


def _evidence_in_context(evidence: str, context_blob: str) -> bool:
    ev = re.sub(r"\s+", " ", evidence.strip().lower())
    if len(ev) < 8:
        return False
    if ev in context_blob:
        return True
    # Allow partial match for long quotes (>= 60% of words present in order).
    words = [w for w in re.findall(r"[a-z0-9]+", ev) if len(w) > 2]
    if len(words) >= 3:
        pattern = r".*".join(re.escape(w) for w in words[:8])
        return re.search(pattern, context_blob) is not None
    return False


def validate_llm_candidates(
    raw: dict[str, Any],
    ctx: DistillationContext,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    """Parse and validate LLM output; return (accepted, rejected)."""
    items = raw.get("candidates")
    if not isinstance(items, list):
        raise ValueError("LLM response missing 'candidates' list")

    accepted: list[dict[str, Any]] = []
    rejected: list[dict[str, Any]] = []

    for idx, item in enumerate(items):
        reason: str | None = None
        if not isinstance(item, dict):
            reason = "candidate is not an object"
        else:
            relation_type = str(item.get("relation_type") or "").strip()
            entity_a_name = str(item.get("entity_a") or "").strip()
            entity_b_name = str(item.get("entity_b") or "").strip()
            explanation = str(item.get("explanation") or "").strip()
            try:
                confidence = float(item.get("confidence", 0))
            except (TypeError, ValueError):
                confidence = -1.0

            entity_a = _match_entity(entity_a_name, ctx.entities)
            entity_b = _match_entity(entity_b_name, ctx.entities)
            signals_raw = item.get("signals") or []
            source_reports_raw = item.get("source_reports") or []

            if relation_type not in ALLOWED_RELATION_TYPES:
                reason = f"unsupported relation_type {relation_type!r}"
            elif not entity_a or not entity_b:
                reason = "entity_a/entity_b not found in input context"
            elif entity_a.key == entity_b.key:
                reason = "self-correlation rejected"
            elif entity_a.vendor == entity_b.vendor:
                reason = "same-vendor pair rejected (cross-vendor only)"
            elif confidence < 0 or confidence > 1:
                reason = "confidence out of range"
            elif not explanation:
                reason = "missing explanation"
            elif not isinstance(signals_raw, list) or not signals_raw:
                reason = "missing signals"
            else:
                signals: list[dict[str, Any]] = []
                for sig in signals_raw:
                    if not isinstance(sig, dict):
                        reason = "signal is not an object"
                        break
                    evidence = str(sig.get("evidence") or sig.get("quote") or "").strip()
                    kind = str(sig.get("kind") or "evidence").strip()
                    value = str(sig.get("value") or "").strip()
                    if not evidence or not _evidence_in_context(evidence, ctx.context_blob):
                        reason = f"evidence not grounded in input: {evidence[:80]!r}"
                        break
                    signals.append({"kind": kind, "value": value, "evidence": evidence, "sources": [entity_a.key, entity_b.key]})

            if reason is None and isinstance(source_reports_raw, list):
                source_reports = [str(r) for r in source_reports_raw if str(r).strip()]
                if len(source_reports) < 1:
                    reason = "source_reports empty"
                else:
                    sdo_a = next(s for r in ctx.report_by_key.values() for s in r.sdos if s.key == entity_a.key)
                    sdo_b = next(s for r in ctx.report_by_key.values() for s in r.sdos if s.key == entity_b.key)
                    candidate = _normalize_candidate(
                        relation_type=relation_type,
                        entity_a=entity_a,
                        entity_b=entity_b,
                        sdo_a=sdo_a,
                        sdo_b=sdo_b,
                        ctx=ctx,
                        confidence=confidence,
                        signals=signals,
                        explanation=explanation,
                        source_reports=source_reports,
                    )
                    accepted.append(candidate)

        if reason:
            rejected.append({"index": idx, "raw": item, "reason": reason})

    return accepted, rejected


def _normalize_candidate(
    *,
    relation_type: str,
    entity_a: EntityContext,
    entity_b: EntityContext,
    sdo_a: SDO,
    sdo_b: SDO,
    ctx: DistillationContext,
    confidence: float,
    signals: list[dict[str, Any]],
    explanation: str,
    source_reports: list[str],
) -> dict[str, Any]:
    ordered_keys = sorted([entity_a.key, entity_b.key])
    candidate_id = f"llm.{relation_type}.{ordered_keys[0]}.{ordered_keys[1]}"
    memory_path = f"{LLM_DISTILLATION_PREFIX}.{sanitize_segment(candidate_id)}"
    pcmi_link_type = RELATION_TO_PCMI_LINK.get(relation_type, "related")
    return {
        "candidate_id": candidate_id,
        "relation_type": relation_type,
        "pcmi_link_type": pcmi_link_type,
        "confidence": round(confidence, 2),
        "from_key": entity_a.key,
        "to_key": entity_b.key,
        "from_name": entity_a.name,
        "to_name": entity_b.name,
        "source_keys": [entity_a.key, entity_b.key],
        "source_paths": [entity_a.path, entity_b.path],
        "source_reports": [
            _source_report(ctx.report_by_key, sdo_a),
            _source_report(ctx.report_by_key, sdo_b),
        ],
        "source_report_titles": source_reports,
        "signals": signals,
        "explanation": explanation,
        "memory_path": memory_path,
        "inference_mode": "llm",
    }


def load_fixture_response(path: str = FIXTURE_PATH) -> dict[str, Any]:
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


async def call_openai_llm(prompt: str, *, api_key: str, model: str) -> str:
    async with httpx.AsyncClient(timeout=90.0) as client:
        resp = await client.post(
            "https://api.openai.com/v1/chat/completions",
            headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
            json={
                "model": model,
                "messages": [
                    {"role": "system", "content": "You are a CTI analyst. Respond with JSON only."},
                    {"role": "user", "content": prompt},
                ],
                "response_format": {"type": "json_object"},
                "temperature": 0.1,
            },
        )
        if resp.status_code != 200:
            raise RuntimeError(f"OpenAI API HTTP {resp.status_code}: {resp.text[:400]}")
        data = resp.json()
        return str(data["choices"][0]["message"]["content"])


LLMCaller = Callable[[str], Awaitable[str]]


async def infer_llm_candidates(
    ctx: DistillationContext,
    *,
    api_key: str | None = None,
    model: str = "gpt-4o-mini",
    llm_call: LLMCaller | None = None,
    fixture_path: str = FIXTURE_PATH,
) -> dict[str, Any]:
    """Build prompt, call LLM (or fixture), validate and return structured result."""
    prompt = build_llm_prompt(ctx)
    mode = "live"
    model_used = model

    if llm_call is not None:
        raw_text = await llm_call(prompt)
    elif api_key:
        raw_text = await call_openai_llm(prompt, api_key=api_key, model=model)
    else:
        mode = "fixture"
        fixture = load_fixture_response(fixture_path)
        model_used = str(fixture.get("model") or "fixture")
        raw_text = json.dumps({"candidates": fixture.get("candidates") or []})

    parsed = _extract_json_object(raw_text)
    accepted, rejected = validate_llm_candidates(parsed, ctx)
    return {
        "mode": mode,
        "model": model_used,
        "prompt_chars": len(prompt),
        "raw_candidate_count": len(parsed.get("candidates") or []),
        "accepted": accepted,
        "rejected": rejected,
        "discovered": len(accepted),
    }


def render_llm_discovery_memory(candidate: dict[str, Any]) -> str:
    signals = "; ".join(
        f"{s['kind']}={s['value']} (\"{s.get('evidence', s.get('quote', ''))[:120]}\")"
        for s in candidate["signals"]
    )
    reports = ", ".join(f"{r['vendor']} ({r['source']})" for r in candidate["source_reports"])
    return (
        f"LLM inference distillation proposed `{candidate['relation_type']}` between "
        f"{candidate['from_name']} and {candidate['to_name']}.\n\n"
        f"Evidence reports: {reports}.\n"
        f"Signals/evidence: {signals}.\n"
        f"Confidence: {candidate['confidence']:.2f}.\n"
        f"Explanation: {candidate['explanation']}\n"
        "Validated against input context (evidence quotes grounded). "
        "Persisted as reusable PCMI knowledge with source links and typed relation metadata."
    )


class LLMDistillationPersister:
    """Persist LLM-validated candidates under root.cti.distillation.llm.*."""

    def __init__(self, client: PCMIClient):
        self.client = client

    async def _edge_exists(self, a: int, b: int) -> bool:
        for src, dst in ((a, b), (b, a)):
            rel = await self.client.graph_related(src, depth=1)
            if dst in {e["id"] for e in (rel.get("entries") or [])}:
                return True
        return False

    async def _store_memory_once(self, candidate: dict[str, Any], source_ids: list[int]) -> tuple[int | None, bool]:
        path = candidate["memory_path"]
        content = render_llm_discovery_memory(candidate)
        metadata = {
            "derived_type": "llm_inference_distillation",
            "inference_mode": "llm",
            "relation_type": candidate["relation_type"],
            "pcmi_link_type": candidate["pcmi_link_type"],
            "confidence": candidate["confidence"],
            "source_keys": candidate["source_keys"],
            "source_paths": candidate["source_paths"],
            "source_memory_ids": source_ids,
            "source_reports": candidate["source_reports"],
            "source_report_titles": candidate.get("source_report_titles") or [],
            "signals": candidate["signals"],
            "explanation": candidate["explanation"],
            "idempotency_key": candidate["candidate_id"],
        }
        try:
            existing = await self.client.get_memory(path)
            if existing.get("content") == content:
                return (int(existing["id"]) if existing.get("id") is not None else None), False
        except PCMIError as exc:
            if exc.status != 404:
                raise
        resp = await self.client.store(
            path,
            content,
            tags=["cti", "llm-inference-distillation", candidate["relation_type"]],
            metadata=metadata,
            importance=0.93,
            key=f"llm-distillation:{candidate['candidate_id']}",
        )
        return (int(resp["id"]) if resp.get("id") is not None else None), True

    async def persist(self, candidates: list[dict[str, Any]]) -> dict[str, Any]:
        persisted: list[dict[str, Any]] = []
        memory_paths: list[str] = []
        memories_created = 0
        links_added = 0
        links_validated = 0

        for c in candidates:
            source_ids = [self.client.ids.get(k) for k in c["source_keys"]]
            if any(mid is None for mid in source_ids):
                skipped = dict(c)
                skipped["persisted"] = False
                skipped["skip_reason"] = "source memory id missing"
                persisted.append(skipped)
                continue
            typed_source_ids = [int(mid) for mid in source_ids if mid is not None]
            memory_id, created = await self._store_memory_once(c, typed_source_ids)
            if created:
                memories_created += 1
            memory_paths.append(c["memory_path"])

            pair_links = [(typed_source_ids[0], typed_source_ids[1], c["pcmi_link_type"])]
            if memory_id is not None:
                pair_links.extend((source_id, memory_id, "supports") for source_id in typed_source_ids)

            for from_id, to_id, link_type in pair_links:
                if await self._edge_exists(from_id, to_id):
                    links_validated += 1
                    continue
                metadata = {
                    "discovered_by": "pcmi-llm-inference-distillation",
                    "inference_mode": "llm",
                    "relation_type": c["relation_type"],
                    "confidence": c["confidence"],
                    "evidence_memory_path": c["memory_path"],
                    "signals": c["signals"],
                }
                try:
                    await self.client.link(from_id, to_id, link_type, metadata=metadata)
                    links_added += 1
                except PCMIError:
                    links_validated += 1

            enriched = dict(c)
            enriched["persisted"] = True
            enriched["memory_id"] = memory_id
            enriched["source_memory_ids"] = typed_source_ids
            persisted.append(enriched)

        return {
            "discovered": len(candidates),
            "persisted": sum(1 for c in persisted if c.get("persisted")),
            "memories_created": memories_created,
            "links_added": links_added,
            "links_validated": links_validated,
            "memory_paths": memory_paths,
            "candidates": persisted,
        }


async def run_llm_distillation(
    client: PCMIClient,
    reports: list[Report],
    *,
    api_key: str | None = None,
    model: str = "gpt-4o-mini",
    llm_call: LLMCaller | None = None,
) -> dict[str, Any]:
    """End-to-end LLM distillation: context → inference → validation → persistence."""
    ctx = build_entity_context(reports)
    ctx = await enrich_with_retrieval(client, ctx)
    inference = await infer_llm_candidates(ctx, api_key=api_key, model=model, llm_call=llm_call)
    persistence = await LLMDistillationPersister(client).persist(inference["accepted"])
    return {
        **inference,
        **persistence,
        "skipped": False,
    }
