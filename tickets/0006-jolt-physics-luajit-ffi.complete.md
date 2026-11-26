# Ticket 0006: Physics Integration (Jolt Physics & LuaJIT FFI)

## Status
Complete

## Context
With the engine core decoupled into `project-engine` and the `GameObject` lifecycle active (`load`, `spawn`, `tick`, `event`, `despawn`), the simulation requires a production-grade 3D physics backend. The engine needs deterministic fixed-timestep simulation, character collision, raycasting for first-person reticle interactions, and dynamic rigid bodies for environmental props (doors, thrown debris, loot containers).

## Architecture Decision: Why Jolt Physics
Jolt Physics (`JoltC` C-ABI) is selected over Bullet, PhysX, and pure-Go alternatives for the following reasons:

1. **Native C-ABI Direct to LuaJIT FFI:**
   - Jolt exposes a clean C-ABI (`JoltC`). This allows LuaJIT scripts to execute high-frequency raycasts (reticle hover, weapon hitscans, line-of-sight checks) directly via `ffi.C` at native machine-code speed.
   - Completely bypasses CGo call overhead and GC allocations on hot paths. Go handles world lifecycle and stepping, while Lua interacts directly with C pointers.

2. **Horde & Ragdoll Multi-Threading:**
   - Engineered from the ground up for modern multi-core systems with job-system integration.
   - Effortlessly scales to dense zombie crowds pressing through chokepoints and dynamic ragdolls without tanking the main simulation thread.

3. **Character Virtual Controller:**
   - Provides a battle-tested kinematic character virtual interface designed for first-person games.
   - Eliminates standard rigid-body capsule jitter when traversing stairs, door thresholds, slopes, and newly aligned corner-floor seams.

4. **Continuous Collision Detection (CCD):**
   - Robust built-in CCD prevents fast-moving projectiles, thrown objects, or high-velocity zombie attacks from tunneling through walls or thin doors.

## Objectives
1. Integrate the Jolt Physics C-ABI (`JoltC`) into `project-engine` with a fixed-timestep accumulator (60 Hz).
2. Establish zero-allocation FFI bindings in `pkg/luabridge` for spatial raycasts and impulse application.
3. Bind character virtual controllers and static geometry colliders to the `GameObject` component tree.

## Scope of Work
- [x] Compile and link `JoltC` shared/static library within the engine build chain.
- [x] Implement `PhysicsWorld` wrapper in Go to manage allocation, gravity, broadphase limits, and `StepSimulation(1/60.0)`.
- [x] Implement static mesh/compound collider generation from map/room importer data (floors, walls, corner seams).
- [x] Add `RigidBody` and `CharacterController` components to `engine/spatial`.
- [x] Expose `JoltC` FFI C-declarations (`cdef`) in LuaJIT for:
  - `Raycast(origin, dir, max_distance) -> HitResult`
  - `AddImpulse(body_id, vec3)`
  - `SetVelocity(body_id, vec3)`
- [x] Synchronize Jolt body transforms to the `Renderable` hierarchical transform matrices at the end of each physics tick.

