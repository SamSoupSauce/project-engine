package luabridge

import (
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
)

// CommandHandler defines a handler for registered developer console / CLI commands.
type CommandHandler func(args []string) (string, error)

// InputHook allows intercepting raw console input strings before standard execution.
type InputHook func(input string) (handled bool, output string, err error)

// LuaBridge coordinates Lua scripting runtime execution, custom command registration,
// input-hook interception, and engine GameObject bindings without circular dependencies.
type LuaBridge struct {
	L             *lua.LState
	commands      map[string]CommandHandler
	inputHooks    []InputHook
	objects       map[string]*engine.GameObject
	dataMu        sync.RWMutex
	stateMu       reentrantMutex
	outputLogs    []string
	OnScriptError func(scriptPath, hook string, err error)
}

// NewLuaBridge creates an initialized Lua runtime bridge.
func NewLuaBridge() *LuaBridge {
	L := lua.NewState()
	bridge := &LuaBridge{
		L:          L,
		commands:   make(map[string]CommandHandler),
		inputHooks: make([]InputHook, 0),
		objects:    make(map[string]*engine.GameObject),
		outputLogs: make([]string, 0, 128),
	}

	bridge.registerStandardEngineAPIs()
	bridge.registerEngineTypes()
	return bridge
}

// NewScriptComponent loads a Lua script file into an isolated ScriptComponent bound to this bridge.
func (b *LuaBridge) NewScriptComponent(scriptPath string) (*ScriptComponent, error) {
	return NewScriptComponent(b, scriptPath)
}

// Close tears down the underlying Lua state.
func (b *LuaBridge) Close() {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.L != nil {
		b.L.Close()
	}
}

// RegisterCommand exposes a command name with custom argument handling.
func (b *LuaBridge) RegisterCommand(name string, handler CommandHandler) {
	b.dataMu.Lock()
	defer b.dataMu.Unlock()
	b.commands[strings.ToLower(strings.TrimSpace(name))] = handler
}

// AddInputHook registers an interceptor for raw input processing.
func (b *LuaBridge) AddInputHook(hook InputHook) {
	b.dataMu.Lock()
	defer b.dataMu.Unlock()
	b.inputHooks = append(b.inputHooks, hook)
}

// RegisterGameObject registers an engine GameObject pointer in the bridge.
func (b *LuaBridge) RegisterGameObject(name string, obj *engine.GameObject) {
	b.dataMu.Lock()
	defer b.dataMu.Unlock()
	b.objects[name] = obj
}

// GetGameObject looks up a registered GameObject by name.
func (b *LuaBridge) GetGameObject(name string) *engine.GameObject {
	b.dataMu.RLock()
	defer b.dataMu.RUnlock()
	return b.objects[name]
}

// Execute processes an input line: checking input hooks, registered commands, and Lua code.
func (b *LuaBridge) Execute(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil
	}

	b.stateMu.Lock()
	defer b.stateMu.Unlock()

	// 1. Run input hooks
	b.dataMu.RLock()
	hooksCopy := make([]InputHook, len(b.inputHooks))
	copy(hooksCopy, b.inputHooks)
	b.dataMu.RUnlock()

	for _, hook := range hooksCopy {
		handled, out, err := hook(trimmed)
		if handled {
			if out != "" {
				b.outputLogs = append(b.outputLogs, out)
			}
			return out, err
		}
	}

	// 2. Check registered CLI commands
	fields := strings.Fields(trimmed)
	cmdName := strings.ToLower(fields[0])
	b.dataMu.RLock()
	handler, exists := b.commands[cmdName]
	b.dataMu.RUnlock()

	if exists {
		args := fields[1:]
		out, err := handler(args)
		if out != "" {
			b.outputLogs = append(b.outputLogs, out)
		}
		return out, err
	}

	// 3. Fallback: Evaluate as Lua script
	err := b.L.DoString(trimmed)
	if err != nil {
		return "", err
	}

	// Fetch top of stack if a value was returned
	top := b.L.GetTop()
	if top > 0 {
		val := b.L.Get(top)
		b.L.Pop(1)
		if val != lua.LNil {
			res := val.String()
			b.outputLogs = append(b.outputLogs, res)
			return res, nil
		}
	}

	return "OK", nil
}

