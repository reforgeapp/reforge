#!/usr/bin/env bash
set -euo pipefail
root="$1" state="$2"
pgbin="${PGBIN:-/tmp/reforge-postgres/bin}"
chrome="${PLAYWRIGHT_CHROMIUM_PATH:-$HOME/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome}"
run="$state/run" logs="$state/logs" artifacts="$state/artifacts"
rm -rf "$run" "$artifacts" "$state/results"
mkdir -p "$run" "$logs" "$artifacts" && chmod 700 "$run"
find "$logs" -type f ! -name web-build.log -delete
{ ip -brief addr; ip route; } >"$logs/namespace.txt"

rand() { openssl rand -hex 24; }
pg_pw="$(rand)" mig_pw="$(rand)" rt_pw="$(rand)"
key="sk-fixture-$(rand)" bad_key="sk-fixture-bad-$(rand)"
db=reforge_model_live pgport=55493 port=8095

pids=()
cleanup() {
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; done
  "$pgbin/pg_ctl" -D "$run/pgdata" -m fast stop >>"$logs/postgres-ctl.log" 2>&1 || true
  rm -rf "$run"
}
trap cleanup EXIT

openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj /CN=reforge-fixture-ca \
  -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign -keyout "$run/ca.key" -out "$run/ca.pem" 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj /CN=reforge-fixture -keyout "$run/tls.key" -out "$run/tls.csr" 2>/dev/null
printf 'subjectAltName=IP:93.184.216.34,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n' >"$run/ext.cnf"
openssl x509 -req -in "$run/tls.csr" -CA "$run/ca.pem" -CAkey "$run/ca.key" -CAcreateserial -days 1 -extfile "$run/ext.cnf" -out "$run/tls.pem" 2>/dev/null

printf '%s\n' "$pg_pw" >"$run/pwfile"
"$pgbin/initdb" -D "$run/pgdata" -U postgres --auth=scram-sha-256 --pwfile="$run/pwfile" >"$logs/initdb.log" 2>&1
rm -f "$run/pwfile"
"$pgbin/pg_ctl" -D "$run/pgdata" -l "$logs/postgres.log" -w -o "-c listen_addresses=127.0.0.1 -p $pgport -c unix_socket_directories=$run -c shared_buffers=32MB -c max_connections=30" start >>"$logs/postgres-ctl.log" 2>&1
export PGPASSWORD="$pg_pw"
psql_super() { "$pgbin/psql" -X -q -h 127.0.0.1 -p "$pgport" -U postgres -v ON_ERROR_STOP=1 "$@"; }
url() { echo "postgres://$1:$2@127.0.0.1:$pgport/$db?sslmode=disable"; }
psql_super -d postgres >"$logs/roles.log" 2>&1 <<SQL
CREATE ROLE reforge_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION BYPASSRLS PASSWORD '$mig_pw';
CREATE ROLE reforge_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '$rt_pw';
CREATE DATABASE $db OWNER reforge_migrator;
SQL
psql_super -d "$db" -c "REVOKE ALL ON SCHEMA public FROM PUBLIC; GRANT ALL ON SCHEMA public TO reforge_migrator;" >>"$logs/roles.log" 2>&1
REFORGE_MIGRATION_DATABASE_URL="$(url reforge_migrator "$mig_pw")" REFORGE_MIGRATION_DIR="$root/pkg/store/migrations" "$state/bin/migrate" >"$logs/migrate.log" 2>&1
PGPASSWORD="$mig_pw" "$pgbin/psql" -X -q -v ON_ERROR_STOP=1 "$(url reforge_migrator "$mig_pw")" -f "$root/scripts/runtime-grants.sql" >"$logs/grants.log" 2>&1

: >"$logs/fixture.jsonl"
FIXTURE_API_KEY="$key" node "$root/scripts/acceptance/model-connections/fixture.mjs" "$run/tls.pem" "$run/tls.key" 0.0.0.0 8443 "$logs/fixture.jsonl" >"$logs/fixture.log" 2>&1 &
pids+=($!)

mkdir -p "$run/var" && chmod 700 "$run/var"
env -i PATH="$PATH" HOME="$HOME" \
  REFORGE_ADDRESS="127.0.0.1:$port" REFORGE_PUBLIC_URL="http://127.0.0.1:$port" \
  REFORGE_DATABASE_URL="$(url reforge_runtime "$rt_pw")" REFORGE_ENCRYPTION_KEY="$(openssl rand -base64 32)" \
  REFORGE_MODE=development REFORGE_FIXTURE_AUTH=true REFORGE_EDITION=self-hosted \
  REFORGE_WEB_DIR="$state/web-dist" REFORGE_MIGRATION_DIR="$root/pkg/store/migrations" REFORGE_ARTIFACT_DIRECTORY="$run/var" \
  "$state/bin/server" >"$logs/server.log" 2>&1 &
pids+=($!)
for _ in $(seq 1 60); do curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null

rc=0
(cd "$root/web" && PLAYWRIGHT_CHROMIUM_PATH="$chrome" REFORGE_BASE_URL="http://127.0.0.1:$port" REFORGE_MODEL_LIVE=1 \
  REFORGE_MODEL_LIVE_KEY="$key" REFORGE_MODEL_LIVE_BAD_KEY="$bad_key" REFORGE_MODEL_LIVE_CA="$run/ca.pem" \
  REFORGE_MODEL_LIVE_FIXTURE_LOG="$logs/fixture.jsonl" REFORGE_MODEL_LIVE_ARTIFACTS="$artifacts" \
  npx playwright test tests/model-connections-live.spec.ts --workers=1 --reporter=list --output "$state/results") >"$logs/playwright.log" 2>&1 || rc=$?

psql_super -d "$db" -At -c "SELECT provider, name, endpoint, state, settings->>'profile', settings->>'model', verified_at IS NOT NULL FROM connections WHERE kind='model' ORDER BY created_at" >"$logs/db-model-connections.txt"
psql_super -d "$db" -At -c "SELECT action, count(*) FROM audit_events GROUP BY action ORDER BY action" >"$logs/db-audit-actions.txt" 2>&1 || true
"$pgbin/pg_dump" -h 127.0.0.1 -p "$pgport" -U postgres --data-only "$db" >"$run/dump.sql" 2>"$logs/pg_dump.log"
leaks=0
for secret in "$key" "$bad_key" "$pg_pw" "$mig_pw" "$rt_pw"; do
  if grep -rlF -- "$secret" "$logs" "$artifacts" "$run/dump.sql" ${state}/results 2>/dev/null; then leaks=1; fi
done
[ "$(grep -c '"auth":"valid"' "$logs/fixture.jsonl")" -gt 0 ] || rc=1
grep -vE '"path":"/v1/models(/[a-z0-9-]+)?"' "$logs/fixture.jsonl" >"$logs/fixture-non-metadata.jsonl" || true
[ -s "$logs/fixture-non-metadata.jsonl" ] && rc=1
printf 'playwright_and_fixture_rc=%s secret_leaks=%s\n' "$rc" "$leaks" | tee "$logs/result.txt"
[ "$rc" = 0 ] && [ "$leaks" = 0 ]
