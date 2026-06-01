# SOC incident graph dataset — data dictionary (realistic v2)

**1000 realistic SOC alerts/incidents + ~1300 graph edges**, ready for PCMI (`memory_entries` + `memory_links` → Apache AGE).

## Files in this folder

| File | Rows | Description |
|------|-----:|-------------|
| `soc_incidents_nodes.csv` | 1000 | One alert/incident per row (node) |
| `soc_incidents_links.csv` | ~1333 | Typed edges (reference `external_id`) |
| `generate_soc_dataset.py` | — | Deterministic generator (seed 1337) |
| `validate.py` | — | Integrity + coherence validation |
| `load_to_pcmi.py` | — | Loader (batch nodes + `memory.<db_id>` links, resumable) |
| `example-scenarios.md` | — | Walkthrough of patterns in the data |

## `soc_incidents_nodes.csv` columns

- **external_id** — `INC0000001…` stable handle (not DB id)
- **path** — ltree `root.soc.<cat>.<year>[.camp_<id>].inc_<n>`
- **content** — narrative with **consistent entities** (same host/user/IP/domain)
- **tags** — pipe-separated
- **disposition** — `true_positive` 52% / `false_positive` 21% / `duplicate` 12% / `benign_true_positive` 5%
- **severity** — P1–P4 by category
- **status** — `resolved`/`contained`/`investigating`/`escalated` (TP) · `closed_false_positive`/`closed_benign`/`closed_duplicate`
- **category** — 21 categories (14/14 MITRE tactics)
- **mitre_tactic** / **mitre_technique** — aligned with category
- **first_seen** / **detected_at** — dwell (`first_seen <= detected_at`)
- **region**, **data_source** — plausible source per category (EDR/WAF/SIEM/DLP…)
- **escalation_tier** — L1/L2/L3 · **analyst** — tier-consistent
- **threat_actor** — APT or crime group (TP only)
- **campaign_id** — `CAMP000001` when in a campaign
- **src_ip** / **dst_host** / **affected_user** — involved entities
- **ioc_hash** / **ioc_domain** / **cve_id** — IOCs when relevant
- **fp_cause** — set iff `false_positive`
- **benign_cause** — set iff `benign_true_positive`
- **closure_reason** — closure rationale
- **asset_criticality** — low/medium/high/critical · **confidence** — low/medium/high
- **cvss_score** — aligned with severity
- **alert_count** — raw alerts aggregated
- **sla_met** — true/false (SLA: P1 15m, P2 1h, P3 8h, P4 24h)
- **ttd_minutes** / **ttr_minutes** — time-to-detect / time-to-respond

## `soc_incidents_links.csv` columns

`from_external_id`, `to_external_id`, `link_type`, `rationale`

**link_type** mix: causal 29% · temporal 29% · related 20% · supports 13% · contradicts 10%

## Graph shape

- Multi-stage campaigns (kill chain, persistent entities) → `causal` + `temporal`
- Postmortem → `supports` toward stages
- Initial false positives → `contradicts` confirmed activity
- Cross-campaign same actor → `related`
- Alert storm (same /24 burst) → `related` + `contradicts` on duplicates
- ~27% isolated nodes (standalone alerts) — realistic

## Usage

```bash
export PCMI_BASE_URL=http://localhost:8000
export PCMI_API_KEY=testkey123
cd examples/soc-incident-graph
python3 load_to_pcmi.py --limit 200              # smoke test
python3 load_to_pcmi.py --batch 50 --link-workers 16   # full load (resumable)
python3 generate_soc_dataset.py                  # regenerate (deterministic)
python3 validate.py                              # expect 0 errors
```
