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

# Path suffixes (longest match wins) for the 9-step CTI tour.
TOUR_PATHS: dict[str, str] = {
    "root": "root.cti.soc.incidents.consolidated",
    "brickstorm": "root.cti.vendors.cisa.brickstorm.consolidated",
    "cs_pressure": "root.cti.vendors.crowdstrike.pressure_chollima_bybit",
    "ms_sapphire": "root.cti.vendors.microsoft.threat_intel.sapphire_sleet",
    "mdt_ps": "root.cti.vendor_reports.mandiant.mtrends2026.promptsteal",
    "ms_forest": "root.cti.vendors.microsoft.threat_intel.forest_blizzard",
    "apt28": "root.cti.vendors.google_gtig.apt28.consolidated",
    "ti_sapphire": "root.cti.distillation.campaign_actor_overlap_cs_pressure_chollima_bybit_ms_sapphire_sleet",
    "ti_forest": "root.cti.distillation.malware_linked_to_actor_mdt_promptsteal_ms_forest_blizzard_atc",
}

FALLBACK_SUFFIXES: dict[str, tuple[str, ...]] = {
    "root": ("root.cti.soc.incidents.consolidated", "root.cti.ti_hub.briefs.issue1.consolidated"),
    "brickstorm": (
        "root.cti.vendors.cisa.brickstorm.consolidated",
        "root.cti.vendors.cisa.brickstorm.mar-251165",
    ),
    "cs_pressure": (
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
        "root.cti.vendors.google_gtig.apt28.consolidated",
        "root.cti.vendors.google_gtig.apt28.promptsteal",
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


def load_path_index(base: str, key: str) -> dict[str, int]:
    index: dict[str, int] = {}
    cursor = ""
    pages = 0
    while pages < 12:
        body: dict = {"path_prefix": "root.cti", "limit": 500}
        if cursor:
            body["cursor"] = cursor
        data = http_post(base, key, body)
        for entry in data.get("entries") or []:
            path = entry.get("path") or ""
            mid = entry.get("id")
            if path and mid is not None:
                index[path] = int(mid)
        cursor = data.get("next_cursor") or ""
        pages += 1
        if not cursor:
            break
    return index


def resolve_ids(index: dict[str, int]) -> dict[str, int]:
    out: dict[str, int] = {}
    for key, candidates in FALLBACK_SUFFIXES.items():
        for path in candidates:
            if path in index:
                out[key] = index[path]
                break
        if key not in out:
            # suffix match fallback
            for path, mid in index.items():
                if candidates[0].split(".")[-1] in path or path.endswith(candidates[0].split(".")[-1]):
                    out[key] = mid
                    break
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

    ids = resolve_ids(index)
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
