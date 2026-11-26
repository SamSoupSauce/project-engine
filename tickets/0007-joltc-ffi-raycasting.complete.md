# Ticket 0007: JoltC C-ABI Bindings & LuaJIT FFI Raycasting

## Status
Complete

## Context
Following the selection of Jolt Physics in Ticket 0006, high-frequency spatial queries—specifically first-person reticle hover checks, object interactions, and weapon hitscans—must not route through CGo due to call overhead and garbage collection friction. This ticket establishes the lightweight C export layer (`jolt_c_api`) and matching LuaJIT FFI declarations (`cdef`) for zero-allocation raycasting directly from Lua.

## Objectives
1. Provide a minimal, flat C wrapper around the Jolt Physics System specifically for raycast queries.
2. Define the corresponding `ffi.cdef` structures and signatures in `pkg/luabridge`.
3. Implement a Lua helper module (`ffi_jolt.lua` / `physics`) that performs reticle queries into a preallocated `cdata` hit result.

## Technical Architecture

### 1. Flat C-ABI Interface (`native/jolt_c_api.h`)
Expose minimal C-compatible types and a stateless query function:

```c
#ifndef JOLT_C_API_H
#define JOLT_C_API_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    float x, y, z;
} JoltVec3;

typedef struct {
    bool     has_hit;
    uint32_t body_id;
    float    fraction;
    JoltVec3 hit_position;
    JoltVec3 hit_normal;
    uint32_t user_data; // Maps to GameObject / Entity ID
} JoltRaycastHit;

// Raycast against the active Jolt physics world
bool Jolt_CastRay(
    void* physics_system_ptr,
    const JoltVec3* origin,
    const JoltVec3* direction,
    float max_distance,
    uint16_t collision_layer_mask,
    JoltRaycastHit* out_hit
);

#ifdef __cplusplus
}
#endif

#endif // JOLT_C_API_H
```

### 2. LuaJIT FFI Definitions (`pkg/luabridge/ffi_jolt.lua`)

Register the C types with LuaJIT's compiler and bind the shared library:

```lua
local ffi = require("ffi")

ffi.cdef[[
typedef struct {
    float x, y, z;
} JoltVec3;

typedef struct {
    bool     has_hit;
    uint32_t body_id;
    float    fraction;
    JoltVec3 hit_position;
    JoltVec3 hit_normal;
    uint32_t user_data;
} JoltRaycastHit;

bool Jolt_CastRay(
    void* physics_system_ptr,
    const JoltVec3* origin,
    const JoltVec3* direction,
    float max_distance,
    uint16_t collision_layer_mask,
    JoltRaycastHit* out_hit
);
]]

-- Load native engine binary containing compiled JoltC exports
local JoltLib = ffi.C

local M = {}

-- Preallocate cdata buffers to avoid per-frame GC churn on tick/hover
local ray_origin = ffi.new("JoltVec3")
local ray_dir    = ffi.new("JoltVec3")
local hit_result = ffi.new("JoltRaycastHit")

function M.cast_ray(world_ptr, ox, oy, oz, dx, dy, dz, max_dist, layer_mask)
    ray_origin.x, ray_origin.y, ray_origin.z = ox, oy, oz
    ray_dir.x,    ray_dir.y,    ray_dir.z    = dx, dy, dz

    local hit = JoltLib.Jolt_CastRay(
        world_ptr,
        ray_origin,
        ray_dir,
        max_dist or 100.0,
        layer_mask or 0xFFFF,
        hit_result
    )

    if hit and hit_result.has_hit then
        return hit_result
    end
    return nil
end

return M
```

## Scope of Work

* [x] Implement `native/jolt_c_api.cpp` wrapping `JPH::PhysicsSystem::GetNarrowPhaseQuery().CastRay(...)`.
* [x] Export `Jolt_CastRay` in the build system (CMake / engine dynamic library output).
* [x] Implement `pkg/luabridge/ffi_jolt.lua` with preallocated struct caching.
* [x] Pass the engine's raw `PhysicsSystem` pointer to Lua on state initialization.
* [x] Connect the interaction reticle update tick to `physics.cast_ray` to drive hover highlights.

