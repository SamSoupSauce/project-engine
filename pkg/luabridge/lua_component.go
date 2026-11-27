package luabridge

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
)

const (
	luaTypeTokenTypeName    = "TypeToken"
	luaLuaComponentTypeName = "LuaComponent"
)

// TypeToken represents a non-component engine type constant exposed to Lua (e.g., GameObject).
type TypeToken struct {
	Name   string
	GoType string
}

// LuaComponentDef stores the declarative schema and shared prototype table registered via Component(...).
type LuaComponentDef struct {
	Name       string
	SourcePath string
	Defaults   map[string]lua.LValue
	FieldNames []string
	Hooks      map[string][]string
	HookNames  []string
	Proto      *lua.LTable
	Class      *ComponentClass
	ClassUD    *lua.LUserData
	Bridge     *LuaBridge
}

// LuaComponent is a Go engine.Component instance backed by a Lua Component(...) schema.
type LuaComponent struct {
	TypeName string
	Owner    *engine.GameObject
	Bridge   *LuaBridge
	Def      *LuaComponentDef
	Fields   *lua.LTable
	mu       sync.RWMutex
}

// HookListener tracks a registered Lua hook listener on a GameObject.
type HookListener struct {
	LuaComp  *LuaComponent
	Component engine.Component
	Value    lua.LValue
}

func (b *LuaBridge) registerComponentFactory() {
	b.registerTypeTokenGlobals()
	b.registerLuaComponentType()

	// Global Component(name [, default_fields [, hook_signatures]]) constructor.
	b.L.SetGlobal("Component", b.L.NewFunction(func(L *lua.LState) int {
		name := strings.TrimSpace(L.CheckString(1))
		if name == "" {
			L.ArgError(1, "component name must not be empty")
			return 0
		}
		if componentReflectType(name) != nil {
			L.ArgError(1, fmt.Sprintf("cannot redefine built-in component %q", name))
			return 0
		}

		defaults := make(map[string]lua.LValue)
		var fieldNames []string
		if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
			fieldsTbl := L.CheckTable(2)
			fieldsTbl.ForEach(func(k, v lua.LValue) {
				ks, ok := k.(lua.LString)
				if !ok {
					L.ArgError(2, fmt.Sprintf("field key must be string, got %s", k.Type().String()))
					return
				}
				fieldName := string(ks)
				defaults[fieldName] = cloneLuaValue(L, v)
				fieldNames = append(fieldNames, fieldName)
			})
			sort.Strings(fieldNames)
		}

		hooks := make(map[string][]string)
		var hookNames []string
		if L.GetTop() >= 3 && L.Get(3) != lua.LNil {
			hooksTbl := L.CheckTable(3)
			hooksTbl.ForEach(func(k, v lua.LValue) {
				ks, ok := k.(lua.LString)
				if !ok {
					L.ArgError(3, fmt.Sprintf("hook name must be string, got %s", k.Type().String()))
					return
				}
				sigTbl, ok := v.(*lua.LTable)
				if !ok {
					L.ArgError(3, fmt.Sprintf("hook %q signature must be a table of type names", string(ks)))
					return
				}
				hookName := string(ks)
				n := sigTbl.Len()
				sig := make([]string, 0, n)
				for i := 1; i <= n; i++ {
					elem := sigTbl.RawGetInt(i)
					typeName, err := resolveHookTypeName(elem)
					if err != nil {
						L.ArgError(3, fmt.Sprintf("hook %q arg %d: %v", hookName, i, err))
						return
					}
					sig = append(sig, typeName)
				}
				hooks[hookName] = sig
				hookNames = append(hookNames, hookName)
			})
			sort.Strings(hookNames)
		}

		def := b.registerCustomComponentDef(name, defaults, fieldNames, hooks, hookNames)
		L.Push(def.ClassUD)
		return 1
	}))
}

