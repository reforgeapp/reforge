#!/usr/bin/env bash
set -euo pipefail
root="$1" state="$2"
pgbin="${PGBIN:-/tmp/reforge-postgres/bin}"
chrome="${PLAYWRIGHT_CHROMIUM_PATH:-$HOME/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome}"
run="$state/run" logs="$state/logs" artifacts="$state/artifacts"
rm -rf "$run" "$artifacts" "$state/results"
mkdir -p "$run/control" "$logs" "$artifacts" && chmod 700 "$run"
find "$logs" -type f ! -name web-build.log -delete
{ ip -brief addr; ip route; getent hosts github.com api.github.com; } >"$logs/namespace.txt"

rand() { openssl rand -hex 24; }
pg_pw="$(rand)" mig_pw="$(rand)" rt_pw="$(rand)" enc="$(openssl rand -base64 32)"
pat="ghp_$(rand)" bad_pat="ghp_$(rand)" sso_pat="ghp_$(rand)"
db=reforge_github_app_live pgport=55497 port=8097

pids=()
cleanup() {
  [ -f "$run/control/stop-supervisor" ] || touch "$run/control/stop-supervisor" 2>/dev/null || true
  [ -f "$run/server.pid" ] && kill "$(cat "$run/server.pid")" 2>/dev/null || true
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; done
  "$pgbin/pg_ctl" -D "$run/pgdata" -m fast stop >>"$logs/postgres-ctl.log" 2>&1 || true
  rm -rf "$run"
}
trap cleanup EXIT

openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj /CN=reforge-github-fixture-ca \
  -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign -keyout "$run/ca.key" -out "$run/ca.pem" 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj /CN=github.com -keyout "$run/tls.key" -out "$run/tls.csr" 2>/dev/null
printf 'subjectAltName=DNS:github.com,DNS:api.github.com\nextendedKeyUsage=serverAuth\n' >"$run/ext.cnf"
openssl x509 -req -in "$run/tls.csr" -CA "$run/ca.pem" -CAkey "$run/ca.key" -CAcreateserial -days 1 -extfile "$run/ext.cnf" -out "$run/tls.pem" 2>/dev/null
spki="$(openssl x509 -in "$run/tls.pem" -pubkey -noout | openssl pkey -pubin -outform der | openssl dgst -sha256 -binary | base64)"

printf '%s\n' "$pg_pw" >"$run/pwfile"
"$pgbin/initdb" -D "$run/pgdata" -U postgres --auth=scram-sha-256 --pwfile="$run/pwfile" >"$logs/initdb.log" 2>&1
rm -f "$run/pwfile"
"$pgbin/pg_ctl" -D "$run/pgdata" -l "$logs/postgres.log" -w -o "-c listen_addresses=127.0.0.1 -p $pgport -c unix_socket_directories=$run -c shared_buffers=32MB -c max_connections=40" start >>"$logs/postgres-ctl.log" 2>&1
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
psql_super -d "$db" -At -c "SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname LIKE 'reforge_%' ORDER BY 1" >"$logs/db-roles.txt"

: >"$logs/fixture.jsonl"
FIXTURE_PAT="$pat" FIXTURE_BAD_PAT="$bad_pat" FIXTURE_SSO_PAT="$sso_pat" \
  node "$root/scripts/acceptance/github-app-onboarding/fixture.mjs" "$run/tls.pem" "$run/tls.key" 0.0.0.0 443 "$logs/fixture.jsonl" "$run/control" >"$logs/fixture.log" 2>&1 &
pids+=($!)

mkdir -p "$run/var" && chmod 700 "$run/var"
start_server() {
  env -i PATH="$PATH" HOME="$HOME" SSL_CERT_FILE="$run/ca.pem" \
    REFORGE_ADDRESS="127.0.0.1:$port" REFORGE_PUBLIC_URL="http://127.0.0.1:$port" \
    REFORGE_DATABASE_URL="$(url reforge_runtime "$rt_pw")" REFORGE_ENCRYPTION_KEY="$enc" \
    REFORGE_MODE=development REFORGE_FIXTURE_AUTH=true REFORGE_EDITION=self-hosted \
    REFORGE_WEB_DIR="$state/web-dist" REFORGE_MIGRATION_DIR="$root/pkg/store/migrations" REFORGE_ARTIFACT_DIRECTORY="$run/var" \
    "$state/bin/server" >>"$logs/server.log" 2>&1 &
  echo $! >"$run/server.pid"
  for _ in $(seq 1 120); do curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1 && return 0; sleep 0.25; done
  return 1
}
start_server
pids+=("$(cat "$run/server.pid")")

