#!/usr/bin/env python3
"""
Build operational CTI dataset from real STIX bundles + vendor-published IOCs.

Sources (all public, no invented indicators):
  - CISA MAR BRICKSTORM STIX 2.1 bundles (downloaded via download_stix_bundles.py)
  - Microsoft DCU StealC/Amadey takedown IOCs (June 2026 blog)
  - ThreatLocker / CERT-UA PROMPTSTEAL (LAMEHUG) IOCs
  - CrowdStrike / Endor Labs / GTIG Axios npm supply chain IOCs (March 2026)
  - Wiz TraderTraitor Bybit/Safe{Wallet} infrastructure IOCs
  - CVE nodes from public TI advisories (CVE IDs only — no fabricated exploit IOCs)
"""
from __future__ import annotations

import json
import os
import sys
from datetime import datetime, timezone
from typing import Any

ROOT = os.path.dirname(os.path.abspath(__file__))
DATA_DIR = os.path.join(ROOT, "data")
RAW_DIR = os.path.join(DATA_DIR, "stix_raw")
OUT_PATH = os.path.join(DATA_DIR, "operational_stix_cti_dataset.json")

sys.path.insert(0, ROOT)
from download_stix_bundles import BUNDLES  # noqa: E402
from stix_to_pcmi import convert_operational_stix_bundle  # noqa: E402

# Microsoft Security Blog — StealC/Amadey takedown (2026-06-24)
# https://www.microsoft.com/en-us/security/blog/2026/06/24/stealc-and-amadey-breaking-down-infostealers-and-the-cybercrime-services-that-deliver-them/
STEALC_SAMPLES = [
    "8f32456359f209a63adfd24b94235e1727382ac7f7bb7f2bcaf754e721925b64",
    "0215f734867bd71c57ff5c524d8cc670be5b4f1861b2c390cf46d18784a53624",
    "2a0f053855da59b3b56812e580d7baeba59fc9493694722aa9e3f121ee3363f1",
    "977b33a9b481cf714946b7d386865cd5d284312aa5ecfa0546c197b1003e1bde",
]
AMADEY_SAMPLES = [
    ("b7d1f172ff3feafe65d47fd1cbe0cc249316371ae0e1cbe3a7c741c738b3353d", "5.87"),
    ("9383572a30ae5b76fadd0700fbd7a1aa7b05d0b6c8f9cdaef9b30a3e1f65d57d", "5.86"),
    ("5f5b25b2e35d404034d0d60975cf1ffbc6f141761ec3f4f15d6f7c6213a056f6", "5.80"),
    ("98e504cc7125b79eda5491f40b998605a05f4cd968b961aab4cce7beb074fefe", "5.78"),
    ("30cef3d3d956e83e2c50579cfbe57a49159cccbcc8b0b0422f27d55e1c401ad9", "5.77"),
    ("8cef760d11d24fc2e9bbd9f770dca5105854f7ece3b0e6948d7c8b7fdd1765ea", "5.73"),
    ("99507f18c4e61fdb109805404bf6a79ea8ce2fddc590ce48d717e97516ab7e8d", "5.70"),
    ("1246c5b89ab668c1137f377507bc3e266a98e93248382aa026610ae1e764a497", "5.65"),
    ("d43c988d6f9cb355497696b580621fb1bdb7b6ed6d90f97520ecf6da5a1a41ff", "5.64"),
    ("ca4d4c4fc3e5d5cfa922b898f2d7411f03a446dddb139ba45dfd4f8f0018b64f", "5.63"),
    ("43455f1ff4a623b783da670d052eb77eaaacb0c66a9f1e8508f802bf22e8129e", "5.60"),
]
STEALC_C2_URLS = [
    "http://polse.us/62ea47cac2534aa18f74.php",
    "http://roger99699.xyz/425f1faf4b214434b8a3.php",
    "http://bluescry.com/01f96fd710e905ca2326.php",
    "http://secure.controlpanel.asia/330311481fe14ab99814.php",
    "https://neltron-geltron.shop/e396586b99ee49d19cc3.php",
    "http://cdntestconnect.com/ed54b97a570943999715.php",
    "https://bartsen284.online/39d9612df78e45b5a4bb.php",
]
AMADEY_C2_URLS = [
    "http://goodpanelforgoodjob.com/hg8jjfSr5hy/index.php",
    "http://rebustan.top/gd7djkDveE2/index.php",
    "http://svclsc.com/ms/index.php",
    "http://microsoft-telemetry.at/cvdfnaFJBmC0/index.php",
    "http://spasopro.at/Lsge63sd3/index.php",
]

