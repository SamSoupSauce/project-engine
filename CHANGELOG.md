# Changelog

All notable changes to **project-engine** are documented in this file.

## [v0.1.0] - 2026-11-27

Initial public release of **project-engine**.

### Highlights
- **Core ECS & OpenGL 3.3 Core Renderer (`engine/`):**
  - Hierarchical `GameObject` / `TransformComponent` scene graph, `BaseScene` fixed-tick simulation, `GLContext`/`GLApp` window management, glTF 2.0/GLB loader (`MeshComponent`, `MeshRenderable`), and anchor-aware 2D HUD (`OrthoBatcher2D`, `Renderable2D`).
  - Dear ImGui tooling overlay (`ImGuiOverlay`): Quake-style Developer Console, tri-pane Lua Script Editor with SHA-256 live hot-reload, JSON Prefab Manager, 3D ray picking, outline pass, frame capture, and persisted first-run **Quick Start Tutorial** (`config/tutorial_state.json`).
  - Configurable workspace root (`--root`, `--config`, `engine.json`, `--init-dirs`) and startup folder structure verification (`assets/`, `scripts/`, `prefabs/`, `config/`).
- **Physics, Interaction & Hazards (`engine/spatial/`, `engine/hazards/`, `pkg/interaction/`, `pkg/props/`):**
  - Jolt Physics C-API integration (`PhysicsWorld`, `RigidBody`, `CharacterController`, `WorldCollider`, `InteractiveComponent`, `FractureComponent`, breakable constraints, and object grabbing).
  - Floor shard hazard system (`HazardSystem`, `FloorShardHazard`) and declarative Lua prop manifest loader (`scripts/props/*.lua`).
- **TMX 3D Tile Importer (`pkg/tileimporter/`):**
  - Tiled `.tmx` parser and `WorldMap3D` generator supporting floors, exterior/interior edge walls, corners, pillars, multi-layer elevations, staircases, and Jolt static compound collider export.
- **Lua 5.1 & LuaJIT 2.1 Scripting (`pkg/luabridge/`):**
  - `LuaBridge` (custom `Component(...)` classes, lifecycle hooks, `shell.register`, `scripts.*`, graceful non-fatal script error reporting) and `LuaJITBridge` (zero-allocation C-FFI UI & Jolt raycast bindings).
  - Runnable Lua & LuaJIT examples in `examples/lua/` and standalone build/packaging scripts in `Makefile`, `build.ps1`, `build.sh`, and `examples/build/`.
- **AI Agent Bridge & Skill (`pkg/luabridge/`, `skills/use-project-engine/`):**
  - Dual-transport HTTP JSON-RPC (`/rpc`) and WebSocket (`/ws`) `AgentServer` with mandatory token auth, dynamic runtime docs introspection (`docs` / `engine.query_docs`), Cloudflare Quick Tunnel (`--tunnel`) support, and the `use-project-engine` skill specification.
