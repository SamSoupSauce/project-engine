# project-engine

A modular, lightweight 3D game engine and world generation toolkit written in Go with an OpenGL 3.3 Core renderer, Dear ImGui tooling overlay, Jolt Physics (C-FFI + LuaJIT FFI), decoupled Lua 5.1 & LuaJIT 2.1 scripting, 2.5D/3D TMX tile-map importing, and a dual-transport AI Agent Bridge server.

---

## Quick Start

### 1. Build the Engine Executable
A default build target builds `./cmd/demo` into `bin/project-engine(.exe)` with `CGO_LDFLAGS_ALLOW='-l:.*\.a'` configured:

```powershell
# Windows (PowerShell)
.\build.ps1

# Linux / macOS / Make
make
# or
./build.sh
```

### 2. Run the Kitchen Sink Demo
```powershell
.\bin\project-engine.exe
```
On first launch, an interactive **Quick Start Tutorial** modal appears with a persisted **Do not show again** checkbox (`config/tutorial_state.json`). You can reopen it anytime via the **Tutorial** button or by typing `tutorial` in the `[~]` Developer Console.

**Demo Controls:**
- `[L]` — Lock/unlock cursor for 3D camera mouse orbit
- `[F]` — Toggle fullscreen
- `[~]` — Toggle Developer Console (`help`, `docs`, `scripts.list`, `comp`, `prefabs`, `edit`, `tutorial`)
- `[Left Click]` — Ray-pick 3D objects in the viewport (binds Lua `selected`)
- `[Esc]` — Unlock cursor or quit

---

## Architecture & Packages

| Package | Responsibility |
| --- | --- |
| [`engine`](engine/) | Core ECS (`GameObject`, `Component`, `TransformComponent`), `BaseScene` fixed-tick simulation, OpenGL 3.3 Core `GLContext`/`GLApp`, glTF 2.0/GLB loader (`MeshComponent`, `MeshRenderable`), `OrthoBatcher2D`/`Renderable2D` 2D HUD, Dear ImGui overlay (`DebugConsoleWindow`, `ScriptEditorWindow`, `PrefabEditorWindow`, `TutorialWindow`), 3D ray picking, outline pass, frame capture, and workspace configuration/verification (`WorkspaceConfig`). |
| [`engine/components`](engine/components/) | Standard component adapters (`TransformComponent`, `MeshRendererComponent`, `RigidBodyComponent`, `ConstraintComponent`, `HazardComponent`) used by `pkg/props`. |
| [`engine/spatial`](engine/spatial/) | Jolt Physics C++ bridge & FFI (`PhysicsWorld`, `RigidBody`, `CharacterController`, `WorldCollider`, `InteractiveComponent`, `FractureComponent`, breakable joints, and object grabbing). |
| [`engine/hazards`](engine/hazards/) | Fracture debris & floor shard hazard system (`HazardSystem`, `FloorShardHazard`). |
| [`engine/ui`](engine/ui/) | Declarative JSON UI layout loader, hit-testing, and event routing. |
| [`pkg/luabridge`](pkg/luabridge/) | Lua 5.1 (`LuaBridge`) + LuaJIT 2.1 (`LuaJITBridge`), custom Lua `Component(...)` registry, `ScriptComponent` lifecycle hooks, `ShellRegistry` (`shell.register`, `scripts.*`), `DocsRegistry` (`docs` / `engine.query_docs`), `ScriptEditorService`, `PrefabService`, graceful `RecordScriptError` handling, HTTP JSON-RPC + WebSocket `AgentServer`, and Cloudflare `TunnelManager`. |
| [`pkg/props`](pkg/props/) | Declarative Lua prop manifest loader (`scripts/props/*.lua`), glTF validation, entity spawning, and acoustic/debris callbacks. |
| [`pkg/interaction`](pkg/interaction/) | Player raycast interaction controller (`quick-tap` / `hold` radial context menus). |
| [`pkg/tileimporter`](pkg/tileimporter/) | Tiled `.tmx` XML/CSV parser and `WorldMap3D` builder (floors, exterior/interior walls, corners, pillars, multi-layer elevations, stairs, and Jolt static collider export). |

