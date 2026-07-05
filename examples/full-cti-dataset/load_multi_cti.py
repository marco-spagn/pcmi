#!/usr/bin/env python3
"""
PCMI · Multi-source CTI loader
==============================
Loads three test datasets into PCMI with cti_source metadata for UI filtering:

  1. SOC + TI Hub  — full_cti_dataset.json (root.cti.soc.* + root.cti.ti_hub.*)
  2. Vendor reports — vendor_reports_cti_dataset.json
  3. Operational STIX — operational_stix_cti_dataset.json (CISA MAR BRICKSTORM + vendor IOCs)

Cross-links from full_cti (soc → vendor_reports keys) are applied when both endpoints exist.

Env:
  CTI_DATASET_JSON          default examples/full-cti-dataset/data/full_cti_dataset.json
  CTI_VENDOR_REPORTS_JSON   default examples/full-cti-dataset/data/vendor_reports_cti_dataset.json
  CTI_STIX_BUNDLE_JSON      default examples/full-cti-dataset/data/operational_stix_cti_dataset.json
  CTI_ROOT_KEY              default soc_inc_001
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
DATA_DIR = os.path.join(ROOT, "data")
sys.path.insert(0, ROOT)

from load_to_pcmi import (  # noqa: E402
    DEFAULT_LINK_WORKERS,
    ID_MAP,
    ID_MAP_META,
    LINK_MAX_ATTEMPTS,
    MAX_BATCH,
    PUBLIC_LINK_TYPES,
    http,
)
from stix_to_pcmi import load_stix_file  # noqa: E402

BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")


def _default_dataset(filename: str) -> str:
    return os.path.join(DATA_DIR, filename)


FULL_CTI_JSON = os.environ.get("CTI_DATASET_JSON") or _default_dataset("full_cti_dataset.json")
VENDOR_JSON = os.environ.get("CTI_VENDOR_REPORTS_JSON") or _default_dataset("vendor_reports_cti_dataset.json")
OPERATIONAL_STIX_JSON = _default_dataset("operational_stix_cti_dataset.json")
STIX_JSON = os.environ.get("CTI_STIX_BUNDLE_JSON") or OPERATIONAL_STIX_JSON
if not os.path.isfile(STIX_JSON):
    legacy = os.environ.get("CTI_STIX_BUNDLE_LEGACY") or _default_dataset(
        "historical_stix_threat_intel_2020_2026_bundle.json"
    )
    if os.path.isfile(legacy):
        print(f"  ⚠ operational STIX missing ({STIX_JSON}) — fallback {legacy}", file=sys.stderr)
        STIX_JSON = legacy
    else:
        print(f"  ⚠ operational STIX missing: {STIX_JSON}", file=sys.stderr)

SOURCES = ("soc", "vendor_reports", "stix")


def _read_json(path: str) -> dict:
    if not os.path.isfile(path):
        print(f"  ⚠ skip missing: {path}", file=sys.stderr)
        return {"nodes": [], "links": []}
    try:
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    except (PermissionError, OSError) as e:
        print(f"  ⚠ cannot read {path}: {e}", file=sys.stderr)
        return {"nodes": [], "links": []}


def _load_stix_safe(path: str) -> dict:
    if not os.path.isfile(path):
        print(f"  ⚠ skip missing STIX bundle: {path}", file=sys.stderr)
        return {"nodes": [], "links": []}
    try:
        return load_stix_file(path)
    except (PermissionError, OSError) as e:
        print(f"  ⚠ cannot read STIX bundle {path}: {e}", file=sys.stderr)
        return {"nodes": [], "links": []}


def _tag_node(node: dict, source: str) -> dict:
    n = dict(node)
    meta = dict(n.get("metadata") or {})
    meta["cti_source"] = source
    if source == "soc":
        if ".soc." in n.get("path", ""):
            meta.setdefault("layer", "soc")
        elif ".ti_hub." in n.get("path", ""):
            meta.setdefault("layer", "ti_hub")
    elif source == "vendor_reports":
        meta.setdefault("layer", "vendors")
    n["metadata"] = meta
    return n


def _tag_link(link: dict, source: str) -> dict:
    l = dict(link)
    meta = dict(l.get("metadata") or {})
    meta["cti_source"] = source
    l["metadata"] = meta
    return l


def build_combined_dataset() -> tuple[dict, dict]:
    """Merge nodes/links from all sources. Returns (dataset, source_stats)."""
    stats = {s: {"nodes": 0, "links": 0} for s in SOURCES}

    full = _read_json(FULL_CTI_JSON)
    soc_nodes = [
        _tag_node(n, "soc")
        for n in full.get("nodes", [])
        if ".soc." in n.get("path", "") or ".ti_hub." in n.get("path", "")
    ]
    soc_keys = {n["key"] for n in soc_nodes if n.get("key")}
    soc_links = [
        _tag_link(l, "soc")
        for l in full.get("links", [])
        if l.get("from") in soc_keys and l.get("to") in soc_keys
    ]
    stats["soc"]["nodes"] = len(soc_nodes)
    stats["soc"]["links"] = len(soc_links)

    vendor = _read_json(VENDOR_JSON)
    vr_nodes = [_tag_node(n, "vendor_reports") for n in vendor.get("nodes", []) if n.get("key")]
    vr_keys = {n["key"] for n in vr_nodes}
    vr_links = [
        _tag_link(l, "vendor_reports")
        for l in vendor.get("links", [])
        if l.get("from") in vr_keys and l.get("to") in vr_keys
    ]
    stats["vendor_reports"]["nodes"] = len(vr_nodes)
    stats["vendor_reports"]["links"] = len(vr_links)

    stix = _load_stix_safe(STIX_JSON)
    stix_nodes = []
    for n in stix.get("nodes") or []:
        tagged = dict(n)
        meta = dict(tagged.get("metadata") or {})
        src = meta.get("cti_source") or "stix"
        if src not in ("stix", "vendor_intel"):
            src = "stix"
        meta["cti_source"] = src
        meta.setdefault("layer", "vendors")
        tagged["metadata"] = meta
        stix_nodes.append(tagged)
    stix_keys = {n["key"] for n in stix_nodes if n.get("key")}
    stix_links = [
        _tag_link(l, "stix")
        for l in stix.get("links") or []
        if l.get("from") in stix_keys and l.get("to") in stix_keys
    ]
    stats["stix"]["nodes"] = len(stix_nodes)
    stats["stix"]["links"] = len(stix_links)

    # Cross-layer links from full_cti (soc/ti_hub ↔ vendor_reports)
    cross_links: list[dict] = []
    all_vendor_keys = vr_keys
    for l in full.get("links", []):
        fk, tk = l.get("from"), l.get("to")
        if not fk or not tk:
            continue
        in_soc = fk in soc_keys or tk in soc_keys
        in_vr = fk in all_vendor_keys or tk in all_vendor_keys
        if in_soc and in_vr and fk in (soc_keys | all_vendor_keys) and tk in (soc_keys | all_vendor_keys):
            cross_links.append(_tag_link(l, "soc"))

    nodes = soc_nodes + vr_nodes + stix_nodes
    links = soc_links + vr_links + stix_links + cross_links
    stats["cross_links"] = len(cross_links)
    stats["operational_stix_json"] = STIX_JSON

    keys_seen: set[str] = set()
    dedup_nodes = []
    for n in nodes:
        k = n.get("key")
        if not k or k in keys_seen:
            continue
        keys_seen.add(k)
        dedup_nodes.append(n)

    return {"nodes": dedup_nodes, "links": links}, stats


def dataset_fingerprint(dataset: dict) -> str:
    h = hashlib.sha256()
    h.update(json.dumps(dataset, sort_keys=True, ensure_ascii=False).encode())
    return h.hexdigest()


def load_id_map(fp: str) -> dict:
    if not os.path.exists(ID_MAP):
        return {}
    if os.path.exists(ID_MAP_META):
        meta = json.load(open(ID_MAP_META, encoding="utf-8"))
        if meta.get("dataset_fingerprint") != fp:
            print("  ⚠ id_map non corrisponde al dataset combinato: riparto da zero")
            return {}
    return json.load(open(ID_MAP, encoding="utf-8"))


def save_id_map(id_map: dict, fp: str, stats: dict) -> None:
    tmp = ID_MAP + ".tmp"
    json.dump(id_map, open(tmp, "w", encoding="utf-8"))
    os.replace(tmp, ID_MAP)
    meta_tmp = ID_MAP_META + ".tmp"
    json.dump(
        {
            "dataset_fingerprint": fp,
            "sources": stats,
            "files": {
                "full_cti": FULL_CTI_JSON,
                "vendor_reports": VENDOR_JSON,
                "stix": STIX_JSON,
            },
        },
        open(meta_tmp, "w", encoding="utf-8"),
    )
    os.replace(meta_tmp, ID_MAP_META)


def load_nodes(batch_size: int, limit: int, dataset: dict, fp: str, stats: dict) -> dict:
    id_map = load_id_map(fp)
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
        save_id_map(id_map, fp, stats)
        rate = done / max(time.time() - t0, 0.001)
        print(f"  nodi {done}/{total}  ({rate:.0f}/s)  errori={errors}", flush=True)

    print(f"  ✓ nodi caricati: {len(id_map)} totali, errori={errors}")
    return id_map


def collect_edges(id_map: dict, dataset: dict, limit: int = 0) -> tuple[list, int]:
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
        meta = dict(link.get("metadata") or {})
        if link.get("rationale"):
            meta["rationale"] = link["rationale"]
        if link.get("weight") is not None:
            meta["weight"] = link["weight"]
        edges.append((f"memory.{src_id}", f"memory.{dst_id}", link_type, meta))
    return edges, skipped


def load_links(id_map: dict, workers: int, limit: int, dataset: dict) -> None:
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
                if done["n"] % 25 == 0 or done["n"] == total:
                    rate = done["n"] / max(time.time() - t0, 0.001)
                    print(f"  archi {done['n']}/{total}  ({rate:.0f}/s)  errori={errors['n']}", flush=True)

    print(f"  ✓ archi creati: {total - errors['n']}/{total}, errori={errors['n']}")
    if errors["n"] and sample_errors:
        print("  ⚠ errori di esempio:")
        for msg in sample_errors:
            print(f"    - {msg}")


def rebuild_id_map_from_api(dataset: dict) -> dict:
    """Map dataset keys → memory IDs using /v1/retrieve path index (no new inserts)."""
    index: dict[str, int] = {}
    cursor = ""
    for _ in range(24):
        body: dict = {"path_prefix": "root.cti", "limit": 500}
        if cursor:
            body["cursor"] = cursor
        st, resp = http("POST", "/v1/retrieve", body)
        if st not in (200, 201):
            raise RuntimeError(f"retrieve HTTP {st}: {resp}")
        for entry in resp.get("entries") or []:
            path = entry.get("path") or ""
            mid = entry.get("id")
            if path and mid is not None:
                index[path] = int(mid)
        cursor = resp.get("next_cursor") or ""
        if not cursor:
            break
    id_map: dict = {}
    missing: list[str] = []
    for node in dataset.get("nodes", []):
        key = node.get("key")
        path = node.get("path")
        if not key or not path:
            continue
        mid = index.get(path)
        if mid is not None:
            id_map[key] = mid
        else:
            missing.append(key)
    print(f"  id_map da API: {len(id_map)} chiavi ({len(missing)} path mancanti)")
    if missing[:5]:
        print(f"  ⚠ esempio path mancanti: {', '.join(missing[:5])}")
    return id_map


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--batch", type=int, default=MAX_BATCH)
    ap.add_argument("--link-workers", type=int, default=DEFAULT_LINK_WORKERS)
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--nodes-only", action="store_true")
    ap.add_argument("--links-only", action="store_true")
    ap.add_argument("--reset", action="store_true", help="Ignore id_map checkpoint")
    ap.add_argument(
        "--rebuild-map",
        action="store_true",
        help="Rebuild id_map.json from live API paths (use with --links-only)",
    )
    args = ap.parse_args()

    dataset, stats = build_combined_dataset()
    fp = dataset_fingerprint(dataset)

    if args.reset and os.path.exists(ID_MAP):
        os.remove(ID_MAP)
        if os.path.exists(ID_MAP_META):
            os.remove(ID_MAP_META)

    print(f"PCMI multi-CTI loader → {BASE}")
    print(f"  full_cti:        {FULL_CTI_JSON}")
    print(f"  vendor_reports:  {VENDOR_JSON}")
    print(f"  stix:            {STIX_JSON}")
    print(
        f"  combined: nodes={len(dataset['nodes'])} links={len(dataset['links'])} "
        f"(soc={stats['soc']['nodes']}, vr={stats['vendor_reports']['nodes']}, "
        f"stix={stats['stix']['nodes']}, cross={stats.get('cross_links', 0)})"
    )

    try:
        _, h = http("GET", "/v1/graph/health")
        print(f"  graph health: {h}", flush=True)
    except Exception as e:
        print(f"  ⚠ health check fallito: {e}")

    id_map: dict = {}
    if not args.links_only:
        print("\n[1/2] NODI")
        id_map = load_nodes(args.batch, args.limit, dataset, fp, stats)
    if not args.nodes_only:
        if args.rebuild_map or (args.links_only and not os.path.exists(ID_MAP)):
            print("\n[map] REBUILD id_map da API")
            id_map = rebuild_id_map_from_api(dataset)
            save_id_map(id_map, fp, stats)
        elif not id_map:
            if not os.path.exists(ID_MAP):
                print("✗ id_map.json mancante: esegui prima i nodi o --rebuild-map.", file=sys.stderr)
                return 1
            id_map = load_id_map(fp)
        print("\n[2/2] LINK (grafo)")
        load_links(id_map, args.link_workers, args.limit, dataset)

    root_key = os.environ.get("CTI_ROOT_KEY", "soc_inc_001")
    root_mem = id_map.get(root_key)
    print("\n✅ done.")
    if root_mem:
        print(f"   Workbench root: mem={root_mem} ({root_key})")
        print(f"   {BASE}/v1/graph/workbench?mem={root_mem}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
