#!/usr/bin/env python3
"""
PCMI · CTI demo: entity registry evolution + hybrid retrieval (BM25 + pgvector)
===============================================================================

Demonstrates two pillars of the cognitive graph on the multi-vendor CTI dataset:

  1. **Entity evolution (Phase D)** — extraction upserts canonical ThreatActor/Campaign
     vertices; each re-extract appends registry snapshots (alias timeline).
  2. **Hybrid retrieval** — POST /v1/retrieve fuses BM25 + pgvector so queries using
     one vendor's naming surface memories under another vendor's taxonomy.

Prerequisites:
  - CTI dataset loaded (load_multi_cti.py --reset)
  - Embeddings worker caught up (~60s after load)
  - setup_cti_extraction_demo.py (extraction + optional alias proposals)

Usage:
  python3 demo_entity_evolution_retrieval.py
  python3 demo_entity_evolution_retrieval.py --skip-retrieval
  python3 demo_entity_evolution_retrieval.py --setup   # run extraction setup first
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.abspath(__file__))
BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")
PREFIX = os.environ.get("CTI_DEMO_PREFIX", "root.cti")

CTI_ACTOR_TOKENS = (
    "Sapphire",
    "PRESSURE",
    "CHOLLIMA",
    "Forest",
    "PROMPT",
    "BRICK",
    "TraderTraitor",
    "STARDUST",
)


def _headers() -> dict[str, str]:
    return {"Content-Type": "application/json", "X-API-Key": KEY}


def http(method: str, path: str, body: dict | None = None, timeout: int = 60) -> tuple[int, dict]:
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        f"{BASE}{path}",
        data=data,
        headers=_headers(),
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"error": raw}


def section(title: str) -> None:
    print(f"\n{'=' * 72}\n{title}\n{'=' * 72}")


def check_api() -> bool:
    try:
        req = urllib.request.Request(f"{BASE}/v1/ready", headers=_headers())
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status == 200
    except urllib.error.HTTPError as e:
        if e.code == 429:
            print(
                "✗ rate limit (429) — restart API with RATE_LIMIT_DISABLED=true\n"
                "  RATE_LIMIT_DISABLED=true docker compose -f docker-compose.yml "
                "-f docker-compose.graph.override.yml --profile graph up -d api",
                file=sys.stderr,
            )
        return False
    except urllib.error.URLError:
        return False


def wait_embeddings(min_with_vector: int = 80, timeout_s: int = 120) -> bool:
    """Poll retrieve until enough CTI memories return scores (implies vectors exist)."""
    import time

    deadline = time.time() + timeout_s
    while time.time() < deadline:
        status, body = http(
            "POST",
            "/v1/retrieve",
            {"path_prefix": PREFIX, "query": "threat intelligence cross vendor", "limit": 100},
        )
        if status == 200:
            n = len(body.get("entries") or [])
            if n >= min_with_vector:
                return True
        time.sleep(3)
    return False


def show_registry_evolution() -> None:
    section("1 · Entity registry evolution (Phase D)")
    print(
        "Extraction promotes slots (actor, campaign, CVE) → canonical registry rows.\n"
        "Manual + accepted alias merges unify cross-vendor naming (same_as in AGE).\n"
        "Each successful extract appends a snapshot — the entity *evolves*\n"
        "as new vendor reports arrive, without losing prior versions.\n"
    )

    for kind in ("ThreatActor", "Campaign"):
        status, body = http("GET", f"/v1/entities/registry?kind={kind}&limit=200")
        if status != 200:
            print(f"  ✗ registry unavailable ({status}): {body.get('error', body)}")
            continue
        rows = body.get("entities") or []
        cti = [
            a
            for a in rows
            if (a.get("metadata") or {}).get("last_profile_id") == "cti.multilayer.v1"
            or any(tok.upper() in (a.get("canonical_key") or "").upper() for tok in CTI_ACTOR_TOKENS)
        ]
        print(f"  {kind} registry: {len(rows)} total · {len(cti)} CTI-relevant\n")
        if not cti:
            print(
                "  ⚠ No CTI entities yet — run:\n"
                "      python3 setup_cti_extraction_demo.py\n"
            )
            continue

        from urllib.parse import quote

        for ent in sorted(cti, key=lambda x: x.get("canonical_key", ""))[:4]:
            key = ent.get("canonical_key", "")
            print(f"  ▸ {kind}/{key}")
            enc = quote(key, safe="")
            st2, det2 = http("GET", f"/v1/entities/registry/{kind}/{enc}")
            if st2 != 200:
                print(f"      (detail API {st2})")
                continue
            snaps = det2.get("snapshots") or []
            aliases = det2.get("aliases") or []
            print(f"      snapshots: {len(snaps)} · active aliases: {len(aliases)}")
            for a in aliases[:5]:
                print(f"        · alias {a.get('alias_key')!r} ({a.get('source', '—')})")
            for snap in snaps[:2]:
                mem = snap.get("memory_id", "?")
                ver = snap.get("memory_version", "?")
                print(f"        · snapshot v{ver} mem.{mem}")
        print()


def run_retrieval_demo() -> int:
    section("2 · Hybrid retrieval (BM25 + pgvector)")
    print(
        "POST /v1/retrieve with path_prefix=root.cti runs a single SQL fusion:\n"
        "  structural ltree filter + BM25 (vendor naming) + cosine similarity (semantics)\n"
        "Cross-vendor correlation works *without* an alias table at query time.\n"
    )
    script = os.path.join(ROOT, "demo_cross_vendor_correlation.py")
    if not os.path.isfile(script):
        print(f"  ✗ missing {script}")
        return 1
    proc = subprocess.run([sys.executable, script], env=os.environ.copy())
    return proc.returncode


def run_setup() -> int:
    setup = os.path.join(ROOT, "setup_cti_extraction_demo.py")
    proc = subprocess.run([sys.executable, setup], env=os.environ.copy())
    return proc.returncode


def print_ui_hint() -> None:
    section("3 · Graph UI")
    resolver = os.path.join(ROOT, "resolve_demo_ids.py")
    proc = subprocess.run(
        [sys.executable, resolver, "--url-only"],
        capture_output=True,
        text=True,
        env=os.environ.copy(),
    )
    url = proc.stdout.strip() if proc.returncode == 0 else f"{BASE}/v1/graph/ui?demo=cti"
    url += "&autostart=1" if "autostart" not in url else ""
    print("  Open the guided tour (steps 1–12: CTI graph + Registry + retrieval):\n")
    print(f"    {url}\n")
    print("  In the UI:")
    print("    · View Registry → ThreatActor/Campaign + alias chips (APT28 ↔ Forest Blizzard)")
    print("    · Proposals → Alias → Accept all (or setup_cti_extraction_demo.py)")
    print("    · View Retrieve → scenario cross-vendor → Run → risultati verdi")
    print("    · Tour step 9–12 cover evolution + alias accept + retrieval panel")


def main() -> int:
    parser = argparse.ArgumentParser(description="CTI entity evolution + hybrid retrieval demo")
    parser.add_argument("--setup", action="store_true", help="Run setup_cti_extraction_demo.py first")
    parser.add_argument("--skip-retrieval", action="store_true")
    parser.add_argument("--skip-wait-embed", action="store_true")
    args = parser.parse_args()

    print(f"PCMI CTI evolution + retrieval demo → {BASE}")

    if not check_api():
        print(f"✗ API not reachable at {BASE}", file=sys.stderr)
        return 1

    if args.setup:
        if run_setup() != 0:
            return 1

    if not args.skip_wait_embed:
        print("\n  → waiting for embeddings (hybrid retrieve needs pgvector)…", end="", flush=True)
        if wait_embeddings(min_with_vector=50, timeout_s=90):
            print(" ok")
        else:
            print(" timeout (retrieval may rank poorly until worker finishes)")

    show_registry_evolution()

    if not args.skip_retrieval:
        rc = run_retrieval_demo()
        if rc != 0:
            return rc

    print_ui_hint()
    return 0


if __name__ == "__main__":
    sys.exit(main())