See detailed guides in [`docs/`](docs/):
- [Engine Architecture Guide](docs/architecture.md)
- [Building, Packaging & Custom Roots](docs/building_and_distribution.md)
- [Lua & LuaJIT Scripting Guide](docs/lua_scripting_guide.md)
- [LuaJIT Zero-Allocation Scripting Standards](docs/scripting_standards.md)

---

## Project Root Structure, Verification & Custom `--root`

Project Engine expects a workspace root containing:
```text
<root>/
├── engine.json          # Optional workspace & window configuration
├── assets/              # Models (assets/models/props/*.gltf) and TMX maps (assets/maps/*.tmx)
├── scripts/             # Lua scripts (components/, props/, ui/)
├── prefabs/             # JSON GameObject prefabs (*.json)
└── config/              # Persisted engine state (e.g., tutorial_state.json)
```

On startup, `cmd/demo` **verifies the folder structure** and fails fast with a clear diagnostic message if required directories are missing.

### Using a Different Game / Script / Config / Data Root
Point the engine at any external project directory with `--root`, `--config`, or scaffold a new project root with `--init-dirs`:
```bash
# Run against the bundled standalone game-root example
./bin/project-engine --root examples/custom-game-root

# Load an explicit config file (infers root from config directory unless --root is also passed)
./bin/project-engine --config /path/to/my-game/engine.json

# Scaffold missing assets/, scripts/, prefabs/, config/ under a new root
./bin/project-engine --root /path/to/new-game --init-dirs
```

---

## Building the Engine & Creating a Distributable Game Build

Ready-to-run build and packaging examples live in [`examples/build/`](examples/build/):
- **Build Engine Executable:** `.\examples\build\build_engine.ps1` / `./examples/build/build_engine.sh` (or root `make` / `.\build.ps1`)
- **Create Standalone Game Build:** `.\examples\build\create_game_build.ps1` / `./examples/build/create_game_build.sh` (or `make package` / `.\build.ps1 -Package`)

---

## Lua Examples

Opt-in Lua examples live in [`examples/lua/`](examples/lua/) and can be run live inside the Developer Console (`scripts.run examples/lua/<file>.lua`):
- `01_hello_shell_commands.lua` — Dynamic `shell.register` commands & `unload` cleanup
- `02_custom_component.lua` — Custom ECS `Component(...)` with Script Editor annotations
- `03_prefabs_and_events.lua` — Prefabs, picking, and event listeners
- `04_jolt_physics_and_raycast.lua` — Physics/fracture inspection & LuaJIT FFI raycasting
- `05_zerocopy_hud_luajit.lua` — Zero-allocation LuaJIT FFI HUD rendering

---

## Running Tests

All Go unit & integration tests run headlessly without requiring an active display:

```powershell
$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"
go test -count=1 ./...
```
Or via `make test` / `.\build.ps1 -Test`.

---

## Agent Bridge Server & Cloudflare Quick Tunnel

External AI agents and tools can inspect, query live runtime documentation (`engine.query_docs`), edit scripts, spawn prefabs, and capture frames over HTTP JSON-RPC (`POST /rpc`) and WebSocket (`GET /ws`):

```bash
# Local listener on 127.0.0.1:4242 (--token is required, even if empty "")
./bin/project-engine --serve --address=127.0.0.1:4242 --token=""

# Automated Cloudflare Quick Tunnel (downloads cloudflared if missing and prints URL in Console & ImGui overlay)
./bin/project-engine --serve --tunnel --address=127.0.0.1:4242 --token=""
```

See [skills/use-project-engine/SKILL.md](skills/use-project-engine/SKILL.md) for the mandatory discovery protocol, capability audit (`engine.query_docs`), token headers, and JSON-RPC / WebSocket examples.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
