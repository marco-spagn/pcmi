#!/usr/bin/env python3
"""Resolve memory IDs for the CTI graph UI guided tour from live PCMI data.

Maps stable path suffixes to memory IDs via /v1/retrieve (no id_map.json required).

Usage:
  PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123 python3 resolve_demo_ids.py
  python3 resolve_demo_ids.py --url-only
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

# Path suffixes for the 12-step CTI tour. Prefer nodes that participate in memory_links
# (consolidated/distillation-only nodes often have zero AGE edges).
TOUR_PATHS: dict[str, str] = {
    # Operational STIX dataset entry: CISA MAR BRICKSTORM (real bundle IOCs)
    "root": "root.cti.vendors.cisa.brickstorm.mar-251165",
    "brickstorm": "root.cti.vendors.cisa.brickstorm.mar-251165",
    "brickstorm_sample": "root.cti.vendors.cisa.brickstorm.samples.aaf5569c8e349c15",
    "cs_pressure": "root.cti.vendors.wiz.tradertraitor.bybit_safe_wallet_2025",
    "ms_sapphire": "root.cti.vendors.microsoft.threat_intel.sapphire_sleet",
    "mdt_ps": "root.cti.vendor_reports.mandiant.mtrends2026.promptsteal",
    "ms_forest": "root.cti.vendors.microsoft.threat_intel.forest_blizzard",
    "apt28": "root.cti.vendors.google_gtig.apt28.promptsteal",
    "ti_sapphire": "root.cti.distillation.campaign_actor_overlap_cs_pressure_chollima_bybit_ms_sapphire_sleet",
    "ti_forest": "root.cti.distillation.malware_linked_to_actor_mdt_promptsteal_ms_forest_blizzard_atc",
}

FALLBACK_SUFFIXES: dict[str, tuple[str, ...]] = {
    "root": (
        "root.cti.vendors.cisa.brickstorm.mar-251165",
        "root.cti.vendors.wiz.tradertraitor.bybit_safe_wallet_2025",
        "root.cti.soc.incidents.INC_2026_001",
    ),
    "brickstorm": (
        "root.cti.vendors.cisa.brickstorm.mar-251165",
        "root.cti.vendors.cisa.brickstorm.mar-251217",
        "root.cti.vendors.cisa.brickstorm.mar-251234",
    ),
    "brickstorm_sample": (
        "root.cti.vendors.cisa.brickstorm.samples.aaf5569c8e349c15",
        "root.cti.vendors.cisa.brickstorm.samples.24a11a26a2586f4f",
    ),
    "cs_pressure": (
        "root.cti.vendors.wiz.tradertraitor.bybit_safe_wallet_2025",
        "root.cti.vendors.crowdstrike.pressure_chollima_bybit",
        "root.cti.vendor_reports.crowdstrike.gtr2026.pressure_chollima",
    ),
    "ms_sapphire": (
        "root.cti.vendors.microsoft.threat_intel.sapphire_sleet",
        "root.cti.vendor_reports.microsoft.threat_intel.sapphire_sleet",
    ),
    "mdt_ps": (
        "root.cti.vendor_reports.mandiant.mtrends2026.promptsteal",
        "root.cti.vendors.mandiant.promptsteal_apt28",
        "root.cti.vendors.google_gtig.apt28.promptsteal",
    ),
    "ms_forest": (
        "root.cti.vendors.microsoft.threat_intel.forest_blizzard",
        "root.cti.vendor_reports.microsoft.threat_intel.forest_blizzard",
    ),
    "apt28": (
        "root.cti.vendors.google_gtig.apt28.promptsteal",
        "root.cti.vendors.google_gtig.apt28.consolidated",
    ),
    "ti_sapphire": (
        "root.cti.distillation.campaign_actor_overlap_cs_pressure_chollima_bybit_ms_sapphire_sleet",
        "root.cti.vendor_reports.microsoft.threat_intel.sapphire_sleet",
    ),
    "ti_forest": (
        "root.cti.distillation.malware_linked_to_actor_mdt_promptsteal_ms_forest_blizzard_atc",
        "root.cti.vendor_reports.microsoft.threat_intel.forest_blizzard",
    ),
}


def http_post(base: str, key: str, body: dict) -> dict:
    req = urllib.request.Request(
        f"{base.rstrip('/')}/v1/retrieve",
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", "X-API-Key": key},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read().decode())


def graph_neighbor_count(base: str, key: str, memory_id: int) -> int:
    """Return AGE neighbor count (depth=1) for a memory id; 0 if unreachable or isolated."""
    url = f"{base.rstrip('/')}/v1/graph/related?memory_id={memory_id}&depth=1&limit=1"
    req = urllib.request.Request(url, headers={"X-API-Key": key})
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            data = json.loads(resp.read().decode())
            return int(data.get("total") or 0)
    except (urllib.error.URLError, TimeoutError, ValueError, json.JSONDecodeError):
        return 0


def _pick_best_memory_id(candidates: list[tuple[int, dict]]) -> int:
    """Prefer operational nodes (stix/vendor_intel metadata) with highest id."""
    if not candidates:
        raise ValueError("empty candidates")
    operational = [
        (mid, meta)
        for mid, meta in candidates
        if (meta or {}).get("cti_source") in ("stix", "vendor_intel")
    ]
    pool = operational or candidates
    return max(pool, key=lambda item: item[0])[0]


def load_path_index(base: str, key: str) -> dict[str, int]:
    by_path: dict[str, list[tuple[int, dict]]] = {}
    cursor = ""
    pages = 0
    while pages < 24:
        body: dict = {"path_prefix": "root.cti", "limit": 500}
        if cursor:
            body["cursor"] = cursor
        data = http_post(base, key, body)
        for entry in data.get("entries") or []:
            path = entry.get("path") or ""
            mid = entry.get("id")
            if path and mid is not None:
                by_path.setdefault(path, []).append((int(mid), entry.get("metadata") or {}))
        cursor = data.get("next_cursor") or ""
        pages += 1
        if not cursor:
            break
    return {path: _pick_best_memory_id(items) for path, items in by_path.items()}


def resolve_ids(index: dict[str, int], base: str, api_key: str) -> dict[str, int]:
    out: dict[str, int] = {}
    for key, candidates in FALLBACK_SUFFIXES.items():
        fallback_id: int | None = None
        for path in candidates:
            mid = index.get(path)
            if mid is None:
                continue
            if fallback_id is None:
                fallback_id = mid
            if graph_neighbor_count(base, api_key, mid) > 0:
                out[key] = mid
                break
        if key not in out and fallback_id is not None:
            out[key] = fallback_id
    return out


def build_ui_query(ids: dict[str, int]) -> str:
    root = ids.get("root", ids.get("brickstorm", 0))
    parts = ["demo=cti", f"mem={root}", f"root={root}"]
    for k, v in ids.items():
        parts.append(f"{k}={v}")
    return "&".join(parts)


def main() -> int:
    parser = argparse.ArgumentParser(description="Resolve CTI graph UI tour memory IDs")
    parser.add_argument("--url-only", action="store_true", help="Print graph UI URL only")
    parser.add_argument("--json", action="store_true", help="Print JSON map")
    args = parser.parse_args()

    base = os.environ.get("PCMI_BASE_URL", "http://localhost:8000")
    key = os.environ.get("PCMI_API_KEY", "testkey123")

    try:
        index = load_path_index(base, key)
    except urllib.error.URLError as e:
        print(f"API unreachable ({base}): {e}", file=sys.stderr)
        return 1

    if len(index) < 10:
        print(
            "Fewer than 10 memories under root.cti — load the CTI dataset first "
            "(see examples/full-cti-dataset/README.md or prior load_multi_cti.py run).",
            file=sys.stderr,
        )
        return 1

    ids = resolve_ids(index, base, key)
    missing = [k for k in FALLBACK_SUFFIXES if k not in ids]
    if missing:
        print(f"Warning: could not resolve tour keys: {', '.join(missing)}", file=sys.stderr)

    if args.json:
        print(json.dumps(ids, indent=2))
        return 0

    query = build_ui_query(ids)
    url = f"{base.rstrip('/')}/v1/graph/ui?{query}"

    if args.url_only:
        print(url)
        return 0

    print(json.dumps(ids, indent=2))
    print()
    print("Graph UI (CTI tour):")
    print(url)
    print()
    print("Autostart tour:")
    print(url + "&autostart=1")
    return 0


if __name__ == "__main__":
    sys.exit(main())
