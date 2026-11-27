---
name: use-project-engine
description: Connect to, inspect, and control a running Project Engine instance over HTTP JSON-RPC (/rpc) and WebSocket (/ws), locally or via Cloudflare Tunnel.
---

# `use-project-engine` Skill

Use this skill when connecting an external AI agent (Antigravity CLI, Claude Code, Codex/GPT CLI, or local LLMs) to a running **Project Engine** instance to inspect scenes, query prefabs/components, spawn entities, execute Lua console commands, stream logs, or capture rendered frames.

---

## 1. Mandatory Connection Discovery Protocol

Whenever this skill is activated or the user issues a Project Engine task, you **MUST** execute these discovery steps before issuing any `/rpc` or `/ws` commands:

1. **Check Tunnel Status (Required Prompt):**
   Ask the user:
   > `"Are you running Project Engine locally, or via a Cloudflare Tunnel?"`
   - **If local:** Default endpoints are:
     - HTTP JSON-RPC: `http://127.0.0.1:4242/rpc`
     - WebSocket: `ws://127.0.0.1:4242/ws`
   - **If Cloudflare Tunnel:** Ask the user for the assigned tunnel URL (for example `https://<tunnel-id>.trycloudflare.com`), and target:
     - HTTP JSON-RPC: `https://<tunnel-id>.trycloudflare.com/rpc`
     - WebSocket: `wss://<tunnel-id>.trycloudflare.com/ws`

2. **Verify Engine Execution & Flag Advisory:**
   If the user has not started the engine yet, instruct them to launch `cmd/demo` with `--serve` (and optional `--tunnel`) plus an explicit `--token`:
   ```bash
   # Local agent bridge server (empty token example):
   go run ./cmd/demo --serve --address=127.0.0.1:4242 --token=""

   # Local server + automated Cloudflare Quick Tunnel:
   go run ./cmd/demo --serve --tunnel --address=127.0.0.1:4242 --token=""
   ```
   Automated `--tunnel` behavior:
   - Checks `PATH` and the local cache for `cloudflared`; if missing, automatically downloads the platform-specific binary.
   - Launches an ad-hoc Cloudflare quick tunnel forwarding to `--address` (default `127.0.0.1:4242`).
   - Prints the assigned `https://<id>.trycloudflare.com` URL directly in the in-engine Developer Console and Dear ImGui debug overlay.

3. **Authentication Token Enforcement:**
   - Check `$PE_SERVER_TOKEN` in your client environment if set, or ask/accept an explicit token value.
   - If unset, default to the empty string `""`.
   - **CRITICAL:** The engine server rejects requests with `401 Unauthorized` if the token field is omitted — even when the server was launched with `--token=""`.
   - Always send one of:
     - HTTP header: `Authorization: Bearer <token>` (e.g. `Authorization: Bearer ` when empty) or `X-Engine-Token: <token>`
     - WebSocket query param: `?token=<token>` (e.g. `wss://<endpoint>/ws?token=` when empty) or handshake header `Authorization: Bearer <token>` / `X-Engine-Token: <token>`.

---

## 2. Transport Selection & Wire Contracts

### A. HTTP JSON-RPC (`POST /rpc`) — Discrete / Rich Queries
Use `POST /rpc` via `curl` or an HTTP client for introspection, prefab queries, spawning entities, one-off Lua execution, and frame captures.

**Request Template:**
```bash
TOKEN="${PE_SERVER_TOKEN:-}"
curl -sS -X POST "${ENGINE_URL:-http://127.0.0.1:4242}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -d '{
    "jsonrpc": "2.0",
    "id": "agent-req-1",
    "method": "<method_name>",
    "params": {}
  }'
```
*(You may also pass `-H "Authorization: Bearer ${TOKEN}"`.)*

**Success Envelope:**
```json
{
  "jsonrpc": "2.0",
  "id": "agent-req-1",
  "result": {
    "status": "success",
    "data": { ... },
    "logs": ["console output line 1"],
    "tick": 45892,
    "timing_ms": 1.42
  }
}
```

**Error Envelope:**
```json
{
  "jsonrpc": "2.0",
  "id": "agent-req-1",
  "error": {
    "code": -32000,
    "message": "error message",
    "data": { "traceback": "..." }
  }
}
```

### B. WebSocket (`GET /ws`) — High-Frequency Duplex & Live Logs
Use `GET /ws` with `websocat` or `wscat` for continuous entity control, rapid loops, and real-time console log tailing.

```bash
TOKEN="${PE_SERVER_TOKEN:-}"
websocat "${WS_URL:-ws://127.0.0.1:4242}/ws?token=${TOKEN}"
```

**Inbound Command Frames:**
```json
{ "id": 1, "op": "exec", "cmd": "print('hello'); return selected" }
```
Also supports `op`: `"ping"`, `"query_prefabs"`, `"query_components"`, `"spawn_prefab"`, `"capture_frame"`, `"console_exec"`, or `"rpc"` (`{ "id": 2, "op": "rpc", "method": "engine.ping", "params": {} }`).

**Outbound Reply & Streamed Log Frames:**
```json
{ "id": 1, "ok": true, "res": null }
{ "event": "log", "msg": "hello" }
```

---

## 3. Core Action Playbooks

### 1. Introspection (`engine.ping`, `engine.query_prefabs`, `engine.query_components`)
Always inspect engine health, available prefabs, and component schemas before mutating a scene.

```json
{ "jsonrpc": "2.0", "id": "1", "method": "engine.ping", "params": {} }
```
```json
{ "jsonrpc": "2.0", "id": "2", "method": "engine.query_prefabs", "params": { "query": "" } }
```
```json
{ "jsonrpc": "2.0", "id": "3", "method": "engine.query_components", "params": { "query": "" } }
```

### 2. Scene Synthesis (`engine.spawn_prefab`)
Instantiate a stored prefab from `prefabs/` at world coordinates `(x, y, z)`:
```json
{
  "jsonrpc": "2.0",
  "id": "4",
  "method": "engine.spawn_prefab",
  "params": {
    "name": "rainbow_cube",
    "id": "cube_spawn_1",
    "position": { "x": 2.0, "y": 1.0, "z": -1.5 },
    "pick": true
  }
}
```

### 3. Runtime Script & Console Execution (`engine.console_exec`)
Execute arbitrary Lua or console commands (`help`, `pick`, `comp`, `prefabs`, `scripts`, `edit`) synchronously on the engine tick loop:
```json
{
  "jsonrpc": "2.0",
  "id": "5",
  "method": "engine.console_exec",
  "params": {
    "cmd": "pick('cube'):GetComponent(Transform):SetPosition(0, 2, 0); return list_objects()"
  }
}
```

### 4. Vision Feedback (`engine.capture_frame`)
Capture the rendered OpenGL backbuffer as base64-encoded PNG (default) or JPEG (`"format": "jpeg"`):
```json
{
  "jsonrpc": "2.0",
  "id": "6",
  "method": "engine.capture_frame",
  "params": { "format": "png" }
}
```
Response `result.data` returns `{ "format": "png", "width": 1280, "height": 720, "data": "<base64>" }`.

---

## 4. Reference Examples
See [examples/](examples/) for ready-to-run shell scripts (`ping.sh`, `query_prefabs_and_components.sh`, `spawn_prefab.sh`, `console_exec.sh`, `capture_frame.sh`, `ws_stream.sh`).
