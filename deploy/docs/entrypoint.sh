#!/bin/sh
set -eu

ROOT=/tmp/docs-root
BASE="$ROOT/docs"
VERSION="${DOCS_VERSION:-dev}"

mkdir -p "$BASE" /tmp/nginx/client_body /tmp/nginx/proxy /tmp/nginx/fastcgi /tmp/nginx/uwsgi /tmp/nginx/scgi
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
