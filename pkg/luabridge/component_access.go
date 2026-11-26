package luabridge

import (
	"fmt"
	"reflect"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

const (
	luaComponentClassTypeName  = "ComponentClass"
	luaTransformTypeName       = "Transform"
	luaMeshTypeName            = "Mesh"
	luaMeshRenderableTypeName  = "MeshRenderable"
	luaRigidBodyTypeName       = "RigidBody"
	luaScriptComponentTypeName = "ScriptComponent"
)

// ComponentClass represents a Lua read-only component class constant (e.g., Mesh, Transform).
type ComponentClass struct {
	Name   string
	GoType string
}

func (b *LuaBridge) registerComponentTypes() {
	b.registerComponentClassGlobals()
	b.registerTransformComponentType()
	b.registerMeshComponentType()
	b.registerMeshRenderableType()
	b.registerRigidBodyType()
	b.registerScriptComponentType()
}

func (b *LuaBridge) registerComponentClassGlobals() {
	mt := b.L.NewTypeMetatable(luaComponentClassTypeName)
	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		cc := checkComponentClass(L, 1)
		key := L.CheckString(2)
		switch key {
		case "name", "Name":
			L.Push(lua.LString(cc.Name))
		case "go_type", "GoType":
			L.Push(lua.LString(cc.GoType))
		case "help", "Help":
			L.Push(L.NewFunction(func(L2 *lua.LState) int {
				c := checkComponentClass(L2, 1)
				if txt, ok := FormatComponentHelp(c.Name); ok {
					L2.Push(lua.LString(txt))
				} else {
					L2.Push(lua.LString(fmt.Sprintf("%s (%s)", c.Name, c.GoType)))
				}
				return 1
			}))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
	b.L.SetField(mt, "__newindex", b.L.NewFunction(func(L *lua.LState) int {
		cc := checkComponentClass(L, 1)
		L.RaiseError("component class %s is read-only", cc.Name)
		return 0
	}))
	b.L.SetField(mt, "__eq", b.L.NewFunction(func(L *lua.LState) int {
		ud1, ok1 := L.Get(1).(*lua.LUserData)
		ud2, ok2 := L.Get(2).(*lua.LUserData)
		if !ok1 || !ok2 {
			L.Push(lua.LFalse)
			return 1
		}
		c1, _ := ud1.Value.(*ComponentClass)
		c2, _ := ud2.Value.(*ComponentClass)
		L.Push(lua.LBool(c1 != nil && c2 != nil && c1.Name == c2.Name))
		return 1
	}))
	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		cc := checkComponentClass(L, 1)
		L.Push(lua.LString(fmt.Sprintf("Class<%s (%s)>", cc.Name, cc.GoType)))
		return 1
	}))

	classDefs := []struct {
		globalNames []string
		canonName   string
		goType      string
	}{
		{[]string{"Transform", "TransformComponent"}, "Transform", "*engine.TransformComponent"},
		{[]string{"Mesh", "MeshComponent"}, "Mesh", "*engine.MeshComponent"},
		{[]string{"MeshRenderable"}, "MeshRenderable", "*engine.MeshRenderable"},
		{[]string{"Renderable"}, "Renderable", "*engine.Renderable"},
		{[]string{"Renderable2D"}, "Renderable2D", "*engine.Renderable2D"},
		{[]string{"RigidBody"}, "RigidBody", "*spatial.RigidBody"},
		{[]string{"CharacterController"}, "CharacterController", "*spatial.CharacterController"},
		{[]string{"InteractiveComponent", "Interactive"}, "InteractiveComponent", "*spatial.InteractiveComponent"},
		{[]string{"FractureComponent", "Fracture"}, "FractureComponent", "*spatial.FractureComponent"},
		{[]string{"ScriptComponent"}, "ScriptComponent", "*luabridge.ScriptComponent"},
	}

	for _, def := range classDefs {
		cc := &ComponentClass{Name: def.canonName, GoType: def.goType}
		ud := b.L.NewUserData()
		ud.Value = cc
		b.L.SetMetatable(ud, mt)
		for _, gName := range def.globalNames {
			b.L.SetGlobal(gName, ud)
		}
	}
}

