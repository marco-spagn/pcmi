import csv, os, re, sys
from collections import Counter

ROOT = os.path.dirname(os.path.abspath(__file__))
errors=[]; LTREE=re.compile(r'^root(\.[a-z0-9_]+)+$')
IPRE=re.compile(r'^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$')
DISP={"true_positive","false_positive","benign_true_positive","duplicate"}
SEV={"P1","P2","P3","P4"}
nodes={}; paths=set()
with open(os.path.join(ROOT, "soc_incidents_nodes.csv"), encoding="utf-8") as f:
    for row in csv.DictReader(f):
        eid=row["external_id"]
        if eid in nodes: errors.append(f"dup external_id {eid}")
        p=row["path"]
        if p in paths: errors.append(f"dup path {p}")
        paths.add(p)
        if not LTREE.match(p): errors.append(f"bad ltree {p}")
        if row["disposition"] not in DISP: errors.append(f"bad disposition {row['disposition']} {eid}")
        if row["severity"] not in SEV: errors.append(f"bad sev {eid}")
        if ',' in row["tags"]: errors.append(f"comma in tags {eid}")
        m=IPRE.match(row["src_ip"])
        if not m or any(int(o)>255 for o in m.groups()): errors.append(f"bad ip {row['src_ip']} {eid}")
        if not re.match(r'^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$', row["detected_at"]): errors.append(f"bad ts {eid}")
        if row["disposition"]=="false_positive" and not row["fp_cause"]: errors.append(f"FP senza fp_cause {eid}")
        if row["disposition"]=="benign_true_positive" and not row["benign_cause"]: errors.append(f"benign senza causa {eid}")
        if row["disposition"]!="false_positive" and row["fp_cause"]: errors.append(f"fp_cause su non-FP {eid}")
        if row["category"]=="ransomware" and row["disposition"] in("false_positive","benign_true_positive"):
            errors.append(f"ransomware FP/benign incoerente {eid}")
        if row["first_seen"]>row["detected_at"]: errors.append(f"first_seen>detected {eid}")
        nodes[eid]=row
LT={"causal","temporal","contradicts","supports","related"}
dang=self_=dup=0; seen=set(); lc=0
with open(os.path.join(ROOT, "soc_incidents_links.csv"), encoding="utf-8") as f:
    for row in csv.DictReader(f):
        lc+=1; a,b,t=row["from_external_id"],row["to_external_id"],row["link_type"]
        if t not in LT: errors.append(f"bad link_type {t}")
        if a==b: self_+=1
        if a not in nodes or b not in nodes: dang+=1
        k=(a,b,t)
        if k in seen: dup+=1
        seen.add(k)
if self_: errors.append(f"{self_} self-loops")
if dang: errors.append(f"{dang} dangling")
if dup: errors.append(f"{dup} dup edges")
print("="*60); print("VALIDATION (realistic v2)"); print("="*60)
print("nodes:",len(nodes)," unique paths:",len(paths)," links:",lc)
print("self-loops:",self_," dangling:",dang," dup-edges:",dup)
print("disposition:",dict(Counter(n['disposition'] for n in nodes.values())))
print("severity:",dict(Counter(n['severity'] for n in nodes.values())))
print("status:",dict(Counter(n['status'] for n in nodes.values())))
print("escalation_tier:",dict(Counter(n['escalation_tier'] for n in nodes.values())))
print("link_types:",dict(Counter(t for(_,_,t) in seen)))
print("tactics:",len(set(n['mitre_tactic'] for n in nodes.values())),"/14")
fp=Counter(n['fp_cause'] for n in nodes.values() if n['fp_cause'])
print("top FP causes:",fp.most_common(6))
conn=set()
for(a,b,_) in seen: conn.add(a); conn.add(b)
print(f"connected: {len(conn)} ({100*len(conn)//len(nodes)}%)  isolated: {len(nodes)-len(conn)}")
print()
if errors:
    print(f"❌ {len(errors)} ERRORS"); [print("  -",e) for e in errors[:15]]; sys.exit(1)
print("✅ NO ERRORS — coerente e pronto al caricamento")
