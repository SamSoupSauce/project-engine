# Ticket 0013: Component Introspection (`help`), `Transform` Component, `Renderable` Transform Constraint & Lua Component Access

## Status
Complete

## Context
Following Ticket 0012, the developer console supports viewport picking into `selected`, bare expression evaluation, and pretty-printed JSON serialization of `GameObject`s. To complete runtime inspection and ECS manipulation from Lua and the dev console:
- Calling `help(selected)` or `help("ComponentName")` / `help(comp)` should introspect live objects, their attached components, and component methods/fields.
- `GameObject` currently stores `Transform` as a plain struct field ([game_object.go](file:///C:/Users/samue/Projects/project-engine/engine/game_object.go)) rather than a first-class engine `Component` enforced by `Renderable` ([renderable.go](file:///C:/Users/samue/Projects/project-engine/engine/renderable.go)), and `pkg/luabridge` lacks generic `GameObject:GetComponent(name)` / `GetComponents()` / `AddComponent(name)` access across all component types.

## Scope of Work

### 1. Object Introspection via `help(obj)` (`pkg/luabridge`)
- Extend the Lua `help(target)` function (and `help` CLI command if given an object/component name) to inspect `*engine.GameObject` userdata (e.g., `help(selected)`, `help(picked())`):
  - Enumerate all callable methods and readable/writable properties available on `GameObject`.
  - List every component currently attached to the object along with its resolved component type name and brief state summary.

### 2. Component Introspection via `help(comp)` / `help("TypeName")` (`pkg/luabridge`)
- Support passing either a live component userdata (`help(selected:GetComponent("Transform"))`, `help(selected:GetRenderable())`) OR a component type name string (`help("Transform")`, `help("Renderable")`, `help("Renderable2D")`, `help("RigidBody")`, `help("InteractiveComponent")`, `help("FractureComponent")`, `help("ScriptComponent")`) to `help(...)`.
- Output the component's description, constraints (e.g., `Renderable` requiring `Transform`), readable/writable fields/properties, and callable Lua methods with concise parameter hints.

### 3. First-Class `Transform` Engine Component (`engine`, `engine/components`)
- Add `engine.TransformComponent` implementing `engine.Component` with 3D `Position Vector3` (and `Rotation`, `Scale`) synchronized seamlessly with `GameObject.Transform` (`GetWorldTransform()`, `SetPosition`, `Translate`, etc.).
- Attach `TransformComponent` by default on `NewGameObject` (keeping existing scenes backward-compatible) while allowing explicit lookup/removal and syncing with `engine/components.TransformComponent`.

### 4. `Renderable` -> `Transform` Component Constraint (`engine`)
- Enforce a structural constraint on `Renderable` requiring a `Transform` component (`*TransformComponent`) on its owning `GameObject`:
  - Expose `ValidateConstraint() error` (and `RequireTransform bool` / strict verification) on `Renderable` that verifies `Owner != nil` and an attached `TransformComponent` is present, reading the local/world transform directly from the `TransformComponent`.
  - In `ExtractFromHierarchy`, `DispatchRenderFromHierarchy`, and `Init`/`Attach`, enforce the `Transform` constraint (skipping/failing render extraction or reporting an error if a `GameObject` with a `Renderable` has no `Transform` component, such as uninitialized struct-literal objects or objects whose `Transform` was removed).

### 5. Generic Lua Component Access (`pkg/luabridge`)
- Register `Transform` (and `TransformComponent` / `RigidBody`) Lua metatables and expose full generic component access on `GameObject`:
  - `obj:GetComponent(typeName)` -> returns typed Lua userdata for `"Transform"` (`"TransformComponent"`), `"Renderable"`, `"Renderable2D"`, `"RigidBody"`, `"InteractiveComponent"` (`"Interactive"`), `"FractureComponent"` (`"Fracture"`), etc.
  - `obj:HasComponent(typeName)` -> returns boolean.
  - `obj:GetComponents()` -> returns a Lua table/array of all attached component userdata values (compatible with `help(...)` and pretty serialization).
  - `obj:AddComponent(typeName, ...)` / `obj:RemoveComponent(typeName)` -> attaches or removes components from Lua cleanly (respecting `Renderable`'s `Transform` constraint or checking validity).

## Acceptance Criteria
- [x] Running `help(selected)` in Lua / dev console enumerates all `GameObject` methods, properties, and a typed list of every component attached to `selected`.
- [x] Running `help(comp)` on any component userdata or `help("Renderable")` / `help("Transform")` displays that component type's methods, fields, and constraints.
- [x] `TransformComponent` exists in `engine` as an `engine.Component` managing a 3D location on `GameObject`s.
- [x] `Renderable` validates/enforces the presence of a `Transform` component on its owning `GameObject` and reads transforms through it.
- [x] Lua scripts and the dev console can access, inspect, add, and mutate components via `GetComponent(name)`, `GetComponents()`, `HasComponent(name)`, and `AddComponent(name)`.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0013-component-help-transform-constraint-and-lua-access
- **Branch:** 0013-component-help-transform-constraint-and-lua-access
- **Summary of Changes:**
  - **`TransformComponent` & `Renderable` Constraint (`engine/transform_component.go`, `engine/game_object.go`, `engine/renderable.go`, `engine/components/components.go`):** Implemented `engine.TransformComponent` (`NewTransformComponent`, `NewTransformComponentAt`, `SetPosition`, `GetPosition`, `Translate`, `SetRotation`, `SetScale`, `WorldTransform`) synchronized with `GameObject.Transform`, attached automatically by `NewGameObject`, and aliased in `engine/components`. Added `RemoveComponent`, `GetTransformComponent()`, and enforced `Renderable`'s `TransformComponent` constraint (`ErrMissingTransformComponent`, `ValidateConstraint()`, `ConstraintErr`, and hierarchy extraction/dispatch gating).
  - **Generic Lua Component Access (`pkg/luabridge/component_access.go`, `pkg/luabridge/userdata.go`, `pkg/luabridge/renderer_userdata.go`, `pkg/luabridge/serialize.go`):** Registered `Transform` (`TransformComponent`), `RigidBody`, and `ScriptComponent` metatables, exposed `GetComponent`, `HasComponent`, `GetComponents`, `AddComponent`, `RemoveComponent`, and `Help` on `GameObject`, exposed `ValidateConstraint`/`GetTransform` on `Renderable`, and added `TransformComponent` details to JSON pretty-printing.
  - **Object & Component Introspection (`pkg/luabridge/help.go`, `pkg/luabridge/bridge.go`):** Implemented `FormatObjectHelp`, `FormatComponentHelp`, and `ResolveHelpArg` so both `help(selected)` / `help selected` and `help(comp)` / `help("ComponentName")` print full methods, properties, constraints, and typed attached component lists.
- **Files Modified / Added:**
  - [engine/transform_component.go](file:///C:/Users/samue/Projects/project-engine/engine/transform_component.go)
  - [engine/transform_constraint_test.go](file:///C:/Users/samue/Projects/project-engine/engine/transform_constraint_test.go)
  - [engine/game_object.go](file:///C:/Users/samue/Projects/project-engine/engine/game_object.go)
  - [engine/renderable.go](file:///C:/Users/samue/Projects/project-engine/engine/renderable.go)
  - [engine/components/components.go](file:///C:/Users/samue/Projects/project-engine/engine/components/components.go)
  - [pkg/luabridge/component_access.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/component_access.go)
  - [pkg/luabridge/help.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/help.go)
  - [pkg/luabridge/userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/userdata.go)
  - [pkg/luabridge/renderer_userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/renderer_userdata.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/serialize.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/serialize.go)
  - [pkg/luabridge/ticket13_component_help_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ticket13_component_help_test.go)
- **Verification Results:**
  - Ran `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 -v ./engine ./engine/ui ./engine/spatial ./engine/hazards ./engine/components ./pkg/interaction ./pkg/props ./pkg/luabridge ./pkg/tileimporter`, `go vet` (including `./cmd/demo`), and `go build` (including `go build -o NUL ./cmd/demo`); all exited with code `0`.
