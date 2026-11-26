package spatial

import (
	"reflect"

	"github.com/SamSoupSauce/project-engine/engine"
)

// VerbID identifies standard and script-defined interaction verbs.
type VerbID uint32

const (
	VerbNone           VerbID = 0
	VerbGrab           VerbID = 1
	VerbThrow          VerbID = 2
	VerbOpenInventory  VerbID = 3
	VerbToggleState    VerbID = 4
	VerbStashInventory VerbID = 5
	VerbDismantle      VerbID = 6
	VerbCustomLua      VerbID = 100
)

// DefaultHighlightTint is the warm amber outline color applied on reticle hover.
var DefaultHighlightTint = [4]float32{1.0, 0.78, 0.2, 1.0}

// InteractionOption describes a primary tap action or secondary hold-menu context verb.
type InteractionOption struct {
	Verb    VerbID
	Label   string
	LuaHook string // Optional Lua global/hook function name (e.g. "on_open_cabinet")
	IsHold  bool   // True for secondary verbs presented in the hold context menu
}

// InteractiveComponent marks a GameObject as raycast-interactable with highlight tint,
// quick-tap primary verb, and hold-menu secondary context verbs.
type InteractiveComponent struct {
	Owner         *engine.GameObject
	Enabled       bool
	DisplayName   string
	HighlightTint [4]float32
	OutlineWidth  float32
	DefaultVerb   InteractionOption
	ContextVerbs  []InteractionOption
	IsDynamicProp bool
	EntityTag     uint32

	Hovered bool
	Held    bool
	Body    *RigidBody
}

// NewInteractiveComponent creates an enabled InteractiveComponent with default highlight styling.
func NewInteractiveComponent(displayName string, isDynamicProp bool) *InteractiveComponent {
	defaultVerb := InteractionOption{
		Verb:  VerbToggleState,
		Label: "Interact",
	}
	if isDynamicProp {
		defaultVerb = InteractionOption{
			Verb:  VerbGrab,
			Label: "Grab",
		}
	}
	return &InteractiveComponent{
		Enabled:       true,
		DisplayName:   displayName,
		HighlightTint: DefaultHighlightTint,
		OutlineWidth:  engine.DefaultOutlineGain,
		DefaultVerb:   defaultVerb,
		ContextVerbs:  make([]InteractionOption, 0, 4),
		IsDynamicProp: isDynamicProp,
	}
}

// Init implements engine.Component and binds sibling physics/renderable state.
func (ic *InteractiveComponent) Init(owner *engine.GameObject) {
	ic.Owner = owner
	if owner == nil {
		return
	}
	if ic.DisplayName == "" {
		ic.DisplayName = owner.Name
	}
	ic.bindRigidBody()
	if ic.Hovered {
		ic.SetHovered(true)
	}
}

// Update implements engine.Component.
func (ic *InteractiveComponent) Update(_ float32) {
	if ic == nil || ic.Owner == nil {
		return
	}
	if ic.Body == nil {
		ic.bindRigidBody()
	}
}

// Destroy implements engine.Component, ensuring highlights are cleared cleanly.
func (ic *InteractiveComponent) Destroy() {
	if ic == nil {
		return
	}
	ic.SetHovered(false)
	ic.Held = false
	ic.Body = nil
	ic.Owner = nil
}

func (ic *InteractiveComponent) bindRigidBody() {
	if ic.Owner == nil {
		return
	}
	comp := ic.Owner.GetComponent(reflect.TypeOf((*RigidBody)(nil)))
	if rb, ok := comp.(*RigidBody); ok && rb != nil {
		ic.Body = rb
		if ic.EntityTag != 0 && rb.UserData == 0 {
			rb.SetUserData(ic.EntityTag)
		} else if ic.EntityTag == 0 && rb.UserData != 0 {
			ic.EntityTag = rb.UserData
		}
		if rb.BodyType == engine.BodyDynamic && ic.DefaultVerb.Verb == VerbNone {
			ic.DefaultVerb = InteractionOption{Verb: VerbGrab, Label: "Grab"}
		}
	}
}

// SetDefaultVerb configures the primary action triggered by a quick tap (< 250ms).
func (ic *InteractiveComponent) SetDefaultVerb(verb VerbID, label, luaHook string) {
	if ic == nil {
		return
	}
	ic.DefaultVerb = InteractionOption{
		Verb:    verb,
		Label:   label,
		LuaHook: luaHook,
		IsHold:  false,
	}
}

// AddContextVerb appends a secondary action shown when holding interact (> 250ms).
func (ic *InteractiveComponent) AddContextVerb(verb VerbID, label, luaHook string) {
	if ic == nil {
		return
	}
	ic.ContextVerbs = append(ic.ContextVerbs, InteractionOption{
		Verb:    verb,
		Label:   label,
		LuaHook: luaHook,
		IsHold:  true,
	})
}