# Bounded supervisor: a test may request a real restart of the same server
# (same database, encryption key, session config) mid-journey to prove resume.
( while [ ! -f "$run/control/stop-supervisor" ]; do
    if [ -f "$run/control/restart-requested" ]; then
      rm -f "$run/control/restart-requested" "$run/control/restart-done" "$run/control/restart-failed"
      [ -f "$run/server.pid" ] && kill "$(cat "$run/server.pid")" 2>/dev/null || true
      sleep 1
      if start_server; then touch "$run/control/restart-done"; else touch "$run/control/restart-failed"; fi
    fi
    sleep 0.2
  done ) &
pids+=($!)
curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null
curl -fsS --cacert "$run/ca.pem" https://api.github.com/meta -o /dev/null -w '%{http_code}\n' >"$logs/fixture-smoke.txt" 2>&1 || true

rc=0
(cd "$root/web" && PLAYWRIGHT_CHROMIUM_PATH="$chrome" REFORGE_BASE_URL="http://127.0.0.1:$port" REFORGE_GITHUB_LIVE=1 \
  REFORGE_GITHUB_LIVE_PAT="$pat" REFORGE_GITHUB_LIVE_BAD_PAT="$bad_pat" REFORGE_GITHUB_LIVE_SSO_PAT="$sso_pat" \
  REFORGE_GITHUB_LIVE_SPKI="$spki" REFORGE_GITHUB_LIVE_CONTROL="$run/control" \
  REFORGE_GITHUB_LIVE_FIXTURE_LOG="$logs/fixture.jsonl" REFORGE_GITHUB_LIVE_ARTIFACTS="$artifacts" \
  npx playwright test tests/github-app-live.spec.ts --workers=1 --reporter=list --output "$state/results" ${GREP:+--grep "$GREP"}) >"$logs/playwright.log" 2>&1 || rc=$?

if [ "${DEFAULT_SUITE:-0}" = 1 ]; then
  d_rc=0
  (cd "$root/web" && PLAYWRIGHT_CHROMIUM_PATH="$chrome" REFORGE_BASE_URL="http://127.0.0.1:$port" \
    npx playwright test --workers=2 --reporter=list --output "$state/default-results") >"$logs/playwright-default.log" 2>&1 || d_rc=$?
  grep -E '[0-9]+ (passed|failed|skipped|flaky|did not run)' "$logs/playwright-default.log" | tail -1 >"$logs/default-summary.txt" || true
  printf 'default_suite_rc=%s %s\n' "$d_rc" "$(cat "$logs/default-summary.txt" 2>/dev/null)" >"$logs/default-result.txt"
fi

q() { psql_super -d "$db" -At -F ' | ' -c "$1" >"$logs/$2" 2>&1 || true; }
q "SELECT provider, name, endpoint, state, settings->>'auth_kind', settings->>'managed', settings->>'namespace', settings->>'app_id' IS NOT NULL, settings->>'installation_id' IS NOT NULL, left(reason,80) FROM connections WHERE kind='forge' ORDER BY created_at" db-forge-connections.txt
q "SELECT r.name, c.name FROM repositories r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id ORDER BY 1" db-repositories.txt
q "SELECT app_id, installation_id, account_id FROM github_installation_bindings ORDER BY 1" db-installation-bindings.txt
q "SELECT count(*) FROM github_app_setups" db-pending-setups.txt
q "SELECT kind, state, processed FROM inventory_jobs ORDER BY created_at" db-inventory-jobs.txt
q "SELECT action, count(*) FROM audit_events GROUP BY action ORDER BY action" db-audit-actions.txt
"$pgbin/pg_dump" -h 127.0.0.1 -p "$pgport" -U postgres --data-only "$db" >"$run/dump.sql" 2>"$logs/pg_dump.log"
leaks=0
touch "$run/control/secrets.txt"
while IFS= read -r secret; do
  [ -n "$secret" ] || continue
  if grep -rlF -- "$secret" "$logs" "$artifacts" "$run/dump.sql" "$state/results" 2>/dev/null; then leaks=$((leaks + 1)); fi
done < <(printf '%s\n' "$pat" "$bad_pat" "$sso_pat" "$pg_pw" "$mig_pw" "$rt_pw" "$enc"; cat "$run/control/secrets.txt")
grep -E '"(code|state|installation_id)=' "$logs/server.log" >"$logs/server-query-leaks.txt" || true
[ -s "$logs/server-query-leaks.txt" ] && leaks=$((leaks + 1))
grep -E '"status":(404|500)' "$logs/fixture.jsonl" | grep -vF '"path":"/favicon.ico"' >"$logs/fixture-unmatched.jsonl" || true
grep -F fixture_error "$logs/fixture.jsonl" >>"$logs/fixture-unmatched.jsonl" || true
unmatched=$(wc -l <"$logs/fixture-unmatched.jsonl")
printf 'playwright_rc=%s secret_leaks=%s fixture_unmatched=%s\n' "$rc" "$leaks" "$unmatched" | tee "$logs/result.txt"
[ "$rc" = 0 ] && [ "$leaks" = 0 ] && [ "$unmatched" = 0 ]
