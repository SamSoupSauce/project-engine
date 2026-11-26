package luabridge

import (
	"reflect"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
)

const (
	luaRendererTypeName   = "Renderer"
	luaRenderableTypeName = "Renderable"
)

func (b *LuaBridge) registerRendererType() {
	mt := b.L.NewTypeMetatable(luaRendererTypeName)

	methods := b.L.NewTable()
	for name, fn := range rendererMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		gfx := checkRenderer(L, 1)
		key := L.CheckString(2)

		if fn := methods.RawGetString(key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}

		switch key {
		case "width", "Width":
			L.Push(lua.LNumber(gfx.ViewportW))
		case "height", "Height":
			L.Push(lua.LNumber(gfx.ViewportH))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
}

func rendererMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"GetViewportSize": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			w, h := gfx.ViewportSize()
			L.Push(lua.LNumber(w))
			L.Push(lua.LNumber(h))
			return 2
		},
		"SetViewportSize": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			w := float32(L.CheckNumber(2))
			h := float32(L.CheckNumber(3))
			gfx.SetViewportSize(w, h)
			return 0
		},
		"DrawRect": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			x := float32(L.CheckNumber(2))
			y := float32(L.CheckNumber(3))
			w := float32(L.CheckNumber(4))
			h := float32(L.CheckNumber(5))
			col, nextIdx := parseOptionalColorArgs(L, 6, engine.White)
			zIndex := int32(L.OptInt(nextIdx, 0))
			gfx.DrawRect(x, y, w, h, col, zIndex)
			return 0
		},
		"DrawQuad": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			x := float32(L.CheckNumber(2))
			y := float32(L.CheckNumber(3))
			w := float32(L.CheckNumber(4))
			h := float32(L.CheckNumber(5))
			texID := uint32(L.OptInt(6, 0))
			u := float32(L.OptNumber(7, 0))
			v := float32(L.OptNumber(8, 0))
			uw := float32(L.OptNumber(9, 1))
			vh := float32(L.OptNumber(10, 1))
			col, nextIdx := parseOptionalColorArgs(L, 11, engine.White)
			zIndex := int32(L.OptInt(nextIdx, 0))
			blend := engine.BlendMode2D(L.OptInt(nextIdx+1, int(engine.BlendAlpha)))
			gfx.DrawTexturedQuad(x, y, w, h, texID, engine.Rect2D{U: u, V: v, W: uw, H: vh}, col, zIndex, blend)
			return 0
		},
		"DrawMesh": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			meshID := L.CheckString(2)
			materialID := L.OptString(3, "default")
			tr := engine.NewTransform()
			nextIdx := 4
			if tbl, ok := L.Get(4).(*lua.LTable); ok {
				tr = tableToTransform(tbl)
				nextIdx = 5
			} else if L.GetTop() >= 6 {
				tr.Position = engine.NewVector3(
					float32(L.CheckNumber(4)),
					float32(L.CheckNumber(5)),
					float32(L.CheckNumber(6)),
				)
				nextIdx = 7
			}
			col, afterCol := parseOptionalColorArgs(L, nextIdx, engine.White)
			flags := engine.RenderFlags(L.OptInt(afterCol, int(engine.FlagVisible|engine.FlagCastShadow)))
			gfx.DrawMesh(meshID, materialID, tr, col, flags)
			return 0
		},
		"SubmitDrawCommand": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			tbl := L.CheckTable(2)
			cmd := engine.DrawCommand{
				WorldTransform: engine.NewTransform(),
				MeshID:         luaStringField(tbl, "mesh_id"),
				MaterialID:     luaStringField(tbl, "material_id"),
				Flags:          engine.FlagVisible | engine.FlagCastShadow,
				Color:          engine.White,
				WireColor:      engine.Black,
			}
			if cmd.MaterialID == "" {
				cmd.MaterialID = "default"
			}
			if posTbl, ok := tbl.RawGetString("position").(*lua.LTable); ok {
				cmd.WorldTransform.Position = tableToVec3(L, posTbl)
			}
			if trTbl, ok := tbl.RawGetString("transform").(*lua.LTable); ok {
				cmd.WorldTransform = tableToTransform(trTbl)
			}
			if colTbl, ok := tbl.RawGetString("color").(*lua.LTable); ok {
				cmd.Color = tableToColor(colTbl)
			}
			if wireTbl, ok := tbl.RawGetString("wire_color").(*lua.LTable); ok {
				cmd.WireColor = tableToColor(wireTbl)
			}
			if tbl.RawGetString("flags") != lua.LNil {
				cmd.Flags = engine.RenderFlags(luaNumberField(tbl, "flags"))
			}
			gfx.SubmitDrawCommand(cmd)
			return 0
		},
		"CommandCount3D": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			L.Push(lua.LNumber(len(gfx.Commands3D())))
			return 1
		},
		"CommandCount2D": func(L *lua.LState) int {
			gfx := checkRenderer(L, 1)
			L.Push(lua.LNumber(len(gfx.Commands2D)))
			return 1
		},
	}
}

