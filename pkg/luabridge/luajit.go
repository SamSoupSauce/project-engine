package luabridge

/*
#cgo CFLAGS: -I${SRCDIR}/thirdparty/luajit/include
#cgo windows,amd64 LDFLAGS: ${SRCDIR}/thirdparty/luajit/lib/windows/amd64/libluajit.a -lmsvcrt -lm
#cgo linux,amd64 LDFLAGS: -lluajit-5.1 -lm -ldl
#cgo darwin LDFLAGS: -lluajit-5.1 -lm

#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>
#include <string.h>
#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>
#include <luajit.h>

static const char* lj_last_error(lua_State* L) {
	if (lua_gettop(L) > 0 && lua_isstring(L, -1)) {
		return lua_tostring(L, -1);
	}
	return "unknown luajit error";
}

static int lj_preload_chunk(lua_State* L, const char* name, const char* code, size_t len) {
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

static int lj_call_pointer_handler(lua_State* L, const char* fn_name, size_t fn_len, const char* node_id, size_t node_len, float mx, float my, int* handled_out) {
	char fn_buf[128];
	if (fn_len >= sizeof(fn_buf)) {
		return -1;
	}
	memcpy(fn_buf, fn_name, fn_len);
	fn_buf[fn_len] = '\0';

	lua_getglobal(L, fn_buf);
	if (!lua_isfunction(L, -1)) {
		lua_pop(L, 1);
		*handled_out = 0;
		return 0;
	}
	lua_pushlstring(L, node_id, node_len);
	lua_pushnumber(L, (lua_Number)mx);
	lua_pushnumber(L, (lua_Number)my);
	int rc = lua_pcall(L, 3, 1, 0);
	if (rc != 0) {
		return rc;
	}
	*handled_out = lua_toboolean(L, -1);
	lua_pop(L, 1);
	return 0;
}

static int lj_call_float1(lua_State* L, const char* fn_name, float arg0) {
	lua_getglobal(L, fn_name);
	if (!lua_isfunction(L, -1)) {
		lua_pop(L, 1);
		return 0;
	}
	lua_pushnumber(L, (lua_Number)arg0);
	return lua_pcall(L, 1, 0, 0);
}

static int lj_call_render(lua_State* L, const char* fn_name, void* draw_list, float w, float h) {
	lua_getglobal(L, fn_name);
	if (!lua_isfunction(L, -1)) {
		lua_pop(L, 1);
		return 0;
	}
	if (draw_list == NULL) {
		lua_pushnil(L);
	} else {
		lua_pushnumber(L, (lua_Number)(uintptr_t)draw_list);
	}
	lua_pushnumber(L, (lua_Number)w);
	lua_pushnumber(L, (lua_Number)h);
	return lua_pcall(L, 3, 0, 0);
}
*/
import "C"

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
	"unsafe"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/SamSoupSauce/project-engine/engine/ui"
)

//go:embed thirdparty/luajit/jit/*.lua
var embeddedLuaJITModules embed.FS

const defaultFFIDefs = `
local ffi = require("ffi")
ffi.cdef[[
	typedef struct {
		float x, y, w, h;
	} Rect;

	typedef struct {
		float x, y;
	} Vec2;

	typedef struct {
		uint8_t r, g, b, a;
	} Color32;

	typedef struct {
		float x, y, w, h;
	} UIRect;

	typedef struct {
		float r, g, b, a;
	} UIColor;

	typedef struct {
		Rect bounds;
		Color32 color;
		float rounding;
		int32_t id_hash;
		uint8_t visible;
		uint8_t hovered;
	} UILayoutNode;

	void DrawList_AddRectFilled(void* draw_list, float min_x, float min_y, float max_x, float max_y, uint32_t color, float rounding);
	void DrawList_PushClipRect(void* draw_list, float min_x, float min_y, float max_x, float max_y, bool intersect);
	void DrawList_PopClipRect(void* draw_list);
	void DrawList_AddText(void* draw_list, float x, float y, uint32_t color, const char* text);
	void igImDrawList_AddRectFilled(void* draw_list, float min_x, float min_y, float max_x, float max_y, uint32_t color, float rounding);
	void igImDrawList_PushClipRect(void* draw_list, float min_x, float min_y, float max_x, float max_y, bool intersect);
	void igImDrawList_PopClipRect(void* draw_list);
	void igImDrawList_AddText(void* draw_list, float x, float y, uint32_t color, const char* text);
]]
`

