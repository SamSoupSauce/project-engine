# Ticket 0004: Stateful GUI System, LuaJIT Bridge Port & Tracing Audit

**Status:** Complete

**Assignee:** Core Engine / Scripting Runtime

**Components:** `engine/ui`, `pkg/luabridge`, `pkg/render`

**Dependencies:** Ticket 0002 (Dear ImGui integration), Ticket 0003 (`Renderable2D` pipeline)

---

## 1. Overview & Objectives

Implement a retained, stateful GUI layer driven by a declarative JSON/Lua node hierarchy on top of Go, backed by `ImDrawList` rasterization. Migrate the scripting bridge from standard Lua/GopherLua to **LuaJIT**, establishing zero-allocation FFI boundaries and auditing all hot-path UI scripts against LuaJIT NYI (Not Yet Implemented) trace aborts.

### Core Goals

1. **Lightweight Flexbox Layout in Pure Go:** Single-pass/two-pass recursive box solver resolving `Direction` (row/column), `Grow`, `Shrink`, `Gap`, `Padding`, and explicit/percentage dimensions.
2. **Retained Node Hierarchy:** Persistent Go structs maintaining widget trees, dirty layout flags, and spatial bounds (`x, y, w, h`).
3. **LuaJIT Migration & FFI Bridge:** Replace reflection/stack-based bindings with LuaJIT C-FFI for high-frequency layout updates, hit-testing dispatch, and draw primitive submission.
4. **Zero-Allocation Paint Pass:** Direct emission of bounding quads, text, and scissor rects into `ImDrawList` via contiguous memory blocks.
5. **JIT Compatibility Audit:** Enforce strict coding standards across UI scripts to ensure zero JIT trace aborts during frame-critical loops.

---

## 2. Technical Specification & Tasks

### Task 1: Pure Go Lightweight Flexbox Layout Engine (`engine/ui/layout.go`)

* Implement minimal retained node structure:
```go
type FlexDirection uint8
const (
    FlexRow FlexDirection = iota
    FlexColumn
)

type LayoutStyle struct {
    Direction FlexDirection
    Grow      float32
    Shrink    float32
    Gap       float32
    Padding   [4]float32 // top, right, bottom, left
    Width     float32
    Height    float32
}

type UINode struct {
    ID       string
    Style    LayoutStyle
    Bounds   [4]float32 // computed: x, y, w, h
    Dirty    bool
    Parent   *UINode
    Children []*UINode
    OnClick  string
    OnHover  string
}

```


* Implement recursive `ComputeLayout(root *UINode, parentWidth, parentHeight float32)`:
* Traverse dirty subtrees only; cache bounds when inputs remain static.
* Compute main-axis distribution (gap calculation + free space distribution via `Grow`).
* Compute cross-axis alignment (`stretch`, `center`, `start`, `end`).



### Task 2: LuaJIT Engine Bridge Migration (`pkg/luabridge/luajit.go`)

* Integrate LuaJIT runtime via CGo wrapper targeting native LuaJIT dynamic/static libraries.
* Expose UI structs to LuaJIT using `ffi.cdef`:
```lua
local ffi = require("ffi")
ffi.cdef[[
    typedef struct {
        float x, y, w, h;
    } UIRect;

    typedef struct {
        float r, g, b, a;
    } UIColor;

    void DrawList_AddRectFilled(void* draw_list, float min_x, float min_y, float max_x, float max_y, uint32_t color, float rounding);
    void DrawList_PushClipRect(void* draw_list, float min_x, float min_y, float max_x, float max_y, bool intersect);
    void DrawList_PopClipRect(void* draw_list);
]]

```


* Wire Go-exported C functions for `DrawList` pass-through directly to Dear ImGui's backend.

### Task 3: JSON UI Deserializer & Event Dispatcher (`engine/ui/loader.go`, `engine/ui/events.go`)

* Parse declarative UI templates from JSON into `UINode` trees.
* Support binding event properties (`on_click`, `on_hover`) to global or modular Lua handler functions.
* Implement 2D bounding-box hit-testing traversal for pointer events:
* Fire events directly across the LuaJIT bridge passing `(node_id, mouse_x, mouse_y)`.
* Support simple bubbling or early-exit capture.



### Task 4: LuaJIT Hot-Loop Compatibility Audit (`scripts/ui/*.lua`)

Audit and rewrite all UI Lua scripts to prevent JIT bailouts to the bytecode interpreter:

* **Eliminate `pairs()`:** Replace all table dictionary loops in the frame loop with flat array indices and numeric `for i = 1, #t do` loops.
* **Eliminate Table Allocations in Render/Update Hooks:**
* Forbid constructing table literals `{ x = ..., y = ... }` inside `on_render` or animation callbacks.
* Use pre-allocated `ffi.new("UIRect")` instances or reuse static module-level scratch structs.


