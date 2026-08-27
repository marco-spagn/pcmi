#!/usr/bin/env bash
# PCMI restore — pg_restore wrapper. Refuses to overwrite a populated database
# unless FORCE=1, so an accidental restore cannot silently clobber live data.
#
# Usage:
#   DATABASE_URL=postgres://user:pass@host:5432/pcmi \
#     ./scripts/backup/pcmi_restore.sh <backup.dump>
#
# Env:
#   DATABASE_URL   required — target libpq connection URI
#   FORCE          set to 1 to restore over a non-empty target (drops+recreates
#                  objects via pg_restore --clean --if-exists)
set -euo pipefail

BACKUP_FILE="${1:-}"
DATABASE_URL="${DATABASE_URL:-}"
FORCE="${FORCE:-0}"

if [ -z "$BACKUP_FILE" ]; then
  echo "usage: pcmi_restore.sh <backup.dump>   (DATABASE_URL env required)" >&2
  exit 2
fi
if [ ! -f "$BACKUP_FILE" ]; then
  echo "[restore] no such file: $BACKUP_FILE" >&2
  exit 2
fi
if [ -z "$DATABASE_URL" ]; then
  echo "[restore] DATABASE_URL is required" >&2
  exit 2
fi
if ! command -v pg_restore >/dev/null 2>&1 || ! command -v psql >/dev/null 2>&1; then
  echo "[restore] pg_restore/psql not found — install postgresql-client" >&2
  exit 2
fi

# Safety gate: count existing tables in the public schema.
existing="$(psql "$DATABASE_URL" -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'" 2>/dev/null || echo 0)"
existing="$(echo "$existing" | tr -d '[:space:]')"
if [ "${existing:-0}" -gt 0 ] && [ "$FORCE" != "1" ]; then
  echo "[restore] target already has ${existing} public tables." >&2
  echo "[restore] refusing to overwrite — re-run with FORCE=1 to proceed." >&2
  exit 3
fi

echo "[restore] pg_restore ← $BACKUP_FILE" >&2
# --clean --if-exists: drop existing objects first (safe on an empty DB too).
# --no-owner/--no-privileges: restore regardless of the dump's original roles.
# --exit-on-error keeps a partial/corrupt restore from looking successful.
pg_restore \
  --clean --if-exists \
  --no-owner --no-privileges \
  --exit-on-error \
  --dbname="$DATABASE_URL" \
  "$BACKUP_FILE"

echo "[restore] done" >&2