## Acceptance Criteria

* Calling `physics.cast_ray` at 60 Hz from a Lua script generates zero Lua garbage collections (`collectgarbage("count")` remains flat).
* Raycast hits accurately return the `body_id`, hit coordinate, surface normal, and entity `user_data`.
* First-person reticle cleanly detects interactable static geometry and dynamic props up to max distance.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0007-joltc-ffi-raycasting
- **Branch:** 0007-joltc-ffi-raycasting
- **Summary of Changes:**
  - Implemented `native/jolt_c_api.h`, `native/jolt_c_api.cpp`, and `native/CMakeLists.txt` providing the flat C-ABI `JoltVec3`, `JoltRaycastHit`, and exported `Jolt_CastRay` with lock-free cached `JPH_ObjectLayerFilter` masking, surface normal lookup, and entity `user_data` extraction.
  - Bridged `native/jolt_c_api.cpp` into `engine/spatial` (`engine/spatial/jolt_c_api_bridge.cpp`, `engine/spatial/physics_world.go`), added `UserData` and `RaycastHit.UserData` (`engine/physics_handle.go`, `engine/spatial/components.go`, `engine/spatial/world_collider.go`), and added collision layers `LayerInteractable` / `LayerProp` with `RaycastMask`.
  - Implemented `pkg/luabridge/ffi_jolt.lua` with preallocated `ray_origin`, `ray_dir`, `hit_result` `cdata`, `M.cast_ray`, and `M.cast_ray_into`, embedded and preloaded as `"ffi_jolt"` and `"physics"` in `pkg/luabridge/physics_luajit.go` alongside `NewLuaJITBridgeWithPhysics` and `PHYSICS_SYSTEM_PTR`.
  - Implemented zero-allocation reticle hover tracking & rendering (`update_reticle_hover`, `render_reticle`, `get_reticle_state`) in `scripts/ui/reticle_hover.lua` using `ffi_jolt`.
  - Added tests in `engine/spatial/physics_test.go` and `pkg/luabridge/physics_luajit_test.go` verifying layer mask filtering, entity `user_data`, reticle hover highlight rendering, `0.00 KB` GC growth across 1,000 frames, and `jit.v` traces with zero `NYI` aborts.
- **Files Modified / Added:**
  - [native/jolt_c_api.h](file:///C:/Users/samue/Projects/project-engine/native/jolt_c_api.h)
  - [native/jolt_c_api.cpp](file:///C:/Users/samue/Projects/project-engine/native/jolt_c_api.cpp)
  - [native/CMakeLists.txt](file:///C:/Users/samue/Projects/project-engine/native/CMakeLists.txt)
  - [engine/physics_handle.go](file:///C:/Users/samue/Projects/project-engine/engine/physics_handle.go)
  - [engine/spatial/jolt_ffi.h](file:///C:/Users/samue/Projects/project-engine/engine/spatial/jolt_ffi.h)
  - [engine/spatial/jolt_c_api_bridge.cpp](file:///C:/Users/samue/Projects/project-engine/engine/spatial/jolt_c_api_bridge.cpp)
  - [engine/spatial/physics_world.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/physics_world.go)
  - [engine/spatial/world_collider.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/world_collider.go)
  - [engine/spatial/components.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/components.go)
  - [engine/spatial/physics_test.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/physics_test.go)
  - [pkg/luabridge/ffi_jolt.lua](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/ffi_jolt.lua)
  - [pkg/luabridge/physics_luajit.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/physics_luajit.go)
  - [pkg/luabridge/physics_luajit_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/physics_luajit_test.go)
  - [scripts/ui/reticle_hover.lua](file:///C:/Users/samue/Projects/project-engine/scripts/ui/reticle_hover.lua)
  - [tickets/0007-joltc-ffi-raycasting.complete.md](file:///C:/Users/samue/Projects/project-engine/tickets/0007-joltc-ffi-raycasting.complete.md)
- **Verification Results:**
  - `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 ./engine ./engine/ui ./engine/spatial ./pkg/luabridge ./pkg/tileimporter`: All 5 packages passed.
  - `go vet` & `go build` across all 5 packages: Passed with 0 issues.
