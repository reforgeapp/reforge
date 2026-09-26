set -euo pipefail
if [[ "${REFORGE_QUALIFY_EXTERNAL:-}" != "authorised" ]]; then
  echo 'External qualification requires REFORGE_QUALIFY_EXTERNAL=authorised and a Reforge session for the qualification org.' >&2
  exit 2
fi
for name in REFORGE_URL REFORGE_ORG REFORGE_QUALIFY_REPOSITORY_ID REFORGE_SESSION_COOKIE; do
  if [[ -z "${!name:-}" ]]; then
    echo "$name is required" >&2
    exit 2
  fi
done
gh auth status >/dev/null
python3 "$(dirname "$0")/qualify.py"