# ThreatLocker / CERT-UA — PROMPTSTEAL (LAMEHUG) SHA-256 (public IOC list)
PROMPTSTEAL_HASHES = [
    "165eaf8183f693f644a8a24d2ec138cd4f8d9fd040e8bafc1b021a0f973692dd",
    "2eb18873273e157a7244bb165d53ea3637c76087eea84b0ab635d04417ffbe1b",
    "384e8f3d300205546fb8c9b9224011b3b3cb71adc994180ff55e1e6416f65715",
    "5ab16a59b12c7c5539d9e22a090ba6c7942fbc5ab8abbc5dffa6b6de6e0f2fc6",
    "766c356d6a4b00078a0293460c5967764fcd788da8c1cd1df708695f3a15b777",
    "8013b23cb78407675f323d54b6b8dfb2a61fb40fb13309337f5b662dbd812a5d",
    "a30930dfb655aa39c571c163ada65ba4dec30600df3bf548cc48bedd0e841416",
    "a32a3751dfd4d7a0a66b7ecbd9bacb5087076377d486afdf05d3de3de3cb7555501",
    "a67465075c91bb15b81e1f898f2b773196d3711d8e1fb321a9d6647958be436b",
    "ae6ed1721d37477494f3f755c124d53a7dd3e24e98c20f3a1372f45cc8130989",
    "b3fcba809984eaffc5b88a1bcded28ac50e71965e61a66dd959792f7750b9e87",
    "b49aa9efd41f82b34a7811a7894f0ebf04e1d9aab0b622e0083b78f54fe8b466",
    "bb2836148527744b11671347d73ca798aca9954c6875082f9e1176d7b52b720f",
    "bdb33bbb4ea11884b15f67e5c974136e6294aa87459cdc276ac2eea85b1deaa3",
    "cf4d430d0760d59e2fa925792f9e2b62d335eaf4d664d02bff16dd1b522a462a",
    "d6af1c9f5ce407e53ec73c8e7187ed804fb4f80cf8dbd6722fc69e15e135db2e",
]
PROMPTSTEAL_NETWORK = {
    "domains": ["stayathomeclasses.com", "router.huggingface.co"],
    "ips": ["144.126.202.227", "107.180.50.236"],
    "urls": [
        "https://stayathomeclasses.com/",
        "https://stayathomeclasses.com/slpw/up.php",
    ],
}

# Endor Labs / CrowdStrike / GTIG — Axios npm compromise (2026-03-31)
AXIOS_IOC = {
    "malicious_packages": ["axios@1.14.1", "axios@0.30.4", "plain-crypto-js@4.2.1"],
    "samples": [
        ("5bb67e88846096f1f8d42a0f0350c9c46260591567612ff9af46f98d1b7571cd", "axios-1.14.1.tgz"),
        ("59336a964f110c25c112bcc5adca7090296b54ab33fa95c0744b94f8a0d80c0f", "axios-0.30.4.tgz"),
        ("58401c195fe0a6204b42f5f90995ece5fab74ce7c69c67a24c61a057325af668", "plain-crypto-js-4.2.1.tgz"),
        ("e10b1fa84f1d6481625f741b69892780140d4e0e7769e7491e5f4d894c2e0e09", "setup.js (Stage 1)"),
        ("92ff08773995ebc8d55ec4b8e1a225d0d1e51efa4ef88b8849d0071230c9645a", "com.apple.act.mond (macOS RAT)"),
        ("617b67a8e1210e4fc87c92d1d1da45a2f311c08d26e89b12307cf583c900d101", "Stage 2 PS1 RAT (Windows)"),
        ("fcb81618bb15edfdedfb638b4c08a2af9cac9ecfa551af135a8402bf980375cf", "ld.py (Stage 2 Linux)"),
        ("f7d335205b8d7b20208fb3ef93ee6dc817905dc3ae0c10a0b164f4e7d07121cd", "system.bat (persistence)"),
    ],
    "domains": ["sfrclak.com", "callnrwise.com"],
    "ips": ["142.11.206.73", "23.254.203.244", "23.254.167.216"],
    "urls": ["http://sfrclak.com:8000/6202033"],
}

