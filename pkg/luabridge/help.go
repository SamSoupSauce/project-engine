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

	pos := obj.GetWorldTransform().Position
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== GameObject Help: %s (%s) ===\n", obj.Name, obj.ID))
	sb.WriteString(fmt.Sprintf("Active: %v | Spawned: %v | Position: (%.2f, %.2f, %.2f)\n",
		obj.Active, obj.IsSpawned(), pos.X, pos.Y, pos.Z))

	sb.WriteString("Properties: id, name, active, scene, position, rotation, scale, transform\n")

	methodsMap := gameObjectMethods()
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
	sb.WriteString("Tip: Run `help(\"<ComponentName>\")` or `help(selected:GetComponent(\"<ComponentName>\"))` for component details.")
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
// any component userdata, or a string topic / component name / registered object name.
func (b *LuaBridge) ResolveHelpArg(arg lua.LValue) string {
	if arg == nil || arg == lua.LNil {
		return b.FormatHelp("")
	}

	if ud, ok := arg.(*lua.LUserData); ok && ud != nil && ud.Value != nil {
		if obj, ok := ud.Value.(*engine.GameObject); ok {
			return FormatObjectHelp(obj)
		}
		if comp, ok := ud.Value.(engine.Component); ok {
			canon := componentCanonicalName(comp)
			if txt, found := FormatComponentHelp(canon); found {
				return txt
			}
			return fmt.Sprintf("Component %s (%T)", canon, comp)
		}
		return SerializeUserDataFallback(b.L, ud)
	}

	topic := strings.TrimSpace(arg.String())
	if topic == "" {
		return b.FormatHelp("")
	}
	if txt, ok := FormatComponentHelp(topic); ok {
		return txt
	}
	if obj := b.findSceneOrRegisteredObject(topic); obj != nil {
		return FormatObjectHelp(obj)
	}
	return b.FormatHelp(topic)
}

func SerializeUserDataFallback(L *lua.LState, ud *lua.LUserData) string {
	return serializeUserData(L, ud)
}

func summarizeComponentShort(c engine.Component) string {
	switch comp := c.(type) {
	case *engine.TransformComponent:
		p := comp.GetPosition()
		return fmt.Sprintf("pos=(%.2f, %.2f, %.2f)", p.X, p.Y, p.Z)
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
	default:
		return ""
	}
}
