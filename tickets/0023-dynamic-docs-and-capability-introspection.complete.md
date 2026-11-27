# Ticket 0023: Dynamic Engine Documentation Querying & Capability Introspection in Agent Skill

## Overview
Update the `use-project-engine` skill and engine introspection endpoints to ensure any connected AI agent dynamically queries engine documentation, available shell commands, ECS components, and prefab schemas at runtime. The agent must not rely on stale or assumed hardcoded API signatures; it must maintain a dynamic local state of active tools and capabilities during its session.

## Functional Requirements

### 1. Engine Documentation Registry & Dynamic Query Verb
* Expose an engine introspection method via JSON-RPC (`engine.query_docs`), WebSocket (`op: "query_docs"` / `op: "shell"`), and the shell registry:
  * `docs [topic|command|component]`: Returns formatted plain-text/Markdown documentation and structured JSON metadata.
  * If called with no arguments (`docs`), returns an index of all available documentation categories, registered shell verbs, component schemas, and engine subsystems.
  * If called with a specific topic (e.g., `docs jolt`, `docs shell.register`, `docs prefab.format`, `docs hooks`, `docs components`, `docs <cmd>`), returns the exact usage syntax, parameter types, default values, and execution constraints.
* The documentation engine must dynamically generate docs for:
  * Native Go shell commands.
  * Dynamically registered Lua shell commands (pulling from `description` and `usage` fields registered via `shell.register`).
  * Registered ECS components and their serialized JSON fields.
  * In-engine runtime hooks (`load`, `unload`, `tick`, `event`, `spawn`, `despawn`).

### 2. Skill Directive: Mandatory Bootstrap Discovery Sequence
Update `skills/use-project-engine/SKILL.md` to mandate a strict bootstrap phase before an agent executes any scene mutation or script injection:

1. **Capability Audit:**
   * Upon establishing a connection (over local socket or Cloudflare Tunnel), the agent must immediately call `engine.query_docs` (or execute `docs`).
   * The agent must cache the returned list of verbs, loaded components, and schemas in its working context.
2. **Dynamic Tool State Maintenance:**
   * When a new Lua script is loaded (`scripts.run`) or reloaded (`scripts.reload`), the agent must re-query `docs` or `help` to detect newly exposed shell commands.
   * If an engine command fails due to missing arguments or type mismatch, the agent must query `docs <cmd>` to inspect the current signature rather than guessing parameters.
3. **Prohibition of Hallucinated APIs:**
   * The agent is strictly prohibited from guessing engine method names, Jolt physics bindings, or ECS component keys.
   * If an agent needs to perform an action and does not know the component layout, it must inspect `engine.query_components` or `docs components` first.

### 3. Transport Payloads
* **JSON-RPC (`POST /rpc`):** Method `engine.query_docs` with `{ "topic": "<optional_topic_string>" }` returning structured JSON fields (`topic`, `type`, `description`, `usage`, `source`, `items`/`sections`) plus `formatted`.
* **Shell Interface (`ImGui Console` / `WebSocket`):** `[agent:gemini] docs` prints formatted, human-readable documentation into the console scrollback.

## Acceptance Criteria
1. Running `docs` in the shell returns a complete list of engine subsystems, shell commands, and component schemas.
2. Dynamically loaded Lua scripts registering shell commands immediately reflect their usage docs in `docs <cmd>`.
3. The `use-project-engine` skill instructions explicitly forbid assumed API calls and enforce querying `engine.query_docs` during session initialization.
4. An external agent following the skill cleanly retrieves documentation and formats subsequent commands matching the queried schemas.

---
### Clarification Log
- **Agent:**
  1. For Ticket 0023 (`engine.query_docs`), how should index and topic responses be structured?
  2. Which documentation topics should `docs [topic]` / `engine.query_docs` include?
  3. How should Ticket 0023 update `skills/use-project-engine/`?
- **User:**
  1. Return structured JSON fields (`topic`, `type`, `description`, `usage`, `source`, `items`/`sections`) plus a formatted string for console display.
  2. Register built-in subsystem topics (`jolt`, `shell.register`, `prefab.format`, `hooks`, `components`) and dynamically merge native commands, Lua `shell.register` commands, and ECS components.
  3. Update `skills/use-project-engine/SKILL.md` with the mandatory bootstrap docs audit & anti-hallucination rules, and add `examples/query_docs.sh`.

---
### Completion Summary
- **Ticket:** 0023-dynamic-docs-and-capability-introspection
- **Branch:** 0023-dynamic-docs-and-capability-introspection
- **Summary of Changes:**
  - Created dynamic documentation registry and query engine in [pkg/luabridge/docs_registry.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/docs_registry.go) (`LuaBridge.QueryDocs`, `DocsQueryResult`, `docs` shell command, `docs([topic])` Lua global) indexing subsystems (`jolt`, `shell.register`, `prefab.format`, `hooks`, `components`, `prefabs`, `scripts`, `console`), native & dynamic Lua `shell.register` commands, ECS component schemas, and runtime lifecycle hooks (`load`, `unload`, `tick`, `event`, `spawn`, `despawn`).
  - Wired `registerDocsAPIs()` into [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go) and added `engine.query_docs` (JSON-RPC `/rpc` and WebSocket `op: "query_docs"`) in [pkg/luabridge/agent_server.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server.go).
  - Updated [skills/use-project-engine/SKILL.md](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/SKILL.md) with the Mandatory Bootstrap Discovery Sequence (`engine.query_docs` capability audit, dynamic tool state refresh after `scripts.run`/`scripts.reload`, and prohibition of hallucinated APIs) and added [skills/use-project-engine/examples/query_docs.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/query_docs.sh).
  - Added unit, dynamic reload/unload, JSON-RPC, WebSocket mirroring, and `-race` tests in [pkg/luabridge/ticket23_docs_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket23_docs_test.go).
- **Files Modified / Added:**
  - [pkg/luabridge/docs_registry.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/docs_registry.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/agent_server.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/agent_server.go)
  - [pkg/luabridge/ticket23_docs_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket23_docs_test.go)
  - [skills/use-project-engine/SKILL.md](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/SKILL.md)
  - [skills/use-project-engine/examples/query_docs.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/query_docs.sh)
  - [tickets/0023-dynamic-docs-and-capability-introspection.complete.md](file:///C:/Users/samue/Projects/project-engine/tickets/0023-dynamic-docs-and-capability-introspection.complete.md)
- **Verification Results:**
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -race -count=1 -v -run "TestTicket20|TestTicket21|TestTicket22|TestTicket23" ./pkg/luabridge` (exit code 0).
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1` across `./engine`, `./engine/ui`, `./engine/spatial`, `./engine/hazards`, `./pkg/interaction`, `./pkg/props`, `./pkg/tileimporter`, `./cmd/demo`, and `-skip "TestTicket20|TestTicket21EnsureCloudflaredDownloadAndCache|TestTicket22UnifiedPipelineAndSenderMirroring|TestTicket23DynamicDocsRegistryAndTransports" ./pkg/luabridge`, plus `go vet ./engine ./pkg/luabridge ./cmd/demo` and `go build -o NUL ./cmd/demo` (exit code 0).