func (b *LuaBridge) registerTypeTokenGlobals() {
	mt := b.L.NewTypeMetatable(luaTypeTokenTypeName)
	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		tt := checkTypeToken(L, 1)
		key := L.CheckString(2)
		switch key {
		case "name", "Name", "__name":
			L.Push(lua.LString(tt.Name))
		case "go_type", "GoType":
			L.Push(lua.LString(tt.GoType))
		case "help", "Help":
			L.Push(L.NewFunction(func(L2 *lua.LState) int {
				L2.Push(lua.LString(FormatTypeTokenHelp(checkTypeToken(L2, 1))))
				return 1
			}))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
	b.L.SetField(mt, "__newindex", b.L.NewFunction(func(L *lua.LState) int {
		tt := checkTypeToken(L, 1)
		L.RaiseError("type token %s is read-only", tt.Name)
		return 0
	}))
	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		tt := checkTypeToken(L, 1)
		L.Push(lua.LString(fmt.Sprintf("Type<%s (%s)>", tt.Name, tt.GoType)))
		return 1
	}))

	for _, tok := range []TypeToken{
		{Name: "GameObject", GoType: "*engine.GameObject"},
		{Name: "Scene", GoType: "engine.Scene"},
	} {
		t := tok
		ud := b.L.NewUserData()
		ud.Value = &t
		b.L.SetMetatable(ud, mt)
		b.L.SetGlobal(t.Name, ud)
	}
}

func checkTypeToken(L *lua.LState, idx int) *TypeToken {
	ud := L.CheckUserData(idx)
	if tt, ok := ud.Value.(*TypeToken); ok && tt != nil {
		return tt
	}
	L.ArgError(idx, "TypeToken expected")
	return nil
}

func (b *LuaBridge) registerCustomComponentDef(
	name string,
	defaults map[string]lua.LValue,
	fieldNames []string,
	hooks map[string][]string,
	hookNames []string,
) *LuaComponentDef {
	b.dataMu.Lock()
	existing := b.customComponents[name]
	b.dataMu.Unlock()

	var proto *lua.LTable
	if existing != nil && existing.Proto != nil {
		proto = existing.Proto
	} else {
		proto = b.L.NewTable()
		b.L.SetField(proto, "__index", proto)
	}

	def := existing
	if def == nil {
		def = &LuaComponentDef{
			Name:   name,
			Proto:  proto,
			Bridge: b,
		}
	}
	def.Defaults = defaults
	def.FieldNames = fieldNames
	def.Hooks = hooks
	def.HookNames = hookNames
	if b.loadingScriptPath != "" {
		def.SourcePath = filepath.Clean(b.loadingScriptPath)
	}

	if def.Class == nil {
		cc := &ComponentClass{
			Name:      name,
			GoType:    "*luabridge.LuaComponent",
			Proto:     proto,
			CustomDef: def,
			Bridge:    b,
		}
		ud := b.L.NewUserData()
		ud.Value = cc
		b.L.SetMetatable(ud, b.L.GetTypeMetatable(luaComponentClassTypeName))
		def.Class = cc
		def.ClassUD = ud
	}

	b.dataMu.Lock()
	b.customComponents[name] = def
	b.componentClasses[name] = def.Class
	b.dataMu.Unlock()

	return def
}

func (b *LuaBridge) getCustomComponentDef(name string) *LuaComponentDef {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	b.dataMu.RLock()
	defer b.dataMu.RUnlock()
	if def, ok := b.customComponents[trimmed]; ok {
		return def
	}
	for k, def := range b.customComponents {
		if strings.EqualFold(k, trimmed) {
			return def
		}
	}
	return nil
}

func (b *LuaBridge) newLuaComponent(def *LuaComponentDef, overrides *lua.LTable) *LuaComponent {
	fields := b.L.NewTable()
	for _, k := range def.FieldNames {
		val := def.Defaults[k]
		if overrides != nil {
			if ov := overrides.RawGetString(k); ov != lua.LNil {
				val = ov
			}
		}
		fields.RawSetString(k, cloneLuaValue(b.L, val))
	}
	if overrides != nil {
		overrides.ForEach(func(k, v lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				key := string(ks)
				if _, hasDefault := def.Defaults[key]; !hasDefault {
					fields.RawSetString(key, cloneLuaValue(b.L, v))
				}
			}
		})
	}
	return &LuaComponent{
		TypeName: def.Name,
		Bridge:   b,
		Def:      def,
		Fields:   fields,
	}
}

