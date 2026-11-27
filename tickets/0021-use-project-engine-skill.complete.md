# Ticket 0021: Agent Skill Specification (`use-project-engine`) & `--tunnel` Quick-Tunnel Support

## Overview
Create the universal agent skill `use-project-engine`. This skill teaches any LLM/CLI agent (Google Antigravity, Claude Code, GPT CLI, open-source local agents) how to detect, connect to, introspect, and manipulate an active Project Engine instance over both local network interfaces and public Cloudflare Tunnels. Additionally, implement `--tunnel` in `cmd/demo` to automatically locate or download `cloudflared`, launch a quick tunnel targeting `--address`, and surface the assigned tunnel URL in the console and Dear ImGui overlay.

## Requirements & Behavioral Directives

### 1. Connection Discovery Protocol
When the skill initializes or when the user issues an engine-related command, the agent MUST execute the following discovery sequence:

1. **Check Tunnel Status:**
   * Prompt the user: `"Are you running Project Engine locally, or via a Cloudflare Tunnel?"`
   * If local: default target endpoint to `http://127.0.0.1:4242` and `ws://127.0.0.1:4242/ws`.
   * If tunnel: request the tunnel URL (e.g., `https://<tunnel-id>.trycloudflare.com`).

2. **Verify Engine Execution & Flag Advisory:**
   * If the user has not started the engine, instruct them to launch it with `--serve` (and optional `--tunnel`) and explicit `--token`:
     ```bash
     go run ./cmd/demo --serve --address=127.0.0.1:4242 --token=""
     # or with automated Cloudflare Quick Tunnel:
     go run ./cmd/demo --serve --tunnel --address=127.0.0.1:4242 --token=""
     ```
   * Explain the automated `--tunnel` behavior:
     * The engine checks for `cloudflared` locally; if missing, it downloads the platform-specific binary.
     * The engine launches an ad-hoc quick tunnel pointing to `--address` (default `127.0.0.1:4242`).
     * The assigned tunnel URL prints directly in the engine runtime console and the Dear ImGui debug overlay.

3. **Auth Token Enforcement:**
   * Agent client scripts/workflows read `PE_SERVER_TOKEN` from the client environment if present, or accept an explicit token; if unset, default to `""`.
   * Always provide the token field in headers/query params (`Authorization: Bearer <token>`, `X-Engine-Token: <token>`, or `?token=<token>`) even when empty `""`, matching the server's mandatory auth check.

### 2. Supported Transport Workflows
* **HTTP JSON-RPC (`POST /rpc`):** Introspection queries, state snapshots, spawning discrete prefabs, running isolated console commands, capturing render buffer frames.
* **WebSocket (`GET /ws`):** Continuous entity movement/teleop, real-time log tailing, rapid stress loops (`websocat` / `wscat` with `?token=<token>`).

### 3. Core Action Playbooks
1. **Introspection (`engine.ping`, `engine.query_prefabs`, `engine.query_components`)**
2. **Scene Synthesis (`engine.spawn_prefab`)**
3. **Runtime Script Execution (`engine.console_exec`)**
4. **Vision Feedback (`engine.capture_frame`)**

## Deliverables
1. `skills/use-project-engine/SKILL.md`: Main instruction file containing the discovery protocol, command contracts, and JSON payload templates.
2. `skills/use-project-engine/examples/`: Reference curl and WebSocket invocation scripts demonstrating connection checks, prefab queries, prefab spawning, console execution, and frame capture.
3. `--tunnel` flag in `cmd/demo` backed by `TunnelManager` (`pkg/luabridge/tunnel.go`) and Dear ImGui overlay URL display (`engine/imgui_overlay.go`).
4. Updated project documentation (`README.md`) covering `--serve`, `--tunnel`, and external agent compatibility.

## Acceptance Criteria
1. Any external agent reading `SKILL.md` asks the required tunnel status question before issuing commands.
2. Agent documentation & examples cleanly target `engine.ping` and core methods over both `127.0.0.1:4242` and a Cloudflare Tunnel domain.
3. Commands and examples always pass the mandatory token parameter without client errors.
4. `--tunnel` locates or downloads `cloudflared`, launches a quick tunnel, surfaces the tunnel URL in console & ImGui overlay, and cleans up on shutdown.

