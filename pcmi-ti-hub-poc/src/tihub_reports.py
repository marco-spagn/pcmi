"""Ingest TI Mindmap HUB's **own published CTI reports** (real data).

`TI_HUB_MODE=reports` reads the Markdown threat-intel reports TI Mindmap HUB
publishes in its research repo (``reports/*.md``) — cross-source campaign analyses
(FortiBleed, Axios npm supply chain, Iran conflict, CopyFail CVE, TeamPCP). Each
report is reshaped into the same ``Report``/``SDO``/``SRO`` objects the pipeline
consumes, so ingest → correlation → graph → temporal → sessions run unchanged.

Each report becomes one "vendor" group (the campaign). Extracted per report:
  * a **campaign** SDO (title + frontmatter description + overview),
  * a **threat-actor** SDO when a named actor/alias is present,
  * a **vulnerability** SDO per CVE,
linked with within-report SROs. Cross-report correlations (shared themes/TTPs) are
surfaced by the correlation phases, not hard-coded here.
"""
from __future__ import annotations

import glob
import os
import re

from .stix_ingest import CORPUS_NS, _slug
from .ti_hub_client import SDO, SRO, Report, _parse_date

_CVE = re.compile(r"CVE-\d{4}-\d{4,7}")
_MITRE = re.compile(r"\bT\d{4}(?:\.\d{3})?\b")
_ACTOR_ROW = re.compile(r"\|\s*(?:Alias|Threat Actor|Actor|Group)\s*\|\s*(.+?)\s*\|", re.IGNORECASE)
_GENERIC_ACTOR = {"", "n/a", "na", "none", "unknown", "unattributed", "not attributed", "-", "tbd"}
_MAX_TECHNIQUES = 8


def _parse_frontmatter(text: str) -> tuple[dict, str]:
    """Minimal YAML-frontmatter parser (stdlib only). Returns (fields, body)."""
    m = re.match(r"^---\s*\n(.*?)\n---\s*\n?(.*)$", text, re.S)
    if not m:
        return {}, text
    block, body = m.group(1), m.group(2)
    fields: dict = {}
    tags: list[str] = []
    in_tags = False
    for raw in block.splitlines():
        if re.match(r"^\s*-\s+", raw) and in_tags:
            tags.append(raw.strip()[1:].strip().strip("\"'"))
            continue
        in_tags = False
        km = re.match(r"^([A-Za-z0-9_]+):\s*(.*)$", raw)
        if not km:
            continue
        key, val = km.group(1), km.group(2).strip()
        if key == "tags" and val == "":
            in_tags = True
            continue
        fields[key] = val.strip().strip("\"'")
    fields["tags"] = tags
    return fields, body


def _campaign_name(title: str) -> str:
    return re.sub(r"^\s*Threat Intelligence Report:\s*", "", title).strip()


def _overview(body: str) -> str:
    """Grab the Executive Summary / Overview prose (first substantial paragraph)."""
    m = re.search(r"##\s*\d*\.?\s*Executive Summary.*?\n(.*)", body, re.S | re.IGNORECASE)
    region = m.group(1) if m else body
    for para in re.split(r"\n\s*\n", region):
        clean = re.sub(r"[#>*`|_-]", " ", para).strip()
        clean = re.sub(r"\s+", " ", clean)
        if len(clean) >= 120 and not clean.lower().startswith("diagram"):
            return clean[:900]
    return ""