func (b *LuaBridge) registerLuaComponentType() {
	mt := b.L.NewTypeMetatable(luaLuaComponentTypeName)

	builtinMethods := b.L.NewTable()
	b.L.SetField(builtinMethods, "GetOwner", b.L.NewFunction(func(L *lua.LState) int {
		lc := checkLuaComponent(L, 1)
		lc.mu.RLock()
		owner := lc.Owner
		lc.mu.RUnlock()
		pushGameObject(L, owner)
		return 1
	}))
	b.L.SetField(builtinMethods, "GetEntity", b.L.NewFunction(func(L *lua.LState) int {
		lc := checkLuaComponent(L, 1)
		lc.mu.RLock()
		owner := lc.Owner
		lc.mu.RUnlock()
		pushGameObject(L, owner)
		return 1
	}))
	b.L.SetField(builtinMethods, "GetTypeName", b.L.NewFunction(func(L *lua.LState) int {
		lc := checkLuaComponent(L, 1)
		L.Push(lua.LString(lc.TypeName))
		return 1
	}))
	b.L.SetField(builtinMethods, "Help", b.L.NewFunction(func(L *lua.LState) int {
		lc := checkLuaComponent(L, 1)
		L.Push(lua.LString(b.formatCustomComponentHelp(lc.Def, lc)))
		return 1
	}))

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		lc := checkLuaComponent(L, 1)
		key := L.CheckString(2)

		switch key {
		case "entity", "owner", "Owner":
			lc.mu.RLock()
			owner := lc.Owner
			lc.mu.RUnlock()
			pushGameObject(L, owner)
			return 1
		case "__name", "__type":
			L.Push(lua.LString(lc.TypeName))
			return 1
		}

		if lc.Fields != nil {
			if val := lc.Fields.RawGetString(key); val != lua.LNil {
				L.Push(val)
				return 1
			}
		}
		if lc.Def != nil && lc.Def.Proto != nil {
			if val := lc.Def.Proto.RawGetString(key); val != lua.LNil {
				L.Push(val)
				return 1
			}
		}
		if fn := builtinMethods.RawGetString(key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	b.L.SetField(mt, "__newindex", b.L.NewFunction(func(L *lua.LState) int {
		lc := checkLuaComponent(L, 1)
		key := L.CheckString(2)
		switch key {
		case "entity", "owner", "Owner", "__name", "__type":
			L.RaiseError("component %s property %q is read-only", lc.TypeName, key)
			return 0
		}
		if lc.Fields == nil {
			lc.Fields = L.NewTable()
		}
		lc.Fields.RawSetString(key, L.Get(3))
		return 0
	}))

	b.L.SetField(mt, "__eq", b.L.NewFunction(func(L *lua.LState) int {
		ud1, ok1 := L.Get(1).(*lua.LUserData)
		ud2, ok2 := L.Get(2).(*lua.LUserData)
		if !ok1 || !ok2 {
			L.Push(lua.LFalse)
			return 1
		}
		c1, _ := ud1.Value.(*LuaComponent)
		c2, _ := ud2.Value.(*LuaComponent)
		L.Push(lua.LBool(c1 != nil && c1 == c2))
		return 1
	}))

	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		lc := checkLuaComponent(L, 1)
		L.Push(lua.LString(SerializeComponentJSON(lc)))
		return 1
	}))
}

func pushLuaComponent(L *lua.LState, lc *LuaComponent) {
	if lc == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = lc
	L.SetMetatable(ud, L.GetTypeMetatable(luaLuaComponentTypeName))
	L.Push(ud)
}

func checkLuaComponent(L *lua.LState, idx int) *LuaComponent {
	ud := L.CheckUserData(idx)
	if lc, ok := ud.Value.(*LuaComponent); ok && lc != nil {
		return lc
	}
	L.ArgError(idx, "LuaComponent expected")
	return nil
}

// Init registers the owning GameObject, binds declared hook listeners, and calls optional Lua Init(self, owner).
func (lc *LuaComponent) Init(owner *engine.GameObject) {
	lc.mu.Lock()
	lc.Owner = owner
	bridge := lc.Bridge
	def := lc.Def
	lc.mu.Unlock()

	if bridge != nil && owner != nil && def != nil {
		for _, hookName := range def.HookNames {
			bridge.registerLuaComponentHook(owner, hookName, lc)
		}
	}

	lc.callMethod("Init", func(L *lua.LState) []lua.LValue {
		return []lua.LValue{gameObjectArg(L, owner)}
	})
}

// Update advances per-frame Lua logic when the prototype declares Update(self, dt).
func (lc *LuaComponent) Update(dt float32) {
	lc.callMethod("Update", func(L *lua.LState) []lua.LValue {
		return []lua.LValue{lua.LNumber(dt)}
	})
}