## Acceptance Criteria
- Engine ticks physics deterministically at 60 Hz independent of render framerate.
- First-person reticle raycast runs via LuaJIT FFI with zero Go GC allocations per frame.
- Player character controller navigates stairs and corner floor geometry smoothly without clipping or snapping.
- Dynamic objects (kicked balls, thrown alarm clocks) collide accurately with static room geometry and sleep when resting.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0006-jolt-physics-luajit-ffi
- **Branch:** 0006-jolt-physics-luajit-ffi
- **Summary of Changes:**
  - Compiled `JoltPhysics v5.7.0` + `amerkoleci/joltc` C-ABI into static library `libjoltc.a` (`engine/spatial/thirdparty/joltc/lib/windows/amd64/libjoltc.a`) with headers (`engine/spatial/thirdparty/joltc/include/joltc.h`) and pure-C exported FFI helpers (`engine/spatial/jolt_ffi.h`, `engine/spatial/jolt_ffi.c`).
  - Implemented `PhysicsWorld` in `engine/spatial/physics_world.go` managing reference-counted `JPH_Init`/`JPH_Shutdown`, broadphase/object layer filters, gravity, 60 Hz (`1/60.0`) fixed-timestep accumulator `StepSimulation`, narrowphase `Raycast`, and end-of-tick `SyncTransforms()` updating `GameObject.Transform` (which feeds `Renderable` hierarchical world transforms).
  - Implemented `BuildWorldMapCollider`, `BuildWorldMapColliderFromBounds`, `BuildStaticCompoundFromBoxes` (`JPH_StaticCompoundShape`), and `BuildStaticMeshCollider` (`JPH_MeshShapeSettings`) in `engine/spatial/world_collider.go`, plus `WorldMap3D.ExportStaticColliderBounds()` in `pkg/tileimporter/world_map3d.go` (decoupled via `MapColliderSource` to avoid `raylib-go`/`glfw` symbol collisions).
  - Implemented `RigidBody` (Box/Sphere/Capsule, Static/Kinematic/Dynamic, optional `LinearCast` CCD, `engine.Component` + `engine.PhysicsBodyHandle`) and `CharacterController` (`JPH_CharacterVirtual` with `ExtendedUpdate`, stair-climbing `walkStairsStepUp`, `stickToFloorStepDown`, and `EnhancedInternalEdgeRemoval`) in `engine/spatial/components.go`.
  - Exposed JoltC FFI C-declarations (`JoltVec3`, `JoltHitResult` / `HitResult`, `Jolt_RaycastOut`, `Jolt_Raycast`, `Jolt_AddImpulseVec3`, `Jolt_SetVelocityVec3`) in `pkg/luabridge/luajit.go` and `pkg/luabridge/physics_luajit.go`, plus `BindPhysicsWorld` installing zero-allocation LuaJIT helpers (`Raycast`, `RaycastInto`, `AddImpulse`, `SetVelocity`).
  - Added headless physics, collider, CCD, stair/corner navigation, Renderable sync, and LuaJIT zero-allocation + `jit.v` trace tests in `engine/spatial/physics_test.go`, `pkg/luabridge/physics_luajit_test.go`, and `pkg/tileimporter/importer_test.go`.
- **Files Modified / Added:**
  - [engine/spatial/thirdparty/joltc/README.md](file:///C:/Users/samue/Projects/project-engine/engine/spatial/thirdparty/joltc/README.md)
  - [engine/spatial/thirdparty/joltc/include/joltc.h](file:///C:/Users/samue/Projects/project-engine/engine/spatial/thirdparty/joltc/include/joltc.h)
  - [engine/spatial/thirdparty/joltc/lib/windows/amd64/libjoltc.a](file:///C:/Users/samue/Projects/project-engine/engine/spatial/thirdparty/joltc/lib/windows/amd64/libjoltc.a)
  - [engine/spatial/jolt_ffi.h](file:///C:/Users/samue/Projects/project-engine/engine/spatial/jolt_ffi.h)
  - [engine/spatial/jolt_ffi.c](file:///C:/Users/samue/Projects/project-engine/engine/spatial/jolt_ffi.c)
  - [engine/spatial/physics_world.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/physics_world.go)
  - [engine/spatial/world_collider.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/world_collider.go)
  - [engine/spatial/components.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/components.go)
  - [engine/spatial/physics_test.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/physics_test.go)
  - [pkg/luabridge/luajit.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/luajit.go)
  - [pkg/luabridge/physics_luajit.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/physics_luajit.go)
  - [pkg/luabridge/physics_luajit_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/physics_luajit_test.go)
  - [pkg/tileimporter/world_map3d.go](file:///C:/Users/samue/Projects/project-engine/pkg/tileimporter/world_map3d.go)
  - [pkg/tileimporter/importer_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/tileimporter/importer_test.go)
  - [tickets/0006-jolt-physics-luajit-ffi.complete.md](file:///C:/Users/samue/Projects/project-engine/tickets/0006-jolt-physics-luajit-ffi.complete.md)
- **Verification Results:**
  - `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 ./engine ./engine/ui ./engine/spatial ./pkg/luabridge ./pkg/tileimporter`: All 5 packages passed (`engine`, `engine/ui`, `engine/spatial`, `pkg/luabridge`, `pkg/tileimporter`).
  - `go vet ./engine ./engine/ui ./engine/spatial ./pkg/luabridge ./pkg/tileimporter` & `go build ./engine ./engine/ui ./engine/spatial ./pkg/luabridge ./pkg/tileimporter`: Passed with 0 issues.
