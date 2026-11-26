package luabridge

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

type componentDoc struct {
	Name        string
	GoType      string
	Description string
	Constraint  string
	Fields      []string
	Methods     []string
}

var componentDocs = map[string]componentDoc{
	"Transform": {
		Name:        "Transform",
		GoType:      "*engine.TransformComponent",
		Description: "Stores 3D spatial location, Euler rotation, and scale on a GameObject.",
		Constraint:  "None (required by Renderable components).",
		Fields: []string{
			"position / location  ({x, y, z} table)",
			"rotation             ({x, y, z} Euler degrees table)",
			"scale                ({x, y, z} scale table)",
			"x, y, z              (float32 coordinates)",
		},
		Methods: []string{
			"GetPosition() -> x, y, z",
			"SetPosition(x, y, z)",
			"GetLocation() -> {x, y, z}",
			"SetLocation(x, y, z)",
			"Translate(dx, dy, dz)",
			"GetRotation() -> rx, ry, rz",
			"SetRotation(rx, ry, rz)",
			"GetScale() -> sx, sy, sz",
			"SetScale(sx, sy, sz)",
			"GetWorldPosition() -> wx, wy, wz",
		},
	},
	"Renderable": {
		Name:        "Renderable",
		GoType:      "*engine.Renderable",
		Description: "Pipeline-driven 3D mesh/material rendering component with outline and script hooks.",
		Constraint:  "Requires an attached Transform component (*engine.TransformComponent) on the owning GameObject.",
		Fields: []string{
			"mesh_id / MeshID           (string mesh asset handle)",
			"material_id / MaterialID   (string material handle)",
			"script_name / ScriptName   (string script prefix)",
			"visible                    (bool FlagVisible)",
			"flags                      (uint32 RenderFlags bitmask)",
		},
		Methods: []string{
			"SetVisible(bool)",
			"IsVisible() -> bool",
			"SetMeshID(id) / GetMeshID() -> string",
			"SetMaterialID(id) / GetMaterialID() -> string",
			"SetColor(r, g, b, [a]) / GetColor() -> {r,g,b,a}",
			"SetFlags(mask) / GetFlags() -> int",
			"SetOutlined(bool) / IsOutlined() -> bool",
			"SetOutlineColor(r, g, b, [a])",
			"ValidateConstraint() -> bool, errString",
			"GetTransform() -> Transform",
		},
	},
	"Mesh": {
		Name:        "Mesh",
		GoType:      "*engine.MeshComponent",
		Description: "Loads and holds a glTF 2.0 / GLB 3D mesh asset (vertices, colors, normals, bounds).",
		Constraint:  "None (required by MeshRenderable components).",
		Fields: []string{
			"file_path / FilePath  (string .gltf/.glb path)",
		},
		Methods: []string{
			"Load(filepath) -> bool, errString",
			"GetPath() -> string",
			"GetTriangleCount() -> int",
			"GetBounds() -> {min={x,y,z}, max={x,y,z}}",
		},
	},
	"MeshRenderable": {
		Name:        "MeshRenderable",
		GoType:      "*engine.MeshRenderable",
		Description: "Concrete OpenGL 3.3 Core Renderable component drawing an entity's MeshComponent.",
		Constraint:  "Requires both Transform (*engine.TransformComponent) and loaded Mesh (*engine.MeshComponent).",
		Fields: []string{
			"visible, wireframe",
		},
		Methods: []string{
			"SetVisible(bool) / IsVisible() -> bool",
			"SetWireframe(bool) / IsWireframe() -> bool",
			"ValidateConstraint() -> bool, errString",
			"GetTransform() -> Transform",
			"GetMesh() -> Mesh",
		},
	},
	"Renderable2D": {
		Name:        "Renderable2D",
		GoType:      "*engine.Renderable2D",
		Description: "2D orthographic textured quad HUD component rendered by OrthoBatcher2D.",
		Fields: []string{
			"texture_id, size ({x,y}), source_rect ({u,v,w,h}), anchor, z_index, visible, color",
		},
		Methods: []string{
			"SetVisible(bool) / IsVisible() -> bool",
			"SetSize(w, h) / GetSize() -> w, h",
			"SetColor(r, g, b, [a]) / GetColor() -> {r,g,b,a}",
			"SetSourceRect(u, v, w, h)",
			"SetZIndex(z) / GetZIndex() -> int",
			"SetAnchor(anchor)",
		},
	},
	"RigidBody": {
		Name:        "RigidBody",
		GoType:      "*spatial.RigidBody",
		Description: "Jolt Physics rigid body synchronized back to the owning GameObject.",
		Fields: []string{
			"id / ID, mass / Mass, body_id / BodyID, active, grounded",
		},
		Methods: []string{
			"GetID() -> string",
			"GetMass() -> float",
			"GetBodyType() -> int",
			"GetPosition() -> x, y, z / SetPosition(x, y, z)",
			"GetVelocity() -> vx, vy, vz / SetVelocity(vx, vy, vz)",
			"AddImpulse(ix, iy, iz)",
			"IsActive() -> bool / IsGrounded() -> bool",
			"GetUserData() -> uint32 / SetUserData(val)",
		},
	},
	"CharacterController": {
		Name:        "CharacterController",
		GoType:      "*spatial.CharacterController",
		Description: "Capsule-based character controller for player/NPC movement, grounding, and stairs.",
		Constraint:  "Syncs with an attached TransformComponent on the owning GameObject.",
		Fields: []string{
			"Config.Radius, Config.HalfHeight, Config.MaxSlopeAngleDeg, Config.Mass, Config.Gravity",
		},
		Methods: []string{
			"SetDesiredVelocity(vel)",
			"Jump(speed)",
			"SetPosition(pos)",
			"GetPosition() -> Vector3",
			"IsGrounded() -> bool",
		},
	},
	"InteractiveComponent": {
		Name:        "InteractiveComponent",
		GoType:      "*spatial.InteractiveComponent",
		Description: "Marks an entity interactable on reticle hover with quick-tap & hold context verbs.",
		Fields: []string{
			"Enabled, DisplayName, HighlightTint, Hovered, Held",
		},
		Methods: []string{
			"SetEnabled(bool) / IsEnabled() -> bool",
			"SetDisplayName(name) / GetDisplayName() -> string",
			"SetHighlightTint(r, g, b, [a])",
			"SetDefaultVerb(verbID, label, [luaHook])",
			"AddContextVerb(verbID, label, luaHook)",
			"ClearContextVerbs()",
			"SetHovered(bool) / IsHovered() -> bool",
			"IsHeld() -> bool",
		},
	},
	"FractureComponent": {
		Name:        "FractureComponent",
		GoType:      "*spatial.FractureComponent",
		Description: "Structural integrity, splinter/shatter debris spawning, and heavy carry damping.",
		Fields: []string{
			"Health, Broken, RequiredCarryHands, CameraLookDamping",
		},
		Methods: []string{
			"GetHealth() -> float / SetHealth(hp)",
			"IsBroken() -> bool",
			"GetRequiredCarryHands() -> int",
			"GetCameraLookDamping() -> float",
			"ApplyImpact(impulse) -> brokeBool",
			"Fracture() -> debrisCount",
		},
	},
	"ScriptComponent": {
		Name:        "ScriptComponent",
		GoType:      "*luabridge.ScriptComponent",
		Description: "Per-instance sandboxed Lua lifecycle script (init, update, render, interact).",
		Fields: []string{
			"script_path / ScriptPath, script_name / ScriptName",
		},
		Methods: []string{
			"GetScriptPath() -> string",
			"GetScriptName() -> string",
		},
	},
}

