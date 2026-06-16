# Realistic Cognitive Graph dataset

Large deterministic dataset for realistic Cognitive Graph testing and demos.

Unlike `examples/cognitive-graph-test-matrix`, which is intentionally small and exhaustive, this dataset models a real SOC / incident-response knowledge graph:

- Multi-stage campaigns with campaign roots, alerts, evidence notes, hypotheses, and postmortems.
- True positives, false positives, benign true positives, duplicates, alert storms, and weak correlations.
- Coherent entities across each campaign: users, hosts, peer assets, domains, CVEs, MITRE tactics and techniques.
- All graph link types: `causal`, `temporal`, `supports`, `contradicts`, `related`.
- Cross-campaign correlations based on shared actor, technique, infrastructure, or analyst review.
- Sparse isolated alerts to mimic real low-signal SOC noise.

Default generated dataset:

- `1200` nodes
- `1389` links
- `17` campaigns
- `13` MITRE tactics
- `97%` connected nodes

## Files

| File | Purpose |
|------|---------|
| `graph_realistic_large.json` | Ready-to-load deterministic dataset |
| `generate_realistic_graph.py` | Parametric generator (`--nodes`, `--seed`, `--output`) |
| `validate_realistic_graph.py` | Structural and realism validation |

## Validate

From the repository root:

```bash
make graph-realistic-validate
```

Or from this directory:

```bash
python3 validate_realistic_graph.py graph_realistic_large.json
```

## Regenerate

```bash
make graph-realistic-generate
```

Custom size:

```bash
cd examples/cognitive-graph-realistic
python3 generate_realistic_graph.py --nodes 5000 --seed 4242 --output graph_realistic_5000.json
python3 validate_realistic_graph.py graph_realistic_5000.json
```

## Load Into PCMI

Use the same public HTTP API shape as the matrix dataset: create each node via `POST /v1/memories`, then create each edge via `POST /v1/memories/links` with `from_path=memory.<id>` and `to_path=memory.<id>`.

The dataset is intentionally realistic enough for UI/manual graph exploration and load testing. For exact behavioral assertions, keep using:

```bash
make test-cognitive-graph-matrix
```
