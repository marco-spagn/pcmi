# Runbook — Backup & Restore / Disaster Recovery

PCMI keeps all durable state in **PostgreSQL** (memory entries and their
versions, links, sessions, distilled knowledge, entity graph tables, tenants,
API keys, audit log). Redis holds only the transient event stream and rate-limit
counters — it is **not** a source of truth and does not need backing up.

This runbook covers logical backups with `pg_dump` and restore with
`pg_restore`. For very large or low-RPO deployments, prefer your platform's
**physical** backup / point-in-time recovery (PITR) — see *Beyond logical dumps*.

## Tooling

Two wrappers under `scripts/backup/`:

| Script | Purpose |
|---|---|
| `pcmi_backup.sh [OUT_DIR]` | `pg_dump` → compressed, timestamped custom-format archive. Prints the archive path. |
| `pcmi_restore.sh <archive>` | `pg_restore` into `DATABASE_URL`. **Refuses a non-empty target unless `FORCE=1`.** |

Both read the target from `DATABASE_URL`. Makefile shortcuts:

```bash
make backup                         # → ./backups/pcmi-<ts>.dump
make backup BACKUP_DIR=/mnt/backups
make restore BACKUP_FILE=./backups/pcmi-20260101T000000Z.dump
make restore BACKUP_FILE=... FORCE=1   # overwrite a populated DB
```

> **Version note:** run `pg_dump` with a client version **equal to or newer than
> the server**, and restore with a client **matching the target server major
> version**. A newer client dumping for an older server can emit settings the
> older server rejects (e.g. `transaction_timeout`, added in PG 17). PCMI targets
> **PostgreSQL 16**; use `postgresql-client-16`. The dockerized DB already ships
> matching tools — `docker exec pcmi-postgres pg_dump …` always version-matches.

## Take a backup

```bash
export DATABASE_URL='postgres://pcmi:pcmi@db-host:5432/pcmi?sslmode=disable'
./scripts/backup/pcmi_backup.sh /mnt/backups
# → /mnt/backups/pcmi-20260724T093000Z.dump
```

Store the archive off-box (object storage, another region). Automate with a cron
/ CronJob calling the same script; keep N daily + M weekly copies.

## Restore

Into an **empty** database (fresh DR target):

```bash
export DATABASE_URL='postgres://pcmi:pcmi@dr-host:5432/pcmi?sslmode=disable'
createdb -h dr-host -U pcmi pcmi          # if the database does not exist yet
./scripts/backup/pcmi_restore.sh /mnt/backups/pcmi-20260724T093000Z.dump
```

Over an **existing** database (accepts data loss — the current contents are
dropped and replaced):

```bash
FORCE=1 ./scripts/backup/pcmi_restore.sh /mnt/backups/pcmi-20260724T093000Z.dump
```

The restore uses `pg_restore --clean --if-exists --exit-on-error`, so a partial
or corrupt archive fails loudly instead of leaving a half-restored database.

## Verify a backup (do this regularly — an untested backup is not a backup)

Restore into a throwaway database and check row counts:

```bash
createdb -h localhost -U pcmi pcmi_verify
DATABASE_URL='postgres://pcmi:pcmi@localhost:5432/pcmi_verify?sslmode=disable' \
  ./scripts/backup/pcmi_restore.sh <archive>
psql "$DATABASE_URL" -c "SELECT count(*) FROM memory_entries;"
dropdb -h localhost -U pcmi pcmi_verify
```

CI runs the full **seed → backup → wipe → restore → assert** cycle on every PR
(`scripts/backup/ci_backup_restore_test.sh`, the `backup-restore` job), so a
regression that breaks restore fails the build.

## Disaster recovery — order of operations

1. Provision a PostgreSQL 16 instance (with the `vector`, `ltree`, `pg_trgm`
   extensions available; the dump recreates them).
2. Restore the most recent verified archive (see *Restore*, empty target).
3. Point `DATABASE_URL` (and `DATABASE_READ_URL`, if used) at the new instance.
4. Start API + worker. Redis can be empty — the worker rebuilds its consumer
   group; embeddings already persisted are restored with the dump.
5. Smoke: `curl $API/v1/ready` → `database_ok:true`, then a `POST /v1/retrieve`.

**RPO/RTO:** with periodic logical dumps, RPO = your backup interval and RTO =
restore time (minutes for small/medium corpora). For tighter objectives use PITR.

## Beyond logical dumps (PITR)

For large corpora or near-zero RPO, use physical backups + WAL archiving
(`pg_basebackup` + `archive_command`, or a managed service's continuous backup —
RDS/Cloud SQL/Crunchy). PITR replays WAL to any point in time; logical dumps
remain useful for portable, cross-version, per-database exports.
