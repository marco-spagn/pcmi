#!/usr/bin/env python3
"""Convert STIX 2.1 bundle objects into PCMI nodes + links."""
from __future__ import annotations

import json
import re
from typing import Any

PUBLIC_LINK_TYPES = {"causal", "temporal", "contradicts", "supports", "related"}

RELATIONSHIP_TO_LINK = {
    "uses": "related",
    "attributed-to": "supports",
    "targets": "related",
    "indicates": "supports",
    "related-to": "related",
    "derived-from": "temporal",
}


def _slug(stix_id: str) -> str:
    return re.sub(r"[^a-zA-Z0-9_-]+", "-", stix_id.strip()).strip("-")[:80]


def _object_content(obj: dict[str, Any]) -> str:
    parts = []
    if obj.get("name"):
        parts.append(str(obj["name"]))
    if obj.get("description"):
        parts.append(str(obj["description"]))
    if obj.get("relationship_type"):
        parts.append(f"relationship: {obj['relationship_type']}")
    return "\n\n".join(parts) or str(obj.get("id", "stix-object"))


def convert_stix_bundle(bundle: dict[str, Any]) -> dict[str, list]:
    """Return {nodes, links} in PCMI dataset format."""
    objects = bundle.get("objects") or []
    if not objects:
        return {"nodes": [], "links": []}

    nodes: list[dict[str, Any]] = []
    id_to_key: dict[str, str] = {}

    for obj in objects:
        otype = obj.get("type")
        oid = obj.get("id")
        if not oid or otype in (None, "relationship", "marking-definition"):
            continue

        key = f"stix_{_slug(oid)}"
        id_to_key[oid] = key
        path = f"root.cti.stix.{otype}.{_slug(oid)}"
        meta: dict[str, Any] = {
            "cti_source": "stix",
            "stix_type": otype,
            "stix_id": oid,
            "layer": "stix",
        }
        if obj.get("created"):
            meta["created"] = obj["created"]
        if obj.get("modified"):
            meta["modified"] = obj["modified"]
        if obj.get("identity_class"):
            meta["identity_class"] = obj["identity_class"]

        tags = ["stix", otype, "historical"]
        if otype == "intrusion-set":
            tags.append("threat-actor")
        elif otype == "campaign":
            tags.append("campaign")

        nodes.append(
            {
                "key": key,
                "path": path,
                "content": _object_content(obj),
                "tags": tags,
                "metadata": meta,
            }
        )

    links: list[dict[str, Any]] = []
    for obj in objects:
        if obj.get("type") != "relationship":
            continue
        src_ref = obj.get("source_ref")
        tgt_ref = obj.get("target_ref")
        if not src_ref or not tgt_ref:
            continue
        src_key = id_to_key.get(src_ref)
        tgt_key = id_to_key.get(tgt_ref)
        if not src_key or not tgt_key:
            continue
        rel = (obj.get("relationship_type") or "related-to").lower()
        link_type = RELATIONSHIP_TO_LINK.get(rel, "related")
        if link_type not in PUBLIC_LINK_TYPES:
            link_type = "related"
        links.append(
            {
                "from": src_key,
                "to": tgt_key,
                "type": link_type,
                "weight": 0.75,
                "rationale": obj.get("description") or rel,
                "metadata": {"cti_source": "stix", "stix_relationship": rel},
            }
        )

    return {"nodes": nodes, "links": links}


def parse_indicator_pattern(pattern: str) -> dict[str, Any]:
    """Extract IOCs from a STIX 2.1 indicator pattern string."""
    if not pattern:
        return {}
    out: dict[str, Any] = {}
    for hash_type in ("SHA-256", "MD5", "SHA-1", "SHA-512"):
        found = re.findall(rf"file:hashes\.'{re.escape(hash_type)}'\s*=\s*'([^']+)'", pattern)
        if found:
            key = hash_type.lower().replace("-", "_")
            out[key] = found[0] if len(found) == 1 else found
    for field, key in (
        (r"ipv4-addr:value\s*=\s*'([^']+)'", "ips"),
        (r"domain-name:value\s*=\s*'([^']+)'", "domains"),
        (r"url:value\s*=\s*'([^']+)'", "urls"),
    ):
        found = re.findall(field, pattern)
        if found:
            out[key] = found[0] if len(found) == 1 else found
    yara = re.search(r"rule_name=(\S+)", pattern)
    if yara:
        out["yara_rule"] = yara.group(1).strip()
    return out


