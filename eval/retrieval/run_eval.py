#!/usr/bin/env python3
"""
PCMI · Retrieval quality evaluation harness
===========================================

Measures whether /v1/retrieve returns the *right* memories (quality), not how
fast (that is the SLO/k6 job). Self-contained and reproducible: it seeds its own
corpus under a dedicated path_prefix, runs a gold query set, and computes
standard IR metrics with a CI gate.

Metrics (per query, then macro-averaged):
  - recall@k      relevant retrieved in top-k / total relevant
  - precision@k   relevant retrieved in top-k / k
  - hit@k         1 if any relevant in top-k else 0
  - MRR           1 / rank of first relevant (0 if none)
  - nDCG@k        graded gain, ideal-normalized (uses `graded` if present, else 1)

Usage:
  # against a running PCMI (make infra-up / go run ./cmd/api + worker):
  python3 eval/retrieval/run_eval.py --seed --wait-embeddings 120
  python3 eval/retrieval/run_eval.py --gold eval/retrieval/gold/seed_basic.jsonl

  # validate gold/corpus format without a server:
  python3 eval/retrieval/run_eval.py --dry-run

  # CI gate (non-zero exit if below thresholds):
  make eval-retrieval

Env:
  PCMI_BASE_URL   default http://localhost:8000
  PCMI_API_KEY    default testkey123
"""
from __future__ import annotations

import argparse
import json
import math
import os
import sys
import time
import urllib.error
import urllib.request
from typing import Any

ROOT = os.path.dirname(os.path.abspath(__file__))
BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")

DEFAULT_CORPUS = os.path.join(ROOT, "corpus", "eval_seed.jsonl")
DEFAULT_GOLD = os.path.join(ROOT, "gold", "seed_basic.jsonl")
DEFAULT_THRESHOLDS = os.path.join(ROOT, "thresholds.json")


# ─── HTTP ────────────────────────────────────────────────────────────────────
def _req(method: str, path: str, body: dict | None = None, timeout: int = 60) -> Any:
    data = json.dumps(body).encode() if body is not None else None
    # A benchmark bursts requests; tolerate rate limits (429) and transient 5xx
    # with bounded exponential backoff so results don't depend on server tuning.
    attempts = 6
    for i in range(attempts):
        req = urllib.request.Request(
            f"{BASE}{path}",
            data=data,
            method=method,
            headers={"Content-Type": "application/json", "X-API-Key": KEY},
        )
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return json.load(resp)
        except urllib.error.HTTPError as e:
            transient = e.code == 429 or 500 <= e.code < 600
            if not transient or i == attempts - 1:
                raise
            retry_after = e.headers.get("Retry-After") if e.headers else None
            delay = float(retry_after) if retry_after and retry_after.isdigit() else min(2 ** i, 15)
            time.sleep(delay)
    raise RuntimeError("unreachable")


def ready() -> bool:
    try:
        r = _req("GET", "/v1/ready", timeout=5)
        return bool(r.get("status") == "ready")
    except Exception:
        return False


