# Ticket 0015: Lua-Defined Custom Component Types, Fields, Methods & Hooks

## Status
Complete

## Context
Following Ticket 0014, `engine.GameObject` is a pure ECS entity whose components are exposed in `pkg/luabridge` via `ComponentClass` constants (`Transform`, `Mesh`, `MeshRenderable`, etc.) and `GetComponent` / `AddComponent` / `HasComponent` / `RemoveComponent` / `help(...)`. However, all component types are currently statically defined in Go, and `ComponentClass` metatables are strictly read-only.

This ticket enables Lua scripts in the `scripts/` folder to define new first-class `Component` types at runtime using a declarative schema, attach default fields and typed hook signatures, and declare shared prototype methods on any component type:

```lua
Flammable = Component("Flammable", {
    -- Fields: default instance values
    Lit = false,
    BurnTime = 10.0,
}, {
    -- Hooks: expected signature metadata (string type names or type tokens)
    Ignite = { "GameObject" },
    Extinguish = {},
})

function Flammable:Ignite(sender)
    if not self.Lit then
        self.Lit = true
        print(string.format("Ignited by: %s", tostring(sender)))
    end
end
```

## Scope of Work

### 1. Global `Component(name [, default_fields [, hook_signatures]])` Factory & Registry (`pkg/luabridge`, `engine`)
- Expose a global `Component(name, [default_fields], [hook_signatures])` constructor (and backing `ComponentRegistry`) in `LuaBridge`:
  - Stores schema metadata (`__name`, `__defaults`, `__hooks`) and sets up a shared prototype (`__index` pointing to the prototype/methods table) so methods are shared across all instances with minimal per-entity allocation.
  - Accepts optional `default_fields` table mapping field names to default values (cloned per instance on creation, with optional per-instance `overrides` passed to `AddComponent(Flammable, overrides)` / `Proto.new(entity, overrides)`).
  - Accepts optional `hook_signatures` table mapping hook names to argument type lists using either string type names (e.g., `{ "GameObject" }`) or type tokens (e.g., `{ GameObject }`), registering `GameObject` as a valid Lua type token.
- Implement a Go-side `LuaComponent` (satisfying `engine.Component` and `engine.ScriptLifecycle` / event dispatch) representing an instance of a Lua-defined component attached to an `*engine.GameObject`:
  - Integrates seamlessly with `GameObject:AddComponent(Flammable, [overrides])`, `GameObject:GetComponent(Flammable)` / `GetComponent("Flammable")`, `GameObject:HasComponent(Flammable)`, `GameObject:RemoveComponent(Flammable)`, and `GameObject:GetComponents()`.
  - Exposes per-instance fields (`self.Lit`, `comp.Lit = true`) initialized from defaults + overrides, plus the owning `GameObject` reference (`self.entity` / `self.owner` / `self:GetOwner()`).
  - Integrates with `help(Flammable)`, `help("Flammable")`, `help(comp)`, `help(selected)`, and `SerializeComponentJSON` / `SerializeGameObjectJSON` so custom Lua components, their default fields (`__defaults`), methods, and hook signatures (`__hooks`) are inspectable from Lua and the developer console.

### 2. Dynamic Method Declaration on Any Component (`pkg/luabridge`)
- Allow declaring new Lua methods on any component type (both custom Lua-defined `Component(...)` prototypes and built-in `ComponentClass` types such as `Transform`, `Mesh`, `MeshRenderable`, etc.) using standard Lua method syntax:
  - `function Flammable:Ignite(sender) ... end`
  - `function Transform:ResetOrigin() ... end`
- Update `ComponentClass` `__newindex` / `__index` and component instance userdata metatables so functions assigned to a component class live on the shared prototype and are callable on all component instances of that type (`comp:Ignite(sender)`), with `self` bound to the component instance.
- Preserve read-only protection for reserved metadata properties (`name`, `Name`, `go_type`, `GoType`, `__name`, `__defaults`, `__hooks`) on `ComponentClass` while permitting method/function assignment.

### 3. Hook Listener Registration, Signature Validation & Dispatch (`pkg/luabridge`, `engine`)
- Expose `GameObject:RegisterHookListener(hookName, componentInstance)` and `GameObject:CallHook(hookName, ...)` (and auto-register declared hook listeners when a `LuaComponent` is attached, resolving `Proto[hookName]` dynamically even if the method is declared after `Component(...)`).
- Validate argument count and types in `CallHook` / event dispatch against the declared `hook_signatures` (e.g., `Ignite = { "GameObject" }` / `{ GameObject }`) before invoking the Lua method, returning or logging clear errors on mismatched/nil arguments.
- Dispatch matching component hooks when `GameObject:Emit(...)`, `EmitTo(...)`, or `DispatchEvent(...)` fires on the owning `GameObject`, as well as optional lifecycle hooks (`Init`, `Update`, `Destroy`, `Spawn`, `Tick`, `Despawn`) if defined.

