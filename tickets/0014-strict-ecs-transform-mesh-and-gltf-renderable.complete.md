# Ticket 0014: Pure ECS `GameObject`, `MeshComponent` (.gltf/.glb), and Strict `MeshRenderable` Pipeline

## Status
Complete

## Context
Previously `engine.GameObject` retained an inline `Transform` struct field and Lua `.position`/`.transform` shortcuts even after introducing `TransformComponent` in Ticket 0013, and `cmd/demo` rendered the Rainbow Cube with hardcoded shader vertices in `main.go` rather than through a component-driven `Renderable` + `Mesh` + `Transform` pipeline.

This ticket completes the strict ECS refactoring:
1. Remove default `Transform` / `position` / `transform` fields from `GameObject` (Go struct and Lua userdata), starting `NewGameObject` with zero default components.
2. Require `Renderable` components to drive 3D rendering (no ad-hoc hardcoded scene draw paths).
3. Enforce that `Renderable` components require an attached `TransformComponent` on their `GameObject`.
4. Implement `MeshRenderable`, the concrete OpenGL `Renderable` component for entities with a `MeshComponent`, plus unit/demo verification.
5. Add a `MeshComponent` (`Mesh`) referencing `.gltf` / `.glb` assets and convert the `cmd/demo` Rainbow Cube to load `assets/models/props/rainbow_cube.gltf` via `MeshComponent` + `MeshRenderable`.

## Scope of Work

### 1. Pure ECS `GameObject` (`engine`, `engine/spatial`, `engine/hazards`, `pkg/*`)
- Remove `Transform` field from `engine.GameObject` and stop auto-attaching `TransformComponent` in `NewGameObject` (`Components` begins empty).
- Compute `GetWorldTransform()` solely from attached `*TransformComponent` nodes along the parent chain (defaulting to identity if omitted).
- Remove `.position`, `.rotation`, `.scale`, `.transform`, `GetPosition`/`SetPosition`, etc. from `GameObject` Lua userdata and `SerializeGameObjectJSON` (moving spatial queries/mutations cleanly onto `TransformComponent`).
- Update all engine/physics/interaction/prop systems and tests to attach and read/write `*TransformComponent` explicitly.

### 2. `.gltf`/`.glb` Vertex Color Support & `MeshComponent` (`engine`)
- Extend `engine/gltf.go` to parse `COLOR_0` (`VEC3` / `VEC4` float32) vertex color attributes alongside `POSITION`, `NORMAL`, `TEXCOORD_0`, and `INDICES`.
- Add `MeshComponent` (`engine/mesh_component.go`) implementing `engine.Component`, referencing `.gltf`/`.glb` paths via `LoadGLTFAsset`, exposing `FilePath`, `Asset`, `TriangleCount()`, and `Bounds()`.
- Expose `Mesh` (`MeshComponent`) to Lua (`AddComponent("Mesh", path)`, `GetComponent("Mesh")`, `help("Mesh")`, `Load`, `GetPath`, `GetTriangleCount`, `GetBounds`).

### 3. Require `Renderable` + `Transform` + `Mesh` Constraints & `MeshRenderable` (`engine`, `cmd/demo`)
- Enforce `Renderable.ValidateConstraint()` requiring `TransformComponent` across extraction, script render dispatch, and Lua attachment.
- Implement `MeshRenderable` (`engine/mesh_renderable.go`) backed by `MeshGLAPI` / `RealMeshGLAPI` (OpenGL 3.3 Core VAO/VBO/EBO + shader):
  - Validates BOTH an attached `TransformComponent` (`ErrMissingTransformComponent`) AND an attached `MeshComponent` with valid `Asset` (`ErrMissingMeshComponent`).
  - Uploads `MeshComponent` vertex positions/colors/normals/indices and renders using the world matrix evaluated from `TransformComponent`.
  - Expose `MeshRenderable` (`Renderable` / `MeshRenderable`) via Lua `AddComponent`, `GetComponent`, `help(...)`, and `SerializeGameObjectJSON`.