# ─── data loading ────────────────────────────────────────────────────────────
def load_jsonl(path: str) -> list[dict]:
    rows: list[dict] = []
    with open(path, encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError as e:
                raise SystemExit(f"{path}:{lineno}: invalid JSON — {e}")
    return rows


def validate_gold(gold: list[dict], corpus_paths: set[str] | None) -> list[str]:
    """Return a list of human-readable problems (empty = valid)."""
    problems: list[str] = []
    seen_ids: set[str] = set()
    for i, q in enumerate(gold):
        qid = q.get("id") or f"#{i}"
        if not q.get("query"):
            problems.append(f"{qid}: missing 'query'")
        rel = q.get("relevant") or list((q.get("graded") or {}).keys())
        if not rel:
            problems.append(f"{qid}: no 'relevant' paths and no 'graded' map")
        if q.get("id") in seen_ids:
            problems.append(f"{qid}: duplicate id")
        seen_ids.add(q.get("id"))
        if corpus_paths is not None:
            for p in rel:
                if p not in corpus_paths:
                    problems.append(f"{qid}: relevant path not in corpus → {p}")
    return problems


# ─── seeding ─────────────────────────────────────────────────────────────────
def seed_corpus(corpus: list[dict]) -> int:
    stored = 0
    for m in corpus:
        payload = {
            "path": m["path"],
            "content": m["content"],
            "tags": m.get("tags", []),
            "metadata": m.get("metadata", {}),
        }
        if "importance" in m:
            payload["importance"] = m["importance"]
        try:
            _req("POST", "/v1/memories", payload)
            stored += 1
        except urllib.error.HTTPError as e:
            print(f"  ! seed failed for {m['path']}: HTTP {e.code}", file=sys.stderr)
    return stored


def embedding_coverage(path_prefix: str) -> tuple[int, int]:
    """(embedded, total) for the eval prefix. Empty-query retrieve returns each
    entry with its `embedding` vector, so we can detect when the worker has
    actually finished embedding — a plain row count would report false-ready and
    let the eval run against PCMI's hard lexical AND-filter (0 rows for synonym
    queries)."""
    try:
        r = _req("POST", "/v1/retrieve", {"path_prefix": path_prefix, "query": "", "limit": 500})
        entries = r.get("entries") or []
        total = len(entries)
        embedded = sum(1 for e in entries if e.get("embedding"))
        return embedded, total
    except Exception:
        return 0, 0


def wait_embeddings(path_prefix: str, want: int, timeout_s: int) -> None:
    if timeout_s <= 0:
        return
    deadline = time.time() + timeout_s
    while time.time() < deadline:
        have, _ = embedding_coverage(path_prefix)
        if have >= want:
            print(f"  embeddings: {have}/{want} ready")
            return
        time.sleep(3)
    print(f"  ! embedding wait timed out ({timeout_s}s) — proceeding (BM25 fallback possible)")


# ─── retrieval + metrics ─────────────────────────────────────────────────────
def retrieve(query: str, path_prefix: str, limit: int, tags: list[str] | None) -> list[dict]:
    body: dict[str, Any] = {"path_prefix": path_prefix, "query": query, "limit": limit}
    if tags:
        body["tags"] = tags
    r = _req("POST", "/v1/retrieve", body)
    return r.get("entries") or []


def dcg(gains: list[float]) -> float:
    return sum(g / math.log2(i + 2) for i, g in enumerate(gains))


def eval_query(q: dict, k: int, default_prefix: str) -> dict:
    prefix = q.get("path_prefix", default_prefix)
    limit = max(k, int(q.get("limit", k)))
    entries = retrieve(q["query"], prefix, limit, q.get("tags"))
    ret_paths = [e.get("path") for e in entries]

    graded: dict[str, float] = dict(q.get("graded") or {})
    relevant: set[str] = set(q.get("relevant") or graded.keys())
    if not graded:
        graded = {p: 1.0 for p in relevant}

    topk = ret_paths[:k]
    hit_set = relevant.intersection(topk)

    recall = len(hit_set) / len(relevant) if relevant else 0.0
    precision = len(hit_set) / k if k else 0.0
    hit = 1.0 if hit_set else 0.0

    mrr = 0.0
    for rank, p in enumerate(ret_paths, 1):
        if p in relevant:
            mrr = 1.0 / rank
            break

    gains = [graded.get(p, 0.0) for p in topk]
    ideal = sorted(graded.values(), reverse=True)[:k]
    ndcg = dcg(gains) / dcg(ideal) if dcg(ideal) > 0 else 0.0

    return {
        "id": q.get("id"),
        "query": q["query"],
        "recall@k": round(recall, 4),
        "precision@k": round(precision, 4),
        "hit@k": hit,
        "mrr": round(mrr, 4),
        "ndcg@k": round(ndcg, 4),
        "retrieved_top": topk,
        "relevant": sorted(relevant),
    }


def macro(rows: list[dict], field: str) -> float:
    return round(sum(r[field] for r in rows) / len(rows), 4) if rows else 0.0


# ─── main ────────────────────────────────────────────────────────────────────
def main() -> int:
    ap = argparse.ArgumentParser(description="PCMI retrieval quality eval")
    ap.add_argument("--corpus", default=DEFAULT_CORPUS)
    ap.add_argument("--gold", default=DEFAULT_GOLD)
    ap.add_argument("--thresholds", default=DEFAULT_THRESHOLDS)
    ap.add_argument("--k", type=int, default=5, help="cutoff for @k metrics")
    ap.add_argument("--seed", action="store_true", help="store corpus before evaluating")
    ap.add_argument("--wait-embeddings", type=int, default=0, metavar="SECS")
    ap.add_argument("--dry-run", action="store_true", help="validate files, no server calls")
    ap.add_argument("--report", default="", help="write JSON report to this path")
    ap.add_argument("--no-gate", action="store_true", help="do not fail on thresholds")
    args = ap.parse_args()

    corpus = load_jsonl(args.corpus)
    gold = load_jsonl(args.gold)
    corpus_paths = {m["path"] for m in corpus}
    default_prefix = os.environ.get("EVAL_PATH_PREFIX", "root.eval")

    # dry-run: format validation only
    if args.dry_run:
        problems = validate_gold(gold, corpus_paths)
        print(f"corpus: {len(corpus)} memories · gold: {len(gold)} queries")
        if problems:
            print("VALIDATION FAILED:")
            for p in problems:
                print(f"  ✗ {p}")
            return 1
        print("✓ corpus + gold are well-formed and cross-referenced")
        return 0

    if not ready():
        print(f"✗ PCMI not ready at {BASE} — start it (make infra-up) or set PCMI_BASE_URL", file=sys.stderr)
        return 2

    if args.seed:
        print(f"seeding {len(corpus)} memories under {default_prefix}.* …")
        n = seed_corpus(corpus)
        print(f"  stored {n}/{len(corpus)}")
        wait_embeddings(default_prefix, len(corpus), args.wait_embeddings)

    print(f"\nRetrieval eval → {BASE}  (k={args.k}, {len(gold)} queries)\n" + "=" * 72)
    rows = [eval_query(q, args.k, default_prefix) for q in gold]

    hdr = f"{'query id':<22} {'recall':>7} {'prec':>6} {'hit':>4} {'mrr':>6} {'ndcg':>6}"
    print(hdr)
    print("-" * len(hdr))
    for r in rows:
        flag = "" if r["hit@k"] else "  ← MISS"
        print(f"{str(r['id']):<22} {r['recall@k']:>7} {r['precision@k']:>6} "
              f"{int(r['hit@k']):>4} {r['mrr']:>6} {r['ndcg@k']:>6}{flag}")

    agg = {
        "queries": len(rows),
        "k": args.k,
        f"recall@{args.k}": macro(rows, "recall@k"),
        f"precision@{args.k}": macro(rows, "precision@k"),
        f"hit@{args.k}": macro(rows, "hit@k"),
        "mrr": macro(rows, "mrr"),
        f"ndcg@{args.k}": macro(rows, "ndcg@k"),
    }
    print("=" * 72)
    print("MACRO-AVERAGE: " + "  ".join(f"{k}={v}" for k, v in agg.items() if k not in ("queries", "k")))

    if args.report:
        with open(args.report, "w", encoding="utf-8") as f:
            json.dump({"aggregate": agg, "per_query": rows}, f, indent=2)
        print(f"report → {args.report}")

    # CI gate
    if args.no_gate:
        return 0
    try:
        thresholds = json.load(open(args.thresholds, encoding="utf-8"))
    except FileNotFoundError:
        print("(no thresholds file — skipping gate)")
        return 0

    failures = []
    for metric, floor in thresholds.items():
        got = agg.get(metric)
        if got is None:
            continue
        if got < floor:
            failures.append(f"{metric}={got} < floor {floor}")
    if failures:
        print("\n✗ GATE FAILED:")
        for fmsg in failures:
            print(f"  {fmsg}")
        return 1
    print("\n✓ gate passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