func checkComponentClass(L *lua.LState, idx int) *ComponentClass {
	ud := L.CheckUserData(idx)
	if cc, ok := ud.Value.(*ComponentClass); ok && cc != nil {
		return cc
	}
	L.ArgError(idx, "ComponentClass expected")
	return nil
}

func resolveComponentArg(L *lua.LState, idx int) string {
	val := L.Get(idx)
	if ud, ok := val.(*lua.LUserData); ok && ud != nil {
		if cc, ok := ud.Value.(*ComponentClass); ok && cc != nil {
			return cc.Name
		}
		L.ArgError(idx, "component class constant or type name string expected")
		return ""
	}
	if s, ok := val.(lua.LString); ok {
		return resolveComponentAlias(string(s))
	}
	L.ArgError(idx, "component class constant or type name string expected")
	return ""
}

func (b *LuaBridge) registerTransformComponentType() {
	mt := b.L.NewTypeMetatable(luaTransformTypeName)

	methods := b.L.NewTable()
	for name, fn := range transformComponentMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		tc := checkTransformComponent(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		pos := tc.GetPosition()
		switch key {
		case "position", "Position", "location", "Location":
			L.Push(vec3ToTable(L, pos))
		case "rotation", "Rotation":
			L.Push(vec3ToTable(L, tc.GetRotation()))
		case "scale", "Scale":
			L.Push(vec3ToTable(L, tc.GetScale()))
		case "x", "X":
			L.Push(lua.LNumber(pos.X))
		case "y", "Y":
			L.Push(lua.LNumber(pos.Y))
		case "z", "Z":
			L.Push(lua.LNumber(pos.Z))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))

	b.L.SetField(mt, "__newindex", b.L.NewFunction(func(L *lua.LState) int {
		tc := checkTransformComponent(L, 1)
		key := L.CheckString(2)
		switch key {
		case "position", "Position", "location", "Location":
			tc.SetPosition(tableToVec3(L, L.CheckTable(3)))
		case "rotation", "Rotation":
			tc.SetRotation(tableToVec3(L, L.CheckTable(3)))
		case "scale", "Scale":
			tc.SetScale(tableToVec3(L, L.CheckTable(3)))
		case "x", "X":
			p := tc.GetPosition()
			p.X = float32(L.CheckNumber(3))
			tc.SetPosition(p)
		case "y", "Y":
			p := tc.GetPosition()
			p.Y = float32(L.CheckNumber(3))
			tc.SetPosition(p)
		case "z", "Z":
			p := tc.GetPosition()
			p.Z = float32(L.CheckNumber(3))
			tc.SetPosition(p)
		default:
			L.RaiseError("Transform has no writable property %q", key)
		}
		return 0
	}))

	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		tc := checkTransformComponent(L, 1)
		L.Push(lua.LString(SerializeComponentJSON(tc)))
		return 1
	}))
}

func transformComponentMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"GetPosition": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			p := tc.GetPosition()
			L.Push(lua.LNumber(p.X))
			L.Push(lua.LNumber(p.Y))
			L.Push(lua.LNumber(p.Z))
			return 3
		},
		"SetPosition": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			tc.SetPosition(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"GetLocation": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			L.Push(vec3ToTable(L, tc.GetPosition()))
			return 1
		},
		"SetLocation": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			tc.SetPosition(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"Translate": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			tc.Translate(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"GetRotation": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			r := tc.GetRotation()
			L.Push(lua.LNumber(r.X))
			L.Push(lua.LNumber(r.Y))
			L.Push(lua.LNumber(r.Z))
			return 3
		},
		"SetRotation": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			tc.SetRotation(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"GetScale": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			s := tc.GetScale()
			L.Push(lua.LNumber(s.X))
			L.Push(lua.LNumber(s.Y))
			L.Push(lua.LNumber(s.Z))
			return 3
		},
		"SetScale": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			tc.SetScale(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"GetWorldPosition": func(L *lua.LState) int {
			tc := checkTransformComponent(L, 1)
			wp := tc.WorldTransform().Position
			L.Push(lua.LNumber(wp.X))
			L.Push(lua.LNumber(wp.Y))
			L.Push(lua.LNumber(wp.Z))
			return 3
		},
	}
}

