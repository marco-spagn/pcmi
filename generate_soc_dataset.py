#!/usr/bin/env python3
"""
PCMI · SOC Incident Graph Dataset Generator  (v2 — REALISTIC)
=============================================================
Genera N alert/incidenti SOC realistici + il grafo cognitivo (default N=1000).

Realismo modellato (come una vera coda di case-management SOC):
  - DISPOSITION:  true_positive / false_positive / benign_true_positive / duplicate
  - ENTITA' COERENTI dentro ogni incidente (stesso host/utente/IP/dominio nella narrativa)
  - CAMPAGNE multi-stadio con entita' persistenti, dwell time crescente, severity che
    sale verso gli stadi di impatto
  - FALSI POSITIVI con CAUSA RICONOSCIBILE (vuln scanner, red team, admin tooling,
    backup job, sw deployment, DoH, impossible-travel da VPN, IOC obsoleto, load test…)
  - BENIGN TRUE POSITIVE (azione reale ma autorizzata: AV disabilitato per troubleshooting…)
  - DUPLICATI e ALERT STORM (burst correlati nello stesso /24 / stessa signature)
  - severity/status/disposition COERENTI con la categoria (un ransomware non e' mai FP)
  - dwell time realistico (APT lungo, commodity breve), bias orario lavorativo
  - metriche SOC: escalation_tier, sla_met, ttd/ttr, playbook SOAR, alert_count

Output (contratto grafo invariato):
  soc_incidents_nodes.csv   (N nodi, default 1000)
  soc_incidents_links.csv   (referenzia external_id → loader traduce in memory.<db_id>)
"""
import csv, random, math, sys
from datetime import datetime, timedelta

SEED = 1337
random.seed(SEED)
TARGET = int(sys.argv[1]) if len(sys.argv) > 1 else 1000
START = datetime(2023, 1, 1)
END   = datetime(2025, 12, 31, 23, 59, 59)
SPAN  = int((END - START).total_seconds())

CATEGORIES = {
  "recon_scanning":      ("Reconnaissance",      ["T1595.001","T1595.002","T1590","T1592"], 1),
  "resource_dev":        ("Resource Development",["T1583.001","T1585.002","T1588.002"], 2),
  "phishing":            ("Initial Access",      ["T1566.001","T1566.002","T1566.003"], 3),
  "web_attack":          ("Initial Access",      ["T1190","T1133"], 3),
  "supply_chain":        ("Initial Access",      ["T1195.001","T1195.002"], 3),
  "valid_accounts":      ("Initial Access",      ["T1078.002","T1078.004"], 3),
  "malware_execution":   ("Execution",           ["T1059.001","T1059.003","T1204.002","T1053.005"], 4),
  "persistence":         ("Persistence",         ["T1547.001","T1543.003","T1136.001","T1505.003"], 5),
  "privilege_escalation":("Privilege Escalation",["T1068","T1134.001","T1548.002"], 6),
  "defense_evasion":     ("Defense Evasion",     ["T1055","T1562.001","T1070.001","T1027"], 7),
  "brute_force":         ("Credential Access",   ["T1110.001","T1110.003","T1110.004"], 8),
  "credential_theft":    ("Credential Access",   ["T1003.001","T1003.002","T1003.006","T1558.003"], 8),
  "discovery":           ("Discovery",           ["T1087.002","T1018","T1046","T1083"], 9),
  "lateral_movement":    ("Lateral Movement",    ["T1021.001","T1021.002","T1021.006","T1550.002"], 10),
  "collection":          ("Collection",          ["T1005","T1114.001","T1113"], 11),
  "c2_beaconing":        ("Command and Control", ["T1071.001","T1071.004","T1572","T1090.003"], 12),
  "data_exfiltration":   ("Exfiltration",        ["T1041","T1048.003","T1567.002"], 13),
  "ddos":                ("Impact",              ["T1498.001","T1499.002"], 14),
  "ransomware":          ("Impact",              ["T1486","T1490","T1485"], 14),
  "insider_threat":      ("Collection",          ["T1078","T1005","T1052.001"], 11),
  "cloud_misconfig":     ("Initial Access",      ["T1530","T1078.004","T1538"], 3),
}
KILL_CHAIN = ["recon_scanning","resource_dev","phishing","web_attack","valid_accounts",
  "malware_execution","persistence","privilege_escalation","defense_evasion","brute_force",
  "credential_theft","discovery","lateral_movement","collection","c2_beaconing",
  "data_exfiltration","ransomware"]

