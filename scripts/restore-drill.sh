#!/usr/bin/env bash
set -euo pipefail

: "${REFORGE_DRILL_SOURCE_URL:?set the source runtime database URL}"
: "${REFORGE_DRILL_ADMIN_DB:?set a superuser URL to the maintenance database}"
: "${REFORGE_DRILL_TARGET_URL:?set the scratch restore database URL}"
: "${REFORGE_ENCRYPTION_KEY:?set the backed-up operator encryption key}"

pg_bin="${REFORGE_DRILL_PG_BIN:-$(dirname "$(command -v pg_dump)")}"
scratch="${REFORGE_DRILL_DB:-reforge_restore_drill}"
dump="$(mktemp)"
trap 'rm -f "$dump"' EXIT

"$pg_bin/pg_dump" --no-owner --format=custom --file="$dump" "$REFORGE_DRILL_SOURCE_URL"
"$pg_bin/psql" "$REFORGE_DRILL_ADMIN_DB" -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE IF EXISTS $scratch" -c "CREATE DATABASE $scratch"
"$pg_bin/pg_restore" --no-owner --exit-on-error --dbname="$REFORGE_DRILL_TARGET_URL" "$dump"

tables="$("$pg_bin/psql" "$REFORGE_DRILL_TARGET_URL" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"
migrations="$("$pg_bin/psql" "$REFORGE_DRILL_TARGET_URL" -tAc "SELECT count(*) FROM schema_migrations")"
test "$tables" -ge 30
test "$migrations" -ge 33

if [ -n "${REFORGE_DRILL_ARTIFACTS:-}" ]; then
  missing=0
  while IFS='|' read -r org id size; do
    file="$REFORGE_DRILL_ARTIFACTS/$org-$id.data"
    if [ ! -f "$file" ] || [ "$(stat -c %s "$file")" != "$size" ]; then
      echo "artifact missing or truncated: $org-$id" >&2
      missing=$((missing + 1))
    fi
  done < <("$pg_bin/psql" "$REFORGE_DRILL_TARGET_URL" -tAc "SELECT org_id,id,size FROM artifacts WHERE expires_at>now()")
  test "$missing" -eq 0
fi

REFORGE_ENCRYPTION_KEY="$REFORGE_ENCRYPTION_KEY" go test -count=1 -run TestRestoreDrillKeyRecovery ./pkg/secrets/

echo "restore drill complete: tables=$tables migrations=$migrations"