# Wiz — TraderTraitor / PRESSURE CHOLLIMA Bybit supply chain (Feb 2025)
BYBIT_IOC = {
    "domains": ["getstockprice.com"],
    "actor": "TraderTraitor / PRESSURE CHOLLIMA (Lazarus)",
    "attribution": "DPRK state-sponsored",
    "mitre": ["T1609", "T1580", "T1578.005", "T1195.002"],
}

# Public CVE advisories referenced in TI Mindmap HUB / vendor reports (CVE ID is the IOC).
PUBLIC_CVES = [
    {
        "cve": "CVE-2026-22769",
        "cvss": 10.0,
        "product": "Dell RecoverPoint",
        "exploited_by": "UNC5221/BRICKSTORM",
        "actor": "PRC state-sponsored",
    },
    {
        "cve": "CVE-2026-21962",
        "cvss": 10.0,
        "product": "Oracle HTTP Server / WebLogic Proxy Plug-in",
        "auth_required": False,
    },
    {
        "cve": "CVE-2026-0755",
        "cvss": 9.8,
        "product": "gemini-mcp-tool",
        "exploited": True,
    },
    {
        "cve": "CVE-2025-8088",
        "cvss": None,
        "product": "WinRAR",
        "malware_delivered": ["NESTPACKER", "POISONIVY", "XWorm", "AsyncRAT"],
    },
    {
        "cve": "CVE-2025-31324",
        "cvss": None,
        "product": "SAP NetWeaver Visual Composer",
        "zero_day": True,
    },
    {
        "cve": "CVE-2025-29927",
        "cvss": 9.1,
        "product": "Next.js",
        "actor": "TeamPCP",
    },
    {
        "cve": "CVE-2026-31431",
        "cvss": 10.0,
        "product": "Linux kernel AF_ALG/algif_aead",
        "alias": "Copy Fail",
    },
    {
        "cve": "CVE-2026-43284",
        "cvss": 9.8,
        "product": "Linux kernel ESP",
        "alias": "Dirty Frag",
    },
    {
        "cve": "CVE-2026-0300",
        "cvss": 9.5,
        "product": "PAN-OS Captive Portal",
        "actor": "CL-STA-1132",
    },
]


def _load_stix_bundles() -> list[dict[str, Any]]:
    loaded = []
    for entry in BUNDLES:
        path = os.path.join(RAW_DIR, entry["filename"])
        if not os.path.isfile(path):
            print(f"  ⚠ missing {path} — run download_stix_bundles.py first", file=sys.stderr)
            continue
        with open(path, encoding="utf-8") as f:
            bundle = json.load(f)
        loaded.append({"entry": entry, "bundle": bundle})
    return loaded


def _dedup_nodes(nodes: list[dict]) -> list[dict]:
    seen: set[str] = set()
    out = []
    for n in nodes:
        k = n.get("key")
        if not k or k in seen:
            continue
        seen.add(k)
        out.append(n)
    return out


def _dedup_links(links: list[dict]) -> list[dict]:
    seen: set[tuple] = set()
    out = []
    for l in links:
        key = (l.get("from"), l.get("to"), l.get("type"))
        if key in seen:
            continue
        seen.add(key)
        out.append(l)
    return out