SOURCES = {
  "recon_scanning":["Suricata IDS","Zeek","Palo Alto Firewall","Cloudflare WAF"],
  "resource_dev":["Recorded Future TI","MISP","Anomali","VirusTotal"],
  "phishing":["Proofpoint","Microsoft Defender for Office365","Mimecast"],
  "web_attack":["Cloudflare WAF","Akamai WAF","ModSecurity","AWS WAF"],
  "supply_chain":["Snyk","Dependabot","Sonatype Nexus IQ","CrowdStrike Falcon"],
  "valid_accounts":["Microsoft Sentinel","Okta","Azure AD Identity Protection"],
  "malware_execution":["CrowdStrike Falcon","Trellix EDR","Defender for Endpoint","SentinelOne"],
  "persistence":["CrowdStrike Falcon","Defender for Endpoint","Wazuh","Sysmon/Splunk"],
  "privilege_escalation":["Defender for Endpoint","CrowdStrike Falcon","Sysmon/Splunk"],
  "defense_evasion":["Defender for Endpoint","CrowdStrike Falcon","Wazuh"],
  "brute_force":["Microsoft Sentinel","Splunk","Okta","Fail2ban"],
  "credential_theft":["CrowdStrike Falcon","Defender for Identity","Sysmon/Splunk"],
  "discovery":["Defender for Endpoint","Darktrace","Zeek"],
  "lateral_movement":["Defender for Identity","CrowdStrike Falcon","Darktrace","Zeek"],
  "collection":["Defender for Endpoint","Microsoft Purview DLP","Forcepoint DLP"],
  "c2_beaconing":["Darktrace","Zeek","Suricata IDS","Cisco Umbrella"],
  "data_exfiltration":["Microsoft Purview DLP","Forcepoint DLP","Netskope CASB","Zeek"],
  "ddos":["Cloudflare","Akamai Prolexic","Arbor/Netscout","Palo Alto Firewall"],
  "ransomware":["CrowdStrike Falcon","Defender for Endpoint","SentinelOne"],
  "insider_threat":["Microsoft Purview DLP","Securonix UEBA","Exabeam","Netskope CASB"],
  "cloud_misconfig":["AWS GuardDuty","Defender for Cloud","Wiz","Prisma Cloud"],
}

ACTORS_APT = ["APT28","APT29","APT41","Lazarus","Kimsuky","Mustang Panda","Sandworm","UNC2452","Turla"]
ACTORS_CRIME = ["LockBit","BlackCat","Cl0p","Conti","FIN7","TA505","Scattered Spider","Royal","Akira"]
REGIONS = ["eu_south","eu_west","eu_north","us_east","us_west","apac","latam","mea"]
CVES = {"web_attack":["CVE-2021-44228","CVE-2023-34362","CVE-2022-41040","CVE-2023-46604","CVE-2024-1709"],
        "valid_accounts":["CVE-2023-4966","CVE-2024-21887","CVE-2024-3400"],
        "privilege_escalation":["CVE-2021-34527","CVE-2022-37969","CVE-2020-1472"],
        "ransomware":["CVE-2023-27997","CVE-2023-34362"],
        "persistence":["CVE-2023-23397"]}
USERS = ["mario.rossi","luca.ferrari","giulia.bianchi","antonio.esposito","sara.ricci","paolo.romano",
         "chiara.greco","davide.conti","elena.russo","marco.lombardi","valeria.gallo","stefano.costa"]
SVC = ["svc_backup","svc_deploy","svc_sql","svc_iis","svc_scan"]
HOSTP = ["WKS","SRV","DC","DB","WEB","MAIL","FS","HV","NAS","K8S","JMP","VPN"]
MALW = ["secure-login","account-verify","update-portal","cdn-assets","telemetry-srv","ms-auth",
        "docusign-verify","invoice-pay","sharepoint-doc","drive-sync"]
TLDS = ["com","net","io","xyz","top","online","ru","cn"]
ANALYSTS = [f"analyst.{n}" for n in ["ferretti","moretti","caputo","de_luca","santoro","vitale","romano","gallo"]]
L2 = ["l2.bianchi","l2.conti","l2.greco"]; L3 = ["l3.forensics","l3.threat_hunt","ir.lead"]

