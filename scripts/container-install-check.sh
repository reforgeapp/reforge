#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
run_id="t28a_$(date -u +%Y%m%d%H%M%S)_$RANDOM"
run_dir="$root/.local/t28a-install/$run_id"
project="$run_id"
server_name="$project"_server_probe
fixture_name="$project"_oidc_probe
migration_password="Mi-$run_id-owner"
runtime_password="Ru-$run_id-runtime"
client_id="reforge-$run_id"
key='BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc='
mkdir -p "$run_dir/probe" "$run_dir/fixture" "$run_dir/migrations" "$run_dir/gomod" "$run_dir/gocache"
org_id='00000000-0000-4000-8000-000000000001'
connection_id='00000000-0000-4000-8000-000000000002'
secret_id='00000000-0000-4000-8000-000000000003'
server_started=0
fixture_started=0
chmod 0755 "$run_dir" "$run_dir/fixture" "$run_dir/migrations"
umask 077
cat > "$run_dir/.env" <<ENV
POSTGRES_USER=reforge
POSTGRES_PASSWORD=Pg-$run_id-owner
POSTGRES_DB=reforge
REFORGE_BIND=127.0.0.1:$((18000 + RANDOM % 20000))
REFORGE_PUBLIC_URL=https://127.0.0.1:8443
ENV
cat >> "$run_dir/.env" <<ENV
REFORGE_DATABASE_URL=postgres://reforge_runtime:$runtime_password@postgres:5432/reforge?sslmode=disable
REFORGE_MIGRATION_DATABASE_URL=postgres://reforge_migrator:$migration_password@postgres:5432/reforge?sslmode=disable
REFORGE_ENCRYPTION_KEY=$key
REFORGE_ENCRYPTION_KEY_ID=primary
REFORGE_EDITION=self-hosted
REFORGE_VERSION=t28a-local
REFORGE_OIDC_ISSUER=https://127.0.0.1:5556
REFORGE_OIDC_CLIENT_ID=$client_id
REFORGE_OIDC_CLIENT_SECRET=
REFORGE_BOOTSTRAP_TOKEN=
REFORGE_BOOTSTRAP_EXPIRES_AT=
REFORGE_RUNNER_DIR=$run_dir/runner
ENV
dc() { docker compose --project-name "$project" --env-file "$run_dir/.env" --file "$root/deploy/compose/compose.yaml" "$@"; }
cleanup() {
  status=$?
  if (( fixture_started )); then docker rm --force "$fixture_name" >/dev/null 2>&1 || true; fi
  if (( server_started )); then docker rm --force "$server_name" >/dev/null 2>&1 || true; fi
  dc down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$project-server:latest" "$project-migrator:latest" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT
need() { command -v "$1" >/dev/null 2>&1 || { printf 'missing host requirement: %s\n' "$1" >&2; exit 2; }; }
fail() { printf 'FAIL %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS %s\n' "$*"; }
need docker
docker compose version >/dev/null 2>&1
docker info >/dev/null
dc build --pull=false migrator server >"$run_dir/build.log" 2>&1 || { tail -100 "$run_dir/build.log" >&2; fail 'control image build'; }
control_image="$(docker image inspect "$project-server:latest" --format '{{.Id}}' 2>/dev/null || true)"
[[ -n "$control_image" ]] || fail 'control image identity unavailable'
dc up -d postgres >"$run_dir/postgres-up.log" 2>&1
state=''
for _ in $(seq 1 90); do
  state="$(dc exec -T postgres pg_isready -U reforge -d reforge 2>/dev/null || true)"
  [[ "$state" == *'accepting connections'* ]] && break
  sleep 1
done
[[ "$state" == *'accepting connections'* ]] || { cat "$run_dir/postgres-up.log" >&2; fail 'disposable PostgreSQL readiness'; }
dc exec -T postgres psql -U reforge -d postgres -v ON_ERROR_STOP=1 -c "CREATE ROLE reforge_migrator LOGIN PASSWORD '$migration_password'" -c "CREATE ROLE reforge_runtime LOGIN PASSWORD '$runtime_password'" -c 'ALTER DATABASE reforge OWNER TO reforge_migrator' >"$run_dir/roles.log" 2>&1 || { cat "$run_dir/roles.log" >&2; fail 'least-privilege database roles'; }
dc run --rm migrator >"$run_dir/clean-migration.log" 2>&1 || { tail -100 "$run_dir/clean-migration.log" >&2; fail 'clean install migration'; }
dc exec -T postgres psql -U reforge_migrator -d reforge -v ON_ERROR_STOP=1 < "$root/scripts/runtime-grants.sql" >"$run_dir/runtime-grants.log" 2>&1 || { cat "$run_dir/runtime-grants.log" >&2; fail 'runtime grants'; }
pass 'container images built; clean database migrated; least-privilege runtime grants applied'
cp "$root"/internal/store/migrations/*.sql "$run_dir/migrations/"
cat > "$run_dir/migrations/900_t28a_upgrade.sql" <<'SQL'
CREATE TABLE t28a_upgrade_marker(id integer PRIMARY KEY);
SQL
cat > "$run_dir/migrations/901_t28a_failed.sql" <<'SQL'
CREATE TABLE t28a_failed_marker(id integer PRIMARY KEY);
SELECT 1 / 0;
SQL
chmod 0644 "$run_dir"/migrations/*.sql
run_test_migrator() {
  dc run --rm --no-deps -v "$run_dir/migrations:/app/t28a-migrations:ro" -e REFORGE_MIGRATION_DIR=/app/t28a-migrations migrator
}
if run_test_migrator >"$run_dir/failed-upgrade.log" 2>&1; then fail 'deliberately failed migration unexpectedly succeeded'; fi
failed_state="$(dc exec -T postgres psql -U reforge -d reforge -Atc "SELECT (to_regclass('public.t28a_upgrade_marker') IS NOT NULL)::text || ':' || (to_regclass('public.t28a_failed_marker') IS NULL)::text || ':' || (SELECT count(*) FROM schema_migrations WHERE name='901_t28a_failed.sql')::text")"
[[ "$failed_state" == 'true:true:0' ]] || { cat "$run_dir/failed-upgrade.log" >&2; fail "failed migration rollback state: $failed_state"; }
rm "$run_dir/migrations/901_t28a_failed.sql"
run_test_migrator >"$run_dir/upgrade-retry.log" 2>&1 || { tail -100 "$run_dir/upgrade-retry.log" >&2; fail 'migration retry after rollback'; }
run_test_migrator >"$run_dir/upgrade-idempotent.log" 2>&1 || { tail -100 "$run_dir/upgrade-idempotent.log" >&2; fail 'migration idempotency'; }
pass 'synthetic additive migration applied; failed migration rolled back; retry and repeat succeeded'
cat > "$run_dir/probe/main.go" <<'GO'
package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"

	"reforge/internal/secrets"
)

const orgID = "00000000-0000-4000-8000-000000000001"
const connectionID = "00000000-0000-4000-8000-000000000002"
const sentinel = "t28a-encrypted-restore-sentinel"

func main() {
	if len(os.Args) != 2 {
		panic("expected seal, open, or oidc")
	}
	key := os.Getenv("REFORGE_ENCRYPTION_KEY")
	vault, err := secrets.New("primary", map[string]string{"primary": key})
	if err != nil {
		panic(err)
	}
	binding := secrets.Binding{OrgID: orgID, ConnectionID: connectionID, Version: 1}
	switch os.Args[1] {
	case "seal":
		envelope, err := vault.Seal(binding, []byte(sentinel))
		if err != nil {
			panic(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(envelope); err != nil {
			panic(err)
		}
	case "open":
		var envelope secrets.Envelope
		if err := json.NewDecoder(os.Stdin).Decode(&envelope); err != nil {
			panic(err)
		}
		plain, err := vault.Open(binding, envelope)
		if err != nil || string(plain) != sentinel {
			panic("restored credential did not decrypt with backed-up key")
		}
		fmt.Println("credential envelope decrypted with backed-up key")
	case "discovery":
		if err := discover(); err != nil {
			panic(err)
		}
		fmt.Println("fixture OIDC discovery passed")
	case "oidc":
		if err := login(); err != nil {
			panic(err)
		}
		fmt.Println("fixture OIDC authorization and session passed; provider is local fixture")
	default:
		panic("unknown mode")
	}
}

func login() error {
	client, err := fixtureClient()
	if err != nil {
		return err
	}
	response, err := client.Get("https://127.0.0.1:8443/auth/login")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("login flow status %d", response.StatusCode)
	}
	response, err = client.Get("https://127.0.0.1:8443/api/v1/session")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("session status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var session map[string]any
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		return err
	}
	user, _ := session["user"].(map[string]any)
	if user["email"] != "owner@example.test" {
		return errors.New("fixture session identity mismatch")
	}
	return nil
}

func discover() error {
	client, err := fixtureClient()
	if err != nil {
		return err
	}
	response, err := client.Get("https://127.0.0.1:5556/.well-known/openid-configuration")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery status %d", response.StatusCode)
	}
	var discovery map[string]any
	if err := json.NewDecoder(response.Body).Decode(&discovery); err != nil {
		return err
	}
	if discovery["issuer"] != "https://127.0.0.1:5556" {
		return errors.New("fixture issuer mismatch")
	}
	return nil
}

func fixtureClient() (*http.Client, error) {
	caPEM, err := os.ReadFile("/fixture/ca.pem")
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("fixture CA unreadable")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{Timeout: 20 * time.Second, Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}, nil
}
GO
cat > "$run_dir/probe/overlay.json" <<JSON
{"Replace":{"/src/cmd/demo-seed/main.go":"/probe/main.go"}}
JSON

golang_image='golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195'
probe() {
  docker run --rm -i --volume "$root:/src:ro" --volume "$run_dir/probe:/probe:ro" --volume "$run_dir/gomod:/go/pkg/mod" --volume "$run_dir/gocache:/root/.cache/go-build" --workdir /src --env "REFORGE_ENCRYPTION_KEY=$key" "$golang_image" go run -overlay=/probe/overlay.json ./cmd/demo-seed "$1"
}
probe_in_server() {
  docker run --rm --network "container:$server_name" --volume "$root:/src:ro" --volume "$run_dir/probe:/probe:ro" --volume "$run_dir/fixture:/fixture:ro" --volume "$run_dir/gomod:/go/pkg/mod" --volume "$run_dir/gocache:/root/.cache/go-build" --workdir /src --env "REFORGE_ENCRYPTION_KEY=$key" "$golang_image" go run -overlay=/probe/overlay.json ./cmd/demo-seed "$1"
}
probe seal >"$run_dir/envelope.json" 2>"$run_dir/seal.log" || { cat "$run_dir/seal.log" >&2; fail 'container key-envelope creation'; }
if grep -qF 't28a-encrypted-restore-sentinel' "$run_dir/envelope.json"; then fail 'credential plaintext found in envelope'; fi
cat > "$run_dir/seed.sql" <<SQL
BEGIN;
INSERT INTO organisations(id,name) VALUES ('$org_id','T28a restore probe');
INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,secret_id) VALUES ('$org_id','$connection_id','model','fixture','T28a encrypted restore','https://example.invalid','{}','$secret_id');
INSERT INTO connection_secrets(org_id,id,connection_id,version,envelope) VALUES ('$org_id','$secret_id','$connection_id',1,'$(cat "$run_dir/envelope.json")'::jsonb);
COMMIT;
SQL
dc exec -T postgres psql -U reforge -d reforge -v ON_ERROR_STOP=1 < "$run_dir/seed.sql" >"$run_dir/seed.log" 2>&1 || { cat "$run_dir/seed.log" >&2; fail 'encrypted credential seed'; }
pass 'encrypted credential stored; plaintext absent from envelope'
dc run -d --name "$server_name" --no-deps -v "$run_dir/fixture:/fixture:ro" -e SSL_CERT_FILE=/fixture/ca.pem server /bin/sh -c 'sleep infinity' >"$run_dir/server-container-id"
server_started=1
docker run -d --name "$fixture_name" --network "container:$server_name" --volume "$root:/src:ro" --volume "$run_dir/fixture:/fixture:rw" --volume "$run_dir/gomod:/go/pkg/mod" --volume "$run_dir/gocache:/root/.cache/go-build" --workdir /src "$golang_image" go run ./test/localfixture --oidc-addr 127.0.0.1:5556 --proxy-addr 127.0.0.1:8443 --upstream http://127.0.0.1:8080 --client-id "$client_id" --ca-out /fixture/ca.pem >"$run_dir/fixture-container-id"
fixture_started=1
for _ in $(seq 1 120); do
  [[ -s "$run_dir/fixture/ca.pem" ]] && break
  docker inspect "$fixture_name" --format '{{.State.Running}}' 2>/dev/null | grep -q true || { docker logs "$fixture_name" >"$run_dir/fixture.log" 2>&1 || true; cat "$run_dir/fixture.log" >&2; fail 'local fixture failed to start'; }
  sleep 1
done
[[ -s "$run_dir/fixture/ca.pem" ]] || fail 'local fixture certificate was not created'
discovery_state=''
for _ in $(seq 1 30); do
  discovery_state="$(probe_in_server discovery 2>&1 || true)"
  [[ "$discovery_state" == *'fixture OIDC discovery passed'* ]] && break
  sleep 1
done
[[ "$discovery_state" == *'fixture OIDC discovery passed'* ]] || { printf '%s\n' "$discovery_state" >&2; fail 'local OIDC fixture discovery'; }
docker exec "$server_name" sh -c '/app/reforge >/app/var/t28a-server.log 2>&1 & echo $! >/app/var/t28a-server.pid'
ready=''
for _ in $(seq 1 90); do
  ready="$(docker exec "$server_name" sh -c 'wget -qO- http://127.0.0.1:8080/readyz' 2>/dev/null || true)"
  [[ "$ready" == *'"ready"'* ]] && break
  sleep 1
done
[[ "$ready" == *'"ready"'* ]] || { docker exec "$server_name" sh -c 'cat /app/var/t28a-server.log' >&2 || true; fail 'fixture-off production server readiness'; }
meta="$(docker exec "$server_name" sh -c 'wget -qO- http://127.0.0.1:8080/api/v1/meta')"
[[ "$meta" == *'"development":false'* && "$meta" == *'"fixture_auth":false'* ]] || fail "production mode reported incorrectly: $meta"
probe_in_server oidc >"$run_dir/oidc-login.log" 2>&1 || { cat "$run_dir/oidc-login.log" >&2; fail 'local fixture OIDC login'; }
pass 'fixture-off production startup; local OIDC discovery, authorization and session passed'

docker exec "$server_name" sh -c 'kill "$(cat /app/var/t28a-server.pid)"'
for _ in $(seq 1 30); do
  app_pid="$(docker exec "$server_name" sh -c 'cat /app/var/t28a-server.pid' 2>/dev/null || true)"
  alive="$(docker exec "$server_name" sh -c "kill -0 $app_pid" 2>/dev/null && echo yes || true)"
  [[ -z "$alive" ]] && break
  sleep 1
done
docker exec "$server_name" sh -c '/app/reforge >/app/var/t28a-server.log 2>&1 & echo $! >/app/var/t28a-server.pid'
ready=''
for _ in $(seq 1 60); do
  ready="$(docker exec "$server_name" sh -c 'wget -qO- http://127.0.0.1:8080/readyz' 2>/dev/null || true)"
  [[ "$ready" == *'"ready"'* ]] && break
  sleep 1
done
[[ "$ready" == *'"ready"'* ]] || fail 'server restart readiness'
pass 'control plane process restart preserved database readiness'
dc exec -T postgres pg_dump --no-owner --format=custom -U reforge -d reforge >"$run_dir/backup.dump" || fail 'container pg_dump'
[[ -s "$run_dir/backup.dump" ]] || fail 'database backup empty'
dc exec -T postgres psql -U reforge -d postgres -v ON_ERROR_STOP=1 -c 'CREATE DATABASE reforge_restore OWNER reforge_migrator' >"$run_dir/restore-create.log" 2>&1 || { cat "$run_dir/restore-create.log" >&2; fail 'restore database create'; }
dc exec -T postgres pg_restore -U reforge --no-owner --exit-on-error --role=reforge_migrator --dbname=reforge_restore <"$run_dir/backup.dump" >"$run_dir/restore.log" 2>&1 || { tail -100 "$run_dir/restore.log" >&2; fail 'encrypted database restore'; }
restore_migration_url="postgres://reforge_migrator:$migration_password@postgres:5432/reforge_restore?sslmode=disable"
restore_runtime_url="postgres://reforge_runtime:$runtime_password@postgres:5432/reforge_restore?sslmode=disable"
dc run --rm --no-deps -e "REFORGE_MIGRATION_DATABASE_URL=$restore_migration_url" migrator >"$run_dir/restore-migration.log" 2>&1 || { tail -100 "$run_dir/restore-migration.log" >&2; fail 'restored database migration verification'; }
dc exec -T postgres psql -U reforge -d reforge_restore -Atc "SELECT envelope::text FROM connection_secrets WHERE org_id='$org_id' AND connection_id='$connection_id'" >"$run_dir/restored-envelope.json" || fail 'restored encrypted envelope query'
[[ -s "$run_dir/restored-envelope.json" ]] || fail 'restored encrypted envelope absent'
cat "$run_dir/restored-envelope.json" | probe open >"$run_dir/key-recovery.log" 2>&1 || { cat "$run_dir/key-recovery.log" >&2; fail 'restored encrypted credential key recovery'; }
pass 'database dump restored; migrated schema retained; encrypted credential decrypted using backed-up key'

docker exec "$server_name" sh -c 'kill "$(cat /app/var/t28a-server.pid)"'
for _ in $(seq 1 30); do
  app_pid="$(docker exec "$server_name" sh -c 'cat /app/var/t28a-server.pid' 2>/dev/null || true)"
  alive="$(docker exec "$server_name" sh -c "kill -0 $app_pid" 2>/dev/null && echo yes || true)"
  [[ -z "$alive" ]] && break
  sleep 1
done
docker exec -e "REFORGE_DATABASE_URL=$restore_runtime_url" "$server_name" sh -c '/app/reforge >/app/var/t28a-server.log 2>&1 & echo $! >/app/var/t28a-server.pid'
ready=''
for _ in $(seq 1 60); do
  ready="$(docker exec "$server_name" sh -c 'wget -qO- http://127.0.0.1:8080/readyz' 2>/dev/null || true)"
  [[ "$ready" == *'"ready"'* ]] && break
  sleep 1
done
[[ "$ready" == *'"ready"'* ]] || { docker exec "$server_name" sh -c 'cat /app/var/t28a-server.log' >&2 || true; fail 'restored database application readiness'; }
probe_in_server oidc >"$run_dir/restored-oidc-login.log" 2>&1 || { cat "$run_dir/restored-oidc-login.log" >&2; fail 'restored database fixture OIDC login'; }
pass 'control plane restarted against restored database; readiness and fixture OIDC session passed'

printf 'Evidence: %s\n' "$run_dir"
printf 'Build image: %s\n' "$control_image"
printf 'Local fixture only. External IdP, customer TLS, KMS, hosted topology, runner recovery and published image upgrade remain uncertified.\n'
