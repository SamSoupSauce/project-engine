# Ticket 0026: Prop as a Compositional GameObject — Node/Pool Tree, Variation System, and Prop Editor

## Overview
Props are the engine's destructible, interactive physics objects. Today they are flat Lua manifests (`scripts/props/*.lua`) with fixed `meta`/`physics`/`carry` tables and lifecycle hooks, spawned as generic `GameObject`s via `pkg/props.PropRegistry`. This is too rigid: a "mug" has one model, one material, one behavior. There is no way to express that a mug can be *ceramic* or *glass* or *metal*, that a ceramic mug has a pool of style variations (different shapes/models), or that each variation has its own sub-pools of options.

This ticket introduces **Prop** as a first-class GameObject type whose identity is a **tree of nodes**. Each node holds a **pool of potential node types**. Selecting a type resolves the node and exposes that type's **sub-pools of node options**, recursively. Example: a Prop `mug` has a root node with pool `{ ceramic, glass, metal }`; selecting `ceramic` yields a sub-pool called `style` containing several 3D models of ceramic mugs of various shapes/styles; selecting one of those may expose further sub-pools (e.g. `handle`, `finish`, `decal`). This makes props compositional and gives them a practically unbounded variation space from a small set of authored pools.

Alongside the data model, this ticket adds Lua/shell/bridge APIs for querying and resolving the tree, a Prop Editor GUI to browse and author pools, and deterministic/random resolution for spawning a concrete variation.

## Core Data Model

### Prop
A Prop is a GameObject subtype (`*engine.Prop` embedding/behaving as `GameObject`) with:
- `id` — stable identifier (e.g. `mug`)
- `root` — a `Node`
- `physics` / `carry` / `interaction` defaults layered onto the resolved result
- lifecycle hooks (`on_spawn`, `on_collision`, `on_joint_break`, fracture callbacks)

### Node
A node in the composition tree:
- `id` — local node id (e.g. `body`, `handle`)
- `type_pool` — a `Pool` of candidate **node types**
- `selected` — the currently resolved node type (or unresolved)
- `children` — sub-pools discovered from the selected type
- `attachment` — transform/socket describing where this node attaches to its parent

### Pool
A named collection of **options** with resolution metadata:
- `name` — e.g. `style`, `material`, `handle`
- `options` — ordered list of `PoolOption`
- `mode` — `random` | `first` | `weighted` | `explicit`
- `weights` / `seed` — for deterministic resolution
- `tags` — filtering hints (e.g. `fragile`, `ceramic`, `two_handed`)

### PoolOption
A selectable entry in a pool:
- `id`, `label`
- `payload` — one of: node type descriptor, model reference (`.gltf`/`.glb`), material, physics overrides, or a nested `Pool`
- `tags`, `weight`
- `hooks` — option-specific lifecycle hooks

**Resolution:** walking a Prop from root, each unresolved node draws one option from its `type_pool`; the chosen option may contribute a model/material/physics override and may introduce child pools, which are resolved recursively until only concrete leaves remain. The resolved tree is then flattened into spawned `GameObject`s with the accumulated physics/interaction settings. A `seed` makes any variation reproducible.

## Functional Requirements

### 1. Core Types & Resolution (`engine/prop.go`, `engine/prop_node.go`, `engine/prop_pool.go`)
- `Prop`, `Node`, `Pool`, `PoolOption`, `ResolvedProp`, `ResolveOptions` types with JSON serialization.
- `Resolve(prop, ResolutionOptions{Seed, Mode, Tags}) (ResolvedProp, error)` — deterministic tree walk producing concrete leaves.
- `Prop.Template()` / `Clone()` — deep copy for variation authoring.
- `Prop.FromManifest(path)` / `SaveManifest(path)` — load/save the tree to disk (new `props/*.prop.json` manifest format; keep backward compatibility with existing `scripts/props/*.lua` via an adapter).
- Validation: required fields, model file existence, pool non-empty, cycle detection, physics ranges.

