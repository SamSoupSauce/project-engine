# Ticket 0022: Dynamic Engine Shell Command Registry, Script Management & Unified Sender Pipeline

## Overview
Implement a native, extensible shell command subsystem within the engine runtime. Decouple high-level CLI interactions from the raw `edit()` code-buffer editor by establishing:
1. A global, thread-safe command registry supporting both engine-native Go commands and dynamically registered Lua commands.
2. Built-in shell commands for inspecting, loading, hot-reloading, and executing scripts from disk or memory.
3. A Lua API permitting scripts to bind custom shell commands with usage flags, help strings, and argument parsing.
4. A strictly unified execution pipeline ensuring parity, zero backdoors, and mandatory sender attribution across all human and agent callers.

## Architecture & Unified Pipeline

### 1. Singular Execution Funnel (Zero Backdoors)
* All inputs—whether typed by a human in the Dear ImGui in-game console or sent remotely by an agent over JSON-RPC (`POST /rpc`) or WebSocket (`/ws`)—must pass through a single entry point:
  ```go
  ExecuteWithContext(ctx ExecutionContext, rawCommand string) (string, error)
  ```
* Agents have no secret or alternate mutation path into the Lua state or scene graph. If an action cannot be run as a console command by a player at the keyboard, an agent cannot run it over the network.

### 2. Universal Invocation Context & Sender Identity
Every command execution must carry an explicit context tracking its origin:
```go
type ExecutionContext struct {
    Sender    string    // e.g. "user", "agent:agy", "agent:ws:127.0.0.1", "script:trap_sys"
    Timestamp time.Time // Exact tick/engine timestamp
    Raw       string    // Raw unparsed command line
    Args      []string  // Whitespace-split arguments honoring quoted substrings
}
```

### 3. Symmetrical Console Feedback & Traceability
* **Live In-Game Mirroring:** When an agent sends a command via JSON-RPC or WebSocket:
  1. The raw command immediately prints to the Dear ImGui console scrollback prefixed with the sender tag (e.g., `[agent:agy] spawn_box 0 5 0`).
  2. The resulting stdout, stderr, and return status print directly beneath it in real time while simultaneously returning across the network wire.
* **Auditability:** Every mutation in the world has a clear, visible author in the console history, preventing unattributed state corruption.

## Built-in Shell Core Commands
The shell provides these fundamental commands out of the box:
* `help [cmd]`: Lists all registered commands with descriptions. If `cmd` is supplied, prints syntax, parameter flags, and examples.
* `scripts.list`: Scans and outputs all `.lua` files in the project script directory along with their active/loaded status.
* `scripts.run <path> [args...]`: Executes a script file immediately within the global Lua environment under the current sender context.
* `scripts.reload <script_name>`: Purges the script's cached chunk and re-runs its initialization hook live without restarting the tick loop.
* `scripts.unload <script_name>`: Calls the script's `unload` lifecycle hook and strips its commands and hooks from the active registry.

## Dynamic Lua Command Registration API
Expose `shell.register` in the LuaJIT environment allowing running scripts to extend the shell registry safely:
```lua
shell.register("spawn_box", {
    description = "Spawns a physics-active test crate at target coordinates",
    usage = "spawn_box <x> <y> <z> [mass]",
    handler = function(ctx, args)
        if #args < 3 then
            return false, "Error: Missing x y z coordinates"
        end
        ...
    end
})
```
* **Cleanup Guarantee:** Unloading or reloading a script must automatically unbind its shell commands to prevent dangling function pointers.

## Transport Integration
* **JSON-RPC (`POST /rpc`):** `engine.console_exec` routes through `ExecuteWithContext(ctx, cmd)` with `Sender = "agent:<client_id>"`.
* **WebSocket (`/ws`):** Command frames (`{ "op": "exec", "cmd": "..." }` and `{ "op": "shell", "cmd": "..." }`) execute through the same pipeline, broadcasting stdout/stderr back across the socket stream and into the local ImGui console.

