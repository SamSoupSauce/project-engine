package luabridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

func TestLuaJITPhysicsFFIRaycastImpulseAndVelocity(t *testing.T) {
	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	// Static target box at z=5
	targetObj := engine.NewGameObject("target", "TargetBox")
	targetObj.Transform.Position = engine.NewVector3(0, 1, 5)
	targetRB := spatial.NewRigidBody("target", world, engine.BodyStatic, spatial.BoxShapeDesc(1, 1, 1))
	targetObj.AddComponent(targetRB)

	// Dynamic crate at (10, 1, 0) with zero gravity for clean impulse/velocity verification
	crateObj := engine.NewGameObject("crate", "Crate")
	crateObj.Transform.Position = engine.NewVector3(10, 1, 0)
	crateRB := spatial.NewRigidBody("crate", world, engine.BodyDynamic, spatial.BoxShapeDesc(0.5, 0.5, 0.5))
	crateRB.GravityFactor = 0
	crateRB.LinearDamping = 0
	crateObj.AddComponent(crateRB)

	world.OptimizeBroadPhase()

	bridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.BindPhysicsWorld(world); err != nil {
		t.Fatalf("BindPhysicsWorld failed: %v", err)
	}

	luaScript := `
local ffi = require("ffi")
local ORIGIN = ffi.new("JoltVec3", { x = 0.0, y = 1.0, z = 0.0 })
local DIR_HIT = ffi.new("JoltVec3", { x = 0.0, y = 0.0, z = 1.0 })
local DIR_MISS = ffi.new("JoltVec3", { x = 0.0, y = 1.0, z = 0.0 })
local IMPULSE = ffi.new("JoltVec3", { x = 6.0, y = 0.0, z = 0.0 })
local VEL = ffi.new("JoltVec3", { x = 0.0, y = 0.0, z = 12.0 })

hit_ok = 0
hit_body = 0
hit_dist = 0.0
hit_nz = 0.0
miss_ok = 1

function run_raycast_probe()
	local res = Raycast(ORIGIN, DIR_HIT, 20.0)
	hit_ok = res.hit
	hit_body = res.body_id
	hit_dist = res.distance
	hit_nz = res.normal_z

	local miss = Raycast(ORIGIN, DIR_MISS, 20.0)
	miss_ok = miss.hit
end

function apply_crate_impulse(body_id)
	AddImpulse(body_id, IMPULSE)
end

function set_crate_velocity(body_id)
	SetVelocity(body_id, VEL)
end
`
	if err := bridge.DoString(luaScript); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	if err := bridge.DoString("run_raycast_probe()"); err != nil {
		t.Fatalf("run_raycast_probe failed: %v", err)
	}
	if err := bridge.DoString("assert(hit_ok == 1, 'expected raycast hit')"); err != nil {
		t.Fatalf("raycast hit assertion failed: %v", err)
	}
	if err := bridge.DoString("assert(miss_ok == 0, 'expected upward raycast miss')"); err != nil {
		t.Fatalf("raycast miss assertion failed: %v", err)
	}
	if err := bridge.DoString("assert(math.abs(hit_dist - 4.0) < 0.05, 'expected distance ~4.0')"); err != nil {
		t.Fatalf("raycast distance assertion failed: %v", err)
	}
	if err := bridge.DoString("assert(hit_nz < -0.9, 'expected normal_z ~ -1.0')"); err != nil {
		t.Fatalf("raycast normal assertion failed: %v", err)
	}

	// Verify AddImpulse from LuaJIT moves dynamic crate along +X
	if err := bridge.DoString(formatCall("apply_crate_impulse(%d)", crateRB.BodyID)); err != nil {
		t.Fatalf("apply_crate_impulse failed: %v", err)
	}
	world.StepSimulation(spatial.DefaultFixedTimestep)
	if crateRB.GetVelocity().X <= 1.0 {
		t.Fatalf("expected positive X velocity after LuaJIT AddImpulse, got %+v", crateRB.GetVelocity())
	}

	// Verify SetVelocity from LuaJIT sets Z velocity cleanly
	if err := bridge.DoString(formatCall("set_crate_velocity(%d)", crateRB.BodyID)); err != nil {
		t.Fatalf("set_crate_velocity failed: %v", err)
	}
	world.StepSimulation(spatial.DefaultFixedTimestep)
	vel := crateRB.GetVelocity()
	if vel.Z < 11.5 || vel.Z > 12.5 {
		t.Fatalf("expected Z velocity ~12.0 after LuaJIT SetVelocity, got %+v", vel)
	}
	if crateObj.Transform.Position.Z <= 0.1 {
		t.Fatalf("expected GameObject transform Z to sync after SetVelocity, got %+v", crateObj.Transform.Position)
	}
}