def build_stealc_amadey_nodes() -> tuple[list[dict], list[dict]]:
    nodes: list[dict] = []
    links: list[dict] = []
    campaign_key = "ms_dcu_stealc_amadey_2026"
    nodes.append(
        {
            "key": campaign_key,
            "path": "root.cti.vendors.microsoft.dcu.stealc_amadey_2026",
            "content": (
                "Microsoft DCU + Europol Operation Endgame takedown (June 24, 2026). "
                "Coordinated disruption of 200+ Amadey and StealC C2 domains/IPs. "
                "Amadey loader delivers StealC infostealer in cybercrime assembly-line model."
            ),
            "tags": ["infostealer", "stealc", "amadey", "microsoft-dcu", "operation-endgame", "2026-06"],
            "metadata": {
                "kind": "campaign_report",
                "vendor": "Microsoft DCU",
                "source": "MS-Security-Blog-2026-06-24",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "operation": "Operation Endgame",
                "c2_count": 200,
                "mitre_techniques": ["T1555", "T1005", "T1071.001", "T1105"],
                "confidence": "high",
            },
        }
    )
    for sha in STEALC_SAMPLES:
        key = f"stealc_sample_{sha[:12]}"
        nodes.append(
            {
                "key": key,
                "path": f"root.cti.vendors.microsoft.dcu.stealc.samples.{sha[:16]}",
                "content": f"StealC infostealer sample. SHA-256: {sha}. MaaS stealer targeting browsers, crypto wallets, messaging apps.",
                "tags": ["malware", "stealc", "infostealer", "sample", "microsoft-dcu"],
                "metadata": {
                    "kind": "malware_sample",
                    "vendor": "Microsoft DCU",
                    "source": "MS-Security-Blog-2026-06-24",
                    "cti_source": "vendor_intel",
                    "layer": "vendors",
                    "iocs": {"sha256": sha, "file_type": "PE32/PE32+"},
                    "c2": {"domains": [], "ips": [], "urls": STEALC_C2_URLS},
                    "mitre_techniques": ["T1555", "T1005", "T1071.001"],
                    "confidence": "high",
                },
            }
        )
        links.append({"from": campaign_key, "to": key, "type": "supports", "weight": 0.9, "metadata": {"cti_source": "vendor_intel"}})

    for sha, ver in AMADEY_SAMPLES:
        key = f"amadey_sample_{sha[:12]}"
        nodes.append(
            {
                "key": key,
                "path": f"root.cti.vendors.microsoft.dcu.amadey.samples.{sha[:16]}",
                "content": f"Amadey loader v{ver}. SHA-256: {sha}. Modular C++ backdoor/loader delivering StealC and other payloads.",
                "tags": ["malware", "amadey", "loader", "sample", "microsoft-dcu"],
                "metadata": {
                    "kind": "malware_sample",
                    "vendor": "Microsoft DCU",
                    "source": "MS-Security-Blog-2026-06-24",
                    "cti_source": "vendor_intel",
                    "layer": "vendors",
                    "iocs": {"sha256": sha, "file_type": "PE32", "version": ver},
                    "c2": {"domains": [], "ips": [], "urls": AMADEY_C2_URLS},
                    "mitre_techniques": ["T1105", "T1071.001", "T1059.001"],
                    "confidence": "high",
                },
            }
        )
        links.append({"from": campaign_key, "to": key, "type": "supports", "weight": 0.9, "metadata": {"cti_source": "vendor_intel"}})

    infra_key = "stealc_amadey_c2_infra"
    nodes.append(
        {
            "key": infra_key,
            "path": "root.cti.vendors.microsoft.dcu.stealc_amadey.c2_infrastructure",
            "content": "StealC and Amadey C2 URLs seized in June 2026 DCU takedown. PHP panel endpoints on compromised/registrar domains.",
            "tags": ["infrastructure", "c2", "stealc", "amadey", "microsoft-dcu"],
            "metadata": {
                "kind": "network_indicator",
                "vendor": "Microsoft DCU",
                "source": "MS-Security-Blog-2026-06-24",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "iocs": {"urls": STEALC_C2_URLS + AMADEY_C2_URLS},
                "c2": {"urls": STEALC_C2_URLS + AMADEY_C2_URLS},
                "confidence": "high",
            },
        }
    )
    links.append({"from": campaign_key, "to": infra_key, "type": "supports", "weight": 0.95, "metadata": {"cti_source": "vendor_intel"}})
    return nodes, links


def build_promptsteal_nodes() -> tuple[list[dict], list[dict]]:
    nodes: list[dict] = []
    links: list[dict] = []
    campaign_key = "apt28_promptsteal_2025"
    nodes.append(
        {
            "key": campaign_key,
            "path": "root.cti.vendors.google_gtig.apt28.promptsteal",
            "content": (
                "PROMPTSTEAL (CERT-UA: LAMEHUG) — first malware using LLM APIs at runtime. "
                "APT28/FROZENLAKE queries Hugging Face Qwen2.5-Coder-32B-Instruct to generate "
                "Windows recon/exfil commands executed blindly. Target: Ukraine defense sector."
            ),
            "tags": ["malware", "promptsteal", "lamehug", "apt28", "ai-malware", "frozenlake"],
            "metadata": {
                "kind": "campaign_report",
                "vendor": "Google GTIG / CERT-UA",
                "source": "GTIG-AI-Threat-Tracker-2025",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "actor": "APT28 / FROZENLAKE",
                "attribution": "Russia GRU Unit 26165",
                "mitre_techniques": ["T1059.001", "T1102", "T1071.001", "T1567"],
                "llm_model": "Qwen2.5-Coder-32B-Instruct",
                "llm_platform": "Hugging Face",
                "confidence": "high",
            },
        }
    )
    for sha in PROMPTSTEAL_HASHES:
        key = f"promptsteal_sample_{sha[:12]}"
        nodes.append(
            {
                "key": key,
                "path": f"root.cti.vendors.google_gtig.apt28.promptsteal.samples.{sha[:16]}",
                "content": f"PROMPTSTEAL/LAMEHUG Python sample. SHA-256: {sha}. Masquerades as image generation tool; queries LLM for dynamic attack commands.",
                "tags": ["malware", "promptsteal", "lamehug", "apt28", "sample", "python"],
                "metadata": {
                    "kind": "malware_sample",
                    "vendor": "ThreatLocker/CERT-UA",
                    "source": "ThreatLocker-IOC-2025",
                    "cti_source": "vendor_intel",
                    "layer": "vendors",
                    "iocs": {"sha256": sha, "file_type": "Python"},
                    "c2": PROMPTSTEAL_NETWORK,
                    "actor": "APT28 / FROZENLAKE",
                    "mitre_techniques": ["T1059.006", "T1102", "T1071.001"],
                    "confidence": "high",
                },
            }
        )
        links.append({"from": campaign_key, "to": key, "type": "supports", "weight": 0.88, "metadata": {"cti_source": "vendor_intel"}})
    return nodes, links