func (b *LuaBridge) registerRigidBodyType() {
	mt := b.L.NewTypeMetatable(luaRigidBodyTypeName)
	methods := b.L.NewTable()
	for name, fn := range rigidBodyMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		rb := checkRigidBody(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		switch key {
		case "id", "ID":
			L.Push(lua.LString(rb.ID))
		case "mass", "Mass":
			L.Push(lua.LNumber(rb.Mass))
		case "body_id", "BodyID":
			L.Push(lua.LNumber(rb.BodyID))
		case "active", "Active":
			L.Push(lua.LBool(rb.IsActive()))
		case "grounded", "Grounded":
			L.Push(lua.LBool(rb.IsGrounded()))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))

	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		rb := checkRigidBody(L, 1)
		L.Push(lua.LString(SerializeComponentJSON(rb)))
		return 1
	}))
}

func rigidBodyMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"GetID": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			L.Push(lua.LString(rb.ID))
			return 1
		},
		"GetMass": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			L.Push(lua.LNumber(rb.Mass))
			return 1
		},
		"GetBodyType": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			L.Push(lua.LNumber(rb.BodyType))
			return 1
		},
		"GetPosition": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			p := rb.GetPosition()
			L.Push(lua.LNumber(p.X))
			L.Push(lua.LNumber(p.Y))
			L.Push(lua.LNumber(p.Z))
			return 3
		},
		"SetPosition": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			rb.SetPosition(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"GetVelocity": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			v := rb.GetVelocity()
			L.Push(lua.LNumber(v.X))
			L.Push(lua.LNumber(v.Y))
			L.Push(lua.LNumber(v.Z))
			return 3
		},
		"SetVelocity": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			rb.SetVelocity(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"AddImpulse": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			rb.AddImpulse(engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4))))
			return 0
		},
		"IsActive": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			L.Push(lua.LBool(rb.IsActive()))
			return 1
		},
		"IsGrounded": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			L.Push(lua.LBool(rb.IsGrounded()))
			return 1
		},
		"GetUserData": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			L.Push(lua.LNumber(rb.GetUserData()))
			return 1
		},
		"SetUserData": func(L *lua.LState) int {
			rb := checkRigidBody(L, 1)
			rb.SetUserData(uint32(L.CheckInt(2)))
			return 0
		},
	}
}

