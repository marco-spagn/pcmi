#!/usr/bin/env python3
"""
PCMI · CTI entity extraction + registry evolution demo setup
============================================================
After load_multi_cti.py, registers cti.multilayer.v1 and runs LLM extraction
on cross-vendor threat-actor memories so Phase D registry + AGE entities populate.

Requires EXTRACTION_ENABLED=true (+ OPENAI_API_KEY) on API/worker.
Optional: ENTITY_ALIAS_PROPOSALS_ENABLED=true for alias merge proposals.

Usage:
  export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
  python3 setup_cti_extraction_demo.py
  python3 setup_cti_extraction_demo.py --skip-alias-propose
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.abspath(__file__))
PROFILE_PATH = os.path.join(
    ROOT, "..", "cognitive-graph-entities", "cti.multilayer.v1.profile.json"
)
RESOLVER = os.path.join(ROOT, "resolve_demo_ids.py")

sys.path.insert(0, ROOT)
from load_to_pcmi import http  # noqa: E402

# Cross-vendor tour anchors — extraction here feeds registry + graph entities.
PRIORITY_KEYS = (
    "ms_sapphire",
    "cs_pressure",
    "mdt_ps",
    "ms_forest",
    "apt28",
)

# Analyst-approved cross-vendor merges (Phase D registry + same_as in AGE).
DEMO_MANUAL_ALIASES: tuple[dict, ...] = (
    # APT28 nexus — Mandiant / GTIG / Microsoft naming
    {
        "kind": "ThreatActor",
        "canonical_key": "APT28 / FROZENLAKE",
        "alias_key": "APT28",
        "confidence": 0.95,
    },
    {
        "kind": "ThreatActor",
        "canonical_key": "APT28 / FROZENLAKE",
        "alias_key": "Forest Blizzard",
        "confidence": 0.92,
    },
    {
        "kind": "ThreatActor",
        "canonical_key": "APT28 / FROZENLAKE",
        "alias_key": "FROZENLAKE",
        "confidence": 0.98,
    },
    # DPRK umbrella — CrowdStrike ↔ Microsoft
    {
        "kind": "ThreatActor",
        "canonical_key": "Sapphire Sleet",
        "alias_key": "PRESSURE CHOLLIMA",
        "confidence": 0.9,
    },
    # Malware/campaign naming — PROMPTSTEAL ↔ CERT-UA LAMEHUG
    {
        "kind": "Campaign",
        "canonical_key": "PROMPTSTEAL",
        "alias_key": "LAMEHUG",
        "confidence": 0.95,
    },
)

CTI_ACTOR_TOKENS = (
    "Sapphire",
    "PRESSURE",
    "CHOLLIMA",
    "Forest",
    "PROMPT",
    "BRICK",
    "TraderTraitor",
    "APT28",
)


def die(msg: str, code: int = 1) -> None:
    print(f"  ✗ {msg}", file=sys.stderr)
    sys.exit(code)


def ok(msg: str) -> None:
    print(f"  ✓ {msg}")


def info(msg: str) -> None:
    print(f"  → {msg}")


def register_profile() -> None:
    if not os.path.isfile(PROFILE_PATH):
        die(f"profile missing: {PROFILE_PATH}")
    with open(PROFILE_PATH, encoding="utf-8") as f:
        profile = json.load(f)
    status, resp = http(
        "PUT",
        "/v1/extraction-profiles/cti.multilayer.v1",
        {"path_prefix": "root.cti", "enabled": True, "profile": profile},
        timeout=30,
    )
    if status not in (200, 201):
        die(f"register profile failed ({status}): {resp}")
    ok("profile cti.multilayer.v1 registered (path_prefix=root.cti)")


def resolve_tour_ids() -> dict[str, int]:
    import subprocess

    proc = subprocess.run(
        [sys.executable, RESOLVER, "--json"],
        capture_output=True,
        text=True,
        env=os.environ.copy(),
        check=False,
    )
    if proc.returncode != 0:
        die(f"resolve_demo_ids failed: {proc.stderr or proc.stdout}")
    return {k: int(v) for k, v in json.loads(proc.stdout).items()}


def memory_ids(limit: int) -> list[int]:
    tour = resolve_tour_ids()
    ordered: list[int] = []
    seen: set[int] = set()
    for key in PRIORITY_KEYS:
        mid = tour.get(key)
        if mid and mid not in seen:
            ordered.append(mid)
            seen.add(mid)
        if len(ordered) >= limit:
            return ordered
    id_map_path = os.path.join(ROOT, "id_map.json")
    if os.path.isfile(id_map_path):
        id_map = json.load(open(id_map_path, encoding="utf-8"))
        for mid in id_map.values():
            mid = int(mid)
            if mid in seen:
                continue
            ordered.append(mid)
            seen.add(mid)
            if len(ordered) >= limit:
                break
    return ordered


def run_extractions(mem_ids: list[int]) -> tuple[int, int]:
    ok_n = fail_n = 0
    for i, mid in enumerate(mem_ids, 1):
        info(f"[{i}/{len(mem_ids)}] extracting memory.{mid} …")
        status, resp = http("POST", f"/v1/memories/extraction/{mid}", timeout=180)
        if status == 503:
            die("EXTRACTION_ENABLED=false — restart API/worker with extraction on")
        if status == 200:
            slots = (resp.get("extraction") or {}).get("slots") or {}
            actor = slots.get("actor") or "—"
            ok(f"memory.{mid} · actor={actor!r} · {len(slots)} slots")
            ok_n += 1
        else:
            print(f"    ⚠ memory.{mid} failed ({status}): {resp.get('error', resp)}")
            fail_n += 1
        time.sleep(0.4)
    return ok_n, fail_n


def seed_manual_aliases() -> tuple[int, int]:
    ok_n = skip_n = 0
    for spec in DEMO_MANUAL_ALIASES:
        status, resp = http("POST", "/v1/entities/registry/aliases", spec, timeout=30)
        label = f"{spec['kind']}/{spec['alias_key']} → {spec['canonical_key']}"
        if status == 200:
            ok(f"manual alias · {label}")
            ok_n += 1
        else:
            err = resp.get("error", resp)
            if "duplicate" in str(err).lower() or "already" in str(err).lower():
                info(f"alias già presente · {label}")
                skip_n += 1
            else:
                print(f"    ⚠ {label} ({status}): {err}")
        time.sleep(0.15)
    return ok_n, skip_n


def accept_pending_aliases(limit: int = 100) -> int:
    status, resp = http(
        "GET",
        f"/v1/graph/entity-alias-proposals?status=pending&limit={limit}",
        timeout=30,
    )
    if status != 200:
        print(f"  ⚠ list alias proposals failed ({status}): {resp.get('error', resp)}")
        return 0
    proposals = resp.get("proposals") or []
    accepted = 0
    for p in proposals:
        pid = p.get("id")
        if pid is None:
            continue
        kind = p.get("kind", "?")
        alias = p.get("alias_key", "?")
        st, body = http(
            "POST",
            f"/v1/graph/entity-alias-proposals/{pid}/accept",
            timeout=30,
        )
        if st == 200:
            ok(f"accepted proposal #{pid} · {kind}/{alias}")
            accepted += 1
        else:
            print(f"    ⚠ accept #{pid} ({st}): {body.get('error', body)}")
        time.sleep(0.2)
    return accepted


def run_alias_proposals(mem_ids: list[int]) -> int:
    created = 0
    for i, mid in enumerate(mem_ids, 1):
        info(f"[{i}/{len(mem_ids)}] alias proposals for memory.{mid} …")
        status, resp = http(
            "POST", f"/v1/graph/entity-alias-proposals/generate/{mid}", timeout=180
        )
        if status == 503:
            print(f"    ⚠ alias proposals disabled: {resp.get('hint', resp)}")
            return created
        if status == 200:
            n = resp.get("count") or len(resp.get("proposals") or [])
            ok(f"memory.{mid} → {n} alias proposal(s)")
            created += n
        else:
            print(f"    ⚠ memory.{mid} ({status}): {resp.get('error', resp)}")
        time.sleep(0.5)
    return created


def registry_summary() -> None:
    status, resp = http("GET", "/v1/entities/registry?kind=ThreatActor&limit=200", timeout=30)
    if status != 200:
        print(f"  ⚠ registry list failed ({status})")
        return
    actors = resp.get("entities") or []
    cti_keys = sorted(
        a["canonical_key"]
        for a in actors
        if any(tok.upper() in (a.get("canonical_key") or "").upper() for tok in CTI_ACTOR_TOKENS)
        or (a.get("metadata") or {}).get("last_profile_id") == "cti.multilayer.v1"
    )
    ok(f"ThreatActor registry: {len(actors)} total · {len(cti_keys)} CTI-relevant")
    for key in sorted(cti_keys)[:8]:
        print(f"      · {key}")


def main() -> int:
    parser = argparse.ArgumentParser(description="CTI extraction + registry evolution setup")
    parser.add_argument("--extract-limit", type=int, default=6)
    parser.add_argument("--alias-limit", type=int, default=2)
    parser.add_argument("--skip-extract", action="store_true")
    parser.add_argument("--skip-alias-propose", action="store_true")
    parser.add_argument("--skip-manual-aliases", action="store_true")
    parser.add_argument("--skip-alias-accept", action="store_true")
    args = parser.parse_args()

    print("\n━━━ CTI entity extraction + registry (Phase D) ━━━\n")
    register_profile()

    mem_ids = memory_ids(args.extract_limit)
    if not mem_ids:
        die("no CTI memory IDs — run load_multi_cti.py first")
    info(f"target memories: {mem_ids}")

    if not args.skip_extract:
        print()
        ok_n, fail_n = run_extractions(mem_ids)
        ok(f"extraction · {ok_n} ok, {fail_n} failed")
        print()
        registry_summary()

    if not args.skip_manual_aliases:
        print()
        info("Manual alias merges (cross-vendor demo pairs) …")
        manual_ok, manual_skip = seed_manual_aliases()
        ok(f"manual aliases · {manual_ok} added, {manual_skip} already present")

    if not args.skip_alias_propose:
        alias_ids = mem_ids[: args.alias_limit]
        print()
        info(f"LLM alias proposals (cross-vendor merge hints) on {alias_ids} …")
        total = run_alias_proposals(alias_ids)
        ok(f"alias proposals queued · {total} pending")

    if not args.skip_alias_accept:
        print()
        info("Accepting pending alias proposals (analyst review simulation) …")
        accepted = accept_pending_aliases()
        ok(f"alias proposals accepted · {accepted}")

    base = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
    print()
    print("  Graph UI (Registry + tour):")
    print(f"    {base}/v1/graph/ui?demo=cti")
    print("  Hybrid retrieval demo:")
    print("    python3 demo_entity_evolution_retrieval.py")
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
