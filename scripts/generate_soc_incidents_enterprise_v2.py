#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
generate_soc_incidents_enterprise_v2.py
========================================

Enterprise SOC Incident Generator for PCMI (Persistent Cognitive Memory
Infrastructure) — produces deterministic, realistic synthetic SOC incidents
for use as test data for the distillation pipeline.

Features
--------
* 1000 (configurable) deterministic incidents (seed=42)
* Real-world 2025-2026 threat distribution (Unit42, SANS, MITRE ATT&CK 2026)
* True Positive / False Positive realistic mix (~28% TP)
* Controlled redundancy: ~65% incidents grouped in repeated campaigns
* PCMI schema compatible (path ltree + rich metadata + tags)
* Batched ingestion via PCMI Python SDK (batch_store, ~50 records/batch)
* JSONL backup on disk
* Publishes `memory.refine.requested` Redis event to trigger distillation

Usage
-----
    python generate_soc_incidents_enterprise_v2.py \
        --num-incidents 1000 \
        --tenant-id a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
        --api-url http://localhost:8080 \
        --api-key sk_test_xxx \
        --seed 42 \
        --output ./soc_incidents_backup.jsonl

Author : PCMI Data Engineering
Version: 2.0.0
"""
from __future__ import annotations

import argparse
import asyncio
import json
import logging
import os
import random
import sys
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Any, Iterable

# --------------------------------------------------------------------------- #
# PCMI SDK import (tollerante a entrambi i naming "pcmi" e "pcmi_sdk").
# L'import è lazy: in modalità --dry-run lo script funziona anche senza SDK.
# --------------------------------------------------------------------------- #
PCMIClient: Any = None  # populated by _lazy_import_sdk()


def _lazy_import_sdk() -> None:
    """Import lazy del PCMI SDK; alza errore solo quando serve davvero."""
    global PCMIClient
    if PCMIClient is not None:
        return
    try:
        from pcmi_sdk import PCMIClient as _Client  # type: ignore[import-not-found]
    except ImportError:
        try:
            from pcmi import PCMIClient as _Client  # type: ignore[no-redef]
        except ImportError as exc:  # pragma: no cover
            raise RuntimeError(
                "PCMI SDK non installato. Installa il pacchetto 'pcmi' "
                "(sdk/python) o 'pcmi_sdk' prima di ingestare."
            ) from exc
    PCMIClient = _Client


# Redis è opzionale: se manca, l'evento di distillation va in fallback API.
try:
    import redis.asyncio as aioredis  # type: ignore[import-not-found]
    _REDIS_AVAILABLE = True
except ImportError:  # pragma: no cover
    _REDIS_AVAILABLE = False


# --------------------------------------------------------------------------- #
# Logging
# --------------------------------------------------------------------------- #
LOG = logging.getLogger("pcmi.soc_gen")
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s | %(levelname)-8s | %(name)s | %(message)s",
)


# --------------------------------------------------------------------------- #
# Costanti enterprise: distribuzione, MITRE mapping, vocabolari
# --------------------------------------------------------------------------- #

# Distribuzione threat 2025-2026 (Unit42 + SANS + MITRE ATT&CK 2026).
# I valori sono percentuali normalizzate a 100.
THREAT_DISTRIBUTION: dict[str, float] = {
    "phishing":        0.38,
    "brute_force":     0.19,
    "ransomware":      0.14,
    "malware":         0.10,
    "sql_injection":   0.09,
    "insider_threat":  0.06,
    "zero_day":        0.03,
    "ddos":            0.01,
}

# MITRE ATT&CK tactic/technique map per threat type.
# Tactics: TA0001..TA0043 (Enterprise). Techniques: Txxxx[.yyy].
MITRE_MAP: dict[str, list[tuple[str, str, str, str]]] = {
    # (tactic_id, tactic_name, technique_id, technique_name)
    "phishing": [
        ("TA0001", "Initial Access",       "T1566.001", "Spearphishing Attachment"),
        ("TA0001", "Initial Access",       "T1566.002", "Spearphishing Link"),
        ("TA0001", "Initial Access",       "T1566.003", "Spearphishing via Service"),
    ],
    "brute_force": [
        ("TA0006", "Credential Access",    "T1110.001", "Password Guessing"),
        ("TA0006", "Credential Access",    "T1110.003", "Password Spraying"),
        ("TA0006", "Credential Access",    "T1110.004", "Credential Stuffing"),
    ],
    "ransomware": [
        ("TA0040", "Impact",               "T1486",     "Data Encrypted for Impact"),
        ("TA0010", "Exfiltration",         "T1567.002", "Exfiltration to Cloud Storage"),
    ],
    "malware": [
        ("TA0002", "Execution",            "T1059.001", "PowerShell"),
        ("TA0005", "Defense Evasion",      "T1027",     "Obfuscated Files or Information"),
        ("TA0003", "Persistence",          "T1547.001", "Registry Run Keys / Startup Folder"),
    ],
    "sql_injection": [
        ("TA0001", "Initial Access",       "T1190",     "Exploit Public-Facing Application"),
        ("TA0009", "Collection",           "T1213.003", "Code Repositories"),
    ],
    "insider_threat": [
        ("TA0010", "Exfiltration",         "T1052.001", "Exfiltration over USB"),
        ("TA0009", "Collection",           "T1005",     "Data from Local System"),
    ],
    "zero_day": [
        ("TA0002", "Execution",            "T1203",     "Exploitation for Client Execution"),
        ("TA0004", "Privilege Escalation", "T1068",     "Exploitation for Privilege Escalation"),
    ],
    "ddos": [
        ("TA0040", "Impact",               "T1498.001", "Direct Network Flood"),
        ("TA0040", "Impact",               "T1499.002", "Service Exhaustion Flood"),
    ],
}

SEVERITIES = ["low", "medium", "high", "critical"]
SEVERITY_WEIGHTS = [0.20, 0.45, 0.25, 0.10]

SOURCE_AGENTS = [
    "soc-edr-agent-01", "soc-edr-agent-02", "siem-correlator",
    "ndr-sensor-eu-1",  "ndr-sensor-us-1",  "email-gateway-1",
    "waf-cluster-prod", "honeypot-dmz-01",  "ueba-analyzer",
]

# Namespace deterministico per derivare UUID stabili dai nomi degli agenti
# (il repository PCMI esegue $::uuid su source_agent_id, quindi servono UUID veri).
_AGENT_NS = uuid.UUID("9d3a4f1e-7c2b-4a31-9b8e-2f0a1c4d5e6f")


def agent_uuid(name: str) -> str:
    """Deriva un UUID v5 deterministico dal nome dell'agente."""
    return str(uuid.uuid5(_AGENT_NS, name))

