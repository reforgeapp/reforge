#!/bin/sh
set -eu

BASE=/usr/share/nginx/html/docs
VERSION="${DOCS_VERSION:-dev}"

mkdir -p "$BASE"
cp -r /opt/docs-site/. "$BASE/"

if [ "$VERSION" != "latest" ] && [ ! -d "$BASE/$VERSION" ]; then
  mkdir -p "$BASE/$VERSION"
  cp -r /opt/docs-site/. "$BASE/$VERSION/"
fi

{
  printf '{"versions":["latest"'
  if [ "$VERSION" != "latest" ]; then
    printf ',"%s"' "$VERSION"
  fi
  printf ']}\n'
} > "$BASE/versions.json"

exec nginx -g 'daemon off;'