### 4. `scripts/` Folder Loading (`pkg/luabridge`, `cmd/demo`)
- Add script loading support in `LuaBridge` (e.g., `LoadScriptsDir`) so Lua scripts in the `scripts/` folder can execute `Component(...)` and register custom component types and methods at startup (while safely skipping LuaJIT FFI scripts such as `scripts/ui/`).

## Acceptance Criteria
- [x] A global `Component(name [, default_fields [, hook_signatures]])` function is available to Lua scripts and creates a registered `ComponentClass` / prototype usable with `AddComponent`, `GetComponent`, `HasComponent`, `RemoveComponent`, and `GetComponents`.
- [x] Component instances created via `AddComponent(Flammable, [overrides])` / `Proto.new(entity, overrides)` receive isolated per-instance field copies initialized from defaults + overrides and expose `entity`/`owner`.
- [x] Any component class (Lua-defined or built-in engine `ComponentClass`) supports declaring new Lua methods on its shared prototype (e.g., `function Flammable:Ignite(sender) ... end`) and invoking them on component instances without duplicating closures per entity.
- [x] Hook signatures declared in `Component(...)` support both string names (`{ "GameObject" }`) and type tokens (`{ GameObject }`), auto-register listeners on attach, validate argument types on `CallHook` / `Emit` / `DispatchEvent`, and invoke matching component methods.
- [x] `help(Flammable)`, `help(comp)`, `help(selected)`, and JSON serialization (`print(comp)`, `print(selected)`) inspect and display Lua-defined components, `__defaults` fields, methods, and `__hooks` signatures.
- [x] Scripts in `scripts/` can define and register `Component` types via `LuaBridge`, verified with unit/integration tests covering field defaults/overrides, instance isolation, dynamic methods on custom and built-in components, hook argument validation, and hook dispatch.

---
### Clarification Log
- **User:** Refined Lua syntax to valid comma-separated tables (`Lit = false, BurnTime = 10.0,`) and `{ "GameObject" }` / `{ GameObject }` hook signature lists; specified prototype-based `Component(name, default_fields, hook_signatures)` factory pattern (`__name`, `__defaults`, `__hooks`, `Proto.__index = Proto`, `Proto.new(entity, overrides)`), console introspection via `__defaults`/`__hooks`, shared prototype methods to avoid per-entity closure allocations, and `entity:RegisterHookListener` / `entity:CallHook` argument type validation.


---
### Completion Summary
- **Ticket:** 0015-lua-custom-components
- **Branch:** 0015-lua-custom-components
- **Summary of Changes:**
  - Implemented global `Component(name [, default_fields [, hook_signatures]])` factory, `LuaComponentDef` schema/prototype registry, `LuaComponent` Go ECS component (`Init`, `Update`, `Destroy`, `OnLoad`, `OnUnload`, `OnSpawn`, `OnEvent`, `OnTick`, `OnDespawn`), deep value cloning, and `TypeToken` globals (`GameObject`, `Scene`) in `pkg/luabridge/lua_component.go`.
  - Updated `ComponentClass` and wrapped built-in component metatables in `pkg/luabridge/component_access.go` so dynamic Lua methods (`function MyComp:Method(...)` and `function Transform:CustomMethod(...)`) share prototype entries without breaking read-only metadata guards.
  - Added `GameObject.RemoveComponentWhere` in `engine/game_object.go` and wired `AddComponent`, `RemoveComponent`, `RegisterHookListener`, `CallHook` (with argument signature validation), `help(...)`, JSON serialization, and `LoadScriptsDir` (skipping `scripts/ui` and `scripts/props`).
- **Files Modified / Added:**
  - [engine/game_object.go](file:///C:/Users/samue/Projects/project-engine/engine/game_object.go)
  - [pkg/luabridge/lua_component.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/lua_component.go)
  - [pkg/luabridge/component_access.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/component_access.go)
  - [pkg/luabridge/userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/userdata.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/help.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/help.go)
  - [pkg/luabridge/serialize.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/serialize.go)
  - [pkg/luabridge/ticket15_lua_component_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket15_lua_component_test.go)
- **Verification Results:**
  - Passed `go test -count=1 ./engine ./engine/ui ./engine/spatial ./engine/hazards ./engine/components ./pkg/interaction ./pkg/props ./pkg/luabridge ./pkg/tileimporter`, `go vet ./engine ./pkg/luabridge ./cmd/demo`, and `go build -o NUL ./cmd/demo`.
