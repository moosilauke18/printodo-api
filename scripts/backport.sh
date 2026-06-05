#!/usr/bin/env bash
# Run the notes backport against the managed Postgres, wiring credentials from
# your existing local files so no secrets are typed or committed.
#
# Usage:
#   DRY=1 scripts/backport.sh                 # preview only (no DB, no AI, no writes)
#   scripts/backport.sh                       # import the newer half: last 12 months
#   scripts/backport.sh backport 24 12        # import an older half: 24..12 months ago
#
# Args: [notes-file=backport] [start-months=12] [end-months=0]
set -euo pipefail
cd "$(dirname "$0")/.."

PG_FILE="${PG_FILE:-$HOME/dev/kubernetes/postgres}"
SECRET_FILE="${SECRET_FILE:-k8s/secret.yaml}"
NOTES_FILE="${1:-backport}"
START_MONTHS="${2:-12}"
END_MONTHS="${3:-0}"

# Pull "key = value" fields from the DO postgres creds file.
pg() { grep -E "^[[:space:]]*$1[[:space:]]*=" "$PG_FILE" | head -1 | sed -E 's/^[^=]*=[[:space:]]*//' | tr -d '"' | tr -d '[:space:]'; }
DB_USER=$(pg username); DB_PASS=$(pg password); DB_HOST=$(pg host); DB_PORT=$(pg port); DB_SSL=$(pg sslmode)
# Target the dedicated printodo database (override with PG_DB=... if needed).
DB_NAME="${PG_DB:-printodo}"

# Pull the Anthropic key from the k8s secret (stringData).
AKEY=$(grep -E 'anthropic-api-key:' "$SECRET_FILE" | head -1 | sed -E 's/.*anthropic-api-key:[[:space:]]*//' | tr -d '"' | tr -d '[:space:]')

export DATABASE_URL="postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSL}"
export ANTHROPIC_API_KEY="$AKEY"

go build -o /tmp/printodo-bin .

ARGS=(backport --start-months "$START_MONTHS" --end-months "$END_MONTHS")
[ "${DRY:-0}" = "1" ] && ARGS+=(--dry-run)
ARGS+=("$NOTES_FILE")

echo "Importing '$NOTES_FILE' into ${DB_NAME} over ${START_MONTHS}..${END_MONTHS} months ago (DRY=${DRY:-0})"
exec /tmp/printodo-bin "${ARGS[@]}"