// Destroy invokes optional Lua Destroy(self) and unregisters hook listeners from the owner.
func (lc *LuaComponent) Destroy() {
	lc.callMethod("Destroy", nil)

	lc.mu.Lock()
	owner := lc.Owner
	bridge := lc.Bridge
	lc.Owner = nil
	lc.mu.Unlock()

	if bridge != nil && owner != nil {
		bridge.unregisterLuaComponentHooks(owner, lc)
	}
}

// OnLoad invokes optional Lua Load(self, scene).
func (lc *LuaComponent) OnLoad(scene engine.Scene) {
	lc.callMethod("Load", func(L *lua.LState) []lua.LValue {
		return []lua.LValue{sceneArg(L, scene)}
	})
}

// OnUnload invokes optional Lua Unload(self, scene).
func (lc *LuaComponent) OnUnload(scene engine.Scene) {
	lc.callMethod("Unload", func(L *lua.LState) []lua.LValue {
		return []lua.LValue{sceneArg(L, scene)}
	})
}

// OnSpawn invokes optional Lua Spawn(self, sender).
func (lc *LuaComponent) OnSpawn(sender any) {
	lc.callMethod("Spawn", func(L *lua.LState) []lua.LValue {
		return []lua.LValue{senderArg(L, sender)}
	})
}

// OnTick invokes optional Lua Tick(self, dtMs).
func (lc *LuaComponent) OnTick(dtMs int) {
	lc.callMethod("Tick", func(L *lua.LState) []lua.LValue {
		return []lua.LValue{lua.LNumber(dtMs)}
	})
}

// OnDespawn invokes optional Lua Despawn(self, sender, data).
func (lc *LuaComponent) OnDespawn(sender *engine.GameObject, data any) {
	lc.callMethod("Despawn", func(L *lua.LState) []lua.LValue {
		return []lua.LValue{gameObjectArg(L, sender), goValueToLua(L, data)}
	})
}

// OnEvent dispatches matching declared hooks or prototype methods when an event fires on the owner.
func (lc *LuaComponent) OnEvent(name string, sender *engine.GameObject, data any) {
	bridge := lc.Bridge
	def := lc.Def
	if bridge == nil || bridge.L == nil || def == nil {
		return
	}

	bridge.stateMu.Lock()
	defer bridge.stateMu.Unlock()

	fn := lc.lookupMethodLocked(name)
	if fn == nil {
		return
	}

	sig, hasSig := def.Hooks[name]
	L := bridge.L
	var args []lua.LValue
	if hasSig {
		args = buildEventArgsForHook(L, sig, sender, data)
		if err := validateHookArgs(L, lc.TypeName, name, sig, args); err != nil {
			bridge.recordHookError(lc.TypeName, name, err)
			return
		}
	} else {
		args = []lua.LValue{gameObjectArg(L, sender), goValueToLua(L, data)}
	}

	pushLuaComponent(L, lc)
	selfUD := L.Get(-1)
	L.Pop(1)

	callArgs := make([]lua.LValue, 0, 1+len(args))
	callArgs = append(callArgs, selfUD)
	callArgs = append(callArgs, args...)
	if err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, callArgs...); err != nil {
		bridge.recordHookError(lc.TypeName, name, err)
	}
}

func (lc *LuaComponent) callMethod(methodName string, buildArgs func(L *lua.LState) []lua.LValue) {
	bridge := lc.Bridge
	if bridge == nil || bridge.L == nil {
		return
	}
	bridge.stateMu.Lock()
	defer bridge.stateMu.Unlock()

	fn := lc.lookupMethodLocked(methodName)
	if fn == nil {
		return
	}

	L := bridge.L
	pushLuaComponent(L, lc)
	selfUD := L.Get(-1)
	L.Pop(1)

	var extra []lua.LValue
	if buildArgs != nil {
		extra = buildArgs(L)
	}
	callArgs := make([]lua.LValue, 0, 1+len(extra))
	callArgs = append(callArgs, selfUD)
	callArgs = append(callArgs, extra...)
	if err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, callArgs...); err != nil {
		bridge.recordHookError(lc.TypeName, methodName, err)
	}
}

func (lc *LuaComponent) lookupMethodLocked(name string) *lua.LFunction {
	if lc.Fields != nil {
		if fn, ok := lc.Fields.RawGetString(name).(*lua.LFunction); ok {
			return fn
		}
	}
	if lc.Def != nil && lc.Def.Proto != nil {
		if fn, ok := lc.Def.Proto.RawGetString(name).(*lua.LFunction); ok {
			return fn
		}
	}
	return nil
}