DEPARTMENTS = [
    "Finance", "HR", "Engineering", "Sales", "Marketing",
    "Legal", "R&D", "IT", "Operations", "Customer Support",
]

COUNTRIES = ["IT", "DE", "FR", "ES", "UK", "US", "NL", "PL", "BR", "JP", "AU"]

FP_REASONS = [
    "legitimate_admin_activity",
    "scheduled_maintenance_window",
    "approved_pentest_campaign",
    "benign_user_typo_password",
    "vendor_software_update",
    "whitelisted_internal_scanner",
    "duplicate_alert_correlation",
    "low_confidence_signature",
    "known_false_positive_rule",
    "training_environment_traffic",
]

# Strong campaigns: 51 campaign IDs ⇒ molte ripetizioni dello stesso campaign_id.
CAMPAIGN_IDS = [f"CAMP-{i:03d}" for i in range(100, 151)]

# Quote-target enterprise.
TRUE_POSITIVE_RATE = 0.28
CAMPAIGN_RATIO = 0.65


# --------------------------------------------------------------------------- #
# Templates narrativi (multi-riga, vari per threat type)
# --------------------------------------------------------------------------- #

NARRATIVE_TEMPLATES: dict[str, list[str]] = {
    "phishing": [
        (
            "SOC ANALYST REPORT — Phishing Investigation\n"
            "Incident ID: {iid}\n"
            "Reported at : {ts}\n\n"
            "Summary:\n"
            "A targeted phishing email was delivered to {user}@{org}.com from the\n"
            "spoofed sender '{sender}'. The message contained a malicious link\n"
            "pointing to '{url}' impersonating a Microsoft 365 login page.\n"
            "Three users in the {dept} department clicked the link before the\n"
            "URL was sinkholed by the secure web gateway.\n\n"
            "Initial triage:\n"
            "- Sender SPF/DKIM/DMARC: FAIL\n"
            "- Domain age: {age} days\n"
            "- URL category: credential-harvesting\n"
            "- IOC sha256 attachment: {sha}\n\n"
            "Containment:\n"
            "- Hard-deleted message from {affected} mailboxes\n"
            "- Forced password reset + MFA challenge for clickers\n"
            "- Blocked sender domain at the email gateway"
        ),
        (
            "[SOC L2] Phishing Wave — Incident {iid}\n"
            "Detected by: {agent}\n"
            "Timestamp UTC: {ts}\n\n"
            "Observation: cluster of {affected} similar emails received in\n"
            "the past {window}h, all referencing the lure 'Invoice {invn}'.\n"
            "The campaign reuses a kit previously seen in {campaign}.\n"
            "Recipients are clustered in {dept} ({country}).\n\n"
            "Actions taken: quarantined messages, added IOC '{ioc}' to TIP,\n"
            "opened ticket with the email security vendor."
        ),
    ],
    "brute_force": [
        (
            "Authentication Anomaly Report — {iid}\n"
            "Time window: {ts} → +15m\n"
            "Detection: {agent}\n\n"
            "Pattern: {attempts} failed logins against {affected} accounts\n"
            "originating from {src_ip} ({country}). Login velocity peaked at\n"
            "{rate} attempts/sec, consistent with credential-stuffing tooling.\n"
            "{succ} accounts saw a successful login after the spray.\n\n"
            "Affected service: {service}\n"
            "Containment: source IP added to perimeter denylist; risky users\n"
            "forced through step-up MFA."
        ),
        (
            "Brute-force attempt on {service} — Incident {iid}\n"
            "Source IP cluster: {src_ip} (ASN {asn}, {country})\n"
            "Targeted accounts: {affected} (mostly {dept})\n\n"
            "The attacker rotates through a list of common passwords\n"
            "(season-year patterns). No interactive session established.\n"
            "Linked campaign: {campaign}."
        ),
    ],
    "ransomware": [
        (
            "RANSOMWARE INCIDENT — {iid}\n"
            "Severity: HIGH\n"
            "First seen : {ts}\n\n"
            "Initial vector: compromised RDP gateway on {service}.\n"
            "{affected} endpoints exhibit mass file rename to '.locked' and\n"
            "drop a ransom note '{ransom_note}'. Encryption thread uses\n"
            "ChaCha20+RSA-2048. Lateral movement via SMB to file servers.\n\n"
            "C2 domain : {url}\n"
            "Sample SHA256: {sha}\n"
            "Suspected actor: {actor} (overlaps with campaign {campaign}).\n\n"
            "Response: isolated {affected} hosts, killed processes, restored\n"
            "{restored} VMs from immutable snapshots."
        ),
    ],
    "malware": [
        (
            "Endpoint Malware Detection — {iid}\n"
            "Host : {host}\n"
            "Time : {ts}\n\n"
            "Process tree: explorer.exe → wscript.exe → powershell.exe -enc <b64>\n"
            "The decoded payload pulls a second-stage loader from {url}.\n"
            "Behavior consistent with the '{family}' loader family.\n\n"
            "IOC sha256: {sha}\n"
            "MITRE: T1059.001 + T1027 (obfuscated)\n"
            "Action: quarantined binary, EDR rollback to last clean snapshot."
        ),
    ],
    "sql_injection": [
        (
            "Web Application Attack — SQLi — {iid}\n"
            "Application: {service}\n"
            "Detected at: {ts}\n\n"
            "WAF observed UNION-based SQLi payloads against the endpoint\n"
            "'{endpoint}'. Source IP {src_ip} ({country}) issued {attempts}\n"
            "requests over {window} minutes. The payload attempts to read\n"
            "from information_schema.tables and to dump the 'users' table.\n\n"
            "Blocked: yes (WAF rule {rule}).\n"
            "Application code review opened (ticket linked: {ticket})."
        ),
    ],
    "insider_threat": [
        (
            "Insider Threat / DLP — {iid}\n"
            "User: {user}@{org}.com ({dept}, {country})\n"
            "Detected: {ts}\n\n"
            "UEBA flagged an anomalous bulk download of {volume} MB from\n"
            "the 'Customer-Contracts' SharePoint site, followed by an\n"
            "attempted upload to a personal cloud service ({url}).\n"
            "The user is in a 60-day notice period.\n\n"
            "Actions: DLP block engaged, manager and HR notified, forensic\n"
            "image of the endpoint scheduled."
        ),
    ],
    "zero_day": [
        (
            "Zero-Day Exploitation (suspected) — {iid}\n"
            "Asset: {service} (version {version})\n"
            "Timestamp: {ts}\n\n"
            "Telemetry shows post-exploitation behavior (suid drop, /etc/passwd\n"
            "modification) without any matching vulnerability signature.\n"
            "No public CVE matches the observed exploit chain. The host was\n"
            "exposed on the internet via {service}.\n\n"
            "IOC sha256 of dropper: {sha}\n"
            "Campaign correlation: {campaign}\n"
            "Vendor notified, IR team engaged, host taken offline."
        ),
    ],
    "ddos": [
        (
            "Volumetric DDoS Event — {iid}\n"
            "Target: {service} (edge {edge})\n"
            "Window: {ts} → +{duration}m\n\n"
            "Peak: {rate} Gbps / {pps} Mpps — UDP amplification (NTP+DNS).\n"
            "Mitigation engaged automatically by the upstream scrubbing\n"
            "center; customer-facing impact contained to a {impact}s blip."
        ),
    ],
}


