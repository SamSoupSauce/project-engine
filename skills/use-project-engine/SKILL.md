---
name: use-project-engine
description: Connect to, dynamically introspect, and control a running Project Engine instance over HTTP JSON-RPC (/rpc) and WebSocket (/ws), locally or via Cloudflare Tunnel.
---

# `use-project-engine` Skill

Use this skill when connecting an external AI agent (Antigravity CLI, Claude Code, Codex/GPT CLI, or local LLMs) to a running **Project Engine** instance to audit runtime documentation/schemas, inspect scenes, query prefabs/components, load/reload scripts, spawn entities, execute shell/Lua commands, stream logs, or capture rendered frames.

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
   If the user has not started the engine yet, instruct them to build/launch the engine executable (`bin/project-engine` / `bin/project-engine.exe` via `make` / `.\build.ps1`, or `go run ./cmd/demo`) with `--serve` (and optional `--tunnel`, `--root`, `--config`, `--init-dirs`) plus an explicit `--token`:
   ```bash
   # Build default engine executable (sets CGO_LDFLAGS_ALLOW automatically):
   make   # or .\build.ps1 on Windows

   # Local agent bridge server (empty token example):
   ./bin/project-engine --serve --address=127.0.0.1:4242 --token=""

   # Using a custom game/script/config/data root directory:
   ./bin/project-engine --root examples/custom-game-root --serve --address=127.0.0.1:4242 --token=""

   # Local server + automated Cloudflare Quick Tunnel:
   ./bin/project-engine --serve --tunnel --address=127.0.0.1:4242 --token=""
   ```
   Startup & runtime notes:
   - **Folder Verification:** On boot the engine verifies `<root>/{assets,scripts,prefabs,config}` exist and fails fast with diagnostics if missing (pass `--init-dirs` to scaffold a new game root).
   - **Graceful Script Errors:** Broken `.lua` scripts are logged to the Developer Console (`[ScriptError] ...`) and ImGui panel without crashing the engine or blocking other scripts.
   - **Automated `--tunnel`:** Checks `PATH` and cache for `cloudflared` (downloading if missing), forwards to `--address`, and prints the `https://<id>.trycloudflare.com` URL in the Console and ImGui overlay.

3. **Auth Token & Sender Attribution:**
   - Check `$PE_SERVER_TOKEN` in your client environment if set, or ask/accept an explicit token value. If unset, default to `""`.
   - **CRITICAL:** The engine rejects requests with `401 Unauthorized` if the token field is omitted — even when launched with `--token=""`.
   - Always pass:
     - HTTP headers: `X-Engine-Token: <token>` (or `Authorization: Bearer <token>`) and `X-Client-ID: <agent_name>` (e.g., `agy`, `claude`, `gemini`) so commands mirror in-game as `[agent:<id>] <cmd>`.
     - WebSocket query params: `?token=<token>&client_id=<agent_name>`.

---

## 2. Mandatory Bootstrap Discovery Sequence (Zero Hallucinations)

**CRITICAL:** Before executing any scene mutation, spawning prefabs, attaching components, or injecting Lua scripts, you **MUST** perform a live capability audit:

1. **Capability Audit (`engine.ping` + `engine.query_docs`):**
   - Immediately upon connecting, call `engine.ping` and then **`engine.query_docs`** (`params: {}` or `docs` in shell) to retrieve the live index of:
     - Engine subsystems (`jolt`, `shell.register`, `prefab.format`, `hooks`, `components`, `prefabs`, `scripts`, `console`, `workspace`, `tutorial`)
     - Native Go shell commands (`help`, `docs`, `tutorial`, `scripts.list`, `scripts.run`, `scripts.reload`, `scripts.unload`, `comp`, `prefabs`, `edit`, etc.)
     - Dynamically registered Lua shell commands (`shell.register(...)`)
     - Registered ECS component schemas (built-in and custom Lua `Component(...)` types such as `Rotator`)
     - Engine lifecycle hooks (`load`, `unload`, `tick`, `event`, `spawn`, `despawn`)
   - Cache the returned capabilities in your working context.

2. **Dynamic Tool State Maintenance:**
   - Whenever you load or reload a Lua script (`scripts.run` or `scripts.reload`), you **MUST** re-query `engine.query_docs` (or `docs` / `help`) to discover newly registered or updated shell commands.
   - If any command fails due to missing arguments, invalid syntax, or type mismatch, immediately query `engine.query_docs` (`{ "topic": "<cmd>" }`) or run `docs <cmd>` to inspect its live `usage` and `description` rather than guessing parameters.

3. **Prohibition of Hallucinated APIs:**
   - You are **strictly prohibited** from guessing engine method names, Jolt physics bindings, shell flags, or ECS component keys.
   - Whenever you need to manipulate an entity or physics/rendering component and do not know its exact layout, first inspect `engine.query_components`, `docs components`, or `docs jolt`.