func (b *LuaBridge) recordHookError(compName, hookName string, err error) {
	b.RecordScriptError(compName, hookName, err)
}

func (b *LuaBridge) registerLuaComponentHook(obj *engine.GameObject, hookName string, lc *LuaComponent) {
	if obj == nil || lc == nil || hookName == "" {
		return
	}
	b.dataMu.Lock()
	defer b.dataMu.Unlock()

	byHook := b.hookListeners[obj]
	if byHook == nil {
		byHook = make(map[string][]*HookListener)
		b.hookListeners[obj] = byHook
	}
	for _, existing := range byHook[hookName] {
		if existing.LuaComp == lc {
			return
		}
	}
	byHook[hookName] = append(byHook[hookName], &HookListener{
		LuaComp:   lc,
		Component: lc,
	})
}

func (b *LuaBridge) registerHookListenerValue(obj *engine.GameObject, hookName string, val lua.LValue) error {
	if obj == nil || strings.TrimSpace(hookName) == "" {
		return fmt.Errorf("invalid GameObject or hook name")
	}
	hl := &HookListener{Value: val}
	if ud, ok := val.(*lua.LUserData); ok && ud != nil {
		if lc, ok := ud.Value.(*LuaComponent); ok {
			hl.LuaComp = lc
			hl.Component = lc
		} else if comp, ok := ud.Value.(engine.Component); ok {
			hl.Component = comp
		}
	}

	b.dataMu.Lock()
	defer b.dataMu.Unlock()

	byHook := b.hookListeners[obj]
	if byHook == nil {
		byHook = make(map[string][]*HookListener)
		b.hookListeners[obj] = byHook
	}
	for _, existing := range byHook[hookName] {
		if hl.LuaComp != nil && existing.LuaComp == hl.LuaComp {
			return nil
		}
		if hl.Component != nil && existing.Component == hl.Component {
			return nil
		}
		if hl.Value != nil && existing.Value == hl.Value {
			return nil
		}
	}
	byHook[hookName] = append(byHook[hookName], hl)
	return nil
}

func (b *LuaBridge) unregisterLuaComponentHooks(obj *engine.GameObject, lc *LuaComponent) {
	if obj == nil || lc == nil {
		return
	}
	b.dataMu.Lock()
	defer b.dataMu.Unlock()

	byHook := b.hookListeners[obj]
	if byHook == nil {
		return
	}
	for hookName, list := range byHook {
		filtered := list[:0]
		for _, item := range list {
			if item.LuaComp != lc {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 0 {
			delete(byHook, hookName)
		} else {
			byHook[hookName] = filtered
		}
	}
	if len(byHook) == 0 {
		delete(b.hookListeners, obj)
	}
}

func (b *LuaBridge) callObjectHook(L *lua.LState, obj *engine.GameObject, hookName string, args []lua.LValue) (int, error) {
	b.dataMu.RLock()
	var listeners []*HookListener
	if byHook := b.hookListeners[obj]; byHook != nil {
		listeners = append(listeners, byHook[hookName]...)
	}
	b.dataMu.RUnlock()

	// Also include any attached LuaComponents declaring hookName or defining it on Proto
	// even if added directly via Go without RegisterHookListener.
	seenLua := make(map[*LuaComponent]bool)
	for _, l := range listeners {
		if l.LuaComp != nil {
			seenLua[l.LuaComp] = true
		}
	}
	for _, c := range obj.Components {
		if lc, ok := c.(*LuaComponent); ok && !seenLua[lc] {
			hasDecl := false
			if lc.Def != nil {
				if _, ok := lc.Def.Hooks[hookName]; ok {
					hasDecl = true
				}
			}
			if hasDecl || lc.lookupMethodLocked(hookName) != nil {
				listeners = append(listeners, &HookListener{LuaComp: lc, Component: lc})
				seenLua[lc] = true
			}
		}
	}

	invoked := 0
	for _, listener := range listeners {
		if lc := listener.LuaComp; lc != nil {
			fn := lc.lookupMethodLocked(hookName)
			if lc.Def != nil {
				if sig, hasSig := lc.Def.Hooks[hookName]; hasSig {
					if err := validateHookArgs(L, lc.TypeName, hookName, sig, args); err != nil {
						return invoked, err
					}
				}
			}
			if fn == nil {
				continue
			}
			pushLuaComponent(L, lc)
			selfUD := L.Get(-1)
			L.Pop(1)
			callArgs := make([]lua.LValue, 0, 1+len(args))
			callArgs = append(callArgs, selfUD)
			callArgs = append(callArgs, args...)
			if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, callArgs...); err != nil {
				return invoked, err
			}
			invoked++
			continue
		}

		if fn, ok := listener.Value.(*lua.LFunction); ok {
			if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, args...); err != nil {
				return invoked, err
			}
			invoked++
			continue
		}

		if listener.Value != nil && listener.Value != lua.LNil {
			fnVal := L.GetField(listener.Value, hookName)
			if fn, ok := fnVal.(*lua.LFunction); ok {
				callArgs := make([]lua.LValue, 0, 1+len(args))
				callArgs = append(callArgs, listener.Value)
				callArgs = append(callArgs, args...)
				if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, callArgs...); err != nil {
					return invoked, err
				}
				invoked++
			}
		}
	}
	return invoked, nil
}