// FormatObjectHelp builds a comprehensive inspection report for a GameObject:
// all callable GameObject methods, properties, and every attached component + its Go/canonical type.
func FormatObjectHelp(obj *engine.GameObject) string {
	if obj == nil {
		return "Object: nil"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== GameObject Help: %s (%s) ===\n", obj.Name, obj.ID))
	sb.WriteString(fmt.Sprintf("Active: %v | Spawned: %v\n",
		obj.Active, obj.IsSpawned()))

	sb.WriteString("Properties: id, name, active, scene\n")

	methodsMap := gameObjectMethods(nil)
	methodNames := make([]string, 0, len(methodsMap))
	for name := range methodsMap {
		methodNames = append(methodNames, name)
	}
	sort.Strings(methodNames)
	sb.WriteString("Methods (" + fmt.Sprintf("%d", len(methodNames)) + "):\n")
	sb.WriteString("  " + strings.Join(methodNames, ", ") + "\n")

	sb.WriteString(fmt.Sprintf("Components (%d):\n", len(obj.Components)))
	if len(obj.Components) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for i, c := range obj.Components {
			if c == nil {
				continue
			}
			canon := componentCanonicalName(c)
			goType := reflect.TypeOf(c).String()
			summary := summarizeComponentShort(c)
			if summary != "" {
				sb.WriteString(fmt.Sprintf("  %d. %s (%s) - %s\n", i+1, canon, goType, summary))
			} else {
				sb.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1, canon, goType))
			}
		}
	}
	sb.WriteString("Tip: Run `help(Mesh)`, `help(<ComponentName>)`, or `help(selected:GetComponent(Mesh))` for component details.")
	return sb.String()
}

