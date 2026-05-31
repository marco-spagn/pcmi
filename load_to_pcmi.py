#!/usr/bin/env python3
"""
PCMI · SOC Dataset Loader  (nodes + cognitive graph links)
==========================================================
Carica soc_incidents_nodes.csv + soc_incidents_links.csv in PCMI.

Flusso (rispetta il contratto del grafo PCMI/AGE):
  1) NODI  → POST /v1/memories/batch  (items[]), cattura l'id assegnato
            costruisce mappa  external_id -> db_id  (checkpoint su disco → resumable)
  2) LINK  → POST /v1/memories/links  con from_path/to_path = "memory.<db_id>"
            (OBBLIGATORIO: il trigger AGE usa memory.<id> come identita' vertice,
             e FindRelated/FindChain fanno il parse dell'intero da quel path)

Uso:
  export PCMI_BASE_URL=http://localhost:8000
  export PCMI_API_KEY=testkey123
  python3 load_to_pcmi.py                 # carica tutto
  python3 load_to_pcmi.py --nodes-only
  python3 load_to_pcmi.py --links-only    # richiede id_map.json gia' presente
  python3 load_to_pcmi.py --batch 50 --link-workers 16
  python3 load_to_pcmi.py --limit 200     # smoke test parziale

Solo stdlib (urllib + threads). Nessuna dipendenza esterna.
"""
import argparse, csv, json, os, sys, time, threading
import urllib.request, urllib.error
from concurrent.futures import ThreadPoolExecutor, as_completed

BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY  = os.environ.get("PCMI_API_KEY", "testkey123")
NODES_CSV = "soc_incidents_nodes.csv"
LINKS_CSV = "soc_incidents_links.csv"
ID_MAP    = "id_map.json"
MAX_BATCH = 50
EMB_MODEL = os.environ.get("PCMI_EMBEDDING_MODEL", "")

CORE = {"external_id", "path", "content", "tags"}
NUMERIC = {"cvss_score": float, "ttd_minutes": int, "ttr_minutes": int, "alert_count": int}
BOOLS = {"sla_met"}

def http(method, path, payload=None, timeout=60):
    url = BASE + path
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    req.add_header("X-API-Key", KEY)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        body = r.read().decode()
        return r.status, (json.loads(body) if body else {})

def build_metadata(row):
    md = {}
    for c, v in row.items():
        if c in CORE or v == "" or v is None:
            continue
        if c in NUMERIC:
            try: v = NUMERIC[c](v)
            except ValueError: pass
        elif c in BOOLS:
            v = (str(v).lower() == "true")
        md[c] = v
    if row.get("mitre_technique"):
        md["mitre_url"] = f"https://attack.mitre.org/techniques/{row['mitre_technique'].replace('.','/')}/"
    return md

def load_nodes(batch_size, limit):
    id_map = {}
    if os.path.exists(ID_MAP):
        id_map = json.load(open(ID_MAP))
        print(f"  resume: {len(id_map)} nodi gia' caricati")

    rows = []
    with open(NODES_CSV, encoding="utf-8") as f:
        for i, row in enumerate(csv.DictReader(f)):
            if limit and i >= limit: break
            if row["external_id"] in id_map: continue
            rows.append(row)

    total = len(rows)
    print(f"  da caricare: {total} nodi (batch={batch_size})")
    done = 0; errors = 0; t0 = time.time()

    for start in range(0, total, batch_size):
        chunk = rows[start:start+batch_size]
        items = []
        for r in chunk:
            item = {
                "path": r["path"],
                "content": r["content"],
                "tags": [t for t in r["tags"].split("|") if t],
                "metadata": build_metadata(r),
            }
            if EMB_MODEL:
                item["embedding_model"] = EMB_MODEL
            items.append(item)

        for attempt in range(1, 4):
            try:
                st, resp = http("POST", "/v1/memories/batch", {"items": items})
                if st not in (200, 201):
                    raise RuntimeError(f"HTTP {st}: {resp}")
                for res in resp.get("results", []):
                    idx = res.get("index", 0)
                    if res.get("id"):
                        id_map[chunk[idx]["external_id"]] = res["id"]
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
        if start % (batch_size*10) == 0 or done >= total:
            json.dump(id_map, open(ID_MAP, "w"))
            rate = done / max(time.time()-t0, .001)
            print(f"  nodi {done}/{total}  ({rate:.0f}/s)  errori={errors}", flush=True)

    json.dump(id_map, open(ID_MAP, "w"))
    print(f"  ✓ nodi caricati: {len(id_map)} totali, errori={errors}")
    return id_map