def _extract_yara_rules(pattern: str) -> list[str]:
    if not pattern:
        return []
    return list(dict.fromkeys(re.findall(r"rule_name=(\S+)", pattern)))


def _infer_file_type(description: str, analysis_results: list[str]) -> str:
    text = " ".join([description or ""] + analysis_results).lower()
    if "native aot" in text or ".net" in text:
        return "ELF 64-bit (.NET Native AOT)"
    if "rust" in text:
        return "ELF 64-bit (Rust)"
    if "go programming" in text or "go-based" in text:
        return "ELF 64-bit (Go)"
    if "windows" in text or "pe32" in text:
        return "PE32"
    return "ELF 64-bit"


# Campaign context from CISA MAR BRICKSTORM + Mandiant M-Trends 2026 (public reporting).
BRICKSTORM_CONTEXT = {
    "actor": "UNC5221 / UNC6201",
    "attribution": "PRC state-sponsored",
    "mitre_techniques": ["T1037", "T1090", "T1071.001", "T1573.002", "T1078", "T1021.004"],
    "cve_exploited": ["CVE-2026-22769"],
    "c2_protocol": "DNS-over-HTTPS + WebSocket + nested TLS",
    "confidence": "high",
    "first_seen": "2024-04",
    "last_seen": "2025-09",
    "dwell_time_days": 393,
}