func resolveHookTypeName(v lua.LValue) (string, error) {
	switch tv := v.(type) {
	case lua.LString:
		s := strings.TrimSpace(string(tv))
		if s == "" {
			return "", fmt.Errorf("type name must not be empty")
		}
		return canonicalHookTypeName(s), nil
	case *lua.LUserData:
		if tv.Value == nil {
			return "", fmt.Errorf("invalid nil userdata type token")
		}
		switch tok := tv.Value.(type) {
		case *TypeToken:
			return tok.Name, nil
		case *ComponentClass:
			return tok.Name, nil
		default:
			return "", fmt.Errorf("unsupported userdata type token %T", tv.Value)
		}
	default:
		return "", fmt.Errorf("expected string type name or type token, got %s", v.Type().String())
	}
}

func canonicalHookTypeName(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "gameobject", "game_object", "*engine.gameobject":
		return "GameObject"
	case "scene", "engine.scene":
		return "Scene"
	case "string":
		return "string"
	case "number", "float", "float32", "float64", "int":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "table":
		return "table"
	case "function":
		return "function"
	case "any", "*":
		return "any"
	default:
		return resolveComponentAlias(raw)
	}
}

func buildEventArgsForHook(L *lua.LState, sig []string, sender *engine.GameObject, data any) []lua.LValue {
	if len(sig) == 0 {
		return nil
	}
	if len(sig) == 1 {
		if sig[0] == "GameObject" {
			if sender != nil {
				return []lua.LValue{gameObjectArg(L, sender)}
			}
			if goData, ok := data.(*engine.GameObject); ok && goData != nil {
				return []lua.LValue{gameObjectArg(L, goData)}
			}
			return []lua.LValue{lua.LNil}
		}
		return []lua.LValue{goValueToLua(L, data)}
	}
	if sig[0] == "GameObject" {
		return []lua.LValue{gameObjectArg(L, sender), goValueToLua(L, data)}
	}
	return []lua.LValue{goValueToLua(L, data), gameObjectArg(L, sender)}
}

func validateHookArgs(L *lua.LState, compName, hookName string, sig []string, args []lua.LValue) error {
	if len(args) < len(sig) {
		got := "none"
		if len(args) > 0 {
			got = fmt.Sprintf("%d arg(s)", len(args))
		}
		return fmt.Errorf("hook %q on component %q expects %d argument(s) (%s), got %s",
			hookName, compName, len(sig), strings.Join(sig, ", "), got)
	}
	for i, expected := range sig {
		if !matchesHookArgType(expected, args[i]) {
			return fmt.Errorf("hook %q on component %q expects argument %d of type %s, got %s",
				hookName, compName, i+1, expected, describeLuaValueType(args[i]))
		}
	}
	return nil
}

