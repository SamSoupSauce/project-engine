#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="${ENGINE_URL:-http://127.0.0.1:4242}"
TOKEN="${PE_SERVER_TOKEN:-}"
PREFAB_NAME="${1:-rainbow_cube}"

curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": \"spawn-1\",
    \"method\": \"engine.spawn_prefab\",
    \"params\": {
      \"name\": \"${PREFAB_NAME}\",
      \"position\": { \"x\": 2.0, \"y\": 1.0, \"z\": 0.0 },
      \"pick\": true
    }
  }"
echo