---
### Clarification Log
- **Agent:**
  1. Ticket 0021 mentions `./project-engine --tunnel=true` (auto-checking/downloading `cloudflared` and displaying the tunnel URL in console & ImGui) in addition to the skill files. Should Ticket 0021 also implement `--tunnel` in Go?
  2. In Ticket 0020, server flags were finalized as `--serve`, `--address`, and required `--token` (no server `PE_SERVER_TOKEN` fallback) on `cmd/demo`. How should Ticket 0021 docs/examples reference launching the engine?
  3. Where should the `use-project-engine` skill files be installed?
- **User:**
  1. Implement `--tunnel` in `cmd/demo` as well (check/download `cloudflared`, launch quick tunnel to `--address`, print URL in console & ImGui overlay) alongside `SKILL.md`, `examples/`, and `README.md`.
  2. Update `SKILL.md`, `examples/`, and `README.md` to use `cmd/demo --serve --address=127.0.0.1:4242 --token=""` (and optional `--tunnel`), while letting agent client examples read `$PE_SERVER_TOKEN` defaulting to `""`.
  3. Create in repo at `skills/use-project-engine/` only.

---
### Completion Summary
- **Ticket:** 0021-use-project-engine-skill
- **Branch:** 0021-use-project-engine-skill
- **Summary of Changes:**
  - Implemented `TunnelManager` (`EnsureCloudflared`, `Start`, `StartWithCommand`, `Stop`, `GetURL`, `ParseTryCloudflareURL`, `NormalizeTunnelUpstreamURL`) in [pkg/luabridge/tunnel.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/tunnel.go) to auto-locate or download `cloudflared`, launch a Cloudflare quick tunnel to `--address`, stream stdout/stderr to detect `https://<id>.trycloudflare.com`, log the assigned URL to the Developer Console, and notify the ImGui overlay.
  - Added thread-safe `SetTunnelURL` / `GetTunnelURL` and ImGui overlay rendering in [engine/imgui_overlay.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_overlay.go).
  - Wired `--tunnel` into [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go) (implying `--serve` and enforcing `--token`) with flag tests in [cmd/demo/main_test.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main_test.go) and unit tests in [pkg/luabridge/tunnel_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/tunnel_test.go).
  - Created [skills/use-project-engine/SKILL.md](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/SKILL.md) and reference scripts in `skills/use-project-engine/examples/` (`ping.sh`, `query_prefabs_and_components.sh`, `spawn_prefab.sh`, `console_exec.sh`, `capture_frame.sh`, `ws_stream.sh`).
  - Updated [README.md](file:///C:/Users/samue/Projects/project-engine/README.md) with `--serve`, `--tunnel`, `--address`, `--token`, and external agent skill documentation.
- **Files Modified / Added:**
  - [pkg/luabridge/tunnel.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/tunnel.go)
  - [pkg/luabridge/tunnel_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/tunnel_test.go)
  - [engine/imgui_overlay.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_overlay.go)
  - [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go)
  - [cmd/demo/main_test.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main_test.go)
  - [skills/use-project-engine/SKILL.md](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/SKILL.md)
  - [skills/use-project-engine/examples/ping.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/ping.sh)
  - [skills/use-project-engine/examples/query_prefabs_and_components.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/query_prefabs_and_components.sh)
  - [skills/use-project-engine/examples/spawn_prefab.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/spawn_prefab.sh)
  - [skills/use-project-engine/examples/console_exec.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/console_exec.sh)
  - [skills/use-project-engine/examples/capture_frame.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/capture_frame.sh)
  - [skills/use-project-engine/examples/ws_stream.sh](file:///C:/Users/samue/Projects/project-engine/skills/use-project-engine/examples/ws_stream.sh)
  - [README.md](file:///C:/Users/samue/Projects/project-engine/README.md)
  - [tickets/0021-use-project-engine-skill.complete.md](file:///C:/Users/samue/Projects/project-engine/tickets/0021-use-project-engine-skill.complete.md)
- **Verification Results:**
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -race -count=1 -v -run "TestTicket20|TestTicket21" ./pkg/luabridge` (exit code 0).
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1` across `./engine`, `./engine/ui`, `./engine/spatial`, `./engine/hazards`, `./pkg/interaction`, `./pkg/props`, `./pkg/tileimporter`, `./cmd/demo`, and `-skip "TestTicket20|TestTicket21EnsureCloudflaredDownloadAndCache" ./pkg/luabridge`, plus `go vet ./engine ./pkg/luabridge ./cmd/demo` and `go build -o NUL ./cmd/demo` (exit code 0).