---

## 3. Transport Selection & Wire Contracts

### A. HTTP JSON-RPC (`POST /rpc`) — Discrete & Introspection Queries
Use `POST /rpc` via `curl` for capability audits (`engine.query_docs`), prefab/component queries, spawning entities, shell/console commands, and frame captures.

```bash
TOKEN="${PE_SERVER_TOKEN:-}"
curl -sS -X POST "${ENGINE_URL:-http://127.0.0.1:4242}/rpc" \
  -H "Content-Type: application/json" \
  -H "X-Engine-Token: ${TOKEN}" \
  -H "X-Client-ID: agy" \
  -d '{
    "jsonrpc": "2.0",
    "id": "agent-req-1",
    "method": "<method_name>",
    "params": {}
  }'
```

### B. WebSocket (`GET /ws`) — High-Frequency Duplex & Live Logs
Use `GET /ws` with `websocat` or `wscat` for continuous entity control, shell verbs (`op: "shell"` or `op: "exec"`), and real-time console log tailing.

```bash
TOKEN="${PE_SERVER_TOKEN:-}"
websocat "${WS_URL:-ws://127.0.0.1:4242}/ws?token=${TOKEN}&client_id=agy"
```

**Inbound Command Frames:**
```json
{ "id": 1, "op": "shell", "cmd": "docs" }
{ "id": 2, "op": "exec", "cmd": "print('hello'); return list_objects()" }
{ "id": 3, "op": "query_docs", "params": { "topic": "workspace" } }
```
Also supports `op`: `"ping"`, `"query_prefabs"`, `"query_components"`, `"query_docs"`, `"spawn_prefab"`, `"capture_frame"`, `"console_exec"`, or `"rpc"`.

**Outbound Reply & Streamed Log Frames:**
```json
{ "id": 1, "ok": true, "res": "..." }
{ "event": "log", "msg": "[agent:agy] docs" }
```

---

## 4. Core Action Playbooks

### 1. Dynamic Documentation & Capability Audit (`engine.query_docs`)
Query the full runtime index or a specific topic (`jolt`, `shell.register`, `prefab.format`, `hooks`, `components`, `workspace`, `tutorial`, or any command name):
```json
{ "jsonrpc": "2.0", "id": "docs-idx", "method": "engine.query_docs", "params": { "topic": "" } }
```
```json
{ "jsonrpc": "2.0", "id": "docs-topic", "method": "engine.query_docs", "params": { "topic": "workspace" } }
```

### 2. Introspection (`engine.ping`, `engine.query_prefabs`, `engine.query_components`)
The default Kitchen Sink Demo exposes `rainbow_cube`, `kitchen_toaster`, `lamp_ceramic`, `nightstand_drawer`, `tiles`, `hud_accent`, `hud_status`, and `hud_crosshair`:
```json
{ "jsonrpc": "2.0", "id": "1", "method": "engine.ping", "params": {} }
{ "jsonrpc": "2.0", "id": "2", "method": "engine.query_prefabs", "params": { "query": "" } }
{ "jsonrpc": "2.0", "id": "3", "method": "engine.query_components", "params": { "query": "" } }
```

### 3. Unified Shell & Script Lifecycle (`engine.console_exec`)
Every command sent to `engine.console_exec` or WebSocket `op: "shell"` runs through `ExecuteWithContext` and mirrors `[agent:<id>] <cmd>` to the in-game console:
```json
{ "jsonrpc": "2.0", "id": "4", "method": "engine.console_exec", "params": { "cmd": "scripts.list" } }
{ "jsonrpc": "2.0", "id": "5", "method": "engine.console_exec", "params": { "cmd": "scripts.run examples/lua/01_hello_shell_commands.lua" } }
{ "jsonrpc": "2.0", "id": "6", "method": "engine.console_exec", "params": { "cmd": "workspace_paths()" } }
```

### 4. Scene Synthesis (`engine.spawn_prefab`)
```json
{
  "jsonrpc": "2.0",
  "id": "7",
  "method": "engine.spawn_prefab",
  "params": {
    "name": "kitchen_toaster",
    "id": "toaster_spawn_1",
    "position": { "x": 1.5, "y": 1.0, "z": -1.0 },
    "pick": true
  }
}
```

### 5. Vision Feedback (`engine.capture_frame`)
```json
{
  "jsonrpc": "2.0",
  "id": "8",
  "method": "engine.capture_frame",
  "params": { "format": "png" }
}
```

---

## 5. Reference Examples
See [examples/](examples/) for ready-to-run shell scripts (`ping.sh`, `query_docs.sh`, `query_prefabs_and_components.sh`, `spawn_prefab.sh`, `console_exec.sh`, `run_script_and_refresh_docs.sh`, `capture_frame.sh`, `ws_stream.sh`).
