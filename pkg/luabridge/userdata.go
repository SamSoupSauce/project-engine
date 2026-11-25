package luabridge

import (
	"fmt"
	"reflect"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
)

const (
	luaGameObjectTypeName   = "GameObject"
	luaSceneTypeName        = "Scene"
	luaRenderable2DTypeName = "Renderable2D"
)

func (b *LuaBridge) registerEngineTypes() {
	b.registerGameObjectType()
	b.registerSceneType()
	b.registerRenderable2DType()

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
		"GetRenderable2D": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			pushRenderable2D(L, findRenderable2D(obj))
			return 1
		},
		"AddRenderable2D": func(L *lua.LState) int {
			obj := checkGameObject(L, 1)
			texID := uint32(L.OptInt(2, 0))
			w := float32(L.OptNumber(3, 64))
			h := float32(L.OptNumber(4, 64))
			r2d := engine.NewRenderable2D(texID, w, h)
			obj.AddComponent(r2d)
			pushRenderable2D(L, r2d)
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

func vec3ToTable(L *lua.LState, v engine.Vector3) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("x", lua.LNumber(v.X))
	tbl.RawSetString("y", lua.LNumber(v.Y))
	tbl.RawSetString("z", lua.LNumber(v.Z))
	return tbl
}

func tableToVec3(L *lua.LState, tbl *lua.LTable) engine.Vector3 {
	return engine.NewVector3(
		float32(luaNumberField(tbl, "x")),
		float32(luaNumberField(tbl, "y")),
		float32(luaNumberField(tbl, "z")),
	)
}

func parseVec3Args(L *lua.LState, startIdx int) engine.Vector3 {
	if tbl, ok := L.Get(startIdx).(*lua.LTable); ok {
		return tableToVec3(L, tbl)
	}
	x := float32(L.CheckNumber(startIdx))
	y := float32(L.CheckNumber(startIdx + 1))
	z := float32(L.CheckNumber(startIdx + 2))
	return engine.NewVector3(x, y, z)
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

func (b *LuaBridge) registerRenderable2DType() {
	mt := b.L.NewTypeMetatable(luaRenderable2DTypeName)

	methods := b.L.NewTable()
	for name, fn := range renderable2DMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		r2d := checkRenderable2D(L, 1)
		key := L.CheckString(2)

		if fn := methods.RawGetString(key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}

		switch key {
		case "width", "Width":
			L.Push(lua.LNumber(r2d.Size.X))
		case "height", "Height":
			L.Push(lua.LNumber(r2d.Size.Y))
		case "size", "Size":
			L.Push(vec2ToTable(L, r2d.Size))
		case "color", "Color":
			L.Push(colorToTable(L, r2d.Color))
		case "uv", "source_rect", "SourceRect":
			L.Push(rect2DToTable(L, r2d.SourceRect))
		case "visible", "Visible":
			L.Push(lua.LBool(r2d.IsVisible()))
		case "z_index", "zindex", "ZIndex":
			L.Push(lua.LNumber(r2d.ZIndex))
		case "texture_id", "TextureID":
			L.Push(lua.LNumber(r2d.TextureID))
		case "anchor", "Anchor":
			L.Push(lua.LNumber(r2d.Anchor))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))

	b.L.SetField(mt, "__newindex", b.L.NewFunction(func(L *lua.LState) int {
		r2d := checkRenderable2D(L, 1)
		key := L.CheckString(2)

		switch key {
		case "width", "Width":
			r2d.Size.X = float32(L.CheckNumber(3))
		case "height", "Height":
			r2d.Size.Y = float32(L.CheckNumber(3))
		case "size", "Size":
			r2d.Size = tableToVec2(L.CheckTable(3))
		case "color", "Color":
			r2d.Color = tableToColor(L.CheckTable(3))
		case "uv", "source_rect", "SourceRect":
			r2d.SourceRect = tableToRect2D(L.CheckTable(3))
		case "visible", "Visible":
			r2d.SetVisible(L.CheckBool(3))
		case "z_index", "zindex", "ZIndex":
			r2d.ZIndex = int32(L.CheckInt(3))
		case "texture_id", "TextureID":
			r2d.TextureID = uint32(L.CheckInt(3))
		case "anchor", "Anchor":
			r2d.Anchor = engine.Anchor2D(L.CheckInt(3))
		default:
			L.RaiseError("Renderable2D has no writable property %q", key)
		}
		return 0
	}))
}

