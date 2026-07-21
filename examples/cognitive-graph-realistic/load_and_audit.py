#!/usr/bin/env python3
"""
Load the realistic Cognitive Graph dataset into PCMI and audit graph behavior.

Designed to surface bugs and unexpected API behavior at scale (default: 1200 nodes,
1389 links). Saves id_map.json for UI exploration.

Usage:
  export PCMI_BASE_URL=http://localhost:8000
  export PCMI_API_KEY=testkey123
  python3 load_and_audit.py
  python3 load_and_audit.py --skip-load --audit-only   # reuse id_map.json
  python3 load_and_audit.py --limit 500                # partial load
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import Counter, defaultdict
from pathlib import Path


ROOT = Path(__file__).resolve().parent
DATASET = ROOT / "graph_realistic_large.json"
PROFILE = ROOT.parent / "cognitive-graph-entities" / "generic.record.v1.profile.json"
ID_MAP = ROOT / "realistic_id_map.json"
BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")

LINK_TYPES = ("causal", "temporal", "supports", "contradicts", "related")


def http(method: str, path: str, payload=None, timeout=60):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    req.add_header("X-API-Key", KEY)
    for attempt in range(1, 6):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                body = resp.read().decode()
                return resp.status, json.loads(body) if body else {}
        except urllib.error.HTTPError as e:
            body = e.read().decode()
            if e.code == 429 and attempt < 5:
                time.sleep(2 * attempt)
                continue
            try:
                parsed = json.loads(body) if body else {"error": body}
            except json.JSONDecodeError:
                parsed = {"error": body}
            return e.code, parsed
        except urllib.error.URLError as e:
            if attempt < 5:
                time.sleep(2)
                continue
            raise RuntimeError(f"request failed: {e}") from e
    raise RuntimeError("unreachable")


def die(msg: str, code: int = 1) -> None:
    print(f"  ✗ {msg}", file=sys.stderr)
    sys.exit(code)


def ok(msg: str) -> None:
    print(f"  ✓ {msg}")


def warn(msg: str) -> None:
    print(f"  ⚠ {msg}")


def info(msg: str) -> None:
    print(f"  → {msg}")


def load_dataset(limit: int) -> tuple[list, list, dict]:
    data = json.loads(DATASET.read_text(encoding="utf-8"))
    nodes = data["nodes"] if limit == 0 else data["nodes"][:limit]
    keys = {n["key"] for n in nodes}
    links = [l for l in data["links"] if l["from"] in keys and l["to"] in keys]
    return nodes, links, data.get("metadata", {})


def load_into_pcmi(nodes: list, links: list) -> dict[str, int]:
    id_map: dict[str, int] = {}
    info(f"Loading {len(nodes)} nodes …")
    for i, node in enumerate(nodes, 1):
        payload = {
            "path": node["path"],
            "content": node["content"],
            "tags": node.get("tags", []),
            "metadata": node.get("metadata", {}),
        }
        status, resp = http("POST", "/v1/memories", payload)
        if status not in (200, 201):
            die(f"store node {node['key']} failed ({status}): {resp}")
        mid = resp.get("id")
        if not mid:
            die(f"store returned no id for {node['key']}")
        id_map[node["key"]] = int(mid)
        if i % 100 == 0:
            info(f"  … {i}/{len(nodes)} nodes stored")
    info(f"Loading {len(links)} links …")
    created = 0
    errors = 0
    for i, link in enumerate(links, 1):
        payload = {
            "from_path": f"memory.{id_map[link['from']]}",
            "to_path": f"memory.{id_map[link['to']]}",
            "link_type": link["type"],
            "metadata": {
                "rationale": link.get("rationale", ""),
                "weight": link.get("weight", 1.0),
            },
        }
        status, resp = http("POST", "/v1/memories/links", payload)
        if status in (200, 201):
            created += 1
        else:
            errors += 1
            if errors <= 5:
                warn(f"link {link['from']}→{link['to']} ({link['type']}): {status} {resp}")
        if i % 200 == 0:
            info(f"  … {i}/{len(links)} links processed")
    ok(f"stored {len(nodes)} nodes, {created} links ({errors} errors)")
    ID_MAP.write_text(json.dumps(id_map, indent=2), encoding="utf-8")
    ok(f"id map saved → {ID_MAP.name}")
    return id_map


def build_adjacency(links: list) -> dict[str, dict[str, set[str]]]:
    """key -> link_type -> set of neighbor keys (undirected, matches direction=both)."""
    adj: dict[str, dict[str, set[str]]] = defaultdict(lambda: defaultdict(set))
    for link in links:
        src, dst, typ = link["from"], link["to"], link["type"]
        adj[src][typ].add(dst)
        adj[dst][typ].add(src)
    return adj


def audit_incoming_neighbors(links: list, id_map: dict[str, int]) -> list[str]:
    """Verify incoming edges are visible with default bidirectional traversal."""
    bugs: list[str] = []
    cross = [
        l
        for l in links
        if l["from"].startswith("noise_")
        and l["to"].startswith("campaign_")
        and l["type"] in ("supports", "contradicts", "related")
    ]
    if not cross:
        return bugs
    sample = cross[0]
    target_key = sample["to"]
    mid = id_map.get(target_key)
    if not mid:
        return bugs
    q = urllib.parse.urlencode(
        {"memory_id": mid, "depth": 1, "link_types": sample["type"], "limit": 50}
    )
    status, resp = http("GET", f"/v1/graph/related?{q}")
    if status != 200:
        bugs.append(f"cross-campaign incoming {sample['from']}→{target_key}: HTTP {status}")
        return bugs
    got_ids = {e.get("id") for e in (resp.get("entries") or [])}
    source_id = id_map.get(sample["from"])
    if source_id not in got_ids:
        bugs.append(
            f"incoming edge {sample['from']}→{target_key} ({sample['type']}) "
            f"not visible from target with direction=both"
        )
    else:
        ok(f"incoming cross-campaign edge visible from {target_key}")
    return bugs


def register_profile() -> None:
    if not PROFILE.exists():
        warn(f"profile missing: {PROFILE}")
        return
    profile = json.loads(PROFILE.read_text(encoding="utf-8"))
    body = {
        "path_prefix": "root.realistic_graph",
        "enabled": True,
        "profile": profile,
    }
    status, resp = http("PUT", "/v1/extraction-profiles/generic.record.v1", body)
    if status not in (200, 201):
        warn(f"register profile failed ({status}): {resp}")
    else:
        ok("profile generic.record.v1 registered (path_prefix=root.realistic_graph)")


def audit_health() -> None:
    status, health = http("GET", "/v1/graph/health")
    if status != 200:
        die(f"graph health failed ({status}): {health}")
    if health.get("available") is not True:
        die(f"AGE unavailable: {health}")
    ok("graph health: AGE available")


def audit_isolated(nodes: list, links: list, id_map: dict[str, int]) -> list[str]:
    linked_keys = set()
    for link in links:
        linked_keys.add(link["from"])
        linked_keys.add(link["to"])
    isolated = [n for n in nodes if n["key"] not in linked_keys]
    bugs: list[str] = []
    sample = isolated[:12]
    info(f"Checking {len(sample)}/{len(isolated)} isolated nodes (expect 0 related at depth=2)")
    for node in sample:
        mid = id_map[node["key"]]
        q = urllib.parse.urlencode({"memory_id": mid, "depth": 2, "limit": 50})
        status, resp = http("GET", f"/v1/graph/related?{q}")
        if status != 200:
            bugs.append(f"isolated {node['key']} (id={mid}): HTTP {status}")
            continue
        total = resp.get("total", 0)
        if total != 0:
            entries = resp.get("entries") or []
            bugs.append(
                f"isolated {node['key']} (id={mid}): expected total=0, got {total} "
                f"(sample ids: {[e.get('id') for e in entries[:3]]})"
            )
    if not bugs:
        ok(f"isolated nodes return empty related ({len(isolated)} in dataset)")
    return bugs


def audit_link_type_filters(
    nodes: list, links: list, id_map: dict[str, int], adj: dict
) -> list[str]:
    bugs: list[str] = []
    # Pick nodes with at least one outgoing edge per type
    by_type: dict[str, str] = {}
    for link in links:
        by_type.setdefault(link["type"], link["from"])
    info(f"Link-type filter audit on {len(by_type)} types")
    for typ in LINK_TYPES:
        key = by_type.get(typ)
        if not key or key not in id_map:
            continue
        mid = id_map[key]
        expected = adj[key].get(typ, set())
        q = urllib.parse.urlencode(
            {"memory_id": mid, "depth": 1, "link_types": typ, "limit": 200}
        )
        status, resp = http("GET", f"/v1/graph/related?{q}")
        if status != 200:
            bugs.append(f"{typ} filter on {key}: HTTP {status}")
            continue
        got_keys = set()
        for entry in resp.get("entries") or []:
            eid = entry.get("id")
            for nk, nv in id_map.items():
                if nv == eid:
                    got_keys.add(nk)
                    break
        missing = expected - got_keys
        extra = got_keys - expected
        if missing or extra:
            bugs.append(
                f"{typ} depth=1 from {key}: missing={len(missing)} extra={len(extra)} "
                f"(expected {len(expected)}, got {len(got_keys)})"
            )
    if not bugs:
        ok("link_type filters match dataset neighbors at depth=1")
    return bugs


def audit_contradicts_branch(links: list, id_map: dict[str, int]) -> list[str]:
    bugs: list[str] = []
    contradicts = [l for l in links if l["type"] == "contradicts"]
    if not contradicts:
        return bugs
    sample = contradicts[:8]
    info(f"Contradicts traversal on {len(sample)} edges")
    for link in sample:
        mid = id_map[link["from"]]
        q = urllib.parse.urlencode(
            {
                "memory_id": mid,
                "depth": 1,
                "link_types": "contradicts",
                "limit": 50,
            }
        )
        status, resp = http("GET", f"/v1/graph/related?{q}")
        if status != 200:
            bugs.append(f"contradicts from {link['from']}: HTTP {status}")
            continue
        ids = {e.get("id") for e in (resp.get("entries") or [])}
        target = id_map.get(link["to"])
        if target not in ids:
            bugs.append(
                f"contradicts edge {link['from']}→{link['to']} not visible in /related"
            )
    if not bugs:
        ok("contradicts edges visible via link_types=contradicts")
    return bugs


def audit_campaign_chain(nodes: list, id_map: dict[str, int]) -> list[str]:
    bugs: list[str] = []
    campaigns = [n for n in nodes if n.get("metadata", {}).get("kind") == "campaign"]
    if len(campaigns) < 2:
        return bugs
    c0 = campaigns[0]
    # find furthest alert in same campaign by key prefix
    prefix = c0["key"]
    same = [
        n
        for n in nodes
        if n["key"].startswith(prefix) and n.get("metadata", {}).get("kind") == "alert"
    ]
    if not same:
        return bugs
    target = same[-1]
    from_id = id_map[c0["key"]]
    to_id = id_map[target["key"]]
    q = urllib.parse.urlencode(
        {"from": from_id, "to": to_id, "max_depth": 12, "link_types": "causal,temporal,supports,related"}
    )
    status, resp = http("GET", f"/v1/graph/chain?{q}")
    if status != 200:
        bugs.append(f"campaign chain {c0['key']}→{target['key']}: HTTP {status}")
        return bugs
    connected = resp.get("connected")
    hops = resp.get("hops")
    if connected is not True:
        bugs.append(
            f"campaign chain {c0['key']}→{target['key']}: connected={connected} hops={hops}"
        )
    else:
        ok(f"campaign chain reachable ({c0['key']}→{target['key']}, hops={hops})")
    return bugs


def audit_disposition_coverage(nodes: list, id_map: dict[str, int]) -> list[str]:
    bugs: list[str] = []
    by_disp: dict[str, list] = defaultdict(list)
    for n in nodes:
        d = n.get("metadata", {}).get("disposition")
        if d:
            by_disp[d].append(n)
    info(f"Disposition samples: {', '.join(f'{k}={len(v)}' for k, v in sorted(by_disp.items()))}")
    for disp in ("false_positive", "duplicate", "benign_true_positive"):
        sample = by_disp.get(disp, [])[:3]
        for node in sample:
            mid = id_map[node["key"]]
            q = urllib.parse.urlencode({"memory_id": mid, "depth": 3, "limit": 100})
            status, resp = http("GET", f"/v1/graph/related?{q}")
            if status != 200:
                bugs.append(f"{disp} {node['key']}: HTTP {status}")
            elif resp.get("total", 0) == 0:
                bugs.append(f"{disp} {node['key']}: zero related at depth=3 (unexpected)")
    if not bugs:
        ok("disposition-tagged alerts have graph reachability")
    return bugs


def audit_extraction_sample(nodes: list, id_map: dict[str, int], sample_n: int) -> list[str]:
    bugs: list[str] = []
    if sample_n <= 0:
        return bugs
    register_profile()
    picks = [
        n
        for n in nodes
        if n.get("metadata", {}).get("kind") in ("alert", "campaign", "hypothesis")
    ][:sample_n]
    info(f"Extraction probe on {len(picks)} memories")
    for node in picks:
        mid = id_map[node["key"]]
        status, resp = http("POST", f"/v1/memories/extraction/{mid}", timeout=180)
        if status == 503:
            warn("extraction disabled — skip extraction audit")
            return []
        if status != 200:
            bugs.append(f"extract {node['key']} (id={mid}): HTTP {status} {resp.get('error', resp)}")
            continue
        ext = (resp.get("extraction") or {})
        st = ext.get("status")
        if st != "ok":
            bugs.append(f"extract {node['key']}: status={st} err={ext.get('error')}")
    if not bugs:
        ok(f"extraction ok on {len(picks)} samples")
    return bugs


def audit_entities_related(id_map: dict[str, int], mem_id: int) -> list[str]:
    bugs: list[str] = []
    status, resp = http("GET", f"/v1/graph/entities/related?memory_id={mem_id}&limit=50")
    if status == 501:
        warn("entities/related not available (501)")
        return []
    if status != 200:
        bugs.append(f"entities/related memory.{mem_id}: HTTP {status}")
        return bugs
    if "entries" not in resp and "memories" in resp:
        bugs.append("entities/related returns 'memories' key — UI bug (expects 'entries')")
    ok(f"entities/related memory.{mem_id}: total={resp.get('total', '?')}")
    return bugs


def main() -> None:
    parser = argparse.ArgumentParser(description="Load + audit realistic cognitive graph")
    parser.add_argument("--limit", type=int, default=0, help="nodes to load; 0 = full dataset")
    parser.add_argument("--skip-load", action="store_true", help="use existing realistic_id_map.json")
    parser.add_argument("--audit-only", action="store_true", help="alias for --skip-load")
    parser.add_argument("--extract-sample", type=int, default=3, help="extraction probes (0=skip)")
    args = parser.parse_args()
    if args.audit_only:
        args.skip_load = True

    print("\n━━━ Realistic graph load + audit ━━━\n")
    nodes, links, meta = load_dataset(args.limit)
    info(f"Dataset slice: {len(nodes)} nodes, {len(links)} links")

    if args.skip_load:
        if not ID_MAP.exists():
            die(f"missing {ID_MAP} — run without --skip-load first")
        id_map = {k: int(v) for k, v in json.loads(ID_MAP.read_text()).items()}
        ok(f"loaded id map ({len(id_map)} keys)")
    else:
        status, _ = http("GET", "/v1/ready")
        if status != 200:
            die(f"API not ready at {BASE} ({status})")
        id_map = load_into_pcmi(nodes, links)

    adj = build_adjacency(links)
    all_bugs: list[str] = []

    audit_health()
    all_bugs.extend(audit_isolated(nodes, links, id_map))
    all_bugs.extend(audit_link_type_filters(nodes, links, id_map, adj))
    all_bugs.extend(audit_incoming_neighbors(links, id_map))
    all_bugs.extend(audit_contradicts_branch(links, id_map))
    all_bugs.extend(audit_campaign_chain(nodes, id_map))
    all_bugs.extend(audit_disposition_coverage(nodes, id_map))
    all_bugs.extend(audit_extraction_sample(nodes, id_map, args.extract_sample))

    # entities probe on first campaign memory
    campaigns = [n for n in nodes if n.get("metadata", {}).get("kind") == "campaign"]
    if campaigns:
        all_bugs.extend(audit_entities_related(id_map, id_map[campaigns[0]["key"]]))

    print()
    if all_bugs:
        print(f"  Found {len(all_bugs)} issue(s):\n")
        for b in all_bugs:
            print(f"    • {b}")
        print()
        # UI hint
        if campaigns:
            mem = id_map[campaigns[0]["key"]]
            print(f"  Explore in UI: {BASE}/v1/graph/ui?mem={mem}")
        sys.exit(1)

    ok("all structural audits passed")
    if campaigns:
        mem = id_map[campaigns[0]["key"]]
        print()
        print(f"  UI: {BASE}/v1/graph/ui?mem={mem}")
        print(f"  Campaign: {campaigns[0]['metadata'].get('campaign')} (memory id {mem})")
    print()


if __name__ == "__main__":
    main()
