#!/usr/bin/env python3
"""
Demo: cross-vendor CTI correlation via PCMI hybrid retrieval (BM25 + pgvector).

Shows that queries using one vendor's naming surface memories documented under
another vendor's taxonomy — without explicit alias tables at query time.

  PRESSURE CHOLLIMA (CrowdStrike) ↔ Sapphire Sleet (Microsoft) — DPRK umbrella
  PROMPTSTEAL / FROZENLAKE (Mandiant) ↔ Forest Blizzard (Microsoft) — APT28/GRU

Dataset: operational_stix_cti_dataset.json + vendor_reports (loaded via load_multi_cti.py)

Usage:
  make cti-stix-build
  python3 load_multi_cti.py --reset    # wait ~60s for embedding worker
  python3 demo_cross_vendor_correlation.py
"""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request

BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")
PREFIX = os.environ.get("CTI_DEMO_PREFIX", "root.cti")

# Operational-dataset keys accept vendor_reports equivalents (same actor, different path).
ALIAS_KEYS = {
    "ms_threat_intel_sapphire_sleet": {"ms_sapphire_sleet"},
    "ms_threat_intel_forest_blizzard": {"ms_forest_blizzard_atc"},
    "apt28_promptsteal_2025": {"mdt_promptsteal"},
    "tradertraitor_bybit_2025": {"cs_pressure_chollima_bybit"},
    "cs_pressure_chollima_bybit": {"tradertraitor_bybit_2025"},
    "mdt_promptsteal": {"apt28_promptsteal_2025"},
    "ms_sapphire_sleet": {"ms_threat_intel_sapphire_sleet"},
    "ms_forest_blizzard_atc": {"ms_threat_intel_forest_blizzard"},
}


PATH_KEY_HINTS: list[tuple[str, str]] = [
    ("pressure_chollima", "cs_pressure_chollima_bybit"),
    ("tradertraitor", "cs_pressure_chollima_bybit"),
    ("sapphire_sleet", "ms_sapphire_sleet"),
    ("mandiant.mtrends2026.promptsteal", "mdt_promptsteal"),
    ("vendor_reports.mandiant", "mdt_promptsteal"),
    ("forest_blizzard", "ms_forest_blizzard_atc"),
    ("apt28_promptsteal", "apt28_promptsteal_2025"),
    ("google_gtig.apt28", "apt28_promptsteal_2025"),
]


def _aliases_for(key: str) -> set[str]:
    return {key} | ALIAS_KEYS.get(key, set())


def _keys_for_entry(entry: dict, key_by_id: dict[int, str]) -> set[str]:
    keys: set[str] = set()
    mid = entry.get("id")
    if mid is not None:
        try:
            mid_i = int(mid)
        except (TypeError, ValueError):
            mid_i = None
        if mid_i is not None and mid_i in key_by_id:
            keys.add(key_by_id[mid_i])
    path = (entry.get("path") or "").lower()
    for fragment, key in PATH_KEY_HINTS:
        if fragment in path:
            keys.add(key)
    return keys


def _is_hit(found_key: str, expect_keys: set[str]) -> bool:
    for exp in expect_keys:
        if found_key in _aliases_for(exp):
            return True
    return False


def _missing_keys(found_keys: set[str], expect_keys: set[str]) -> set[str]:
    return {exp for exp in expect_keys if not (_aliases_for(exp) & found_keys)}


SCENARIOS = [
    {
        "title": "DPRK — CrowdStrike 'PRESSURE CHOLLIMA' → Microsoft 'Sapphire Sleet'",
        "query": "PRESSURE CHOLLIMA Bybit Safe Wallet macOS supply chain cryptocurrency",
        "expect_keys": {"cs_pressure_chollima_bybit", "ms_sapphire_sleet"},
        "limit": 20,
    },
    {
        "title": "DPRK — Microsoft 'Sapphire Sleet' → CrowdStrike 'PRESSURE CHOLLIMA'",
        "query": "Sapphire Sleet North Korean macOS cryptocurrency social engineering",
        "expect_keys": {"ms_sapphire_sleet", "cs_pressure_chollima_bybit"},
        "limit": 16,
    },
    {
        "title": "APT28 — Mandiant 'PROMPTSTEAL/FROZENLAKE' ↔ operational STIX IOC nodes",
        "query": "PROMPTSTEAL FROZENLAKE LLM Hugging Face Ukraine Mandiant",
        "expect_keys": {"mdt_promptsteal", "apt28_promptsteal_2025"},
        "limit": 10,
    },
    {
        "title": "APT28 — Microsoft 'Forest Blizzard' → Mandiant PROMPTSTEAL campaign",
        "query": "Forest Blizzard password spray NATO aviation GRU PROMPTSTEAL LLM",
        "expect_keys": {"ms_forest_blizzard_atc", "mdt_promptsteal"},
        "limit": 22,
    },
]


