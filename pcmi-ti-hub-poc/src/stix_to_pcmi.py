"""Map pseudo-STIX reports into PCMI memories + typed Cognitive Graph links.

Two critical rules are taken from the PCMI project, not assumed:

* **ltree path sanitisation** — ``docs/DATA-MODEL.md`` documents paths as ltree
  (``root.team.project.context``). ltree labels accept only ``[A-Za-z0-9_]`` and
  ``.`` as separator, so every segment is sanitised to that set.

* **link types** — ``migrations/021_link_type_check.sql`` adds a DB CHECK that
  constrains ``memory_links.link_type`` to ``causal | temporal | contradicts |
  supports | related`` (+ ``duplicate``, reserved for dedup). Only the five
  user-assignable types are emitted here. STIX relationship verbs are mapped onto
  them; native PCMI types pass straight through.
"""
from __future__ import annotations

import re
from typing import Any

from .pcmi_client import PCMIClient
from .ti_hub_client import SRO, Report

# STIX relationship_type -> PCMI link_type. Native PCMI types are handled first
# (pass-through) in map_link_type(); this table only covers STIX verbs.
STIX_REL_TO_LINK: dict[str, str] = {
    "uses": "causal",
    "targets": "causal",
    "exploits": "causal",
    "delivers": "temporal",
    "drops": "temporal",
    "attributed-to": "supports",
    "indicates": "supports",
    "variant-of": "related",
    "related-to": "related",
}

PCMI_NATIVE: frozenset[str] = frozenset(
    {"causal", "temporal", "contradicts", "supports", "related"}
)

# Per-kind retrieval importance (0..1) — boosts the hybrid ranking so the
# high-signal objects (actors, campaigns, incidents) surface first.
IMPORTANCE_BY_STIX: dict[str, float] = {
    "threat-actor": 0.85,
    "campaign": 0.80,
    "incident": 0.80,
    "vulnerability": 0.72,
    "malware": 0.70,
    "indicator": 0.60,
    "note": 0.50,
}

_NON_LTREE = re.compile(r"[^A-Za-z0-9_]")
_MULTI_US = re.compile(r"_+")


def map_link_type(relationship_type: str) -> str:
    """STIX verb (or native PCMI type) -> one of the five PCMI link types."""
    rel = (relationship_type or "").strip().lower()
    if rel in PCMI_NATIVE:
        return rel
    return STIX_REL_TO_LINK.get(rel, "related")


def sanitize_segment(segment: str) -> str:
    """Coerce one path label into a valid ltree segment ([A-Za-z0-9_])."""
    s = _NON_LTREE.sub("_", segment.strip().lower())
    s = _MULTI_US.sub("_", s).strip("_")
    return s or "x"


def sanitize_path(path: str) -> str:
    segments = [sanitize_segment(p) for p in path.split(".") if p.strip()]
    return ".".join(segments) if segments else "root.cti.unknown"


class StixToPcmi:
    """Ingests reports (SDOs -> memories) and relationships (SROs -> links)."""

    def __init__(self, client: PCMIClient):
        self.client = client

    async def ingest_report(self, report: Report) -> dict[str, Any]:
        """Store every SDO in ``report`` as a PCMI memory; record key -> id."""
        by_type: dict[str, int] = {}
        for sdo in report.sdos:
            path = sanitize_path(sdo.path)
            metadata = dict(sdo.metadata)
            metadata["stix_type"] = sdo.stix_type
            metadata["pcmi_key"] = sdo.key
            tags = list(sdo.tags)
            if sdo.stix_type not in tags:
                tags.append(sdo.stix_type)
            await self.client.store(
                path,
                sdo.content,
                tags=tags,
                metadata=metadata,
                importance=IMPORTANCE_BY_STIX.get(sdo.stix_type, 0.5),
                key=sdo.key,
            )
            by_type[sdo.stix_type] = by_type.get(sdo.stix_type, 0) + 1
        return {"vendor": report.vendor, "stored": report.node_count, "by_type": by_type}

    async def link_relationships(self, relationships: list[SRO]) -> dict[str, Any]:
        """Create one typed graph edge per SRO whose endpoints are both stored.

        Links are flushed after all reports are ingested because a relationship
        can connect findings from two different vendors (different reports), so
        both ``memory.<id>`` endpoints must already exist.
        """
        ids = self.client.ids
        created = 0
        skipped = 0
        by_type: dict[str, int] = {}
        for sro in relationships:
            a = ids.get(sro.source)
            b = ids.get(sro.target)
            if a is None or b is None:
                skipped += 1
                continue
            link_type = map_link_type(sro.relationship_type)
            metadata: dict[str, Any] = {"weight": sro.weight, "stix_relationship_type": sro.relationship_type}
            if sro.rationale:
                metadata["rationale"] = sro.rationale
            await self.client.link(a, b, link_type, metadata=metadata)
            created += 1
            by_type[link_type] = by_type.get(link_type, 0) + 1
        return {"created": created, "skipped": skipped, "by_type": by_type}
