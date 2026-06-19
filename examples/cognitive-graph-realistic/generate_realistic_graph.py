#!/usr/bin/env python3
"""Generate a deterministic, realistic Cognitive Graph JSON dataset.

The output uses the same compact contract as graph_matrix.json:

  {
    "nodes": [{"key", "path", "content", "tags", "metadata"}],
    "links": [{"from", "to", "type", "weight", "rationale"}]
  }

It models a SOC/incident-response knowledge graph with campaigns, alerts,
evidence notes, hypotheses, postmortem notes, duplicates, false positives,
cycles, and isolated low-signal alerts.
"""

from __future__ import annotations

import argparse
import json
import random
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path


LINK_TYPES = ("causal", "temporal", "supports", "contradicts", "related")
TACTICS = (
    "recon",
    "initial_access",
    "execution",
    "persistence",
    "privilege_escalation",
    "defense_evasion",
    "credential_access",
    "discovery",
    "lateral_movement",
    "collection",
    "command_and_control",
    "exfiltration",
    "impact",
)
STAGES = {
    "recon": ("T1595.002", "External scan found exposed services on {asset}."),
    "initial_access": ("T1566.001", "Phishing delivery targeted {user} from {domain}."),
    "execution": ("T1059.001", "PowerShell payload executed on {asset} by {user}."),
    "persistence": ("T1547.001", "Persistence artifact created on {asset}."),
    "privilege_escalation": ("T1068", "Privilege escalation attempt on {asset} using {cve}."),
    "defense_evasion": ("T1562.001", "Security tooling was disabled on {asset}."),
    "credential_access": ("T1003.001", "Credential material was accessed on {asset}."),
    "discovery": ("T1087.002", "Directory and host discovery ran from {asset}."),
    "lateral_movement": ("T1021.002", "Lateral movement from {asset} to {peer_asset}."),
    "collection": ("T1005", "Sensitive files were staged on {asset}."),
    "command_and_control": ("T1071.001", "Beaconing from {asset} to {domain}."),
    "exfiltration": ("T1041", "Outbound transfer from {asset} to {domain}."),
    "impact": ("T1486", "Encryption and service-impact indicators observed on {asset}."),
}
CAMPAIGN_NAMES = (
    "royal-river",
    "silver-needle",
    "black-canal",
    "atlas-fog",
    "orange-otter",
    "winter-lotus",
    "ember-lake",
    "quiet-mantis",
)
ACTORS = ("LockBit", "FIN7", "APT29", "Scattered Spider", "Lazarus", "Akira", "UNC2452")
DISPOSITIONS = ("true_positive", "false_positive", "benign_true_positive", "duplicate")
FALSE_POSITIVE_CAUSES = (
    "authorized vulnerability scan",
    "planned red-team exercise",
    "SCCM software deployment",
    "backup replication job",
    "VPN GeoIP mismatch",
    "monitoring probe",
    "stale threat-intel IOC",
)
BENIGN_CAUSES = (
    "approved emergency admin access",
    "maintenance-window security-tool change",
    "approved DLP exception",
    "authorized inventory scan",
)
USERS = (
    "mario.rossi",
    "giulia.bianchi",
    "luca.ferrari",
    "sara.ricci",
    "paolo.romano",
    "chiara.greco",
    "davide.conti",
    "elena.russo",
    "svc_backup",
    "svc_deploy",
)
ASSET_PREFIXES = ("WKS", "SRV", "WEB", "DB", "DC", "FS", "VPN", "K8S", "JMP")
DOMAINS = (
    "cdn-verify-login.net",
    "account-session-check.io",
    "m365-security-update.com",
    "telemetry-gateway.xyz",
    "sharepoint-document.online",
    "backup-sync-cloud.net",
)
CVES = ("CVE-2023-34362", "CVE-2021-44228", "CVE-2024-1709", "CVE-2023-4966")
SOURCES = (
    "CrowdStrike Falcon",
    "Microsoft Sentinel",
    "Defender for Endpoint",
    "Okta",
    "Cloudflare WAF",
    "Zeek",
    "Suricata",
    "Microsoft Purview DLP",
)


@dataclass(frozen=True)
class EntitySet:
    user: str
    asset: str
    peer_asset: str
    domain: str
    cve: str


def slug(value: str) -> str:
    out = []
    for ch in value.lower():
        out.append(ch if ch.isalnum() else "_")
    return "_".join(part for part in "".join(out).split("_") if part)