func (b *LuaBridge) registerRenderableType() {
	mt := b.L.NewTypeMetatable(luaRenderableTypeName)

	methods := b.L.NewTable()
	for name, fn := range renderableMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}

	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		r := checkRenderable(L, 1)
		key := L.CheckString(2)

		if fn := methods.RawGetString(key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}

		switch key {
		case "mesh_id", "MeshID":
			L.Push(lua.LString(r.MeshID))
		case "material_id", "MaterialID":
			L.Push(lua.LString(r.MaterialID))
		case "script_name", "ScriptName":
			L.Push(lua.LString(r.ScriptName))
		case "visible", "Visible":
			L.Push(lua.LBool(r.IsVisible()))
		case "flags", "Flags":
			L.Push(lua.LNumber(r.Flags))
		case "color", "Color":
			L.Push(colorToTable(L, r.Color))
		case "wire_color", "WireColor":
			L.Push(colorToTable(L, r.WireColor))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))

	b.L.SetField(mt, "__newindex", b.L.NewFunction(func(L *lua.LState) int {
		r := checkRenderable(L, 1)
		key := L.CheckString(2)

		switch key {
		case "mesh_id", "MeshID":
			r.MeshID = L.CheckString(3)
		case "material_id", "MaterialID":
			r.MaterialID = L.CheckString(3)
		case "script_name", "ScriptName":
			r.ScriptName = L.CheckString(3)
		case "visible", "Visible":
			r.SetVisible(L.CheckBool(3))
		case "flags", "Flags":
			r.Flags = engine.RenderFlags(L.CheckInt(3))
		case "color", "Color":
			r.Color = tableToColor(L.CheckTable(3))
		case "wire_color", "WireColor":
			r.WireColor = tableToColor(L.CheckTable(3))
		default:
			L.RaiseError("Renderable has no writable property %q", key)
		}
		return 0
	}))
}

func renderableMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"SetVisible": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.SetVisible(L.CheckBool(2))
			return 0
		},
		"IsVisible": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			L.Push(lua.LBool(r.IsVisible()))
			return 1
		},
		"SetScriptName": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.ScriptName = L.CheckString(2)
			return 0
		},
		"GetScriptName": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			L.Push(lua.LString(r.ScriptName))
			return 1
		},
		"SetMeshID": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.MeshID = L.CheckString(2)
			return 0
		},
		"GetMeshID": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			L.Push(lua.LString(r.MeshID))
			return 1
		},
		"SetMaterialID": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.MaterialID = L.CheckString(2)
			return 0
		},
		"GetMaterialID": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			L.Push(lua.LString(r.MaterialID))
			return 1
		},
		"SetColor": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.Color = engine.NewColor(
				uint8(L.CheckInt(2)),
				uint8(L.CheckInt(3)),
				uint8(L.CheckInt(4)),
				uint8(L.OptInt(5, 255)),
			)
			return 0
		},
		"GetColor": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			L.Push(colorToTable(L, r.Color))
			return 1
		},
		"SetFlags": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.Flags = engine.RenderFlags(L.CheckInt(2))
			return 0
		},
		"GetFlags": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			L.Push(lua.LNumber(r.Flags))
			return 1
		},
		"SetOutlined": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.SetOutlined(L.CheckBool(2))
			return 0
		},
		"IsOutlined": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			L.Push(lua.LBool(r.IsOutlined()))
			return 1
		},
		"SetOutlineColor": func(L *lua.LState) int {
			r := checkRenderable(L, 1)
			r.SetOutlineColor(engine.Color{
				R: luaChannelToByte(float64(L.CheckNumber(2))),
				G: luaChannelToByte(float64(L.CheckNumber(3))),
				B: luaChannelToByte(float64(L.CheckNumber(4))),
				A: luaChannelToByte(float64(L.OptNumber(5, 1.0))),
			})
			return 0
		},
	}
}

func luaChannelToByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v <= 1.0 {
		return uint8(v*255.0 + 0.5)
	}
	if v >= 255.0 {
		return 255
	}
	return uint8(v + 0.5)
}

func findRenderable(obj *engine.GameObject) *engine.Renderable {
	if obj == nil {
		return nil
	}
	comp := obj.GetComponent(reflect.TypeOf((*engine.Renderable)(nil)))
	if r, ok := comp.(*engine.Renderable); ok {
		return r
	}
	return nil
}

func pushRenderable(L *lua.LState, r *engine.Renderable) {
	if r == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = r
	L.SetMetatable(ud, L.GetTypeMetatable(luaRenderableTypeName))
	L.Push(ud)
}

func checkRenderable(L *lua.LState, idx int) *engine.Renderable {
	ud := L.CheckUserData(idx)
	if r, ok := ud.Value.(*engine.Renderable); ok && r != nil {
		return r
	}
	L.ArgError(idx, "Renderable expected")
	return nil
}

func pushRenderer(L *lua.LState, gfx *engine.Renderer) {
	if gfx == nil {
		L.Push(lua.LNil)
		return
	}
	L.Push(rendererToUserData(L, gfx))
}

func rendererToUserData(L *lua.LState, gfx *engine.Renderer) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = gfx
	L.SetMetatable(ud, L.GetTypeMetatable(luaRendererTypeName))
	return ud
}

func checkRenderer(L *lua.LState, idx int) *engine.Renderer {
	ud := L.CheckUserData(idx)
	if gfx, ok := ud.Value.(*engine.Renderer); ok && gfx != nil {
		return gfx
	}
	L.ArgError(idx, "Renderer expected")
	return nil
}

func parseOptionalColorArgs(L *lua.LState, startIdx int, def engine.Color) (engine.Color, int) {
	top := L.GetTop()
	if top < startIdx || L.Get(startIdx) == lua.LNil {
		return def, startIdx + 1
	}
	if tbl, ok := L.Get(startIdx).(*lua.LTable); ok {
		return tableToColor(tbl), startIdx + 1
	}
	r := uint8(L.CheckInt(startIdx))
	g := uint8(L.OptInt(startIdx+1, int(def.G)))
	b := uint8(L.OptInt(startIdx+2, int(def.B)))
	a := uint8(L.OptInt(startIdx+3, 255))
	return engine.NewColor(r, g, b, a), startIdx + 4
}

func tableToTransform(tbl *lua.LTable) engine.Transform {
	tr := engine.NewTransform()
	if p, ok := tbl.RawGetString("position").(*lua.LTable); ok {
		tr.Position = engine.NewVector3(
			float32(luaNumberField(p, "x")),
			float32(luaNumberField(p, "y")),
			float32(luaNumberField(p, "z")),
		)
	}
	if r, ok := tbl.RawGetString("rotation").(*lua.LTable); ok {
		tr.Rotation = engine.NewVector3(
			float32(luaNumberField(r, "x")),
			float32(luaNumberField(r, "y")),
			float32(luaNumberField(r, "z")),
		)
	}
	if s, ok := tbl.RawGetString("scale").(*lua.LTable); ok {
		tr.Scale = engine.NewVector3(
			float32(luaNumberField(s, "x")),
			float32(luaNumberField(s, "y")),
			float32(luaNumberField(s, "z")),
		)
	}
	return tr
}

func luaStringField(tbl *lua.LTable, key string) string {
	if s, ok := tbl.RawGetString(key).(lua.LString); ok {
		return string(s)
	}
	return ""
}
