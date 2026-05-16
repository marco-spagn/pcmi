# Memory compaction vs pruning

PCMI uses **append-only** memory rows (`valid_from` / `valid_to`). Two mechanisms reduce storage pressure:

## Global pruning (`prune_superseded_memories`)

- **Worker** calls the SQL function periodically (`PRUNE_RETENTION_DAYS`, `PRUNE_INTERVAL_SECS`).
- Deletes **any** superseded row whose `valid_to` is older than the retention window.
- Tenant-agnostic sweep across the whole table.

## Per-path compaction (`compact_memory_path_history`)

- **API**: `POST /v1/memories/compact` with `{ "path": "root.topic", "keep_superseded": 20 }` (write role).
- For **one** `tenant_id` + `path`, deletes superseded rows (`valid_to IS NOT NULL`) except the **newest `keep_superseded` closed versions** (by `version` DESC). The **current** row (`valid_to IS NULL`) is never touched.
- Use when a hot path accumulated hundreds of versions and you want bounded history without waiting for global retention.

## Operational notes

- Compaction **destroys** historical rows; audit / lineage for deleted versions is gone. Prefer higher `keep_superseded` first.
- After compaction, `GET /v1/memories/history` returns fewer entries.
- Pruning and compaction are compatible: pruning still removes old closed rows globally; compaction trims depth per path.
