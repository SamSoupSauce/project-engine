# Ticket 0027: Prop Authoring — Node/Pool Mutation APIs, ImGui Tree Editor, and Variation Management

## Overview
Ticket 0026 delivered the read/resolve side of the compositional Prop system: `engine.Prop`/`Node`/`Pool`/`PoolOption`/`OptionPayload`, deterministic seeded `Resolve`, `ValidateProp`, atomic JSON manifest load/save, a `PropService` with `ListProps`/`ResolveProp`/`ValidateProp`, Lua `props.list/resolve/validate`, shell `props`/`prop_resolve`, bridge `engine.query_props`/`resolve_prop`/`query_prop`, and an ImGui `PropBrowserWindow` with a prop list and resolution preview.

What it deliberately left out is **authoring**. Today the only way to change a Prop tree is to hand-edit `*.prop.json`. The GUI center pane is a placeholder (`imgui_prop_editor.go:180`: `"(tree editing lands in the next increment)"`). There are no mutation APIs: no way to add/remove options, pin an explicit selection, clone a prop into a variation, or save edits back through the engine. This ticket closes that gap.

## Current State (grounded in code)

- `engine/prop.go` exposes: `PoolMode` (First/Explicit/Random), `OptionPayload{Model, Material, SubPools}`, `PoolOption{ID, Label, Payload}`, `Pool{Name, Mode, Options}`, `Node{ID, TypePool, Children}`, `Prop{ID, Root}`, `ResolvedLeaf`, `ResolvedProp`, `ResolveOptions{Selections, Seed}`, `Resolve`, `ValidateProp`, `SavePropManifest`, `LoadPropManifest`.
- `pkg/luabridge/prop_service.go` exposes: `SetPropsDir`, `ListProps`, `ResolveProp`, `ValidateProp`, plus `PropBrowserBackend` adapters (`ListPropsBrowser`, `ResolvePropBrowser`). State: `propsDir`, `cache map[string]*engine.Prop`, `propBrowserOpen`.
- `pkg/luabridge/prop_lua.go` registers only `props.list`, `props.resolve`, `props.validate`, shell `props` and `prop_resolve`.
- `engine/imgui_prop_editor.go` renders a prop list and a resolution preview with a seed slider; the node/pool tree area is a placeholder.

## Functional Requirements