def make_entities(rng: random.Random) -> EntitySet:
    asset = f"{rng.choice(ASSET_PREFIXES)}-{rng.randint(100, 999)}"
    peer = f"{rng.choice(ASSET_PREFIXES)}-{rng.randint(100, 999)}"
    return EntitySet(
        user=rng.choice(USERS),
        asset=asset,
        peer_asset=peer if peer != asset else f"SRV-{rng.randint(100, 999)}",
        domain=rng.choice(DOMAINS),
        cve=rng.choice(CVES),
    )


def render(template: str, entities: EntitySet) -> str:
    return template.format(
        user=entities.user,
        asset=entities.asset,
        peer_asset=entities.peer_asset,
        domain=entities.domain,
        cve=entities.cve,
    )


def add_node(nodes: list[dict], key: str, path: str, content: str, tags: list[str], metadata: dict) -> None:
    nodes.append(
        {
            "key": key,
            "path": path,
            "content": content,
            "tags": tags,
            "metadata": metadata,
        }
    )


def add_link(links: list[dict], seen: set[tuple[str, str, str]], src: str, dst: str, link_type: str, weight: float, rationale: str) -> None:
    edge = (src, dst, link_type)
    if src == dst or edge in seen:
        return
    seen.add(edge)
    links.append(
        {
            "from": src,
            "to": dst,
            "type": link_type,
            "weight": round(weight, 3),
            "rationale": rationale,
        }
    )


