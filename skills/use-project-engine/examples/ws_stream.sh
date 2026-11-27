#!/usr/bin/env bash
set -euo pipefail

WS_URL="${WS_URL:-ws://127.0.0.1:4242/ws}"
TOKEN="${PE_SERVER_TOKEN:-}"

# Connects over WebSocket, runs a quick ping + Lua print, and streams live console logs.
printf '%s\n%s\n' \
  '{"id":1,"op":"ping"}' \
  '{"id":2,"op":"exec","cmd":"print(\"[ws-example] connected\"); return list_objects()"}' \
  | websocat "${WS_URL}?token=${TOKEN}"
