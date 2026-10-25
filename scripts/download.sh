#!/bin/sh
# Сохраняет TXT и явно разрешает владельцу читать и изменять файл.
set -eu
output=${1:-symbol-art.txt}
endpoint=${SYMBOL_URL:-http://localhost:8080}
temporary=$(mktemp)
trap 'rm -f "$temporary"' EXIT HUP INT TERM
curl --fail --silent --show-error "$endpoint/export" \
  --data-urlencode 'content@-' --data 'format=txt' --output "$temporary"
chmod 0644 "$temporary"
mv -- "$temporary" "$output"