def pub_ip():  return f"{random.randint(1,223)}.{random.randint(0,255)}.{random.randint(0,255)}.{random.randint(1,254)}"
def priv_ip(): return f"{random.choice(['10.10','10.20','172.16','192.168'])}.{random.randint(0,254)}.{random.randint(1,254)}"
def h64():     return ''.join(random.choices('0123456789abcdef', k=64))
def host():    return f"{random.choice(HOSTP)}-{random.randint(100,999)}"
def domain():  return f"{random.choice(MALW)}-{random.randint(100,9999)}.{random.choice(TLDS)}"

TP_TPL = {
 "recon_scanning":["Port scan da {sip} verso {dnet}: {n} porte TCP sondate in {mins} min, pattern SYN tipico di Nmap.",
   "Enumerazione servizi esposti su {dhost} da {sip}; fingerprinting versioni, nessun referrer."],
 "resource_dev":["Dominio look-alike {domain} registrato di recente, infrastruttura riconducibile a {actor}.",
   "Nuovo certificato TLS per {domain} osservato in TI; staging per campagna {actor}."],
 "phishing":["Spear-phishing a {user} con allegato {hash} dal dominio {domain}; macro VBA che lancia PowerShell su {host}.",
   "Campagna phishing O365 verso {n} mailbox; landing page clone su {domain}, {user} ha inserito credenziali da {host}."],
 "web_attack":["Exploitation {cve} su {dhost} da {sip}; webshell deployata in wwwroot, RCE come utente di servizio.",
   "SQL injection time-based su {domain} (param id) da {sip}; WAF bypassato, dump tabella utenti."],
 "supply_chain":["Pacchetto NPM malevolo in pipeline CI/CD su {host}; esfiltrazione env vars, hash {hash}.",
   "Aggiornamento vendor trojanizzato su {host}; DLL firmata anomala {hash}, C2 verso {domain}."],
 "valid_accounts":["Login con credenziali valide di {user} da {sip} (geo anomala) fuori orario; sessione su {dhost}.",
   "Accesso VPN come {user} da {sip}; credenziali da leak esterno, pivot verso {dhost}."],
 "malware_execution":["PowerShell offuscato su {host} (utente {user}); parent anomalo, stage2 scaricato da {sip}:{port}.",
   "Dropper {hash} eseguito su {host}; scheduled task creato, beacon verso {domain}."],
 "persistence":["Run key di registro creata su {host} per persistenza ({user}); binario {hash} non firmato.",
   "Nuovo servizio Windows malevolo su {host}; avvio automatico, C2 {domain}."],
 "privilege_escalation":["Escalation a SYSTEM su {host} via {cve}; token theft rilevato dopo accesso di {user}.",
   "SeImpersonate (Potato) su {host} da APPPOOL; escalation a SYSTEM, poi accesso a {dhost}."],
 "defense_evasion":["Process injection in svchost.exe su {host}; EDR eluso ~{mins} min, shellcode in memoria.",
   "Tamper protection Defender disabilitata su {host} da processo malevolo; log 4688 alterati."],
 "brute_force":["RDP brute-force su {dhost} da {sip}: {n} tentativi, 1 successo su {user}.",
   "Password spraying Azure AD da {sip}/24: {n} tentativi su molti account, {user} compromesso."],
 "credential_theft":["Dump LSASS via Mimikatz su {host} ({user}); estratti {n} hash NTLM incl. 1 Domain Admin.",
   "Kerberoasting da {host}: {n} richieste TGS; hash service account {user} craccato offline."],
 "discovery":["Ricognizione AD (BloodHound) da {host} ({user}); enumerati {n} oggetti, mapping percorsi a DA.",
   "Network discovery da {host}: scansione SMB/LDAP verso {dnet}."],
 "lateral_movement":["Lateral via SMB/PsExec da {host} verso {dhost} e altri {n} server; PSEXESVC creato.",
   "Pass-the-Hash da {host}: hash di {user} usato su {n} host (Impacket wmiexec)."],
 "collection":["Staging dati su {host}: archivio creato in temp prima dell'exfiltration da parte di {user}.",
   "Export massivo mailbox di {user} su {host}: {n} email negli ultimi 90 giorni."],
 "c2_beaconing":["Beacon C2 da {host} verso {domain} ({sip}:{port}) ogni {mins} min, jitter regolare HTTPS.",
   "Tunnel DNS verso {domain}: {n} query/h da {host}, dati nei subdomain."],
 "data_exfiltration":["Exfiltration {gb}GB da {host} verso {domain} via HTTPS; DLP allertato in ritardo.",
   "Esfiltrazione {records} record PII da {host} verso storage non aziendale ({user})."],
 "ddos":["DDoS volumetrico {gbps}Gbps (UDP flood) verso {dhost}; BGP blackhole + scrubbing.",
   "DDoS L7 HTTP/2 verso {dhost}; {n}k req/s, WAF in mitigazione."],
 "ransomware":["Ransomware {actor} su {host}: {records} file cifrati; vettore RDP da {sip}, backup verificati.",
   "Doppia estorsione {actor} su {host}: {gb}GB esfiltrati prima della cifratura, C2 {sip}."],
 "insider_threat":["UEBA: {user} (in uscita) copia {gb}GB verso USB/cloud personale da {host}.",
   "Abuso privilegi: {user} legge {records} record clienti fuori orario senza motivo, da {host}."],
 "cloud_misconfig":["Bucket S3 pubblico con {records} record PII esposti per {days}gg; rilevato da scanner esterno.",
   "Security Group 0.0.0.0/0 su porta {port} di {dhost}; brute-force da {sip}."],
}

