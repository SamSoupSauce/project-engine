#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="${ENGINE_URL:-http://127.0.0.1:4242}"
TOKEN="${PE_SERVER_TOKEN:-}"
QUERY="${1:-}"

echo "=== Prefabs ==="
curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":\"q-prefabs\",\"method\":\"engine.query_prefabs\",\"params\":{\"query\":\"${QUERY}\"}}"
echo

echo "=== Components ==="
curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":\"q-comps\",\"method\":\"engine.query_components\",\"params\":{\"query\":\"${QUERY}\"}}"
echo