def parse_report(path: str, ns: str = CORPUS_NS) -> tuple[Report, list[SRO]]:
    text = open(path, encoding="utf-8").read()
    fm, body = _parse_frontmatter(text)
    title = fm.get("title") or os.path.basename(path)
    campaign = _campaign_name(title)
    # A short, stable group label for this campaign (acts as the "vendor").
    vendor = re.split(r"[—:(]", campaign)[0].strip()[:40] or campaign[:40]
    date = fm.get("date", "")
    tags = list(fm.get("tags", []))
    description = fm.get("description", "")
    overview = _overview(body)
    vslug = _slug(vendor)

    base_md = {
        "vendor": vendor,
        "source": f"TI Mindmap HUB — {title}",
        "report_date": date,
        "severity": fm.get("severity", ""),
        "classification": fm.get("classification", ""),
        "sources_count": fm.get("sources_count", ""),
        "publisher": fm.get("author", "TI Mindmap HUB"),
    }

    sdos: list[SDO] = []
    sros: list[SRO] = []

    campaign_key = f"tihub:{vslug}:campaign"
    campaign_content = ". ".join(x for x in (campaign, description, overview) if x)
    sdos.append(SDO(
        key=campaign_key, stix_type="campaign",
        path=f"{ns}.{vslug}.campaign.{_slug(campaign)}",
        content=campaign_content, tags=[*tags, "campaign"],
        metadata={**base_md, "stix_type": "campaign", "kind": "campaign", "campaign_name": campaign},
        kind="campaign",
    ))

    # Named threat actor (only when the report attributes one).
    actor_val = None
    am = _ACTOR_ROW.search(body)
    if am and am.group(1).strip().lower() not in _GENERIC_ACTOR:
        actor_val = re.sub(r"\*\*|`", "", am.group(1)).strip()
    actor_val = actor_val or (fm.get("actor") if fm.get("actor", "").lower() not in _GENERIC_ACTOR else None)
    if actor_val:
        actor_key = f"tihub:{vslug}:actor"
        sdos.append(SDO(
            key=actor_key, stix_type="threat-actor",
            path=f"{ns}.{vslug}.threat_actor.{_slug(actor_val)}",
            content=f"{actor_val}. Attributed threat actor in the {campaign} campaign. {overview}".strip(),
            tags=[*tags, "threat-actor", actor_val],
            metadata={**base_md, "stix_type": "threat-actor", "kind": "threat_actor_activity",
                      "actor": actor_val, "public_alias": actor_val},
            kind="threat-actor",
        ))
        sros.append(SRO(source=campaign_key, target=actor_key, relationship_type="attributed-to",
                        weight=1.0, rationale=f"{campaign} attributed to {actor_val}"))

    # CVEs → vulnerability SDOs.
    for cve in sorted(set(_CVE.findall(text))):
        cve_key = f"tihub:{vslug}:{cve}"
        sdos.append(SDO(
            key=cve_key, stix_type="vulnerability",
            path=f"{ns}.{vslug}.vulnerability.{_slug(cve)}",
            content=f"{cve} exploited/analysed in the {campaign} campaign. {description}".strip(),
            tags=[*tags, "vulnerability", cve],
            metadata={**base_md, "stix_type": "vulnerability", "kind": "vulnerability", "cve": cve},
            kind="vulnerability",
        ))
        sros.append(SRO(source=campaign_key, target=cve_key, relationship_type="exploits",
                        weight=1.0, rationale=f"{campaign} exploits {cve}"))

    # MITRE ATT&CK techniques → attack-pattern SDOs (capped) so the graph and
    # cross-report TTP correlation have real behavioural nodes to work with.
    for tech in sorted(set(_MITRE.findall(text)))[:_MAX_TECHNIQUES]:
        tech_key = f"tihub:{vslug}:{tech}"
        sdos.append(SDO(
            key=tech_key, stix_type="note",
            path=f"{ns}.{vslug}.technique.{_slug(tech)}",
            content=f"MITRE ATT&CK {tech} observed in the {campaign} campaign.",
            tags=[*tags, "attack-pattern", tech],
            metadata={**base_md, "stix_type": "note", "kind": "technique_trend", "mitre": tech},
            kind="attack-pattern",
        ))
        sros.append(SRO(source=campaign_key, target=tech_key, relationship_type="uses",
                        weight=1.0, rationale=f"{campaign} uses {tech}"))

    report = Report(vendor=vendor, source=base_md["source"], report_date=date, data_period="",
                    sort_key=_parse_date(date), sdos=sdos)
    return report, sros


def load_reports(path: str, ns: str = CORPUS_NS) -> tuple[list[Report], list[SRO], list[str]]:
    files = sorted(glob.glob(os.path.join(path, "*.md")))
    reports: list[Report] = []
    relationships: list[SRO] = []
    loaded: list[str] = []
    for fpath in files:
        report, sros = parse_report(fpath, ns=ns)
        if report.sdos:
            reports.append(report)
            relationships.extend(sros)
            loaded.append(os.path.basename(fpath))
    reports.sort(key=lambda r: (r.sort_key, r.vendor))
    return reports, relationships, loaded