FP_CAUSES = [
 ("vuln_scanner","Traffico del vulnerability scanner autorizzato (Qualys/Nessus, IP {sip} in allowlist) scambiato per attacco",
   {"recon_scanning","web_attack","brute_force","discovery"}),
 ("pentest_redteam","Attivita' di penetration test / red-team pianificata e autorizzata (rules of engagement attive)",
   {"phishing","web_attack","lateral_movement","credential_theft","privilege_escalation","malware_execution","persistence","c2_beaconing"}),
 ("admin_tooling","Uso legittimo di strumenti di amministrazione (PsExec/WinRM) dal jump server da parte del team IT",
   {"lateral_movement","malware_execution","discovery","persistence"}),
 ("backup_job","Job di backup/replica (Veeam/rsync/robocopy) con grande volume in uscita verso il sito DR",
   {"data_exfiltration","collection"}),
 ("sw_deployment","Deployment software massivo (SCCM/Intune/Ansible) scambiato per propagazione malware",
   {"malware_execution","lateral_movement","persistence"}),
 ("legit_cloud_sync","Sincronizzazione cloud aziendale legittima (OneDrive/Box) classificata come exfiltration/C2",
   {"data_exfiltration","c2_beaconing"}),
 ("doh_dns","DNS-over-HTTPS / risolutore legittimo classificato erroneamente come tunneling DNS",
   {"c2_beaconing"}),
 ("geoip_vpn","Errore Geo-IP: egress da VPN aziendale ha generato un falso 'impossible travel' per {user}",
   {"valid_accounts","brute_force","credential_theft"}),
 ("new_role","Cambio ruolo/onboarding di {user}: accesso a risorse nuove interpretato come anomalia UEBA",
   {"insider_threat","valid_accounts","discovery","collection"}),
 ("stale_ioc","Match su IOC obsoleto di TI verso CDN/servizio legittimo ({domain}) — IOC ritirato dal feed",
   {"c2_beaconing","malware_execution","resource_dev"}),
 ("cert_misconfig","Errore di configurazione (certificato scaduto/SNI mismatch) su {dhost}, non un attacco",
   {"web_attack"}),
 ("loadtest","Load/stress test pianificato verso {dhost} scambiato per DDoS volumetrico",
   {"ddos"}),
 ("password_manager","Accesso massivo a credenziali dal password manager aziendale (CyberArk/1Password) da parte di {user}",
   {"credential_theft"}),
 ("monitoring_probe","Probe di monitoring/uptime (Pingdom/Nagios da {sip}) scambiato per scanning",
   {"recon_scanning","ddos"}),
 ("av_quarantine","L'EDR ha gia' messo in quarantena: alert correlato di un secondo tool sullo stesso evento gia' gestito su {host}",
   {"malware_execution","persistence","defense_evasion"}),
]
BENIGN_CAUSES = [
 ("auth_admin_action","Azione reale ma AUTORIZZATA: l'admin ha disabilitato temporaneamente l'AV su {host} per troubleshooting (change ticket approvato)",
   {"defense_evasion"}),
 ("dev_secret_rotated","Esposizione reale ma gestita: {user} ha committato un secret e l'ha ruotato entro pochi minuti",
   {"credential_theft","collection"}),
 ("net_team_nmap","Scanning reale ma AUTORIZZATO: il team di rete ha eseguito nmap da {sip} per inventory asset",
   {"recon_scanning","discovery"}),
 ("emergency_access","Accesso break-glass reale usato da {user} durante un incidente operativo (autorizzato e tracciato)",
   {"valid_accounts","privilege_escalation"}),
 ("maintenance_window","Attivita' reale durante finestra di manutenzione pianificata su {dhost} (deploy/patch notturna)",
   {"malware_execution","persistence","lateral_movement"}),
]

