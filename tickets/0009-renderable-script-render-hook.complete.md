# 0009: Add Renderable Per-Instance Script Render Hook (`ScriptName_render(self, gfx)`)

## Summary
Currently, `engine.Renderable` (`engine/renderable.go`) and `engine.Renderable2D` (`engine/renderable2d.go`) are passive data components evaluated by `RenderPassBuffer.ExtractFromHierarchy` and `Extract2DFromHierarchy`, and `engine.ScriptLifecycle` (`engine/script.go`) only dispatches six hooks (`load`, `unload`, `spawn`, `event`, `tick`, `despawn`). Neither component triggers a per-instance script render callback, and no `Renderer` (`gfx`) handle is exposed to `GameObject` scripts.

Add support for a per-instance render hook—such as `ScriptName_render(self: GameObject, gfx: Renderer)`—invoked during the render pass for every active, spawned `GameObject` that has an attached `Renderable` component.

## Requirements
- **Engine Render Hook Contract (`engine/`)**:
  - Define a decoupled render lifecycle / dispatcher interface in `engine/` (following the `ScriptLifecycle` pattern in `engine/script.go` to avoid circular imports with `pkg/luabridge`).
  - Update `Renderable` (or scene render traversal in `BaseScene` / `GLApp.StepFrame`) so that during the frame render pass, any `GameObject` that is `Active`, `IsSpawned()`, and has an attached `Renderable` component (respecting `FlagVisible` / visibility cascade) dispatches its render hook with `(self, gfx)`.
  - Ensure `GameObject`s that are not spawned (`!obj.IsSpawned()`), inactive (`!obj.Active`), hidden, or lack a `Renderable` component do not invoke the render hook.
  - Gracefully no-op if a spawned `GameObject` with `Renderable` has no attached script or does not define a render function.
- **`Renderer` (`gfx`) Abstraction (`engine/` & `pkg/luabridge/`)**:
  - Define a `Renderer` interface/handle in `engine/` representing the frame drawing context passed as `gfx`.
  - Expose `Renderer` bindings to Lua (`pkg/luabridge`) so scripts can submit draw calls or update render commands via `gfx` without leaking raw OpenGL state.
  - Ensure per-frame `render` dispatch adheres to [`docs/scripting_standards.md`](../docs/scripting_standards.md) (avoiding per-frame Lua table/string allocations on hot render paths).
- **Script Hook Resolution (`pkg/luabridge/`)**:
  - Support resolving and calling `ScriptName_render(self, gfx)` (where `ScriptName` is derived from the attached script name/identifier or configured on `Renderable`/`ScriptComponent`) and/or clarify alignment with `ScriptComponent`'s existing isolated-environment hook convention (`load`, `unload`, `spawn`, `event`, `tick`, `despawn`).
  - Record and surface runtime Lua errors via `LuaBridge.OnScriptError` / `ScriptComponent.LastError()` without crashing the render loop.

## Verification & Acceptance Criteria
- [x] Headless test verifies that a spawned, active `GameObject` with both `Renderable` and an attached script triggers `ScriptName_render(self, gfx)` once per render pass with valid `GameObject` and `Renderer` arguments.
- [x] Unspawned (`!IsSpawned()`), inactive (`!Active`), or hidden (`FlagVisible` cleared) `GameObject`s do **not** trigger the render hook.
- [x] A `GameObject` with a script component but **without** a `Renderable` component does not trigger the render hook.
- [x] A `GameObject` with `Renderable` whose script omits the render function runs cleanly as a no-op with zero errors or panics.
- [x] `GLApp.StepFrame` / `BaseScene` integration executes the render pass in the expected frame order (after `Update`, preserving GL state).
- [x] `go test ./...`, `go vet ./...`, and `go build ./...` pass with zero regressions.