// LuaJITBridge manages a native LuaJIT 2.1 runtime with C-FFI ImDrawList bindings
// and embedded jit.v / jit.dump profiling support.
type LuaJITBridge struct {
	L  *C.lua_State
	mu sync.Mutex
}

// NewLuaJITBridge initializes a LuaJIT state, preloads jit.* profiling modules,
// and registers the engine's FFI C declarations.
func NewLuaJITBridge() (*LuaJITBridge, error) {
	L := C.luaL_newstate()
	if L == nil {
		return nil, fmt.Errorf("luabridge: luaL_newstate failed")
	}
	C.luaL_openlibs(L)

	b := &LuaJITBridge{L: L}
	if err := b.preloadEmbeddedJITModules(); err != nil {
		C.lua_close(L)
		return nil, err
	}
	if err := b.DoString(defaultFFIDefs); err != nil {
		C.lua_close(L)
		return nil, fmt.Errorf("luabridge: failed registering default ffi.cdef: %w", err)
	}
	if err := b.RegisterPhysicsFFI(); err != nil {
		C.lua_close(L)
		return nil, fmt.Errorf("luabridge: failed registering physics ffi.cdef: %w", err)
	}
	return b, nil
}

// Close tears down the native LuaJIT state.
func (b *LuaJITBridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.L != nil {
		C.lua_close(b.L)
		b.L = nil
	}
}

func (b *LuaJITBridge) preloadEmbeddedJITModules() error {
	return fs.WalkDir(embeddedLuaJITModules, "thirdparty/luajit/jit", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".lua") {
			return err
		}
		data, rerr := embeddedLuaJITModules.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		modName := "jit." + strings.TrimSuffix(path.Base(p), ".lua")
		cName := C.CString(modName)
		defer C.free(unsafe.Pointer(cName))

		var codePtr *C.char
		if len(data) > 0 {
			codePtr = (*C.char)(unsafe.Pointer(&data[0]))
		} else {
			codePtr = C.CString("")
			defer C.free(unsafe.Pointer(codePtr))
		}
		if rc := C.lj_preload_chunk(b.L, cName, codePtr, C.size_t(len(data))); rc != 0 {
			errMsg := C.GoString(C.lj_last_error(b.L))
			C.lua_settop(b.L, 0)
			return fmt.Errorf("luabridge: preload %s failed: %s", modName, errMsg)
		}
		return nil
	})
}

// DoString compiles and executes a Lua chunk in the LuaJIT VM.
func (b *LuaJITBridge) DoString(code string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.L == nil {
		return fmt.Errorf("luabridge: luajit state is closed")
	}
	cCode := C.CString(code)
	defer C.free(unsafe.Pointer(cCode))
	cName := C.CString("=chunk")
	defer C.free(unsafe.Pointer(cName))

	if rc := C.luaL_loadbuffer(b.L, cCode, C.size_t(len(code)), cName); rc != 0 {
		errMsg := C.GoString(C.lj_last_error(b.L))
		C.lua_settop(b.L, 0)
		return fmt.Errorf("luabridge: luajit load error: %s", errMsg)
	}
	if rc := C.lua_pcall(b.L, 0, C.LUA_MULTRET, 0); rc != 0 {
		errMsg := C.GoString(C.lj_last_error(b.L))
		C.lua_settop(b.L, 0)
		return fmt.Errorf("luabridge: luajit runtime error: %s", errMsg)
	}
	C.lua_settop(b.L, 0)
	return nil
}

// DoFile loads and executes a Lua file from disk in the LuaJIT VM.
func (b *LuaJITBridge) DoFile(filePath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.L == nil {
		return fmt.Errorf("luabridge: luajit state is closed")
	}
	cPath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cPath))

	if rc := C.luaL_loadfile(b.L, cPath); rc != 0 {
		errMsg := C.GoString(C.lj_last_error(b.L))
		C.lua_settop(b.L, 0)
		return fmt.Errorf("luabridge: luajit loadfile %q: %s", filePath, errMsg)
	}
	if rc := C.lua_pcall(b.L, 0, C.LUA_MULTRET, 0); rc != 0 {
		errMsg := C.GoString(C.lj_last_error(b.L))
		C.lua_settop(b.L, 0)
		return fmt.Errorf("luabridge: luajit execfile %q: %s", filePath, errMsg)
	}
	C.lua_settop(b.L, 0)
	return nil
}

