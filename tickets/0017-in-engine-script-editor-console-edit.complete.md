# Ticket 0017: In-Engine Script Editor via Console `edit()` Command

## Status: Complete
## Component: engine/editor, engine/console, pkg/luabridge

---

## 1. Overview
Implement an in-engine editor window using Dear ImGui that can be invoked directly from the developer console via the global Lua function `edit("MyScript")` (or `edit("path/to/script.lua")`).

The editor provides a tri-pane authoring modal isolating:
1. **Data Schema (`data` JSON entry):** Key-value default state and configurable properties.
2. **Hook Bindings (`hooks` JSON entry):** Enabled engine lifecycle bindings (`load`, `tick`, `unload`, `event`, `spawn`, `despawn`).
3. **Behavior Logic (`functions` Lua entry):** Raw Lua method definitions and component logic.

---

## 2. Console Hook & Lua Bridge Binding

### Lua Global Function
Expose `edit(target_script)` into the Lua runtime environment during engine bootstrap:

```lua
-- In engine console bootstrap / global env setup
function edit(script_name)
    __engine_open_editor(script_name)
end
```

### Go Bridge (`pkg/luabridge`)
Bind `__engine_open_editor` to set editor state in the Dear ImGui subsystem:

```go
// OpenEditorCmd sets the active script target and flags the Dear ImGui window visible
func OpenEditorCmd(L *lua.LState) int {
    scriptName := lua.CheckString(L, 1)
    editor.OpenScript(scriptName)
    return 0
}
```

---

## 3. UI Layout & Window Specification (Dear ImGui)

When active, the editor displays as a docked or modal ImGui window titled `Script Editor: <ScriptName>`:

```
+-----------------------------------------------------------------------+
| Script Editor: MyScript.lua                                     [ X ] |
+-----------------------------------+-----------------------------------+
| Pane 1: Data (JSON)               | Pane 2: Hooks (JSON)              |
| {                                 | {                                 |
|   "hp": 100,                      |   "tick": true,                   |
|   "speed": 6.5,                   |   "load": true,                   |
|   "color": "chartreuse"           |   "event": false                  |
| }                                 | }                                 |
+-----------------------------------+-----------------------------------+
| Pane 3: Functions (Lua Logic)                                         |
|                                                                       |
| function Component:tick(dt)                                           |
|     self.pos.x = self.pos.x + (self.speed * dt)                       |
| end                                                                   |
|                                                                       |
+-----------------------------------------------------------------------+
| [ Revert ]                                [ Format JSON ]  [ Commit ] |
+-----------------------------------------------------------------------+
```

### Pane Details:
* **Pane 1 (Data JSON):** Multiline text input for instance attributes and configurable fields. Validates JSON syntax on edit.
* **Pane 2 (Hooks JSON):** Explicit boolean toggles or JSON array/object specifying which engine lifecycle hooks are active. Prevents empty Go-to-Lua per-frame hook dispatches if a hook is disabled.
* **Pane 3 (Functions Lua):** Multiline text input containing the Lua methods and runtime code.

---

## 4. File Sync & Disk Format

When saving, the editor compiles these sections back to disk. Scripts support either:

1. A single file format with structured headers/metadata sections:
```lua
-- @data: {"hp": 100, "speed": 6.5}
-- @hooks: {"tick": true, "load": true}

local MyScript = {}
MyScript.__index = MyScript

function Component(go, config)
    local self = setmetatable({}, MyScript)
    self.gameObject = go
    self.hp = config.hp or 100
    return self
end

function MyScript:tick(dt)
    -- runtime tick
end
```

2. Or an automated split/merge against a paired metadata file (`MyScript.meta.json` and `MyScript.lua`).

---

## 5. Commit & Re-Instantiation Flow

1. **Syntax Validation:**
   * Parse `data` and `hooks` inputs using `json.Valid`.
   * Test-compile `functions` buffer via `L.LoadString` / `luaL_loadstring`.
2. **Disk Write:** Write updated source to disk.
3. **Instance Hot-Reload:**
   * Calculate new SHA-256 hash.
   * If running entities currently use this component, trigger map-state reconstruction.

---