---
### Clarification Log
- **Agent:** How should the per-instance render hook be resolved and executed in Lua?
- **User:** Support both `<ScriptName>_render(self, gfx)` and isolated `render(self, gfx)` in `ScriptComponent` (gopher-lua).
- **Agent:** What drawing API should the `Renderer` (`gfx`) parameter expose to scripts?
- **User:** Unified `Renderer` (`gfx`) supporting 2D quads (`OrthoBatcher2D`) and 3D `DrawCommand`s (`RenderPassBuffer`) plus viewport size.
- **Agent:** How should `Renderable` relate to `ScriptComponent` when dispatching the render hook?
- **User:** `Renderable` gates visibility/eligibility and dispatches to sibling `ScriptComponent`(s), with optional `Renderable.ScriptName` override.
- **Agent:** How should git branching be handled given uncommitted `GLContext` changes on `feat/gl-context-component-demo`?
- **User:** Create branch `0006-renderable-script-render-hook` from current HEAD and stage only ticket 0006 files (leaving `GLContext` changes untouched).

---
### Completion Summary
- **Ticket:** 0009-renderable-script-render-hook (renumbered to avoid colliding with `0006`–`0008`)
- **Branch:** 0008-renderable-script-render-hook (committed in `ff7b517`)
- **Summary of Changes:**
  - Added unified `Renderer` (`gfx`) in `engine/renderer.go` collecting 3D `DrawCommand`s (`RenderPassBuffer`) and 2D `QuadCommand2D`s (`OrthoBatcher2D`) with viewport size tracking and stable `ZIndex` sorting.
  - Added `RenderLifecycle` and `NamedRenderLifecycle` interfaces in `engine/script.go`, `Renderable.ScriptName` + `SetVisible`/`IsVisible` + `DispatchRenderFromHierarchy` in `engine/renderable.go`, and `GameObject.DispatchRender` in `engine/game_object.go`.
  - Integrated `Renderer`, `Render(gfx)`, `Draw3D()`, and `Draw2D()` into `BaseScene` (`engine/scene.go`) so active, spawned, visible `GameObject`s with a `Renderable` dispatch per-instance render hooks each frame.
  - Implemented `OnRender` / `OnRenderWithScriptName` in `pkg/luabridge/script_component.go` resolving `<ScriptName>_render(self, gfx)` (using `Renderable.ScriptName` override, `ScriptComponent.ScriptName`, or `ScriptPath` stem across isolated env, module table, or `_G`) with fallback to isolated `render(self, gfx)`.
  - Added `Renderer` and `Renderable` Lua userdata metatables and `GameObject:GetRenderable()` / `AddRenderable()` / `Scene:GetRenderer()` bindings in `pkg/luabridge/renderer_userdata.go` and `pkg/luabridge/userdata.go`.
  - Added unit and integration tests in `engine/renderer_test.go` and `pkg/luabridge/renderable_hook_test.go`.
- **Files Modified / Added:**
  - [engine/renderer.go](file:///C:/Users/samue/Projects/project-engine/engine/renderer.go)
  - [engine/renderer_test.go](file:///C:/Users/samue/Projects/project-engine/engine/renderer_test.go)
  - [engine/script.go](file:///C:/Users/samue/Projects/project-engine/engine/script.go)
  - [engine/renderable.go](file:///C:/Users/samue/Projects/project-engine/engine/renderable.go)
  - [engine/game_object.go](file:///C:/Users/samue/Projects/project-engine/engine/game_object.go)
  - [engine/scene.go](file:///C:/Users/samue/Projects/project-engine/engine/scene.go)
  - [pkg/luabridge/script_component.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/script_component.go)
  - [pkg/luabridge/renderer_userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/renderer_userdata.go)
  - [pkg/luabridge/userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/userdata.go)
  - [pkg/luabridge/renderable_hook_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/renderable_hook_test.go)
- **Verification Results:**
  - `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 ./engine ./pkg/luabridge ./pkg/tileimporter`: All tests passed.
  - `go vet ./engine ./pkg/luabridge ./pkg/tileimporter` & `go build ./engine ./pkg/luabridge ./pkg/tileimporter`: Passed with 0 issues.
