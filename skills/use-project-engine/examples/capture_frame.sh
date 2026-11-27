#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="${ENGINE_URL:-http://127.0.0.1:4242}"
TOKEN="${PE_SERVER_TOKEN:-}"
OUT_FILE="${1:-frame.png}"

RESP=$(curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -d '{"jsonrpc":"2.0","id":"cap-1","method":"engine.capture_frame","params":{"format":"png"}}')

echo "${RESP}" | jq -r '.result.data.data' | base64 -d > "${OUT_FILE}"
echo "Saved frame to ${OUT_FILE}"
