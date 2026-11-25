package luabridge

import (
	"fmt"
	"reflect"

	rl "github.com/gen2brain/raylib-go/raylib"
	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
)

const (
	luaGameObjectTypeName = "GameObject"
	luaSceneTypeName      = "Scene"
)

func (b *LuaBridge) registerEngineTypes() {
	b.registerGameObjectType()
	b.registerSceneType()

	b.L.SetGlobal("new_game_object", b.L.NewFunction(func(L *lua.LState) int {
		id := L.CheckString(1)
		name := L.OptString(2, id)
		pushGameObject(L, engine.NewGameObject(id, name))
		return 1
	}))
}

func (b *LuaBridge) registerGameObjectType() {
	mt := b.L.NewTypeMetatable(luaGameObjectTypeName)

	methods := b.L.NewTable()
	for name, fn := range gameObjectMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		obj := checkGameObject(L, 1)
		key := L.CheckString(2)

		if fn := methods.RawGetString(key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}

		switch key {
		case "id", "ID":
			L.Push(lua.LString(obj.ID))
		case "name", "Name":
			L.Push(lua.LString(obj.Name))
		case "active", "Active":
			L.Push(lua.LBool(obj.Active))
		case "scene", "Scene":
			pushScene(L, obj.GetScene())
		case "position", "Position":
			L.Push(vec3ToTable(L, obj.Transform.Position))
		case "rotation", "Rotation":
			L.Push(vec3ToTable(L, obj.Transform.Rotation))
		case "scale", "Scale":
			L.Push(vec3ToTable(L, obj.Transform.Scale))
		case "transform", "Transform":
			L.Push(transformToTable(L, obj.Transform))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))

	b.L.SetField(mt, "__newindex", b.L.NewFunction(func(L *lua.LState) int {
		obj := checkGameObject(L, 1)
		key := L.CheckString(2)

		switch key {
		case "name", "Name":
			obj.Name = L.CheckString(3)
		case "active", "Active":
			obj.Active = L.CheckBool(3)
		case "position", "Position":
			obj.Transform.Position = tableToVec3(L, L.CheckTable(3))
		case "rotation", "Rotation":
			obj.Transform.Rotation = tableToVec3(L, L.CheckTable(3))
		case "scale", "Scale":
			obj.Transform.Scale = tableToVec3(L, L.CheckTable(3))
		default:
			L.RaiseError("GameObject has no writable property %q", key)
		}
		return 0
	}))

	b.L.SetField(mt, "__eq", b.L.NewFunction(func(L *lua.LState) int {
		ud1, ok1 := L.Get(1).(*lua.LUserData)
		ud2, ok2 := L.Get(2).(*lua.LUserData)
		if !ok1 || !ok2 {
			L.Push(lua.LFalse)
			return 1
		}
		o1, _ := ud1.Value.(*engine.GameObject)
		o2, _ := ud2.Value.(*engine.GameObject)
		L.Push(lua.LBool(o1 != nil && o1 == o2))
		return 1
	}))

	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		obj := checkGameObject(L, 1)
		L.Push(lua.LString(fmt.Sprintf("GameObject(%s:%s)", obj.ID, obj.Name)))
		return 1
	}))
}

func gameObjectMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"GetID": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(lua.LString(obj.ID))
			return 1
		},
		"GetName": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(lua.LString(obj.Name))
			return 1
		},
		"SetName": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			obj.Name = L.CheckString(2)
			return 0
		},
		"IsActive": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(lua.LBool(obj.Active))
			return 1
		},
		"SetActive": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			obj.Active = L.CheckBool(2)
			return 0
		},
		"GetPosition": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(vec3ToTable(L, obj.Transform.Position))
			return 1
		},
		"SetPosition": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			obj.Transform.Position = parseVec3Args(L, 2)
			return 0
		},
		"GetRotation": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(vec3ToTable(L, obj.Transform.Rotation))
			return 1
		},
		"SetRotation": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			obj.Transform.Rotation = parseVec3Args(L, 2)
			return 0
		},
		"GetScale": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(vec3ToTable(L, obj.Transform.Scale))
			return 1
		},
		"SetScale": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			obj.Transform.Scale = parseVec3Args(L, 2)
			return 0
		},
		"GetTransform": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(transformToTable(L, obj.Transform))
			return 1
		},
		"GetWorldTransform": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			L.Push(transformToTable(L, obj.GetWorldTransform()))
			return 1
		},
		"GetScene": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			pushScene(L, obj.GetScene())
			return 1
		},
		"Emit": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			name := L.CheckString(2)
			var data any
			if L.GetTop() >= 3 && L.Get(3) != lua.LNil {
				data = luaValueToGo(L.Get(3))
			}
			if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
				target := checkGameObject(L, 4)
				obj.EmitTo(target, name, data)
				return 0
			}
			obj.Emit(name, data)
			return 0
		},
		"EmitTo": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			target := checkGameObject(L, 2)
			name := L.CheckString(3)
			var data any
			if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
				data = luaValueToGo(L.Get(4))
			}
			obj.EmitTo(target, name, data)
			return 0
		},
		"Spawn": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			target := checkGameObject(L, 2)
			if sc, ok := obj.GetScene().(engine.SceneController); ok {
				sc.Spawn(target, obj)
			}
			return 0
		},
		"Despawn": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			var sender *engine.GameObject
			if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
				sender = checkGameObject(L, 2)
			}
			var data any
			if L.GetTop() >= 3 && L.Get(3) != lua.LNil {
				data = luaValueToGo(L.Get(3))
			}
			if sc, ok := obj.GetScene().(engine.SceneController); ok {
				sc.Despawn(obj, sender, data)
			}
			return 0
		},
		"AddChild": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			child := checkGameObject(L, 2)
			obj.AddChild(child)
			return 0
		},
		"RemoveChild": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			child := checkGameObject(L, 2)
			L.Push(lua.LBool(obj.RemoveChild(child)))
			return 1
		},
		"GetParent": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			pushGameObject(L, obj.Parent)
			return 1
		},
		"GetChildren": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			tbl := L.NewTable()
			for i, child := range obj.Children {
				tbl.RawSetInt(i+1, gameObjectToUserData(L, child))
			}
			L.Push(tbl)
			return 1
		},
	}
}

func (b *LuaBridge) registerSceneType() {
	mt := b.L.NewTypeMetatable(luaSceneTypeName)

	methods := b.L.NewTable()
	for name, fn := range sceneMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		s := checkScene(L, 1)
		key := L.CheckString(2)

		if fn := methods.RawGetString(key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}

		switch key {
		case "name", "Name":
			if sc, ok := s.(engine.SceneController); ok {
				L.Push(lua.LString(sc.GetName()))
			} else {
				L.Push(lua.LString(""))
			}
		case "loaded", "Loaded":
			if sc, ok := s.(engine.SceneController); ok {
				L.Push(lua.LBool(sc.IsLoaded()))
			} else {
				L.Push(lua.LFalse)
			}
		case "root", "Root":
			if sc, ok := s.(engine.SceneController); ok {
				pushGameObject(L, sc.GetRoot())
			} else {
				L.Push(lua.LNil)
			}
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))

	b.L.SetField(mt, "__eq", b.L.NewFunction(func(L *lua.LState) int {
		ud1, ok1 := L.Get(1).(*lua.LUserData)
		ud2, ok2 := L.Get(2).(*lua.LUserData)
		if !ok1 || !ok2 {
			L.Push(lua.LFalse)
			return 1
		}
		s1, ok1 := ud1.Value.(engine.Scene)
		s2, ok2 := ud2.Value.(engine.Scene)
		L.Push(lua.LBool(ok1 && ok2 && scenesEqual(s1, s2)))
		return 1
	}))

	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		s := checkScene(L, 1)
		name := ""
		if sc, ok := s.(engine.SceneController); ok {
			name = sc.GetName()
		}
		L.Push(lua.LString(fmt.Sprintf("Scene(%s)", name)))
		return 1
	}))
}

func sceneMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"GetName": func(L *lua.LState) int {
			s := checkScene(L, 1)
			if sc, ok := s.(engine.SceneController); ok {
				L.Push(lua.LString(sc.GetName()))
				return 1
			}
			L.Push(lua.LString(""))
			return 1
		},
		"IsLoaded": func(L *lua.LState) int {
			s := checkScene(L, 1)
			if sc, ok := s.(engine.SceneController); ok {
				L.Push(lua.LBool(sc.IsLoaded()))
				return 1
			}
			L.Push(lua.LFalse)
			return 1
		},
		"GetRoot": func(L *lua.LState) int {
			s := checkScene(L, 1)
			if sc, ok := s.(engine.SceneController); ok {
				pushGameObject(L, sc.GetRoot())
				return 1
			}
			L.Push(lua.LNil)
			return 1
		},
		"FindByID": func(L *lua.LState) int {
			s := checkScene(L, 1)
			id := L.CheckString(2)
			if sc, ok := s.(engine.SceneController); ok {
				pushGameObject(L, sc.FindByID(id))
				return 1
			}
			L.Push(lua.LNil)
			return 1
		},
		"FindByName": func(L *lua.LState) int {
			s := checkScene(L, 1)
			name := L.CheckString(2)
			if sc, ok := s.(engine.SceneController); ok {
				pushGameObject(L, sc.FindByName(name))
				return 1
			}
			L.Push(lua.LNil)
			return 1
		},
		"GetObjects": func(L *lua.LState) int {
			s := checkScene(L, 1)
			tbl := L.NewTable()
			if sc, ok := s.(engine.SceneController); ok {
				for i, obj := range sc.GetObjects() {
					tbl.RawSetInt(i+1, gameObjectToUserData(L, obj))
				}
			}
			L.Push(tbl)
			return 1
		},
		"ObjectCount": func(L *lua.LState) int {
			s := checkScene(L, 1)
			if sc, ok := s.(engine.SceneController); ok {
				L.Push(lua.LNumber(sc.ObjectCount()))
				return 1
			}
			L.Push(lua.LNumber(0))
			return 1
		},
		"Spawn": func(L *lua.LState) int {
			s := checkScene(L, 1)
			obj := checkGameObject(L, 2)
			var sender any = s
			if L.GetTop() >= 3 && L.Get(3) != lua.LNil {
				if ud, ok := L.Get(3).(*lua.LUserData); ok {
					switch v := ud.Value.(type) {
					case *engine.GameObject:
						sender = v
					case engine.Scene:
						sender = v
					}
				}
			}
			if sc, ok := s.(engine.SceneController); ok {
				sc.Spawn(obj, sender)
			}
			return 0
		},
		"Despawn": func(L *lua.LState) int {
			s := checkScene(L, 1)
			obj := checkGameObject(L, 2)
			var sender *engine.GameObject
			if L.GetTop() >= 3 && L.Get(3) != lua.LNil {
				sender = checkGameObject(L, 3)
			}
			var data any
			if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
				data = luaValueToGo(L.Get(4))
			}
			if sc, ok := s.(engine.SceneController); ok {
				sc.Despawn(obj, sender, data)
			}
			return 0
		},
		"Emit": func(L *lua.LState) int {
			s := checkScene(L, 1)
			name := L.CheckString(2)
			var sender *engine.GameObject
			if L.GetTop() >= 3 && L.Get(3) != lua.LNil {
				sender = checkGameObject(L, 3)
			}
			var data any
			if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
				data = luaValueToGo(L.Get(4))
			}
			if bus, ok := s.(engine.EventBus); ok {
				bus.Emit(name, sender, data)
			}
			return 0
		},
		"CreateObject": func(L *lua.LState) int {
			_ = checkScene(L, 1)
			id := L.CheckString(2)
			name := L.OptString(3, id)
			pushGameObject(L, engine.NewGameObject(id, name))
			return 1
		},
	}
}

func pushGameObject(L *lua.LState, obj *engine.GameObject) {
	if obj == nil {
		L.Push(lua.LNil)
		return
	}
	L.Push(gameObjectToUserData(L, obj))
}

func gameObjectToUserData(L *lua.LState, obj *engine.GameObject) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = obj
	L.SetMetatable(ud, L.GetTypeMetatable(luaGameObjectTypeName))
	return ud
}

func checkGameObject(L *lua.LState, idx int) *engine.GameObject {
	ud := L.CheckUserData(idx)
	if obj, ok := ud.Value.(*engine.GameObject); ok && obj != nil {
		return obj
	}
	L.ArgError(idx, "GameObject expected")
	return nil
}

func pushScene(L *lua.LState, s engine.Scene) {
	if s == nil {
		L.Push(lua.LNil)
		return
	}
	L.Push(sceneToUserData(L, s))
}

