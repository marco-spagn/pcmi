---
title: "Threat Intelligence Report: FortiBleed — Massive Fortinet Credential Compromise Campaign"
date: "2026-06-20"
severity: "CRITICAL"
classification: "TLP:WHITE"
description: "Cross-source analysis of the FortiBleed campaign exposing 73,000+ Fortinet FortiGate firewall credentials via brute-force, hash cracking, and configuration exfiltration across 194 countries."
tags:
  - fortinet
  - fortigate
  - credential-compromise
  - vpn
  - initial-access-broker
  - brute-force
  - hash-cracking
  - hashtopolis
  - lateral-movement
  - active-directory
  - russian-speaking
  - critical-infrastructure
sources_count: 9
author: "TI Mindmap HUB"
---

# 🛡️ Threat Intelligence Report: FortiBleed — Massive Fortinet Credential Compromise Campaign

---

## 1. Source Reports Table

| # | Title | Publication Date | Source | Platform Link |
|---|-------|-----------------|--------|---------------|
| 1 | FortiBleed (2026): The Compromise of 80,000+ Fortinet Firewalls and Credential Leak | 2026-06-19 | SOCRadar | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/35048609-8512-45a9-a23b-856145bd1ab9) |
| 2 | FortiBleed: 75,000 Fortinet Firewalls Compromised: Global Enterprises Exposed | 2026-06-19 | Hudson Rock | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/3868cba5-3262-4dbb-a828-d865f9a63aa2) |
| 3 | Active FortiBleed Campaign Impacting Fortinet Devices Across 194 Countries | 2026-06-19 | Arctic Wolf | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/8066f0f9-da4d-4683-a5b1-82a43562b30b) |
| 4 | FortiBleed Campaign Exposing Credentials for 73,932 FortiGate Systems | 2026-06-19 | Recorded Future | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/a250846f-4175-43a8-826f-7d8fdca229ba) |
| 5 | More Than a Leak: What SpyCloud Found Inside the FortiBleed Threat Actor Infrastructure | 2026-06-20 | SpyCloud | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/d4b7155c-a4b9-4449-b10a-d6c9b465bb97) |
| 6 | Inside the FortiBleed Open Directory: A Technical Analysis of What the Attacker Left Behind | 2026-06-20 | CloudSEK | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/264c71de-3fdd-4c39-92d0-339f60ee3f64) |
| 7 | FortiBleed: Fortinet Credentials Exposed, Italian Public Sector Also Impacted | 2026-06-20 | CERT-AGID | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/75740157-d717-4f02-a35b-afaf3071b633) |
| 8 | FortiBleed: Inside the 73,000-Firewall Credential Leak | 2026-06-20 | SecurityWall | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/6888d19a-61ac-47c8-9a06-f3bff97287b6) |
| 9 | Major Security Event: Fortinet VPN Credentials and Configuration Data Exposed for 73,000 Devices | 2026-06-20 | Bitsight | [TI Mindmap HUB](https://ti-mindmap-hub.com/report/2b1747a2-3782-4046-9e6b-46704bd31dc3) |

---

## 2. Executive Summary

### Overview

On June 17, 2026, security researcher Volodymyr "Bob" Diachenko disclosed the existence of **FortiBleed** — a massive credential compromise campaign targeting Fortinet FortiGate firewalls and SSL VPN gateways worldwide. The dataset, which surfaced on underground forums and was subsequently confirmed by multiple independent threat intelligence firms, contains valid administrator and SSL VPN credentials for approximately **73,932 unique FortiGate device URLs** spanning **194 countries** and over **21,600 domains**. This represents roughly **50% of all internet-facing FortiGate firewalls globally**.

FortiBleed is **not a single zero-day vulnerability**, but rather the culmination of a long-running, multi-pronged credential-harvesting operation. A Russian-speaking threat group, operating as an Initial Access Broker (IAB) under the alias **"SantaAd"**, executed over **1.16 billion credential attempts** against FortiGate devices and **2.1 billion brute-force attempts** against Microsoft SQL Server systems. The attackers intercepted SSL VPN authentication hashes and systematically cracked them using a distributed **45-GPU cluster** managed via Hashtopolis, yielding plaintext credentials at industrial scale.

The campaign's verified victims include **Fortune Global 500 companies**, government agencies, defense contractors (including a Turkish NATO defense contractor from which **105 GB of classified military data** was exfiltrated), critical infrastructure operators, hospitals, universities, and multinational corporations — including Foxconn, Samsung, Comcast, Siemens, Lenovo, PwC, Accenture, and Oracle.

The exposure window remains **ongoing** as of June 20, 2026, with new victims continually added to the attacker's validated credential database. Many affected organizations remain unaware of compromise because the credential theft originated from exported configuration files — meaning **no login event was logged** on the device itself during the initial hash extraction.

### Diagram: Attack Flow Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     FORTIBLEED CAMPAIGN ATTACK FLOW                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. RECONNAISSANCE & SCANNING                                               │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ Internet-wide scanning for exposed FortiGate management          │       │
│  │ interfaces and SSL VPN portals (ports 443, 4443, 8443, 10443)    │       │
│  │ Also targeting: Synology NAS, Sophos firewalls, MSSQL servers    │       │
│  └──────────────────────────────┬───────────────────────────────────┘       │
│                                 │                                           │
│  2. CREDENTIAL HARVESTING       ▼                                           │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ ┌────────────────┐  ┌─────────────────┐  ┌───────────────────┐  │       │
│  │ │ Brute-Force    │  │ Configuration   │  │ Infostealer Logs  │  │       │
│  │ │ 1.16B attempts │  │ File Export     │  │ (prior leaks)     │  │       │
│  │ │ against 320K   │  │ via exposed     │  │ + Credential      │  │       │
│  │ │ FortiGate URLs │  │ mgmt interfaces │  │   Stuffing        │  │       │
│  │ └───────┬────────┘  └────────┬────────┘  └────────┬──────────┘  │       │
│  │         │                    │                     │             │       │
│  └─────────┼────────────────────┼─────────────────────┼─────────────┘       │
│            │                    │                     │                      │
│  3. HASH CRACKING              ▼                     │                      │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ Hashtopolis distributed GPU cluster (45 GPUs)                     │       │
│  │ ─ SHA-256 with Salt (legacy FortiOS hashing) → cracked offline   │       │
│  │ ─ SSL VPN authentication hashes → intercepted & cracked          │       │
│  │ ─ Kerberos/NTLM hashes from AD → post-compromise cracking       │       │
│  └──────────────────────────────┬───────────────────────────────────┘       │
│                                 │                                           │
│  4. VALIDATION & ENRICHMENT     ▼                                           │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ ─ Verify every credential before adding to database               │       │
│  │ ─ Enrich with business intelligence (revenue, sector, geography)  │       │
│  │ ─ Filter out honeypots (clean_honeypots.py)                       │       │
│  │ ─ Package validated access for underground market sale            │       │
│  └──────────────────────────────┬───────────────────────────────────┘       │
│                                 │                                           │
│  5. POST-EXPLOITATION           ▼                                           │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ ┌──────────────────┐  ┌───────────────────┐  ┌────────────────┐ │       │
│  │ │ Deploy network   │  │ AD enumeration    │  │ Exfiltration   │ │       │
│  │ │ sniffers on FW   │  │ (ad_enum.py)      │  │ via SMB/DFS    │ │       │
│  │ │ (fg_capture.log) │  │ Password spray    │  │ (105 GB from   │ │       │
│  │ │ → feed creds     │  │ (spray_admin.sh)  │  │  Turkish MoD)  │ │       │
│  │ │   back to system │  │ impacket tools    │  │                │ │       │
│  │ └──────────────────┘  └───────────────────┘  └────────────────┘ │       │
│  └──────────────────────────────────────────────────────────────────┘       │
│                                                                             │
│  6. MONETIZATION                                                            │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ ─ Sale on underground forums (sorted by revenue/sector)           │       │
│  │ ─ IAB marketplace listings ("SantaAd" persona)                    │       │
│  │ ─ Ransomware precursor access                                     │       │
│  │ ─ Espionage-driven selective intrusions                           │       │
│  └──────────────────────────────────────────────────────────────────┘       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Attribution & Threat Actor

**Primary Attribution:** A **Russian-speaking, multi-operator cybercriminal group** operating as an Initial Access Broker (IAB) under the alias **"SantaAd"** (per SpyCloud Labs). Key attribution indicators:

- **Language:** Russian-language comments in code, Russian-language Telegram channels for coordination
- **Infrastructure:** Multi-server setup with operational separation (brute-force server, operator workstation, Hashtopolis cracking cluster)
- **AI-Assisted Tooling:** Operators built custom tooling using AI code editors (Cursor) and agentic pentesting frameworks (CyberStrike), enabling rapid scaling
- **Operational Maturity:** Credential databases enriched with business intelligence (company revenue, industry vertical), honeypot filtering, and systematic victim prioritization
- **Targeting Bias:** Disproportionate focus on NATO-adjacent states (India, Ukraine, Poland, United States), government sectors, and high-revenue enterprises
- **Motive:** Primarily **financially motivated** (IAB operations, credential sales), with a **geopolitical element** in targeting NATO defense contractors and government agencies

**Operational Profile:**

| Attribute | Detail |
|-----------|--------|
| Alias | SantaAd |
| Language | Russian-speaking |
| Type | Initial Access Broker (IAB) |
| Motivation | Financial (credential sales) + Geopolitical (NATO targeting) |
| Scale | 1.16B FortiGate attempts + 2.1B MSSQL attempts |
| Infrastructure | Multi-server (brute-force, cracking, jumpbox, operator workstation) |
| AI Usage | Cursor IDE, CyberStrike framework, Telegram-controlled cracking bots |

**Note (CloudSEK Divergence):** CloudSEK's analysis provides a notable counterpoint to the headline impact figures. Their investigation of the attacker's open directory revealed that while ~73,932 device entries exist, only **918 organizations** showed evidence of internal Kerberos traffic capture, and just **148** had confirmed cracked Active Directory credentials. This suggests the actual number of deep compromises is significantly lower than headline figures, though the credential exposure itself remains critical.

---

## 3. Technical Details

### 3.1 Root Cause: Legacy Password Hashing in FortiOS

The campaign's success is fundamentally enabled by a **legacy password hashing weakness** in FortiOS. Historically, FortiGate devices stored administrator credentials using **SHA-256 with Salt** — a mechanism vulnerable to offline GPU-accelerated cracking. While Fortinet introduced PBKDF2-based hashing in newer FortiOS versions (7.2.11, 7.4.8, 7.6.1), a critical gap remains:

> **Existing administrator credentials are NOT automatically re-hashed upon firmware upgrade.** They remain stored as SHA-256 until each administrator actively logs in post-upgrade to trigger credential re-hashing.

This means even organizations running current FortiOS versions may retain crackable legacy hashes in their configuration files. The attackers exploited this gap systematically.

### 3.2 Attack Infrastructure (per SpyCloud & CloudSEK)

The attacker's operational infrastructure was inadvertently exposed when their backend server was left publicly accessible. This revealed the complete attack pipeline:

**Server Architecture:**

| Server Role | Description |
|-------------|-------------|
| **Brute-Force Server** | Mass login attempts against FortiGate, Sophos, Synology NAS, MSSQL endpoints |
| **Operator Workstation** | Code development, hands-on intrusion, session replay attacks (7 Kali VMs) |
| **Hashtopolis Server** | Distributed password cracking coordination (45 GPUs via rented cloud instances) |
| **Jump Box** | Staging server for relay into compromised victim networks |

**Tooling Discovered:**

| Tool | Purpose |
|------|---------|
| `Hashtopolis 0.14.3` | Distributed GPU hash-cracking orchestration |
| `impacket` | AD enumeration, Kerberos/NTLM hash extraction |
| `OpenConnect` | VPN session cookie replay for authentication bypass |
| `ad_enum.py` | Active Directory LDAP enumeration |
| `ad_full_audit.py` | Full AD audit and credential extraction |
| `spray_admin.sh` / `spray_da.py` | Kerberos and SMB password spraying |
| `backup_dfs.py` / `backup_dfs2.py` | SMB/DFS data exfiltration |
| `spider.py` / `smb_test.py` | SMB share enumeration and spider |
| `clean_honeypots.py` | Honeypot filtering from credential database |
| `bot.py` | Telegram-controlled cracking bot ("Cracker v10") |
| `fg_capture.log` | Network sniffer output for credential interception |
| `Chisel` | SOCKS5/HTTP tunneling for lateral movement |
| `Neo-reGeorg` | HTTP tunneling web shell for persistence |

### 3.3 Credential Harvesting Methodology

The attackers employed four parallel credential acquisition techniques:

1. **Mass Brute-Force:** 1.16 billion credential attempts against 320,777 FortiGate targets using curated wordlists from prior Fortinet leaks and infostealer databases
2. **Configuration File Export:** Direct extraction of `config system admin` blocks from exposed management interfaces, yielding SHA-256 hashed credentials for offline cracking
3. **SSL VPN Hash Interception:** Deploying network sniffers on compromised firewalls to capture authentication hashes from legitimate user sessions in transit
4. **Credential Stuffing:** Using plaintext credentials from infostealer malware logs (Raccoon, RedLine, Vidar) against Fortinet interfaces

### 3.4 Post-Exploitation — Case Study: Turkish Defense Contractor

SpyCloud's analysis revealed the most severe confirmed intrusion:

- Attackers used stolen FortiGate session cookies (via `OpenConnect`) to bypass VPN authentication
- Pivoted into internal Active Directory environment using `impacket`
- Exfiltrated Group Policy data, live Kerberos tickets, and NTLM hashes
- Accessed file server over SMB using recovered administrator account
- **Exfiltrated 105 GB (12,000+ files) of sensitive military data** including technical and operational documents related to military systems deployed in Ukraine

### 3.5 Affected CVEs (Historical, Enabling Access)

While FortiBleed is not a zero-day vulnerability, the campaign was amplified by organizations failing to patch known Fortinet CVEs:

| CVE | Description | Relevance |
|-----|-------------|-----------|
| CVE-2022-40684 | Authentication bypass on administrative interface | Enabled config file extraction without credentials |
| CVE-2023-27997 | Heap buffer overflow in SSL VPN (XORtigate) | Remote code execution on FortiGate |
| CVE-2024-55591 | Authentication bypass via Node.js websocket | Admin access without credentials |
| CVE-2026-24858 | (New) Referenced in SecurityWall report | Exploited in conjunction with FortiBleed campaign |

### 3.6 Victimology & Scale

| Metric | Value |
|--------|-------|
| Unique device URLs | 73,932 |
| Unique domains | 21,600+ |
| Countries affected | 194 |
| Verified working credentials | 86,644+ |
| % of internet-facing FortiGate | ~50% |
| Credential attempts (FortiGate) | 1.16 billion |
| Credential attempts (MSSQL) | 2.1 billion |
| Data exfiltrated (Turkish MoD) | 105 GB |
| Organizations with confirmed AD compromise | 148 (CloudSEK estimate) |
| Organizations with Kerberos capture evidence | 918 (CloudSEK estimate) |

**Top Affected Countries:** India, United States, Ukraine, Poland, Turkey, Germany, Italy

**Top Affected Sectors:** Government, Telecommunications, IT Services, Financial Services, Healthcare, Manufacturing, Education, Defense

**Named Victims (per Hudson Rock):** Foxconn, Samsung, Comcast, Siemens, Lenovo, PwC, Accenture, Oracle, Chevron, plus multiple government ministries and NATO-affiliated defense contractors.

---

## 4. Detection Opportunities

### 4.1 Immediate Device-Level Checks (HIGHEST PRIORITY)

- **Audit administrator accounts:** Look for unexpected accounts (especially `support`, `admin2`, `fortimanager`, or any unrecognized names)
- **Check password hashing:** Verify all admin credentials use PBKDF2 (`set password-hash pbkdf2`) — if SHA-256 hashes remain, credentials are compromised
- **Review firewall policies:** Look for wide-open cross-zone traversal rules or recently modified policies
- **Inspect SSL VPN sessions:** Alert on active sessions from unexpected geographies, especially outside business hours
- **Check for network sniffers:** Look for unauthorized packet capture configurations on FortiGate interfaces

### 4.2 Network & Authentication Monitoring

- Monitor authentication logs for anomalous admin login patterns (unusual IPs, times, geographies)
- Alert on mass failed login attempts against FortiGate management interfaces
- Detect unauthorized configuration exports or backup operations
- Monitor for tunneling tool signatures (Chisel, Neo-reGeorg) in network traffic
- Alert on SSH, VNC, or RDP connections from FortiGate device IPs to internal resources

### 4.3 Active Directory Monitoring

- Monitor for password spraying patterns against domain controllers
- Alert on abnormal Kerberos ticket requests or NTLM authentication from unexpected sources
- Detect `impacket` tool signatures in network traffic (DCE/RPC patterns)
- Monitor for mass LDAP enumeration queries
- Alert on new service accounts or administrator account creation
- Check for unauthorized Group Policy modifications

### 4.4 SIEM/XDR Detection Rules

```yaml
# Detect FortiGate Management Access from Attacker Infrastructure
title: FortiGate Admin Access from Known FortiBleed Infrastructure
status: experimental
description: Detects administrative access to FortiGate devices from IPs associated with FortiBleed campaign
logsource:
  product: fortinet
  service: event
detection:
  selection:
    srcip:
      - '85.11.187.8'
      - '85.11.187.28'
      - '193.8.187.2'
      - '185.229.26.83'
      - '213.169.49.142'
      - '38.117.87.37'
      - '198.53.64.194'
      - '175.155.64.221'
    action: 'login'
  condition: selection
level: critical
tags:
  - attack.initial_access
  - attack.t1078
  - fortibleed
```

```yaml
# Detect Mass Authentication Failures Indicative of Brute-Force
title: FortiGate Mass Authentication Failure - Potential FortiBleed
status: experimental
description: Detects brute-force patterns against FortiGate admin/VPN interfaces
logsource:
  product: fortinet
  service: event
detection:
  selection:
    action: 'login'
    status: 'failed'
  condition: selection | count(srcip) by dstip > 100
  timeframe: 1h
level: high
tags:
  - attack.credential_access
  - attack.t1110
  - fortibleed
```

```yaml
# Detect Credential Spraying Against Active Directory Post-FortiBleed Compromise
title: Kerberos Password Spray from Network Perimeter Device
status: experimental
description: Detects password spraying from FortiGate/VPN IP ranges targeting AD
logsource:
  product: windows
  service: security
detection:
  selection:
    EventID: 4771
  condition: selection | count(TargetUserName) by IpAddress > 25
  timeframe: 10m
level: high
tags:
  - attack.credential_access
  - attack.t1110.003
  - fortibleed
```

### 4.5 FortiOS CLI Verification Commands

```bash
# Check for legacy SHA-256 hashes (VULNERABLE)
config system admin
  get | grep -i "password"
end

# Verify PBKDF2 enforcement
config system global
  get | grep "admin-password-hash"
end

# List all admin accounts
diagnose sys admin list

# Check for unauthorized SSL VPN sessions
get vpn ssl monitor

# Review recent configuration changes
execute log filter category event
execute log filter field action "config-change"
execute log display
```

### 4.6 Detection Limitations

- **No login event logged:** Credentials extracted from configuration file exports do not trigger authentication log entries on the device
- **Offline cracking invisible:** Hash cracking occurs entirely on attacker infrastructure — no network indicator during cracking phase
- **Legacy devices may lack logging:** Older FortiOS versions may not log administrative actions comprehensively
- **Session cookie replay:** OpenConnect-based VPN session replay may appear as legitimate reconnection

---

## 5. Conclusion

FortiBleed represents one of the most significant perimeter credential compromise operations ever documented. Key takeaways across all 9 analyzed sources:

1. **Scale Without Precedent:** The campaign compromised credentials for ~50% of all internet-facing FortiGate firewalls globally, affecting 194 countries and sectors ranging from government and defense to healthcare and finance.

2. **Not a Zero-Day — Worse:** FortiBleed exploits a fundamental architectural weakness (legacy password hashing persisting after firmware upgrades) combined with widespread poor credential hygiene. Patching alone does not remediate the threat — organizations must force credential re-hashing and rotation.

3. **IAB-as-a-Service Model:** The "SantaAd" threat group demonstrates the maturation of Initial Access Broker operations into fully automated, AI-enhanced enterprises. Their infrastructure reveals complete end-to-end automation from scanning through credential validation to marketplace listing.

4. **Confirmed Espionage Impact:** The 105 GB military data exfiltration from a Turkish NATO defense contractor elevates FortiBleed beyond financially motivated cybercrime into the geopolitical threat domain.

5. **Diverging Impact Assessments:** While headline figures cite 73,000+ compromised devices, CloudSEK's forensic analysis of the attacker's database suggests that confirmed deep network compromises (with AD credential recovery) number closer to 148 organizations. However, all sources agree that any organization with credentials in the dataset should assume compromise.

6. **Ongoing Campaign:** Unlike point-in-time breaches, FortiBleed is a **live, evolving operation** with new victims added continuously. The credential feed-back loop (compromised firewalls sniffing additional credentials) creates a self-reinforcing cycle of compromise.

**Recommended Immediate Actions:**

- **Rotate ALL FortiGate admin and SSL VPN credentials immediately** — regardless of patch status
- **Force PBKDF2 re-hashing** by requiring all administrators to log in after upgrading to FortiOS 7.2.11+/7.4.8+/7.6.1+
- **Enforce MFA** on all administrative and VPN interfaces
- **Restrict management interface exposure** — remove all internet-facing admin panels; use out-of-band management
- **Hunt for compromise indicators** — unauthorized accounts, sniffers, tunneling tools, AD enumeration evidence
- **Check exposure databases** — use Hudson Rock's Fortinet Look-Up portal or contact SOCRadar to verify inclusion in the FortiBleed dataset
- **Assume compromise** if any credential matches exist — initiate full incident response including AD password resets and forensic investigation

---

## 6. Indicators of Compromise (IoC List)

### 6.1 Network Indicators — Attacker Infrastructure

| Type | Value | Description | Confidence |
|------|-------|-------------|------------|
| IPv4 | `85.11.187[.]8` | Hashtopolis coordination server / primary C2 (AS211486, HTTP port 9999, SSH/VNC/RDP attacks) | HIGH |
| IPv4 | `85.11.187[.]28` | Credential harvesting server (FortiGate credential collection & management) | HIGH |
| IPv4 | `193.8.187[.]2` | Jump box — staging server for relay into compromised networks | HIGH |
| IPv4 | `185.229.26[.]83` | Hashtopolis GPU worker instance (distributed hash cracking) | HIGH |
| IPv4 | `213.169.49[.]142` | Hashtopolis GPU worker instance (distributed hash cracking) | HIGH |
| IPv4 | `38.117.87[.]37` | Hashtopolis GPU worker instance (distributed hash cracking) | HIGH |
| IPv4 | `198.53.64[.]194` | Hashtopolis GPU worker instance (distributed hash cracking) | HIGH |
| IPv4 | `175.155.64[.]221` | Hashtopolis GPU worker instance (distributed hash cracking) | HIGH |

### 6.2 Tools & Artifacts (File-Based)

| Artifact | Type | Description |
|----------|------|-------------|
| `fg_capture.log` | Log File | Network sniffer output capturing FortiGate authentication credentials |
| `bot.py` | Script | Telegram-controlled cracking bot ("Cracker v10") |
| `hashpanel.log` | Log File | Hash-cracking orchestration log |
| `setup_hashcat.sh` | Script | Hashcat GPU cracking environment setup |
| `setup_hashtopolis.sh` | Script | Hashtopolis distributed cracking setup |
| `ad_enum.py` | Script | Active Directory LDAP enumeration tool |
| `ad_full_audit.py` | Script | Full AD audit and credential extraction |
| `spray_admin.sh` | Script | Admin account password spraying |
| `spray_da.py` | Script | Domain Admin password spraying (Kerberos) |
| `spray_results.txt` | Output | Successful spray results |
| `backup_dfs.py` / `backup_dfs2.py` | Script | SMB/DFS data exfiltration |
| `spider.py` | Script | SMB share spider/enumeration |
| `smb_test.py` | Script | SMB connectivity testing |
| `clean_honeypots.py` | Script | Honeypot filtering from victim database |
| `Chisel` | Tool | SOCKS5/HTTP tunnel for lateral movement |
| `Neo-reGeorg` | Tool | HTTP tunneling web shell for persistence |

### 6.3 Behavioral Indicators

| Indicator | Type | Description |
|-----------|------|-------------|
| Unexpected admin accounts | Account | Accounts named `support`, `admin2`, or other unrecognized names on FortiGate |
| Wide-open firewall policies | Configuration | Cross-zone traversal rules allowing unrestricted traffic |
| Active SSL VPN sessions from unusual geolocations | Authentication | Connections from Eastern Europe, Russia, or outside normal operational areas |
| SHA-256 password hashes in configuration | Vulnerability | Admin credentials stored using legacy hashing (not PBKDF2) |
| Config backup/export operations | Activity | Unauthorized `execute backup config` commands |
| Network sniffer enabled on interfaces | Configuration | Unauthorized packet capture on VPN or management interfaces |
| Mass failed authentication events | Brute-Force | >100 failed logins per hour against management interface |
| impacket-style DCE/RPC traffic | Lateral Movement | Kerberos/NTLM enumeration patterns from perimeter devices |
| FortiGate SSH to internal AD servers | Lateral Movement | Unexpected admin shell connections to domain controllers |

### 6.4 Related CVEs

| CVE | Description | Exploitation Status |
|-----|-------------|---------------------|
| CVE-2022-40684 | FortiOS Authentication Bypass (admin interface) | Exploited in campaign |
| CVE-2023-27997 | FortiOS Heap Buffer Overflow (SSL VPN — XORtigate) | Exploited in campaign |
| CVE-2024-55591 | FortiOS Auth Bypass via Node.js websocket | Exploited in campaign |
| CVE-2026-24858 | FortiOS (details pending full disclosure) | Referenced in campaign |

### 6.5 Underground Market Indicators

| Indicator | Type | Description |
|-----------|------|-------------|
| "SantaAd" forum account | Threat Actor | IAB persona selling validated FortiGate access on Russian forums |
| Credential listings sorted by revenue/sector | Market Activity | Datasets categorized by victim organization revenue and industry |
| Telegram channels with Fortinet credential sales | Distribution | Russian-language Telegram groups trading FortiBleed credentials |

---

## 7. MITRE ATT&CK Techniques

| Technique ID | Technique Name | Tactic | Description |
|-------------|----------------|--------|-------------|
| T1595 | Active Scanning | Reconnaissance | Internet-wide scanning for exposed FortiGate management interfaces and SSL VPN portals across ports 443, 4443, 8443, 10443 |
| T1591 | Gather Victim Org Information | Reconnaissance | Enrichment of compromised credential database with company revenue, size, industry vertical, and geographic data for prioritized monetization |
| T1587.001 | Develop Capabilities: Malware | Resource Development | AI-assisted tool development using Cursor IDE and CyberStrike framework; Telegram-controlled cracking bots |
| T1190 | Exploit Public-Facing Application | Initial Access | Exploitation of known Fortinet CVEs (CVE-2022-40684, CVE-2023-27997, CVE-2024-55591) to extract configuration files |
| T1078 | Valid Accounts | Initial Access | Use of cracked administrator and SSL VPN credentials to access FortiGate devices and pivot into internal networks |
| T1110 | Brute Force | Credential Access | 1.16 billion credential attempts against 320,777 FortiGate targets; 2.1 billion against MSSQL servers |
| T1110.002 | Brute Force: Password Cracking | Credential Access | Offline GPU-accelerated cracking of SHA-256 hashed credentials using 45-GPU Hashtopolis cluster |
| T1110.003 | Brute Force: Password Spraying | Credential Access | Post-compromise Kerberos and SMB password spraying against internal Active Directory (spray_admin.sh, spray_da.py) |
| T1552 | Unsecured Credentials | Credential Access | Extraction of plaintext and weakly hashed credentials from FortiGate configuration file exports |
| T1040 | Network Sniffing | Credential Access | Deployment of network sniffers on compromised firewalls (fg_capture.log) to capture additional authentication credentials in transit |
| T1003 | OS Credential Dumping | Credential Access | Dumping encrypted credentials from Active Directory via impacket |
| T1003.003 | OS Credential Dumping: NTDS | Credential Access | Extraction of NTLM hashes from AD domain controllers |
| T1003.004 | OS Credential Dumping: LSA Secrets | Credential Access | Kerberos pre-authentication hash capture and offline cracking |
| T1550 | Use Alternate Authentication Material | Defense Evasion | Replay of captured FortiGate VPN session cookies via OpenConnect to bypass authentication |
| T1550.003 | Use Alternate Authentication Material: Pass the Ticket | Defense Evasion | Exfiltration and reuse of live Kerberos tickets from compromised AD environments |
| T1562.002 | Impair Defenses: Disable or Modify System Logging | Defense Evasion | Disabling logging on compromised FortiGate devices to hide unauthorized access |
| T1562.004 | Impair Defenses: Disable or Modify Firewall | Defense Evasion | Manipulation of firewall rules to enable persistent attacker access and traffic interception |
| T1572 | Protocol Tunneling | Command and Control | Deployment of Chisel and Neo-reGeorg tunneling tools for persistent access and lateral movement |
| T1563 | Remote Service Session Hijacking | Lateral Movement | VPN session cookie replay to hijack authenticated sessions without password |
| T1021.002 | Remote Services: SMB/Windows Admin Shares | Lateral Movement | SMB-based access to file servers using recovered administrator credentials |
| T1087.002 | Account Discovery: Domain Account | Discovery | AD enumeration via ad_enum.py and ad_full_audit.py to identify domain administrators and high-value accounts |
| T1602 | Data from Configuration Repository | Collection | Extraction of network device configuration files containing credentials and network architecture details |
| T1119 | Automated Collection | Collection | Fully automated credential generation (1.16B combinations) and validation pipeline |
| T1074 | Data Staged | Collection | Multi-server staging of credentials, hashes, and exfiltrated data across attack infrastructure |
| T1041 | Exfiltration Over C2 Channel | Exfiltration | 105 GB of military data exfiltrated from Turkish defense contractor via SMB to C2 |
| T1048.002 | Exfiltration Over Alternative Protocol: Exfiltration Over Asymmetric Encrypted Non-C2 Protocol | Exfiltration | SMB/DFS-based data collection and exfiltration scripts |
| T1136 | Create Account | Persistence | Creation of persistent backdoor administrator accounts on compromised FortiGate devices |
| T1070 | Indicator Removal on Host | Defense Evasion | Log-clearing operations to remove evidence of unauthorized access |
| T1486 | Data Encrypted for Impact | Impact | Potential ransomware deployment leveraging compromised FortiGate credentials (documented precursor pattern) |

---

*Report generated 2026-06-20 via TI Mindmap HUB cross-source correlation. All intelligence derived from 9 open-source and vendor publications.*
