# Full CTI dataset · threat-actor graph demo

Multi-vendor cyber threat intelligence under `root.cti.*`:

- SOC / TI hub incidents
- Vendor reports (CrowdStrike, Mandiant, Microsoft, …)
- Operational STIX IOCs (CISA MAR BRICKSTORM, APT28 samples, …)

## Quick start (graph UI tour)

If CTI memories are already loaded in PCMI:

```bash
python3 examples/full-cti-dataset/launch_cti_graph_demo.py --autostart
```

Or from the repo root:

```bash
make demo-cti-graph-ui
```

API key: `testkey123` · URL pattern: `/v1/graph/ui?demo=cti&…`

## Resolve tour memory IDs

```bash
PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123 \
  python3 examples/full-cti-dataset/resolve_demo_ids.py
```

## Cross-vendor correlation (retrieval)

After embeddings are ready, hybrid retrieval connects vendor naming (e.g. PRESSURE CHOLLIMA ↔ Sapphire Sleet):

```bash
curl -s -H 'X-API-Key: testkey123' -H 'Content-Type: application/json' \
  -d '{"path_prefix":"root.cti","query":"PRESSURE CHOLLIMA Sapphire Sleet Bybit","limit":10}' \
  http://localhost:8000/v1/retrieve | jq '.entries[:5] | .[] | {id, path, score}'
```

## Reload dataset

Historical loaders (`load_multi_cti.py`, STIX build scripts) may live on branch `feat/cognitive-graph-ui-v2` or in local backups. If `root.cti` has fewer than ~100 memories, reload from that branch before running the tour.
