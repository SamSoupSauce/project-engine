# Ticket 0019: Prefabs, Console Search Functions, Component Command, and Prefab UI

## Summary
Add JSON-based GameObject Prefabs with a dedicated `prefabs/` storage folder, console search functions for Prefabs and Scripts, an ergonomic `comp` command (and Lua helpers) to search and interact with Components, and a Dear ImGui Prefab Manager/Editor window to view, search, modify, save, and spawn prefabs.

## Requirements
1. Create Prefabs where a prefab is JSON describing a `GameObject` (identity/name, active flag, components with type/name/details, and optional nested children), supporting saving a live `GameObject` as a prefab and spawning/instantiating a prefab into a scene.
2. Provide a dedicated `prefabs/` directory at the repository root for prefab storage.
3. Add search functions to the developer console (both Lua helpers and CLI commands) for Prefabs.
4. Add search functions to the developer console (both Lua helpers and CLI commands) for Scripts.
5. Add an easy command (`comp` / `component` and Lua helpers) to search and interact with Components (list, search, inspect/help, add, remove, get/set fields, call methods, edit custom scripts).
6. Add a Dear ImGui interface (`PrefabEditorWindow`) to modify, search, and view prefabs (live search, structured GameObject & component inspector/editor, raw JSON editor, format/validate, revert, save, spawn, create from selected, and delete), wired into `ImGuiOverlay`, `DebugConsoleWindow`, and `cmd/demo`.

## Verification & Acceptance Criteria
- [x] `prefabs/` folder exists and stores `.json` GameObject prefabs.
- [x] Prefab serialization, loading, saving from live `GameObject`, and spawning into a scene with components and children work cleanly.
- [x] Console search functions for Prefabs and Scripts work from both Lua REPL and CLI command syntax.
- [x] `comp` command and Lua helpers cleanly search, inspect, add, remove, get, set, and call methods on built-in and custom Components.
- [x] ImGui `PrefabEditorWindow` renders cleanly in headless and desktop modes and supports searching, viewing, modifying, formatting, saving, deleting, and spawning prefabs.
- [x] All unit/integration tests, `go vet`, and `go build` pass.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0019-prefabs-console-search-and-ui
- **Branch:** 0019-prefabs-console-search-and-ui
- **Summary of Changes:**
  - Created dedicated `prefabs/` storage folder with [prefabs/.gitkeep](file:///C:/Users/samue/Projects/project-engine/prefabs/.gitkeep) and sample [prefabs/rainbow_cube.json](file:///C:/Users/samue/Projects/project-engine/prefabs/rainbow_cube.json).
  - Implemented JSON GameObject Prefab serialization, storage, loading, validation, and scene spawning in [pkg/luabridge/prefab.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/prefab.go) (`PrefabService`, `GameObjectToPrefabSpec`, `SpawnPrefabObject`) supporting built-in components, custom Lua components, and nested children.
  - Added console search and management commands/Lua functions for Prefabs (`search_prefabs`, `find_prefabs`, `list_prefabs`, `save_prefab`, `create_prefab`, `load_prefab`, `spawn_prefab`, `instantiate_prefab`, `delete_prefab`, `edit_prefab`, `view_prefabs`, `prefabs_ui`, `prefabs`, `prefab <search|list|view|save|spawn|edit|ui|delete>`).
  - Added console search commands/Lua functions for Scripts (`search_scripts`, `find_scripts`, `list_scripts`, `scripts`, `script <search|list|edit>`) in [pkg/luabridge/search_and_comp.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/search_and_comp.go).
  - Added ergonomic Component search & interaction command `comp` (with aliases `component`, `components`, and Lua helpers `search_components`, `find_components`, `list_components`, `comp(...)`) supporting `search`, `list`, `help`, `add`, `remove`, `get`, `set`, `call`, and `edit` across built-in and custom Lua components.
  - Built Dear ImGui Prefab Manager/Editor window (`PrefabEditorWindow` + `PrefabEditorBackend` + `DebugConsolePrefabUI`) in [engine/imgui_prefab_editor.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_prefab_editor.go), wired into [engine/imgui_overlay.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_overlay.go), [engine/imgui_console.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_console.go), [pkg/luabridge/console_adapter.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_adapter.go), [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go), and [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go).
  - Added headless ImGui tests in [engine/imgui_prefab_editor_test.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_prefab_editor_test.go) and integration tests in [pkg/luabridge/ticket19_prefab_search_comp_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket19_prefab_search_comp_test.go).
- **Files Modified / Added:**
  - [prefabs/.gitkeep](file:///C:/Users/samue/Projects/project-engine/prefabs/.gitkeep)
  - [prefabs/rainbow_cube.json](file:///C:/Users/samue/Projects/project-engine/prefabs/rainbow_cube.json)
  - [engine/imgui_prefab_editor.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_prefab_editor.go)
  - [engine/imgui_prefab_editor_test.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_prefab_editor_test.go)
  - [engine/imgui_overlay.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_overlay.go)
  - [engine/imgui_console.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_console.go)
  - [pkg/luabridge/prefab.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/prefab.go)
  - [pkg/luabridge/search_and_comp.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/search_and_comp.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/console_adapter.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_adapter.go)
  - [pkg/luabridge/ticket19_prefab_search_comp_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket19_prefab_search_comp_test.go)
  - [cmd/demo/main.go](file:///C:/Users/samue/Projects/project-engine/cmd/demo/main.go)
  - [tickets/0019-prefabs-console-search-and-ui.complete.md](file:///C:/Users/samue/Projects/project-engine/tickets/0019-prefabs-console-search-and-ui.complete.md)
- **Verification Results:**
  - Passed `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 ./engine ./engine/ui ./engine/spatial ./engine/hazards ./engine/components ./pkg/interaction ./pkg/props ./pkg/luabridge ./pkg/tileimporter` (all 9 packages exited 0).
  - Passed `go vet ./engine ./pkg/luabridge ./cmd/demo` and `go build -o NUL ./cmd/demo` (exit code 0).
