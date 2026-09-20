set -euo pipefail
if [[ -f .local/development.env ]]; then set -a; source .local/development.env; set +a; fi
: "${REFORGE_DATABASE_URL:?Set the non-privileged runtime database URL; see docs/implementation/local-development.md}"
: "${REFORGE_ENCRYPTION_KEY:?Set the base64 encryption key}"
export REFORGE_MODE=development
export REFORGE_FIXTURE_AUTH=true
export REFORGE_EDITION=self-hosted
export REFORGE_ADDRESS=127.0.0.1:8080
export REFORGE_PUBLIC_URL=http://127.0.0.1:8080
exec bin/reforge
