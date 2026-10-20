set -euo pipefail
paths=(pkg/httpapi/generated pkg/store/dbgen web/src/api/schema.ts)
before=$(mktemp -d)
trap 'for file in "$before/hashes" "$before/after"; do if [[ -f "$file" ]]; then unlink "$file"; fi; done; rmdir "$before"' EXIT
manifest() {
  for path in "${paths[@]}"; do
    if [[ -d "$path" ]]; then find "$path" -type f -print0 | sort -z | xargs -r -0 sha256sum; elif [[ -f "$path" ]]; then sha256sum "$path"; fi
  done
}
if [[ "${1:-}" == "--check" ]]; then
  manifest > "$before/hashes"
fi
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config api/codegen.yaml api/openapi.yaml
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
npm --prefix web run generate
python3 scripts/strip-generated-comments.py
gofmt -w pkg/httpapi/generated pkg/store/dbgen
if [[ "${1:-}" == "--check" ]]; then
  manifest > "$before/after"
  diff -u "$before/hashes" "$before/after"
fi
