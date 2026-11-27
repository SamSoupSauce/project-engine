# Ticket 0020: Dual Transport Agent Bridge Server (WebSocket + JSON-RPC)

## Overview
Implement an internal network server enabling external AI agents (e.g., Antigravity CLI, web models, local SLMs) to inspect and control the running engine instance. The server must operate concurrently with the engine, expose both HTTP/JSON-RPC (for heavy/discrete operations like queries, batch updates, and frame grabs) and WebSocket (for high-frequency duplex operations like live console streams and continuous entity inputs), support required token-based authentication, and guarantee complete thread safety by dispatching all mutations onto the main engine tick loop.

## Functional Requirements

### 1. Dual Transport Endpoints
* **HTTP JSON-RPC:** Listen for `POST /rpc` handling discrete, request-response execution.
* **WebSocket:** Listen on `GET /ws` for persistent, bidirectional event and command streaming.
* **Configurable Address:** Default listener to `127.0.0.1:4242` (bindable to `0.0.0.0` or configurable via `--address` CLI flag to facilitate local setups and Cloudflare Tunnel ingresses).

### 2. Authentication Contract
* Require `--token=<string>` on server launch when `--serve` is enabled (no environment variable fallback; `--token` must be explicitly passed even if empty `""`).
* **Enforcement Rule:** The client *must* pass the token field in every request regardless of whether it is an empty string:
  * For HTTP: Header `Authorization: Bearer <token>` or `X-Engine-Token: <token>`.
  * For WebSocket: Query param `?token=<token>` or connection handshake header (`Authorization: Bearer <token>` or `X-Engine-Token: <token>`).
  * If the token field is missing or does not match the configured token (even when empty vs missing), reject immediately with HTTP `401 Unauthorized`.

### 3. Execution Model & Thread Safety (MPSC Loop Queue)
* Engine state (LuaJIT state, OpenGL context, ECS registries, and Jolt physics instances) is non-reentrant and strictly confined to the main Go loop thread.
* Network handlers running on separate goroutines **must not** touch engine state directly.
* Incoming agent payloads must be parsed, validated, and pushed into a thread-safe, buffered channel (`CommandQueue`).
* At the top of every engine `Tick()`:
  1. The main loop drains all pending commands from `CommandQueue`.
  2. Drained commands are executed synchronously against the scene graph, Lua runtime, or prefab registry.
  3. Execution results, console log outputs, or error states are sent back to the awaiting goroutine via an internal response channel / promise pattern to be serialized back to the client.

### 4. Wire Protocols & Schemas

#### A. JSON-RPC (`POST /rpc`) - Fat Expansive Schema
Optimized for deep introspection, file payloads, and detailed telemetry.
* **Request:**
  ```json
  {
    "jsonrpc": "2.0",
    "id": "req-101",
    "method": "engine.console_exec" | "engine.query_prefabs" | "engine.query_components" | "engine.spawn_prefab" | "engine.capture_frame",
    "params": { ... }
  }
  ```

* **Response (Success):**
  ```json
  {
    "jsonrpc": "2.0",
    "id": "req-101",
    "result": {
      "status": "success",
      "data": { ... },
      "logs": ["console output line 1", "debug info line 2"],
      "tick": 45892,
      "timing_ms": 1.42
    }
  }
  ```

* **Response (Error):** Standard JSON-RPC error object containing code, message, and execution traceback.

#### B. WebSocket (`/ws`) - Slim Fast Schema
Optimized for high-frequency control frames and real-time event subscriptions.

* **Inbound Frame:**
  ```json
  { "id": 1, "op": "exec", "cmd": "player:move(0, 1)" }
  ```
  Also supports dispatching any `engine.*` method (e.g., `ping`, `query_prefabs`, `query_components`, `spawn_prefab`, `capture_frame`, `console_exec`).

* **Outbound Frame:**
  ```json
  { "id": 1, "ok": true, "res": null }
  ```

* Streamed console log lines push as raw, compact messages:
  ```json
  { "event": "log", "msg": "[physics] joint snapped on entity_42" }
  ```

### 5. Core Methods to Expose
* `engine.ping`: Returns engine uptime, current tick, and loaded scene name.
* `engine.console_exec`: Runs an arbitrary Lua command via the console runtime (`edit()` context).
* `engine.query_prefabs`: Returns list of available JSON prefab definitions.
* `engine.query_components`: Lists all registered ECS components and their schema layouts.
* `engine.spawn_prefab`: Instantiates a named prefab at specific coordinate vectors `(x, y, z)`.
* `engine.capture_frame`: Returns the current OpenGL backbuffer as a base64-encoded PNG (default) or JPEG (for vision models), or an explicit JSON-RPC error when no active OpenGL window exists.

## Acceptance Criteria
1. Engine boots with `--serve --token=... [--address=...]` and initializes listener on `127.0.0.1:4242` by default without blocking the main render loop; launching with `--serve` without `--token` fails fast.
2. Sending a `POST /rpc` command to execute a Lua script returns the command's logs and updates engine state on the subsequent tick without race conditions or memory faults (`go test -race` passes).
3. WebSocket connection establishes via standard `ws://` client (`github.com/gorilla/websocket`), accepts rapid command frames (including `exec` and `engine.*` ops), and receives live streamed console logs.
4. Requests lacking the matching authentication token field (even when configured token is empty `""`) are rejected with `401 Unauthorized`.
5. Cloudflare Tunnel cleanly forwards HTTPS and WSS traffic through to the local listener without transport drops or disconnects (WebSocket ping/pong keepalives and tunnel-compatible origin handling).

