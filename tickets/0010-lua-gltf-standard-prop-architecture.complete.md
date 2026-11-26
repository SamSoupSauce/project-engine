# Ticket 0010: Lua + glTF Standard Prop Architecture

## Status
Complete

## Description
Refactor engine prop loading and physics material binding so that **100% of game props are defined in Lua table manifests** and load standard **.gltf / .glb** assets. The Go engine core (`project-engine`) provides only the standard generic ECS infrastructure (`GameObject`, `TransformComponent`, `MeshRendererComponent`, `RigidBodyComponent`, `ConstraintComponent`, `HazardComponent`). No concrete game props (toasters, skillets, lamps) live as hardcoded Go structs. All asset loading, mesh binding, collision hull generation, joint break limits, and lifecycle events are scripted through Lua using the unified engine bridge.

## Architectural Changes & Specifications

### 1. Unified Engine Components (`engine/components/`)
The engine exposes standard, generic components to Lua via the Go/CGo bridge:

- **`TransformComponent`:** Wraps entity position, rotation, and scale synchronized with `GameObject.Transform`.
- **`MeshRendererComponent`:** Holds references to loaded `.gltf`/`.glb` meshes, material sub-indices, shader pipelines, visibility bitmasks, and bone attachment points. Completely manipulated via Lua (`LoadGLTF`, `SetMesh`, `SetVisible`, `SetCastShadow`, `SetMaterialOverride`).
- **`RigidBodyComponent`:** Wraps the underlying Jolt body ID. Exposes mass, inertia/material response, velocity, impulse injection, convex-hull/box creation, and dynamic vs. kinematic toggles.
- **`ConstraintComponent`:** Manages 1D slider (prismatic) and hinge constraints for drawers, cabinet doors, and hinges with runtime break thresholds.
- **`HazardComponent`:** Handles floor contact surface flags, shard cutting triggers, and footwear integrity validation.

### 2. Standard Lua Prop Manifest Structure (`scripts/props/`)
Every prop is an isolated Lua script returning a declarative definition table with standard lifecycle hooks (`on_spawn`, `on_interact`, `on_collision`, `on_break`, `on_joint_break`):

- `scripts/props/kitchen_toaster.lua` (unbreakable / acoustic clatter metal prop loading `assets/models/props/kitchen_toaster.gltf`)
- `scripts/props/nightstand_drawer.lua` (prismatic breakable drawer loading `assets/models/props/nightstand_drawer.gltf` with `on_joint_break` and splinter fracture in `on_collision`)
- `scripts/props/lamp_ceramic.lua` (fragile ceramic lamp loading `assets/models/props/lamp_ceramic.gltf` spawning dynamic `FloorShardHazard` entities with `HazardComponent` in `on_collision`)

### 3. Engine Factory & Lua Bridge API (`pkg/props/factory.go`)
- `Engine.CreateEntity(name)`: Spawns base `GameObject` with identity and transform.
- `Engine.DestroyEntity(entity_id)`: Despawns and releases an entity cleanly.
- `Engine.GetComponent(entity_id, comp_name)`: Returns Lua wrapper around standard Go/CGo component pointers.
- `Engine.Audio.BroadcastAcousticAlert(...)` & `Engine.Debris.SpawnSplinters(...)` & `Engine.Player.GetHandVelocity()`.
- `MeshRendererComponent:LoadGLTF(filepath)`: Loads `.gltf`/`.glb` geometry, parses embedded PBR materials, and extracts bounding box / convex hull vertex data for Jolt.
- `PropRegistry.LoadDirectory(path)`: Scans all `.lua` files in `scripts/props/` on startup, caching manifest tables and validating `.gltf`/`.glb` asset references.

## Acceptance Criteria