def build_axios_nodes() -> tuple[list[dict], list[dict]]:
    nodes: list[dict] = []
    links: list[dict] = []
    campaign_key = "stardust_chollima_axios_2026"
    nodes.append(
        {
            "key": campaign_key,
            "path": "root.cti.vendors.crowdstrike.stardust_chollima.axios_npm_2026",
            "content": (
                "Axios npm supply chain compromise (March 31, 2026). STARDUST CHOLLIMA (UNC1069) "
                "hijacked maintainer credentials, published axios@1.14.1 and axios@0.30.4 with "
                "malicious plain-crypto-js@4.2.1 postinstall dropper deploying WAVESHAPER.V2/ZshBucket RAT."
            ),
            "tags": ["supply-chain", "npm", "axios", "stardust-chollima", "dprk", "2026-03"],
            "metadata": {
                "kind": "campaign_report",
                "vendor": "CrowdStrike / Google GTIG / Endor Labs",
                "source": "CrowdStrike-STARDUST-CHOLLIMA-Axios-2026-03-31",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "actor": "STARDUST CHOLLIMA / UNC1069",
                "attribution": "DPRK-nexus",
                "mitre_techniques": ["T1195.002", "T1059.007", "T1071.001"],
                "malicious_packages": AXIOS_IOC["malicious_packages"],
                "confidence": "high",
            },
        }
    )
    for sha, label in AXIOS_IOC["samples"]:
        key = f"axios_sample_{sha[:12]}"
        nodes.append(
            {
                "key": key,
                "path": f"root.cti.vendors.crowdstrike.stardust_chollima.axios.samples.{sha[:16]}",
                "content": f"Axios npm compromise artifact: {label}. SHA-256: {sha}.",
                "tags": ["malware", "supply-chain", "npm", "axios", "sample"],
                "metadata": {
                    "kind": "malware_sample",
                    "vendor": "Endor Labs",
                    "source": "EndorLabs-Axios-Compromise-2026-03-31",
                    "cti_source": "vendor_intel",
                    "layer": "vendors",
                    "iocs": {"sha256": sha, "file_name": label},
                    "c2": {
                        "domains": AXIOS_IOC["domains"],
                        "ips": AXIOS_IOC["ips"],
                        "urls": AXIOS_IOC["urls"],
                    },
                    "actor": "STARDUST CHOLLIMA",
                    "confidence": "high",
                },
            }
        )
        links.append({"from": campaign_key, "to": key, "type": "supports", "weight": 0.9, "metadata": {"cti_source": "vendor_intel"}})

    infra_key = "axios_c2_infra"
    nodes.append(
        {
            "key": infra_key,
            "path": "root.cti.vendors.crowdstrike.stardust_chollima.axios.c2",
            "content": "Axios npm compromise C2: sfrclak.com (142.11.206.73) serving platform-specific RAT payloads on port 8000.",
            "tags": ["infrastructure", "c2", "axios", "stardust-chollima"],
            "metadata": {
                "kind": "network_indicator",
                "vendor": "CrowdStrike / Huntress",
                "source": "CrowdStrike-STARDUST-CHOLLIMA-Axios-2026-03-31",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "iocs": {
                    "domains": AXIOS_IOC["domains"],
                    "ips": AXIOS_IOC["ips"],
                    "urls": AXIOS_IOC["urls"],
                },
                "confidence": "high",
            },
        }
    )
    links.append({"from": campaign_key, "to": infra_key, "type": "supports", "weight": 0.95, "metadata": {"cti_source": "vendor_intel"}})
    return nodes, links


