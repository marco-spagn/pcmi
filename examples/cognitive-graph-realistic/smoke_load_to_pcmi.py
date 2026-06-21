#!/usr/bin/env python3
"""Load a realistic Cognitive Graph sample into a live PCMI API and smoke it."""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parent
BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")


def http(method: str, path: str, payload=None, timeout=30):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    req.add_header("X-API-Key", KEY)
    for attempt in range(1, 5):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                body = resp.read().decode()
                return resp.status, json.loads(body) if body else {}
        except urllib.error.HTTPError as e:
            body = e.read().decode()
            if e.code == 429 and attempt < 4:
                time.sleep(2 * attempt)
                continue
            raise RuntimeError(f"HTTP {e.code}: {body}") from e


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dataset", default=str(ROOT / "graph_realistic_large.json"))
    parser.add_argument("--limit", type=int, default=250, help="nodes to load; 0 loads the full dataset")
    args = parser.parse_args()

    data = json.loads(Path(args.dataset).read_text(encoding="utf-8"))
    nodes = data["nodes"] if args.limit == 0 else data["nodes"][:args.limit]
    loaded_keys = {node["key"] for node in nodes}
    links = [link for link in data["links"] if link["from"] in loaded_keys and link["to"] in loaded_keys]
    if len(nodes) < 50:
        raise SystemExit("realistic smoke requires at least 50 nodes")
    if not links:
        raise SystemExit("realistic smoke selected no links")

    id_map: dict[str, int] = {}
    for node in nodes:
        payload = {
            "path": node["path"],
            "content": node["content"],
            "tags": node.get("tags", []),
            "metadata": node.get("metadata", {}),
        }
        _, resp = http("POST", "/v1/memories", payload)
        memory_id = resp.get("id")
        if not memory_id:
            raise RuntimeError(f"store returned no id for {node['key']}: {resp}")
        id_map[node["key"]] = memory_id

    created_links = 0
    for link in links:
        payload = {
            "from_path": f"memory.{id_map[link['from']]}",
            "to_path": f"memory.{id_map[link['to']]}",
            "link_type": link["type"],
            "metadata": {"rationale": link.get("rationale", ""), "weight": link.get("weight", 1.0)},
        }
        st, _ = http("POST", "/v1/memories/links", payload)
        if st in (200, 201):
            created_links += 1

    if created_links == 0:
        raise RuntimeError("no graph links created")

    _, health = http("GET", "/v1/graph/health")
    if health.get("available") is not True:
        raise RuntimeError(f"graph unavailable after realistic load: {health}")

    first_from = links[0]["from"]
    first_id = id_map[first_from]
    query = urllib.parse.urlencode({"memory_id": first_id, "depth": 2, "limit": 20})
    _, related = http("GET", f"/v1/graph/related?{query}")
    if related.get("total", 0) <= 0:
        raise RuntimeError(f"realistic graph traversal returned no entries from {first_from}: {related}")

    print(
        "OK: realistic graph smoke loaded "
        f"{len(nodes)} nodes, {created_links} links; related({first_from}) total={related.get('total')}"
    )


if __name__ == "__main__":
    main()