def generate(target_nodes: int, seed: int) -> dict:
    if target_nodes < 120:
        raise ValueError("--nodes must be at least 120 for realistic coverage")

    rng = random.Random(seed)
    base_time = datetime(2026, 1, 7, 8, 30, 0)
    nodes: list[dict] = []
    links: list[dict] = []
    seen_edges: set[tuple[str, str, str]] = set()

    campaign_count = max(6, min(18, target_nodes // 70))
    events_per_campaign = max(8, min(16, (target_nodes * 55 // 100) // campaign_count))

    campaign_stage_keys: dict[str, list[str]] = {}
    campaign_hypothesis_keys: dict[str, list[str]] = {}
    all_alert_keys: list[str] = []
    all_evidence_keys: list[str] = []
    all_hypothesis_keys: list[str] = []

    for campaign_idx in range(campaign_count):
        campaign = CAMPAIGN_NAMES[campaign_idx % len(CAMPAIGN_NAMES)]
        actor = rng.choice(ACTORS)
        entities = make_entities(rng)
        campaign_key = f"campaign_{campaign_idx:02d}_{slug(campaign)}"
        campaign_root = f"root.realistic_graph.campaign.campaign_{campaign_idx:02d}_{slug(campaign)}"
        add_node(
            nodes,
            campaign_key,
            campaign_root,
            f"Incident campaign {campaign} attributed to {actor}; primary asset {entities.asset}, user {entities.user}, domain {entities.domain}.",
            ["graph", "realistic", "campaign", slug(actor)],
            {
                "kind": "campaign",
                "campaign": campaign,
                "actor": actor,
                "primary_asset": entities.asset,
                "primary_user": entities.user,
                "start_time": (base_time + timedelta(days=campaign_idx * 4)).isoformat() + "Z",
            },
        )

        stage_keys: list[str] = []
        tactics = list(TACTICS)
        start = rng.randint(0, 2)
        selected = tactics[start : start + events_per_campaign]
        for stage_idx, tactic in enumerate(selected):
            technique, template = STAGES[tactic]
            event_time = base_time + timedelta(days=campaign_idx * 4, hours=stage_idx * rng.randint(2, 9))
            key = f"{campaign_key}_{stage_idx:02d}_{tactic}"
            severity = rng.choices(("P1", "P2", "P3", "P4"), weights=(10, 25, 45, 20), k=1)[0]
            if tactic in {"exfiltration", "impact", "credential_access", "lateral_movement"}:
                severity = rng.choices(("P1", "P2", "P3"), weights=(30, 50, 20), k=1)[0]
            add_node(
                nodes,
                key,
                f"{campaign_root}.{stage_idx:02d}_{tactic}",
                f"{render(template, entities)} Source={rng.choice(SOURCES)}. Severity={severity}.",
                ["graph", "realistic", "alert", tactic, technique.lower().replace(".", "_")],
                {
                    "kind": "alert",
                    "campaign": campaign,
                    "actor": actor,
                    "disposition": "true_positive",
                    "severity": severity,
                    "mitre_tactic": tactic,
                    "mitre_technique": technique,
                    "asset": entities.asset,
                    "user": entities.user,
                    "domain": entities.domain,
                    "detected_at": event_time.isoformat() + "Z",
                    "source": rng.choice(SOURCES),
                },
            )
            stage_keys.append(key)
            all_alert_keys.append(key)

            if stage_idx == 0:
                add_link(links, seen_edges, campaign_key, key, "causal", 0.88, "Campaign root caused or motivated the first observed alert.")
            else:
                add_link(links, seen_edges, stage_keys[stage_idx - 1], key, "temporal", 0.92, "Observed in chronological order within the same incident.")
                if tactic in {"execution", "lateral_movement", "impact", "exfiltration"}:
                    add_link(links, seen_edges, campaign_key, key, "causal", 0.78, "Campaign context explains this downstream alert.")

            # Evidence notes are separate memories that support or occasionally contradict alerts.
            evidence_count = 2 if rng.random() < 0.65 else 1
            for evidence_idx in range(evidence_count):
                ev_key = f"{key}_evidence_{evidence_idx}"
                all_evidence_keys.append(ev_key)
                evidence_kind = rng.choice(("edr", "network", "identity", "ticket", "forensics"))
                add_node(
                    nodes,
                    ev_key,
                    f"{campaign_root}.evidence.{stage_idx:02d}.{evidence_idx}",
                    f"{evidence_kind.upper()} evidence for {tactic}: asset={entities.asset}, user={entities.user}, domain={entities.domain}. Analyst note #{evidence_idx + 1}.",
                    ["graph", "realistic", "evidence", evidence_kind],
                    {
                        "kind": "evidence",
                        "campaign": campaign,
                        "supports_alert": key,
                        "evidence_type": evidence_kind,
                        "confidence": round(rng.uniform(0.62, 0.98), 2),
                    },
                )
                add_link(links, seen_edges, ev_key, key, "supports", rng.uniform(0.65, 0.98), "Evidence note supports the alert assessment.")

        campaign_stage_keys[campaign_key] = stage_keys

        hypothesis_keys: list[str] = []
        for hyp_idx, label in enumerate(("cacheable_root_cause", "credential_compromise", "benign_admin_activity")):
            hyp_key = f"{campaign_key}_hypothesis_{hyp_idx}_{label}"
            all_hypothesis_keys.append(hyp_key)
            hypothesis_keys.append(hyp_key)
            is_wrong = label == "benign_admin_activity"
            add_node(
                nodes,
                hyp_key,
                f"{campaign_root}.hypothesis.{hyp_idx}_{label}",
                (
                    f"Hypothesis {label} for campaign {campaign}: "
                    f"{'likely wrong after evidence review' if is_wrong else 'still plausible based on current evidence'}."
                ),
                ["graph", "realistic", "hypothesis", label],
                {
                    "kind": "hypothesis",
                    "campaign": campaign,
                    "status": "rejected" if is_wrong else "active",
                    "confidence": round(rng.uniform(0.3 if is_wrong else 0.58, 0.55 if is_wrong else 0.91), 2),
                },
            )
            target = rng.choice(stage_keys)
            add_link(
                links,
                seen_edges,
                hyp_key,
                target,
                "contradicts" if is_wrong else "supports",
                rng.uniform(0.45, 0.9),
                "Hypothesis is compared against observed alert evidence.",
            )
        add_link(links, seen_edges, hypothesis_keys[0], hypothesis_keys[1], "related", 0.5, "Root-cause hypotheses share the same campaign context.")
        add_link(links, seen_edges, hypothesis_keys[1], hypothesis_keys[0], "related", 0.5, "Bidirectional analyst review cycle.")
        campaign_hypothesis_keys[campaign_key] = hypothesis_keys

        post_key = f"{campaign_key}_postmortem"
        add_node(
            nodes,
            post_key,
            f"{campaign_root}.postmortem",
            f"Postmortem for {campaign}: containment actions, root cause, blast radius, and control gaps.",
            ["graph", "realistic", "postmortem"],
            {"kind": "postmortem", "campaign": campaign, "owner": rng.choice(("IR", "SOC", "Threat Hunting"))},
        )
        add_link(links, seen_edges, stage_keys[-1], post_key, "causal", 0.82, "Final incident stage led to postmortem.")
        for hyp_key in hypothesis_keys[:2]:
            add_link(links, seen_edges, hyp_key, post_key, "supports", 0.64, "Accepted hypotheses inform postmortem conclusions.")

    # Cross-campaign related links: shared actors, domains, techniques.
    campaign_keys = list(campaign_stage_keys)
    for idx, key in enumerate(campaign_keys):
        nxt = campaign_keys[(idx + 1) % len(campaign_keys)]
        add_link(
            links,
            seen_edges,
            rng.choice(campaign_stage_keys[key]),
            rng.choice(campaign_stage_keys[nxt]),
            "related",
            rng.uniform(0.35, 0.7),
            "Cross-campaign link based on shared technique, actor, or infrastructure.",
        )

    # Real SOC noise: false positives, benign true positives, duplicates, alert storms.
    remaining = target_nodes - len(nodes)
    noise_idx = 0
    while remaining > 0:
        entities = make_entities(rng)
        disposition = rng.choices(DISPOSITIONS, weights=(10, 45, 20, 25), k=1)[0]
        tactic = rng.choice(TACTICS)
        technique, template = STAGES[tactic]
        key = f"noise_{noise_idx:04d}_{disposition}_{tactic}"
        path = f"root.realistic_graph.noise.{disposition}.{noise_idx:04d}_{tactic}"
        cause = ""
        if disposition == "false_positive":
            cause = rng.choice(FALSE_POSITIVE_CAUSES)
        elif disposition == "benign_true_positive":
            cause = rng.choice(BENIGN_CAUSES)
        content = render(template, entities)
        if cause:
            content = f"{content} Triage result: {cause}."
        else:
            content = f"{content} Triage result: duplicate or low-confidence alert requiring correlation."
        add_node(
            nodes,
            key,
            path,
            content,
            ["graph", "realistic", "noise", disposition, tactic],
            {
                "kind": "alert",
                "campaign": "uncorrelated",
                "disposition": disposition,
                "severity": rng.choice(("P2", "P3", "P4")),
                "mitre_tactic": tactic,
                "mitre_technique": technique,
                "asset": entities.asset,
                "user": entities.user,
                "domain": entities.domain,
                "false_positive_cause": cause if disposition == "false_positive" else "",
                "benign_cause": cause if disposition == "benign_true_positive" else "",
            },
        )
        all_alert_keys.append(key)
        remaining -= 1

        if disposition == "duplicate" and all_alert_keys:
            add_link(links, seen_edges, key, rng.choice(all_alert_keys), "related", rng.uniform(0.4, 0.8), "Duplicate alert linked to original correlated memory.")
        elif disposition == "false_positive" and all_hypothesis_keys:
            add_link(links, seen_edges, key, rng.choice(all_hypothesis_keys), "contradicts", rng.uniform(0.35, 0.7), "False-positive triage contradicts active incident hypothesis.")
        elif disposition == "benign_true_positive" and all_evidence_keys:
            add_link(links, seen_edges, key, rng.choice(all_evidence_keys), "supports", rng.uniform(0.35, 0.75), "Benign true positive still supports an observed control signal.")
        elif rng.random() < 0.35:
            add_link(links, seen_edges, key, rng.choice(all_alert_keys), "related", rng.uniform(0.25, 0.55), "Low-confidence alert shares weak context with another memory.")
        noise_idx += 1

    # Add additional analyst review cycles and sparse weak relations without making the graph a hairball.
    for _ in range(max(20, target_nodes // 12)):
        src_pool = all_hypothesis_keys + all_evidence_keys
        dst_pool = all_alert_keys + all_evidence_keys
        if src_pool and dst_pool:
            link_type = rng.choices(LINK_TYPES, weights=(8, 10, 34, 14, 34), k=1)[0]
            add_link(
                links,
                seen_edges,
                rng.choice(src_pool),
                rng.choice(dst_pool),
                link_type,
                rng.uniform(0.25, 0.82),
                "Additional analyst correlation added during case review.",
            )

    counts = Counter(link["type"] for link in links)
    metadata = {
        "seed": seed,
        "node_count": len(nodes),
        "link_count": len(links),
        "campaign_count": campaign_count,
        "link_type_counts": dict(sorted(counts.items())),
        "generated_at": "2026-06-16T00:00:00Z",
    }
    return {
        "name": "cognitive-graph-realistic",
        "description": "Large realistic SOC/incident-response graph for PCMI Cognitive Graph testing and demos.",
        "metadata": metadata,
        "nodes": nodes,
        "links": links,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--nodes", type=int, default=1200, help="number of nodes to generate, minimum 120")
    parser.add_argument("--seed", type=int, default=4242, help="deterministic RNG seed")
    parser.add_argument("--output", default="graph_realistic_large.json", help="output JSON path")
    args = parser.parse_args()

    dataset = generate(args.nodes, args.seed)
    output = Path(args.output)
    output.write_text(json.dumps(dataset, indent=2, sort_keys=False) + "\n", encoding="utf-8")
    print(
        f"wrote {output} with {len(dataset['nodes'])} nodes and {len(dataset['links'])} links "
        f"(seed={args.seed})"
    )


if __name__ == "__main__":
    main()
