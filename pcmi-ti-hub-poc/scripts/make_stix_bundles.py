#!/usr/bin/env python3
"""Transcode the curated CTI dataset into **STIX 2.1 bundles** (one per vendor).

TI Mindmap HUB emits STIX 2.1 bundles. To exercise the ``TI_HUB_MODE=live`` STIX
ingestion path at realistic volume, this script re-expresses
``examples/vendor_reports_cti_dataset.json`` in that exact schema and writes one
bundle per vendor to ``examples/tihub_stix/``. The 25 dataset correlations become
STIX ``relationship`` objects. Alongside these sits TI Mindmap HUB's own published
``example-apt-campaign.json`` (downloaded verbatim), so the ingester is proven on
the platform's real output too.

Run:  python3 scripts/make_stix_bundles.py
"""
from __future__ import annotations

import json
import os
import re
import uuid

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DATASET = os.path.join(ROOT, "examples", "vendor_reports_cti_dataset.json")
OUT_DIR = os.path.join(ROOT, "examples", "tihub_stix")
NS = uuid.NAMESPACE_URL

KIND_TO_STIX_NATIVE = {
    "threat_actor": "threat-actor",
    "threat_actor_activity": "threat-actor",
    "threat_actor_campaign": "campaign",
    "campaign": "campaign",
    "malware": "malware",
    "vulnerability": "vulnerability",
    "indicator": "indicator",
}


def native_type(kind: str) -> str:
    return KIND_TO_STIX_NATIVE.get(kind or "", "note")


def stix_id(node_key: str, ntype: str) -> str:
    return f"{ntype}--{uuid.uuid5(NS, 'tihub/' + node_key)}"


def title_case(segment: str) -> str:
    return re.sub(r"[_.]+", " ", segment).strip().title()


def iso_created(report_date: str) -> str:
    """Pad a partial report date (``2026`` / ``2026-02``) to a valid STIX timestamp."""
    m = re.match(r"(\d{4})(?:-(\d{2}))?(?:-(\d{2}))?", (report_date or "").strip())
    if not m:
        return "2026-01-01T00:00:00.000Z"
    year, month, day = m.group(1), m.group(2) or "01", m.group(3) or "01"
    if not 1 <= int(month) <= 12:
        month = "01"
    return f"{year}-{month}-{day}T00:00:00.000Z"


def obj_name(node) -> str:
    md = node.get("metadata", {})
    return str(
        md.get("actor")
        or md.get("family")
        or md.get("cve")
        or title_case(node["path"].rsplit(".", 1)[-1])
        or (node.get("content", "")[:60])
    )


def build_object(node, created: str) -> dict:
    kind = node.get("metadata", {}).get("kind", "")
    ntype = native_type(kind)
    md = node.get("metadata", {})
    name = obj_name(node)
    content = node.get("content", "")
    obj: dict = {
        "type": ntype,
        "id": stix_id(node["key"], ntype),
        "spec_version": "2.1",
        "created": created,
        "modified": created,
        "name": name,
        "description": content,
    }
    if ntype in ("threat-actor", "campaign"):
        aliases = [a.strip() for a in re.split(r"[/,]", md.get("public_alias", "")) if a.strip()]
        if aliases:
            obj["aliases"] = aliases
    if ntype == "malware":
        obj["is_family"] = True
        if md.get("type"):
            obj["malware_types"] = [str(md["type"]).lower().replace(" ", "-")]
    if ntype == "vulnerability" and md.get("cve"):
        obj["external_references"] = [
            {"source_name": "cve", "external_id": md["cve"], "url": f"https://nvd.nist.gov/vuln/detail/{md['cve']}"}
        ]
    if ntype == "note":  # keep parseable name/description AND STIX-native fields
        obj["abstract"] = name
        obj["content"] = content
    mitre = md.get("mitre_techniques") or md.get("mitre")
    if mitre:
        ids = mitre if isinstance(mitre, list) else [mitre]
        obj.setdefault("external_references", []).extend(
            {"source_name": "mitre-attack", "external_id": str(m)} for m in ids
        )
    # STIX custom properties (x_*) carry the full source metadata + tags so the
    # live ingester can reconstruct demo-level richness. Real HUB bundles simply
    # omit these — the ingester then falls back to STIX-native fields.
    obj["x_cti_metadata"] = md
    if node.get("tags"):
        obj["x_cti_tags"] = node["tags"]
    return obj


def main() -> None:
    with open(DATASET, encoding="utf-8") as fh:
        data = json.load(fh)
    os.makedirs(OUT_DIR, exist_ok=True)

    nodes = data["nodes"]
    by_vendor: dict[str, list[dict]] = {}
    key_index: dict[str, dict] = {}
    for n in nodes:
        by_vendor.setdefault(n["metadata"]["vendor"], []).append(n)
        key_index[n["key"]] = n

    # id lookup for relationships (source node's vendor owns the relationship)
    id_of = {n["key"]: stix_id(n["key"], native_type(n["metadata"].get("kind", ""))) for n in nodes}
    vendor_of = {n["key"]: n["metadata"]["vendor"] for n in nodes}

    rels_by_vendor: dict[str, list[dict]] = {}
    for link in data.get("links", []):
        src, dst = link["from"], link["to"]
        if src not in id_of or dst not in id_of:
            continue
        rel = {
            "type": "relationship",
            "id": f"relationship--{uuid.uuid5(NS, 'tihub/' + src + '|' + dst + '|' + link.get('type', 'related'))}",
            "spec_version": "2.1",
            "relationship_type": link.get("type", "related"),
            "source_ref": id_of[src],
            "target_ref": id_of[dst],
            "description": link.get("rationale", ""),
            "x_weight": link.get("weight", 1.0),
        }
        rels_by_vendor.setdefault(vendor_of[src], []).append(rel)

    written = []
    for vendor, vnodes in by_vendor.items():
        created = iso_created(vnodes[0]["metadata"].get("report_date", ""))
        objects = [build_object(n, created) for n in vnodes]
        report = {
            "type": "report",
            "id": f"report--{uuid.uuid5(NS, 'tihub/report/' + vendor)}",
            "spec_version": "2.1",
            "created": created,
            "published": created,
            "name": vnodes[0]["metadata"].get("source", vendor),
            "x_vendor": vendor,
            "x_data_period": vnodes[0]["metadata"].get("data_period", ""),
            "description": f"STIX 2.1 bundle transcoded from curated CTI for vendor {vendor}.",
            "report_types": ["threat-report"],
            "object_refs": [o["id"] for o in objects],
        }
        bundle = {
            "type": "bundle",
            "id": f"bundle--{uuid.uuid5(NS, 'tihub/bundle/' + vendor)}",
            "objects": [report, *objects, *rels_by_vendor.get(vendor, [])],
        }
        fname = f"vendor_{re.sub(r'[^a-z0-9]+', '_', vendor.lower()).strip('_')}.stix.json"
        with open(os.path.join(OUT_DIR, fname), "w", encoding="utf-8") as fh:
            json.dump(bundle, fh, indent=2, ensure_ascii=False)
        written.append((fname, len(objects), len(rels_by_vendor.get(vendor, []))))

    print(f"wrote {len(written)} STIX 2.1 bundles to {os.path.relpath(OUT_DIR)}:")
    for fname, nobj, nrel in written:
        print(f"  {fname}: {nobj} SDOs, {nrel} relationships")


if __name__ == "__main__":
    main()
