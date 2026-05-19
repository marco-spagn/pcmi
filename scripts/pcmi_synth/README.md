# PCMI synthetic data generator (`pcmi_synth`)

Generate deterministic (or LLM-authored) synthetic memories for **distillation E2E** and load tests. Replaces ad-hoc SOC-only scripts with parameterized **presets**, **seed**, and **record count**.

## Presets

| Preset | Path prefix | Mode |
|--------|-------------|------|
| `soc` | `root.security.incidents.soc` | Deterministic (legacy SOC generator) |
| `finance` | `root.finance.events` | Deterministic templates |
| `advertising` | `root.marketing.ads` | Deterministic templates |
| `healthcare` | `root.healthcare.ops` | Deterministic templates |
| `custom` | `root.custom.synthetic` | **Requires** `--llm` + `--domain "..."` |

List presets:

```bash
PYTHONPATH=scripts python -m pcmi_synth list
```

## Quick examples

```bash
# From repo root — 1000 deterministic SOC incidents (seed 42), JSONL only
PYTHONPATH=scripts python -m pcmi_synth generate \
  --preset soc --num 1000 --seed 42 --dry-run \
  --output .pcmi_test_out/soc_seed42_n1000.jsonl

# Finance — ingest into running API
export PCMI_API_KEY=testkey123
PYTHONPATH=scripts python -m pcmi_synth generate \
  --preset finance --num 200 --seed 1 \
  --output .pcmi_test_out/finance.jsonl

# Custom domain via LLM (needs OPENAI_API_KEY)
PYTHONPATH=scripts python -m pcmi_synth generate \
  --preset custom --domain "EU retail loyalty program anomalies" \
  --num 50 --seed 7 --llm --dry-run
```

Sharding aligns with worker `DISTILLATION_BATCH_SIZE` (default **10** records per shard → one refine event per shard).

## Full distillation E2E

Use the orchestrator (Docker + tenant + ingest + refine + assertions):

```bash
./scripts/distill_e2e.sh --preset finance --num 500 --seed 42
make distillation-e2e PRESET=advertising NUM=100 SEED=1
```

## Environment

| Variable | Purpose |
|----------|---------|
| `PCMI_API_KEY` | Ingest auth |
| `PCMI_BASE_URL` | API (default `http://localhost:8000`) |
| `OPENAI_API_KEY` | Required for `--llm` and for worker distillation |
| `DISTILLATION_BATCH_SIZE` | Shard size (sync with `--shard-size`) |

Legacy entry point `scripts/generate_soc_incidents_enterprise_v2.py` remains; new work should use `pcmi_synth`.