func formatCall(tmpl string, id uint32) string {
	return strings.Replace(tmpl, "%d", itoaUint32(id), 1)
}

func itoaUint32(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func TestLuaJITPhysicsZeroAllocationAndJITTraceAudit(t *testing.T) {
	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	target := engine.NewGameObject("wall", "Wall")
	target.Transform.Position = engine.NewVector3(0, 1, 5)
	target.AddComponent(spatial.NewRigidBody("wall", world, engine.BodyStatic, spatial.BoxShapeDesc(4, 4, 0.5)))

	dyn := engine.NewGameObject("dyn", "Dyn")
	dyn.Transform.Position = engine.NewVector3(5, 1, 0)
	dynRB := spatial.NewRigidBody("dyn", world, engine.BodyDynamic, spatial.BoxShapeDesc(0.5, 0.5, 0.5))
	dynRB.GravityFactor = 0
	dyn.AddComponent(dynRB)
	world.OptimizeBroadPhase()

	benchScript := `
local ffi = require("ffi")
local RAY_ORIGIN = ffi.new("JoltVec3", { x = 0.0, y = 1.0, z = 0.0 })
local RAY_DIR = ffi.new("JoltVec3", { x = 0.0, y = 0.0, z = 1.0 })
local IMPULSE = ffi.new("JoltVec3", { x = 0.0, y = 0.0, z = 0.0 })
local VEL = ffi.new("JoltVec3", { x = 1.0, y = 0.0, z = 0.0 })
local BODY_ID = ` + itoaUint32(dynRB.BodyID) + `

local acc = 0.0
local function step_hot()
	local res = Raycast(RAY_ORIGIN, RAY_DIR, 20.0)
	if res.hit == 1 then
		acc = acc + res.distance
	end
	AddImpulse(BODY_ID, IMPULSE)
	SetVelocity(BODY_ID, VEL)
end

local function run_bench_iters(n)
	for i = 1, n do
		step_hot()
	end
end

-- Warmup for JIT trace compilation using the exact same loop function
run_bench_iters(150)

collectgarbage("collect")
collectgarbage("collect")
local mem_before = collectgarbage("count")

run_bench_iters(1000)

local mem_after = collectgarbage("count")
assert(mem_before == mem_after, string.format("Physics FFI loop allocated %.2f KB", mem_after - mem_before))
assert(acc > 0.0, "expected raycast hits during hot loop")
`

	// Phase 1: Verify 0.00 KB GC growth cleanly without jit.v hook allocation overhead.
	bridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.BindPhysicsWorld(world); err != nil {
		t.Fatalf("BindPhysicsWorld failed: %v", err)
	}
	if err := bridge.DoString(benchScript); err != nil {
		t.Fatalf("zero-allocation physics FFI benchmark failed: %v", err)
	}

	// Phase 2: Verify jit.v trace compilation with zero NYI aborts on a fresh LuaJIT state.
	jitBridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge (jit.v) failed: %v", err)
	}
	defer jitBridge.Close()

	if err := jitBridge.BindPhysicsWorld(world); err != nil {
		t.Fatalf("BindPhysicsWorld (jit.v) failed: %v", err)
	}
	tracePath := filepath.Join(t.TempDir(), "physics_jit_v.log")
	if err := jitBridge.StartJITVerbose(tracePath); err != nil {
		t.Fatalf("StartJITVerbose failed: %v", err)
	}
	if err := jitBridge.DoString(benchScript); err != nil {
		t.Fatalf("jit.v physics FFI benchmark failed: %v", err)
	}
	if err := jitBridge.StopJITVerbose(); err != nil {
		t.Fatalf("StopJITVerbose failed: %v", err)
	}

	logBytes, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("failed reading jit.v log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "TRACE") {
		t.Errorf("expected compiled LuaJIT traces in jit.v log, got:\n%s", logText)
	}
	if strings.Contains(logText, "NYI") {
		t.Errorf("unexpected NYI trace abort in physics FFI jit.v log:\n%s", logText)
	}
}

