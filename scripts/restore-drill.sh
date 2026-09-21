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

REFORGE_ENCRYPTION_KEY="$REFORGE_ENCRYPTION_KEY" go test -count=1 -run TestRestoreDrillKeyRecovery ./internal/secrets/

echo "restore drill complete: tables=$tables migrations=$migrations"