// registerStandardEngineAPIs sets up built-in Lua helper functions.
func (b *LuaBridge) registerStandardEngineAPIs() {
	// engine.get_position(name)
	b.L.SetGlobal("get_entity_position", b.L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		b.dataMu.RLock()
		obj, ok := b.objects[name]
		b.dataMu.RUnlock()
		if !ok || obj == nil {
			L.Push(lua.LNil)
			return 1
		}
		pos := obj.GetWorldTransform().Position
		tbl := L.NewTable()
		tbl.RawSetString("x", lua.LNumber(pos.X))
		tbl.RawSetString("y", lua.LNumber(pos.Y))
		tbl.RawSetString("z", lua.LNumber(pos.Z))
		L.Push(tbl)
		return 1
	}))

	// engine.set_active(name, bool)
	b.L.SetGlobal("set_entity_active", b.L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		active := L.CheckBool(2)
		b.dataMu.RLock()
		obj, ok := b.objects[name]
		b.dataMu.RUnlock()
		if !ok || obj == nil {
			L.Push(lua.LFalse)
			return 1
		}
		obj.Active = active
		L.Push(lua.LTrue)
		return 1
	}))

	// set_ui_size(name, width, height)
	b.L.SetGlobal("set_ui_size", b.L.NewFunction(func(L *lua.LState) int {
		r2d := b.lookupRenderable2D(L.CheckString(1))
		if r2d == nil {
			L.Push(lua.LFalse)
			return 1
		}
		r2d.Size = engine.NewVector2(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)))
		L.Push(lua.LTrue)
		return 1
	}))

	// set_ui_visible(name, visible)
	b.L.SetGlobal("set_ui_visible", b.L.NewFunction(func(L *lua.LState) int {
		r2d := b.lookupRenderable2D(L.CheckString(1))
		if r2d == nil {
			L.Push(lua.LFalse)
			return 1
		}
		r2d.SetVisible(L.CheckBool(2))
		L.Push(lua.LTrue)
		return 1
	}))

	// set_ui_color(name, r, g, b, a)
	b.L.SetGlobal("set_ui_color", b.L.NewFunction(func(L *lua.LState) int {
		r2d := b.lookupRenderable2D(L.CheckString(1))
		if r2d == nil {
			L.Push(lua.LFalse)
			return 1
		}
		r2d.Color = engine.NewColor(
			uint8(L.CheckInt(2)),
			uint8(L.CheckInt(3)),
			uint8(L.CheckInt(4)),
			uint8(L.OptInt(5, 255)),
		)
		L.Push(lua.LTrue)
		return 1
	}))

	// set_ui_uv(name, u, v, w, h)
	b.L.SetGlobal("set_ui_uv", b.L.NewFunction(func(L *lua.LState) int {
		r2d := b.lookupRenderable2D(L.CheckString(1))
		if r2d == nil {
			L.Push(lua.LFalse)
			return 1
		}
		r2d.SourceRect = engine.Rect2D{
			U: float32(L.CheckNumber(2)),
			V: float32(L.CheckNumber(3)),
			W: float32(L.CheckNumber(4)),
			H: float32(L.CheckNumber(5)),
		}
		L.Push(lua.LTrue)
		return 1
	}))
}

func (b *LuaBridge) lookupRenderable2D(name string) *engine.Renderable2D {
	b.dataMu.RLock()
	obj, ok := b.objects[name]
	b.dataMu.RUnlock()
	if !ok || obj == nil {
		return nil
	}
	return findRenderable2D(obj)
}
