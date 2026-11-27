# Ticket 0012: Default 3D Scene Picking & Console REPL Serialization (`Nil` / Object Print)

## Status
Complete

## Context
Following Ticket 0011, the developer console supports raycast object picking (`PickAtScreenPos`), `help()`, and stdout/stderr capture, while evaluating commands via `LuaBridge.executeWithStreamFlag` in [bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go). Three usability gaps remain during interactive inspection in [main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go) and [bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go):

1. Clicking directly on the 3D scene currently ignores picking unless `Pick Mode: ON` or `Shift+Left Click` is held.
2. Evaluating a Lua expression in the console currently returns `"OK"` for bare/void results or prints opaque pointers (`"userdata: 0x..."` / `"table: 0x..."`) instead of a human-readable serialized representation of the returned object or table.
3. When an evaluated expression or statement succeeds with no return value (or returns `nil`), the console currently prints `"OK"` (or nothing) instead of `Nil`.

## Scope of Work

### 1. Default Viewport Click Object Picking (`cmd/demo`, `pkg/luabridge`, `engine`)
- Make 3D scene left-clicking trigger object picking by default whenever an uncaptured left mouse click (`button == glfw.MouseButtonLeft && action == glfw.Press`) reaches `OnGameMouseButton` (i.e., outside Dear ImGui windows when `!overlay.WantCaptureMouse()` and cursor is unlocked for scene clicking).
- Retain `Pick Object` / `Unpick` controls in `DebugConsoleWindow` ([imgui_console.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_console.go)) and `Shift+Click` compatibility cleanly without requiring users to manually enable `Pick Mode` first.
- Ensure clicking empty background logs no crash cleanly (or optionally leaves/clears selection cleanly as configured).

### 2. REPL Expression Evaluation & Serialized / JSON Pretty-Printed Output (`pkg/luabridge`)
- Support evaluating bare Lua expressions in the developer console (attempting `return <input>` compilation first cleanly like a standard Lua REPL, falling back to statement compilation `L.DoString(<input>)` when the input is a statement/assignment such as `x = 1` or `selected:SetPosition(...)`).
- Implement `SerializeLuaValue(L *lua.LState, val lua.LValue)` and `SerializeGameObjectJSON(obj *engine.GameObject)` in [bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go) (replacing `"OK"` / raw pointer `.String()`):
  - **`*engine.GameObject` userdata (both returned expressions and Lua `print(obj)` / `tostring(obj)`):** Pretty-print as indented JSON (`encoding/json` `MarshalIndent(..., "", "  ")`) containing structured fields (`id`, `name`, `active`, `spawned`, `parent_id`, `child_count`, `transform` with `position`/`rotation`/`scale`, and `components` list/metadata).
  - **Engine component userdata (`*engine.Renderable`, `*engine.Renderable2D`, `*spatial.RigidBody`, `*spatial.InteractiveComponent`, `*spatial.FractureComponent`, `Vector3`):** Format human-readable key/value summaries.
  - **Lua tables (`*lua.LTable`):** Format key/value pairs and array elements cleanly with cycle detection and bounded depth.
  - **Primitive values:** Strings, numbers, booleans formatted cleanly; multi-return values formatted across values cleanly.
- Update `executeWithStreamFlag` / `Execute` and `print(...)` / `GameObject.__tostring` so any returned or printed `GameObject` prints indented JSON to the console instead of `"OK"` or opaque pointer addresses.

### 3. `Nil` Result on Void / Nil Expressions (`pkg/luabridge`)
- When an evaluated Lua command/expression completes with no error and produces no return value (empty stack or explicit `lua.LNil`, and without prior `print(...)` streamed output), return/print `"Nil"` instead of `"OK"`.
- Ensure statements that explicitly stream output via `print(...)` / `help()` continue displaying their streamed lines without appending an extra `"Nil"` line unless desired.

## Acceptance Criteria
- [x] Left-clicking on an object in the 3D viewport (without needing `Pick Mode` or `Shift`) selects the object, logs the pick in the console, updates `DebugConsoleWindow`, and binds Lua global `selected`.
- [x] Evaluating an expression returning a `GameObject` or calling `print(go)` outputs valid pretty-printed indented JSON for that `GameObject`, and returning tables/components prints their serialized structure instead of `"OK"` or raw memory addresses.
- [x] Evaluating an expression or statement that returns `nil` or no value (and produces no error) prints `Nil` in the console instead of `"OK"`.
- [x] Unit tests verify default viewport picking behavior, pretty-printed JSON serialization for `GameObject`, Lua table/component serialization formatting, and `Nil` output on void/nil expressions.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0012-default-picking-and-repl-serialization
- **Branch:** 0012-default-picking-and-repl-serialization
- **Summary of Changes:**
  - **Default 3D Viewport Click Picking (`cmd/demo/main.go`):** Removed the `Pick Mode` / `Shift+Click` gate on `overlay.OnGameMouseButton` so uncaptured left-clicks anywhere on the 3D scene trigger `console.PickAtScreenPos(...)` by default (using cursor position when unlocked or screen center when cursor-locked).
  - **GameObject Pretty-Printed JSON & Lua REPL Serialization (`pkg/luabridge/serialize.go`, `pkg/luabridge/userdata.go`, `pkg/luabridge/bridge.go`, `pkg/luabridge/console.go`):** Implemented `SerializeGameObjectJSON` (indented JSON with `id`, `name`, `active`, `spawned`, `parent_id`, `children`, `transform`, and `components`) wired into both `GameObject.__tostring` (for `print(go)` / `tostring(go)`) and `SerializeLuaValue` (alongside formatted engine components, `Vector3`, and Lua tables with cycle/depth guards). Updated `Console.Print` to split multi-line output cleanly for Dear ImGui rendering.
  - **Expression REPL & `Nil` Void Return (`pkg/luabridge/bridge.go`):** Updated `executeWithStreamFlag` to evaluate bare Lua expressions (`return <expr>` compilation first cleanly falling back to statement execution) and return `"Nil"` instead of `"OK"` when an expression or statement yields `nil` or zero return values (without prior streamed `print`/`help` output).
- **Files Modified / Added:**
  - [pkg/luabridge/serialize.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/serialize.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/userdata.go)
  - [pkg/luabridge/console.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console.go)
  - [pkg/luabridge/console_picking_help_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_picking_help_test.go)
  - [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go) (local working copy wiring)
- **Verification Results:**
  - Ran `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 -v ./engine ./engine/ui ./engine/spatial ./engine/hazards ./engine/components ./pkg/interaction ./pkg/props ./pkg/luabridge ./pkg/tileimporter`, `go vet` (including `./cmd/demo`), and `go build` (including `go build -o NUL ./cmd/demo`); all passed with exit code `0`.
