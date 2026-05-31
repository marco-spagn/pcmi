# SOC incident graph — example dataset

**Optional demo data** for the [Cognitive Graph Explorer](../../docs/cognitive-graph.md#graph-ui--demo-video). PCMI itself is domain-agnostic; this folder is only a realistic **security-operations** sample (alerts, kill chains, campaigns) so you can try `make graph-ui` without loading your own memories.

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