// ClearContextVerbs removes all configured secondary context verbs.
func (ic *InteractiveComponent) ClearContextVerbs() {
	if ic == nil {
		return
	}
	ic.ContextVerbs = ic.ContextVerbs[:0]
}

// SetHovered toggles hover state and synchronizes FlagOutline + tint onto attached Renderables.
func (ic *InteractiveComponent) SetHovered(hovered bool) {
	if ic == nil {
		return
	}
	if !ic.Enabled {
		hovered = false
	}
	ic.Hovered = hovered
	if ic.Owner == nil {
		return
	}

	tint := engine.Color{
		R: floatToColorByte(ic.HighlightTint[0]),
		G: floatToColorByte(ic.HighlightTint[1]),
		B: floatToColorByte(ic.HighlightTint[2]),
		A: floatToColorByte(ic.HighlightTint[3]),
	}
	if tint.A == 0 {
		tint = engine.Color{
			R: floatToColorByte(DefaultHighlightTint[0]),
			G: floatToColorByte(DefaultHighlightTint[1]),
			B: floatToColorByte(DefaultHighlightTint[2]),
			A: floatToColorByte(DefaultHighlightTint[3]),
		}
	}
	width := ic.OutlineWidth
	if width <= 0 {
		width = engine.DefaultOutlineGain
	}

	if !applyOutlineToObject(ic.Owner, hovered, tint, width) {
		for _, child := range ic.Owner.Children {
			applyOutlineToSubtree(child, hovered, tint, width)
		}
	} else if !hovered {
		// Ensure child overrides are also cleared if unhighlighting.
		for _, child := range ic.Owner.Children {
			applyOutlineToSubtree(child, false, tint, width)
		}
	}
}

func floatToColorByte(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1.0 {
		return 255
	}
	return uint8(v*255.0 + 0.5)
}

func applyOutlineToObject(obj *engine.GameObject, hovered bool, tint engine.Color, width float32) bool {
	if obj == nil {
		return false
	}
	comp := obj.GetComponent(reflect.TypeOf((*engine.Renderable)(nil)))
	r, ok := comp.(*engine.Renderable)
	if !ok || r == nil {
		return false
	}
	r.SetOutlined(hovered)
	if hovered {
		r.OutlineColor = tint
		r.OutlineWidth = width
	}
	return true
}

func applyOutlineToSubtree(obj *engine.GameObject, hovered bool, tint engine.Color, width float32) {
	if obj == nil {
		return
	}
	applyOutlineToObject(obj, hovered, tint, width)
	for _, child := range obj.Children {
		applyOutlineToSubtree(child, hovered, tint, width)
	}
}

// FindInteractiveComponent returns the InteractiveComponent attached to obj, or nil.
func FindInteractiveComponent(obj *engine.GameObject) *InteractiveComponent {
	if obj == nil {
		return nil
	}
	comp := obj.GetComponent(reflect.TypeOf((*InteractiveComponent)(nil)))
	if ic, ok := comp.(*InteractiveComponent); ok && ic != nil {
		return ic
	}
	return nil
}

// ResolveInteractiveFromHit resolves the hit GameObject and its InteractiveComponent from a PhysicsWorld raycast hit.
func ResolveInteractiveFromHit(world *PhysicsWorld, scene engine.Scene, hit engine.RaycastHit) (*engine.GameObject, *InteractiveComponent) {
	if !hit.Hit {
		return nil, nil
	}

	var obj *engine.GameObject
	if world != nil {
		if handle := world.GetBody(hit.BodyID); handle != nil {
			if rb, ok := handle.(*RigidBody); ok && rb != nil && rb.Owner != nil {
				obj = rb.Owner
			}
		}
		if obj == nil && hit.BodyID != "" {
			if rb := world.FindBodyByRayHit(hit); rb != nil {
				if rb.Owner != nil {
					obj = rb.Owner
				} else if tgt := world.GetSyncTarget(rb.BodyID); tgt != nil {
					obj = tgt
				}
			}
		}
	}

	if obj == nil && scene != nil && hit.BodyID != "" {
		if finder, ok := scene.(interface{ FindByID(string) *engine.GameObject }); ok {
			obj = finder.FindByID(hit.BodyID)
		}
	}
	if obj == nil {
		return nil, nil
	}

	// Walk up parent chain in case a collider is mounted on a child node of an interactive prefab.
	for cur := obj; cur != nil; cur = cur.Parent {
		if ic := FindInteractiveComponent(cur); ic != nil {
			if ic.Enabled {
				return cur, ic
			}
			return cur, nil
		}
	}
	return obj, nil
}
