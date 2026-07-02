"""TI Mindmap HUB connector (demo mode).

The real TI Mindmap HUB exposes AI-generated threat-intel artefacts via an API
that needs a key. In ``demo`` mode this connector reads the bundled
``vendor_reports_cti_dataset.json`` (40 real findings + 25 correlations drawn from
Mandiant M-Trends 2026, CrowdStrike GTR 2026, Unit 42 GIRR 2026 and Microsoft
Threat Intelligence) and reshapes it into pseudo-STIX *reports*:

  * Nodes are grouped by ``metadata.vendor`` — each vendor becomes one report
    (a STIX-bundle-like object), ordered by report date so the demo plays out
    longitudinally.
  * Each node becomes an SDO whose ``stix_type`` is mapped from ``metadata.kind``.
  * Each dataset link becomes an SRO (relationship). Links are *global* — they
    cross report boundaries on purpose, which is exactly the cross-vendor /
    longitudinal value PCMI is meant to surface.
"""
from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass, field

# metadata.kind  ->  STIX SDO type. Anything unmapped becomes a generic "note".
KIND_TO_STIX: dict[str, str] = {
    "threat_actor": "threat-actor",
    "threat_actor_activity": "threat-actor",
    "threat_actor_campaign": "campaign",
    "campaign": "campaign",
    "malware": "malware",
    "vulnerability": "vulnerability",
    "indicator": "indicator",
    "incident": "incident",
    "event": "incident",
}


def stix_type(kind: str | None) -> str:
    return KIND_TO_STIX.get(kind or "", "note")


@dataclass
class SDO:
    """A STIX-ish domain object (one dataset node)."""

    key: str
    stix_type: str
    path: str
    content: str
    tags: list[str]
    metadata: dict
    kind: str


@dataclass
class SRO:
    """A STIX-ish relationship object (one dataset link)."""

    source: str  # source node key
    target: str  # target node key
    relationship_type: str
    weight: float
    rationale: str


@dataclass
class Report:
    """All findings from a single vendor — the unit PCMI ingests at a time."""

    vendor: str
    source: str
    report_date: str
    data_period: str
    sort_key: tuple[int, int, int]
    sdos: list[SDO] = field(default_factory=list)

    @property
    def node_count(self) -> int:
        return len(self.sdos)


def _parse_date(value: str | None) -> tuple[int, int, int]:
    """Tolerant date parse for ordering: handles ``2026-03-23``, ``2026``,
    ``2025-2026`` and ``None``. Missing parts default low so partial dates sort
    before fully-specified later ones."""
    if not value:
        return (9999, 12, 31)
    m = re.match(r"(\d{4})(?:-(\d{2}))?(?:-(\d{2}))?", str(value))
    if not m:
        return (9999, 12, 31)
    year = int(m.group(1))
    month = int(m.group(2) or 1)
    day = int(m.group(3) or 1)
    # Guard against year ranges like "2025-2026" (captures month "20") and other
    # out-of-range parts — treat as the start of the period.
    if not 1 <= month <= 12:
        month = 1
    if not 1 <= day <= 31:
        day = 1
    return (year, month, day)


_DEFAULT_STIX_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "examples", "tihub_stix")


class TiHubClient:
    """Yields ordered reports + relationships from the CTI corpus.

    Two modes:
      * ``demo`` — the curated ``vendor_reports_cti_dataset.json`` (custom shape).
      * ``live`` — real **STIX 2.1 bundles** (the format TI Mindmap HUB emits),
        read from ``stix_dir`` (``examples/tihub_stix/`` by default), which holds
        TI Mindmap HUB's own published example plus per-vendor bundles transcoded
        into that schema. Both modes feed the identical downstream pipeline.
    """

    def __init__(self, dataset_path: str, mode: str = "demo", stix_dir: str | None = None):
        self.dataset_path = dataset_path
        self.mode = mode
        self.stix_dir = stix_dir or _DEFAULT_STIX_DIR
        self._raw: dict | None = None
        self._reports: list[Report] = []
        self._relationships: list[SRO] = []
        self._name = ""
        self._description = ""
        self.loaded_bundles: list[str] = []

    def load(self) -> "TiHubClient":
        if self.mode == "demo":
            with open(self.dataset_path, encoding="utf-8") as fh:
                self._raw = json.load(fh)
            self._name = self._raw.get("name", "")
            self._description = self._raw.get("description", "")
            self._reports = self._demo_reports()
            self._relationships = self._demo_relationships()
        elif self.mode in ("live", "stix"):
            from .stix_ingest import load_stix_bundles

            reports, relationships, loaded = load_stix_bundles(self.stix_dir)
            if not reports:
                raise FileNotFoundError(f"no STIX 2.1 bundles found in {self.stix_dir}")
            self._reports, self._relationships, self.loaded_bundles = reports, relationships, loaded
            self._name = f"TI Mindmap HUB — live STIX 2.1 ingest ({len(loaded)} bundles)"
            self._description = "Real STIX 2.1 bundles: " + ", ".join(loaded)
        else:
            raise ValueError(f"unknown TI_HUB_MODE={self.mode!r} (use demo|live)")
        return self

    @property
    def name(self) -> str:
        return self._name

    @property
    def description(self) -> str:
        return self._description

    def reports(self) -> list[Report]:
        return self._reports

    def relationships(self) -> list[SRO]:
        return self._relationships

    def _demo_reports(self) -> list[Report]:
        """Group nodes by vendor into reports, ordered by report date ascending."""
        assert self._raw is not None, "call load() first"
        groups: dict[str, Report] = {}
        for node in self._raw["nodes"]:
            md = node.get("metadata", {})
            vendor = md.get("vendor", "unknown")
            report = groups.get(vendor)
            if report is None:
                report = Report(
                    vendor=vendor,
                    source=md.get("source", vendor),
                    report_date=md.get("report_date", "") or "",
                    data_period=md.get("data_period", "") or "",
                    sort_key=_parse_date(md.get("report_date")),
                )
                groups[vendor] = report
            report.sdos.append(
                SDO(
                    key=node["key"],
                    stix_type=stix_type(md.get("kind")),
                    path=node["path"],
                    content=node["content"],
                    tags=list(node.get("tags", [])),
                    metadata=md,
                    kind=md.get("kind", ""),
                )
            )
        return sorted(groups.values(), key=lambda r: (r.sort_key, r.vendor))

    def _demo_relationships(self) -> list[SRO]:
        assert self._raw is not None, "call load() first"
        return [
            SRO(
                source=link["from"],
                target=link["to"],
                relationship_type=link.get("type", "related"),
                weight=float(link.get("weight", 1.0)),
                rationale=link.get("rationale", ""),
            )
            for link in self._raw.get("links", [])
        ]
