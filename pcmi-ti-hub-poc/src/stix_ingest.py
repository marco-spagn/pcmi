"""Ingest real **STIX 2.1 bundles** — the exact format TI Mindmap HUB emits.

This is the ``TI_HUB_MODE=live`` path. Instead of the bundled demo JSON, it reads
STIX 2.1 bundles (``{"type":"bundle","objects":[...]}``) — either the bundles TI
Mindmap HUB publishes / exports, or ones transcoded into that schema — and reshapes
them into the same ``Report``/``SDO``/``SRO`` objects the rest of the pipeline
consumes, so ingest, correlation, graph, temporal, sessions, etc. all run unchanged.

STIX 2.1 objects handled (matching TI Mindmap HUB's data model):
  SDOs: report (container), threat-actor (with ``aliases``), intrusion-set,
        campaign, malware (``malware_types``), tool, indicator (``pattern``),
        attack-pattern (MITRE ``external_references``), vulnerability (CVE), note.
  SROs: relationship (``relationship_type``, ``source_ref`` → ``target_ref``).

Vertices are keyed by their STIX ``id`` so relationships resolve across bundles.
"""
from __future__ import annotations

import glob
import json
import os
import re

from .ti_hub_client import SDO, SRO, Report, _parse_date

# The corpus namespace — the SAME prefix the demo uses, so the correlator's
# NAMESPACE (root.cti.vendor_reports) covers live-mode memories too.
CORPUS_NS = "root.cti.vendor_reports"

# STIX 2.1 SDO type -> PCMI stix_type bucket (mirrors ti_hub_client.KIND_TO_STIX).
STIX_TYPE_TO_PCMI: dict[str, str] = {
    "threat-actor": "threat-actor",
    "intrusion-set": "threat-actor",
    "campaign": "campaign",
    "malware": "malware",
    "vulnerability": "vulnerability",
    "indicator": "indicator",
    "incident": "incident",
    "tool": "note",
    "attack-pattern": "note",
    "identity": "note",
    "note": "note",
    "observed-data": "note",
    "infrastructure": "note",
}

_SLUG = re.compile(r"[^a-z0-9]+")


def _slug(value: str, maxlen: int = 48) -> str:
    s = _SLUG.sub("_", (value or "").lower()).strip("_")
    return s[:maxlen] or "x"


def pcmi_type(stix_native_type: str) -> str:
    return STIX_TYPE_TO_PCMI.get(stix_native_type, "note")


def parse_bundle(bundle: dict, ns: str = CORPUS_NS) -> tuple[Report, list[SRO]]:
    """Parse one STIX 2.1 bundle into a Report (of SDOs) + its relationships."""
    objects = bundle.get("objects", []) or []
    report_obj = next((o for o in objects if o.get("type") == "report"), None)
    rep = report_obj or {}
    vendor = rep.get("x_vendor") or rep.get("name") or "TI Mindmap HUB"
    source = rep.get("name") or vendor
    date = str(rep.get("published") or rep.get("created") or "")
    data_period = str(rep.get("x_data_period") or "")
    vendor_slug = _slug(vendor)

    sdos: list[SDO] = []
    sros: list[SRO] = []
    for obj in objects:
        otype = obj.get("type")
        if otype == "relationship":
            sros.append(
                SRO(
                    source=obj.get("source_ref", ""),
                    target=obj.get("target_ref", ""),
                    relationship_type=obj.get("relationship_type", "related"),
                    weight=float(obj.get("x_weight") or 1.0),
                    rationale=obj.get("description", "") or "",
                )
            )
            continue
        if otype in ("report", "bundle", "marking-definition", "language-content"):
            continue

        stype = pcmi_type(otype)
        name = obj.get("name") or obj.get("value") or (obj.get("id") or "")
        desc = obj.get("description", "") or ""
        content = f"{name}. {desc}".strip() if desc else name

        # Start from any preserved source metadata (x_cti_metadata, present in our
        # transcoded bundles; absent in real HUB bundles), then overlay the
        # STIX-authoritative fields so ingestion is correct for any STIX 2.1 input.
        metadata: dict = dict(obj.get("x_cti_metadata") or {})
        metadata.update({
            "vendor": vendor,
            "source": source,
            "stix_type": stype,
            "stix_native_type": otype,
            "stix_id": obj.get("id"),
            "report_date": date,
        })
        tags: list[str] = [stype, *(obj.get("x_cti_tags") or [])]

        aliases = obj.get("aliases") or []
        if otype in ("threat-actor", "intrusion-set", "campaign"):
            metadata.setdefault("actor", name)
            if aliases:
                metadata.setdefault("public_alias", " / ".join(aliases))
                metadata.setdefault("aliases", aliases)
                tags.extend(aliases)
        for k in ("malware_types", "labels", "tool_types", "indicator_types"):
            if obj.get(k):
                tags.extend(obj[k])
        for ref in obj.get("external_references", []) or []:
            eid = ref.get("external_id")
            if not eid:
                continue
            tags.append(eid)
            src = (ref.get("source_name") or "").lower()
            if src == "mitre-attack":
                metadata["mitre"] = eid
            elif src == "cve":
                metadata["cve"] = eid
        if obj.get("pattern"):
            metadata["pattern"] = obj["pattern"]
            content = f"{content} pattern={obj['pattern']}".strip()

        key = obj.get("id") or f"{otype}--{_slug(name)}"
        path = f"{ns}.{vendor_slug}.{_slug(stype)}.{_slug(name)}"
        sdos.append(SDO(key=key, stix_type=stype, path=path, content=content, tags=tags, metadata=metadata, kind=otype))

    report = Report(
        vendor=vendor, source=source, report_date=date, data_period=data_period,
        sort_key=_parse_date(date), sdos=sdos,
    )
    return report, sros


def load_stix_bundles(path: str, ns: str = CORPUS_NS) -> tuple[list[Report], list[SRO], list[str]]:
    """Load every ``*.json`` STIX bundle under ``path`` (file or directory)."""
    if os.path.isdir(path):
        files = sorted(glob.glob(os.path.join(path, "*.json")))
    else:
        files = [path]
    reports: list[Report] = []
    relationships: list[SRO] = []
    loaded: list[str] = []
    for fpath in files:
        with open(fpath, encoding="utf-8") as fh:
            bundle = json.load(fh)
        if bundle.get("type") != "bundle":
            continue
        report, sros = parse_bundle(bundle, ns=ns)
        if report.sdos:
            reports.append(report)
            relationships.extend(sros)
            loaded.append(os.path.basename(fpath))
    reports.sort(key=lambda r: (r.sort_key, r.vendor))
    return reports, relationships, loaded
