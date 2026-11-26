# LuaJIT Scripting Standards & Zero-Allocation Guide

All engine scripts executed on high-frequency paths (per-frame `tick`, `render`, UI layout math, animation easing, and pointer hit-testing) run under **LuaJIT 2.1** and **MUST** adhere to the zero-allocation FFI standards below. This guarantees `0.00 KB` garbage collection pressure per frame and uninterrupted JIT trace compilation.

---

## 1. The Zero-Allocation Mandate

Dynamic Lua table creation (`{ x = 0, y = 0 }`) is **strictly prohibited** inside any update, render, layout, animation, or input callback:

```lua
-- PROHIBITED in frame loops (allocates a GC table every call):
function bad_tick(dt)
    local pos = { x = 10, y = 20 }
    return pos
end
```

All high-frequency data structures must use pre-allocated LuaJIT FFI `cdata` structs written in place.

---

## 2. Canonical FFI Struct Idioms

The engine pre-registers core UI and math C types via `ffi.cdef` in [`pkg/luabridge/luajit.go`](../pkg/luabridge/luajit.go):

```lua
local ffi = require("ffi")
ffi.cdef[[
    typedef struct { float x, y, w, h; } Rect;
    typedef struct { float x, y; } Vec2;
    typedef struct { uint8_t r, g, b, a; } Color32;

    typedef struct {
        Rect bounds;
        Color32 color;
        float rounding;
        int32_t id_hash;
        uint8_t visible;
        uint8_t hovered;
    } UILayoutNode;
]]
```

Write directly to struct fields (`rect.x`, `rect.y`, `rect.w`, `rect.h`) and pass destination pointers for composite operations (e.g., `intersect_rects(out, a, b)` or `lerp_rect(out, a, b, t)`).

---

## 3. Allocation Sinking & Static Scratch Structs

LuaJIT's `SINK` optimization can eliminate short-lived `ffi.new` allocations that never escape a single compiled trace into registers or stack slots. However, any `cdata` that crosses C boundaries, escapes a trace side-exit, or persists across calls will force heap allocation unless pre-allocated.

Prescribed patterns:
1. **Module-Scoped Scratch Structs:** Allocate reusable scratch values once at module load time:
   ```lua
   local SCRATCH_RECT = ffi.new("Rect")
   local SCRATCH_VEC  = ffi.new("Vec2")
   ```
2. **Contiguous Fixed-Size Pools:** Store dynamic UI hierarchies in flat `cdata` arrays rather than Lua tables:
   ```lua
   local MAX_NODES = 64
   local NODE_POOL = ffi.new("UILayoutNode[?]", MAX_NODES)
   ```

---

## 4. Iteration Protocols

- **REQUIRED:** Numeric `for` loops with 0-based indexing over contiguous `cdata` arrays:
  ```lua
  for i = 0, count - 1 do
      local node = NODE_POOL[i]
      -- mutate node.bounds in place
  end
  ```
- **FORBIDDEN:** `pairs()` and `ipairs()` in frame loops (`pairs()` cannot be JIT-compiled in LuaJIT 2.1 and triggers `NYI` trace aborts).

---

## 5. String Allocation Ban

String concatenation (`..`) allocates an interned `GCstr` on the Lua heap and is **forbidden** inside hot loops:
- Pre-allocate static label strings at module initialization (`local LABEL_HUD = "HUD"`).
- Pass static strings or pre-allocated `char[N]` C buffers directly to C-level draw calls (`ffi.C.igImDrawList_AddText`, `ffi.C.DrawList_AddText`).

---

## 6. Prohibited Bytecode & NYI Traps

Do not invoke any of the following inside frame-critical paths:
- `pairs()`, `next()` on hash tables
- `table.insert`, `table.remove`, `table.sort`
- `string.dump`, `loadstring`, `load`
- Deep recursion exceeding trace unroll limits
- Passing `lightuserdata` directly to FFI functions on x64 (convert addresses once or pass `nil` / `cdata` pointers via `ffi.cast("void*", addr)`)

---

## 7. Profiling Rules (`jit.v` & `jit.dump`)

Embedded `jit.v` and `jit.dump` modules are preloaded by `LuaJITBridge` so you can audit traces in any test or debug build:

```lua
local jit_v = require("jit.v")
jit_v.start("jit_verbose.log")
-- run 200+ frame loop
jit_v.off()
```

Or in Go tests via `bridge.StartJITVerbose(path)` / `bridge.StartJITDump("bitmsx", path)`.

### Verification Checklist
1. Confirm compiled traces appear (`TRACE ... -> loop` or `-> return`) in `jit.v` / `jit.dump`.
2. Confirm **zero** `NYI` lines appear in the trace log.
3. Run `tests/ui_benchmark.lua` (or `go test ./pkg/luabridge -run TestUIZeroAllocationBenchmark`) and verify `0.00 KB` GC growth across 1,000 continuous frames.