# --------------------------------------------------------------------------- #
# Data classes
# --------------------------------------------------------------------------- #


@dataclass
class GeneratorConfig:
    num_incidents: int
    tenant_id: str
    api_url: str
    api_key: str
    seed: int
    output: str
    batch_size: int = 50
    throttle_ms: int = 0   # delay tra batch consecutivi (rate-limit ingest)
    redis_url: str = "redis://localhost:6379/0"
    # NB: il worker PCMI sottoscrive il canale 'memory_events' e dispatcha
    # in base al campo Type del wrapper. Il "logical event" è
    # 'memory.refine.requested'.
    redis_channel: str = "memory_events"
    redis_event_type: str = "memory.refine.requested"
    refine_path_prefix: str = "root.security.incidents.soc"
    test_data_version: str = "2.0"
    use_sharding: bool = True
    shard_size: int = 10


@dataclass
class Incident:
    path: str
    content: str
    metadata: dict[str, Any]
    tags: list[str]
    source_agent_id: str
    valid_from: str
    version: int = 1


# --------------------------------------------------------------------------- #
# Helpers deterministici
# --------------------------------------------------------------------------- #


def _weighted_choice(rng: random.Random, items: list[str], weights: list[float]) -> str:
    return rng.choices(items, weights=weights, k=1)[0]