- Create `assets/models/props/rainbow_cube.gltf` and update `cmd/demo/main.go` to render the Rainbow Cube exclusively through `TransformComponent` + `MeshComponent` + `MeshRenderable`.

## Acceptance Criteria
- [x] `GameObject` has no default `Transform` field in Go or default `.transform`/`.position` fields in Lua; `NewGameObject` starts with zero components.
- [x] Rendering 3D scene objects requires a `Renderable` / `MeshRenderable` component.
- [x] `Renderable` and `MeshRenderable` enforce `TransformComponent` (and `MeshComponent` for `MeshRenderable`) constraints.
- [x] `MeshComponent` loads `.gltf`/`.glb` assets (including `COLOR_0`) and `MeshRenderable` draws them using `TransformComponent`.
- [x] `cmd/demo` loads `assets/models/props/rainbow_cube.gltf` via `Transform` + `Mesh` + `MeshRenderable` and passes all package tests, `go vet`, and `go build`.

---
### Clarification Log
- **Agent:** When removing default transform/position fields from `GameObject`, should `NewGameObject()` start with zero components or still auto-attach a `TransformComponent`?
- **User:** (Recommended) Remove `GameObject.Transform` struct field & Lua `.position`/`.transform` properties; `NewGameObject` starts with 0 components so `Transform` must be added explicitly.
- **Agent:** How should we wire the first `Renderable` for `Mesh` components (Requirement 4)?
- **User:** (Recommended) Convert `cmd/demo`'s Rainbow Cube into a `.gltf` asset + `Mesh` component + OpenGL `MeshRenderable`/`Renderable` so the demo renders via the new pipeline.

---
### Completion Summary
- **Strict ECS `GameObject`**: Removed the default `Transform` struct field from `engine.GameObject`, initialized `NewGameObject` with zero components (`Components: []Component{}`), added `EnsureTransform()` on `*GameObject`, and removed default `.position`/`.rotation`/`.scale`/`.transform` properties, `GetPosition`/`SetPosition`/etc. methods, and top-level JSON `transform` output from Lua/REPL serialization. Updated all callers across `engine`, `engine/components`, `engine/spatial`, `engine/hazards`, `pkg/interaction`, `pkg/props`, and `pkg/luabridge` to attach and mutate `*TransformComponent` explicitly.
- **`.gltf`/`.glb` Color Support & `MeshComponent`**: Added `COLOR_0` (`VEC3`/`VEC4` float32) parsing in `engine/gltf.go` (`GLTFPrimitive.Colors`) and created `engine.MeshComponent` (`engine/mesh_component.go`) referencing `.gltf`/`.glb` files via `LoadGLTFAsset`, with Lua bindings (`Mesh`), `help("Mesh")` introspection, and `picking.go` world bounds support.
- **`MeshRenderable` & Demo Pipeline**: Created `engine.MeshRenderable` (`engine/mesh_renderable.go`) backed by `MeshGLAPI`/`RealMeshGLAPI` requiring both `TransformComponent` (`ErrMissingTransformComponent`) and a loaded `MeshComponent` (`ErrMissingMeshComponent` / `ErrMeshNotLoaded`), wired `BaseScene.DrawMeshRenderables`, registered `MeshRenderable` in Lua + `help`, generated `assets/models/props/rainbow_cube.gltf`, and updated `cmd/demo/main.go` to render the Rainbow Cube solely through `TransformComponent` + `MeshComponent` + `MeshRenderable`.
- **Verification**: Passed `go test -count=1`, `go vet`, and `go build` across `./engine`, `./engine/ui`, `./engine/spatial`, `./engine/hazards`, `./engine/components`, `./pkg/interaction`, `./pkg/props`, `./pkg/luabridge`, `./pkg/tileimporter`, and `./cmd/demo`.