func (b *LuaBridge) registerScriptComponentType() {
	mt := b.L.NewTypeMetatable(luaScriptComponentTypeName)
	methods := b.L.NewTable()
	b.L.SetField(methods, "GetScriptPath", b.L.NewFunction(func(L *lua.LState) int {
		sc := checkScriptComponent(L, 1)
		L.Push(lua.LString(sc.ScriptPath))
		return 1
	}))
	b.L.SetField(methods, "GetScriptName", b.L.NewFunction(func(L *lua.LState) int {
		sc := checkScriptComponent(L, 1)
		L.Push(lua.LString(sc.ScriptName))
		return 1
	}))

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		sc := checkScriptComponent(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		switch key {
		case "script_path", "ScriptPath":
			L.Push(lua.LString(sc.ScriptPath))
		case "script_name", "ScriptName":
			L.Push(lua.LString(sc.ScriptName))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
}

func resolveComponentAlias(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "transform", "transformcomponent", "*engine.transformcomponent":
		return "Transform"
	case "mesh", "meshcomponent", "*engine.meshcomponent":
		return "Mesh"
	case "meshrenderable", "mesh_renderable", "*engine.meshrenderable":
		return "MeshRenderable"
	case "renderable", "*engine.renderable":
		return "Renderable"
	case "renderable2d", "*engine.renderable2d":
		return "Renderable2D"
	case "rigidbody", "rigidbodycomponent", "*spatial.rigidbody":
		return "RigidBody"
	case "charactercontroller", "*spatial.charactercontroller":
		return "CharacterController"
	case "interactive", "interactivecomponent", "*spatial.interactivecomponent":
		return "InteractiveComponent"
	case "fracture", "fracturecomponent", "*spatial.fracturecomponent":
		return "FractureComponent"
	case "script", "scriptcomponent", "*luabridge.scriptcomponent":
		return "ScriptComponent"
	default:
		return strings.TrimSpace(name)
	}
}

func componentReflectType(name string) reflect.Type {
	switch resolveComponentAlias(name) {
	case "Transform":
		return reflect.TypeOf((*engine.TransformComponent)(nil))
	case "Mesh":
		return reflect.TypeOf((*engine.MeshComponent)(nil))
	case "MeshRenderable":
		return reflect.TypeOf((*engine.MeshRenderable)(nil))
	case "Renderable":
		return reflect.TypeOf((*engine.Renderable)(nil))
	case "Renderable2D":
		return reflect.TypeOf((*engine.Renderable2D)(nil))
	case "RigidBody":
		return reflect.TypeOf((*spatial.RigidBody)(nil))
	case "CharacterController":
		return reflect.TypeOf((*spatial.CharacterController)(nil))
	case "InteractiveComponent":
		return reflect.TypeOf((*spatial.InteractiveComponent)(nil))
	case "FractureComponent":
		return reflect.TypeOf((*spatial.FractureComponent)(nil))
	case "ScriptComponent":
		return reflect.TypeOf((*ScriptComponent)(nil))
	default:
		return nil
	}
}

func componentCanonicalName(comp engine.Component) string {
	switch comp.(type) {
	case *engine.TransformComponent:
		return "Transform"
	case *engine.MeshComponent:
		return "Mesh"
	case *engine.MeshRenderable:
		return "MeshRenderable"
	case *engine.Renderable:
		return "Renderable"
	case *engine.Renderable2D:
		return "Renderable2D"
	case *spatial.RigidBody:
		return "RigidBody"
	case *spatial.CharacterController:
		return "CharacterController"
	case *spatial.InteractiveComponent:
		return "InteractiveComponent"
	case *spatial.FractureComponent:
		return "FractureComponent"
	case *ScriptComponent:
		return "ScriptComponent"
	default:
		return reflect.TypeOf(comp).String()
	}
}

func pushEngineComponent(L *lua.LState, comp engine.Component) bool {
	switch c := comp.(type) {
	case *engine.TransformComponent:
		pushTransformComponent(L, c)
		return true
	case *engine.MeshComponent:
		pushMeshComponent(L, c)
		return true
	case *engine.MeshRenderable:
		pushMeshRenderable(L, c)
		return true
	case *engine.Renderable:
		pushRenderable(L, c)
		return true
	case *engine.Renderable2D:
		pushRenderable2D(L, c)
		return true
	case *spatial.RigidBody:
		pushRigidBody(L, c)
		return true
	case *spatial.InteractiveComponent:
		pushInteractive(L, c)
		return true
	case *spatial.FractureComponent:
		pushFracture(L, c)
		return true
	case *ScriptComponent:
		pushScriptComponent(L, c)
		return true
	default:
		L.Push(lua.LNil)
		return false
	}
}

func (b *LuaBridge) registerMeshComponentType() {
	mt := b.L.NewTypeMetatable(luaMeshTypeName)
	methods := b.L.NewTable()
	b.L.SetField(methods, "Load", b.L.NewFunction(func(L *lua.LState) int {
		mc := checkMeshComponent(L, 1)
		if err := mc.Load(L.CheckString(2)); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}))
	b.L.SetField(methods, "GetPath", b.L.NewFunction(func(L *lua.LState) int {
		mc := checkMeshComponent(L, 1)
		L.Push(lua.LString(mc.FilePath))
		return 1
	}))
	b.L.SetField(methods, "GetTriangleCount", b.L.NewFunction(func(L *lua.LState) int {
		mc := checkMeshComponent(L, 1)
		L.Push(lua.LNumber(mc.TriangleCount()))
		return 1
	}))
	b.L.SetField(methods, "GetBounds", b.L.NewFunction(func(L *lua.LState) int {
		mc := checkMeshComponent(L, 1)
		bounds := mc.Bounds()
		tbl := L.NewTable()
		L.SetField(tbl, "min", vec3ToTable(L, bounds.Min))
		L.SetField(tbl, "max", vec3ToTable(L, bounds.Max))
		L.Push(tbl)
		return 1
	}))

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		mc := checkMeshComponent(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		switch key {
		case "file_path", "FilePath":
			L.Push(lua.LString(mc.FilePath))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		mc := checkMeshComponent(L, 1)
		L.Push(lua.LString(SerializeComponentJSON(mc)))
		return 1
	}))
}

