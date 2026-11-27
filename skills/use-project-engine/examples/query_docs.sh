#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="${ENGINE_URL:-http://127.0.0.1:4242}"
TOKEN="${PE_SERVER_TOKEN:-}"
TOPIC="${1:-}"
CLIENT_ID="${CLIENT_ID:-agy}"

curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -H "X-Client-ID: ${CLIENT_ID}" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":\"docs-1\",\"method\":\"engine.query_docs\",\"params\":{\"topic\":\"${TOPIC}\"}}"
echo
