# Ticket 0008: Interactive Component, Reticle Highlighting & Context Verb System

## Status
Complete

## Context
Following the implementation of JoltC FFI raycasting (Ticket 0007), first-person reticle traces now provide immediate access to target entities under the crosshair. This ticket implements the visual feedback (outline shader / stencil pass) and the multi-tier interaction state machine (tap vs. hold, default verbs, physics grab/throw mechanics, and contextual popup menus) powered by a generic `Interactive` component.

## Interaction Specification

1. **Visual Highlight (Glowing Outline):**
   - Reticle raycast hit on an entity with an `Interactive` component sets a hover target.
   - Target entity mesh is tagged for outline rendering during the forward pass (e.g., stencil bit `0x02` or `FlagOutline` draw flag).
   - A single-pass outline/glow kernel highlights the object border with an emissive tint (soft amber/cyan). Cleared immediately when raycast leaves the bounding volume.

2. **Short Click / Quick Tap (< 250ms): Primary Verb Execution:**
   - Evaluates the first valid entry in the `Interactive` component's verb array:
     - **Containers (Cabinets, Drawers, Fridges):** Open container inventory UI.
     - **Doors / Windows / Gates:** Toggle Open / Closed state.
     - **Switches / Lamps:** Toggle power bit / light node.
     - **Free Props (Unheld):** Physics Grab (attach to kinematic grab anchor).
     - **Free Props (Currently Held):** Throw along look-vector with impulse.

3. **Long Press / Hold (> 250ms): Radial / Context Menu:**
   - If held while looking at an interactive entity (or while holding a prop), display an ImGui radial/contextual popup listing all secondary verbs defined in the component (e.g., "Take to Inventory", "Dismantle", "Lock", "Barricade", "Inspect", "Gently Drop").
   - Releasing the hold on an option executes that verb callback in Lua.
   - Releasing the hold without moving the cursor/pointer dismisses the menu or defaults to the primary action.

4. **Prop Handling State Machine (Physics Grab vs. Throw):**
   - **Pickup Behavior:** Props are *not* automatically sent to bag inventory. The primary default action attaches the object to the player's physics carry anchor (using a Jolt kinematic/spring grab constraint).
   - **Held State Behavior:** While holding a prop:
     - Short click/tap: Releases or throws the prop forward with force proportional to player throw stat.
     - Hold interaction: Pops context menu (e.g., "Inspect Closely", "Stash in Backpack", "Place Carefully").

## Architecture & Data Schema

### 1. Engine Component (`engine/spatial/interactive.go`)
```go
type VerbID uint16

const (
    VerbGrab VerbID = iota + 1
    VerbThrow
    VerbOpenInventory
    VerbToggleState
    VerbStashInventory
    VerbDismantle
    VerbCustomLua
)

type InteractionOption struct {
    ID          VerbID
    LuaHook     string // e.g., "Cabinet_on_search"
    Label       string // e.g., "Search Drawer"
    Icon        string
    RequiresKey string // Optional tag/item ID required
}

type InteractiveComponent struct {
    Enabled       bool
    HighlightTint [4]float32 // RGBA glow
    DefaultVerb   InteractionOption
    ContextVerbs  []InteractionOption
}
```

### 2. Interaction Controller (`pkg/interaction/controller.go`)
```go
type InteractionState struct {
    HoveredEntity    EntityID
    ActiveHeldProp   EntityID // Non-zero if player is currently carrying an object
    PressStartTime   float64
    HoldThreshold    float64 // 0.250 seconds
    IsMenuOpen       bool
}
```

## Scope of Work

* [x] Implement stencil/outline pass on active hovered entity mesh.
* [x] Implement `InteractiveComponent` registration in Go engine and expose bindings to Lua.
* [x] Implement input timer to differentiate short tap vs. hold (> 250ms).
* [x] Wire default short-tap dispatch:
  * Free dynamic props: call Jolt grab constraint attachment.
  * Carried dynamic props: apply impulse along camera forward vector and release constraint.
  * Static interactive nodes: invoke primary `LuaHook` (toggle/inventory).
* [x] Render Dear ImGui contextual popup menu when hold threshold is crossed.
* [x] Implement secondary verb dispatch to entity Lua hooks.

## Acceptance Criteria

* Hovering the reticle over any entity with an `Interactive` component renders a glowing border around its geometry with zero dropped frames.
* A quick tap on a cabinet opens its inventory immediately.
* A quick tap on a loose prop (e.g., alarm clock, ball) grabs and holds it in front of the player via physics spring; a subsequent quick tap throws it.
* Holding click/tap on a prop or container pops the context menu allowing "Put in Backpack" or secondary actions.
* Moving the crosshair away from the object clears the highlight and dismisses any unpinned interaction prompts.

## Completion Summary
- **Outline & Stencil Highlighting (`engine/renderable.go`, `engine/outline_pass.go`):** Added `FlagOutline`, `OutlineColor`, `OutlineWidth`, `SetOutlined`/`IsOutlined`/`SetOutlineColor` on `Renderable` (propagated via `DrawCommand` in `ExtractFromHierarchy`), plus `OutlinePass` (`OutlineStencilBit = 0x02`, `OutlineVertexShader`/`OutlineFragmentShader`, and `OutlineGLAPI`/`RealOutlineGLAPI`). Also exposed `SetOutlined`, `IsOutlined`, and `SetOutlineColor` on `Renderable` Lua userdata in `pkg/luabridge/renderer_userdata.go` and fixed `GameObject.GetComponent` in `engine/game_object.go` so concrete pointer lookups do not panic on multi-component objects.
- **Interactive Component & Jolt Grab/Throw (`engine/spatial/interactive.go`, `engine/spatial/grab.go`, `engine/spatial/physics_world.go`):** Added `VerbID`, `InteractionOption`, `InteractiveComponent`, and `ResolveInteractiveFromHit` syncing hover outlines onto `Renderable`s, along with `PhysicsGrab` (`GrabBody`, `UpdateGrabTarget`, `ReleaseGrab`, `ThrowGrab`, `HeldBody`) using a Jolt kinematic sensor carry anchor and `JPH_PointConstraint` stepped cleanly in `PhysicsWorld.StepSimulation`.
- **Interaction Controller & ImGui Context Menu (`pkg/interaction/controller.go`, `pkg/interaction/imgui_menu.go`):** Implemented `InteractionState`, `Controller`, reticle hover highlight management, `< 250ms` quick-tap (`DefaultVerb` Lua dispatch, dynamic prop grab, held-prop throw), `>= 250ms` long-press context menu (both on hovered entities and held props), hover-loss dismissal, and Dear ImGui reticle prompts / contextual menu rendering (`DrawImGui`, `ComputeRadialLayout`).
- **Lua Bindings (`pkg/luabridge/interactive_userdata.go`, `pkg/luabridge/userdata.go`):** Registered `InteractiveComponent`, `InteractionVerb`, `GameObject:AddInteractive`/`GetInteractive`, and `NewInteractionDispatcher`/`BindInteractionController`.
- **Verification:** Passed `go test -count=1 -v`, `go vet`, and `go build` across `./engine`, `./engine/ui`, `./engine/spatial`, `./pkg/interaction`, `./pkg/luabridge`, and `./pkg/tileimporter`.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*