def convert_operational_stix_bundle(
    bundle: dict[str, Any],
    *,
    mar_id: str,
    vendor: str = "CISA/Mandiant",
    campaign: str = "BRICKSTORM",
) -> dict[str, list]:
    """Convert a CISA MAR STIX bundle into operational PCMI nodes with extracted IOCs."""
    objects = bundle.get("objects") or []
    if not objects:
        return {"nodes": [], "links": []}

    by_id: dict[str, dict[str, Any]] = {o["id"]: o for o in objects if o.get("id")}
    files = [o for o in objects if o.get("type") == "file"]
    indicators = [o for o in objects if o.get("type") == "indicator"]
    analyses = [o for o in objects if o.get("type") == "malware-analysis"]
    reports = [o for o in objects if o.get("type") == "report"]

    # Map file STIX id → malware-analysis results.
    analysis_by_file: dict[str, list[str]] = {}
    for ma in analyses:
        ref = ma.get("sample_ref")
        if ref:
            analysis_by_file.setdefault(ref, []).append(ma.get("result_name") or ma.get("product") or "")

    # Map SHA-256 → YARA rules from hash + YARA indicators.
    yara_by_sha256: dict[str, list[str]] = {}
    hash_iocs_by_sha256: dict[str, dict[str, str]] = {}
    network_indicators: list[dict[str, Any]] = []
    yara_orphans: list[tuple[list[str], str]] = []  # (rules, name_prefix)

    for ind in indicators:
        pattern = ind.get("pattern") or ""
        parsed = parse_indicator_pattern(pattern)
        yaras = _extract_yara_rules(pattern)
        sha = parsed.get("sha256")
        if isinstance(sha, list):
            sha = sha[0] if sha else None
        if sha:
            if yaras:
                yara_by_sha256.setdefault(sha, []).extend(yaras)
            for hk in ("md5", "sha1", "sha256", "sha512"):
                if parsed.get(hk):
                    hash_iocs_by_sha256.setdefault(sha, {})[hk] = parsed[hk]
        elif yaras:
            # YARA-only indicator — correlate via sha256 in rule_content meta or name prefix.
            meta_sha = re.findall(r"sha256_\d+\s*=\s*\"([a-f0-9]{64})\"", pattern)
            if meta_sha:
                for s in meta_sha:
                    yara_by_sha256.setdefault(s, []).extend(yaras)
            else:
                yara_orphans.append((yaras, ind.get("name") or ""))
        if parsed.get("ips") or parsed.get("domains") or parsed.get("urls"):
            network_indicators.append({"indicator": ind, "parsed": parsed})

    def _attach_orphan_yaras(sha256: str, fname: str) -> None:
        for yaras, prefix in yara_orphans:
            if prefix and (prefix in sha256 or prefix in fname or sha256.startswith(prefix)):
                yara_by_sha256.setdefault(sha256, []).extend(yaras)

    nodes: list[dict[str, Any]] = []
    links: list[dict[str, Any]] = []
    file_keys: dict[str, str] = {}
    campaign_key = f"{campaign.lower()}_campaign_{mar_id.lower().replace('.', '_')}"

    report_desc = ""
    if reports:
        report_desc = reports[0].get("description") or reports[0].get("name") or ""

    nodes.append(
        {
            "key": campaign_key,
            "path": f"root.cti.vendors.cisa.{campaign.lower()}.{mar_id.lower()}",
            "content": (
                f"{campaign} — {mar_id}. {report_desc[:500]}"
                if report_desc
                else f"{campaign} malware analysis report {mar_id} from {vendor}."
            ),
            "tags": ["malware", campaign.lower(), "cisa-mar", "stix", mar_id.lower()],
            "metadata": {
                "kind": "campaign_report",
                "vendor": vendor,
                "source": mar_id,
                "cti_source": "stix",
                "stix_type": "report",
                "layer": "vendors",
                "campaign": campaign,
                **BRICKSTORM_CONTEXT,
            },
        }
    )

    for idx, fobj in enumerate(files, start=1):
        fid = fobj["id"]
        hashes = fobj.get("hashes") or {}
        sha256 = hashes.get("SHA-256") or fobj.get("name", "")
        if not sha256 or len(sha256) < 32:
            continue
        key = f"brickstorm_sample_{sha256[:12]}"
        file_keys[fid] = key
        fname = fobj.get("name") or ""
        desc = ""
        for ind in indicators:
            if sha256[:32] in (ind.get("name") or "") or sha256 in (ind.get("pattern") or ""):
                desc = ind.get("description") or desc
        if not desc:
            for ind in indicators:
                p = ind.get("pattern") or ""
                if sha256 in p:
                    desc = ind.get("description") or desc
                    break

        _attach_orphan_yaras(sha256, fname)
        yara_rules = list(dict.fromkeys(yara_by_sha256.get(sha256, [])))
        analysis_results = analysis_by_file.get(fid, [])
        file_type = _infer_file_type(desc, analysis_results)

        iocs: dict[str, Any] = {
            "sha256": sha256,
            "md5": hashes.get("MD5"),
            "sha1": hashes.get("SHA-1"),
            "file_name": fname if fname != sha256 else None,
            "file_type": file_type,
        }
        iocs = {k: v for k, v in iocs.items() if v}

        content_parts = [
            f"{campaign} sample {idx}: {file_type} backdoor.",
            f"SHA-256: {sha256}",
        ]
        if fname and fname != sha256:
            content_parts.append(f"File name: {fname}")
        if analysis_results:
            content_parts.append(f"AV/sandbox: {', '.join(r for r in analysis_results if r)}")
        if desc:
            content_parts.append(desc[:600])

        nodes.append(
            {
                "key": key,
                "path": f"root.cti.vendors.cisa.{campaign.lower()}.samples.{sha256[:16]}",
                "content": "\n\n".join(content_parts),
                "tags": ["malware", campaign.lower(), "sample", "stix", mar_id.lower()],
                "metadata": {
                    "kind": "malware_sample",
                    "vendor": vendor,
                    "source": mar_id,
                    "cti_source": "stix",
                    "stix_type": "file",
                    "stix_id": fid,
                    "layer": "vendors",
                    "iocs": iocs,
                    "c2": {
                        "protocol": BRICKSTORM_CONTEXT["c2_protocol"],
                        "domains": [],
                        "ips": [],
                    },
                    "mitre_techniques": BRICKSTORM_CONTEXT["mitre_techniques"],
                    "yara_rules": yara_rules,
                    "cve_exploited": BRICKSTORM_CONTEXT["cve_exploited"],
                    "actor": BRICKSTORM_CONTEXT["actor"],
                    "attribution": BRICKSTORM_CONTEXT["attribution"],
                    "confidence": BRICKSTORM_CONTEXT["confidence"],
                    "first_seen": BRICKSTORM_CONTEXT["first_seen"],
                    "last_seen": BRICKSTORM_CONTEXT["last_seen"],
                    "dwell_time_days": BRICKSTORM_CONTEXT["dwell_time_days"],
                },
            }
        )
        links.append(
            {
                "from": campaign_key,
                "to": key,
                "type": "supports",
                "weight": 0.9,
                "rationale": f"{mar_id} malware sample indicator",
                "metadata": {"cti_source": "stix", "mar_id": mar_id},
            }
        )

    for nind in network_indicators:
        ind = nind["indicator"]
        parsed = nind["parsed"]
        ips = parsed.get("ips")
        if isinstance(ips, str):
            ips = [ips]
        domains = parsed.get("domains")
        if isinstance(domains, str):
            domains = [domains]
        urls = parsed.get("urls")
        if isinstance(urls, str):
            urls = [urls]

        label = ips[0] if ips else (domains[0] if domains else (urls[0] if urls else ind.get("name", "network")))
        key = f"brickstorm_net_{re.sub(r'[^a-zA-Z0-9]+', '_', label)[:40]}"
        nodes.append(
            {
                "key": key,
                "path": f"root.cti.vendors.cisa.{campaign.lower()}.infrastructure.{key}",
                "content": ind.get("description") or f"{campaign} network indicator: {label}",
                "tags": ["infrastructure", campaign.lower(), "c2", "stix", mar_id.lower()],
                "metadata": {
                    "kind": "network_indicator",
                    "vendor": vendor,
                    "source": mar_id,
                    "cti_source": "stix",
                    "stix_type": "indicator",
                    "stix_id": ind.get("id"),
                    "layer": "vendors",
                    "iocs": {
                        "ips": ips or [],
                        "domains": domains or [],
                        "urls": urls or [],
                    },
                    "c2": {
                        "protocol": BRICKSTORM_CONTEXT["c2_protocol"],
                        "domains": domains or [],
                        "ips": ips or [],
                        "urls": urls or [],
                    },
                    "actor": BRICKSTORM_CONTEXT["actor"],
                    "attribution": BRICKSTORM_CONTEXT["attribution"],
                    "confidence": "high",
                },
            }
        )
        links.append(
            {
                "from": campaign_key,
                "to": key,
                "type": "supports",
                "weight": 0.85,
                "rationale": f"{mar_id} C2/network indicator",
                "metadata": {"cti_source": "stix", "mar_id": mar_id},
            }
        )
        # Link network IOC to samples in same MAR.
        for fk in file_keys.values():
            links.append(
                {
                    "from": fk,
                    "to": key,
                    "type": "related",
                    "weight": 0.7,
                    "rationale": f"Same {mar_id} campaign infrastructure",
                    "metadata": {"cti_source": "stix"},
                }
            )

    return {"nodes": nodes, "links": links}


def load_stix_file(path: str) -> dict[str, list]:
    with open(path, encoding="utf-8") as f:
        bundle = json.load(f)
    if bundle.get("nodes"):
        return {"nodes": bundle["nodes"], "links": bundle.get("links") or []}
    return convert_stix_bundle(bundle)
