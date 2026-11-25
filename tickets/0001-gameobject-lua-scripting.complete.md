# 0001: Add GameObject Lua Scripting Component and Lifecycle Hooks

## Summary
Implement a scripting component for `GameObject` in the core engine that binds external Lua script files to individual entity lifecycles. The component executes defined callback hooks during scene transitions, entity instantiation, event dispatches, physics/fixed ticks, and entity destruction.

## Requirements
- Add a `ScriptComponent` (or script binding container) to `engine.GameObject` capable of loading and running a specified Lua script file path via `pkg/luabridge`.
- Ensure each script instance maintains its own isolated Lua environment or table scope while accessing shared engine bindings.
- Expose engine userdata types to Lua:
  - `GameObject` (identity, active state, transform access, event emission).
  - `Scene` (current scene handle, scene-level queries).
- Implement engine dispatchers for the following six lifecycle hooks if defined in the attached script:
  - `load(scene)`: Triggered on every attached script across all scene objects upon scene load completion.
  - `unload(scene)`: Triggered on every attached script across all scene objects prior to scene destruction/unloading.
  - `spawn(self, sender)`: Triggered when the GameObject is spawned into the active scene. `sender` passes the spawning `GameObject`, or the `Scene` instance if instantiated as part of the initial scene graph.
  - `event(name, self, sender, data)`: Generic event bus hook triggered by `GameObject:Emit(name, data)`. Passes the string event `name`, `self`, `sender` (instigating `GameObject`), and an optional Lua `table` or `nil` as `data`.
  - `tick(self, dt)`: Fixed-rate update hook called every simulation tick while the object is actively spawned. `dt` passes delta time in milliseconds as an integer.
  - `despawn(self, sender, data)`: Triggered immediately prior to removing the GameObject from the active scene. Passes `self`, `sender` (`GameObject`), and optional event `data` (table or `nil`).
- Gracefully no-op if a script omits any of the optional hook functions.

## Verification & Acceptance Criteria
- [x] Automated headless test verifies loading a dummy Lua script containing all six hooks.
- [x] Verification tests confirm `load` and `unload` receive valid `Scene` references.
- [x] Spawning an entity invokes `spawn(self, sender)` with correct sender types (`Scene` on initial load, `GameObject` on manual runtime spawn).
- [x] Emitting an arbitrary event delivers the event name, sender, and payload table to `event(name, self, sender, data)`.
- [x] Simulation step invokes `tick(self, dt)` with correct millisecond integer intervals.
- [x] Calling despawn fires `despawn(self, sender, data)` before cleanup.
- [x] Scripts missing one or more hooks run without throwing runtime errors or panics.

---
### Clarification Log
<!-- Agent logs clarification questions asked in console and user answers here, or marks as clear -->
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0001-gameobject-lua-scripting
- **Branch:** 0001-gameobject-lua-scripting
- **Summary of Changes:**
  - Added `ScriptLifecycle`, `EventBus`, and `SceneController` contracts in `engine/script.go` to decouple engine lifecycle dispatching from `pkg/luabridge` without circular imports.
  - Added `BaseScene` in `engine/scene.go` with scene-graph management (`AddObject`, `Spawn`, `Despawn`, `FindByID`, `FindByName`, `GetObjects`, `ObjectCount`), fixed-rate millisecond tick accumulation (`TickIntervalMs`, `Update`, `Tick`), and `load`/`spawn`/`event`/`tick`/`despawn`/`unload` lifecycle orchestration.
  - Extended `engine.GameObject` in `engine/game_object.go` with `Scene` tracking, spawned state, `Emit`/`EmitTo`, and `ScriptLifecycle` dispatchers.
  - Added `ScriptComponent` in `pkg/luabridge/script_component.go` with per-instance isolated Lua environments (`L.SetFEnv` + `__index = _G`), optional hook no-op safety, and error logging.
  - Registered `GameObject` and `Scene` Lua userdata metatables in `pkg/luabridge/userdata.go` (exposing identity, active state, transform access, event emission, scene queries, spawn/despawn, and Go/Lua payload marshaling) and added a re-entrant mutex (`pkg/luabridge/mutex.go`) for nested Lua-to-Go-to-Lua callbacks.
  - Added headless unit and integration tests in `engine/engine_test.go` and `pkg/luabridge/script_component_test.go` covering all six hooks, environment isolation, userdata bindings, and missing-hook no-ops.
- **Files Modified / Added:**
  - [engine/script.go](file:///C:/Users/samue/Projects/project-engine/engine/script.go)
  - [engine/scene.go](file:///C:/Users/samue/Projects/project-engine/engine/scene.go)
  - [engine/game_object.go](file:///C:/Users/samue/Projects/project-engine/engine/game_object.go)
  - [engine/engine_test.go](file:///C:/Users/samue/Projects/project-engine/engine/engine_test.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/mutex.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/mutex.go)
  - [pkg/luabridge/userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/userdata.go)
  - [pkg/luabridge/script_component.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/script_component.go)
  - [pkg/luabridge/script_component_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/script_component_test.go)
- **Verification Results:**
  - `go test -count=1 -v ./...`: All tests passed (`engine`: 3 tests, `pkg/luabridge`: 8 tests, `pkg/tileimporter`: 4 tests).
  - `go vet ./...`: Passed with 0 issues.
  - `go build ./...`: Passed with 0 errors.
