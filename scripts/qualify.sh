set -euo pipefail
if [[ "${REFORGE_QUALIFY_EXTERNAL:-}" != "authorised" ]]; then
  echo 'External qualification requires REFORGE_QUALIFY_EXTERNAL=authorised, dedicated test accounts and explicit bounded budgets.' >&2
  exit 2
fi
echo 'External certification scenarios will be registered by their owning implementation tickets. No certification has run.' >&2
exit 2
