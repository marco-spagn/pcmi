# PCMI · SOC Dataset — Spiegazione di casi specifici (dati reali del file)

Il dataset modella una **coda di case-management SOC** realistica: ogni riga è un alert
*triagiato* con una **disposition**. Non è la lista di soli "attacchi riusciti" — è quello
che un analista vede davvero, rumore incluso.

## Distribuzione disposition (1000 alert)

| Disposition | N. | % | Cosa rappresenta |
|---|---:|---:|---|
| `true_positive` | ~520 | 52% | Attività malevola confermata (campagne + incidenti standalone) |
| `false_positive` | ~210 | 21% | Alert con **causa benigna riconosciuta** dopo triage |
| `duplicate` | ~120 | 12% | Stesso evento già tracciato / alert storm |
| `benign_true_positive` | ~50 | 5% | Attività **reale ma autorizzata** (non malevola) |

## Caso 1 — Campagna true positive (kill-chain coerente)

Una campagna multi-stadio con **entità persistenti** (stesso host/utente/IP), severity che
**sale verso l'impatto**, dwell realistico (giorni tra gli stadi). Gli stadi sono collegati
`causal`+`temporal`. Un nodo **postmortem** finale `supports` l'intera catena (come un report
IR che consolida gli IOC).

**Esplora nella UI:** dal primo nodo della campagna, depth 5, link_types `causal,temporal`.

## Caso 2 — Falso positivo con causa: vulnerability scanner

```
status=closed_false_positive  fp_cause=vuln_scanner
"Traffico del vulnerability scanner autorizzato (Qualys/Nessus, IP in allowlist)
 scambiato per attacco"
```

Le cause FP più frequenti: `pentest_redteam`, `vuln_scanner`, `geoip_vpn`, `new_role`,
`av_quarantine`, `monitoring_probe`.

## Caso 3 — Benign true positive: azione reale ma autorizzata

```
status=closed_benign
"Azione reale ma AUTORIZZATA: l'admin ha disabilitato temporaneamente l'AV per
 troubleshooting (change ticket approvato)"
```

## Caso 4 — `contradicts`: il triage corregge un'attribuzione sbagliata

Un FP iniziale (analista sbaglia) → `contradicts` → il TP confermato. Modella il conflitto
fra due ipotesi/alert — utile per ragionamento sul grafo (smentite, ipotesi rivali).

## Caso 5 — Alert storm + duplicati

Burst di 8-40 alert nello stesso `/24` e finestra di ~15 min: la maggioranza è
`duplicate`/`false_positive` collegata `related` a un alert rappresentativo, con i duplicati
che lo `contradicts`.

## Coerenze garantite (verificate da `validate.py`)

- Un `ransomware` non è **mai** FP/benign
- `fp_cause` valorizzato **iff** disposition = `false_positive`
- `severity` distribuita per categoria
- `first_seen ≤ detected_at` (dwell time)
- Entità coerenti dentro l'incidente; IOC popolati solo dove pertinenti
- 14/14 tattiche MITRE; tutti i `link_type`; path ltree validi e univoci
- 0 archi pendenti/duplicati/self-loop