// FormatComponentHelp formats fields, constraints, and methods for a component type name or live component.
func FormatComponentHelp(typeNameOrComp string) (string, bool) {
	canon := resolveComponentAlias(typeNameOrComp)
	doc, ok := componentDocs[canon]
	if !ok {
		return "", false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Component Help: %s (%s) ===\n", doc.Name, doc.GoType))
	sb.WriteString(fmt.Sprintf("Description: %s\n", doc.Description))
	if doc.Constraint != "" {
		sb.WriteString(fmt.Sprintf("Constraint:  %s\n", doc.Constraint))
	}
	if len(doc.Fields) > 0 {
		sb.WriteString("Fields / Properties:\n")
		for _, f := range doc.Fields {
			sb.WriteString("  " + f + "\n")
		}
	}
	if len(doc.Methods) > 0 {
		sb.WriteString("Methods:\n")
		for _, m := range doc.Methods {
			sb.WriteString("  " + m + "\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n"), true
}

// ResolveHelpArg evaluates `help(...)` against either nothing, a GameObject userdata,
// any component class/instance userdata, or a string topic / component name / registered object name.
func (b *LuaBridge) ResolveHelpArg(arg lua.LValue) string {
	txt, err := b.resolveHelpArgWithError(arg, "")
	if err != nil {
		return "Error: " + err.Error()
	}
	return txt
}

func (b *LuaBridge) resolveHelpArgWithError(arg lua.LValue, exprHint string) (string, error) {
	if arg == nil || arg == lua.LNil {
		hint := strings.TrimSpace(exprHint)
		if hint == "" {
			if exprHint == "" {
				return b.FormatHelp(""), nil
			}
			return "", fmt.Errorf("no class name or variable exists named %q", hint)
		}
		if txt, ok := b.formatNamedTypeHelp(hint); ok {
			return txt, nil
		}
		if obj := b.findSceneOrRegisteredObject(hint); obj != nil {
			return FormatObjectHelp(obj), nil
		}
		return "", fmt.Errorf("no class name or variable exists named %q", hint)
	}

	if ud, ok := arg.(*lua.LUserData); ok && ud != nil && ud.Value != nil {
		if obj, ok := ud.Value.(*engine.GameObject); ok {
			return FormatObjectHelp(obj), nil
		}
		if tt, ok := ud.Value.(*TypeToken); ok {
			return FormatTypeTokenHelp(tt), nil
		}
		if cc, ok := ud.Value.(*ComponentClass); ok {
			return b.formatComponentClassHelp(cc), nil
		}
		if lc, ok := ud.Value.(*LuaComponent); ok {
			return b.formatCustomComponentHelp(lc.Def, lc), nil
		}
		if comp, ok := ud.Value.(engine.Component); ok {
			canon := componentCanonicalName(comp)
			if txt, found := b.formatBuiltinComponentHelp(canon); found {
				return txt, nil
			}
			return formatReflectedComponentHelp(comp), nil
		}
		return SerializeUserDataFallback(b.L, ud), nil
	}

	if s, ok := arg.(lua.LString); ok {
		topic := strings.TrimSpace(string(s))
		if topic == "" {
			return b.FormatHelp(""), nil
		}
		if txt, ok := b.formatHelpTopicOnly(topic); ok {
			return txt, nil
		}
		if txt, ok := b.formatNamedTypeHelp(topic); ok {
			return txt, nil
		}
		if obj := b.findSceneOrRegisteredObject(topic); obj != nil {
			return FormatObjectHelp(obj), nil
		}
		return "", fmt.Errorf("no class name or variable exists named %q", topic)
	}

	return fmt.Sprintf("Lua %s: %s", arg.Type().String(), SerializeLuaValue(b.L, arg)), nil
}

func (b *LuaBridge) formatNamedTypeHelp(name string) (string, bool) {
	if def := b.getCustomComponentDef(name); def != nil {
		return b.formatCustomComponentHelp(def, nil), true
	}
	if txt, ok := b.formatBuiltinComponentHelp(name); ok {
		return txt, true
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gameobject":
		return FormatTypeTokenHelp(&TypeToken{Name: "GameObject", GoType: "*engine.GameObject"}), true
	case "scene":
		return FormatTypeTokenHelp(&TypeToken{Name: "Scene", GoType: "engine.Scene"}), true
	}
	return "", false
}

func (b *LuaBridge) formatBuiltinComponentHelp(name string) (string, bool) {
	canon := resolveComponentAlias(name)
	txt, ok := FormatComponentHelp(canon)
	if !ok {
		return "", false
	}
	b.dataMu.RLock()
	cc := b.componentClasses[canon]
	b.dataMu.RUnlock()
	if cc != nil && cc.Proto != nil {
		dynMethods := collectProtoMethodNames(cc.Proto)
		if len(dynMethods) > 0 {
			var sb strings.Builder
			sb.WriteString(txt)
			sb.WriteString("\nCustom Lua Methods:\n")
			for _, m := range dynMethods {
				sb.WriteString(fmt.Sprintf("  %s(...)\n", m))
			}
			return strings.TrimRight(sb.String(), "\n"), true
		}
	}
	return txt, true
}

func (b *LuaBridge) formatComponentClassHelp(cc *ComponentClass) string {
	if cc == nil {
		return "Component Class: nil"
	}
	if cc.CustomDef != nil {
		return b.formatCustomComponentHelp(cc.CustomDef, nil)
	}
	if txt, found := b.formatBuiltinComponentHelp(cc.Name); found {
		return txt
	}
	return fmt.Sprintf("Component Class: %s (%s)", cc.Name, cc.GoType)
}

func (b *LuaBridge) formatCustomComponentHelp(def *LuaComponentDef, inst *LuaComponent) string {
	if def == nil {
		return "Component Help: LuaComponent"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Component Help: %s (*luabridge.LuaComponent) ===\n", def.Name))
	sb.WriteString("Description: Lua-defined custom component.\n")

	if len(def.FieldNames) > 0 {
		sb.WriteString("Fields / Properties:\n")
		for _, k := range def.FieldNames {
			val := def.Defaults[k]
			if inst != nil && inst.Fields != nil {
				if cur := inst.Fields.RawGetString(k); cur != lua.LNil {
					val = cur
				}
			}
			sb.WriteString(fmt.Sprintf("  %s (%s = %s)\n", k, describeLuaValueType(val), SerializeLuaValue(b.L, val)))
		}
	}

	if len(def.HookNames) > 0 {
		sb.WriteString("Hooks:\n")
		for _, h := range def.HookNames {
			sig := def.Hooks[h]
			sb.WriteString(fmt.Sprintf("  %s(%s)\n", h, strings.Join(sig, ", ")))
		}
	}

	methods := collectProtoMethodNames(def.Proto)
	sb.WriteString("Methods:\n")
	for _, builtin := range []string{"GetOwner() -> GameObject", "GetEntity() -> GameObject", "GetTypeName() -> string"} {
		sb.WriteString("  " + builtin + "\n")
	}
	for _, m := range methods {
		sb.WriteString(fmt.Sprintf("  %s(...)\n", m))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// FormatTypeTokenHelp returns inspection details for non-component engine type tokens (e.g., GameObject).
func FormatTypeTokenHelp(tt *TypeToken) string {
	if tt == nil {
		return "Type: nil"
	}
	if tt.Name == "GameObject" {
		methodsMap := gameObjectMethods(nil)
		methodNames := make([]string, 0, len(methodsMap))
		for name := range methodsMap {
			methodNames = append(methodNames, name)
		}
		sort.Strings(methodNames)
		return fmt.Sprintf("=== Type Help: GameObject (*engine.GameObject) ===\nProperties: id, name, active, scene\nMethods (%d):\n  %s",
			len(methodNames), strings.Join(methodNames, ", "))
	}
	return fmt.Sprintf("=== Type Help: %s (%s) ===", tt.Name, tt.GoType)
}

func collectProtoMethodNames(proto *lua.LTable) []string {
	if proto == nil {
		return nil
	}
	var names []string
	proto.ForEach(func(k, v lua.LValue) {
		ks, ok := k.(lua.LString)
		if !ok {
			return
		}
		key := string(ks)
		if key == "__index" {
			return
		}
		if v.Type() == lua.LTFunction {
			names = append(names, key)
		}
	})
	sort.Strings(names)
	return names
}

func formatReflectedComponentHelp(comp engine.Component) string {
	t := reflect.TypeOf(comp)
	elem := t
	if elem.Kind() == reflect.Ptr {
		elem = elem.Elem()
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Component Help: %s (%s) ===\n", elem.Name(), t.String()))
	if elem.Kind() == reflect.Struct && elem.NumField() > 0 {
		sb.WriteString("Fields / Properties:\n")
		for i := 0; i < elem.NumField(); i++ {
			f := elem.Field(i)
			if f.IsExported() {
				sb.WriteString(fmt.Sprintf("  %s (%s)\n", f.Name, f.Type.String()))
			}
		}
	}
	if t.NumMethod() > 0 {
		sb.WriteString("Methods:\n")
		for i := 0; i < t.NumMethod(); i++ {
			m := t.Method(i)
			sb.WriteString(fmt.Sprintf("  %s\n", m.Name))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func SerializeUserDataFallback(L *lua.LState, ud *lua.LUserData) string {
	return serializeUserData(L, ud)
}

func summarizeComponentShort(c engine.Component) string {
	switch comp := c.(type) {
	case *engine.TransformComponent:
		p := comp.GetPosition()
		return fmt.Sprintf("pos=(%.2f, %.2f, %.2f)", p.X, p.Y, p.Z)
	case *engine.MeshComponent:
		return fmt.Sprintf("path=%q tris=%d", comp.FilePath, comp.TriangleCount())
	case *engine.MeshRenderable:
		return fmt.Sprintf("visible=%v valid=%v", comp.Visible, comp.ValidateConstraint() == nil)
	case *engine.Renderable:
		return fmt.Sprintf("mesh=%q mat=%q visible=%v", comp.MeshID, comp.MaterialID, comp.IsVisible())
	case *engine.Renderable2D:
		return fmt.Sprintf("tex=%d size=(%.0f, %.0f)", comp.TextureID, comp.Size.X, comp.Size.Y)
	case *spatial.RigidBody:
		return fmt.Sprintf("id=%q body_id=%d mass=%.2f", comp.ID, comp.BodyID, comp.Mass)
	case *spatial.InteractiveComponent:
		return fmt.Sprintf("name=%q enabled=%v", comp.DisplayName, comp.Enabled)
	case *spatial.FractureComponent:
		return fmt.Sprintf("hp=%.1f hands=%d", comp.Health, comp.RequiredCarryHands)
	case *ScriptComponent:
		return fmt.Sprintf("script=%q", comp.ScriptPath)
	case *LuaComponent:
		if comp.Fields == nil {
			return ""
		}
		var keys []string
		comp.Fields.ForEach(func(k, _ lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				keys = append(keys, string(ks))
			}
		})
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, SerializeLuaValue(comp.Bridge.L, comp.Fields.RawGetString(k))))
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}