---
### Clarification Log
- **Agent:**
  1. WebSocket Dependency: `go.mod` currently has no WebSocket library. Would you prefer adding a dependency such as `github.com/gorilla/websocket` (or `github.com/coder/websocket`), or implementing a zero-dependency RFC 6455 WebSocket handler using the Go standard library?
  2. Empty Token Enforcement (`--server-token` / `PE_SERVER_TOKEN`): When the configured server token defaults to `""`, should requests that explicitly include an empty token (`Authorization: Bearer `, `X-Engine-Token: `, or `?token=`) be accepted while requests omitting the header/query param altogether are rejected with `401 Unauthorized`?
  3. WebSocket Operations & Log Streaming (`GET /ws`): Besides `op: "exec"`, should `/ws` also allow invoking the other `engine.*` methods, and should `{ "event": "log", "msg": "..." }` stream all `Console.Print` / `LuaBridge.EmitConsole` output lines in real time?
  4. Frame Capture (`engine.capture_frame`) & Bind Flags: Should `engine.capture_frame` default to base64 PNG (with optional `format`), returning a clean JSON-RPC error without an active OpenGL window (while supporting a mock/offscreen frame reader for unit tests)? Should `cmd/demo` wire `--serve`, `--serve-addr`, and `--server-token` (with `PE_SERVER_TOKEN` fallback)?
  5. Truncated Line at Bottom of Ticket: Criterion 5 ends mid-word (`without transport drop`) — please confirm keepalive/tunnel expectations.
- **User:**
  1. Use a library
  2. Yes. Require token field, just empty. Even for server launch.
  3. Yes, and yes stream console for WS.
  4. Yes, and Yes, but no fallback. Also just call it --token --serve and --address
  5. Just keep it alive and working if possible.

---
### Completion Summary
- **Ticket:** 0020-dual-transport-agent-bridge-server
- **Branch:** 0020-dual-transport-agent-bridge-server
- **Summary of Changes:**
  - Added `BaseScene.AddPreTickHook`, `BaseScene.GetTickCount`, and top-of-`Tick()` pre-tick hook drainage in [engine/scene.go](file:///C:/Users/samue/Projects/project-engine/engine/scene.go).
  - Added `EncodeRGBABufferBase64` and `CaptureGLBackbuffer` in [engine/frame_capture.go](file:///C:/Users/samue/Projects/project-engine/engine/frame_capture.go) plus `GLApp.OnPostRender` hook in [engine/gl_window.go](file:///C:/Users/samue/Projects/project-engine/engine/gl_window.go) for main-thread OpenGL backbuffer capture before `SwapBuffers`.
  - Added `Console.AddLogSink` in [pkg/luabridge/console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go) and `LuaBridge.AddLogSink` in [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go) for lock-safe real-time console log streaming.
  - Added `github.com/gorilla/websocket` (`v1.5.3`) to [go.mod](file:///C:/Users/samue/Projects/project-engine/go.mod) / [go.sum](file:///C:/Users/samue/Projects/project-engine/go.sum).
  - Implemented `AgentServer` in [pkg/luabridge/agent_server.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server.go) exposing `POST /rpc` (JSON-RPC 2.0) and `GET /ws` (WebSocket with ping/pong keepalives, `CheckOrigin` for Cloudflare Tunnel, and real-time `{ "event": "log", "msg": "..." }` streaming), strict token-field enforcement (even when `--token=""`), MPSC `CommandQueue` drained at the top of `Tick()`, and `engine.ping`, `engine.console_exec`, `engine.query_prefabs`, `engine.query_components`, `engine.spawn_prefab`, and `engine.capture_frame`.
  - Wired `--serve`, `--address` (default `127.0.0.1:4242`), and required `--token` (no env fallback) into [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go) with flag tests in [cmd/demo/main_test.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main_test.go) and full HTTP/WS/auth/`-race` tests in [pkg/luabridge/agent_server_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server_test.go).
- **Files Modified / Added:**
  - [go.mod](file:///C:/Users/samue/Projects/project-engine/go.mod)
  - [go.sum](file:///C:/Users/samue/Projects/project-engine/go.sum)
  - [engine/scene.go](file:///C:/Users/samue/Projects/project-engine/engine/scene.go)
  - [engine/frame_capture.go](file:///C:/Users/samue/Projects/project-engine/engine/frame_capture.go)
  - [engine/gl_window.go](file:///C:/Users/samue/Projects/project-engine/engine/gl_window.go)
  - [pkg/luabridge/console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/agent_server.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server.go)
  - [pkg/luabridge/agent_server_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server_test.go)
  - [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go)
  - [cmd/demo/main_test.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main_test.go)
  - [tickets/0020-dual-transport-agent-bridge-server.complete.md](file:///C:/Users/samue/Projects/project-engine/tickets/0020-dual-transport-agent-bridge-server.complete.md)
- **Verification Results:**
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -race -count=1 -v -run TestTicket20 ./pkg/luabridge` (all 3 test suites passed with `-race`, exit code 0).
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 ./engine ./engine/ui ./engine/spatial ./engine/hazards ./engine/components ./pkg/interaction ./pkg/props ./pkg/tileimporter ./cmd/demo` and `-skip TestTicket20 ./pkg/luabridge`, `go vet ./engine ./pkg/luabridge ./cmd/demo`, and `go build -o NUL ./cmd/demo` (exit code 0).