def _exact_threat_assignments(rng: random.Random, total: int) -> list[str]:
    """
    Crea un vettore di threat types lungo `total` rispettando *esattamente*
    le quote di THREAT_DISTRIBUTION (con remainder distribuito sui più frequenti).
    """
    counts: dict[str, int] = {}
    assigned = 0
    items = list(THREAT_DISTRIBUTION.items())
    for name, pct in items:
        n = int(round(pct * total))
        counts[name] = n
        assigned += n
    # Bilancia eventuali drift di arrotondamento sul threat più frequente.
    drift = total - assigned
    if drift != 0:
        counts["phishing"] += drift
    expanded: list[str] = []
    for name, n in counts.items():
        expanded.extend([name] * n)
    rng.shuffle(expanded)
    return expanded


def _random_timestamp(rng: random.Random, days_back: int = 365) -> datetime:
    """Timestamp uniforme negli ultimi `days_back` giorni (UTC, deterministico)."""
    now = datetime(2026, 5, 16, 12, 0, 0, tzinfo=timezone.utc)
    delta_seconds = rng.randint(0, days_back * 24 * 3600)
    return now - timedelta(seconds=delta_seconds)


def _rand_ip(rng: random.Random) -> str:
    return ".".join(str(rng.randint(1, 254)) for _ in range(4))


def _rand_sha256(rng: random.Random) -> str:
    return "".join(rng.choice("0123456789abcdef") for _ in range(64))


def _rand_domain(rng: random.Random) -> str:
    tlds = ["com", "net", "io", "biz", "co", "xyz", "info"]
    name = "".join(rng.choice("abcdefghijklmnopqrstuvwxyz") for _ in range(rng.randint(6, 12)))
    return f"{name}.{rng.choice(tlds)}"


def _rand_user(rng: random.Random) -> str:
    first = rng.choice([
        "marco", "anna", "lucas", "sofia", "kenji", "ines", "diego",
        "yuki", "noah", "elena", "omar", "priya", "lars", "wei", "ahmed",
    ])
    last = rng.choice([
        "rossi", "smith", "garcia", "schmidt", "dupont", "tanaka",
        "silva", "petrova", "novak", "khan", "olsen", "müller", "rivera",
    ])
    return f"{first}.{last}"


# --------------------------------------------------------------------------- #
# Narrative renderer
# --------------------------------------------------------------------------- #


def _render_narrative(
    rng: random.Random,
    threat_type: str,
    incident_id: str,
    ts: datetime,
    agent: str,
    campaign: str | None,
) -> tuple[str, dict[str, Any]]:
    """Restituisce (testo_narrativo, dict di parametri ausiliari per metadata)."""
    template = rng.choice(NARRATIVE_TEMPLATES[threat_type])

    extras: dict[str, Any] = {
        "iid":         incident_id,
        "ts":          ts.isoformat(),
        "agent":       agent,
        "user":        _rand_user(rng),
        "org":         rng.choice(["acme", "globex", "initech", "umbrella", "soylent"]),
        "sender":      f"no-reply@{_rand_domain(rng)}",
        "url":         f"https://{_rand_domain(rng)}/login",
        "endpoint":    rng.choice(["/api/v1/users", "/login", "/search", "/checkout"]),
        "dept":        rng.choice(DEPARTMENTS),
        "country":     rng.choice(COUNTRIES),
        "age":         rng.randint(1, 14),
        "sha":         _rand_sha256(rng),
        "affected":    rng.randint(1, 250),
        "attempts":    rng.randint(50, 50000),
        "succ":        rng.randint(0, 5),
        "src_ip":      _rand_ip(rng),
        "asn":         f"AS{rng.randint(1000, 65000)}",
        "rate":        rng.randint(5, 800),
        "service":     rng.choice([
                          "exchange-online", "okta-prod", "vpn-gw-eu",
                          "rdp-jumpbox", "salesforce", "github-enterprise",
                          "atlassian-cloud", "sap-erp", "kubernetes-api",
                      ]),
        "window":      rng.randint(5, 120),
        "invn":        f"INV-{rng.randint(10000, 99999)}",
        "campaign":    campaign or "n/a",
        "ioc":         _rand_sha256(rng)[:32],
        "ransom_note": rng.choice(["README_HOW_TO_DECRYPT.txt", "!!!RESTORE_FILES.txt"]),
        "actor":       rng.choice([
                          "FIN7", "Lazarus", "APT29", "Conti-spinoff",
                          "BlackBasta", "ScatteredSpider", "UNC4841",
                      ]),
        "restored":    rng.randint(1, 50),
        "host":        f"WS-{rng.randint(1000, 9999)}",
        "family":      rng.choice([
                          "QakBot", "IcedID", "Emotet", "AsyncRAT",
                          "Cobalt-Strike-beacon", "Sliver",
                      ]),
        "rule":        f"RULE-{rng.randint(900, 999)}",
        "ticket":      f"JIRA-SEC-{rng.randint(1000, 9999)}",
        "volume":      rng.randint(50, 5000),
        "version":     f"{rng.randint(1,9)}.{rng.randint(0,30)}.{rng.randint(0,99)}",
        "edge":        f"edge-{rng.choice(['ams','fra','mil','nyc','sjc'])}-{rng.randint(1,12)}",
        "duration":    rng.randint(2, 90),
        "pps":         rng.randint(5, 250),
        "impact":      rng.randint(0, 30),
    }

    return template.format(**extras), extras