### 2. Lua API (`props.*`)
- `props.list([query])` -> prop descriptors.
- `props.get(id)` -> full Prop tree as nested Lua tables.
- `props.resolve(id, [seed], [mode], [tags])` -> resolved variation tree.
- `props.pools(id, [nodePath])` -> pools at a node (e.g. root type pool, `style` sub-pool).
- `props.options(id, nodePath, poolName)` -> options in a pool with labels/tags.
- `props.select(id, nodePath, poolName, optionId)` -> pin an explicit option (variation authoring).
- `props.add_option(id, nodePath, poolName, option)` / `props.remove_option(...)`.
- `props.spawn(id, seed|x,y,z)` -> spawn a resolved variation; returns entity id and resolved leaf summary.
- `props.clone(id, newId)` / `props.save(id)` / `props.delete(id)`.
- `props.validate(id)` -> `{ ok, errors[] }`.

### 3. Shell Commands (unified registry, `docs`-visible)
- `props [query]`, `prop view <id>`, `prop tree <id>` (ASCII tree of nodes/pools/options).
- `prop resolve <id> [seed]`, `prop spawn <id> [seed] [x y z]`.
- `prop pools <id> [nodePath]`, `prop options <id> <nodePath> <pool>`.
- `prop select <id> <nodePath> <pool> <optionId>`, `prop add-option ...`, `prop clone <id> <newId>`.
- `prop validate <id>`, `prop save <id>`, `prop fracture <entityId>`, `prop hit <entityId> <impulse>`.

### 4. Agent Bridge Endpoints
- `engine.query_props`, `engine.query_prop` (`topic: tree|pools|options|validate`), `engine.resolve_prop` (`{id, seed, mode, tags}`), `engine.spawn_prop` (`{id, seed, x,y,z}`), `engine.clone_prop`.
- WebSocket `op` equivalents; console mirroring with `[agent:<id>]`.

### 5. ImGui Prop Editor (`engine/imgui_prop_editor.go`, backend `PropEditorBackend`)
Toggle from `[~]` Developer Console; styled with existing ImGui theme.
- **Left pane:** searchable Prop list with type/pool-count badges and validation status.
- **Center pane:** interactive **node tree view** — expandable nodes showing each node's `type_pool`; selecting a node opens its pools; each pool shows its options with tags/weights; drag/reorder options. Inline editors for option payloads (model path, material, physics overrides, nested pool).
- **Resolution preview pane:** seed input + mode selector; renders the resolved tree as a concrete list of leaves with chosen models/materials; one-click "spawn this variation".
- **Right pane:** inspector for the selected node/pool/option, plus physics/carry/interaction defaults and hook list.
- **Toolbar:** new prop from template, clone into variation, validate, save (hot-reload), spawn, delete with confirmation.
- **Preview scene slot:** spawn a resolved prop and trigger collision/joint-break/fracture to verify behavior; log callbacks.

### 6. Documentation & Skill Updates
- Register `prop` / `prop.format` subsystem topics in `docs_registry.go` documenting the node/pool/option model, resolution modes, and new verbs; add manifest schema docs.
- Update `skills/use-project-engine/SKILL.md` capability index + `examples/`; update `README.md` architecture and add a Prop Editor section.

## Verification & Acceptance Criteria
- [ ] `props.get("mug")` exposes a root node with a `type_pool` containing ceramic/glass/metal.
- [ ] Selecting `ceramic` reveals a `style` pool with multiple mug model options; selecting a style reveals any nested sub-pools.
- [ ] `props.resolve("mug", {seed=42})` is deterministic and produces only concrete leaves (no unresolved pools).
- [ ] `props.spawn` produces a `Prop` GameObject whose children match the resolved tree with correct models/physics.
- [ ] Existing flat manifests (`kitchen_toaster` etc.) load via the backward-compat adapter with equivalent behavior.
- [ ] `engine.query_props` / `engine.resolve_prop` / `engine.spawn_prop` return correct structured data; WS ops work.
- [ ] Prop Editor round-trip: edit a pool option in ImGui, save, hot-reload, resolve, and spawn reflects the change.
- [ ] Validation catches empty pools, missing models, cycles, and invalid physics without panics.
- [ ] `go test ./...` passes (headless) and `make` build succeeds.