* [x] Zero concrete prop types hardcoded in Go; all prop declarations live under `scripts/props/*.lua`.
* [x] Every prop loads its visual mesh from a valid `.gltf`/`.glb` asset via `MeshRendererComponent`.
* [x] All props use standard engine components (`TransformComponent`, `MeshRendererComponent`, `RigidBodyComponent`, `ConstraintComponent`, `HazardComponent`).
* [x] Prismatic and hinge joint limits, break events, and fracture particle spawning are driven through Lua callbacks.
* [x] `FloorShardHazard` entities created dynamically in Lua when ceramic/glass fracture conditions trigger.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0010-lua-gltf-standard-prop-architecture
- **Branch:** 0010-lua-gltf-standard-prop-architecture
- **Summary of Changes:**
  - **glTF 2.0 / GLB Loader & Jolt Convex Hulls (`engine/gltf.go`, `engine/spatial/components.go`):** Implemented pure-Go glTF 2.0 JSON (embedded data URIs & external `.bin`) and GLB v2 binary loader parsing PBR materials, vertex/normal/UV/index accessors, node attachment transforms, tight bounding boxes, and convex hull vertices. Added `ShapeConvexHull` (`JPH_ConvexHullShapeSettings_Create`) to `spatial.RigidBody`.
  - **Generic Engine Components (`engine/components/components.go`):** Added standard generic ECS components `TransformComponent`, `MeshRendererComponent` (`LoadGLTF`, `SetMesh`, `SetVisible`, `SetCastShadow`, `SetMaterialOverride`), `RigidBodyComponent` (generating box/convex-hull/sphere Jolt bodies from loaded glTF geometry and material presets), `ConstraintComponent` (slider/hinge joints with break force/torque & break hook), and `HazardComponent` (`Configure` backed by `hazards.HazardSystem`).
  - **Lua Prop Registry & Factory (`pkg/props/factory.go`, `pkg/luabridge/props_bridge.go`):** Implemented `PropRegistry` (`LoadDirectory`, `LoadManifestFile`, `CreateEntity`, `DestroyEntity`, `SpawnProp`, `DispatchCollision`, `DispatchJointBreak`) and registered global `Engine` Lua bindings (`Engine.CreateEntity`, `Engine.DestroyEntity`, `Engine.GetComponent`, `Engine.Audio.BroadcastAcousticAlert`, `Engine.Debris.SpawnSplinters`, `Engine.Player.GetHandVelocity`) plus operator-overloaded `PropVec3` (`__mul`).
  - **Lua Manifests & glTF Assets (`scripts/props/*.lua`, `assets/models/props/*.gltf`):** Created valid glTF 2.0 embedded assets and Lua manifests for `kitchen_toaster`, `nightstand_drawer`, and `lamp_ceramic`, validating zero concrete prop structs hardcoded in Go.
- **Files Modified / Added:**
  - [engine/gltf.go](file:///C:/Users/samue/Projects/project-engine/engine/gltf.go)
  - [engine/gltf_test.go](file:///C:/Users/samue/Projects/project-engine/engine/gltf_test.go)
  - [engine/spatial/components.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/components.go)
  - [engine/components/components.go](file:///C:/Users/samue/Projects/project-engine/engine/components/components.go)
  - [pkg/props/factory.go](file:///C:/Users/samue/Projects/project-engine/pkg/props/factory.go)
  - [pkg/props/factory_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/props/factory_test.go)
  - [pkg/luabridge/props_bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/props_bridge.go)
  - [assets/models/props/kitchen_toaster.gltf](file:///C:/Users/samue/Projects/project-engine/assets/models/props/kitchen_toaster.gltf)
  - [assets/models/props/nightstand_drawer.gltf](file:///C:/Users/samue/Projects/project-engine/assets/models/props/nightstand_drawer.gltf)
  - [assets/models/props/lamp_ceramic.gltf](file:///C:/Users/samue/Projects/project-engine/assets/models/props/lamp_ceramic.gltf)
  - [scripts/props/kitchen_toaster.lua](file:///C:/Users/samue/Projects/project-engine/scripts/props/kitchen_toaster.lua)
  - [scripts/props/nightstand_drawer.lua](file:///C:/Users/samue/Projects/project-engine/scripts/props/nightstand_drawer.lua)
  - [scripts/props/lamp_ceramic.lua](file:///C:/Users/samue/Projects/project-engine/scripts/props/lamp_ceramic.lua)
- **Verification Results:**
  - Ran `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 -v`, `go vet`, and `go build` across all 9 packages (`./engine`, `./engine/ui`, `./engine/spatial`, `./engine/hazards`, `./engine/components`, `./pkg/interaction`, `./pkg/props`, `./pkg/luabridge`, `./pkg/tileimporter`); all exited with code `0`.