# --------------------------------------------------------------------------- #
# Core: incident factory
# --------------------------------------------------------------------------- #


def build_incident(
    rng: random.Random,
    threat_type: str,
    cfg: GeneratorConfig,
    *,
    shard_id: int | None = None,
    shard_size: int = 10,
) -> Incident:
    """Costruisce un singolo Incident PCMI-compatibile."""
    incident_id = f"INC-{uuid.UUID(int=rng.getrandbits(128)).hex[:12].upper()}"

    # Campaign assignment (ridondanza controllata).
    is_campaign = rng.random() < CAMPAIGN_RATIO
    campaign_id = rng.choice(CAMPAIGN_IDS) if is_campaign else None

    # MITRE mapping
    tactic_id, tactic_name, technique_id, technique_name = rng.choice(MITRE_MAP[threat_type])

    # Severity, agent, timestamps
    severity = _weighted_choice(rng, SEVERITIES, SEVERITY_WEIGHTS)
    agent = rng.choice(SOURCE_AGENTS)
    ts = _random_timestamp(rng)

    # Narrativa
    content, extras = _render_narrative(rng, threat_type, incident_id, ts, agent, campaign_id)

    # TP / FP
    true_positive = rng.random() < TRUE_POSITIVE_RATE
    fp_reason = None if true_positive else rng.choice(FP_REASONS)
    # Confidence: TP più alta, FP più bassa, con rumore realistico.
    if true_positive:
        detection_confidence = round(rng.uniform(0.70, 0.99), 2)
    else:
        detection_confidence = round(rng.uniform(0.20, 0.74), 2)

    response_time_minutes = max(1, int(rng.gauss(mu=22, sigma=18)))

    affected_systems = sorted({
        f"host-{rng.randint(100, 9999)}"
        for _ in range(rng.randint(1, 6))
    })

    # Path ltree: gerarchia leggibile e compatibile (no hyphen).
    severity_seg = severity.replace("-", "_")
    threat_seg = threat_type.replace("-", "_")
    incident_seg = incident_id.replace("-", "_").lower()
    # Shard segment (opzionale): se presente, viene inserito subito sotto 'soc'
    # per consentire un refine event per shard → 1 distilled record ogni N raw,
    # con copertura totale del dataset (no LIMIT 10 wasteful).
    if shard_id is not None:
        shard_seg = f"shard_{shard_id:03d}"
        path = (
            f"root.security.incidents.soc.{shard_seg}."
            f"{threat_seg}.{severity_seg}.{incident_seg}"
        )
    else:
        path = f"root.security.incidents.soc.{threat_seg}.{severity_seg}.{incident_seg}"

    metadata: dict[str, Any] = {
        "incident_id":           incident_id,
        "severity":              severity,
        "reported_at":           ts.isoformat(),
        "true_positive":         true_positive,
        "false_positive_reason": fp_reason,
        "detection_confidence":  detection_confidence,
        "response_time_minutes": response_time_minutes,
        "affected_systems":      affected_systems,
        "mitre_tactic":          {"id": tactic_id, "name": tactic_name},
        "mitre_technique":       {"id": technique_id, "name": technique_name},
        "campaign_id":           campaign_id,
        "source_agent":          agent,
        "threat_type":           threat_type,
        "country":               extras["country"],
        "department":            extras["dept"],
        "service":               extras["service"],
        "test_data_seed":        "42",
        "test_data_version":     cfg.test_data_version,
        "shard_id":              shard_id,
    }

    tags = [
        f"threat:{threat_type}",
        f"severity:{severity}",
        f"mitre:{technique_id}",
        f"tactic:{tactic_id.lower()}",
        f"tp:{str(true_positive).lower()}",
        f"agent:{agent}",
        "soc",
        "synthetic",
        f"v{cfg.test_data_version}",
    ]
    if campaign_id:
        tags.append(f"campaign:{campaign_id.lower()}")
    if shard_id is not None:
        tags.append(f"shard:{shard_id:03d}")

    return Incident(
        path=path,
        content=content,
        metadata=metadata,
        tags=tags,
        # source_agent_id deve essere un UUID (cast $::uuid in repository);
        # il nome leggibile resta in metadata.source_agent.
        source_agent_id=agent_uuid(agent),
        valid_from=ts.isoformat(),
        version=1,
    )