def load_links(id_map, workers, limit):
    edges = []
    skipped = 0
    with open(LINKS_CSV, encoding="utf-8") as f:
        for i, row in enumerate(csv.DictReader(f)):
            if limit and i >= limit: break
            a = id_map.get(row["from_external_id"]); b = id_map.get(row["to_external_id"])
            if a is None or b is None:
                skipped += 1; continue
            edges.append((f"memory.{a}", f"memory.{b}", row["link_type"], row.get("rationale","")))

    total = len(edges)
    print(f"  da creare: {total} archi (workers={workers}, skip(nomap)={skipped})")
    done = {"n": 0}; errors = {"n": 0}; lock = threading.Lock(); t0 = time.time()

    def post_edge(e):
        fp, tp, lt, why = e
        payload = {"from_path": fp, "to_path": tp, "link_type": lt}
        if why: payload["metadata"] = {"rationale": why}
        for attempt in range(1, 4):
            try:
                st, _ = http("POST", "/v1/memories/links", payload, timeout=30)
                if st in (200, 201): return True
                raise RuntimeError(f"HTTP {st}")
            except Exception:
                if attempt == 3: return False
                time.sleep(1.5 * attempt)

    with ThreadPoolExecutor(max_workers=workers) as ex:
        futs = [ex.submit(post_edge, e) for e in edges]
        for fut in as_completed(futs):
            ok = fut.result()
            with lock:
                done["n"] += 1
                if not ok: errors["n"] += 1
                if done["n"] % 500 == 0 or done["n"] == total:
                    rate = done["n"] / max(time.time()-t0, .001)
                    print(f"  archi {done['n']}/{total}  ({rate:.0f}/s)  errori={errors['n']}", flush=True)

    print(f"  ✓ archi creati: {total-errors['n']}/{total}, errori={errors['n']}")

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--batch", type=int, default=MAX_BATCH,
                    help=f"batch size (max {MAX_BATCH} per PCMI API)")
    ap.add_argument("--link-workers", type=int, default=16)
    ap.add_argument("--limit", type=int, default=0, help="limita righe (smoke test)")
    ap.add_argument("--nodes-only", action="store_true")
    ap.add_argument("--links-only", action="store_true")
    args = ap.parse_args()
    if args.batch < 1 or args.batch > MAX_BATCH:
        print(f"✗ --batch must be 1–{MAX_BATCH} (API limit)"); sys.exit(1)

    print(f"PCMI loader → {BASE}")
    try:
        st, h = http("GET", "/v1/graph/health")
        print(f"  graph health: {h}")
    except Exception as e:
        print(f"  ⚠ health check fallito: {e}")

    id_map = {}
    if not args.links_only:
        print("\n[1/2] NODI")
        id_map = load_nodes(args.batch, args.limit)
    if not args.nodes_only:
        if not id_map:
            if not os.path.exists(ID_MAP):
                print("✗ id_map.json mancante: esegui prima i nodi."); sys.exit(1)
            id_map = json.load(open(ID_MAP))
        print("\n[2/2] LINK (grafo)")
        load_links(id_map, args.link_workers, args.limit)

    print("\n✅ done. Apri:  " + BASE + "/v1/graph/ui")
    if id_map:
        sample = list(id_map.values())[:5]
        print("   Memory ID di esempio da esplorare nella UI:", sample)

if __name__ == "__main__":
    main()