* **Avoid String Concatenation in Frame Routines:** Disallow `..` string formatting in dynamic labels; use fixed text buffers or push formatted values directly through C-level draw commands.
* **Eliminate Uncompiled Bytecode Traps:** Ensure no calls to `string.dump`, `loadstring`, `table.insert` in tight loops, or deep recursion beyond trace limits.
* **Add Trace Profiling:** Embed `require("jit.v").start()` and `require("jit.dump").start()` hooks in debug mode to verify traces compile without `NYI` warnings.

---

## 3. Acceptance Criteria

* [x] `ComputeLayout` correctly calculates bounds for nested flex rows and columns without leaking memory or allocating on clean frames.
* [x] JSON templates deserialize into valid `UINode` trees and immediately render expected bounding rects via `ImDrawList`.
* [x] Pointer clicks trigger configured Lua functions via LuaJIT FFI in `< 0.05ms`.
* [x] `jit.dump` confirms zero NYI aborts during continuous 60+ FPS UI interaction loops (dragging, hovering, layout resizing).
* [ ] Cross-compilation targets (`linux/amd64`, `windows/amd64`) compile cleanly against the configured LuaJIT CGo toolchain. *(Note: `windows/amd64` compiles and passes all tests natively; `#cgo linux,amd64` flags are configured for `-lluajit-5.1 -lm -ldl`, while `linux/amd64` Cgo cross-compilation was not run on this Windows host because no `x86_64-linux-gnu-gcc` cross-compiler is installed.)*

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0004-stateful-gui-luajit-audit
- **Branch:** 0004-stateful-gui-luajit-audit (branched from `0003-renderable2d-ortho-batcher`)
- **Summary of Changes:**
  - Built and vendored LuaJIT 2.1 static library (`libluajit.a`), headers, and embedded `jit/*.lua` profiling modules (`jit.v`, `jit.dump`, `jit.vmdef`, etc.) under `pkg/luabridge/thirdparty/luajit/`.
  - Implemented pure Go flexbox layout solver `ComputeLayout` (`engine/ui/layout.go`) supporting `FlexRow`/`FlexColumn`, `Grow`, `Shrink`, `Gap`, `Padding`, fixed/percentage dimensions, cross-axis alignment (`stretch`, `start`, `center`, `end`), and dirty-subtree caching with 0 heap allocations on clean frames.
  - Implemented declarative JSON UI template loader (`engine/ui/loader.go`), 2D hit-testing & bubbling `PointerEventDispatcher` (`engine/ui/events.go`), and `ImDrawList` tree painter (`engine/ui/render.go`).
  - Implemented `LuaJITBridge` (`pkg/luabridge/luajit.go`) and exported `ImDrawList` C/FFI pass-through functions (`DrawList_AddRectFilled`, `DrawList_PushClipRect`, `DrawList_PopClipRect`, `DrawList_AddText`, `igImDrawList_AddRectFilled`, `igImDrawList_AddText`) in `pkg/luabridge/luajit_exports.go` with embedded `jit.v`/`jit.dump` profiling hooks.
  - Created JIT-audited UI script `scripts/ui/hud_overlay.lua` using pre-allocated `ffi.new` scratch structs and contiguous `UILayoutNode` pools with zero table allocations, zero string concatenations, and numeric-only loops.
  - Added tests in `engine/ui/ui_test.go` and `pkg/luabridge/luajit_test.go` verifying layout math, 0-alloc clean frames, JSON load + `ImDrawList` rendering, pointer click latency (`29.005µs` avg < `0.05ms`), and `jit.dump` zero-NYI-abort trace compilation.
- **Files Modified / Added:**
  - [engine/ui/layout.go](file:///C:/Users/samue/Projects/project-engine/engine/ui/layout.go)
  - [engine/ui/loader.go](file:///C:/Users/samue/Projects/project-engine/engine/ui/loader.go)
  - [engine/ui/events.go](file:///C:/Users/samue/Projects/project-engine/engine/ui/events.go)
  - [engine/ui/render.go](file:///C:/Users/samue/Projects/project-engine/engine/ui/render.go)
  - [engine/ui/ui_test.go](file:///C:/Users/samue/Projects/project-engine/engine/ui/ui_test.go)
  - [pkg/luabridge/luajit.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/luajit.go)
  - [pkg/luabridge/luajit_exports.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/luajit_exports.go)
  - [pkg/luabridge/luajit_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/luajit_test.go)
  - [pkg/luabridge/thirdparty/luajit/](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/thirdparty/luajit)
  - [scripts/ui/hud_overlay.lua](file:///C:/Users/samue/Projects/project-engine/scripts/ui/hud_overlay.lua)
- **Verification Results:**
  - `CGO_LDFLAGS_ALLOW="-l:.*\.a" go test -count=1 -v ./...`: All 33 tests passed (`engine`: 12, `engine/ui`: 4, `pkg/luabridge`: 13, `pkg/tileimporter`: 4).
  - `go vet ./...` & `go build ./...`: Passed with 0 issues.
