# Full CTI dataset · operational STIX + vendor reports

Multi-vendor cyber threat intelligence under `root.cti.*` for the graph UI tour (`?demo=cti`):

- SOC / TI hub incidents (`full_cti_dataset.json`)
- Vendor reports — CrowdStrike, Mandiant, Microsoft, … (`vendor_reports_cti_dataset.json`)
- Operational STIX IOCs — CISA MAR BRICKSTORM, APT28 samples, … (`operational_stix_cti_dataset.json`)

## Build operational STIX dataset

```bash
make cti-stix-build
# or:
python3 examples/full-cti-dataset/download_stix_bundles.py
python3 examples/full-cti-dataset/build_operational_stix_dataset.py
python3 examples/full-cti-dataset/validate.py
```

Output: `data/operational_stix_cti_dataset.json` — real SHA-256, YARA, IP, cross-vendor actors (PRESSURE CHOLLIMA ↔ Sapphire Sleet, PROMPTSTEAL ↔ Forest Blizzard).

## Load into PCMI

```bash
cd examples/full-cti-dataset
python3 load_multi_cti.py --reset
python3 launch_cti_graph_demo.py --autostart
```

From repo root: `make demo-cti-graph` or `make demo-cti-graph-ui` (if data already loaded).

API key: `testkey123`

## Resolve tour memory IDs

```bash
PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123 \
  python3 examples/full-cti-dataset/resolve_demo_ids.py
```

## Cross-vendor retrieval demo

```bash
make cti-cross-vendor-demo
python3 demo_entity_evolution_retrieval.py   # registry evolution + retrieval
make demo-cti-evolution-ui                   # setup + CLI demo + browser tour
```