# --------------------------------------------------------------------------- #
# Generation pipeline
# --------------------------------------------------------------------------- #


def generate_incidents(cfg: GeneratorConfig) -> list[Incident]:
    """Genera in modo deterministico la lista completa di incidenti."""
    rng = random.Random(cfg.seed)
    threats = _exact_threat_assignments(rng, cfg.num_incidents)
    LOG.info(
        "Threat distribution computed: %s",
        {k: threats.count(k) for k in THREAT_DISTRIBUTION},
    )

    # Sharding deterministico: ogni shard contiene esattamente
    # cfg.shard_size record. Numero shard = ceil(num_incidents/shard_size).
    # Refinendo per ogni shard otteniamo 1 distilled per shard.
    shard_size = max(1, cfg.shard_size) if cfg.use_sharding else 0
    incidents: list[Incident] = []
    for i, threat_type in enumerate(threats):
        if shard_size > 0:
            shard_id = i // shard_size
            inc = build_incident(rng, threat_type, cfg,
                                 shard_id=shard_id, shard_size=shard_size)
        else:
            inc = build_incident(rng, threat_type, cfg)
        incidents.append(inc)
        if (i + 1) % 100 == 0:
            LOG.info("Generated %d/%d incidents", i + 1, cfg.num_incidents)
    LOG.info("Total generated: %d", len(incidents))
    if shard_size > 0:
        n_shards = (cfg.num_incidents + shard_size - 1) // shard_size
        LOG.info("Sharding: %d shards of %d records (shard_000..shard_%03d)",
                 n_shards, shard_size, n_shards - 1)
    return incidents


