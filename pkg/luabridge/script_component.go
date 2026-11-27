package luabridge

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
)

// ScriptComponent binds a Lua script file to an engine.GameObject with an isolated
// per-instance Lua environment table and engine lifecycle hook dispatchers.
type ScriptComponent struct {
	Bridge     *LuaBridge
	ScriptPath string
	ScriptName string
	Owner      *engine.GameObject

	env         *lua.LTable
	moduleTable *lua.LTable
	loaded      bool
	lastErr     error
	mu          sync.RWMutex
}

// NewScriptComponent loads and executes the specified Lua script file inside its own
// isolated Lua environment bound to the provided LuaBridge.
func NewScriptComponent(bridge *LuaBridge, scriptPath string) (*ScriptComponent, error) {
	if bridge == nil {
		return nil, fmt.Errorf("luabridge: bridge must not be nil")
	}
	if strings.TrimSpace(scriptPath) == "" {
		return nil, fmt.Errorf("luabridge: script path must not be empty")
	}

	sc := &ScriptComponent{
		Bridge:     bridge,
		ScriptPath: scriptPath,
	}
	if err := sc.loadFromFile(scriptPath); err != nil {
		return nil, err
	}
	return sc, nil
}

// NewScriptComponentFromString compiles and executes an inline Lua script source inside
// an isolated Lua environment bound to the provided LuaBridge.
func NewScriptComponentFromString(bridge *LuaBridge, chunkName, source string) (*ScriptComponent, error) {
	if bridge == nil {
		return nil, fmt.Errorf("luabridge: bridge must not be nil")
	}
	sc := &ScriptComponent{
		Bridge:     bridge,
		ScriptPath: chunkName,
	}
	if err := sc.loadFromReader(chunkName, source); err != nil {
		return nil, err
	}
	return sc, nil
}

func (sc *ScriptComponent) loadFromFile(path string) error {
	sc.Bridge.stateMu.Lock()
	defer sc.Bridge.stateMu.Unlock()

	fn, err := sc.Bridge.L.LoadFile(path)
	if err != nil {
		sc.setLastError(err)
		return err
	}
	return sc.initChunkLocked(fn)
}

func (sc *ScriptComponent) loadFromReader(chunkName, source string) error {
	sc.Bridge.stateMu.Lock()
	defer sc.Bridge.stateMu.Unlock()

	fn, err := sc.Bridge.L.Load(strings.NewReader(source), chunkName)
	if err != nil {
		sc.setLastError(err)
		return err
	}
	return sc.initChunkLocked(fn)
}

func (sc *ScriptComponent) initChunkLocked(fn *lua.LFunction) error {
	L := sc.Bridge.L

	// Isolate script definitions into a per-instance environment table whose metatable
	// falls back to _G so shared engine bindings remain accessible.
	env := L.NewTable()
	mt := L.NewTable()
	L.SetField(mt, "__index", L.Get(lua.GlobalsIndex))
	L.SetMetatable(env, mt)
	L.SetFEnv(fn, env)

	if err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}); err != nil {
		sc.setLastError(err)
		return err
	}

	ret := L.Get(-1)
	L.Pop(1)

	sc.mu.Lock()
	sc.env = env
	if tbl, ok := ret.(*lua.LTable); ok {
		sc.moduleTable = tbl
	} else {
		sc.moduleTable = nil
	}
	sc.loaded = true
	sc.lastErr = nil
	sc.mu.Unlock()
	return nil
}

// Init registers the owning GameObject and lazily loads ScriptPath if not yet loaded.
func (sc *ScriptComponent) Init(owner *engine.GameObject) {
	sc.mu.Lock()
	sc.Owner = owner
	alreadyLoaded := sc.loaded
	path := sc.ScriptPath
	bridge := sc.Bridge
	sc.mu.Unlock()

	if !alreadyLoaded && bridge != nil && strings.TrimSpace(path) != "" {
		if err := sc.loadFromFile(path); err != nil {
			bridge.RecordScriptError(path, "init", err)
		}
	}
}