def retrieve(query: str, limit: int = 10) -> list[dict]:
    payload = {"path_prefix": PREFIX, "query": query, "limit": limit}
    req = urllib.request.Request(
        f"{BASE}/v1/retrieve",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json", "X-API-Key": KEY},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        body = json.load(resp)
    return body.get("entries") or []


def load_key_map() -> dict[int, str]:
    id_map_path = os.path.join(os.path.dirname(__file__), "id_map.json")
    if not os.path.isfile(id_map_path):
        return {}
    id_map = json.load(open(id_map_path, encoding="utf-8"))
    return {int(v): k for k, v in id_map.items()}


def main() -> int:
    print(f"PCMI cross-vendor correlation demo → {BASE}")
    print(f"  path_prefix: {PREFIX}")
    print(f"  retrieval:   hybrid BM25 + pgvector (POST /v1/retrieve)\n")

    try:
        req = urllib.request.Request(f"{BASE}/v1/graph/health", headers={"X-API-Key": KEY})
        with urllib.request.urlopen(req, timeout=10) as r:
            health = json.load(r)
        if not health.get("available"):
            print("✗ graph/AGE not available", file=sys.stderr)
            return 1
    except urllib.error.HTTPError as e:
        if e.code == 429:
            print(
                "✗ rate limit (429) — restart API with RATE_LIMIT_DISABLED=true",
                file=sys.stderr,
            )
        else:
            print(f"✗ PCMI HTTP {e.code}", file=sys.stderr)
        return 1
    except urllib.error.URLError as exc:
        print(f"✗ PCMI not reachable: {exc}", file=sys.stderr)
        return 1

    key_by_id = load_key_map()
    passed = 0
    failed = 0

    for scenario in SCENARIOS:
        print("=" * 72)
        print(scenario["title"])
        print(f"Query: {scenario['query']!r}\n")

        limit = scenario.get("limit", 10)
        entries = retrieve(scenario["query"], limit=limit)
        found_keys: set[str] = set()
        for rank, e in enumerate(entries, 1):
            mem_id = e.get("id")
            entry_keys = _keys_for_entry(e, key_by_id)
            found_keys |= entry_keys
            label = next(iter(entry_keys), f"mem:{mem_id}")
            score = e.get("relevance_score") or e.get("score") or 0.0
            vendor = (e.get("metadata") or {}).get("vendor", "—")
            actor = (e.get("metadata") or {}).get("actor") or (e.get("metadata") or {}).get("attributed_to", "—")
            mark = "✓" if any(_is_hit(k, scenario["expect_keys"]) for k in entry_keys) else " "
            path = (e.get("path") or "")[-60:]
            print(f"  {mark} {rank:2}. score={score:.4f}  {label}")
            print(f"       path=…{path}")
            print(f"       vendor={vendor}  actor={actor}")
            print(f"       {e.get('content', '')[:100].replace(chr(10), ' ')}…")

        missing = _missing_keys(found_keys, scenario["expect_keys"])
        if missing:
            print(f"\n  ✗ MISSING in top-{len(entries)}: {', '.join(sorted(missing))}")
            failed += 1
        else:
            print(f"\n  ✓ Cross-vendor correlation confirmed in top-{len(entries)}")
            passed += 1
        print()

    print("=" * 72)
    print(f"Results: {passed}/{len(SCENARIOS)} scenarios passed")
    if failed:
        print("\nTip: embeddings required for hybrid retrieval (nodes without vectors are excluded).")
        print("  curl -X POST $PCMI_BASE_URL/v1/embeddings/migrate -H 'X-API-Key: ...' \\")
        print("    -d '{\"path_prefix\":\"root.cti\",\"target_model\":\"text-embedding-3-small\"}'")
        print("  Wait ~60s for pcmi-worker, then re-run.")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
