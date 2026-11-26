package luabridge

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

type vec3JSON struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

type transformJSON struct {
	Position vec3JSON `json:"position"`
	Rotation vec3JSON `json:"rotation"`
	Scale    vec3JSON `json:"scale"`
}

type componentSummaryJSON struct {
	Type    string         `json:"type"`
	Details map[string]any `json:"details,omitempty"`
}

type gameObjectJSON struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Active     bool                   `json:"active"`
	Spawned    bool                   `json:"spawned"`
	ParentID   string                 `json:"parent_id,omitempty"`
	Children   []string               `json:"children,omitempty"`
	Components []componentSummaryJSON `json:"components"`
}

// SerializeGameObjectJSON formats an engine.GameObject (identity, hierarchy, components) as indented JSON.
func SerializeGameObjectJSON(obj *engine.GameObject) string {
	if obj == nil {
		return "null"
	}

	parentID := ""
	if obj.Parent != nil {
		parentID = obj.Parent.ID
	}

	children := make([]string, 0, len(obj.Children))
	for _, ch := range obj.Children {
		if ch != nil {
			children = append(children, ch.ID)
		}
	}

	comps := make([]componentSummaryJSON, 0, len(obj.Components))
	for _, c := range obj.Components {
		if c == nil {
			continue
		}
		summary := componentSummaryJSON{
			Type: reflect.TypeOf(c).String(),
		}
		switch typed := c.(type) {
		case *engine.TransformComponent:
			p := typed.GetPosition()
			r := typed.GetRotation()
			s := typed.GetScale()
			summary.Details = map[string]any{
				"position": vec3JSON{X: p.X, Y: p.Y, Z: p.Z},
				"rotation": vec3JSON{X: r.X, Y: r.Y, Z: r.Z},
				"scale":    vec3JSON{X: s.X, Y: s.Y, Z: s.Z},
			}
		case *engine.MeshComponent:
			summary.Details = map[string]any{
				"file_path": typed.FilePath,
				"triangles": typed.TriangleCount(),
				"loaded":    typed.Asset != nil,
			}
		case *engine.MeshRenderable:
			summary.Details = map[string]any{
				"visible":   typed.Visible,
				"wireframe": typed.Wireframe,
				"valid":     typed.ValidateConstraint() == nil,
			}
		case *engine.Renderable:
			summary.Details = map[string]any{
				"mesh_id":     typed.MeshID,
				"material_id": typed.MaterialID,
				"visible":     typed.IsVisible(),
				"outlined":    typed.IsOutlined(),
				"flags":       uint32(typed.Flags),
			}
		case *engine.Renderable2D:
			summary.Details = map[string]any{
				"texture_id": typed.TextureID,
				"width":      typed.Size.X,
				"height":     typed.Size.Y,
				"z_index":    typed.ZIndex,
				"visible":    typed.IsVisible(),
			}
		case *spatial.RigidBody:
			summary.Details = map[string]any{
				"id":        typed.ID,
				"body_id":   typed.BodyID,
				"body_type": typed.BodyType,
				"mass":      typed.Mass,
			}
		case *spatial.InteractiveComponent:
			summary.Details = map[string]any{
				"display_name": typed.DisplayName,
				"enabled":      typed.Enabled,
				"hovered":      typed.Hovered,
				"held":         typed.Held,
			}
		case *spatial.FractureComponent:
			summary.Details = map[string]any{
				"health":      typed.Health,
				"mode":        typed.FractureMode,
				"carry_hands": typed.RequiredCarryHands,
				"broken":      typed.Broken,
			}
		}
		comps = append(comps, summary)
	}

	payload := gameObjectJSON{
		ID:         obj.ID,
		Name:       obj.Name,
		Active:     obj.Active,
		Spawned:    obj.IsSpawned(),
		ParentID:   parentID,
		Children:   children,
		Components: comps,
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("GameObject(%s:%s)", obj.ID, obj.Name)
	}
	return string(raw)
}

// SerializeLuaValue converts a Lua return value into a human-readable serialized representation
// (pretty-printed JSON for GameObjects, formatted summaries for engine userdata and tables, and "Nil" for nil).
func SerializeLuaValue(L *lua.LState, val lua.LValue) string {
	return serializeLuaValueDepth(L, val, 0, make(map[*lua.LTable]bool))
}

func serializeLuaValueDepth(L *lua.LState, val lua.LValue, depth int, seen map[*lua.LTable]bool) string {
	if val == nil || val == lua.LNil {
		return "Nil"
	}

	switch v := val.(type) {
	case lua.LBool, lua.LNumber, lua.LString:
		return v.String()
	case *lua.LUserData:
		return serializeUserData(L, v)
	case *lua.LTable:
		return serializeLuaTable(L, v, depth, seen)
	default:
		return val.String()
	}
}

