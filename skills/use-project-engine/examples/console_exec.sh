#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="${ENGINE_URL:-http://127.0.0.1:4242}"
TOKEN="${PE_SERVER_TOKEN:-}"
CMD="${1:-list_objects()}"

curl -sS -X POST "${ENDPOINT}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -d "$(jq -nc --arg cmd "$CMD" '{jsonrpc:"2.0",id:"exec-1",method:"engine.console_exec",params:{cmd:$cmd}}')"
echo
