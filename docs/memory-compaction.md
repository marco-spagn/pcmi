# Memory compaction vs pruning

PCMI uses **append-only** memory rows (`valid_from` / `valid_to`). Two mechanisms reduce storage pressure:

## Global pruning (`prune_superseded_memories_with_policies`)

- **Worker** calls the SQL function periodically (`PRUNE_RETENTION_DAYS`, `PRUNE_INTERVAL_SECS`).
- Deletes superseded rows whose `valid_to` is older than the retention window. `PRUNE_RETENTION_DAYS` is the default; a matching [namespace retention policy](#namespace-retention-policies) overrides it.
- A worker running against a schema older than migration 028 falls back to the legacy global `prune_superseded_memories`.

## Namespace retention policies

Migration 028 adds `retention_policies`: per-tenant rules bound to an ltree `path_prefix` (`""` = tenant-wide). For each memory row the **most specific** matching prefix wins, and each rule is resolved independently (a child that sets only `max_age_days` still inherits its parent's `superseded_retention_days`).

| Rule | Effect | Applied by |
|------|--------|-----------|
| `superseded_retention_days` | Hard-delete closed versions older than N days — overrides `PRUNE_RETENTION_DAYS` in either direction (keep 7 years for `finance`, 1 day for `scratch`) | Pruning worker |
| `max_age_days` | Soft-close (`valid_to = NOW()`) current memories whose `valid_from` is older than N days; they then age out through the rule above | Expiry worker (`EXPIRY_INTERVAL_SECS`) |

```bash
# Admin: keep finance history 7 years; expire scratch notes after 14 days.
curl -s -X PUT "${PCMI_BASE_URL}/v1/retention-policies" -H "X-API-Key: ${PCMI_ADMIN_KEY}" \
  -H 'Content-Type: application/json' -d '{"path_prefix":"finance","superseded_retention_days":2555,"description":"SOX"}'
curl -s -X PUT "${PCMI_BASE_URL}/v1/retention-policies" -H "X-API-Key: ${PCMI_ADMIN_KEY}" \
  -H 'Content-Type: application/json' -d '{"path_prefix":"scratch","max_age_days":14}'
curl -s "${PCMI_BASE_URL}/v1/retention-policies" -H "X-API-Key: ${PCMI_API_KEY}" | jq .policies
curl -s -X DELETE "${PCMI_BASE_URL}/v1/retention-policies?path_prefix=scratch" -H "X-API-Key: ${PCMI_ADMIN_KEY}"
```

A `PUT` replaces the whole policy for its prefix: omitted rules are cleared, and at least one rule is required (1–36500 days).

## Right to erasure (`POST /v1/memories/erase`)

Admin-only hard delete for data-subject requests (GDPR Art. 17). For one tenant and one non-empty `path_prefix`, a single transaction removes:

- **every version** (current and superseded) of every memory under the prefix — which also removes it from `as_of` time travel;
- `memory_links` touching the prefix; `distilled_knowledge` under the prefix **or derived from an erased memory**; `graph_link_proposals`, `entity_snapshots`, `entity_alias_proposals`, and `consolidation_runs` that reference erased memories.

It then appends a `gdpr_erase` row (prefix, reason, counts — no content) to the [tamper-evident audit chain](../SECURITY.md#tamper-evident-audit-log) as evidence. With Apache AGE installed, the matching `:Memory` graph vertices are removed after commit; a graph failure is reported in `graph_error` without undoing the SQL erasure.

```bash
# Preview (exact counts; the transaction is rolled back).
curl -s -X POST "${PCMI_BASE_URL}/v1/memories/erase" -H "X-API-Key: ${PCMI_ADMIN_KEY}" \
  -H 'Content-Type: application/json' -d '{"path_prefix":"users.alice","dry_run":true}' | jq .counts
# Erase.
curl -s -X POST "${PCMI_BASE_URL}/v1/memories/erase" -H "X-API-Key: ${PCMI_ADMIN_KEY}" \
  -H 'Content-Type: application/json' -d '{"path_prefix":"users.alice","reason":"DSR-2026-0042"}'
```

**Not covered:** backups taken before the erasure (expire them per your backup retention), the `events` ingest table (free-form payloads cannot be mapped to a path), transient Redis stream entries, and webhook payloads already delivered to receivers. Plan tenant path layouts so one data subject maps to one prefix (e.g. `users.<id>`).

## Per-path compaction (`compact_memory_path_history`)

- **API**: `POST /v1/memories/compact` with `{ "path": "root.topic", "keep_superseded": 20 }` (write role).
- For **one** `tenant_id` + `path`, deletes superseded rows (`valid_to IS NOT NULL`) except the **newest `keep_superseded` closed versions** (by `version` DESC). The **current** row (`valid_to IS NULL`) is never touched.
- Use when a hot path accumulated hundreds of versions and you want bounded history without waiting for global retention.

## Operational notes

- Compaction **destroys** historical rows; audit / lineage for deleted versions is gone. Prefer higher `keep_superseded` first.
- After compaction, `GET /v1/memories/history` returns fewer entries.
- Pruning and compaction are compatible: pruning still removes old closed rows globally; compaction trims depth per path.
- Erasure is irreversible and removes current rows too; always preview with `dry_run: true`.
