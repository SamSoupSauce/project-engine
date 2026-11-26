package luabridge

import (
	"fmt"
	"sort"
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
	scene         *engine.BaseScene
	console       *Console
	pickedObj     *engine.GameObject
	dataMu        sync.RWMutex
	stateMu       reentrantMutex
	outputLogs    []string
	execCapture   *[]string
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
	bridge.registerConsoleCommands()
	return bridge
}

// AttachConsole binds a Console output target for Lua print(), help(), and pick() logs.
func (b *LuaBridge) AttachConsole(c *Console) {
	b.dataMu.Lock()
	b.console = c
	b.dataMu.Unlock()
}

// BindScene registers a BaseScene on LuaBridge for console object picking and object listing.
func (b *LuaBridge) BindScene(scene *engine.BaseScene) {
	b.dataMu.Lock()
	b.scene = scene
	b.dataMu.Unlock()
}

// SetPickedObject updates the currently picked GameObject and sets Lua global `selected`.
func (b *LuaBridge) SetPickedObject(obj *engine.GameObject) {
	b.dataMu.Lock()
	b.pickedObj = obj
	b.dataMu.Unlock()

	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.L == nil {
		return
	}
	if obj == nil {
		b.L.SetGlobal("selected", lua.LNil)
		return
	}
	pushGameObject(b.L, obj)
	ud := b.L.Get(-1)
	b.L.Pop(1)
	b.L.SetGlobal("selected", ud)
}

// GetPickedObject returns the currently picked GameObject, or nil.
func (b *LuaBridge) GetPickedObject() *engine.GameObject {
	b.dataMu.RLock()
	defer b.dataMu.RUnlock()
	return b.pickedObj
}

// EmitConsole forwards text lines into outputLogs, any active execution capture, and attached Console.
func (b *LuaBridge) EmitConsole(text string, lineType LineType) {
	if text == "" {
		return
	}
	b.dataMu.RLock()
	c := b.console
	b.dataMu.RUnlock()

	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		b.outputLogs = append(b.outputLogs, line)
		if b.execCapture != nil {
			*b.execCapture = append(*b.execCapture, line)
		}
		if c != nil {
			c.Print(line, lineType)
		}
	}
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
	out, _, err := b.executeWithStreamFlag(input)
	return out, err
}

func (b *LuaBridge) executeWithStreamFlag(input string) (string, bool, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false, nil
	}

	b.stateMu.Lock()
	defer b.stateMu.Unlock()

	var capture []string
	prevCapture := b.execCapture
	b.execCapture = &capture
	defer func() {
		b.execCapture = prevCapture
	}()

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
			return out, len(capture) > 0, err
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
		return out, len(capture) > 0, err
	}

	// 3. Fallback: Evaluate as Lua expression first (REPL style `return <expr>`),
	// falling back to statement execution when not a pure expression.
	baseTop := b.L.GetTop()
	var fn *lua.LFunction
	var err error

	if !strings.HasPrefix(trimmed, "return ") && trimmed != "return" {
		if compiled, compErr := b.L.LoadString("return " + trimmed); compErr == nil {
			fn = compiled
		}
	}
	if fn != nil {
		b.L.Push(fn)
		err = b.L.PCall(0, lua.MultRet, nil)
	} else {
		err = b.L.DoString(trimmed)
	}
	if err != nil {
		b.L.SetTop(baseTop)
		return "", len(capture) > 0, err
	}

	top := b.L.GetTop()
	if top > baseTop {
		parts := make([]string, 0, top-baseTop)
		for i := baseTop + 1; i <= top; i++ {
			parts = append(parts, SerializeLuaValue(b.L, b.L.Get(i)))
		}
		b.L.SetTop(baseTop)
		res := strings.Join(parts, "\n")
		for _, line := range strings.Split(res, "\n") {
			b.outputLogs = append(b.outputLogs, line)
		}
		return res, false, nil
	}

	if len(capture) > 0 {
		return strings.Join(capture, "\n"), true, nil
	}

	return "Nil", false, nil
}

// FormatHelp builds readable documentation lines for commands and built-in console/Lua helpers.
func (b *LuaBridge) FormatHelp(topic string) string {
	t := strings.ToLower(strings.TrimSpace(topic))
	switch t {
	case "pick", "selected", "picked":
		return strings.Join([]string{
			"Object Picking Help:",
			"  pick(nameOrId)    Select a GameObject and bind Lua global `selected`",
			"  picked()          Return the currently selected GameObject (or nil)",
			"  unpick()          Clear current object selection",
			"  list_objects()    List registered and scene GameObjects",
			"  Viewport: Click 'Pick Object' in Console (or Shift+Click in viewport) to ray-pick 3D objects",
		}, "\n")
	case "stdio", "print":
		return strings.Join([]string{
			"Console Output & Stdio Redirection:",
			"  print(...)        Prints Lua values into Console output buffer",
			"  io.write(...)     Forwards formatted writes into Console output",
			"  os.Stdout/Stderr  Redirected into Console when Console:RedirectStdio() is active",
		}, "\n")
	}

	b.dataMu.RLock()
	cmds := make([]string, 0, len(b.commands))
	for name := range b.commands {
		cmds = append(cmds, name)
	}
	b.dataMu.RUnlock()
	sort.Strings(cmds)

	lines := []string{
		"=== Project Engine Developer Console Help ===",
		"Lua Helpers:",
		"  help([topic])               Show this help, or help('pick') / help('stdio')",
		"  clear()                     Clear console output buffer",
		"  print(...)                  Print values to console output",
		"  pick(idOrName) / unpick()   Select or deselect a GameObject (`selected` / `picked`)",
		"  picked() / list_objects()   Inspect active target or list scene GameObjects",
		"  get_entity_position(name)   Return {x, y, z} world position table",
		"  set_entity_active(name, b)  Toggle GameObject.Active",
	}
	if len(cmds) > 0 {
		lines = append(lines, "Console Commands: "+strings.Join(cmds, ", "))
	}
	return strings.Join(lines, "\n")
}

