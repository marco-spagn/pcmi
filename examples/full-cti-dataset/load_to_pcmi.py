#!/usr/bin/env python3
"""
PCMI · Full CTI JSON dataset loader (SOC + vendors + TI Mindmap HUB)
==================================================================
Loads nodes + links from full_cti_dataset.json into PCMI.

  export PCMI_BASE_URL=http://localhost:8000
  export PCMI_API_KEY=testkey123
  export CTI_DATASET_JSON=examples/full-cti-dataset/data/full_cti_dataset.json
  python3 load_to_pcmi.py
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import threading
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed

ROOT = os.path.dirname(os.path.abspath(__file__))
BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")
DATASET_JSON = os.environ.get(
    "CTI_DATASET_JSON",
    os.path.join(ROOT, "data", "full_cti_dataset.json"),
)
ID_MAP = os.path.join(ROOT, "id_map.json")
ID_MAP_META = os.path.join(ROOT, "id_map.meta.json")
MAX_BATCH = 50
DEFAULT_LINK_WORKERS = int(os.environ.get("PCMI_LINK_WORKERS", "4"))
LINK_MAX_ATTEMPTS = 12
PUBLIC_LINK_TYPES = {"causal", "temporal", "contradicts", "supports", "related"}


def http(method, path, payload=None, timeout=60):
    url = BASE + path
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    req.add_header("X-API-Key", KEY)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            body = r.read().decode()
            return r.status, (json.loads(body) if body else {})
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        try:
            parsed = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed = {"error": body}
        parsed["_retry_after"] = e.headers.get("Retry-After", "")
        return e.code, parsed


def load_dataset():
    if not os.path.isfile(DATASET_JSON):
        print(f"✗ dataset not found: {DATASET_JSON}", file=sys.stderr)
        sys.exit(1)
    with open(DATASET_JSON, encoding="utf-8") as f:
        return json.load(f)


def dataset_fingerprint():
    h = hashlib.sha256()
    with open(DATASET_JSON, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def load_id_map():
    if not os.path.exists(ID_MAP):
        return {}
    current = dataset_fingerprint()
    if os.path.exists(ID_MAP_META):
        meta = json.load(open(ID_MAP_META, encoding="utf-8"))
        if meta.get("dataset_fingerprint") != current:
            print("  ⚠ id_map.json non corrisponde al JSON corrente: riparto da zero")
            return {}
    return json.load(open(ID_MAP, encoding="utf-8"))


def save_id_map(id_map):
    tmp = ID_MAP + ".tmp"
    json.dump(id_map, open(tmp, "w", encoding="utf-8"))
    os.replace(tmp, ID_MAP)
    meta_tmp = ID_MAP_META + ".tmp"
    json.dump({"dataset_fingerprint": dataset_fingerprint()}, open(meta_tmp, "w", encoding="utf-8"))
    os.replace(meta_tmp, ID_MAP_META)


def load_nodes(batch_size, limit, dataset):
    id_map = load_id_map()
    if id_map:
        print(f"  resume: {len(id_map)} nodi già caricati")

    pending = []
    for i, node in enumerate(dataset.get("nodes", [])):
        if limit and i >= limit:
            break
        key = node.get("key")
        if not key or key in id_map:
            continue
        pending.append(node)

    total = len(pending)
    print(f"  da caricare: {total} nodi (batch={batch_size})")
    done = 0
    errors = 0
    t0 = time.time()

    for start in range(0, total, batch_size):
        chunk = pending[start : start + batch_size]
        items = []
        for node in chunk:
            tags = node.get("tags") or []
            if isinstance(tags, str):
                tags = [t for t in tags.split("|") if t]
            items.append(
                {
                    "path": node["path"],
                    "content": node.get("content", ""),
                    "tags": tags,
                    "metadata": node.get("metadata") or {},
                    "embedding_model": "text-embedding-3-small",
                }
            )

        for attempt in range(1, 4):
            try:
                st, resp = http("POST", "/v1/memories/batch", {"items": items})
                if st not in (200, 201):
                    raise RuntimeError(f"HTTP {st}: {resp}")
                for res in resp.get("results", []):
                    idx = res.get("index", 0)
                    if res.get("id"):
                        id_map[chunk[idx]["key"]] = res["id"]
                    elif res.get("error"):
                        errors += 1
                break
            except Exception as e:
                if attempt == 3:
                    print(f"  ✗ batch @ {start} fallito: {e}")
                    errors += len(chunk)
                else:
                    time.sleep(2 * attempt)

        done += len(chunk)
        save_id_map(id_map)
        rate = done / max(time.time() - t0, 0.001)
        print(f"  nodi {done}/{total}  ({rate:.0f}/s)  errori={errors}", flush=True)

    print(f"  ✓ nodi caricati: {len(id_map)} totali, errori={errors}")
    return id_map


def collect_edges(id_map, dataset, limit=0):
    keys = set(id_map.keys())
    if limit:
        keys = {n["key"] for i, n in enumerate(dataset.get("nodes", [])) if i < limit and n.get("key") in id_map}

    edges = []
    skipped = 0
    for link in dataset.get("links", []):
        src_key = link.get("from")
        dst_key = link.get("to")
        if src_key not in keys or dst_key not in keys:
            skipped += 1
            continue
        src_id = id_map.get(src_key)
        dst_id = id_map.get(dst_key)
        if src_id is None or dst_id is None:
            skipped += 1
            continue
        link_type = (link.get("type") or "related").strip().lower()
        if link_type not in PUBLIC_LINK_TYPES:
            link_type = "related"
        meta = {}
        if link.get("rationale"):
            meta["rationale"] = link["rationale"]
        if link.get("weight") is not None:
            meta["weight"] = link["weight"]
        edges.append((f"memory.{src_id}", f"memory.{dst_id}", link_type, meta))
    return edges, skipped


def load_links(id_map, workers, limit, dataset):
    edges, skipped = collect_edges(id_map, dataset, limit)
    total = len(edges)
    print(f"  da creare: {total} archi (workers={workers}, skip={skipped})")
    done = {"n": 0}
    errors = {"n": 0}
    lock = threading.Lock()
    t0 = time.time()
    sample_errors: list[str] = []

    def post_edge(edge):
        from_path, to_path, link_type, meta = edge
        payload = {"from_path": from_path, "to_path": to_path, "link_type": link_type}
        if meta:
            payload["metadata"] = meta
        for attempt in range(1, LINK_MAX_ATTEMPTS + 1):
            try:
                st, resp = http("POST", "/v1/memories/links", payload, timeout=30)
                if st in (200, 201):
                    return True
                err = resp.get("error", resp)
                if st == 429:
                    time.sleep(min(30, 2 * attempt))
                    continue
                if "already exists" in str(err).lower() or "duplicate" in str(err).lower():
                    return True
                raise RuntimeError(f"HTTP {st}: {err}")
            except Exception as ex:
                if attempt == LINK_MAX_ATTEMPTS:
                    with lock:
                        if len(sample_errors) < 5:
                            sample_errors.append(str(ex))
                    return False
                time.sleep(min(30, 1.5 * attempt))
        return False

    with ThreadPoolExecutor(max_workers=workers) as pool:
        futs = [pool.submit(post_edge, e) for e in edges]
        for fut in as_completed(futs):
            ok = fut.result()
            with lock:
                done["n"] += 1
                if not ok:
                    errors["n"] += 1
                if done["n"] % 20 == 0 or done["n"] == total:
                    rate = done["n"] / max(time.time() - t0, 0.001)
                    print(f"  archi {done['n']}/{total}  ({rate:.0f}/s)  errori={errors['n']}", flush=True)

    print(f"  ✓ archi creati: {total - errors['n']}/{total}, errori={errors['n']}")
    if errors["n"] and sample_errors:
        print("  ⚠ errori di esempio:")
        for msg in sample_errors:
            print(f"    - {msg}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--batch", type=int, default=MAX_BATCH)
    ap.add_argument("--link-workers", type=int, default=DEFAULT_LINK_WORKERS)
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--nodes-only", action="store_true")
    ap.add_argument("--links-only", action="store_true")
    args = ap.parse_args()

    dataset = load_dataset()
    print(f"PCMI CTI loader → {BASE}")
    print(f"  dataset: {DATASET_JSON}")
    print(f"  nodes={len(dataset.get('nodes', []))} links={len(dataset.get('links', []))}")

    try:
        _, h = http("GET", "/v1/graph/health")
        print(f"  graph health: {h}", flush=True)
    except Exception as e:
        print(f"  ⚠ health check fallito: {e}")

    id_map: dict = {}
    if not args.links_only:
        print("\n[1/2] NODI")
        id_map = load_nodes(args.batch, args.limit, dataset)
    if not args.nodes_only:
        if not id_map:
            if not os.path.exists(ID_MAP):
                print("✗ id_map.json mancante: esegui prima i nodi.", file=sys.stderr)
                sys.exit(1)
            id_map = load_id_map()
        print("\n[2/2] LINK (grafo)")
        load_links(id_map, args.link_workers, args.limit, dataset)

    root_key = os.environ.get("CTI_ROOT_KEY", "soc_inc_001")
    root_mem = id_map.get(root_key)
    print("\n✅ done.")
    if root_mem:
        print(f"   Workbench root: mem={root_mem} ({root_key})")
        print(f"   {BASE}/v1/graph/workbench?mem={root_mem}")
    if id_map:
        print("   Sample IDs:", list(id_map.values())[:5])


if __name__ == "__main__":
    main()