func matchesHookArgType(expected string, val lua.LValue) bool {
	if expected == "any" {
		return true
	}
	if val == nil || val == lua.LNil {
		return false
	}
	switch expected {
	case "string":
		return val.Type() == lua.LTString
	case "number":
		return val.Type() == lua.LTNumber
	case "boolean":
		return val.Type() == lua.LTBool
	case "table":
		return val.Type() == lua.LTTable
	case "function":
		return val.Type() == lua.LTFunction
	case "GameObject":
		ud, ok := val.(*lua.LUserData)
		if !ok || ud == nil {
			return false
		}
		obj, ok := ud.Value.(*engine.GameObject)
		return ok && obj != nil
	case "Scene":
		ud, ok := val.(*lua.LUserData)
		if !ok || ud == nil {
			return false
		}
		_, ok = ud.Value.(engine.Scene)
		return ok
	default:
		ud, ok := val.(*lua.LUserData)
		if !ok || ud == nil || ud.Value == nil {
			return false
		}
		comp, ok := ud.Value.(engine.Component)
		if !ok {
			return false
		}
		if expected == "Component" {
			return true
		}
		return strings.EqualFold(componentCanonicalName(comp), expected)
	}
}

func describeLuaValueType(val lua.LValue) string {
	if val == nil || val == lua.LNil {
		return "nil"
	}
	if ud, ok := val.(*lua.LUserData); ok && ud != nil {
		switch v := ud.Value.(type) {
		case *engine.GameObject:
			return "GameObject"
		case *LuaComponent:
			return v.TypeName
		case engine.Component:
			return componentCanonicalName(v)
		case *ComponentClass:
			return "ComponentClass<" + v.Name + ">"
		case *TypeToken:
			return "TypeToken<" + v.Name + ">"
		default:
			return fmt.Sprintf("userdata<%T>", ud.Value)
		}
	}
	return val.Type().String()
}

func cloneLuaValue(L *lua.LState, val lua.LValue) lua.LValue {
	if val == nil {
		return lua.LNil
	}
	tbl, ok := val.(*lua.LTable)
	if !ok {
		return val
	}
	return cloneLuaTable(L, tbl, make(map[*lua.LTable]*lua.LTable))
}

func cloneLuaTable(L *lua.LState, src *lua.LTable, seen map[*lua.LTable]*lua.LTable) *lua.LTable {
	if src == nil {
		return nil
	}
	if existing, ok := seen[src]; ok {
		return existing
	}
	dst := L.NewTable()
	seen[src] = dst
	src.ForEach(func(k, v lua.LValue) {
		ck := k
		if kt, ok := k.(*lua.LTable); ok {
			ck = cloneLuaTable(L, kt, seen)
		}
		cv := v
		if vt, ok := v.(*lua.LTable); ok {
			cv = cloneLuaTable(L, vt, seen)
		}
		dst.RawSet(ck, cv)
	})
	return dst
}

// ScriptLoadError describes a single script file that failed during LoadScriptsDir.
type ScriptLoadError struct {
	Path string
	Err  error
}

func (e ScriptLoadError) Error() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

func (e ScriptLoadError) Unwrap() error {
	return e.Err
}

// ScriptLoadErrors aggregates non-fatal script loading failures so remaining scripts still load.
type ScriptLoadErrors []ScriptLoadError

func (e ScriptLoadErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e))
	for _, item := range e {
		parts = append(parts, item.Error())
	}
	return fmt.Sprintf("luabridge: %d script(s) failed to load: %s", len(e), strings.Join(parts, "; "))
}

func (e ScriptLoadErrors) Unwrap() []error {
	out := make([]error, 0, len(e))
	for _, item := range e {
		out = append(out, item.Err)
	}
	return out
}

// LoadScriptsDir recursively loads *.lua scripts under dir (skipping scripts/ui LuaJIT FFI
// and scripts/props PropManifest directories) so scripts can register custom Component types.
// If a script fails to compile or execute, the error is recorded gracefully via RecordScriptError
// and loading continues for all remaining scripts.
func (b *LuaBridge) LoadScriptsDir(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("luabridge: %q is not a directory", dir)
	}

	var files []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != dir {
				base := strings.ToLower(d.Name())
				if base == "ui" || base == "props" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".lua") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	loaded := make([]string, 0, len(files))
	var loadErrs ScriptLoadErrors
	b.stateMu.Lock()
	prevLoading := b.loadingScriptPath
	for _, f := range files {
		b.loadingScriptPath = f
		if err := b.L.DoFile(f); err != nil {
			loadErrs = append(loadErrs, ScriptLoadError{Path: f, Err: err})
			b.RecordScriptError(f, "load", err)
			continue
		}
		loaded = append(loaded, f)
	}
	b.loadingScriptPath = prevLoading
	b.stateMu.Unlock()

	if len(loadErrs) > 0 {
		return loaded, loadErrs
	}
	return loaded, nil
}
