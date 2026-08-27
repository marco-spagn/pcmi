#!/usr/bin/env bash
# PCMI backup — pg_dump wrapper producing a compressed, timestamped custom-format
# archive suitable for pcmi_restore.sh / pg_restore.
#
# Usage:
#   DATABASE_URL=postgres://user:pass@host:5432/pcmi ./scripts/backup/pcmi_backup.sh [OUT_DIR]
#
# Env:
#   DATABASE_URL       required — libpq connection URI
#   PCMI_BACKUP_DIR    default output dir when OUT_DIR arg is omitted (default ./backups)
#
# Prints the archive path on stdout (so callers can capture it); progress on stderr.
set -euo pipefail

DATABASE_URL="${DATABASE_URL:-}"
OUT_DIR="${1:-${PCMI_BACKUP_DIR:-./backups}}"

if [ -z "$DATABASE_URL" ]; then
  echo "[backup] DATABASE_URL is required" >&2
  exit 2
fi
if ! command -v pg_dump >/dev/null 2>&1; then
  echo "[backup] pg_dump not found — install postgresql-client" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
ts="$(date -u +%Y%m%dT%H%M%SZ)"
out="$OUT_DIR/pcmi-${ts}.dump"

echo "[backup] pg_dump → $out" >&2
# --format=custom: compressed, restorable with pg_restore (selective/parallel).
# --no-owner/--no-privileges: portable across roles (restore into any owner).
pg_dump "$DATABASE_URL" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file="$out"

size="$(du -h "$out" | cut -f1)"
echo "[backup] wrote ${size} — $out" >&2
echo "$out"