// Update satisfies engine.Component. Fixed-rate Lua ticks are dispatched via OnTick.
func (sc *ScriptComponent) Update(dt float32) {}

// Destroy releases ownership and marks the component unloaded so hooks no longer fire.
func (sc *ScriptComponent) Destroy() {
	sc.mu.Lock()
	sc.Owner = nil
	sc.loaded = false
	sc.mu.Unlock()
}

// Env returns the isolated Lua environment table for this script instance.
func (sc *ScriptComponent) Env() *lua.LTable {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.env
}

// LastError returns the most recent load or hook execution error encountered by this script.
func (sc *ScriptComponent) LastError() error {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.lastErr
}

// OnLoad invokes the script's optional load(scene) hook.
func (sc *ScriptComponent) OnLoad(scene engine.Scene) {
	sc.callHook("load", func(L *lua.LState, _ *engine.GameObject) []lua.LValue {
		return []lua.LValue{sceneArg(L, scene)}
	})
}

// OnUnload invokes the script's optional unload(scene) hook.
func (sc *ScriptComponent) OnUnload(scene engine.Scene) {
	sc.callHook("unload", func(L *lua.LState, _ *engine.GameObject) []lua.LValue {
		return []lua.LValue{sceneArg(L, scene)}
	})
}

// OnSpawn invokes the script's optional spawn(self, sender) hook.
func (sc *ScriptComponent) OnSpawn(sender any) {
	sc.callHook("spawn", func(L *lua.LState, owner *engine.GameObject) []lua.LValue {
		return []lua.LValue{
			gameObjectArg(L, owner),
			senderArg(L, sender),
		}
	})
}

// OnEvent invokes the script's optional event(name, self, sender, data) hook.
func (sc *ScriptComponent) OnEvent(name string, sender *engine.GameObject, data any) {
	sc.callHook("event", func(L *lua.LState, owner *engine.GameObject) []lua.LValue {
		return []lua.LValue{
			lua.LString(name),
			gameObjectArg(L, owner),
			gameObjectArg(L, sender),
			goValueToLua(L, data),
		}
	})
}

// OnTick invokes the script's optional tick(self, dt) hook with dt in milliseconds.
func (sc *ScriptComponent) OnTick(dtMs int) {
	sc.callHook("tick", func(L *lua.LState, owner *engine.GameObject) []lua.LValue {
		return []lua.LValue{
			gameObjectArg(L, owner),
			lua.LNumber(dtMs),
		}
	})
}

// OnDespawn invokes the script's optional despawn(self, sender, data) hook.
func (sc *ScriptComponent) OnDespawn(sender *engine.GameObject, data any) {
	sc.callHook("despawn", func(L *lua.LState, owner *engine.GameObject) []lua.LValue {
		return []lua.LValue{
			gameObjectArg(L, owner),
			gameObjectArg(L, sender),
			goValueToLua(L, data),
		}
	})
}

