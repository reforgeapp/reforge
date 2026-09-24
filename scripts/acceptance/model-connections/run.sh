#!/usr/bin/env bash
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../../.." && pwd)"
state="${STATE:-$root/.local/model-connections/connected}"
mkdir -p "$state/bin" "$state/logs"
(cd "$root" && go build -o "$state/bin/server" ./cmd/server && go build -o "$state/bin/migrate" ./cmd/migrate)
(cd "$root/web" && npx tsc --noEmit && npx vite build --outDir "$state/web-dist" --emptyOutDir) >"$state/logs/web-build.log" 2>&1
exec timeout 900 unshare --user --map-root-user --net sh -c 'ip link set lo up && ip addr add 93.184.216.34/32 dev lo && exec unshare --user --map-user="$0" --map-group="$1" bash "$2" "$3" "$4"' "$(id -u)" "$(id -g)" "$here/inner.sh" "$root" "$state"
