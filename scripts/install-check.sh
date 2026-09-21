#!/usr/bin/env bash
set -euo pipefail

: "${REFORGE_INSTALL_ADMIN_DB:?set a superuser maintenance database URL}"
: "${REFORGE_INSTALL_MIGRATION_URL:?set the migration-owner URL for the install database}"
: "${REFORGE_INSTALL_RUNTIME_URL:?set the runtime URL for the install database}"
: "${REFORGE_ENCRYPTION_KEY:?set the operator encryption key}"

root="$(cd "$(dirname "$0")/.." && pwd)"
db="${REFORGE_INSTALL_DB:-reforge_install}"
public_url="${REFORGE_INSTALL_PUBLIC_URL:-https://127.0.0.1:8443}"
oidc_issuer="${REFORGE_INSTALL_OIDC_ISSUER:-https://127.0.0.1:5556}"
oidc_client="${REFORGE_INSTALL_OIDC_CLIENT:-reforge-local}"
address="${REFORGE_INSTALL_ADDRESS:-127.0.0.1:8090}"
oidc_addr="${REFORGE_INSTALL_OIDC_ADDR:-127.0.0.1:5556}"
proxy_addr="${REFORGE_INSTALL_PROXY_ADDR:-127.0.0.1:8443}"
pg_bin="${REFORGE_DRILL_PG_BIN:-$(dirname "$(command -v psql)")}"
fixture_bin="$(mktemp -u /tmp/reforge-localfixture-XXXXXX)"
ca_file="$(mktemp -u /tmp/reforge-localfixture-ca-XXXXXX.pem)"
server_pid=""
fixture_pid=""

cleanup() {
  [ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
  [ -n "$fixture_pid" ] && kill "$fixture_pid" 2>/dev/null || true
  rm -f "$fixture_bin" "$ca_file"
}
trap cleanup EXIT

go build -o "$fixture_bin" ./test/localfixture
go build -o /tmp/reforge-install-server ./cmd/server

"$pg_bin/psql" "$REFORGE_INSTALL_ADMIN_DB" -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE IF EXISTS $db" \
  -c "CREATE DATABASE $db OWNER reforge_migrator" >/dev/null
REFORGE_MIGRATION_DATABASE_URL="$REFORGE_INSTALL_MIGRATION_URL" go run ./cmd/migrate >/dev/null
"$pg_bin/psql" "$REFORGE_INSTALL_MIGRATION_URL" -v ON_ERROR_STOP=1 -f scripts/runtime-grants.sql >/dev/null

"$fixture_bin" --oidc-addr "$oidc_addr" --proxy-addr "$proxy_addr" --upstream "http://$address" --client-id "$oidc_client" --ca-out "$ca_file" >/tmp/reforge-localfixture.log 2>&1 &
fixture_pid=$!
for _ in $(seq 1 20); do [ -s "$ca_file" ] && break; sleep 0.2; done
ready=0
for _ in $(seq 1 30); do if curl -s --cacert "$ca_file" -o /dev/null "$oidc_issuer/.well-known/openid-configuration"; then ready=1; break; fi; sleep 0.3; done
if [ "$ready" != "1" ]; then
  echo "local OIDC fixture did not become ready; is another process bound to the fixture ports?"
  cat /tmp/reforge-localfixture.log
  exit 1
fi

env -u REFORGE_MODE -u REFORGE_FIXTURE_AUTH \
  REFORGE_EDITION=self-hosted REFORGE_ADDRESS="$address" REFORGE_PUBLIC_URL="$public_url" \
  REFORGE_DATABASE_URL="$REFORGE_INSTALL_RUNTIME_URL" REFORGE_ENCRYPTION_KEY="$REFORGE_ENCRYPTION_KEY" \
  REFORGE_OIDC_ISSUER="$oidc_issuer" REFORGE_OIDC_CLIENT_ID="$oidc_client" SSL_CERT_FILE="$ca_file" \
  /tmp/reforge-install-server >/tmp/reforge-install-server.log 2>&1 &
server_pid=$!
for _ in $(seq 1 40); do [ "$(curl -sk -o /dev/null -w '%{http_code}' "$public_url/readyz")" = "200" ] && break; sleep 0.5; done
[ "$(curl -sk -o /dev/null -w '%{http_code}' "$public_url/readyz")" = "200" ] || { echo "control plane did not become ready"; tail -5 /tmp/reforge-install-server.log; exit 1; }

meta="$(curl -sk "$public_url/api/v1/meta")"
echo "$meta" | grep -q '"development":false' || { echo "server is not in production mode: $meta"; exit 1; }
echo "$meta" | grep -q '"fixture_auth":false' || { echo "fixture auth is enabled: $meta"; exit 1; }
curl -sk -o /dev/null -w 'login %{http_code}\n' "$public_url/auth/login" | grep -q '302' || { echo "OIDC login did not redirect"; exit 1; }

jar="$(mktemp)"
touch "$jar"
authorize="$(curl -s --cacert "$ca_file" -c "$jar" -b "$jar" -o /dev/null -w '%{redirect_url}' "$public_url/auth/login")"
callback="$(curl -s --cacert "$ca_file" -c "$jar" -b "$jar" -o /dev/null -w '%{redirect_url}' "$authorize")"
curl -s --cacert "$ca_file" -c "$jar" -b "$jar" -o /dev/null -w '' "$callback"
session="$(curl -s --cacert "$ca_file" -b "$jar" -H "Origin: $public_url" "$public_url/api/v1/session")"
echo "$session" | grep -q 'owner@example.test' || { echo "OIDC session was not established: $session"; exit 1; }
rm -f "$jar"

if [ "${REFORGE_INSTALL_SKIP_BROWSER:-0}" != "1" ] && command -v npx >/dev/null; then
  ( cd web && REFORGE_INSTALL_URL="$public_url" npx playwright test install-oidc.spec.ts )
fi

echo "install check complete: $public_url production mode, OIDC login and session verified"
