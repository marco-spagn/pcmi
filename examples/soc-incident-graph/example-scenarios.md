# PCMI · SOC dataset — example scenarios (from the CSV files)

The dataset models a **realistic SOC case-management queue**: each row is a **triaged alert** with a **disposition**. It is not a list of “successful attacks only” — it includes noise, duplicates, and benign activity, as analysts see in production.

## Disposition mix (1000 alerts)

| Disposition | Count | % | Meaning |
|-------------|------:|--:|---------|
| `true_positive` | ~520 | 52% | Confirmed malicious activity (campaigns + standalone incidents) |
| `false_positive` | ~210 | 21% | Alert closed with a **recognized benign cause** after triage |
| `duplicate` | ~120 | 12% | Same event already tracked / alert storm |
| `benign_true_positive` | ~50 | 5% | **Real but authorized** activity (not malicious) |

## Scenario 1 — True-positive campaign (coherent kill chain)

A multi-stage campaign with **persistent entities** (same host/user/IP), severity **rising toward impact**, and realistic dwell (days between stages). Stages are linked `causal` + `temporal`. A final **postmortem** node `supports` the whole chain (like an IR report consolidating IOCs).

**Try in the UI:** from the campaign’s first memory, depth 5, link types `causal,temporal`.

## Scenario 2 — False positive: vulnerability scanner

```
status=closed_false_positive  fp_cause=vuln_scanner
"Authorized vulnerability-scanner traffic (Qualys/Nessus, IP on allowlist)
 mistaken for attack"
```

Common FP causes: `pentest_redteam`, `vuln_scanner`, `geoip_vpn`, `new_role`, `av_quarantine`, `monitoring_probe`.

## Scenario 3 — Benign true positive: real but authorized

```
status=closed_benign
"Real action but AUTHORIZED: admin temporarily disabled AV for
 approved troubleshooting (change ticket)"
```

## Scenario 4 — `contradicts`: triage corrects a wrong attribution

An initial FP (analyst wrong) → `contradicts` → confirmed TP. Models conflict between two hypotheses/alerts — useful for graph reasoning (refutations, competing hypotheses).

## Scenario 5 — Alert storm + duplicates

Burst of 8–40 alerts in the same `/24` and ~15 minute window: most are `duplicate` / `false_positive`, linked `related` to a representative alert; duplicates may `contradicts` it.

## Guaranteed coherence (checked by `validate.py`)

- `ransomware` is **never** FP/benign
- `fp_cause` set **iff** disposition = `false_positive`
- `severity` distributed by category
- `first_seen ≤ detected_at` (dwell time)
- Entities consistent within an incident; IOCs only where relevant
- 14/14 MITRE tactics; all `link_type` values; valid unique ltree paths
- 0 dangling/duplicate/self-loop edges
