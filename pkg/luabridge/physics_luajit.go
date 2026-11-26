package luabridge

/*
#include <stdlib.h>
#include <lua.h>
#include <lauxlib.h>

static const char* lj_physics_last_error(lua_State* L) {
	if (lua_gettop(L) > 0 && lua_isstring(L, -1)) {
		return lua_tostring(L, -1);
	}
	return "unknown luajit error";
}

static int lj_preload_physics_mod(lua_State* L, const char* name, const char* code, size_t len) {
	int rc = luaL_loadbuffer(L, code, len, name);
	if (rc != 0) {
		return rc;
	}
	lua_getglobal(L, "package");
	lua_getfield(L, -1, "preload");
	lua_pushvalue(L, -3);
	lua_setfield(L, -2, name);
	lua_pop(L, 3);
	return 0;
}
*/
import "C"

import (
	_ "embed"
	"fmt"
	"unsafe"

	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

//go:embed ffi_jolt.lua
var embeddedFFIJoltLua []byte

const physicsFFIDefs = `
local ffi = require("ffi")
if not pcall(ffi.typeof, "JoltVec3") then
	ffi.cdef[[
		typedef struct {
			float x, y, z;
		} JoltVec3;
	]]
end
if not pcall(ffi.typeof, "JoltHitResult") then
	ffi.cdef[[
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
end
`

const physicsLuaHelpers = `
local ffi = require("ffi")
local _JPH_SYSTEM = ffi.cast("void*", %dULL)
local _JPH_BODY_IFACE = ffi.cast("void*", %dULL)
PHYSICS_SYSTEM_PTR = _JPH_SYSTEM
BODY_INTERFACE_PTR = _JPH_BODY_IFACE
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

// NewLuaJITBridgeWithPhysics initializes a LuaJITBridge and binds the active PhysicsWorld's
// native pointers into LuaJIT at state initialization.
func NewLuaJITBridgeWithPhysics(world *spatial.PhysicsWorld) (*LuaJITBridge, error) {
	b, err := NewLuaJITBridge()
	if err != nil {
		return nil, err
	}
	if err := b.BindPhysicsWorld(world); err != nil {
		b.Close()
		return nil, err
	}
	return b, nil
}

// RegisterPhysicsFFI registers JoltC FFI struct and function declarations in LuaJIT
// and preloads the embedded ffi_jolt / physics Lua module.
func (b *LuaJITBridge) RegisterPhysicsFFI() error {
	if err := b.DoString(physicsFFIDefs); err != nil {
		return err
	}
	if err := b.preloadModule("ffi_jolt", embeddedFFIJoltLua); err != nil {
		return err
	}
	if err := b.preloadModule("physics", embeddedFFIJoltLua); err != nil {
		return err
	}
	return b.DoString(`require("ffi_jolt")`)
}

func (b *LuaJITBridge) preloadModule(name string, src []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.L == nil {
		return fmt.Errorf("luabridge: luajit state is closed")
	}
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var codePtr *C.char
	if len(src) > 0 {
		codePtr = (*C.char)(unsafe.Pointer(&src[0]))
	} else {
		codePtr = C.CString("")
		defer C.free(unsafe.Pointer(codePtr))
	}
	if rc := C.lj_preload_physics_mod(b.L, cName, codePtr, C.size_t(len(src))); rc != 0 {
		errMsg := C.GoString(C.lj_physics_last_error(b.L))
		C.lua_settop(b.L, 0)
		return fmt.Errorf("luabridge: preload %s failed: %s", name, errMsg)
	}
	return nil
}

// BindPhysicsWorld binds a spatial.PhysicsWorld's native C pointers directly to LuaJIT FFI
// and installs zero-allocation Raycast, RaycastInto, AddImpulse, SetVelocity, and PHYSICS_SYSTEM_PTR.
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
