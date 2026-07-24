# PCMI Retrieval Quality Evaluation

Measures whether `/v1/retrieve` returns the **right** memories — recall\@k, precision\@k,
MRR, nDCG\@k — not how fast it does (that is [`docs/SLO.md`](../../docs/SLO.md) + the k6
load test). This closes audit gap **P0 #3**: without it, retrieval quality is
neither tunable nor provable against Mem0/Zep.

## Why this exists

Latency benchmarks tell you the pipeline is fast. They say nothing about whether a
query for *"what made our API respond faster"* actually surfaces the memory that
says *"latency dropped from 340ms to 12ms"* — different words, same meaning. That is
a **quality** question, and quality is what a memory layer lives or dies on.

## What it does

1. **Seeds** a small, self-contained corpus under `root.eval.*` (`corpus/eval_seed.jsonl`)
   so the eval is reproducible without the CTI demo dataset.
2. **Runs** a gold query set (`gold/seed_basic.jsonl`) whose queries deliberately use
   *synonyms* of the memory wording, so BM25-only retrieval misses several — the gap
   the semantic (pgvector) half must close.
3. **Scores** each query and macro-averages recall\@k, precision\@k, hit\@k, MRR, nDCG\@k.
4. **Gates** on `thresholds.json` (non-zero exit if below), ready for CI.

## Run it

```bash
# 1. bring up the base stack (postgres + redis + api + worker) with an embedding key
export OPENAI_API_KEY=sk-...          # required for the semantic half
make infra-up

# 2. seed + evaluate (waits up to 120s for the embedding worker)
make eval-retrieval

# offline format check (no server needed):
make eval-retrieval-validate
```

Direct invocation gives finer control:

```bash
python3 eval/retrieval/run_eval.py --seed --wait-embeddings 120 --k 5
python3 eval/retrieval/run_eval.py --gold eval/retrieval/gold/seed_basic.jsonl --report out.json
python3 eval/retrieval/run_eval.py --dry-run           # validate files only
python3 eval/retrieval/run_eval.py --no-gate           # report without CI gate
```

## Files

| File | Purpose |
|------|---------|
| `run_eval.py` | Harness (Python stdlib only — no deps) |
| `corpus/eval_seed.jsonl` | Self-contained memories to seed (domain-neutral) |
| `gold/seed_basic.jsonl` | Queries + relevant paths (+ optional graded gains) |
| `thresholds.json` | CI gate floors (macro-average) |

## Gold format

```json
{"id": "q_sso",
 "query": "our decision about single sign-on identity provider",
 "graded": {"root.eval.decision.auth_provider": 3},
 "relevant": ["root.eval.decision.auth_provider"]}
```

- `relevant` — paths that count as correct hits (binary).
- `graded` — optional path→gain map for nDCG (`3` = primary, `1` = partial). If
  omitted, every relevant path gets gain 1.
- `path_prefix`, `limit`, `tags` — optional per-query overrides.

## Metrics

| Metric | Definition |
|--------|-----------|
| **recall\@k** | relevant found in top-k / total relevant |
| **precision\@k** | relevant found in top-k / k |
| **hit\@k** | 1 if any relevant in top-k, else 0 |
| **MRR** | 1 / rank of first relevant (0 if none) |
| **nDCG\@k** | graded gain, ideal-normalized |

## Important: the gate needs embeddings

The gold queries are written with synonym gaps on purpose. **Without `OPENAI_API_KEY`**
the API falls back to BM25-only (lexical) and several semantic queries will miss —
the gate is *expected* to fail. That is the point: it proves the semantic half is
doing real work. Run the gate only with embeddings enabled; use `--no-gate` for a
lexical-only smoke.

## Extending

- **New domain**: add a corpus JSONL + a gold JSONL, point `--corpus`/`--gold` at them.
- **CTI corpus**: once `make demo_cti` has loaded `root.cti.*`, write a gold file with
  the cross-vendor facts (e.g. a query in CrowdStrike vocabulary whose relevant answer
  is the Microsoft memory) and run with `--gold gold/cti_cross_vendor.jsonl --no-gate`.
- **Reranking A/B**: once an LLM reranker lands (gap #5), run the same gold set before
  and after — the delta in nDCG is the value of the reranker, quantified.