def write_jsonl_backup(incidents: Iterable[Incident], output_path: str, cfg: GeneratorConfig) -> int:
    """Salva un backup JSONL completo per audit / replay deterministico."""
    n = 0
    os.makedirs(os.path.dirname(os.path.abspath(output_path)) or ".", exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as fh:
        for inc in incidents:
            record = {
                "tenant_id":       cfg.tenant_id,
                "path":            inc.path,
                "content":         inc.content,
                "metadata":        inc.metadata,
                "tags":            inc.tags,
                "version":         inc.version,
                "valid_from":      inc.valid_from,
                "source_agent_id": inc.source_agent_id,
            }
            fh.write(json.dumps(record, ensure_ascii=False) + "\n")
            n += 1
    LOG.info("JSONL backup written: %s (%d records)", output_path, n)
    return n


# --------------------------------------------------------------------------- #
# Ingest + Redis
# --------------------------------------------------------------------------- #


def _to_batch_item(inc: Incident) -> dict[str, Any]:
    """Converte un Incident nel formato accettato da PCMIClient.batch_store()."""
    return {
        "path":            inc.path,
        "content":         inc.content,
        "metadata":        inc.metadata,
        "tags":            inc.tags,
        "source_agent_id": inc.source_agent_id,
    }


async def ingest_via_sdk(
    cfg: GeneratorConfig,
    incidents: list[Incident],
) -> tuple[int, int]:
    """
    Ingest a batch tramite SDK PCMI.
    Espone l'alias `store_batch` come richiesto dalla spec enterprise.
    Ritorna (ok_count, fail_count).
    """
    _lazy_import_sdk()
    ok = 0
    fail = 0
    first_error_logged = False
    async with PCMIClient(base_url=cfg.api_url, api_key=cfg.api_key) as client:
        # Alias semantico richiesto dalla spec ("store_batch").
        store_batch = getattr(client, "store_batch", client.batch_store)

        total = len(incidents)
        for start in range(0, total, cfg.batch_size):
            chunk = incidents[start:start + cfg.batch_size]
            payload = [_to_batch_item(i) for i in chunk]
            try:
                resp = await store_batch(payload)
            except Exception as exc:  # noqa: BLE001
                fail += len(chunk)
                LOG.error("Batch %d-%d HTTP failed: %s",
                          start + 1, start + len(chunk), exc)
                continue

            # Il batch endpoint risponde 200 anche se i singoli item falliscono:
            # bisogna controllare results[].status per ogni elemento.
            results = (resp or {}).get("results", []) if isinstance(resp, dict) else []
            stored_in_chunk = 0
            errors_in_chunk: list[str] = []
            for item in results:
                status = item.get("status")
                if status in ("stored", "skipped"):
                    stored_in_chunk += 1
                else:
                    errors_in_chunk.append(item.get("error") or status or "unknown")

            ok += stored_in_chunk
            fail += (len(chunk) - stored_in_chunk)

            if errors_in_chunk:
                # Logga il primo errore in modo verboso, poi solo summary.
                if not first_error_logged:
                    LOG.error(
                        "Batch %d-%d: %d/%d FAILED. First error: %s",
                        start + 1, start + len(chunk),
                        len(errors_in_chunk), len(chunk),
                        errors_in_chunk[0],
                    )
                    first_error_logged = True
                else:
                    LOG.error(
                        "Batch %d-%d: %d/%d FAILED (first: %.120s)",
                        start + 1, start + len(chunk),
                        len(errors_in_chunk), len(chunk),
                        errors_in_chunk[0],
                    )
            else:
                LOG.info(
                    "Batch %d-%d/%d uploaded ok=%d",
                    start + 1, start + len(chunk), total, stored_in_chunk,
                )

            # Throttle tra batch per non saturare il worker di distillation
            # (ogni Store triggera 1 call LLM → 429 con bulk velocissimo).
            if cfg.throttle_ms > 0 and (start + cfg.batch_size) < total:
                await asyncio.sleep(cfg.throttle_ms / 1000.0)
    return ok, fail


async def publish_refine_event(cfg: GeneratorConfig) -> bool:
    """
    Pubblica su Redis l'evento `memory.refine.requested` per triggerare
    la pipeline di distillation lato worker.
    Fallback: chiama POST /v1/memories/refine via SDK se Redis manca.
    """
    # Il worker PCMI legge dal canale Redis 'memory_events' e fa lo switch
    # sul campo Type del wrapper (vedi internal/event/redis.go + cmd/worker).
    # Il wrapper Go è `type Event struct { Type string; Payload map[string]any }`
    # → in JSON i campi sono PascalCase ("Type", "Payload"). L'Unmarshal di Go
    # è comunque case-insensitive sui field names, quindi entrambe le forme
    # funzionano; usiamo PascalCase per essere espliciti.
    wrapper = {
        "Type": cfg.redis_event_type,  # "memory.refine.requested"
        "Payload": {
            "tenant_id":         cfg.tenant_id,
            "path_prefix":       cfg.refine_path_prefix,
            "reason":            "bulk_synthetic_load_completed",
            "correlation_id":    f"soc-bulk-{cfg.seed}-{cfg.num_incidents}",
            "agent_id":          "soc-data-engineer",
            "test_data_seed":    "42",
            "test_data_version": cfg.test_data_version,
            "timestamp":         datetime.now(tz=timezone.utc).isoformat(),
        },
    }

    if _REDIS_AVAILABLE:
        try:
            r = aioredis.from_url(cfg.redis_url, encoding="utf-8", decode_responses=True)
            n = await r.publish(cfg.redis_channel, json.dumps(wrapper))
            await r.aclose()
            LOG.info(
                "Redis published on channel='%s' type='%s' subscribers=%d",
                cfg.redis_channel, cfg.redis_event_type, n,
            )
            if n == 0:
                LOG.warning(
                    "0 subscribers su '%s' — il worker non è in ascolto. "
                    "Verifica `docker compose logs worker` (deve mostrare "
                    "'subscribing to memory_events…').",
                    cfg.redis_channel,
                )
            return True
        except Exception as exc:  # noqa: BLE001
            LOG.warning("Redis publish failed (%s). Falling back to SDK /refine.", exc)

    # Fallback API
    try:
        _lazy_import_sdk()
        async with PCMIClient(base_url=cfg.api_url, api_key=cfg.api_key) as client:
            await client.refine(cfg.refine_path_prefix)
        LOG.info("Distillation queued via SDK /v1/memories/refine.")
        return True
    except Exception as exc:  # noqa: BLE001
        LOG.error("Could not trigger distillation: %s", exc)
        return False


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #


def parse_args(argv: list[str] | None = None) -> GeneratorConfig:
    p = argparse.ArgumentParser(
        prog="generate_soc_incidents_enterprise_v2",
        description="Enterprise SOC incidents generator for PCMI distillation tests.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    p.add_argument("--num-incidents", type=int, default=1000)
    p.add_argument("--tenant-id", type=str,
                   default="a1b2c3d4-e5f6-7890-abcd-ef1234567890")
    p.add_argument("--api-url",  type=str, default=os.getenv("PCMI_API_URL", "http://localhost:8080"))
    p.add_argument("--api-key",  type=str, default=os.getenv("PCMI_API_KEY", ""))
    p.add_argument("--seed",     type=int, default=42)
    p.add_argument("--output",   type=str, default="./soc_incidents_backup.jsonl")
    p.add_argument("--batch-size", type=int, default=50)
    p.add_argument("--throttle-ms", type=int, default=0,
                   help="Pausa tra batch consecutivi (utile per non far esplodere il rate limit del worker LLM).")
    p.add_argument("--redis-url", type=str,
                   default=os.getenv("PCMI_REDIS_URL", "redis://localhost:6379/0"))
    # NB: il canale corretto è 'memory_events' (vedi internal/event/redis.go).
    # Il vecchio default 'memory.refine.requested' era sbagliato (canale errato → subscribers=0).
    p.add_argument("--redis-channel", type=str, default="memory_events")
    p.add_argument("--redis-event-type", type=str, default="memory.refine.requested")
    p.add_argument("--refine-path-prefix", type=str,
                   default="root.security.incidents.soc")
    p.add_argument("--shard-size", type=int, default=10,
                   help="Numero di record per shard (cfr. LIMIT 10 del worker distillation).")
    p.add_argument("--no-sharding", action="store_true",
                   help="Disattiva lo sharding del path (1 refine event copre solo 10 record).")
    p.add_argument("--dry-run", action="store_true",
                   help="Generate + JSONL only, do not call the PCMI API/Redis.")
    p.add_argument("--skip-publish", action="store_true",
                   help="Skip Redis publish (utile quando il refine viene triggerato esternamente).")

    args = p.parse_args(argv)

    cfg = GeneratorConfig(
        num_incidents=args.num_incidents,
        tenant_id=args.tenant_id,
        api_url=args.api_url,
        api_key=args.api_key,
        seed=args.seed,
        output=args.output,
        batch_size=args.batch_size,
        throttle_ms=args.throttle_ms,
        redis_url=args.redis_url,
        redis_channel=args.redis_channel,
        redis_event_type=args.redis_event_type,
        refine_path_prefix=args.refine_path_prefix,
        use_sharding=not args.no_sharding,
        shard_size=args.shard_size,
    )
    cfg._dry_run = args.dry_run  # type: ignore[attr-defined]
    cfg._skip_publish = args.skip_publish  # type: ignore[attr-defined]
    return cfg


# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #


async def _async_main(cfg: GeneratorConfig) -> int:
    LOG.info("=== PCMI SOC Incident Generator v2.0 ===")
    LOG.info("Tenant: %s | API: %s | seed=%d | n=%d",
             cfg.tenant_id, cfg.api_url, cfg.seed, cfg.num_incidents)

    incidents = generate_incidents(cfg)

    # Sanity: distribution check
    dist = {}
    for inc in incidents:
        dist[inc.metadata["threat_type"]] = dist.get(inc.metadata["threat_type"], 0) + 1
    LOG.info("Realized distribution: %s", dist)

    tp_count = sum(1 for i in incidents if i.metadata["true_positive"])
    LOG.info("TP rate realized: %.2f%% (target ~%.0f%%)",
             100.0 * tp_count / len(incidents), TRUE_POSITIVE_RATE * 100)

    camp_count = sum(1 for i in incidents if i.metadata["campaign_id"])
    LOG.info("Campaign coverage: %.2f%% (target ~%.0f%%)",
             100.0 * camp_count / len(incidents), CAMPAIGN_RATIO * 100)

    # JSONL backup (sempre)
    write_jsonl_backup(incidents, cfg.output, cfg)

    if getattr(cfg, "_dry_run", False):
        LOG.warning("--dry-run set: skipping PCMI ingest and Redis publish.")
        return 0

    if not cfg.api_key:
        LOG.error("Missing --api-key (or PCMI_API_KEY env). Aborting ingest.")
        return 2

    # Ingest
    ok, fail = await ingest_via_sdk(cfg, incidents)
    LOG.info("Ingest result: ok=%d fail=%d", ok, fail)

    # Sanity check: dopo l'ingest, lo /v1/stats deve mostrare un delta
    # coerente. Se ok>0 ma stats=0, c'è un problema di RLS / tenant context.
    try:
        _lazy_import_sdk()
        async with PCMIClient(base_url=cfg.api_url, api_key=cfg.api_key) as c:
            stats = await c.tenant_stats()
        active = stats.get("active_memories", 0)
        LOG.info("Post-ingest /v1/stats.active_memories=%d", active)
        if ok > 0 and active == 0:
            LOG.error(
                "STATS=0 nonostante ok=%d → l'ingest non ha persistito davvero. "
                "Cause tipiche: RLS / tenant context errato, oppure path ltree "
                "non valido. Controlla `docker compose logs api`.",
                ok,
            )
    except Exception as exc:  # noqa: BLE001
        LOG.warning("Could not fetch /v1/stats for sanity check: %s", exc)

    # Trigger distillation (a meno che lo shell non lo faccia esternamente)
    if getattr(cfg, "_skip_publish", False):
        LOG.info("--skip-publish: refine event NON pubblicato (lo farà l'orchestrator).")
        return 0 if fail == 0 else 1

    refined = await publish_refine_event(cfg)
    if not refined:
        LOG.error("Distillation event could not be dispatched.")

    return 0 if fail == 0 and refined else 1


def main(argv: list[str] | None = None) -> int:
    cfg = parse_args(argv)
    try:
        return asyncio.run(_async_main(cfg))
    except KeyboardInterrupt:
        LOG.warning("Interrupted by user.")
        return 130


if __name__ == "__main__":
    sys.exit(main())