func (b *LuaBridge) registerMeshRenderableType() {
	mt := b.L.NewTypeMetatable(luaMeshRenderableTypeName)
	methods := b.L.NewTable()
	b.L.SetField(methods, "SetVisible", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		mr.Visible = L.CheckBool(2)
		return 0
	}))
	b.L.SetField(methods, "IsVisible", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		L.Push(lua.LBool(mr.Visible))
		return 1
	}))
	b.L.SetField(methods, "SetWireframe", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		mr.Wireframe = L.CheckBool(2)
		return 0
	}))
	b.L.SetField(methods, "IsWireframe", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		L.Push(lua.LBool(mr.Wireframe))
		return 1
	}))
	b.L.SetField(methods, "ValidateConstraint", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		if err := mr.ValidateConstraint(); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}))
	b.L.SetField(methods, "GetTransform", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		if mr.Owner != nil {
			pushTransformComponent(L, mr.Owner.GetTransformComponent())
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))
	b.L.SetField(methods, "GetMesh", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		if mr.Owner != nil {
			pushMeshComponent(L, mr.Owner.GetMeshComponent())
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		switch key {
		case "visible", "Visible":
			L.Push(lua.LBool(mr.Visible))
		case "wireframe", "Wireframe":
			L.Push(lua.LBool(mr.Wireframe))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
	b.L.SetField(mt, "__tostring", b.L.NewFunction(func(L *lua.LState) int {
		mr := checkMeshRenderable(L, 1)
		L.Push(lua.LString(SerializeComponentJSON(mr)))
		return 1
	}))
}

func pushMeshComponent(L *lua.LState, mc *engine.MeshComponent) {
	if mc == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = mc
	L.SetMetatable(ud, L.GetTypeMetatable(luaMeshTypeName))
	L.Push(ud)
}

func checkMeshComponent(L *lua.LState, idx int) *engine.MeshComponent {
	ud := L.CheckUserData(idx)
	if mc, ok := ud.Value.(*engine.MeshComponent); ok && mc != nil {
		return mc
	}
	L.ArgError(idx, "Mesh component expected")
	return nil
}

func pushMeshRenderable(L *lua.LState, mr *engine.MeshRenderable) {
	if mr == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = mr
	L.SetMetatable(ud, L.GetTypeMetatable(luaMeshRenderableTypeName))
	L.Push(ud)
}

func checkMeshRenderable(L *lua.LState, idx int) *engine.MeshRenderable {
	ud := L.CheckUserData(idx)
	if mr, ok := ud.Value.(*engine.MeshRenderable); ok && mr != nil {
		return mr
	}
	L.ArgError(idx, "MeshRenderable component expected")
	return nil
}

func findComponentByName(obj *engine.GameObject, name string) engine.Component {
	if obj == nil {
		return nil
	}
	rt := componentReflectType(name)
	if rt == nil {
		return nil
	}
	return obj.GetComponent(rt)
}

func pushTransformComponent(L *lua.LState, tc *engine.TransformComponent) {
	if tc == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = tc
	L.SetMetatable(ud, L.GetTypeMetatable(luaTransformTypeName))
	L.Push(ud)
}

func checkTransformComponent(L *lua.LState, idx int) *engine.TransformComponent {
	ud := L.CheckUserData(idx)
	if tc, ok := ud.Value.(*engine.TransformComponent); ok && tc != nil {
		return tc
	}
	L.ArgError(idx, "Transform component expected")
	return nil
}

func pushRigidBody(L *lua.LState, rb *spatial.RigidBody) {
	if rb == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = rb
	L.SetMetatable(ud, L.GetTypeMetatable(luaRigidBodyTypeName))
	L.Push(ud)
}

func checkRigidBody(L *lua.LState, idx int) *spatial.RigidBody {
	ud := L.CheckUserData(idx)
	if rb, ok := ud.Value.(*spatial.RigidBody); ok && rb != nil {
		return rb
	}
	L.ArgError(idx, "RigidBody expected")
	return nil
}

func pushScriptComponent(L *lua.LState, sc *ScriptComponent) {
	if sc == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = sc
	L.SetMetatable(ud, L.GetTypeMetatable(luaScriptComponentTypeName))
	L.Push(ud)
}

func checkScriptComponent(L *lua.LState, idx int) *ScriptComponent {
	ud := L.CheckUserData(idx)
	if sc, ok := ud.Value.(*ScriptComponent); ok && sc != nil {
		return sc
	}
	L.ArgError(idx, "ScriptComponent expected")
	return nil
}
