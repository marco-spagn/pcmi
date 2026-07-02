"""Deterministic LLM-style CTI distillation for the PoC.

This module intentionally does not call an external LLM. It models the kind of
cross-report synthesis an analyst would ask an LLM to perform, but keeps the demo
repeatable by deriving every claim from explicit sparse signals in the ingested
vendor reports: aliases, attribution labels, targets, tools, platforms, TTPs and
campaign metadata.
"""
from __future__ import annotations

import json
from typing import Any

from .correlator import NAMESPACE, normalize_aliases
from .pcmi_client import PCMIClient, PCMIError
from .stix_to_pcmi import sanitize_segment
from .ti_hub_client import Report, SDO

DISTILLATION_PREFIX = "root.cti.distillation"


def _as_list(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, list):
        return [str(v) for v in value if str(v).strip()]
    return [str(value)] if str(value).strip() else []


def _blob(sdo: SDO) -> str:
    metadata_values: list[str] = []
    for value in sdo.metadata.values():
        if isinstance(value, list):
            metadata_values.extend(str(v) for v in value)
        else:
            metadata_values.append(str(value))
    return " ".join([sdo.content, " ".join(sdo.tags), " ".join(metadata_values)]).lower()


def _display_name(sdo: SDO) -> str:
    md = sdo.metadata
    return str(
        md.get("actor")
        or md.get("family")
        or md.get("campaign_name")
        or md.get("cluster")
        or sdo.path.rsplit(".", 1)[-1].replace("_", " ").title()
    )


def _identity_aliases(sdo: SDO) -> set[str]:
    md = dict(sdo.metadata)
    alias_parts = []
    for key in ("actor", "public_alias", "attributed_to", "operator", "compromised_actor"):
        alias_parts.extend(_as_list(md.get(key)))
    if alias_parts:
        md["actor"] = " / ".join(alias_parts)
    return normalize_aliases(md)


def _signals_for_pair(a: SDO, b: SDO) -> list[dict[str, Any]]:
    """Shared sparse signals used for non-alias campaign/actor hypotheses."""
    a_blob, b_blob = _blob(a), _blob(b)
    probes = [
        ("attribution", "DPRK", ("dprk", "north korean", "north korea", "chollima", "lazarus", "tradertraitor")),
        ("target", "cryptocurrency", ("cryptocurrency", "crypto", "bybit", "wallet")),
        ("platform", "macOS", ("macos",)),
        ("ttp", "social engineering", ("social engineering", "user-driven", "trojanized software")),
        ("ttp", "supply chain", ("supply chain", "trusted connectivity")),
        ("objective", "credential theft", ("credential", "credentials")),
    ]
    signals: list[dict[str, Any]] = []
    for kind, value, terms in probes:
        a_hit = any(term in a_blob for term in terms)
        b_hit = any(term in b_blob for term in terms)
        if a_hit and b_hit:
            signals.append({"kind": kind, "value": value, "sources": [a.key, b.key]})
    return signals


def _source_report(report_by_key: dict[str, Report], sdo: SDO) -> dict[str, str]:
    report = report_by_key[sdo.key]
    return {"vendor": report.vendor, "source": report.source, "report_date": report.report_date}


def _candidate_id(relation_type: str, a: SDO, b: SDO) -> str:
    ordered = sorted([a.key, b.key])
    return f"{relation_type}.{ordered[0]}.{ordered[1]}"


def _memory_path(candidate_id: str) -> str:
    return f"{DISTILLATION_PREFIX}.{sanitize_segment(candidate_id)}"


def _candidate(
    *,
    relation_type: str,
    pcmi_link_type: str,
    a: SDO,
    b: SDO,
    report_by_key: dict[str, Report],
    confidence: float,
    signals: list[dict[str, Any]],
    explanation: str,
) -> dict[str, Any]:
    cid = _candidate_id(relation_type, a, b)
    source_reports = [_source_report(report_by_key, a), _source_report(report_by_key, b)]
    return {
        "candidate_id": cid,
        "relation_type": relation_type,
        "pcmi_link_type": pcmi_link_type,
        "confidence": round(confidence, 2),
        "from_key": a.key,
        "to_key": b.key,
        "from_name": _display_name(a),
        "to_name": _display_name(b),
        "source_keys": [a.key, b.key],
        "source_paths": [a.path, b.path],
        "source_reports": source_reports,
        "signals": signals,
        "explanation": explanation,
        "memory_path": _memory_path(cid),
    }


def _dedupe(candidates: list[dict[str, Any]]) -> list[dict[str, Any]]:
    by_id: dict[str, dict[str, Any]] = {}
    for c in candidates:
        existing = by_id.get(c["candidate_id"])
        if existing is None or c["confidence"] > existing["confidence"]:
            by_id[c["candidate_id"]] = c
    return sorted(by_id.values(), key=lambda c: (-c["confidence"], c["relation_type"], c["candidate_id"]))


