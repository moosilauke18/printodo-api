#!/usr/bin/env bash
# Re-run AI classification over all existing items in the printodo DB, wiring
# credentials from your existing local files. By default it wipes the current
# categories and rebuilds a clean, specific set (notes themselves are kept).
#
# Usage:
#   scripts/reclassify.sh            # wipe categories and reclassify everything
#   RESET=0 scripts/reclassify.sh    # reclassify without wiping (keep current set)
set -euo pipefail
cd "$(dirname "$0")/.."

PG_FILE="${PG_FILE:-$HOME/dev/kubernetes/postgres}"
SECRET_FILE="${SECRET_FILE:-k8s/secret.yaml}"

pg() { grep -E "^[[:space:]]*$1[[:space:]]*=" "$PG_FILE" | head -1 | sed -E 's/^[^=]*=[[:space:]]*//' | tr -d '"' | tr -d '[:space:]'; }
DB_USER=$(pg username); DB_PASS=$(pg password); DB_HOST=$(pg host); DB_PORT=$(pg port); DB_SSL=$(pg sslmode)
DB_NAME="${PG_DB:-printodo}"
AKEY=$(grep -E 'anthropic-api-key:' "$SECRET_FILE" | head -1 | sed -E 's/.*anthropic-api-key:[[:space:]]*//' | tr -d '"' | tr -d '[:space:]')

export DATABASE_URL="postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSL}"
export ANTHROPIC_API_KEY="$AKEY"

go build -o /tmp/printodo-bin .

ARGS=(reclassify)
[ "${RESET:-1}" = "0" ] && ARGS+=(--no-reset)

echo "Reclassifying all items in ${DB_NAME} (reset=${RESET:-1})"
exec /tmp/printodo-bin "${ARGS[@]}"