def build_bybit_nodes() -> tuple[list[dict], list[dict]]:
    nodes: list[dict] = []
    links: list[dict] = []
    campaign_key = "tradertraitor_bybit_2025"
    nodes.append(
        {
            "key": campaign_key,
            "path": "root.cti.vendors.wiz.tradertraitor.bybit_safe_wallet_2025",
            "content": (
                "Bybit $1.5B theft (Feb 21, 2025). TraderTraitor/PRESSURE CHOLLIMA compromised "
                "Safe{Wallet} developer workstation, injected malicious JS into AWS S3 frontend, "
                "manipulated multisig transaction to upgrade Bybit cold wallet contract."
            ),
            "tags": ["supply-chain", "crypto", "bybit", "safe-wallet", "tradertraitor", "dprk", "2025-02"],
            "metadata": {
                "kind": "campaign_report",
                "vendor": "Wiz / FBI / Sygnia",
                "source": "Wiz-TraderTraitor-Deep-Dive",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "actor": BYBIT_IOC["actor"],
                "attribution": BYBIT_IOC["attribution"],
                "mitre_techniques": BYBIT_IOC["mitre"],
                "loss_usd": 1_500_000_000,
                "confidence": "high",
            },
        }
    )
    infra_key = "bybit_c2_getstockprice"
    nodes.append(
        {
            "key": infra_key,
            "path": "root.cti.vendors.wiz.tradertraitor.bybit.infrastructure",
            "content": "TraderTraitor C2 domain getstockprice.com registered early Feb 2025; used during Safe{Wallet} developer compromise.",
            "tags": ["infrastructure", "c2", "bybit", "tradertraitor"],
            "metadata": {
                "kind": "network_indicator",
                "vendor": "Wiz",
                "source": "Wiz-TraderTraitor-Deep-Dive",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "iocs": {"domains": BYBIT_IOC["domains"]},
                "c2": {"domains": BYBIT_IOC["domains"]},
                "actor": BYBIT_IOC["actor"],
                "confidence": "high",
            },
        }
    )
    links.append({"from": campaign_key, "to": infra_key, "type": "supports", "weight": 0.9, "metadata": {"cti_source": "vendor_intel"}})
    return nodes, links


