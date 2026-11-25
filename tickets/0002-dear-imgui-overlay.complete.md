# 0002: Integrate Dear ImGui Overlay for Debug Tools and Console

## Summary
Integrate Dear ImGui into the core engine using GLFW and OpenGL 3 backends. This provides an immediate-mode overlay pipeline pass for developer tooling, performance metrics, and the in-game Lua console without interfering with the 3D scene rendering state.

## Requirements
- Add Dear ImGui bindings (`cimgui-go` or equivalent modern Cgo wrapper) to `engine/`.
- Hook GLFW window callbacks to ImGui's input dispatcher:
  - Capture mouse movement, button clicks, scroll offsets, and text/keyboard character inputs.
  - Expose an input-capture query (`WantCaptureMouse`, `WantCaptureKeyboard`) so game input handling can yield when UI windows are focused.
- Create an ImGui lifecycle wrapper within the engine:
  - `Init(window *glfw.Window)`: Setup context, build font atlases, and initialize OpenGL 3 shader pipelines.
  - `NewFrame()`: Poll inputs and start the per-frame ImGui context before UI definitions.
  - `Render()`: Flush ImGui draw lists as the final pipeline overlay pass immediately before `glfw.SwapBuffers`.
- Ensure the OpenGL render pass preserves engine pipeline state:
  - Backup and restore OpenGL state (viewport, blend mode, depth test, scissor test, active program) around the ImGui draw pass to prevent 3D pipeline corruption.
- Connect `pkg/luabridge` console output:
  - Provide a toggleable debug console window (toggled via backtick/tilde or configured key) that displays engine logs and accepts command inputs dispatched to Lua.

## Verification & Acceptance Criteria
- [x] Engine compiles and links Dear ImGui with Cgo enabled.
- [x] ImGui demo window or custom metrics window renders cleanly on top of the 3D scene.
- [x] Keyboard and mouse inputs interact with ImGui widgets without leaking into game camera controls when `WantCaptureMouse` / `WantCaptureKeyboard` are active.
- [x] OpenGL depth and blend states in the 3D scene graph remain uncorrupted before and after the ImGui render pass.
- [x] In-game console window toggles open/closed, accepts text input, and dispatches to `pkg/luabridge`.

---
### Clarification Log
<!-- Agent logs clarification questions asked in console and user answers here, or marks as clear -->
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0002-dear-imgui-overlay
- **Branch:** 0002-dear-imgui-overlay
- **Summary of Changes:**
  - Integrated Dear ImGui (`github.com/AllenDang/cimgui-go/imgui`, `impl/opengl3`), GLFW 3.3 (`github.com/go-gl/glfw/v3.3/glfw`), and OpenGL 3.3 Core (`github.com/go-gl/gl/v3.3-core/gl`) with Cgo enabled.
  - Replaced `raylib-go` references inside `engine/` and `pkg/luabridge/bridge_test.go` with pure-Go math/graphics primitives (`engine/types.go`) and migrated `Engine.Run()` to `GLApp` (`engine/gl_window.go`) to avoid duplicate GLFW C symbol linkage.
  - Implemented `ImGuiOverlay` (`engine/imgui_overlay.go`) providing `Init(window *glfw.Window)`, `InitHeadless(width, height)`, `NewFrame()`, `DrawDebugWindows()`, `Render()`, `Shutdown()`, GLFW input callback hooking with chaining, `WantCaptureMouse()` / `WantCaptureKeyboard()` game-input guards, and optional Demo / Engine Metrics windows.
  - Implemented `BackupGLState` and `RestoreGLState` (`engine/gl_state.go`) around the ImGui draw pass to preserve viewport, scissor box, active shader program, active texture, VAO/VBO bindings, blend mode/func/equation, depth test/write mask, cull face, and scissor test state.
  - Implemented `DebugConsoleWindow` (`engine/imgui_console.go`) and `luabridge.Console` adapter methods (`pkg/luabridge/console_adapter.go`) for a grave-accent/tilde-toggleable Dear ImGui console executing Lua commands via `pkg/luabridge`.
  - Note: Building `cimgui-go` v1.6.1 with Cgo on Windows requires `CGO_LDFLAGS_ALLOW="-l:.*\.a"` because `cimgui-go`'s upstream `#cgo` directives use `-l:cimgui.a`.
- **Files Modified / Added:**
  - [go.mod](file:///C:/Users/samue/Projects/project-engine/go.mod)
  - [go.sum](file:///C:/Users/samue/Projects/project-engine/go.sum)
  - [engine/types.go](file:///C:/Users/samue/Projects/project-engine/engine/types.go)
  - [engine/transform.go](file:///C:/Users/samue/Projects/project-engine/engine/transform.go)
  - [engine/renderable.go](file:///C:/Users/samue/Projects/project-engine/engine/renderable.go)
  - [engine/physics_handle.go](file:///C:/Users/samue/Projects/project-engine/engine/physics_handle.go)
  - [engine/engine.go](file:///C:/Users/samue/Projects/project-engine/engine/engine.go)
  - [engine/engine_test.go](file:///C:/Users/samue/Projects/project-engine/engine/engine_test.go)
  - [engine/gl_state.go](file:///C:/Users/samue/Projects/project-engine/engine/gl_state.go)
  - [engine/gl_state_test_helper.go](file:///C:/Users/samue/Projects/project-engine/engine/gl_state_test_helper.go)
  - [engine/gl_window.go](file:///C:/Users/samue/Projects/project-engine/engine/gl_window.go)
  - [engine/imgui_console.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_console.go)
  - [engine/imgui_overlay.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_overlay.go)
  - [engine/imgui_overlay_test.go](file:///C:/Users/samue/Projects/project-engine/engine/imgui_overlay_test.go)
  - [pkg/luabridge/bridge_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/bridge_test.go)
  - [pkg/luabridge/console_adapter.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_adapter.go)
  - [pkg/luabridge/console_adapter_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/console_adapter_test.go)
- **Verification Results:**
  - `CGO_LDFLAGS_ALLOW="-l:.*\.a" go test -count=1 -v ./...`: All tests passed (`engine`: 7 tests including `TestImGuiOverlayLiveGLFWOpenGL3`, `pkg/luabridge`: 5 tests, `pkg/tileimporter`: 4 tests).
  - `go vet ./...` & `go build ./...`: Passed with 0 issues.`
