# Project Engine Architecture Guide

This document summarizes the runtime architecture, packages, and data flows of `project-engine`.

---

## 1. Core ECS & Scene Graph (`engine/`)

- **`GameObject` (`engine/game_object.go`)**: Hierarchical scene node with `ID`, `Name`, `Active`, `Parent`/`Children`, component registry, and event/hook dispatch (`Emit`, `EmitTo`, `DispatchLoad`, `DispatchSpawn`, `DispatchTick`, `DispatchDespawn`).
- **`Component` (`engine/component.go`)**: Lifecycle interface (`Init(*GameObject)`, `Update(dt float32)`, `Destroy()`).
- **`TransformComponent` (`engine/transform_component.go`)**: Local and compounded hierarchical world transforms (`Position`, `Rotation`, `Scale`).
- **`MeshComponent` & `GLTFAsset` (`engine/mesh_component.go`, `engine/gltf.go`)**: Parses `.gltf` (JSON + data URI / external `.bin`) and `.glb` (binary glTF 2.0) assets, extracts PBR material `BaseColorFactor`, vertex colors, bounding boxes, and Jolt convex hull points.
- **`MeshRenderable` (`engine/mesh_renderable.go`)**: OpenGL 3.3 Core mesh renderer enforcing `TransformComponent` + loaded `MeshComponent` constraints, supporting wireframe toggling and `BaseScene.DrawMeshRenderables(viewProj)`.
- **`Renderable2D` & `OrthoBatcher2D` (`engine/renderable2d.go`, `engine/ortho_batcher2d.go`)**: Anchor-aware (`AnchorTopLeft`, `AnchorCenter`, `AnchorBottomLeft`, etc.) orthographic 2D UI batcher rendered over the 3D scene.
- **`GLContext` & `GLApp` (`engine/gl_context.go`, `engine/gl_window.go`)**: GLFW window + OpenGL 3.3 Core context component with MSAA, VSync, fullscreen toggling (`[F]`), cursor lock (`[L]`), and frame timing.
- **`ImGuiOverlay` (`engine/imgui_overlay.go`)**: Dear ImGui integration with GL state backup/restore, `DebugConsoleWindow` (`engine/imgui_console.go`), `ScriptEditorWindow` (`engine/imgui_script_editor.go`), `PrefabEditorWindow` (`engine/imgui_prefab_editor.go`), and first-run `TutorialWindow` (`engine/imgui_tutorial.go`).
- **`WorkspaceConfig` (`engine/workspace_config.go`)**: Resolves `--root`, `--config` (`engine.json`), verifies required directories (`assets/`, `scripts/`, `prefabs/`, `config/`), and scaffolds new roots via `--init-dirs`.

---

## 2. Physics, Interaction & Hazards (`engine/spatial/`, `engine/hazards/`, `pkg/interaction/`)

- **`PhysicsWorld` (`engine/spatial/physics_world.go`)**: Jolt Physics C-API wrapper with deterministic fixed-step accumulation (`DefaultFixedTimestep = 1/60s`) and collision layers (`LayerNonMoving`, `LayerMoving`, `LayerInteractable`, `LayerProp`).
- **Spatial Components (`engine/spatial/components.go`)**: `RigidBody`, `CharacterController`, `InteractiveComponent` (display name, default quick-tap verb, hold context verbs), and `FractureComponent` (structural health, impact threshold, fracture dispatch).
- **`WorldCollider` (`engine/spatial/world_collider.go`)**: Compiles `MapColliderSource` (`tileimporter.WorldMap3D.ExportStaticColliderBounds()`) into a single Jolt `StaticCompoundShape` static body.
- **`HazardSystem` (`engine/hazards/shard.go`)**: Spawns `FloorShardHazard` areas on ceramic/glass fracture events with bleed probability and acoustic crunch radius.
- **`InteractionController` (`pkg/interaction/controller.go`)**: Raycasts from camera/reticle, manages highlight states, and triggers quick-tap or hold radial menus (`pkg/interaction/imgui_menu.go`).

---

## 3. Tile Map Importer (`pkg/tileimporter/`)

- **`ParseTMX` (`pkg/tileimporter/tmx.go`)**: Parses Tiled `.tmx` XML maps, CSV tile layers, tileset properties, flip flags, and object groups.
- **`WorldMap3D` (`pkg/tileimporter/world_map3d.go`)**: Converts 2D layers into 3D tiles with:
  - Floor slabs (`type="floor"`, `type="grass"`)
  - 0.2m-thick directional edge walls (`walls="north,west"`, `wall_type="exterior|interior"`) and top/bottom corner collision seams
  - Pillars & barricades (`type="pillar"`, `type="barricade"`)
  - Multi-step staircases (`type="stairs"`, `stair_dir`, `step_count`, `stair_height`)
  - Multi-layer elevations (`elevation` layer property)
  - `ExportStaticColliderBounds()` for Jolt static world collision and `BuildTileMapGLTFAsset()` in `cmd/demo/kitchen_sink.go` for OpenGL 3.3 rendering.

---

## 4. Lua Scripting, Prefabs & Agent Bridge (`pkg/luabridge/`, `pkg/props/`)

- **`LuaBridge` (`pkg/luabridge/bridge.go`)**: Thread-safe (`reentrantMutex`) Lua 5.1 state exposing `GameObject`, `Scene`, `Transform`, `Mesh`, `MeshRenderable`, `Renderable2D`, `RigidBody`, `CharacterController`, `InteractiveComponent`, `FractureComponent`, `pick()`, `list_objects()`, `workspace_paths()`, `docs()`, and `help()`.
- **`LuaJITBridge` (`pkg/luabridge/luajit.go`)**: Embedded LuaJIT 2.1 runtime with C-FFI exports (`igImDrawList_*`, `Jolt_CastRay`) for zero-allocation HUD and raycast paths.
- **`ShellRegistry` (`pkg/luabridge/shell.go`)**: Unified `ExecuteWithContext` command pipeline shared by the ImGui Console, HTTP `/rpc`, and WebSocket `/ws`, supporting `shell.register`, `scripts.list`, `scripts.run`, `scripts.reload`, `scripts.unload`, and `tutorial`.
- **`DocsRegistry` (`pkg/luabridge/docs_registry.go`)**: Runtime introspection powering `docs [topic]` and `engine.query_docs`.
- **`ScriptEditorService` & `PrefabService` (`pkg/luabridge/script_editor.go`, `pkg/luabridge/prefab.go`)**: Tri-pane Lua component authoring with SHA-256 live hot-reload and JSON prefab serialization/spawning.
- **`AgentServer` & `TunnelManager` (`pkg/luabridge/agent_server.go`, `pkg/luabridge/tunnel.go`)**: Dual-transport JSON-RPC (`/rpc`) and WebSocket (`/ws`) server with mandatory token authentication, main-thread tick execution, frame capture, and optional `cloudflared` quick-tunnel integration.
