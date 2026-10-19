#!/usr/bin/env bash
set -euo pipefail

if (( $# > 1 )); then
  echo "Usage: release-version.sh [COMMIT]" >&2
  exit 1
fi

if ! commit=$(git rev-parse --verify --end-of-options "${1-HEAD}^{commit}" 2>/dev/null); then
  echo "Release commit does not resolve to a commit" >&2
  exit 1
fi

tags=$(git tag --merged "$commit" --list 'v*')
base=$(printf '%s\n' "$tags" | sed -nE '/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/p' | LC_ALL=C sort -V | tail -n 1)
if [[ -z $base ]]; then
  echo "No vMAJOR.MINOR tag reachable from release commit" >&2
  exit 1
fi

line="${base#v}"
pattern="^v${line//./\\.}\\.(0|[1-9][0-9]*)$"
existing=$(git tag --points-at "$commit" --list 'v*' | grep -E "$pattern" | LC_ALL=C sort -V | tail -n 1 || true)
if [[ -n $existing ]]; then
  printf '%s\n' "${existing#v}"
  exit 0
fi

latest=$(git tag --list 'v*' | grep -E "$pattern" | LC_ALL=C sort -V | tail -n 1 || true)
if [[ -z $latest ]]; then
  printf '%s.0\n' "$line"
else
  printf '%s.%s\n' "$line" "$(( ${latest##*.} + 1 ))"
fi