## Acceptance Criteria
1. Submitting `help` in the Dear ImGui console or via `POST /rpc` returns all native and dynamically registered script commands.
2. Running a command over JSON-RPC or WebSocket prints `[agent:<id>] <cmd>` and its output directly into the in-game Dear ImGui console.
3. Running `scripts.list` outputs the current script directory state accurately.
4. Dynamically registered Lua commands cleanly validate arguments and return structured error messages without crashing the engine loop or causing memory leaks.
5. All commands execute on the main thread during the tick queue drain, preserving full thread safety.

---
### Clarification Log
- **Agent:**
  1. For Ticket 0022, where should `ExecutionContext`, `ShellRegistry`, and the unified `Execute(ctx, rawCommand)` pipeline live?
  2. For Ticket 0022 sender attribution and command compatibility, how should `agent:<id>` and existing console commands behave?
- **User:**
  1. Implement in `pkg/luabridge` (`ShellRegistry`, `ExecutionContext`, `Execute(ctx, rawCommand)`) unified across Console, JSON-RPC `engine.console_exec`, and WebSocket `op:"exec"` / `op:"shell"`.
  2. Mirror `[agent:<id>] <cmd>` + output into Console; determine `<id>` from `params.client_id` / `X-Client-ID` / `?client_id=` (falling back to `rpc` or `ws:<remote_addr>`); keep existing commands alongside `scripts.*` and `shell.register`.

---
### Completion Summary
- **Ticket:** 0022-dynamic-shell-and-unified-sender-pipeline
- **Branch:** 0022-dynamic-shell-and-unified-sender-pipeline
- **Summary of Changes:**
  - Created `ExecutionContext`, `ShellCommandDef`, `LoadedScriptRecord`, `ShellRegistry`, `ParseShellArgs`, and `LuaBridge.ExecuteWithContext(ctx, rawCommand)` in [pkg/luabridge/shell.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/shell.go).
  - Implemented built-in core shell commands `scripts.list`, `scripts.run <path> [args...]`, `scripts.reload <name>`, and `scripts.unload <name>` with module `load`/`unload` hooks, `package.loaded` cleanup, and automatic script-command unbinding.
  - Exposed Lua `shell.register(name, { description, usage, examples, handler })`, `shell.unregister(name)`, and `shell.list()` tracked by owning script path.
  - Updated [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go) and [pkg/luabridge/help.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/help.go) so `RegisterCommand` syncs with `ShellRegistry`, `help` lists all native & Lua shell commands, and `help <cmd>` / `help("<cmd>")` displays command usage/description/source.
  - Routed [pkg/luabridge/console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go) (`Sender: "user"`) and [pkg/luabridge/agent_server.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server.go) (`POST /rpc` `engine.console_exec` and `GET /ws` `op: "exec"` / `op: "shell"`, attributing `agent:<id>` from `X-Client-ID`, `params.client_id`, `?client_id=`, or `rpc`/`ws:<ip>`) through the single `ExecuteWithContext` funnel with live `[agent:<id>] <cmd>` mirroring into the Dear ImGui console.
  - Added integration and `-race` tests in [pkg/luabridge/ticket22_shell_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket22_shell_test.go).
- **Files Modified / Added:**
  - [pkg/luabridge/shell.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/shell.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/help.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/help.go)
  - [pkg/luabridge/console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go)
  - [pkg/luabridge/agent_server.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server.go)
  - [pkg/luabridge/ticket22_shell_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket22_shell_test.go)
  - [tickets/0022-dynamic-shell-and-unified-sender-pipeline.complete.md](file:///C:/Users/samue/Projects/project-engine/tickets/0022-dynamic-shell-and-unified-sender-pipeline.complete.md)
- **Verification Results:**
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -race -count=1 -v -run "TestTicket20|TestTicket21|TestTicket22" ./pkg/luabridge` (exit code 0).
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1` across `./engine`, `./engine/ui`, `./engine/spatial`, `./engine/hazards`, `./pkg/interaction`, `./pkg/props`, `./pkg/tileimporter`, `./cmd/demo`, and `-skip "TestTicket20|TestTicket21EnsureCloudflaredDownloadAndCache|TestTicket22UnifiedPipelineAndSenderMirroring" ./pkg/luabridge`, plus `go vet ./engine ./pkg/luabridge ./cmd/demo` and `go build -o NUL ./cmd/demo` (exit code 0).
