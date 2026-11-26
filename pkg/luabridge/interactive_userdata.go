package luabridge

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
	"github.com/SamSoupSauce/project-engine/pkg/interaction"
)

const luaInteractiveTypeName = "InteractiveComponent"

func (b *LuaBridge) registerInteractiveType() {
	mt := b.L.NewTypeMetatable(luaInteractiveTypeName)

	methods := b.L.NewTable()
	for name, fn := range interactiveMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		_ = checkInteractive(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	verbs := b.L.NewTable()
	b.L.SetField(verbs, "None", lua.LNumber(spatial.VerbNone))
	b.L.SetField(verbs, "Grab", lua.LNumber(spatial.VerbGrab))
	b.L.SetField(verbs, "Throw", lua.LNumber(spatial.VerbThrow))
	b.L.SetField(verbs, "OpenInventory", lua.LNumber(spatial.VerbOpenInventory))
	b.L.SetField(verbs, "ToggleState", lua.LNumber(spatial.VerbToggleState))
	b.L.SetField(verbs, "StashInventory", lua.LNumber(spatial.VerbStashInventory))
	b.L.SetField(verbs, "Dismantle", lua.LNumber(spatial.VerbDismantle))
	b.L.SetField(verbs, "Custom", lua.LNumber(spatial.VerbCustomLua))
	b.L.SetGlobal("InteractionVerb", verbs)
}

func interactiveMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"SetEnabled": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			ic.Enabled = L.CheckBool(2)
			if !ic.Enabled {
				ic.SetHovered(false)
			}
			return 0
		},
		"IsEnabled": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			L.Push(lua.LBool(ic.Enabled))
			return 1
		},
		"SetDisplayName": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			ic.DisplayName = L.CheckString(2)
			return 0
		},
		"GetDisplayName": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			L.Push(lua.LString(ic.DisplayName))
			return 1
		},
		"SetHighlightTint": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			ic.HighlightTint = [4]float32{
				float32(L.CheckNumber(2)),
				float32(L.CheckNumber(3)),
				float32(L.CheckNumber(4)),
				float32(L.OptNumber(5, 1.0)),
			}
			return 0
		},
		"SetDefaultVerb": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			verb := spatial.VerbID(L.CheckInt(2))
			label := L.CheckString(3)
			hook := L.OptString(4, "")
			ic.SetDefaultVerb(verb, label, hook)
			return 0
		},
		"AddContextVerb": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			verb := spatial.VerbID(L.CheckInt(2))
			label := L.CheckString(3)
			hook := L.OptString(4, "")
			ic.AddContextVerb(verb, label, hook)
			return 0
		},
		"ClearContextVerbs": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			ic.ClearContextVerbs()
			return 0
		},
		"SetHovered": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			ic.SetHovered(L.CheckBool(2))
			return 0
		},
		"IsHovered": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			L.Push(lua.LBool(ic.Hovered))
			return 1
		},
		"IsHeld": func(L *lua.LState) int {
			ic := checkInteractive(L, 1)
			L.Push(lua.LBool(ic.Held))
			return 1
		},
	}
}

// NewInteractionDispatcher builds a LuaVerbDispatcher bound to this LuaBridge LState.
func (b *LuaBridge) NewInteractionDispatcher() interaction.LuaVerbDispatcher {
	return func(target *engine.GameObject, option spatial.InteractionOption) error {
		if b == nil || b.L == nil {
			return fmt.Errorf("luabridge: nil LuaState for interaction verb dispatch")
		}
		hookName := option.LuaHook
		if hookName == "" {
			return nil
		}
		fnVal := b.L.GetGlobal(hookName)
		fn, ok := fnVal.(*lua.LFunction)
		if !ok || fn == nil {
			return fmt.Errorf("luabridge: interaction LuaHook %q not found", hookName)
		}
		pushGameObject(b.L, target)
		ud := b.L.Get(-1)
		b.L.Pop(1)
		return b.L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}, ud, lua.LString(option.Label), lua.LNumber(option.Verb))
	}
}

// BindInteractionController wires this LuaBridge as the Lua verb callback dispatcher for ctrl.
func (b *LuaBridge) BindInteractionController(ctrl *interaction.Controller) {
	if b == nil || ctrl == nil {
		return
	}
	ctrl.OnLuaVerb = b.NewInteractionDispatcher()
}

func pushInteractive(L *lua.LState, ic *spatial.InteractiveComponent) {
	if ic == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = ic
	L.SetMetatable(ud, L.GetTypeMetatable(luaInteractiveTypeName))
	L.Push(ud)
}

func checkInteractive(L *lua.LState, idx int) *spatial.InteractiveComponent {
	ud := L.CheckUserData(idx)
	if ic, ok := ud.Value.(*spatial.InteractiveComponent); ok && ic != nil {
		return ic
	}
	L.ArgError(idx, "InteractiveComponent expected")
	return nil
}
