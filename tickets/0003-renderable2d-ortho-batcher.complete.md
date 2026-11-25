# 0003: Implement Renderable2D and Orthographic Quad Batcher

## Summary
Implement a native 2D orthographic rendering pipeline and `Renderable2D` component for in-game HUD elements, UI text, health bars, inventory panels, and crosshairs. The system batches textured quads and sprites into dynamic vertex buffers to minimize draw calls.

## Requirements
- Define `Renderable2D` component in `engine/`:
  - `TextureID`: GPU handle for the sprite or UI atlas.
  - `SourceRect`: Normalized UV rectangle coordinates $(U, V, W, H)$ for sprite sheet slicing.
  - `Size`: 2D pixel or viewport dimensions $(Width, Height)$.
  - `Anchor`: Screen alignment anchor enum (TopLeft, Center, BottomRight, etc.).
  - `Color`: 32-bit RGBA tint / opacity multiplier.
  - `ZIndex`: Sorting layer for 2D depth ordering.
  - `Flags`: Bitmask flags (`FlagVisible`, `FlagOverrideParent`, etc.).
- Build an orthographic quad batcher (`OrthoBatcher2D`):
  - Maintain a dynamic vertex buffer (VBO/EBO) supporting textured quads (position, UV, color).
  - Automatically batch draw commands sharing the same `TextureID` and blend state into single `glDrawElements` calls.
  - Flush batch on texture state changes, buffer capacity limits, or explicit pass completion.
- Integrate into the main camera/render pipeline:
  - Add an orthographic pass that executes after the 3D scene pass and before the debug ImGui pass.
  - Set an orthographic projection matrix matching the current GLFW window width and height.
  - Disable depth writes while preserving alpha blending (`GL_SRC_ALPHA`, `GL_ONE_MINUS_SRC_ALPHA`).
- Expose basic 2D component mutations to Lua scripts via `pkg/luabridge` (e.g., updating health bar width, toggling crosshair visibility, changing tint color).

## Verification & Acceptance Criteria
- [x] Multiple `Renderable2D` quads sharing a single texture atlas render in a single batched draw call.
- [x] Orthographic viewport automatically resizes and maintains correct aspect scaling on window resize events.
- [x] Alpha blending renders transparent PNG sprites without black borders or depth-fighting artifacts.
- [x] Dynamic transforms and property updates (size, color, UV offsets) made via Lua script reflect immediately in the HUD.
- [x] Z-index sorting renders foreground UI elements on top of background panels predictably.

---
### Clarification Log
<!-- Agent logs clarification questions asked in console and user answers here, or marks as clear -->
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0003-renderable2d-ortho-batcher
- **Branch:** 0003-renderable2d-ortho-batcher (branched from `0002-dear-imgui-overlay` and merged `0001-gameobject-lua-scripting`)
- **Summary of Changes:**
  - Added `Renderable2D`, `Vector2`, `Rect2D`, `Anchor2D` (all 9 screen anchors), `BlendMode2D`, `QuadCommand2D`, and `Extract2DFromHierarchy` (visibility cascade, anchored coordinate resolution, and stable `ZIndex` sorting) in `engine/renderable2d.go`.
  - Implemented `OrthoBatcher2D` in `engine/ortho_batcher2d.go` with dynamic VBO/EBO/VAO + GLSL 3.30 Core shader, 1x1 white fallback texture, `CreateTextureRGBA` (`GL_CLAMP_TO_EDGE` + `GL_RGBA`), automatic batch flushing on texture/blend/capacity changes into single `glDrawElements` calls, `Ortho2DMatrix` projection, depth-write disabling, and `GL_SRC_ALPHA`/`GL_ONE_MINUS_SRC_ALPHA` blending.
  - Integrated `OrthoBatcher2D` into `GLApp` (`engine/gl_window.go`) so the 2D orthographic pass executes after the 3D scene pass and before the ImGui overlay pass using the current GLFW window dimensions.
  - Exposed `Renderable2D` userdata (`GetRenderable2D`, `AddRenderable2D`, `SetSize`, `SetWidth`, `SetHeight`, `SetColor`, `SetUV`, `SetVisible`, `SetZIndex`, `SetTextureID`, `SetAnchor`, and property accessors) plus `set_ui_size`, `set_ui_color`, `set_ui_visible`, and `set_ui_uv` global helpers in `pkg/luabridge/userdata.go` and `pkg/luabridge/bridge.go`.
  - Added unit, Lua integration, and live OpenGL 3.3 tests in `engine/renderable2d_test.go` and `pkg/luabridge/renderable2d_lua_test.go`.
- **Files Modified / Added:**
  - [engine/renderable2d.go](file:///C:/Users/samue/Projects/project-engine/engine/renderable2d.go)
  - [engine/ortho_batcher2d.go](file:///C:/Users/samue/Projects/project-engine/engine/ortho_batcher2d.go)
  - [engine/gl_window.go](file:///C:/Users/samue/Projects/project-engine/engine/gl_window.go)
  - [engine/scene.go](file:///C:/Users/samue/Projects/project-engine/engine/scene.go)
  - [engine/renderable2d_test.go](file:///C:/Users/samue/Projects/project-engine/engine/renderable2d_test.go)
  - [pkg/luabridge/bridge.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge.go)
  - [pkg/luabridge/userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/userdata.go)
  - [pkg/luabridge/script_component_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/script_component_test.go)
  - [pkg/luabridge/renderable2d_lua_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/renderable2d_lua_test.go)
- **Verification Results:**
  - `CGO_LDFLAGS_ALLOW="-l:.*\.a" go test -count=1 -v ./...`: All 26 tests passed (`engine`: 12, `pkg/luabridge`: 10, `pkg/tileimporter`: 4).
  - `go vet ./...` & `go build ./...`: Passed with 0 issues.