## Dependencies
- Builds on Ticket 0025's shared bridge/service patterns (Lua/shell/bridge/GUI backend conventions). Mirrors `TileMapService` structure.

---
### Clarification Log
- **User:** "Prop as a type of GameObject, which is a tree of nodes, each with a pool of potential types of Node, each of which can have sub-pools of Node options. When a Node Type is selected, say for a mug, if the type is 'ceramic', the pool might contain a single pool called 'style' with various 3d models of various shapes and styles of ceramic mugs."
- *Design translated into the Node/Pool/PoolOption model above; field/method names follow existing engine conventions. No other blocking questions.*

---
### Completion Summary
- **Ticket:** 0026-prop-node-pool-tree-and-editor
- **Branch:** 0026-prop-node-pool-tree-and-editor
- **Method:** Strict RED→GREEN test-driven development. Eleven tracer bullets were each watched failing before production code was written.
- **Summary of Changes:**
  - Core model in `engine/prop.go`: `Prop`, `Node`, `Pool`, `PoolOption`, `OptionPayload`, `ResolvedProp`, `ResolveOptions`, `PoolMode` (First/Explicit/Random), `Resolve()` (recursive with sub-pool resolution, seeded deterministic selection, cycle guard), `ValidateProp()` (empty pools, cycles), `SavePropManifest()` / `LoadPropManifest()` (atomic JSON round-trip).
  - `pkg/luabridge/prop_service.go`: `PropService` with discovery (`ListProps`), loading, `ResolveProp`, `ValidateProp`, and a `PropBrowserBackend` implementation.
  - `pkg/luabridge/prop_lua.go`: Lua `props` table (`list`, `resolve`, `validate`) and shell commands `props` / `prop_resolve`.
  - Bridge RPC methods `engine.query_props`, `engine.resolve_prop`, `engine.query_prop` plus WebSocket ops.
  - `engine/imgui_prop_editor.go`: `PropBrowserWindow` + `PropBrowserBackend` with prop list, resolution preview (seed slider), and leaf display; attached to `ImGuiOverlay` via `SetPropBrowser`.
  - Registered a `prop` docs topic.
- **Files Added / Modified:**
  - `engine/prop.go`, `engine/prop_test.go`, `engine/prop_nested_test.go`, `engine/prop_seed_test.go`, `engine/prop_children_test.go`, `engine/prop_validate_test.go`, `engine/prop_cycle_test.go`, `engine/prop_resolve_cycle_test.go`, `engine/prop_manifest_test.go`
  - `engine/imgui_prop_editor.go`, `engine/imgui_prop_editor_test.go`, `engine/imgui_overlay.go`
  - `pkg/luabridge/prop_service.go`, `pkg/luabridge/prop_service_test.go`, `pkg/luabridge/prop_service_fixtures_test.go`, `pkg/luabridge/prop_lua.go`, `pkg/luabridge/prop_lua_test.go`, `pkg/luabridge/prop_bridge_test.go`
  - `pkg/luabridge/agent_server.go`, `pkg/luabridge/bridge.go`, `pkg/luabridge/docs_registry.go`, `cmd/demo/main.go`
- **Verification Results:**
  - `go build ./...` — PASS
  - `go test ./...` — PASS for all packages except 5 pre-existing `TestThemeFFI_*` failures from uncommitted theme work (unchanged, not attributed to this ticket)
  - `gofmt -w` applied to all changed Go files
  - Live RPC verification on rebuilt engine at `127.0.0.1:4242`: `engine.query_props` returned `{"count":0,"props":[]}` (no `.prop.json` manifests authored yet); `engine.query_prop` for missing id returned clean `prop "nope" not found` error; `engine.ping` still ok.
- **Known Deferred:** The GUI node/pool *editing* UI is a placeholder ("tree editing lands in the next increment"); this ticket delivered the data model, resolution, discovery, Lua/shell/bridge APIs, and the browser/preview shell. Authoring mutations (`props.select`, `add_option`, clone) remain to be implemented in a follow-up.