def applicable(catalog, cat):
    return [c for c in catalog if cat in c[2]]

SEV_BIAS = {
  "ransomware":[60,30,8,2],"data_exfiltration":[35,40,20,5],"c2_beaconing":[25,40,28,7],
  "credential_theft":[30,40,25,5],"lateral_movement":[28,42,25,5],"privilege_escalation":[25,40,28,7],
  "persistence":[15,38,37,10],"malware_execution":[20,40,32,8],"web_attack":[22,38,30,10],
  "supply_chain":[30,40,25,5],"valid_accounts":[18,37,35,10],"phishing":[10,30,40,20],
  "insider_threat":[20,38,32,10],"cloud_misconfig":[25,38,30,7],"collection":[12,33,40,15],
  "discovery":[6,24,45,25],"defense_evasion":[18,37,35,10],"ddos":[25,40,28,7],
  "brute_force":[5,20,40,35],"recon_scanning":[2,12,40,46],"resource_dev":[3,15,42,40],
}
SEV=["P1","P2","P3","P4"]
SLA_MIN={"P1":15,"P2":60,"P3":480,"P4":1440}

def dwell_minutes(cat, actor_apt):
    long_cats={"persistence","lateral_movement","c2_beaconing","data_exfiltration","credential_theft","collection","insider_threat"}
    if actor_apt and cat in long_cats:
        return int(random.expovariate(1/ (60*24*14)))+30
    if cat in long_cats:
        return random.randint(60, 60*24*5)
    return random.randint(1, 240)

def business_dt():
    base = START + timedelta(seconds=random.randint(0, SPAN))
    if random.random() < 0.65:
        base = base.replace(hour=random.randint(8,19), minute=random.randint(0,59))
    return base

def ts(dt): return dt.replace(microsecond=0).isoformat()+"Z"

def status_for(disp, sev):
    if disp=="false_positive":      return "closed_false_positive","triage: falso positivo, regola tunata"
    if disp=="benign_true_positive":return "closed_benign","attivita' reale ma autorizzata, chiusa benigna"
    if disp=="duplicate":           return "closed_duplicate","duplicato di alert correlato"
    if sev=="P1": return random.choices(["contained","investigating","escalated","resolved"],[30,25,25,20])[0], "incidente confermato"
    if sev=="P2": return random.choices(["resolved","contained","investigating","escalated"],[40,25,20,15])[0], "incidente confermato"
    if sev=="P3": return random.choices(["resolved","investigating","contained"],[65,25,10])[0], "incidente confermato"
    return random.choices(["resolved","investigating"],[85,15])[0], "incidente confermato"

nodes=[]; links=[]; seen=set(); eid_n=0
def add_link(a,b,t,why):
    if a==b: return
    k=(a,b,t)
    if k in seen: return
    seen.add(k); links.append((a,b,t,why))
def new_eid():
    global eid_n; eid_n+=1; return f"INC{eid_n:07d}"

