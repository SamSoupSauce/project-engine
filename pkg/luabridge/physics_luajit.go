package luabridge

import (
	"fmt"

	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

const physicsFFIDefs = `
local ffi = require("ffi")
ffi.cdef[[
	typedef struct {
		float x, y, z;
	} JoltVec3;

	typedef struct {
		uint8_t hit;
		uint8_t _pad[3];
		uint32_t body_id;
		float distance;
		float fraction;
		float point_x, point_y, point_z;
		float normal_x, normal_y, normal_z;
	} JoltHitResult;

	typedef JoltHitResult HitResult;

	bool Jolt_RaycastOut(void* physics_system, const JoltVec3* origin, const JoltVec3* dir, float max_distance, JoltHitResult* out_hit);
	JoltHitResult Jolt_Raycast(void* physics_system, float ox, float oy, float oz, float dx, float dy, float dz, float max_distance);
	void Jolt_AddImpulseVec3(void* body_interface, uint32_t body_id, const JoltVec3* impulse);
	void Jolt_AddImpulse(void* body_interface, uint32_t body_id, float ix, float iy, float iz);
	void Jolt_SetVelocityVec3(void* body_interface, uint32_t body_id, const JoltVec3* vel);
	void Jolt_SetVelocity(void* body_interface, uint32_t body_id, float vx, float vy, float vz);
]]
`

const physicsLuaHelpers = `
local ffi = require("ffi")
local _JPH_SYSTEM = ffi.cast("void*", %dULL)
local _JPH_BODY_IFACE = ffi.cast("void*", %dULL)
local _SCRATCH_HIT = ffi.new("JoltHitResult")

function RaycastInto(origin, dir, max_distance, out_hit)
	return ffi.C.Jolt_RaycastOut(_JPH_SYSTEM, origin, dir, max_distance, out_hit)
end

function Raycast(origin, dir, max_distance)
	ffi.C.Jolt_RaycastOut(_JPH_SYSTEM, origin, dir, max_distance, _SCRATCH_HIT)
	return _SCRATCH_HIT
end

function AddImpulse(body_id, vec3)
	ffi.C.Jolt_AddImpulseVec3(_JPH_BODY_IFACE, body_id, vec3)
end

function SetVelocity(body_id, vec3)
	ffi.C.Jolt_SetVelocityVec3(_JPH_BODY_IFACE, body_id, vec3)
end
`

// RegisterPhysicsFFI registers JoltC FFI struct and function declarations in LuaJIT.
func (b *LuaJITBridge) RegisterPhysicsFFI() error {
	return b.DoString(physicsFFIDefs)
}

// BindPhysicsWorld binds a spatial.PhysicsWorld's native C pointers directly to LuaJIT FFI
// and installs zero-allocation Raycast, RaycastInto, AddImpulse, and SetVelocity helpers.
func (b *LuaJITBridge) BindPhysicsWorld(world *spatial.PhysicsWorld) error {
	if world == nil {
		return fmt.Errorf("luabridge: physics world must not be nil")
	}
	sysPtr, bodyIfacePtr := world.NativePointers()
	if sysPtr == nil || bodyIfacePtr == nil {
		return fmt.Errorf("luabridge: physics world native pointers are nil")
	}
	code := fmt.Sprintf(physicsLuaHelpers, uintptr(sysPtr), uintptr(bodyIfacePtr))
	return b.DoString(code)
}