func renderable2DMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"SetSize": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.Size = engine.NewVector2(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)))
			return 0
		},
		"GetSize": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(vec2ToTable(L, r2d.Size))
			return 1
		},
		"SetWidth": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.Size.X = float32(L.CheckNumber(2))
			return 0
		},
		"GetWidth": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(lua.LNumber(r2d.Size.X))
			return 1
		},
		"SetHeight": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.Size.Y = float32(L.CheckNumber(2))
			return 0
		},
		"GetHeight": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(lua.LNumber(r2d.Size.Y))
			return 1
		},
		"SetColor": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.Color = engine.NewColor(
				uint8(L.CheckInt(2)),
				uint8(L.CheckInt(3)),
				uint8(L.CheckInt(4)),
				uint8(L.OptInt(5, 255)),
			)
			return 0
		},
		"GetColor": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(colorToTable(L, r2d.Color))
			return 1
		},
		"SetUV": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.SourceRect = engine.Rect2D{
				U: float32(L.CheckNumber(2)),
				V: float32(L.CheckNumber(3)),
				W: float32(L.CheckNumber(4)),
				H: float32(L.CheckNumber(5)),
			}
			return 0
		},
		"GetUV": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(rect2DToTable(L, r2d.SourceRect))
			return 1
		},
		"SetVisible": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.SetVisible(L.CheckBool(2))
			return 0
		},
		"IsVisible": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(lua.LBool(r2d.IsVisible()))
			return 1
		},
		"SetZIndex": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.ZIndex = int32(L.CheckInt(2))
			return 0
		},
		"GetZIndex": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(lua.LNumber(r2d.ZIndex))
			return 1
		},
		"SetTextureID": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.TextureID = uint32(L.CheckInt(2))
			return 0
		},
		"GetTextureID": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(lua.LNumber(r2d.TextureID))
			return 1
		},
		"SetAnchor": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			r2d.Anchor = engine.Anchor2D(L.CheckInt(2))
			return 0
		},
		"GetAnchor": func(L *lua.LState) int {
			r2d := checkRenderable2D(L, 1)
			L.Push(lua.LNumber(r2d.Anchor))
			return 1
		},
	}
}

func findRenderable2D(obj *engine.GameObject) *engine.Renderable2D {
	if obj == nil {
		return nil
	}
	comp := obj.GetComponent(reflect.TypeOf((*engine.Renderable2D)(nil)))
	if r2d, ok := comp.(*engine.Renderable2D); ok {
		return r2d
	}
	return nil
}

func pushRenderable2D(L *lua.LState, r2d *engine.Renderable2D) {
	if r2d == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = r2d
	L.SetMetatable(ud, L.GetTypeMetatable(luaRenderable2DTypeName))
	L.Push(ud)
}

func checkRenderable2D(L *lua.LState, idx int) *engine.Renderable2D {
	ud := L.CheckUserData(idx)
	if r2d, ok := ud.Value.(*engine.Renderable2D); ok && r2d != nil {
		return r2d
	}
	L.ArgError(idx, "Renderable2D expected")
	return nil
}

func vec2ToTable(L *lua.LState, v engine.Vector2) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("width", lua.LNumber(v.X))
	tbl.RawSetString("height", lua.LNumber(v.Y))
	tbl.RawSetString("x", lua.LNumber(v.X))
	tbl.RawSetString("y", lua.LNumber(v.Y))
	return tbl
}

func tableToVec2(tbl *lua.LTable) engine.Vector2 {
	w := float32(luaNumberField(tbl, "width"))
	h := float32(luaNumberField(tbl, "height"))
	if w == 0 && h == 0 {
		w = float32(luaNumberField(tbl, "x"))
		h = float32(luaNumberField(tbl, "y"))
	}
	return engine.NewVector2(w, h)
}

func colorToTable(L *lua.LState, c engine.Color) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("r", lua.LNumber(c.R))
	tbl.RawSetString("g", lua.LNumber(c.G))
	tbl.RawSetString("b", lua.LNumber(c.B))
	tbl.RawSetString("a", lua.LNumber(c.A))
	return tbl
}

func tableToColor(tbl *lua.LTable) engine.Color {
	a := uint8(255)
	if tbl.RawGetString("a") != lua.LNil {
		a = uint8(luaNumberField(tbl, "a"))
	}
	return engine.NewColor(
		uint8(luaNumberField(tbl, "r")),
		uint8(luaNumberField(tbl, "g")),
		uint8(luaNumberField(tbl, "b")),
		a,
	)
}

func rect2DToTable(L *lua.LState, r engine.Rect2D) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("u", lua.LNumber(r.U))
	tbl.RawSetString("v", lua.LNumber(r.V))
	tbl.RawSetString("w", lua.LNumber(r.W))
	tbl.RawSetString("h", lua.LNumber(r.H))
	return tbl
}

func tableToRect2D(tbl *lua.LTable) engine.Rect2D {
	return engine.Rect2D{
		U: float32(luaNumberField(tbl, "u")),
		V: float32(luaNumberField(tbl, "v")),
		W: float32(luaNumberField(tbl, "w")),
		H: float32(luaNumberField(tbl, "h")),
	}
}