func TestFFIJoltCastRayLayerMaskUserDataAndReticleHover(t *testing.T) {
	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	// Static room wall on LayerNonMoving (mask bit 0 = 1) with entity ID 101
	wallObj := engine.NewGameObject("wall", "RoomWall")
	wallObj.Transform.Position = engine.NewVector3(0, 1.5, 9)
	wallRB := spatial.NewRigidBody("wall", world, engine.BodyStatic, spatial.BoxShapeDesc(4, 3, 0.5))
	wallRB.UserData = 101
	wallObj.AddComponent(wallRB)

	// Interactable prop on LayerInteractable (mask bit 2 = 4) with entity ID 4242
	propObj := engine.NewGameObject("clock", "AlarmClock")
	propObj.Transform.Position = engine.NewVector3(0, 1.5, 4)
	propRB := spatial.NewRigidBody("clock", world, engine.BodyDynamic, spatial.BoxShapeDesc(0.5, 0.5, 0.5))
	propRB.GravityFactor = 0
	propRB.UserData = 4242
	propRB.SetObjectLayer(spatial.LayerInteractable)
	propObj.AddComponent(propRB)

	world.OptimizeBroadPhase()

	bridge, err := NewLuaJITBridgeWithPhysics(world)
	if err != nil {
		t.Fatalf("NewLuaJITBridgeWithPhysics failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "reticle_hover.lua")); err != nil {
		t.Fatalf("DoFile reticle_hover.lua failed: %v", err)
	}

	var calls []DrawListCall
	SetDrawListRecorder(&calls)
	defer SetDrawListRecorder(nil)

	// Verify physics.cast_ray hits interactable prop with accurate body_id, position, normal, user_data
	verifyLua := `
local physics = require("ffi_jolt")
assert(PHYSICS_SYSTEM_PTR ~= nil, "PHYSICS_SYSTEM_PTR must be initialized")

-- 1. All layers hit interactable prop at z=3.5
local h1 = physics.cast_ray(PHYSICS_SYSTEM_PTR, 0, 1.5, 0, 0, 0, 1, 20.0, 0xFFFF)
assert(h1 ~= nil and h1.has_hit, "expected hit on interactable prop")
assert(h1.body_id == ` + itoaUint32(propRB.BodyID) + `, "body_id mismatch")
assert(h1.user_data == 4242, "expected user_data 4242")
assert(math.abs(h1.hit_position.z - 3.5) < 0.05, "expected hit_position.z ~ 3.5")
assert(h1.hit_normal.z < -0.9, "expected hit_normal.z ~ -1")

-- 2. LayerNonMoving mask (0x0001) bypasses prop and hits static wall at z=8.5
local h2 = physics.cast_ray(PHYSICS_SYSTEM_PTR, 0, 1.5, 0, 0, 0, 1, 20.0, 0x0001)
assert(h2 ~= nil and h2.has_hit, "expected hit on static wall")
assert(h2.body_id == ` + itoaUint32(wallRB.BodyID) + `, "wall body_id mismatch")
assert(h2.user_data == 101, "expected wall user_data 101")
assert(math.abs(h2.hit_position.z - 8.5) < 0.05, "expected wall hit_position.z ~ 8.5")

-- 3. Short max_distance misses prop at 3.5m
local h3 = physics.cast_ray(PHYSICS_SYSTEM_PTR, 0, 1.5, 0, 0, 0, 1, 2.0, 0xFFFF)
assert(h3 == nil, "expected miss beyond max_distance")

-- 4. Reticle hover tick drives highlight state
local hovered = update_reticle_hover(nil, 0, 1.5, 0, 0, 0, 1, 10.0, 0x0004)
assert(hovered == true, "expected reticle hovered on interactable")
local state_hov, state_body, state_ud = get_reticle_state()
assert(state_hov == 1 and state_body == ` + itoaUint32(propRB.BodyID) + ` and state_ud == 4242)
render_reticle(nil, 1280, 720)

-- 5. Aiming away clears hover highlight
local hovered2 = update_reticle_hover(nil, 0, 1.5, 0, 0, 1, 0, 10.0, 0xFFFF)
assert(hovered2 == false, "expected reticle unhovered when aiming up")
render_reticle(nil, 1280, 720)
`
	if err := bridge.DoString(verifyLua); err != nil {
		t.Fatalf("ffi_jolt / reticle verification failed: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 AddRectFilled reticle draw calls, got %d", len(calls))
	}
	if calls[0].Color == calls[1].Color {
		t.Errorf("expected hovered reticle color (%#x) to differ from idle color (%#x)", calls[0].Color, calls[1].Color)
	}
}

func TestFFIJoltCastRayAndReticleZeroAllocationBenchmark(t *testing.T) {
	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	propObj := engine.NewGameObject("prop", "Prop")
	propObj.Transform.Position = engine.NewVector3(0, 1.5, 5)
	propRB := spatial.NewRigidBody("prop", world, engine.BodyStatic, spatial.BoxShapeDesc(1, 1, 1))
	propRB.UserData = 777
	propObj.AddComponent(propRB)
	world.OptimizeBroadPhase()

	benchLua := `
local function run_reticle_frames(n)
	for _ = 1, n do
		local ok = update_reticle_hover(PHYSICS_SYSTEM_PTR, 0.0, 1.5, 0.0, 0.0, 0.0, 1.0, 25.0, 0xFFFF)
		if not ok then
			error("expected hit during reticle loop")
		end
	end
end

run_reticle_frames(500)
collectgarbage("collect")
collectgarbage("collect")
run_reticle_frames(1)
local mem_before = collectgarbage("count")

run_reticle_frames(1000)
local mem_after = collectgarbage("count")
assert(mem_before == mem_after, string.format("reticle physics.cast_ray loop allocated %.2f KB", mem_after - mem_before))
`

	// Phase 1: Zero GC allocation check
	bridge, err := NewLuaJITBridgeWithPhysics(world)
	if err != nil {
		t.Fatalf("NewLuaJITBridgeWithPhysics failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "reticle_hover.lua")); err != nil {
		t.Fatalf("DoFile reticle_hover.lua failed: %v", err)
	}
	if err := bridge.DoString(benchLua); err != nil {
		t.Fatalf("reticle zero-allocation benchmark failed: %v", err)
	}

	// Phase 2: JIT trace audit via jit.v
	jitBridge, err := NewLuaJITBridgeWithPhysics(world)
	if err != nil {
		t.Fatalf("NewLuaJITBridgeWithPhysics (jit.v) failed: %v", err)
	}
	defer jitBridge.Close()

	if err := jitBridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "reticle_hover.lua")); err != nil {
		t.Fatalf("DoFile reticle_hover.lua (jit.v) failed: %v", err)
	}
	tracePath := filepath.Join(t.TempDir(), "ffi_jolt_jit_v.log")
	if err := jitBridge.StartJITVerbose(tracePath); err != nil {
		t.Fatalf("StartJITVerbose failed: %v", err)
	}
	if err := jitBridge.DoString(benchLua); err != nil {
		t.Fatalf("reticle benchmark (jit.v) failed: %v", err)
	}
	if err := jitBridge.StopJITVerbose(); err != nil {
		t.Fatalf("StopJITVerbose failed: %v", err)
	}

	raw, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read jit.v log failed: %v", err)
	}
	logText := string(raw)
	if !strings.Contains(logText, "[TRACE") {
		t.Errorf("expected compiled [TRACE entries in jit.v log, got:\n%s", logText)
	}
	if strings.Contains(logText, "NYI") {
		t.Errorf("unexpected NYI bailout in jit.v log:\n%s", logText)
	}
}