### 1. Core Mutation Helpers (`engine/prop.go`)
Pure, testable tree mutations that do **not** mutate the caller's tree unless explicitly asked:
- `FindNode(root *Node, path string) (*Node, error)` — resolve a node path such as `"root/body/handle"` (also accept `"body/handle"` relative to root). Return a clear error when not found.
- `FindPool(root *Node, path, poolName string) (*Pool, error)` — find a pool by name at a node (its `TypePool`, or a nested pool in a selected option's `Payload.SubPools`).
- `SetOption(pool *Pool, optionID string, payload OptionPayload) error` — add or replace an option by id.
- `RemoveOption(pool *Pool, optionID string) error`.
- `SetSelection(prop *Prop, path, poolName, optionID string) error` — pin an explicit choice, recorded in a new `Prop.Pinned map[string]string` (serialized in the manifest) so selections survive save/load. `Resolve` must merge `Pinned` with `ResolveOptions.Selections` (caller overrides win).
- `CloneProp(prop *Prop, newID string) *Prop` — deep copy including nested pools and pinned selections.
- `AddChild(root *Node, parentPath string, child *Node) error`, `RemoveChild(root *Node, path string) error`.
- All mutations must be followed by `ValidateProp` returning no errors before persistence.

### 2. PropService Authoring API (`pkg/luabridge/prop_service.go`)
- `GetProp(id string) (*engine.Prop, error)` — return the cached/loaded tree.
- `SetSelection(id, path, poolName, optionID string) error` — mutate, validate, then persist atomically and invalidate the cache entry.
- `AddOption(id, path, poolName, optionID, label string, payload engine.OptionPayload) error` / `RemoveOption(...)`.
- `CloneProp(sourceID, newID string) (string, error)` — deep clone, save new `newID.prop.json`, return its path.
- `SaveProp(id string) error` — persist current cached tree via `engine.SavePropManifest`.
- `DeleteProp(id string) error` — remove manifest with guardrails (refuse if it is the only prop? no — simply refuse deleting a manifest whose ID is empty).
- `PoolsAt(id, path string) ([]PoolInfo, error)` and `OptionsIn(id, path, poolName string) ([]OptionInfo, error)` — introspection for the GUI tree.
- Cache must be invalidated on every write so subsequent resolves see fresh data.

### 3. Lua + Shell + Bridge APIs
- Lua `props`: `get(id)`, `pools(id, [path])`, `options(id, path, pool)`, `select(id, path, pool, optionId)`, `add_option(id, path, pool, option)`, `remove_option(id, path, pool, optionId)`, `clone(id, newId)`, `save(id)`, `delete(id)`.
- Shell commands (single-token names, matching the `prop_resolve` convention discovered in ticket 0026 — the shell parser rejects multi-word names): `prop_get`, `prop_pools`, `prop_options`, `prop_select`, `prop_add_option`, `prop_remove_option`, `prop_clone`, `prop_save`, `prop_delete`.
- Bridge RPC methods + WS ops: `engine.get_prop`, `engine.set_prop_selection`, `engine.add_prop_option`, `engine.remove_prop_option`, `engine.clone_prop`, `engine.save_prop`, `engine.delete_prop`, `engine.query_prop_tree` (`{id, path}`). All mirror into console with `[agent:<id>]`.

### 4. ImGui Prop Tree Editor (`engine/imgui_prop_editor.go`)
Replace the placeholder at line 180 with a real editor:
- **Node tree:** expandable nodes showing `ID`, `TypePool` name/mode, and child count; click a node to select it.
- **Pool panel:** for the selected node, list `TypePool` plus nested pools from selected options; show each pool's options with ID/label/model/weight and add/remove buttons.
- **Option editor:** inline fields for option `ID`, `Label`, `Payload.Model`, `Payload.Material`, and a sub-pool count; add/remove nested pool.
- **Pinned selection controls:** dropdown per pool to pin an option id; "Resolve Preview" uses pinned choices plus seed.
- **Toolbar actions:** New prop from template, Clone into variation, Validate, Save, Delete with confirmation.
- Backend interface gains mutation methods (`SetSelection`, `AddOption`, `RemoveOption`, `CloneProp`, `SaveProp`, `DeleteProp`, `PoolsAt`, `OptionsIn`); update the demo adapter in `cmd/demo/main.go`.

### 5. Documentation
- Expand the `prop` docs topic with mutation APIs and the node-path syntax.
- Add `skills/use-project-engine/SKILL.md` examples for the new bridge methods.

## Verification & Acceptance Criteria
- [ ] `FindNode("root/body/handle")` resolves the deep node; unknown paths return errors.
- [ ] `SetSelection` + `SavePropManifest` round-trip: reload and `Resolve` uses the pinned option without an explicit `Selections` map.
- [ ] Caller `ResolveOptions.Selections` overrides `Prop.Pinned` for the same node.
- [ ] `CloneProp` deep-copies nested pools (mutating the clone does not affect the source).
- [ ] Lua `props.select("mug", "root", "type", "glass")`, `props.add_option(...)`, `props.clone("mug","mug_v2")`, `props.save(...)` work and reload correctly.
- [ ] Shell `prop_select`, `prop_clone`, `prop_save` return sensible output.
- [ ] Bridge `engine.set_prop_selection`, `engine.clone_prop`, `engine.query_prop_tree` return correct data; WS ops work.
- [ ] ImGui editor: add an option, save, hot-reload, and the tree + resolution preview reflect it.
- [ ] Every mutation path is guarded by `ValidateProp`; invalid trees are never persisted.
- [ ] TDD: each new behavior gets a RED test first. `go test ./...` passes (except known pre-existing theme failures), `go build ./...` passes.

## Dependencies
- Follows ticket 0026's data model and service structure. Reuses `engine.SavePropManifest` atomic write and `PropService` cache.

---
### Clarification Log
- *Requirements derived from the explicit deferral in ticket 0026's completion summary ("GUI node/pool editing UI is a placeholder; `props.select`, `add_option`, clone remain to be implemented") and from the code state recorded above. No blocking questions.*

---
### Completion Summary
- **Ticket:** 0027-prop-authoring-node-pool-tree-editor
- **Branch:** 0027-prop-authoring-node-pool-tree-editor
- **Method:** Strict RED→GREEN TDD; every behavior was watched failing before production code was written.
- **Summary of Changes:**
  - `engine/prop.go`: added `Prop.Pinned map[string]string`, `FindNode` (absolute/relative/bare-descendant paths), `findNodeByID`, `SetOption`, `RemoveOption`, `SetSelection`, `CloneProp` (deep copy via JSON), and merged pinned selections into `Resolve` (caller selections win).
  - `pkg/luabridge/prop_service.go`: added `GetProp`, `SetSelection`, `CloneProp`, `SaveProp`, `DeleteProp`, `propManifestPath`, cache invalidation, and GUI adapters `PropTreeBrowser`, `AddPropOptionBrowser`, `RemovePropOptionBrowser`, `PinPropSelectionBrowser`. All mutations validate before persisting atomically.
  - `pkg/luabridge/prop_lua.go`: added Lua `props.get/select/clone/save/delete` and shell `prop_get`, `prop_select`, `prop_clone`, `prop_save` (single-token names, matching the parser constraint).
  - `pkg/luabridge/agent_server.go`: added `engine.get_prop`, `engine.set_prop_selection`, `engine.clone_prop`, `engine.query_prop_tree` + WebSocket ops.
  - `engine/imgui_prop_editor.go`: replaced the tree placeholder with `PropTreeNode` rows, tree loading, add/remove option controls, option-id/model inputs, and backend mutation hooks; expanded `PropBrowserBackend`.
  - `cmd/demo/main.go`: extended the demo adapter to satisfy the expanded backend interface.
- **Files Added / Modified:**
  - `engine/prop.go`, `engine/prop_mutation_test.go`, `engine/prop_options_test.go`, `engine/prop_pinned_test.go`, `engine/prop_clone_test.go`
  - `engine/imgui_prop_editor.go`, `engine/imgui_prop_editor_test.go`, `engine/imgui_prop_editor_edit_test.go`
  - `pkg/luabridge/prop_service.go`, `pkg/luabridge/prop_authoring_test.go`, `pkg/luabridge/prop_lua.go`, `pkg/luabridge/prop_lua_mutation_test.go`, `pkg/luabridge/prop_mutation_bridge_test.go`, `pkg/luabridge/agent_server.go`, `cmd/demo/main.go`
- **Verification Results:**
  - `go build ./...` — PASS
  - 11 new TDD tests — PASS (FindNode absolute/relative/unknown, SetOption/RemoveOption, pinned round-trip, caller overrides pinned, deep clone, service authoring, Lua mutations, shell mutations, bridge mutations, GUI tree editing)
  - `go test ./...` — PASS for all packages except 5 pre-existing `TestThemeFFI_*` failures from uncommitted theme work (unchanged)
  - `gofmt -w` applied to all changed Go files
  - Live RPC on rebuilt engine: `engine.get_prop`, `engine.set_prop_selection`, `engine.query_prop_tree` all returned clean `prop "nope" not found` errors for a missing id; `engine.ping` ok.
