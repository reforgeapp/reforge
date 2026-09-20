set -euo pipefail
: "${REFORGE_TEST_DATABASE_URL:?Set a disposable PostgreSQL runtime URL}"
: "${REFORGE_TEST_MIGRATION_DATABASE_URL:?Set its migration-owner URL}"
python3 scripts/check-test-database.py
REFORGE_MIGRATION_DATABASE_URL="$REFORGE_TEST_MIGRATION_DATABASE_URL" go run ./cmd/migrate
go test -race -count=1 ./test/integration/...
