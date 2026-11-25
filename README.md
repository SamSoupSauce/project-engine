# project-engine

A modular, lightweight 3D game engine and world generation toolkit written in Go using [raylib-go](https://github.com/gen2brain/raylib-go) and [gopher-lua](https://github.com/yuin/gopher-lua).

Extracted from game-specific logic to provide a clean, standalone foundation for 3D games with spatial hierarchies, pipeline-driven rendering, decoupled Lua scripting, and 2.5D/3D tile map import capabilities.

---

## Architecture & Packages

### 1. `engine` (`github.com/SamSoupSauce/project-engine/engine`)
Core engine primitives with zero dependencies on gameplay systems:
- **`GameObject` & `Transform`**: Hierarchical scene graph with parent-child relationships and recursive spatial transformation compounding.
- **`Component`**: Modular component lifecycle (`Init`, `Update`, `Destroy`).
- **`Renderable`**: Pipeline-driven renderable component featuring cascade evaluation (overriding or inheriting parent render flags such as visibility, shadow casting, wireframe), model/mesh rendering, and flat draw-command collection.
- **`PhysicsHandle` & `PhysicsWorld`**: Abstract interfaces and handles for physics bodies and collisions.
- **`Engine`**: Main game loop managing window initialization, delta timing, scene transitions, and graceful shutdown.

### 2. `pkg/luabridge` (`github.com/SamSoupSauce/project-engine/pkg/luabridge`)
Extensible Lua 5.1 runtime integration:
- **`LuaBridge`**: Thread-safe bridge separating execution lock (`stateMu`) from data lookups (`dataMu`).
- **Command & Hook System**: Register custom engine/gameplay commands and input hooks cleanly without tight coupling.
- **Game Object Exposer**: Reflect `GameObject` positions, rotations, and hierarchy into the Lua state for scripting.
- **`Console`**: Interactive REPL developer console with history navigation, line buffer management, and log routing.

### 3. `pkg/tileimporter` (`github.com/SamSoupSauce/project-engine/pkg/tileimporter`)
Robust 2D TMX to 3D world voxel/mesh importer:
- **TMX Parsing**: Fast XML parser supporting CSV tile data and Tiled flip/rotation flags.
- **Coordinate Transformation**: Seamless mapping between grid space `(col, row, layer)` and 3D world space `(X, Y, Z)`.
- **Vertical Stitching & Stacking**: Support for multi-layer maps, staircase generation, and vertical contiguous extensions.
- **Top & Bottom Corner Collision**: Complete collision mesh and bounding box generation for corner floor tiles, stairs, walls, and pillars.

---

## Getting Started

### Installation

```bash
go get github.com/SamSoupSauce/project-engine
```

Or reference locally in your `go.mod`:
```go
module mygame

go 1.22

require github.com/SamSoupSauce/project-engine v0.0.0

replace github.com/SamSoupSauce/project-engine => ../project-engine
```

### Running Tests

All unit tests run headlessly without requiring an active graphical context:

```bash
go test -v ./...
```

---

## License

MIT License. See [LICENSE](LICENSE) for details.
