#!/usr/bin/env bash
# End-to-end proof that a PCMI backup can be restored with zero data loss:
#   seed a marker → backup → wipe the schema → restore → assert the marker and
#   row counts survived. Exits non-zero on any mismatch (CI gate).
#
# Requires: DATABASE_URL pointing at a migrated PCMI database + postgresql-client.
set -euo pipefail

DATABASE_URL="${DATABASE_URL:?DATABASE_URL is required}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

psql_q() { psql "$DATABASE_URL" -tAc "$1"; }

marker="backup-restore-$(date +%s)"

echo "== 1. seed a marker tenant =="
psql_q "INSERT INTO tenants (slug, name) VALUES ('$marker', '$marker')" >/dev/null
tenants_before="$(psql_q "SELECT count(*) FROM tenants")"
tables_before="$(psql_q "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"
echo "   tenants=$tenants_before tables=$tables_before"

echo "== 2. backup =="
backup="$(DATABASE_URL="$DATABASE_URL" bash "$HERE/pcmi_backup.sh" "$WORK")"
[ -f "$backup" ] || { echo "FAIL: backup file missing"; exit 1; }

echo "== 3. wipe the schema =="
psql "$DATABASE_URL" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" >/dev/null
wiped="$(psql_q "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"
[ "$wiped" = "0" ] || { echo "FAIL: schema not empty after wipe ($wiped tables)"; exit 1; }
echo "   confirmed empty"

echo "== 4. restore =="
DATABASE_URL="$DATABASE_URL" bash "$HERE/pcmi_restore.sh" "$backup"

echo "== 5. assert no data loss =="
tenants_after="$(psql_q "SELECT count(*) FROM tenants")"
tables_after="$(psql_q "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"
found="$(psql_q "SELECT count(*) FROM tenants WHERE slug='$marker'")"
echo "   tenants=$tenants_after tables=$tables_after marker_found=$found"

fail=0
[ "$tenants_after" = "$tenants_before" ] || { echo "FAIL: tenant count $tenants_before → $tenants_after"; fail=1; }
[ "$tables_after" = "$tables_before" ]   || { echo "FAIL: table count $tables_before → $tables_after"; fail=1; }
[ "$found" = "1" ]                       || { echo "FAIL: marker tenant not restored"; fail=1; }

# The restore safety gate must refuse a second restore over the now-populated DB.
echo "== 6. assert restore refuses to clobber without FORCE =="
if DATABASE_URL="$DATABASE_URL" bash "$HERE/pcmi_restore.sh" "$backup" >/dev/null 2>&1; then
  echo "FAIL: restore overwrote a populated DB without FORCE=1"; fail=1
else
  echo "   refused (as expected)"
fi

if [ "$fail" != "0" ]; then
  echo "BACKUP/RESTORE TEST: FAILED"; exit 1
fi
echo "BACKUP/RESTORE TEST: PASSED — zero data loss across backup → wipe → restore"
