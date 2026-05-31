# PCMI · SOC Incident Graph Dataset — Data Dictionary (v2 realistic)

**1000 alert/incidenti SOC realistici + ~1300 archi** del grafo cognitivo, pronti per
PCMI (memory_entries + memory_links -> grafo Apache AGE).

## File
| File | Righe | Descrizione |
|------|------:|-------------|
| soc_incidents_nodes.csv | 1000 | Un alert/incidente per riga (nodo) |
| soc_incidents_links.csv | ~1333 | Archi tipizzati (referenziano external_id) |
| generate_soc_dataset.py | - | Generatore deterministico (seed 1337) |
| validate.py | - | Validazione integrità + coerenza |
| load_to_pcmi.py | - | Loader (batch nodi + link memory.<db_id>, resumable) |
| CASI_SPECIFICI.md | - | Spiegazione di casi reali estratti dal dataset |

## Colonne soc_incidents_nodes.csv
- external_id : INC0000001... handle stabile (NON l'id DB)
- path        : ltree root.soc.<cat>.<anno>[.camp_<id>].inc_<n>
- content     : narrativa con ENTITA' COERENTI (stesso host/utente/IP/dominio)
- tags        : '|'-separati
- disposition : true_positive 52% / false_positive 21% / duplicate 12% / benign_true_positive 5%
- severity    : P1..P4 distribuita per categoria
- status      : resolved/contained/investigating/escalated (TP) · closed_false_positive/closed_benign/closed_duplicate
- category    : 21 categorie (14/14 tattiche MITRE)
- mitre_tactic / mitre_technique : coerenti con la categoria
- first_seen / detected_at : dwell time (first_seen <= detected_at)
- region, data_source : sorgente plausibile per categoria (EDR/WAF/SIEM/DLP...)
- escalation_tier : L1/L2/L3   |  analyst : coerente col tier
- threat_actor : APT o crime group (solo per TP)
- campaign_id  : CAMP000001 se in campagna
- src_ip / dst_host / affected_user : entità coinvolte
- ioc_hash / ioc_domain / cve_id : IOC, solo dove pertinente
- fp_cause     : valorizzato IFF false_positive
- benign_cause : valorizzato IFF benign_true_positive
- closure_reason : motivazione chiusura
- asset_criticality : low/medium/high/critical  |  confidence : low/medium/high
- cvss_score   : coerente con severità
- alert_count  : n. alert grezzi aggregati
- sla_met      : true/false (SLA: P1 15m, P2 1h, P3 8h, P4 24h)
- ttd_minutes / ttr_minutes : time-to-detect / time-to-respond

## Colonne soc_incidents_links.csv
from_external_id, to_external_id, link_type, rationale
link_type: causal 29% · temporal 29% · related 20% · supports 13% · contradicts 10%

## Struttura del grafo
- Campagne multi-stadio (kill chain coerente, entità persistenti) -> causal + temporal
- Postmortem -> supports verso gli stadi
- Falsi positivi iniziali -> contradicts l'attività confermata
- Cross-campagna stesso attore -> related
- Alert storm (burst stesso /24) -> related + contradicts per i duplicati
- ~27% nodi isolati (alert standalone) — caso reale

## Uso
  export PCMI_BASE_URL=http://localhost:8000
  export PCMI_API_KEY=testkey123
  python3 load_to_pcmi.py --limit 2000                    # smoke test
  python3 load_to_pcmi.py --batch 50 --link-workers 16    # full load (resumable)
  python3 generate_soc_dataset.py                         # rigenera identico
  python3 validate.py                                     # 0 errori attesi
