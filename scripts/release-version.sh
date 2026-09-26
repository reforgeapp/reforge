#!/usr/bin/env bash
set -euo pipefail

if (( $# < 1 || $# > 2 )) || [[ ! ${1:-} =~ ^[1-9][0-9]*$ ]]; then
  echo "Usage: release-version.sh RUN_NUMBER [COMMIT]; RUN_NUMBER must be a positive integer without leading zeros" >&2
  exit 1
fi

if ! commit=$(git rev-parse --verify --end-of-options "${2-HEAD}^{commit}" 2>/dev/null); then
  echo "Release commit does not resolve to a commit" >&2
  exit 1
fi

tags=$(git tag --merged "$commit" --list 'v*')
base=$(printf '%s\n' "$tags" | sed -nE '/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/p' | LC_ALL=C sort -V | tail -n 1)
if [[ -z $base ]]; then
  echo "No vMAJOR.MINOR tag reachable from release commit" >&2
  exit 1
fi

printf '%s.%s\n' "${base#v}" "$1"