def bundle(actor_apt, fixed=None):
    b = fixed.copy() if fixed else {}
    b.setdefault("host", host())
    b.setdefault("dhost", host())
    b.setdefault("user", random.choice(USERS+SVC))
    b.setdefault("sip", pub_ip() if random.random()<0.6 else priv_ip())
    b.setdefault("domain", domain())
    b.setdefault("hash", h64())
    b.setdefault("actor", random.choice(ACTORS_APT if actor_apt else ACTORS_CRIME))
    b.setdefault("dnet", priv_ip().rsplit('.',1)[0]+".0/24")
    b.setdefault("port", random.choice([22,80,443,445,3389,4444,5985,53,8080,8443]))
    b.setdefault("n", random.randint(3,400))
    b.setdefault("mins", random.randint(2,180))
    b.setdefault("gb", round(random.uniform(0.2,120),1))
    b.setdefault("gbps", round(random.uniform(5,350),1))
    b.setdefault("records", random.randint(50,900000))
    b.setdefault("days", random.randint(1,30))
    return b

def make(cat, dt, disp, *, bnd, actor=None, camp=None, cve=None, alert_count=1):
    eid=new_eid()
    tactic,techs,_=CATEGORIES[cat]
    sev=random.choices(SEV, SEV_BIAS[cat])[0]
    if disp in ("false_positive","benign_true_positive") and cat in ("ransomware",):
        disp="true_positive"
    if disp=="false_positive": sev=random.choices(SEV,[1,8,40,51])[0]
    if disp=="benign_true_positive": sev=random.choices(SEV,[1,10,45,44])[0]
    if disp=="duplicate": sev=random.choices(SEV,[3,15,42,40])[0]
    tech=random.choice(techs)
    actor = actor or bnd["actor"]
    apt = actor in ACTORS_APT
    cve = cve or (random.choice(CVES[cat]) if cat in CVES and random.random()<0.5 else "")

    if disp=="false_positive":
        choices=applicable(FP_CAUSES,cat)
        cause,tpl,_=random.choice(choices) if choices else ("generic_fp","Alert risultato falso positivo dopo triage e arricchimento TI ({sip})",None)
        narr=tpl.format(**bnd); fpc=cause; benc=""
    elif disp=="benign_true_positive":
        choices=applicable(BENIGN_CAUSES,cat)
        cause,tpl,_=random.choice(choices) if choices else ("authorized_activity","Attivita' reale ma autorizzata, confermata col business owner",None)
        narr=tpl.format(**bnd); fpc=""; benc=cause
    elif disp=="duplicate":
        narr=("Alert duplicato dello stesso evento su {host} ({sip}); correlato a un caso gia' aperto.").format(**bnd); fpc=""; benc=""
    else:
        narr=random.choice(TP_TPL[cat]).format(cve=cve or "n/d", **bnd); fpc=""; benc=""

    st, closure = status_for(disp, sev)
    dwell = dwell_minutes(cat, apt)
    first_seen = dt - timedelta(minutes=dwell)
    if first_seen < START: first_seen = START
    ttd = dwell
    ttr = SLA_MIN[sev]*random.choice([0.4,0.6,0.8,1.0,1.3,1.8,3.0,6.0])
    ttr = int(max(5, ttr))
    sla_met = ttr <= SLA_MIN[sev]*1.0
    tier = "L1" if disp in("false_positive","duplicate") else random.choices(["L1","L2","L3"],
            [10,55,35] if sev in("P1","P2") else [55,38,7])[0]

    region=random.choice(REGIONS)
    year="y"+str(dt.year)
    seg=["root","soc",cat,year]+([f"camp_{camp:06d}"] if camp is not None else [])+[f"inc_{eid[3:].lstrip('0') or '0'}"]
    path=".".join(seg)

    tags=["soc","incident",cat,sev,disp,"mitre_"+tactic.lower().replace(" ","_")]
    if disp=="true_positive" and not apt and actor: tags.append("actor_"+actor.lower().replace(" ","_"))
    if disp=="true_positive" and apt: tags+=["apt","actor_"+actor.lower()]
    if fpc: tags.append("fp_"+fpc)
    if cve: tags.append(cve.lower())
    tags=list(dict.fromkeys(tags))

    row={
      "external_id":eid,"path":path,
      "content":f"[{eid}] [{sev}] {narr}",
      "tags":"|".join(tags),
      "disposition":disp,"severity":sev,"status":st,"category":cat,
      "mitre_tactic":tactic,"mitre_technique":tech,
      "first_seen":ts(first_seen),"detected_at":ts(dt),
      "region":region,"data_source":random.choice(SOURCES[cat]),
      "escalation_tier":tier,
      "analyst":(random.choice(ANALYSTS) if tier=="L1" else random.choice(L2) if tier=="L2" else random.choice(L3)),
      "threat_actor":(actor if disp=="true_positive" else ("n/a" if disp!="duplicate" else "n/a")),
      "campaign_id":(f"CAMP{camp:06d}" if camp is not None else ""),
      "src_ip":bnd["sip"],"dst_host":bnd["dhost"] if cat in("web_attack","ddos","lateral_movement","cloud_misconfig","valid_accounts") else bnd["host"],
      "affected_user":bnd["user"],
      "ioc_hash":(bnd["hash"] if (disp=="true_positive" and cat in("phishing","malware_execution","ransomware","supply_chain","persistence","credential_theft")) else ""),
      "ioc_domain":(bnd["domain"] if (disp=="true_positive" and cat in("phishing","c2_beaconing","data_exfiltration","supply_chain","resource_dev")) else ""),
      "cve_id":cve,
      "fp_cause":fpc,"benign_cause":benc,"closure_reason":closure,
      "asset_criticality":random.choices(["low","medium","high","critical"],[28,40,24,8])[0],
      "confidence":("high" if disp=="true_positive" and sev in("P1","P2") else random.choices(["low","medium","high"],[25,45,30])[0]),
      "cvss_score":round(random.uniform(7.5,10.0) if sev=="P1" else random.uniform(4.0,8.5) if sev=="P2" else random.uniform(2.0,6.5),1),
      "alert_count":alert_count,"sla_met":str(sla_met).lower(),
      "ttd_minutes":ttd,"ttr_minutes":ttr,
    }
    nodes.append(row); return eid