def build_cross_vendor_actor_nodes() -> tuple[list[dict], list[dict]]:
    """Microsoft threat-intel naming for actors tracked under different aliases by CrowdStrike/Mandiant."""
    nodes: list[dict] = []
    links: list[dict] = []

    sapphire_key = "ms_threat_intel_sapphire_sleet"
    nodes.append(
        {
            "key": sapphire_key,
            "path": "root.cti.vendors.microsoft.threat_intel.sapphire_sleet",
            "content": (
                "Sapphire Sleet (Microsoft Threat Intelligence): North Korean state-sponsored threat actor. "
                "Sophisticated macOS intrusion campaign using social engineering and user-driven execution "
                "to bypass macOS protections and steal credentials, cryptocurrency wallets, and sensitive data. "
                "Same DPRK-nexus umbrella as CrowdStrike PRESSURE CHOLLIMA and TraderTraitor: includes the "
                "February 2025 Bybit $1.5 billion Safe{Wallet} supply-chain theft via compromised developer macOS "
                "workstation and trojanized front-end JavaScript."
            ),
            "tags": ["sapphire-sleet", "dprk", "macos", "crypto", "microsoft", "cross-vendor"],
            "metadata": {
                "kind": "threat_actor_activity",
                "vendor": "Microsoft",
                "source": "Microsoft Security blog 2026",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "actor": "Sapphire Sleet",
                "attribution": "DPRK state-sponsored",
                "vendor_aliases": [
                    "PRESSURE CHOLLIMA",
                    "TraderTraitor",
                    "Lazarus Group",
                    "FAMOUS CHOLLIMA",
                ],
                "crowdstrike_equivalent": "PRESSURE CHOLLIMA",
                "mitre_techniques": ["T1195.002", "T1059.007", "T1609", "T1578.005"],
                "targets": ["cryptocurrency exchanges", "fintech", "macOS developers"],
                "confidence": "high",
            },
        }
    )

    forest_key = "ms_threat_intel_forest_blizzard"
    nodes.append(
        {
            "key": forest_key,
            "path": "root.cti.vendors.microsoft.threat_intel.forest_blizzard",
            "content": (
                "Forest Blizzard (Microsoft Threat Intelligence): Russian military intelligence (GRU) threat actor. "
                "Public aliases APT-28, Fancy Bear, Sofacy. Since August 2024 increased password spray attacks "
                "against NATO member states' air traffic control providers; previously targeted Ukrainian aviation (2023). "
                "Mandiant/Google tracks the same intrusion set as FROZENLAKE and attributes PROMPTSTEAL (CERT-UA: LAMEHUG) "
                "LLM-assisted malware — first nation-state malware querying Hugging Face APIs at runtime against Ukraine."
            ),
            "tags": ["forest-blizzard", "apt28", "frozenlake", "russia", "gru", "microsoft", "cross-vendor"],
            "metadata": {
                "kind": "threat_actor_activity",
                "vendor": "Microsoft",
                "source": "Microsoft Threat Intelligence 2025",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "actor": "Forest Blizzard",
                "attribution": "Russia GRU Unit 26165",
                "vendor_aliases": ["APT28", "FROZENLAKE", "Fancy Bear", "Sofacy", "UAC-0001"],
                "mandiant_equivalent": "APT28 / FROZENLAKE",
                "malware_families": ["PROMPTSTEAL", "LAMEHUG"],
                "mitre_techniques": ["T1110.003", "T1078", "T1059.006", "T1102"],
                "targets": ["NATO ATC providers", "Ukrainian aviation", "Ukraine government"],
                "confidence": "high",
            },
        }
    )

    # Cross-vendor links (same actor, different vendor naming).
    cross_links = [
        (sapphire_key, "tradertraitor_bybit_2025", "Sapphire Sleet (MS) = PRESSURE CHOLLIMA/TraderTraitor (CS/Wiz). DPRK crypto theft."),
        (forest_key, "apt28_promptsteal_2025", "Forest Blizzard (MS) = FROZENLAKE (Mandiant). PROMPTSTEAL is their LLM malware."),
    ]
    for src, dst, rationale in cross_links:
        links.append(
            {
                "from": src,
                "to": dst,
                "type": "related",
                "weight": 0.85,
                "rationale": rationale,
                "metadata": {"cti_source": "vendor_intel", "cross_vendor": True},
            }
        )
        links.append(
            {
                "from": dst,
                "to": src,
                "type": "related",
                "weight": 0.85,
                "rationale": rationale,
                "metadata": {"cti_source": "vendor_intel", "cross_vendor": True},
            }
        )

    return nodes, links


def build_midnight_blizzard_node() -> list[dict]:
    return [
        {
            "key": "midnight_blizzard_oauth_2024",
            "path": "root.cti.vendors.microsoft.midnight_blizzard.oauth_2024",
            "content": (
                "Midnight Blizzard (APT29) OAuth/token abuse campaign against Microsoft corporate email (2024). "
                "CISA ED 24-02: exfiltrated FCEB agency correspondence; actors reuse secrets for lateral cloud access. "
                "No public STIX bundle — techniques and remediation from CISA directive."
            ),
            "tags": ["apt29", "midnight-blizzard", "oauth", "token-abuse", "russia", "2024"],
            "metadata": {
                "kind": "campaign_report",
                "vendor": "Microsoft / CISA",
                "source": "CISA-ED-24-02",
                "cti_source": "vendor_intel",
                "layer": "vendors",
                "actor": "Midnight Blizzard / APT29 / COZY BEAR",
                "attribution": "Russia SVR",
                "mitre_techniques": ["T1078", "T1098.003", "T1528", "T1114"],
                "confidence": "high",
                "note": "No public STIX IOC bundle; OAuth app audit required",
            },
        }
    ]


