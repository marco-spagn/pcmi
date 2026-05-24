#!/usr/bin/env bash
# Wait for Postgres and apply migrations/*.sql (CI services + local compose).
set -euo pipefail

PGHOST="${PGHOST:-127.0.0.1}"
PGUSER="${PGUSER:-pcmi}"
PGDATABASE="${PGDATABASE:-pcmi}"
export PGPASSWORD="${PGPASSWORD:-pcmi}"

for i in $(seq 1 30); do
  if psql -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" -c 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  if [[ "$i" -eq 30 ]]; then
    echo "migrate_postgres: Postgres not ready at ${PGHOST}" >&2
    exit 1
  fi
  sleep 2
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
for f in "$ROOT"/migrations/*.sql; do
  echo "Applying $(basename "$f")"
  psql -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" -v ON_ERROR_STOP=1 -f "$f"
done