# ════════════════════════════════════════════════════════════════════════════
# 1) CAMPAGNE TRUE-POSITIVE (~52%) — kill chain coerente, entita' persistenti
# ════════════════════════════════════════════════════════════════════════════
camp=0; actor_first={}; asset_nodes={}
CAMP_BUDGET=int(TARGET*0.52)
while len(nodes)<CAMP_BUDGET:
    camp+=1
    apt=random.random()<0.4
    actor=random.choice(ACTORS_APT if apt else ACTORS_CRIME)
    fixed={"actor":actor,"user":random.choice(USERS),"host":host(),
           "sip":pub_ip(),"domain":domain()}
    cve = random.choice(sum(CVES.values(),[])) if random.random()<0.4 else None
    L=random.randint(3,9); i0=random.randint(0,max(0,len(KILL_CHAIN)-L))
    stages=KILL_CHAIN[i0:i0+L]
    t=business_dt(); chain=[]
    for s in stages:
        t=t+timedelta(minutes=random.randint(10,3000))
        if t>END: t=END
        bnd=bundle(apt, fixed if random.random()<0.7 else None)
        if s in("lateral_movement","collection","credential_theft"): bnd["dhost"]=host()
        eid=make(s,t,"true_positive",bnd=bnd,actor=actor,camp=camp,cve=cve)
        chain.append(eid)
        asset_nodes.setdefault(fixed["host"],[]).append(eid)
        if len(nodes)>=CAMP_BUDGET: break
    for i in range(len(chain)-1):
        add_link(chain[i],chain[i+1],"causal","progressione kill-chain stesso attore/campagna")
        add_link(chain[i],chain[i+1],"temporal","stessa campagna, sequenza temporale")
    if len(chain)>=3 and random.random()<0.35 and len(nodes)<CAMP_BUDGET:
        bnd=bundle(apt,fixed)
        syn=make("collection",min(t+timedelta(hours=random.randint(2,72)),END),"true_positive",bnd=bnd,actor=actor,camp=camp)
        nodes[-1]["content"]=f"[{syn}] [P3] Postmortem campagna {actor}: ricostruiti {len(chain)} stadi, IOC consolidati, lessons learned."
        nodes[-1]["severity"]="P3"; nodes[-1]["status"]="resolved"; nodes[-1]["closure_reason"]="postmortem chiuso"; nodes[-1]["confidence"]="high"
        for e in chain: add_link(syn,e,"supports","postmortem corrobora lo stadio")
    if random.random()<0.18 and len(nodes)<CAMP_BUDGET and chain:
        bnd=bundle(apt,fixed)
        fp=make(random.choice(stages),min(t,END),"false_positive",bnd=bnd,camp=camp)
        add_link(fp,random.choice(chain),"contradicts","falso positivo iniziale vs attivita' confermata")
    if chain: actor_first.setdefault(actor,[]).append(chain[0])