def build_cve_nodes() -> list[dict]:
    nodes = []
    for cve in PUBLIC_CVES:
        cid = cve["cve"].lower().replace("-", "_")
        key = cid
        cvss = cve.get("cvss")
        product = cve.get("product", "")
        content = f"{cve['cve']}: {product}"
        if cvss:
            content += f", CVSS {cvss}"
        if cve.get("alias"):
            content += f" ({cve['alias']})"
        if cve.get("exploited_by"):
            content += f". Exploited by {cve['exploited_by']}."
        nodes.append(
            {
                "key": key,
                "path": f"root.cti.vulnerabilities.{cid}",
                "content": content,
                "tags": ["cve", "vulnerability", cve["cve"].lower()],
                "metadata": {
                    "kind": "vulnerability",
                    "vendor": "Public TI advisories",
                    "source": "TI-Mindmap-HUB-vendor-reports",
                    "cti_source": "vendor_intel",
                    "layer": "vendors",
                    "iocs": {"cve": cve["cve"]},
                    "cve": cve["cve"],
                    "cvss": cvss,
                    "product": product,
                    **{k: v for k, v in cve.items() if k not in ("cve", "cvss", "product")},
                    "confidence": "high",
                },
            }
        )
    return nodes


def build_dataset() -> dict[str, Any]:
    all_nodes: list[dict] = []
    all_links: list[dict] = []

    # 1. CISA STIX bundles (primary — real IOCs from downloaded JSON)
    for item in _load_stix_bundles():
        entry = item["entry"]
        converted = convert_operational_stix_bundle(
            item["bundle"],
            mar_id=entry["id"],
            vendor=entry["vendor"],
            campaign=entry["campaign"],
        )
        all_nodes.extend(converted["nodes"])
        all_links.extend(converted["links"])
        print(f"  STIX {entry['id']}: {len(converted['nodes'])} nodes, {len(converted['links'])} links")

    # 2. Supplemental vendor-published IOC sets (no STIX bundle available)
    for builder in (
        build_stealc_amadey_nodes,
        build_promptsteal_nodes,
        build_axios_nodes,
        build_bybit_nodes,
    ):
        n, l = builder()
        all_nodes.extend(n)
        all_links.extend(l)

    all_nodes.extend(build_midnight_blizzard_node())
    all_nodes.extend(build_cve_nodes())

    cv_nodes, cv_links = build_cross_vendor_actor_nodes()
    all_nodes.extend(cv_nodes)
    all_links.extend(cv_links)

    # Cross-link BRICKSTORM CVE node
    cve_brick = "cve_2026_22769"
    for n in all_nodes:
        if n.get("key", "").startswith("brickstorm_sample_"):
            all_links.append(
                {
                    "from": n["key"],
                    "to": cve_brick,
                    "type": "related",
                    "weight": 0.8,
                    "rationale": "BRICKSTORM campaign exploited CVE-2026-22769 (Dell RecoverPoint)",
                    "metadata": {"cti_source": "vendor_intel"},
                }
            )

    nodes = _dedup_nodes(all_nodes)
    links = _dedup_links(all_links)
    keys = {n["key"] for n in nodes}
    links = [l for l in links if l.get("from") in keys and l.get("to") in keys]

    stix_nodes = sum(1 for n in nodes if (n.get("metadata") or {}).get("cti_source") == "stix")
    vendor_nodes = len(nodes) - stix_nodes

    return {
        "name": "Operational CTI Dataset — Real STIX IOCs + Vendor Intelligence (2025-2026)",
        "description": (
            "Dataset operativo con IOC reali estratti da bundle STIX 2.1 CISA MAR (BRICKSTORM) "
            "e IOC pubblicati da Microsoft DCU, Google GTIG, CrowdStrike, Endor Labs, Wiz, ThreatLocker. "
            "Nessun IOC inventato. Ogni nodo malware/infrastructure include hash, IP, domini o CVE concreti."
        ),
        "metadata": {
            "version": "1.0.0",
            "generated": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "pcmi_path_namespace": "root.cti",
            "stix_bundles": [e["filename"] for e in BUNDLES],
            "node_counts": {
                "total": len(nodes),
                "from_stix": stix_nodes,
                "from_vendor_intel": vendor_nodes,
            },
            "link_count": len(links),
        },
        "nodes": nodes,
        "links": links,
    }


def main() -> int:
    print("Building operational STIX CTI dataset…")
    dataset = build_dataset()
    os.makedirs(DATA_DIR, exist_ok=True)
    with open(OUT_PATH, "w", encoding="utf-8") as f:
        json.dump(dataset, f, indent=2, ensure_ascii=False)
    mc = dataset["metadata"]["node_counts"]
    print(f"✓ wrote {OUT_PATH}")
    print(f"  nodes={mc['total']} (stix={mc['from_stix']}, vendor={mc['from_vendor_intel']})")
    print(f"  links={dataset['metadata']['link_count']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