// EffectiveScriptName returns the configured ScriptName, or derives it from the
// base filename of ScriptPath without its extension.
func (sc *ScriptComponent) EffectiveScriptName() string {
	sc.mu.RLock()
	name := strings.TrimSpace(sc.ScriptName)
	path := sc.ScriptPath
	sc.mu.RUnlock()

	if name != "" {
		return name
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	base := filepath.Base(trimmed)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// OnRender invokes the script's optional <ScriptName>_render(self, gfx) or render(self, gfx) hook.
func (sc *ScriptComponent) OnRender(gfx *engine.Renderer) {
	sc.OnRenderWithScriptName(gfx, "")
}

// OnRenderWithScriptName invokes <scriptName>_render(self, gfx) using scriptNameOverride
// (when provided by Renderable.ScriptName) or the script's effective ScriptName, falling
// back to an unprefixed render(self, gfx) function in the script's isolated environment.
func (sc *ScriptComponent) OnRenderWithScriptName(gfx *engine.Renderer, scriptNameOverride string) {
	sc.mu.RLock()
	bridge := sc.Bridge
	owner := sc.Owner
	env := sc.env
	mod := sc.moduleTable
	loaded := sc.loaded
	scriptPath := sc.ScriptPath
	sc.mu.RUnlock()

	if !loaded || bridge == nil || bridge.L == nil {
		return
	}

	prefix := strings.TrimSpace(scriptNameOverride)
	if prefix == "" {
		prefix = sc.EffectiveScriptName()
	}

	bridge.stateMu.Lock()
	defer bridge.stateMu.Unlock()

	L := bridge.L
	var fn *lua.LFunction
	hookName := "render"

	if prefix != "" {
		prefixed := prefix + "_render"
		fn = lookupHookWithGlobal(L, env, mod, prefixed)
		if fn != nil {
			hookName = prefixed
		}
	}
	if fn == nil {
		fn = lookupHook(env, mod, "render")
	}
	if fn == nil {
		return
	}

	args := []lua.LValue{
		gameObjectArg(L, owner),
		rendererArg(L, gfx),
	}
	if err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, args...); err != nil {
		sc.setLastError(err)
		bridge.RecordScriptError(scriptPath, hookName, err)
	}
}

func (sc *ScriptComponent) callHook(hookName string, buildArgs func(L *lua.LState, owner *engine.GameObject) []lua.LValue) {
	sc.mu.RLock()
	bridge := sc.Bridge
	owner := sc.Owner
	env := sc.env
	mod := sc.moduleTable
	loaded := sc.loaded
	scriptPath := sc.ScriptPath
	sc.mu.RUnlock()

	if !loaded || bridge == nil || bridge.L == nil {
		return
	}

	bridge.stateMu.Lock()
	defer bridge.stateMu.Unlock()

	fn := lookupHook(env, mod, hookName)
	if fn == nil {
		return
	}

	L := bridge.L
	args := buildArgs(L, owner)
	if err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, args...); err != nil {
		sc.setLastError(err)
		bridge.RecordScriptError(scriptPath, hookName, err)
	}
}

func (sc *ScriptComponent) setLastError(err error) {
	sc.mu.Lock()
	sc.lastErr = err
	sc.mu.Unlock()
}

func lookupHook(env, mod *lua.LTable, name string) *lua.LFunction {
	if env != nil {
		if fn, ok := env.RawGetString(name).(*lua.LFunction); ok {
			return fn
		}
	}
	if mod != nil {
		if fn, ok := mod.RawGetString(name).(*lua.LFunction); ok {
			return fn
		}
	}
	return nil
}

func lookupHookWithGlobal(L *lua.LState, env, mod *lua.LTable, name string) *lua.LFunction {
	if fn := lookupHook(env, mod, name); fn != nil {
		return fn
	}
	if L != nil {
		if fn, ok := L.GetGlobal(name).(*lua.LFunction); ok {
			return fn
		}
	}
	return nil
}

func rendererArg(L *lua.LState, gfx *engine.Renderer) lua.LValue {
	if gfx == nil {
		return lua.LNil
	}
	return rendererToUserData(L, gfx)
}

func gameObjectArg(L *lua.LState, obj *engine.GameObject) lua.LValue {
	if obj == nil {
		return lua.LNil
	}
	return gameObjectToUserData(L, obj)
}

func sceneArg(L *lua.LState, s engine.Scene) lua.LValue {
	if s == nil {
		return lua.LNil
	}
	return sceneToUserData(L, s)
}

func senderArg(L *lua.LState, sender any) lua.LValue {
	switch v := sender.(type) {
	case nil:
		return lua.LNil
	case *engine.GameObject:
		return gameObjectArg(L, v)
	case engine.Scene:
		return sceneArg(L, v)
	default:
		return goValueToLua(L, v)
	}
}
