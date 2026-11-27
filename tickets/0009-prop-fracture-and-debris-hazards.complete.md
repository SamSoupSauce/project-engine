# Ticket 0009: Prop Fracture and Debris Hazards

## Status
Complete

## Description
Implement structural fracture limits, dynamic breakable joints, and persistent floor shard hazards for physical props in `project-engine`. Flimsy containers (e.g., small nightstand drawers) must yield to tensile/shear stress when yanked or struck, disintegrating on high-impulse impacts into low-damage matchwood debris while spilling contents dynamically. Fragile ceramic and glass props (e.g., table lamps, mugs, mirrors) must fracture on impact into persistent surface colliders that degrade barefoot player health and broadcast acoustic footstep events.

## Architectural Changes & Requirements

### 1. Jolt Joint Tension & Dynamic Break Limits (`engine/spatial`)
- Define `BreakForce` and `BreakTorque` thresholds on prismatic (slider) and revolute (hinge) constraints.
- When an interaction impulse exceeds the threshold:
  - Destroy the constraint in the Jolt physics world.
  - Convert the child drawer/door entity into a free-floating dynamic rigid body parented to the player's active arm socket if currently held.
  - Preserve linear and angular momentum on all internal dynamic props so loose contents scatter naturally into the room.

### 2. Prop Fracture & Matchwood Splintering (`engine/spatial`, `engine`)
- Add `FractureComponent` containing:
  - `Health`: Maximum impulse absorption before structural failure.
  - `FractureMode`: Splinter (wooden furniture/drawers), Shatter (glass/ceramic), or Deform (metal pans/toasters).
  - `DebrisPrefab`: Spawns sub-mesh physics fragments with low mass (~0.05–0.1 kg) upon destruction.
- Low-integrity props (nightstand drawers, light chairs) hitting an entity capsule deliver minimal kinetic impulse and stagger frames, converting entirely into decorative/low-tier wood colliders on impact.
- Heavy-integrity props (whole nightstands, desks) enforce two-arm carry mode, heavy camera rotational damping, and deliver high knockdown impulse without self-destructing.

### 3. Surface Shard Hazard & Acoustic Footwear Pipeline (`engine/hazards`, `pkg/luabridge`)
- Shattered ceramic and glass spawn persistent flat collision volumes tagged as `HazardShard`:
  - Traversal Check: On entity contact, query the player's footwear equipment slot.
  - Barefoot / Torn Footwear: Inflict laceration damage, apply bleeding debuff, trigger limp movement penalty, and fire a vocal audio event.
  - Booted Traversal: Prevent damage, but trigger high-frequency crunch sound events (`SoundRadius = 15.0m`) broadcasting an acoustic alert to nearby zombie AI.
- Expose `on_sweep` and kick physics impulses so debris can be displaced or cleared from defensive chokepoints using push brooms or physical strikes.

## Acceptance Criteria
- [x] Pulling a nightstand drawer past its tensile force threshold cleanly snaps the prismatic joint and sends unrestrained internal items flying along the pull vector.
- [x] Striking an enemy with a detached lightweight drawer obliterates the drawer into matchwood splinters with minimal enemy stagger.
- [x] Dropping or throwing a ceramic table lamp spawns sharp floor debris colliders.
- [x] Crossing floor shards without boots applies laceration damage, reduced movement speed, and audible screams.
- [x] Crossing floor shards with boots suppresses damage but emits high-range acoustic alerts to zombie entities.

---
### Clarification Log
- *Requirements are complete and clear; no clarifying questions needed.*

---
### Completion Summary
- **Ticket:** 0009-prop-fracture-and-debris-hazards
- **Branch:** 0009-prop-fracture-and-debris-hazards
- **Summary of Changes:**
  - **Breakable Jolt Joints (`engine/spatial/breakable_joint.go`, `engine/spatial/physics_world.go`):** Added `BreakableJoint` wrapping `JPH_SliderConstraint` (`JointSlider`) and `JPH_HingeConstraint` (`JointHinge`) with `BreakForce`/`BreakTorque` reaction lambda evaluation inside `PhysicsWorld.StepSimulation`, converting snapped child bodies into free dynamic bodies attached cleanly to the active carry anchor/arm socket when `ArmSocket` is active, detaching scene parenting while preserving world transforms, and spilling `ContainedProps` with preserved momentum along the pull vector.
  - **Prop Fracture & Weight Carry Tiers (`engine/spatial/fracture.go`, `pkg/interaction/controller.go`):** Implemented `FractureComponent` with `Health`, `FractureMode` (`FractureSplinter`, `FractureShatter`, `FractureDeform`), `PropIntegrityTier` (`IntegrityLow`, `IntegrityStandard`, `IntegrityHeavy`), `DebrisPrefab` (~0.05–0.1 kg fragments), `StrikeEntity`, and automatic collision $\Delta v$ monitoring. Wired `CarryHands` (two-arm carry for heavy props), `ApplyCameraLookDelta` rotational damping, and `PullBreakableJoint` into `pkg/interaction.Controller`.
  - **Floor Shard Hazards & Acoustic Footwear Pipeline (`engine/hazards/shard.go`):** Implemented `HazardSystem`, `HazardShard` (`Tag = "HazardShard"`), `FootwearKind` (`FootwearBarefoot`, `FootwearTorn`, `FootwearBoots`), `EvaluateTraversal` (laceration/bleeding/limp speed scale `0.60`/vocal screams when unbooted; zero damage + `15.0m` `SoundRadius` zombie alerts when booted), plus `OnSweep` and `Kick` clearing impulses.
  - **Lua Bindings (`pkg/luabridge/hazard_userdata.go`, `pkg/luabridge/userdata.go`):** Exposed `FractureComponent`, `HazardSystem`, `FractureMode`, `PropIntegrity`, `Footwear`, `GameObject:AddFracture`/`GetFracture`, `new_hazard_system`, `on_sweep`, and `kick_hazards`.
- **Files Modified / Added:**
  - [engine/spatial/breakable_joint.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/breakable_joint.go)
  - [engine/spatial/fracture.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/fracture.go)
  - [engine/spatial/physics_world.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/physics_world.go)
  - [engine/spatial/fracture_joint_test.go](file:///C:/Users/samue/Projects/project-engine/engine/spatial/fracture_joint_test.go)
  - [engine/hazards/shard.go](file:///C:/Users/samue/Projects/project-engine/engine/hazards/shard.go)
  - [engine/hazards/shard_test.go](file:///C:/Users/samue/Projects/project-engine/engine/hazards/shard_test.go)
  - [pkg/interaction/controller.go](file:///C:/Users/samue/Projects/project-engine/pkg/interaction/controller.go)
  - [pkg/luabridge/hazard_userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/hazard_userdata.go)
  - [pkg/luabridge/userdata.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/userdata.go)
  - [pkg/luabridge/hazard_lua_test.go](file:///C:/Users/samue/Projects/project-engine/pkg/luabridge/hazard_lua_test.go)
- **Verification Results:**
  - Ran `$env:CGO_LDFLAGS_ALLOW="-l:.*\.a"; go test -count=1 -v`, `go vet`, and `go build` across `./engine`, `./engine/ui`, `./engine/spatial`, `./engine/hazards`, `./pkg/interaction`, `./pkg/luabridge`, and `./pkg/tileimporter` — all passed cleanly with exit code `0`.