func (b *LuaBridge) registerConsoleCommands() {
	b.RegisterCommand("help", func(args []string) (string, error) {
		topic := ""
		if len(args) > 0 {
			topic = args[0]
		}
		txt := b.FormatHelp(topic)
		b.EmitConsole(txt, LineInfo)
		return txt, nil
	})
	b.RegisterCommand("clear", func(_ []string) (string, error) {
		b.dataMu.RLock()
		c := b.console
		b.dataMu.RUnlock()
		if c != nil {
			c.ClearOutput()
		}
		return "", nil
	})
}

// registerStandardEngineAPIs sets up built-in Lua helper functions.
func (b *LuaBridge) registerStandardEngineAPIs() {
	// Redirect Lua print(...) to the developer console instead of bypassing to terminal stdout.
	b.L.SetGlobal("print", b.L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		parts := make([]string, 0, top)
		for i := 1; i <= top; i++ {
			parts = append(parts, L.ToStringMeta(L.Get(i)).String())
		}
		b.EmitConsole(strings.Join(parts, "\t"), LineOutput)
		return 0
	}))

	// Route Lua io.write(...) to console output as well when io table exists.
	if ioTbl, ok := b.L.GetGlobal("io").(*lua.LTable); ok && ioTbl != nil {
		b.L.SetField(ioTbl, "write", b.L.NewFunction(func(L *lua.LState) int {
			top := L.GetTop()
			var sb strings.Builder
			for i := 1; i <= top; i++ {
				sb.WriteString(L.ToStringMeta(L.Get(i)).String())
			}
			txt := strings.TrimRight(sb.String(), "\r\n")
			if txt != "" {
				b.EmitConsole(txt, LineOutput)
			}
			L.Push(lua.LTrue)
			return 1
		}))
	}

	// help([topic]) callable Lua function
	b.L.SetGlobal("help", b.L.NewFunction(func(L *lua.LState) int {
		topic := L.OptString(1, "")
		txt := b.FormatHelp(topic)
		b.EmitConsole(txt, LineInfo)
		return 0
	}))

	// clear() clears attached console output
	b.L.SetGlobal("clear", b.L.NewFunction(func(_ *lua.LState) int {
		b.dataMu.RLock()
		c := b.console
		b.dataMu.RUnlock()
		if c != nil {
			c.ClearOutput()
		}
		return 0
	}))

	// pick(nameOrID) selects a GameObject and sets globals `selected` / `picked`
	b.L.SetGlobal("pick", b.L.NewFunction(func(L *lua.LState) int {
		query := L.CheckString(1)
		obj := b.findSceneOrRegisteredObject(query)
		if obj == nil {
			L.Push(lua.LNil)
			return 1
		}
		b.dataMu.RLock()
		c := b.console
		b.dataMu.RUnlock()
		if c != nil {
			c.PickObject(obj)
		} else {
			b.SetPickedObject(obj)
			b.EmitConsole(fmt.Sprintf("[Picked] %s (%s)", obj.Name, obj.ID), LineSystem)
		}
		pushGameObject(L, obj)
		return 1
	}))

	// unpick() clears active selection
	b.L.SetGlobal("unpick", b.L.NewFunction(func(_ *lua.LState) int {
		b.dataMu.RLock()
		c := b.console
		b.dataMu.RUnlock()
		if c != nil {
			c.PickObject(nil)
		} else {
			b.SetPickedObject(nil)
		}
		return 0
	}))

	// picked() returns the currently selected GameObject userdata or nil
	b.L.SetGlobal("picked", b.L.NewFunction(func(L *lua.LState) int {
		pushGameObject(L, b.GetPickedObject())
		return 1
	}))

	// list_objects() prints and returns a table of available GameObject IDs/names
	b.L.SetGlobal("list_objects", b.L.NewFunction(func(L *lua.LState) int {
		seen := make(map[*engine.GameObject]bool)
		tbl := L.NewTable()
		idx := 1

		addObj := func(label string, obj *engine.GameObject) {
			if obj == nil || seen[obj] {
				return
			}
			seen[obj] = true
			pos := obj.GetWorldTransform().Position
			b.EmitConsole(fmt.Sprintf("  %s (%s) @ (%.2f, %.2f, %.2f)", obj.Name, obj.ID, pos.X, pos.Y, pos.Z), LineInfo)
			tbl.RawSetInt(idx, lua.LString(obj.ID))
			idx++
		}

		b.dataMu.RLock()
		keys := make([]string, 0, len(b.objects))
		for k := range b.objects {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			addObj(k, b.objects[k])
		}
		scene := b.scene
		b.dataMu.RUnlock()

		if scene != nil && scene.Root != nil {
			var walk func(n *engine.GameObject)
			walk = func(n *engine.GameObject) {
				if n == nil {
					return
				}
				if n.ID != "__root__" {
					addObj(n.ID, n)
				}
				for _, ch := range n.Children {
					walk(ch)
				}
			}
			walk(scene.Root)
		}
		if idx == 1 {
			b.EmitConsole("(no GameObjects registered)", LineInfo)
		}
		L.Push(tbl)
		return 1
	}))

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
