#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="${ENGINE_URL:-http://127.0.0.1:4242}"
TOKEN="${PE_SERVER_TOKEN:-}"
CLIENT_ID="${CLIENT_ID:-agy}"
SCRIPT_PATH="${1:-examples/lua/01_hello_shell_commands.lua}"

echo "=== 1. Running script: ${SCRIPT_PATH} ==="
curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -H "X-Client-ID: ${CLIENT_ID}" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":\"run-script\",\"method\":\"engine.console_exec\",\"params\":{\"cmd\":\"scripts.run ${SCRIPT_PATH}\"}}"
echo

echo "=== 2. Refreshing runtime docs index ==="
curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -H "X-Client-ID: ${CLIENT_ID}" \
  -d '{"jsonrpc":"2.0","id":"refresh-docs","method":"engine.query_docs","params":{"topic":""}}'
echo