## 6. Acceptance Criteria

* [x] Running `edit("MyScript")` or `edit("scripts/props/box.lua")` in the dev console pops the Dear ImGui editor window.
* [x] Editor provides three dedicated text input buffers: `Data (JSON)`, `Hooks (JSON)`, and `Functions (Lua)`.
* [x] Malformed JSON or invalid Lua syntax flags an error in the status bar without crashing the game engine loop.
* [x] Pressing `Commit` writes changes to disk and notifies the component registry to refresh active instances.
* [x] Closing the window (`[X]` or running `edit()` without args) hides the ImGui window and returns input focus to the game/console.

---
### Clarification Log
- **Agent:** Which disk format should the script editor use when loading and saving Data, Hooks, and Functions?
- **User:** (Recommended) Single .lua file with `-- @data` / `-- @hooks` metadata headers (and Ticket 0015 `Component(...)` synthesis/sync)
- **Agent:** Which Lua runtime should `edit()` and `Commit` syntax validation / hot-reload target?
- **User:** (Recommended) `LuaBridge` (`gopher-lua` `L.LoadString` + `Component` / `ScriptComponent` registry)
- **Agent:** How should `edit("MyScript")` resolve short script names vs explicit file paths?
- **User:** (Recommended) Search registered components & `scripts/` recursively by stem; create new template under `scripts/components/<Name>.lua` if missing
- **Agent:** When `Commit` hot-reloads an active component/script on live entities, how should instance state be handled?
- **User:** (Recommended) Hot-reload prototype methods/hooks and merge new defaults while preserving matching live instance field values

---
### Completion Summary
- **Ticket:** 0017-in-engine-script-editor-console-edit
- **Branch:** 0017-in-engine-script-editor-console-edit
- **Summary of Changes:**
  - **Dear ImGui Tri-Pane Script Editor (`engine/imgui_script_editor.go`, `engine/imgui_overlay.go`):** Implemented `ScriptEditorDocument`, `ScriptEditorBackend`, and `ScriptEditorWindow` with Data (JSON), Hooks (JSON), and Functions (Lua) multiline buffers, live validation status bar, `Revert`, `Format JSON`, `Commit`, `[X]` close handling, backend state sync, and `ImGuiOverlay.SetScriptEditor`.
  - **`LuaBridge` `edit()` & Hot-Reload Service (`pkg/luabridge/script_editor.go`, `pkg/luabridge/bridge.go`, `pkg/luabridge/lua_component.go`, `cmd/demo/main.go`):** Exposed global `edit([target])`, `__engine_open_editor([target])`, and CLI `edit` command; implemented target resolution (registered component `SourcePath`, recursive `scripts/` stem search, or new template under `scripts/components/<Name>.lua`); added single-file `-- @data:` / `-- @hooks:` / `-- @component:` parsing & synthesis, JSON + `L.LoadString` syntax validation, SHA-256 hashing, and live entity hot-reload preserving existing field values while merging new defaults and refreshing hook listeners.
  - **Tests (`engine/imgui_script_editor_test.go`, `pkg/luabridge/ticket17_script_editor_test.go`):** Verified headless Dear ImGui rendering, JSON formatting/errors, Lua syntax error rejection, `Revert`, `Commit` disk write & SHA-256 hash tracking, `edit()` open/close sync, and live entity state preservation across hot-reload.
- **Files Modified / Added:**
  - [engine/imgui_script_editor.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_script_editor.go)
  - [engine/imgui_script_editor_test.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_script_editor_test.go)
  - [engine/imgui_overlay.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_overlay.go)
  - [pkg/luabridge/script_editor.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/script_editor.go)
  - [pkg/luabridge/ticket17_script_editor_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket17_script_editor_test.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/lua_component.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/lua_component.go)
  - [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go)
- **Verification Results:**
  - Passed `go test -count=1 ./engine ./engine/ui ./engine/spatial ./engine/hazards ./engine/components ./pkg/interaction ./pkg/props ./pkg/luabridge ./pkg/tileimporter`, `go vet ./engine ./pkg/luabridge ./cmd/demo`, and `go build -o NUL ./cmd/demo` (exit code 0).
