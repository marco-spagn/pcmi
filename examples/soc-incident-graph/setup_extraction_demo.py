#!/usr/bin/env python3
"""
PCMI · SOC entity extraction + LLM link proposal demo setup
============================================================
After load_to_pcmi.py, registers the soc.siem.v1 profile and runs extraction
(and optionally LLM link proposals) on a small subset of loaded memories.

Requires:
  EXTRACTION_ENABLED=true (+ OPENAI_API_KEY) on the API
  LINK_PROPOSALS_ENABLED=true for proposal generation
  Apache AGE (postgres-age) for entity promotion

Usage:
  export PCMI_BASE_URL=http://localhost:8000
  export PCMI_API_KEY=testkey123
  python3 setup_extraction_demo.py
  python3 setup_extraction_demo.py --extract-limit 8 --propose-limit 3
  python3 setup_extraction_demo.py --skip-propose
"""
import argparse
import json
import os
import sys
import time

ROOT = os.path.dirname(os.path.abspath(__file__))
PROFILE_PATH = os.path.join(
    ROOT, "..", "cognitive-graph-entities", "soc.siem.v1.profile.json"
)
ID_MAP = os.path.join(ROOT, "id_map.json")

# Akira campaign chain — good for causal/temporal link proposals
PRIORITY_EXTERNAL = [
    "INC0000001",
    "INC0000002",
    "INC0000003",
    "INC0000004",
    "INC0000005",
    "INC0000006",
    "INC0000007",
    "INC0000008",
    "INC0000009",
    "INC0000010",
    "INC0000011",
    "INC0000012",
]

sys.path.insert(0, ROOT)
from load_to_pcmi import http  # noqa: E402


def die(msg, code=1):
    print(f"  ✗ {msg}", file=sys.stderr)
    sys.exit(code)


def ok(msg):
    print(f"  ✓ {msg}")


def info(msg):
    print(f"  → {msg}")


def register_profile():
    if not os.path.exists(PROFILE_PATH):
        die(f"profile not found: {PROFILE_PATH}")
    with open(PROFILE_PATH, encoding="utf-8") as f:
        profile = json.load(f)
    body = {
        "path_prefix": "root.soc",
        "enabled": True,
        "profile": profile,
    }
    status, resp = http("PUT", "/v1/extraction-profiles/soc.siem.v1", body, timeout=30)
    if status not in (200, 201):
        die(f"register profile failed ({status}): {resp}")
    ok(f"profile soc.siem.v1 registered (path_prefix=root.soc)")


def memory_ids(limit):
    if not os.path.exists(ID_MAP):
        die(f"id_map missing — run load_to_pcmi.py first ({ID_MAP})")
    id_map = json.load(open(ID_MAP, encoding="utf-8"))
    ordered = []
    seen = set()
    for ext in PRIORITY_EXTERNAL:
        mid = id_map.get(ext)
        if mid and mid not in seen:
            ordered.append(int(mid))
            seen.add(mid)
        if len(ordered) >= limit:
            return ordered
    for mid in id_map.values():
        mid = int(mid)
        if mid in seen:
            continue
        ordered.append(mid)
        seen.add(mid)
        if len(ordered) >= limit:
            break
    return ordered


def run_extractions(mem_ids):
    ok_count = 0
    fail_count = 0
    for i, mid in enumerate(mem_ids, 1):
        info(f"[{i}/{len(mem_ids)}] extracting memory.{mid} …")
        status, resp = http("POST", f"/v1/memories/extraction/{mid}", timeout=180)
        if status == 503:
            die(
                "extraction disabled — set EXTRACTION_ENABLED=true on API/worker "
                "and restart containers"
            )
        if status == 200:
            slots = (resp.get("extraction") or {}).get("slots") or {}
            ok(f"memory.{mid} ok · {len(slots)} slots")
            ok_count += 1
        else:
            err = resp.get("error") or resp
            print(f"    ⚠ memory.{mid} failed ({status}): {err}")
            fail_count += 1
        time.sleep(0.3)
    return ok_count, fail_count


def run_proposals(mem_ids):
    created = 0
    for i, mid in enumerate(mem_ids, 1):
        info(f"[{i}/{len(mem_ids)}] LLM link proposals for memory.{mid} …")
        status, resp = http(
            "POST", f"/v1/graph/link-proposals/generate/{mid}", timeout=180
        )
        if status == 503:
            die(
                "link proposals disabled — set LINK_PROPOSALS_ENABLED=true "
                "on API/worker and restart containers"
            )
        if status == 200:
            n = resp.get("count") or len(resp.get("proposals") or [])
            ok(f"memory.{mid} → {n} proposal(s)")
            created += n
        elif status == 422:
            print(f"    ⚠ memory.{mid}: {resp.get('error', resp)}")
        else:
            print(f"    ⚠ memory.{mid} failed ({status}): {resp.get('error', resp)}")
        time.sleep(0.5)
    return created


def main():
    parser = argparse.ArgumentParser(description="SOC extraction + link proposal demo setup")
    parser.add_argument("--extract-limit", type=int, default=12)
    parser.add_argument("--propose-limit", type=int, default=5)
    parser.add_argument("--skip-extract", action="store_true")
    parser.add_argument("--skip-propose", action="store_true")
    args = parser.parse_args()

    print("\n━━━ Entity extraction demo setup ━━━\n")
    register_profile()

    mem_ids = memory_ids(args.extract_limit)
    if not mem_ids:
        die("no memory ids in id_map")
    info(f"target memories: {mem_ids[:6]}{'…' if len(mem_ids) > 6 else ''}")

    if not args.skip_extract:
        print()
        info(f"Running LLM extraction on {len(mem_ids)} memories …")
        ok_n, fail_n = run_extractions(mem_ids)
        ok(f"extraction done · {ok_n} ok, {fail_n} failed")
    else:
        info("skipping extraction (--skip-extract)")

    if not args.skip_propose:
        propose_ids = mem_ids[: args.propose_limit]
        print()
        info(f"Generating LLM link proposals for {len(propose_ids)} memories …")
        total = run_proposals(propose_ids)
        ok(f"proposals queued · {total} total pending rows")
    else:
        info("skipping link proposals (--skip-propose)")

    print()
    print("  Open the graph UI and switch to Entities / Proposals view:")
    base = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
    print(f"    {base}/v1/graph/ui")
    print()


if __name__ == "__main__":
    main()
