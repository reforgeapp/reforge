#!/usr/bin/env bash
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../../.." && pwd)"
state="${STATE:-$root/.local/github-app-onboarding/connected}"
mkdir -p "$state/bin" "$state/logs"
if [ "${SKIP_BUILD:-0}" != 1 ]; then
  (cd "$root" && go build -o "$state/bin/server" ./cmd/server && go build -o "$state/bin/migrate" ./cmd/migrate)
  (cd "$root/web" && npx tsc --noEmit && npx vite build --outDir "$state/web-dist" --emptyOutDir) >"$state/logs/web-build.log" 2>&1
fi
printf '93.184.216.34 github.com api.github.com\n127.0.0.1 localhost\n' >"$state/hosts"
exec timeout 1200 unshare --user --map-root-user --net --mount sh -c 'ip link set lo up && ip addr add 93.184.216.34/32 dev lo && mount --bind "$5" /etc/hosts && echo 0 >/proc/sys/net/ipv4/ip_unprivileged_port_start && exec unshare --user --map-user="$0" --map-group="$1" bash "$2" "$3" "$4"' "$(id -u)" "$(id -g)" "$here/inner.sh" "$root" "$state" "$state/hosts"