# ════════════════════════════════════════════════════════════════════════════
# 2) STANDALONE + FP + BENIGN + DUPLICATI + ALERT STORM (~48%)
# ════════════════════════════════════════════════════════════════════════════
NOISE_W={"brute_force":16,"recon_scanning":12,"phishing":10,"malware_execution":9,"web_attack":7,
  "valid_accounts":6,"c2_beaconing":6,"persistence":5,"discovery":5,"credential_theft":5,
  "defense_evasion":4,"lateral_movement":4,"data_exfiltration":4,"cloud_misconfig":4,
  "collection":3,"ddos":3,"insider_threat":2,"supply_chain":2,"privilege_escalation":3,"resource_dev":2}
NC=list(NOISE_W); NW=[NOISE_W[c] for c in NC]
DISP_W=[("true_positive",30),("false_positive",42),("benign_true_positive",14),("duplicate",14)]
DC=[d for d,_ in DISP_W]; DW=[w for _,w in DISP_W]

last_by_cat={}
while len(nodes)<TARGET:
    if random.random()<0.015 and len(nodes)<TARGET-40:
        cat=random.choices(NC,NW)[0]; base=business_dt()
        net=priv_ip().rsplit('.',1)[0]
        size=random.randint(8,40); storm=[]
        for _ in range(size):
            if len(nodes)>=TARGET: break
            bnd=bundle(False); bnd["sip"]=f"{net}.{random.randint(1,254)}"
            disp=random.choices(["duplicate","false_positive","true_positive"],[55,35,10])[0]
            e=make(cat, base+timedelta(seconds=random.randint(0,900)), disp, bnd=bnd, alert_count=1)
            storm.append((e,disp))
        rep=next((e for e,d in storm if d=="true_positive"), storm[0][0])
        for e,d in storm:
            if e==rep: continue
            add_link(e,rep,"related","alert storm: stessa origine/segnatura")
            if d=="duplicate": add_link(e,rep,"contradicts","duplicato dello stesso evento")
        continue

    cat=random.choices(NC,NW)[0]
    disp=random.choices(DC,DW)[0]
    apt=random.random()<0.15
    bnd=bundle(apt)
    dt=business_dt()
    e=make(cat,dt,disp,bnd=bnd)
    if disp=="duplicate" and cat in last_by_cat:
        add_link(e,last_by_cat[cat],"related","duplicato/correlato stessa categoria")
        add_link(e,last_by_cat[cat],"contradicts","duplicato dello stesso evento")
    if disp=="true_positive":
        last_by_cat[cat]=e

for actor,eids in actor_first.items():
    random.shuffle(eids)
    for i in range(len(eids)-1):
        if random.random()<0.45: add_link(eids[i],eids[i+1],"related",f"stesso threat actor {actor}")

for a,eids in asset_nodes.items():
    eids=sorted(set(eids))
    for i in range(min(len(eids),5)-1):
        if random.random()<0.3: add_link(eids[i],eids[i+1],"related",f"stesso asset {a}")

# ════════════════════════════════════════════════════════════════════════════
# WRITE
# ════════════════════════════════════════════════════════════════════════════
FIELDS=["external_id","path","content","tags","disposition","severity","status","category",
  "mitre_tactic","mitre_technique","first_seen","detected_at","region","data_source",
  "escalation_tier","analyst","threat_actor","campaign_id","src_ip","dst_host","affected_user",
  "ioc_hash","ioc_domain","cve_id","fp_cause","benign_cause","closure_reason","asset_criticality",
  "confidence","cvss_score","alert_count","sla_met","ttd_minutes","ttr_minutes"]
with open("soc_incidents_nodes.csv","w",newline="",encoding="utf-8") as f:
    w=csv.DictWriter(f,fieldnames=FIELDS); w.writeheader()
    for r in nodes: w.writerow(r)
with open("soc_incidents_links.csv","w",newline="",encoding="utf-8") as f:
    w=csv.writer(f); w.writerow(["from_external_id","to_external_id","link_type","rationale"])
    for a,b,t,why in links: w.writerow([a,b,t,why])

from collections import Counter
print("nodes:",len(nodes)," links:",len(links)," campaigns:",camp)
print("disposition:",dict(Counter(n['disposition'] for n in nodes)))