func sceneToUserData(L *lua.LState, s engine.Scene) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = s
	L.SetMetatable(ud, L.GetTypeMetatable(luaSceneTypeName))
	return ud
}

func checkScene(L *lua.LState, idx int) engine.Scene {
	ud := L.CheckUserData(idx)
	if s, ok := ud.Value.(engine.Scene); ok && s != nil {
		return s
	}
	L.ArgError(idx, "Scene expected")
	return nil
}

func scenesEqual(a, b engine.Scene) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}

func vec3ToTable(L *lua.LState, v rl.Vector3) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("x", lua.LNumber(v.X))
	tbl.RawSetString("y", lua.LNumber(v.Y))
	tbl.RawSetString("z", lua.LNumber(v.Z))
	return tbl
}

func tableToVec3(L *lua.LState, tbl *lua.LTable) rl.Vector3 {
	return rl.NewVector3(
		float32(luaNumberField(tbl, "x")),
		float32(luaNumberField(tbl, "y")),
		float32(luaNumberField(tbl, "z")),
	)
}

func parseVec3Args(L *lua.LState, startIdx int) rl.Vector3 {
	if tbl, ok := L.Get(startIdx).(*lua.LTable); ok {
		return tableToVec3(L, tbl)
	}
	x := float32(L.CheckNumber(startIdx))
	y := float32(L.CheckNumber(startIdx + 1))
	z := float32(L.CheckNumber(startIdx + 2))
	return rl.NewVector3(x, y, z)
}

func luaNumberField(tbl *lua.LTable, key string) float64 {
	if n, ok := tbl.RawGetString(key).(lua.LNumber); ok {
		return float64(n)
	}
	return 0
}

func transformToTable(L *lua.LState, t engine.Transform) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("position", vec3ToTable(L, t.Position))
	tbl.RawSetString("rotation", vec3ToTable(L, t.Rotation))
	tbl.RawSetString("scale", vec3ToTable(L, t.Scale))
	return tbl
}

func luaValueToGo(val lua.LValue) any {
	return luaValueToGoSeen(val, make(map[*lua.LTable]bool))
}

func luaValueToGoSeen(val lua.LValue, seen map[*lua.LTable]bool) any {
	switch v := val.(type) {
	case nil, *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LUserData:
		return v.Value
	case *lua.LTable:
		if seen[v] {
			return nil
		}
		seen[v] = true
		defer delete(seen, v)

		arrLen := v.Len()
		totalKeys := 0
		isPureArray := arrLen > 0
		v.ForEach(func(k, _ lua.LValue) {
			totalKeys++
			if n, ok := k.(lua.LNumber); !ok || float64(int(n)) != float64(n) || int(n) < 1 || int(n) > arrLen {
				isPureArray = false
			}
		})

		if isPureArray && totalKeys == arrLen {
			arr := make([]any, arrLen)
			for i := 1; i <= arrLen; i++ {
				arr[i-1] = luaValueToGoSeen(v.RawGetInt(i), seen)
			}
			return arr
		}

		m := make(map[string]any, totalKeys)
		v.ForEach(func(k, item lua.LValue) {
			m[k.String()] = luaValueToGoSeen(item, seen)
		})
		return m
	default:
		return val.String()
	}
}

func goValueToLua(L *lua.LState, val any) lua.LValue {
	switch v := val.(type) {
	case nil:
		return lua.LNil
	case lua.LValue:
		return v
	case bool:
		return lua.LBool(v)
	case int:
		return lua.LNumber(v)
	case int32:
		return lua.LNumber(v)
	case int64:
		return lua.LNumber(v)
	case float32:
		return lua.LNumber(v)
	case float64:
		return lua.LNumber(v)
	case string:
		return lua.LString(v)
	case *engine.GameObject:
		if v == nil {
			return lua.LNil
		}
		return gameObjectToUserData(L, v)
	case engine.Scene:
		if v == nil {
			return lua.LNil
		}
		return sceneToUserData(L, v)
	case []any:
		tbl := L.NewTable()
		for i, item := range v {
			tbl.RawSetInt(i+1, goValueToLua(L, item))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k, item := range v {
			tbl.RawSetString(k, goValueToLua(L, item))
		}
		return tbl
	case map[any]any:
		tbl := L.NewTable()
		for k, item := range v {
			tbl.RawSet(goValueToLua(L, k), goValueToLua(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}
