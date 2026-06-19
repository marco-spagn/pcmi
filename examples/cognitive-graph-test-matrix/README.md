# Cognitive Graph test matrix

Deterministic dataset for exhaustive Cognitive Graph E2E checks. It is domain-neutral on purpose: every node is a small incident-analysis memory, and every edge is an explicit `memory_links` relationship supplied by the test.

## Coverage

- All supported link types: `causal`, `temporal`, `supports`, `contradicts`, `related`.
- Multi-hop mixed chain: `causal -> temporal -> supports -> related`.
- Branching graph with positive and contradictory branches.
- `link_types` filters that must include paths only when every edge is allowed.
- Fan-out graph for cursor pagination.
- Directed path behavior, including reverse-path negatives.
- Bidirectional cycle and self-loop traversal.
- Isolated node with no reachable neighbors.
- Cypher passthrough with an existing `WHERE` clause and rejected write queries.

## Run

From the repository root:

```bash
API_PORT=8011 AGE_PORT=5435 REDIS_PORT=6381 bash scripts/e2e/test_cognitive_graph_matrix.sh
```

The script starts its own AGE-enabled PostgreSQL, Redis, and PCMI API process, loads `graph_matrix.json` through the public HTTP API, runs assertions, and cleans up all temporary containers/processes.
