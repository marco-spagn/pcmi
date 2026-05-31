# SOC incident graph — example dataset

**Optional demo data** for the [Cognitive Graph Explorer](../../docs/cognitive-graph.md#graph-ui--demo-video).

PCMI is **domain-agnostic**. This folder is only a realistic **security-operations (SOC)** *story* so the Graph UI looks busy out of the box. PCMI does **not** classify incidents: the generator and loader **pretend** an analyst already triaged alerts and typed the graph edges; your production integration would send the same `POST /v1/memories` and `POST /v1/memories/links` calls with **your** metadata and link types.

**Required reading (no SOC background needed):** [How data enters PCMI — who classifies what](../../docs/cognitive-graph.md#how-data-enters-pcmi--who-classifies-what).

| File | Purpose |
|------|---------|
| [`soc_incidents_nodes.csv`](soc_incidents_nodes.csv) | 1000 triaged alert/incident rows (graph nodes) |
| [`soc_incidents_links.csv`](soc_incidents_links.csv) | ~1333 typed `memory_links` (by `external_id`) |
| [`generate_soc_dataset.py`](generate_soc_dataset.py) | Deterministic generator (seed 1337) |
| [`validate.py`](validate.py) | Integrity and coherence checks |
| [`load_to_pcmi.py`](load_to_pcmi.py) | Batch loader into PCMI (`id_map.json` checkpoint) |
| [`data-dictionary.md`](data-dictionary.md) | Column reference (33 node fields + links) |
| [`example-scenarios.md`](example-scenarios.md) | Five patterns illustrated with real rows from the CSVs |

## Quick use

From the repository root (with API up and AGE enabled):

```bash
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
cd examples/soc-incident-graph
python3 validate.py
python3 load_to_pcmi.py --batch 50 --link-workers 16   # resumable; Ctrl+C safe
```

Or one command: `make graph-ui` (runs [`scripts/e2e/launch_graph_ui.sh`](../../scripts/e2e/launch_graph_ui.sh) against this directory).

## Regenerate CSVs

```bash
cd examples/soc-incident-graph
python3 generate_soc_dataset.py        # 1000 nodes (default)
python3 generate_soc_dataset.py 5000   # custom size
python3 validate.py
```

Loader artifacts (`id_map.json`, `*.log`) are gitignored — keep them local.