def discover_cross_report_knowledge(reports: list[Report]) -> list[dict[str, Any]]:
    """Infer reusable cross-report knowledge from sparse vendor signals.

    The function consumes only report SDOs, not curated dataset relationships, so
    tests can prove that discoveries are generated at runtime.
    """
    report_by_key = {sdo.key: report for report in reports for sdo in report.sdos}
    sdos = [sdo for report in reports for sdo in report.sdos]
    candidates: list[dict[str, Any]] = []

    for i, a in enumerate(sdos):
        for b in sdos[i + 1:]:
            if report_by_key[a.key].vendor == report_by_key[b.key].vendor:
                continue

            aliases_a = _identity_aliases(a)
            aliases_b = _identity_aliases(b)
            shared_aliases = sorted(aliases_a & aliases_b)
            if shared_aliases:
                relation_type = "alias_equivalent"
                pcmi_type = "related"
                confidence = 0.94
                explanation = (
                    f"{_display_name(a)} and {_display_name(b)} share structured alias token(s) "
                    f"{', '.join(shared_aliases)} across different vendor reports."
                )
                if a.stix_type == "malware" or b.stix_type == "malware":
                    relation_type = "malware_linked_to_actor"
                    pcmi_type = "supports"
                    confidence = 0.88
                    malware = a if a.stix_type == "malware" else b
                    actor = b if malware is a else a
                    explanation = (
                        f"{_display_name(malware)} is attributed to alias token(s) "
                        f"{', '.join(shared_aliases)}, which another vendor maps to "
                        f"{_display_name(actor)}."
                    )
                candidates.append(
                    _candidate(
                        relation_type=relation_type,
                        pcmi_link_type=pcmi_type,
                        a=a,
                        b=b,
                        report_by_key=report_by_key,
                        confidence=confidence,
                        signals=[{"kind": "alias", "value": alias, "sources": [a.key, b.key]} for alias in shared_aliases],
                        explanation=explanation,
                    )
                )
                continue

            signals = _signals_for_pair(a, b)
            signal_values = {s["value"] for s in signals}
            actorish = a.stix_type in {"threat-actor", "campaign", "incident"} and b.stix_type in {
                "threat-actor",
                "campaign",
                "incident",
            }
            if actorish and {"DPRK", "cryptocurrency", "macOS"}.issubset(signal_values):
                candidates.append(
                    _candidate(
                        relation_type="campaign_actor_overlap",
                        pcmi_link_type="related",
                        a=a,
                        b=b,
                        report_by_key=report_by_key,
                        confidence=0.74,
                        signals=signals,
                        explanation=(
                            f"{_display_name(a)} and {_display_name(b)} are not claimed as the same actor. "
                            "The reusable hypothesis is that both reports describe overlapping DPRK crypto "
                            "operations: DPRK/North Korea attribution, cryptocurrency targeting and macOS "
                            "execution appear in both evidence sets."
                        ),
                    )
                )

    return _dedupe(candidates)


def render_discovery_memory(candidate: dict[str, Any]) -> str:
    signals = "; ".join(f"{s['kind']}={s['value']}" for s in candidate["signals"])
    reports = ", ".join(f"{r['vendor']} ({r['source']})" for r in candidate["source_reports"])
    return (
        f"LLM-style distillation discovered `{candidate['relation_type']}` between "
        f"{candidate['from_name']} and {candidate['to_name']}.\n\n"
        f"Evidence reports: {reports}.\n"
        f"Signals used: {signals}.\n"
        f"Confidence: {candidate['confidence']:.2f}.\n"
        f"Explanation: {candidate['explanation']}\n"
        "Persisted as reusable PCMI knowledge with source links and typed relation metadata."
    )


class DistillationPersister:
    """Persist discovery candidates as reusable memories plus graph links."""

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
        content = render_discovery_memory(candidate)
        metadata = {
            "derived_type": "llm_style_distillation",
            "relation_type": candidate["relation_type"],
            "pcmi_link_type": candidate["pcmi_link_type"],
            "confidence": candidate["confidence"],
            "source_keys": candidate["source_keys"],
            "source_paths": candidate["source_paths"],
            "source_memory_ids": source_ids,
            "source_reports": candidate["source_reports"],
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
            tags=["cti", "llm-style-distillation", candidate["relation_type"]],
            metadata=metadata,
            importance=0.92,
            key=f"distillation:{candidate['candidate_id']}",
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
                    "discovered_by": "pcmi-llm-style-distillation",
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
            "summary_json": json.dumps(
                [
                    {
                        "relation_type": c["relation_type"],
                        "from": c["from_name"],
                        "to": c["to_name"],
                        "confidence": c["confidence"],
                        "signals": [s["value"] for s in c["signals"]],
                    }
                    for c in persisted
                ],
                ensure_ascii=False,
            ),
        }