func serializeUserData(L *lua.LState, ud *lua.LUserData) string {
	if ud == nil || ud.Value == nil {
		return "Nil"
	}
	switch obj := ud.Value.(type) {
	case *engine.GameObject:
		return SerializeGameObjectJSON(obj)
	case *engine.TransformComponent:
		p, r, s := obj.GetPosition(), obj.GetRotation(), obj.GetScale()
		return fmt.Sprintf("Transform{pos=(%.2f, %.2f, %.2f), rot=(%.2f, %.2f, %.2f), scale=(%.2f, %.2f, %.2f)}",
			p.X, p.Y, p.Z, r.X, r.Y, r.Z, s.X, s.Y, s.Z)
	case *engine.MeshComponent:
		return fmt.Sprintf("Mesh{path=%q, tris=%d, loaded=%v}", obj.FilePath, obj.TriangleCount(), obj.Asset != nil)
	case *engine.MeshRenderable:
		return fmt.Sprintf("MeshRenderable{visible=%v, wireframe=%v, valid=%v}",
			obj.Visible, obj.Wireframe, obj.ValidateConstraint() == nil)
	case *engine.Renderable:
		return fmt.Sprintf("Renderable{mesh=%q, material=%q, visible=%v, outlined=%v, flags=0x%X}",
			obj.MeshID, obj.MaterialID, obj.IsVisible(), obj.IsOutlined(), uint32(obj.Flags))
	case *engine.Renderable2D:
		return fmt.Sprintf("Renderable2D{tex=%d, size=(%.1f, %.1f), z=%d, visible=%v}",
			obj.TextureID, obj.Size.X, obj.Size.Y, obj.ZIndex, obj.IsVisible())
	case *spatial.RigidBody:
		return fmt.Sprintf("RigidBody{id=%q, body_id=%d, mass=%.2f}", obj.ID, obj.BodyID, obj.Mass)
	case *spatial.InteractiveComponent:
		return fmt.Sprintf("InteractiveComponent{name=%q, enabled=%v, hovered=%v, held=%v}",
			obj.DisplayName, obj.Enabled, obj.Hovered, obj.Held)
	case *spatial.FractureComponent:
		return fmt.Sprintf("FractureComponent{health=%.1f, hands=%d, broken=%v}",
			obj.Health, obj.RequiredCarryHands, obj.Broken)
	case engine.Vector3:
		return fmt.Sprintf("Vector3(%.3f, %.3f, %.3f)", obj.X, obj.Y, obj.Z)
	case *engine.BaseScene:
		return fmt.Sprintf("Scene{name=%q}", obj.GetName())
	}

	if L != nil {
		if strVal := L.ToStringMeta(ud); strVal != lua.LNil {
			s := strVal.String()
			if !strings.HasPrefix(s, "userdata:") {
				return s
			}
		}
	}
	return fmt.Sprintf("userdata<%T>", ud.Value)
}

func serializeLuaTable(L *lua.LState, tbl *lua.LTable, depth int, seen map[*lua.LTable]bool) string {
	if tbl == nil {
		return "{}"
	}
	if seen[tbl] {
		return "{<cycle>}"
	}
	if depth >= 4 {
		return "{...}"
	}
	seen[tbl] = true
	defer delete(seen, tbl)

	type kv struct {
		key string
		val string
		num bool
		idx int
	}
	var items []kv
	tbl.ForEach(func(k, v lua.LValue) {
		valStr := serializeLuaValueDepth(L, v, depth+1, seen)
		if ks, ok := k.(lua.LString); ok {
			items = append(items, kv{key: string(ks), val: valStr})
		} else if kn, ok := k.(lua.LNumber); ok {
			items = append(items, kv{key: kn.String(), val: valStr, num: true, idx: int(kn)})
		} else {
			items = append(items, kv{key: k.String(), val: valStr})
		}
	})

	if len(items) == 0 {
		return "{}"
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].num != items[j].num {
			return items[i].num
		}
		if items[i].num {
			return items[i].idx < items[j].idx
		}
		return items[i].key < items[j].key
	})

	parts := make([]string, 0, len(items))
	allSeq := true
	for i, it := range items {
		if !it.num || it.idx != i+1 {
			allSeq = false
			break
		}
	}
	for _, it := range items {
		if allSeq {
			parts = append(parts, it.val)
		} else {
			parts = append(parts, fmt.Sprintf("%s = %s", it.key, it.val))
		}
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}
