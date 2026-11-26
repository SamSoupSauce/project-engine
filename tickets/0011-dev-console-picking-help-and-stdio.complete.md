# Ticket 0011: Dev Console Object Picking, `help()` Function & Stdout/Stderr Redirection

## Status
Complete

## Context
Currently the developer console in [console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go), [bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go), and [imgui_console.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_console.go) initializes with a literal greeting referencing `'help'` even though no callable `help()` function exists in Lua or the command registry. Furthermore:
- Lua `print(...)` and Go `os.Stdout` / `os.Stderr` (including `log` output) write directly to the terminal instead of appearing inside the in-engine `DebugConsoleWindow` scrollback.
- There is no integrated way to click/raycast-pick a 3D `GameObject` in the viewport (or pick via command/Lua) and bind it into the developer console for live inspection and manipulation.

## Scope of Work

### 1. Real `help()` Function (`pkg/luabridge`)
- Replace the placeholder `'help'` hint/text behavior with an actual callable Lua global `help([topic])` function (and allow bare `help` command invocation to route cleanly into `help()` / command help).
- `help()` must print structured documentation into the console listing:
  - Registered console commands (`LuaBridge.commands`) and built-in helpers (`help()`, `clear()`, `pick()`, `picked()`, `list_objects()`).
  - Registered engine globals & `GameObject` / `Renderable` / `InteractiveComponent` APIs.
  - Topic-filtered details when called with a string argument, e.g., `help("pick")` or `help("GameObject")`.
- Update the startup banner in [console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go) and input hint in [imgui_console.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_console.go) to reference `help()`.

### 2. Stdout / Stderr & Lua `print()` Redirection (`pkg/luabridge`, `engine`)
- Override Lua's global `print(...)` (and `io.write` / error handlers) in `LuaBridge` so every printed line forwards to attached `Console` / `outputLogs` (`LineOutput` for normal messages, `LineError` for errors) rather than bypassing the dev console.
- Implement thread-safe process-level `os.Stdout` / `os.Stderr` pipe redirection (and `log.SetOutput` integration) that captures newline-delimited Go stdout (`ConsoleLineInfo` / `LineOutput`) and stderr (`ConsoleLineError` / `LineError`) into the `Console` ring buffer, optionally teeing to the original terminal file descriptors without deadlock or recursion.
- Ensure proper teardown/restoration (`RestoreStdio()`) on console/app shutdown.

### 3. Dev Console Object Picking (`engine`, `pkg/luabridge`, `cmd/demo`)
- Add raycast / bounding-volume object picking against the active `Scene` (`BaseScene`) and/or `PhysicsWorld`:
  - Support screen-space cursor coordinate unprojection `(mouseX, mouseY, viewportW, viewportH)` using active `Camera3D` into a world-space `Ray`.
  - When Pick Mode is active (or via viewport click shortcut, e.g., Pick button in `DebugConsoleWindow` or modifier+click while ImGui is not capturing input), pick the closest intersected `GameObject` (via `PhysicsWorld.Raycast` when available, falling back to `GameObject` world transform / `Renderable` bounds intersection).
- Expose picking state to both the console UI and Lua:
  - Display the currently picked target (`Picked: <Name> (<ID>)`) in the `DebugConsoleWindow` header bar with quick-clear/inspect actions.
  - Bind the picked `GameObject` userdata to Lua globals `selected` / `picked` (updating automatically on pick, or clearing to `nil` on `unpick()`), and log `[Picked] <Name> (<ID>) @ (x, y, z)` to console output.
  - Provide Lua console helpers: `pick(nameOrID)` / `unpick()` / `picked()` / `list_objects()` for keyboard-driven picking and discovery.
- Wire viewport object picking in [main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go) so clicking the Rainbow Cube (or HUD/scene objects) binds `selected` directly in the live developer console.

## Acceptance Criteria
- [x] Running `help()` (or `help("...")`) in the dev console executes as a real Lua function and outputs command/API reference lines into the console scrollback.
- [x] Calling Lua `print(...)`, writing to `os.Stdout` / `fmt.Println`, and writing to `os.Stderr` / `log.Println` appear in `Console.Output` with appropriate line types (`LineOutput` / `LineError`) and do not deadlock on cleanup.
- [x] Viewport cursor ray picking selects the intersected `GameObject`, logs the pick in the console, shows it in `DebugConsoleWindow`, and binds Lua global `selected` (returned by `picked()`) to that `GameObject` userdata.
- [x] Lua helpers `pick(idOrName)`, `unpick()`, `picked()`, and `list_objects()` inspect and mutate scene objects directly from the console.
- [x] Automated tests in `pkg/luabridge` and `engine` cover `help()`, stdout/stderr redirection cleanup, ray/name object picking, and Lua `selected` manipulation.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0011-dev-console-picking-help-and-stdio
- **Branch:** 0011-dev-console-picking-help-and-stdio
- **Summary of Changes:**
  - **`help()` Function & Command Routing (`pkg/luabridge/bridge.go`, `pkg/luabridge/console.go`, `engine/imgui_console.go`):** Implemented callable Lua `help([topic])` and CLI `help`/`clear` command handlers (`FormatHelp`), replacing the placeholder `'help'` string/hint with live documentation of registered console commands, picking APIs, and stdio capture.
  - **Lua `print(...)` & Process Stdout/Stderr Redirection (`pkg/luabridge/bridge.go`, `pkg/luabridge/stdio.go`, `pkg/luabridge/console.go`, `pkg/luabridge/console_adapter.go`):** Overrode Lua `print(...)` and `io.write(...)` to emit directly to `Console`, added mutex-safe `Console` locking, and implemented `RedirectStdio()` / `RestoreStdio()` capturing `os.Stdout`, `os.Stderr`, and `log` output via pipes with teeing and clean teardown.
  - **Dev Console Object Picking (`engine/picking.go`, `engine/imgui_console.go`, `pkg/luabridge/console_picking.go`, `cmd/demo/main.go`):** Added `ScreenPointToRay`, `IntersectRayBoundingBox`, and `PickSceneObject`, `DebugConsolePicker` UI toolbar (`Pick Object`, active selection label, `Unpick`), `Console.PickAtScreenPos`/`PickByNameOrID`/`PickObject`, Lua helpers `pick`/`unpick`/`picked`/`list_objects` setting global `selected`, and viewport picking + stdio wiring in `cmd/demo/main.go`.
- **Files Modified / Added (committed in Ticket 0011; `cmd/demo/main.go` updated locally alongside untracked `feat/gl-context-component-demo` work):**
  - [engine/picking.go](file:///C:/Users/samue/Projects/project-engine/engine/picking.go)
  - [engine/picking_test.go](file:///C:/Users/samue/Projects/project-engine/engine/picking_test.go)
  - [engine/imgui_console.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_console.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go)
  - [pkg/luabridge/console_adapter.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_adapter.go)
  - [pkg/luabridge/stdio.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/stdio.go)
  - [pkg/luabridge/console_picking.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_picking.go)
  - [pkg/luabridge/console_picking_help_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_picking_help_test.go)
  - [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go) (local working copy wiring)
- **Verification Results:**
  - Ran `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 -v ./engine ./engine/ui ./engine/spatial ./engine/hazards ./engine/components ./pkg/interaction ./pkg/props ./pkg/luabridge ./pkg/tileimporter`, `go vet`, and `go build` (plus `go build -o NUL ./cmd/demo`); all passed with exit code `0`.