// CallPointerHandler invokes a global LuaJIT pointer handler fn(nodeID, mouseX, mouseY)
// directly via the Lua C stack without string evaluation or C heap allocation overhead.
func (b *LuaJITBridge) CallPointerHandler(handlerName, nodeID string, mouseX, mouseY float32) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.L == nil {
		return false, fmt.Errorf("luabridge: luajit state is closed")
	}
	fnPtr := (*C.char)(unsafe.Pointer(unsafe.StringData(handlerName)))
	var nodePtr *C.char
	if len(nodeID) > 0 {
		nodePtr = (*C.char)(unsafe.Pointer(unsafe.StringData(nodeID)))
	}

	var handled C.int
	if rc := C.lj_call_pointer_handler(b.L, fnPtr, C.size_t(len(handlerName)), nodePtr, C.size_t(len(nodeID)), C.float(mouseX), C.float(mouseY), &handled); rc != 0 {
		errMsg := C.GoString(C.lj_last_error(b.L))
		C.lua_settop(b.L, 0)
		return false, fmt.Errorf("luabridge: pointer handler %q error: %s", handlerName, errMsg)
	}
	return handled != 0, nil
}

// CallTick invokes a global LuaJIT tick function fn(dt) directly via the Lua C stack.
func (b *LuaJITBridge) CallTick(fnName string, dt float32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.L == nil {
		return fmt.Errorf("luabridge: luajit state is closed")
	}
	cFn := C.CString(fnName)
	defer C.free(unsafe.Pointer(cFn))

	if rc := C.lj_call_float1(b.L, cFn, C.float(dt)); rc != 0 {
		errMsg := C.GoString(C.lj_last_error(b.L))
		C.lua_settop(b.L, 0)
		return fmt.Errorf("luabridge: tick %q error: %s", fnName, errMsg)
	}
	return nil
}

// CallRender invokes a global LuaJIT render function fn(drawListPtr, width, height)
// passing an ImDrawList C pointer directly across FFI.
func (b *LuaJITBridge) CallRender(fnName string, drawList *imgui.DrawList, width, height float32) error {
	var dlPtr unsafe.Pointer
	if drawList != nil {
		cData, fin := drawList.Handle()
		defer fin()
		dlPtr = unsafe.Pointer(cData)
	}
	return b.CallRenderPtr(fnName, dlPtr, width, height)
}

// CallRenderPtr invokes fn(drawListPtr, width, height) using a raw ImDrawList pointer.
func (b *LuaJITBridge) CallRenderPtr(fnName string, drawListPtr unsafe.Pointer, width, height float32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.L == nil {
		return fmt.Errorf("luabridge: luajit state is closed")
	}
	cFn := C.CString(fnName)
	defer C.free(unsafe.Pointer(cFn))

	if rc := C.lj_call_render(b.L, cFn, drawListPtr, C.float(width), C.float(height)); rc != 0 {
		errMsg := C.GoString(C.lj_last_error(b.L))
		C.lua_settop(b.L, 0)
		return fmt.Errorf("luabridge: render %q error: %s", fnName, errMsg)
	}
	return nil
}

// StartJITVerbose enables LuaJIT jit.v trace logging to the specified file path.
func (b *LuaJITBridge) StartJITVerbose(outPath string) error {
	escaped := strings.ReplaceAll(outPath, `\`, `\\`)
	return b.DoString(fmt.Sprintf(`require("jit.v").start(%q)`, escaped))
}

// StopJITVerbose disables LuaJIT jit.v trace logging.
func (b *LuaJITBridge) StopJITVerbose() error {
	return b.DoString(`require("jit.v").off()`)
}

// StartJITDump enables LuaJIT jit.dump trace diagnostics with the given flags (e.g. "bitmsx") to outPath.
func (b *LuaJITBridge) StartJITDump(flags, outPath string) error {
	escaped := strings.ReplaceAll(outPath, `\`, `\\`)
	return b.DoString(fmt.Sprintf(`require("jit.dump").start(%q, %q)`, flags, escaped))
}

// StopJITDump disables LuaJIT jit.dump trace diagnostics.
func (b *LuaJITBridge) StopJITDump() error {
	return b.DoString(`require("jit.dump").off()`)
}

// BindPointerDispatcher wires a ui.PointerEventDispatcher so pointer clicks and hovers
// dispatch directly into LuaJIT functions.
func (b *LuaJITBridge) BindPointerDispatcher(dispatcher *ui.PointerEventDispatcher) {
	if dispatcher == nil {
		return
	}
	dispatcher.OnDispatch = func(handlerName, nodeID string, mx, my float32) bool {
		handled, err := b.CallPointerHandler(handlerName, nodeID, mx, my)
		if err != nil {
			return false
		}
		return handled
	}
}
