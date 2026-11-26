package interaction

import (
	"fmt"
	"math"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

const (
	// DefaultHoldThreshold is the press duration (seconds) after which a hold opens the context menu.
	DefaultHoldThreshold float64 = 0.250
	// DefaultInteractRange is the default maximum reticle raycast reach in meters.
	DefaultInteractRange float32 = 4.5
	// DefaultCarryDistance is the forward offset in front of the camera for held dynamic props.
	DefaultCarryDistance float32 = 1.8
	// DefaultThrowImpulse is the impulse applied along camera forward on throw release.
	DefaultThrowImpulse float32 = 12.0
)

// CameraPose supplies the player eye origin, normalized look vector, and viewport size.
type CameraPose struct {
	Origin    engine.Vector3
	Forward   engine.Vector3
	ViewportW float32
	ViewportH float32
}

// InteractionResult summarizes what happened when an interact input ended or menu option fired.
type InteractionResult struct {
	Executed     bool
	Action       string
	Target       *engine.GameObject
	Option       spatial.InteractionOption
	GrabbedProp  *engine.GameObject
	ThrownProp   *engine.GameObject
	MenuOpened   bool
	MenuCanceled bool
}

// InteractionState holds the current hover, grab, press-timer, and radial/context menu state.
type InteractionState struct {
	HoveredEntity       *engine.GameObject
	HoveredComp         *spatial.InteractiveComponent
	ActiveHeldProp      *engine.GameObject
	ActiveHeldBody      *spatial.RigidBody
	PressActive         bool
	PressStartTime      float64
	PressedTarget       *engine.GameObject
	HoldThreshold       float64
	IsMenuOpen          bool
	MenuTarget          *engine.GameObject
	MenuOptions         []spatial.InteractionOption
	SelectedOptionIndex int
}

// LuaVerbDispatcher invokes a named Lua callback on behalf of an executed interaction option.
type LuaVerbDispatcher func(target *engine.GameObject, option spatial.InteractionOption) error

// Controller orchestrates reticle raycast hovering, stencil outline toggling,
// <250ms tap primary verbs / grab / throw, and >250ms hold context menu dispatch.
type Controller struct {
	World                  *spatial.PhysicsWorld
	Scene                  engine.Scene
	State                  InteractionState
	InteractRange          float32
	CarryDistance          float32
	ThrowImpulse           float32
	CollisionMask          uint16
	Clock                  float64
	DismissMenuOnHoverLoss bool

	OnLuaVerb      LuaVerbDispatcher
	OnVerbExecuted func(target *engine.GameObject, option spatial.InteractionOption)
}

// NewController constructs an Interaction Controller wired to a PhysicsWorld and Scene.
func NewController(world *spatial.PhysicsWorld, scene engine.Scene) *Controller {
	return &Controller{
		World:                  world,
		Scene:                  scene,
		InteractRange:          DefaultInteractRange,
		CarryDistance:          DefaultCarryDistance,
		ThrowImpulse:           DefaultThrowImpulse,
		CollisionMask:          spatial.LayerMaskAll,
		DismissMenuOnHoverLoss: true,
		State: InteractionState{
			HoldThreshold:       DefaultHoldThreshold,
			SelectedOptionIndex: -1,
		},
	}
}

// Update advances the controller clock, maintains held prop target tracking, updates reticle hover
// highlights, and opens the hold context menu once HoldThreshold is exceeded.
func (c *Controller) Update(cam CameraPose, dt float32) InteractionResult {
	if c == nil {
		return InteractionResult{}
	}
	if dt > 0 {
		c.Clock += float64(dt)
	}

	if c.State.ActiveHeldProp != nil && c.World != nil {
		c.World.UpdateGrabTarget(c.carryTargetPos(cam))
	}

	var hit engine.RaycastHit
	if c.World != nil && c.InteractRange > 0 {
		dir := normalizeVec3(cam.Forward)
		if dir.X != 0 || dir.Y != 0 || dir.Z != 0 {
			mask := c.CollisionMask
			if mask == 0 {
				mask = spatial.LayerMaskAll
			}
			hit = c.World.RaycastMask(engine.Ray{Position: cam.Origin, Direction: dir}, c.InteractRange, mask)
		}
	}

	target, comp := spatial.ResolveInteractiveFromHit(c.World, c.Scene, hit)
	if comp == nil {
		target = nil
	}

	c.setHoverTarget(target, comp)

	var res InteractionResult
	if c.State.PressActive && !c.State.IsMenuOpen {
		threshold := c.State.HoldThreshold
		if threshold <= 0 {
			threshold = DefaultHoldThreshold
		}
		if c.Clock-c.State.PressStartTime >= threshold {
			if c.State.ActiveHeldProp != nil && c.State.PressedTarget == c.State.ActiveHeldProp {
				if heldIC := spatial.FindInteractiveComponent(c.State.ActiveHeldProp); heldIC != nil && heldIC.Enabled && len(heldIC.ContextVerbs) > 0 {
					c.openMenu(c.State.ActiveHeldProp, heldIC.ContextVerbs)
					res.MenuOpened = true
					res.Target = c.State.ActiveHeldProp
				}
			} else if c.State.HoveredComp != nil && c.State.HoveredEntity == c.State.PressedTarget && len(c.State.HoveredComp.ContextVerbs) > 0 {
				c.openMenu(c.State.HoveredEntity, c.State.HoveredComp.ContextVerbs)
				res.MenuOpened = true
				res.Target = c.State.HoveredEntity
			}
		}
	}
	return res
}

// SetHoverTargetDirectly sets or clears hover highlight explicitly (useful for custom raycasts or tests).
func (c *Controller) SetHoverTargetDirectly(target *engine.GameObject) {
	if c == nil {
		return
	}
	comp := spatial.FindInteractiveComponent(target)
	if comp != nil && !comp.Enabled {
		comp = nil
	}
	if comp == nil {
		target = nil
	}
	c.setHoverTarget(target, comp)
}

func (c *Controller) setHoverTarget(target *engine.GameObject, comp *spatial.InteractiveComponent) {
	// While carrying a prop, do not highlight arbitrary background geometry or drop hold prompts.
	if c.State.ActiveHeldProp != nil {
		if c.State.HoveredComp != nil && c.State.HoveredEntity != c.State.ActiveHeldProp {
			c.State.HoveredComp.SetHovered(false)
			c.State.HoveredComp = nil
			c.State.HoveredEntity = nil
		}
		return
	}

	if c.State.HoveredEntity == target && c.State.HoveredComp == comp {
		return
	}

	if c.State.HoveredComp != nil {
		c.State.HoveredComp.SetHovered(false)
	}
	c.State.HoveredEntity = target
	c.State.HoveredComp = comp
	if comp != nil {
		comp.SetHovered(true)
	}

	if c.State.IsMenuOpen && c.DismissMenuOnHoverLoss && target != c.State.MenuTarget {
		c.CloseMenu()
	}
}

// OnInteractPressed starts tracking hold duration for tap vs. long-press interaction.
func (c *Controller) OnInteractPressed() {
	if c == nil {
		return
	}
	c.State.PressActive = true
	c.State.PressStartTime = c.Clock
	if c.State.ActiveHeldProp != nil {
		c.State.PressedTarget = c.State.ActiveHeldProp
	} else {
		c.State.PressedTarget = c.State.HoveredEntity
	}
}

// OnInteractReleased completes the interaction: executing selected hold-menu option if open,
// throwing a held prop, grabbing a dynamic prop, or executing DefaultVerb Lua hook on quick tap.
func (c *Controller) OnInteractReleased(cam CameraPose) (InteractionResult, error) {
	if c == nil {
		return InteractionResult{}, nil
	}
	heldDuration := c.Clock - c.State.PressStartTime
	c.State.PressActive = false
	pressedTarget := c.State.PressedTarget
	c.State.PressedTarget = nil

	if c.State.IsMenuOpen {
		idx := c.State.SelectedOptionIndex
		if idx >= 0 && idx < len(c.State.MenuOptions) {
			return c.SelectMenuOption(idx)
		}
		c.CloseMenu()
		return InteractionResult{MenuCanceled: true}, nil
	}

	threshold := c.State.HoldThreshold
	if threshold <= 0 {
		threshold = DefaultHoldThreshold
	}
	if heldDuration >= threshold {
		// Held long without secondary ContextVerbs: dismiss cleanly unless hovering same target without menu.
		return InteractionResult{MenuCanceled: true}, nil
	}

	// Quick tap while holding a dynamic prop -> Throw!
	if c.State.ActiveHeldProp != nil {
		thrown, err := c.ThrowHeldProp(cam.Forward)
		return InteractionResult{
			Executed:   err == nil && thrown != nil,
			Action:     "throw",
			Target:     thrown,
			ThrownProp: thrown,
			Option:     spatial.InteractionOption{Verb: spatial.VerbThrow, Label: "Throw"},
		}, err
	}

	target := c.State.HoveredEntity
	comp := c.State.HoveredComp
	if target == nil || comp == nil || !comp.Enabled {
		return InteractionResult{}, nil
	}
	if pressedTarget != nil && pressedTarget != target {
		return InteractionResult{}, nil
	}

	// Free dynamic prop -> Grab!
	if isGrabVerb(comp) {
		if err := c.GrabEntity(target, cam); err != nil {
			return InteractionResult{}, err
		}
		return InteractionResult{
			Executed:    true,
			Action:      "grab",
			Target:      target,
			GrabbedProp: target,
			Option:      spatial.InteractionOption{Verb: spatial.VerbGrab, Label: "Grab"},
		}, nil
	}

	return c.executeOption(target, comp.DefaultVerb)
}

// SelectMenuOption executes the option at index from the currently open context menu and closes it.
func (c *Controller) SelectMenuOption(index int) (InteractionResult, error) {
	if c == nil || !c.State.IsMenuOpen {
		return InteractionResult{}, nil
	}
	if index < 0 || index >= len(c.State.MenuOptions) {
		c.CloseMenu()
		return InteractionResult{MenuCanceled: true}, nil
	}
	target := c.State.MenuTarget
	opt := c.State.MenuOptions[index]
	c.CloseMenu()
	return c.executeOption(target, opt)
}

// SetHoveredMenuOption updates the currently highlighted menu index for mouse/radial release selection.
func (c *Controller) SetHoveredMenuOption(index int) {
	if c == nil || !c.State.IsMenuOpen {
		return
	}
	if index < -1 || index >= len(c.State.MenuOptions) {
		index = -1
	}
	c.State.SelectedOptionIndex = index
}

// CloseMenu dismisses any active context popup cleanly.
func (c *Controller) CloseMenu() {
	if c == nil {
		return
	}
	c.State.IsMenuOpen = false
	c.State.MenuTarget = nil
	c.State.MenuOptions = c.State.MenuOptions[:0]
	c.State.SelectedOptionIndex = -1
}

// GrabEntity attaches a dynamic prop entity to the carry anchor.
func (c *Controller) GrabEntity(target *engine.GameObject, cam CameraPose) error {
	if c == nil || target == nil {
		return fmt.Errorf("interaction: cannot grab nil target")
	}
	comp := spatial.FindInteractiveComponent(target)
	if comp == nil || !comp.Enabled {
		return fmt.Errorf("interaction: target %q has no enabled InteractiveComponent", target.Name)
	}
	rb := comp.Body
	if rb == nil && c.World != nil {
		if h := c.World.GetBody(target.ID); h != nil {
			rb, _ = h.(*spatial.RigidBody)
		}
	}
	if rb == nil {
		return fmt.Errorf("interaction: dynamic grab target %q has no RigidBody", target.Name)
	}
	if c.World == nil {
		return fmt.Errorf("interaction: controller has nil PhysicsWorld")
	}

	if err := c.World.GrabBody(rb, c.carryTargetPos(cam)); err != nil {
		return err
	}

	comp.Held = true
	comp.SetHovered(false)
	c.State.ActiveHeldProp = target
	c.State.ActiveHeldBody = rb
	c.State.HoveredEntity = nil
	c.State.HoveredComp = nil
	c.CloseMenu()
	return nil
}

// ThrowHeldProp releases the held dynamic prop constraint and launches it along forward.
func (c *Controller) ThrowHeldProp(forward engine.Vector3) (*engine.GameObject, error) {
	if c == nil || c.State.ActiveHeldProp == nil {
		return nil, nil
	}
	held := c.State.ActiveHeldProp
	if comp := spatial.FindInteractiveComponent(held); comp != nil {
		comp.Held = false
	}
	if c.World != nil {
		impulse := c.ThrowImpulse
		if impulse <= 0 {
			impulse = DefaultThrowImpulse
		}
		c.World.ThrowGrab(normalizeVec3(forward), impulse)
	}
	c.State.ActiveHeldProp = nil
	c.State.ActiveHeldBody = nil
	return held, nil
}

// DropHeldProp releases any held prop gently without impulse.
func (c *Controller) DropHeldProp() *engine.GameObject {
	if c == nil || c.State.ActiveHeldProp == nil {
		return nil
	}
	held := c.State.ActiveHeldProp
	if comp := spatial.FindInteractiveComponent(held); comp != nil {
		comp.Held = false
	}
	if c.World != nil {
		c.World.ReleaseGrab()
	}
	c.State.ActiveHeldProp = nil
	c.State.ActiveHeldBody = nil
	return held
}

func (c *Controller) openMenu(target *engine.GameObject, options []spatial.InteractionOption) {
	c.State.IsMenuOpen = true
	c.State.MenuTarget = target
	c.State.SelectedOptionIndex = -1
	c.State.MenuOptions = append(c.State.MenuOptions[:0], options...)
}

func (c *Controller) executeOption(target *engine.GameObject, opt spatial.InteractionOption) (InteractionResult, error) {
	if opt.Verb == spatial.VerbGrab && target != nil && c.World != nil {
		// Support grabbing via explicit context menu option too.
		err := c.GrabEntity(target, CameraPose{
			Origin:  target.Transform.Position,
			Forward: engine.NewVector3(0, 0, 1),
		})
		return InteractionResult{
			Executed:    err == nil,
			Action:      "grab",
			Target:      target,
			GrabbedProp: target,
			Option:      opt,
		}, err
	}

	if c.OnLuaVerb != nil && (opt.LuaHook != "" || opt.Verb != spatial.VerbNone) {
		if err := c.OnLuaVerb(target, opt); err != nil {
			return InteractionResult{Target: target, Option: opt}, err
		}
	}
	if c.OnVerbExecuted != nil {
		c.OnVerbExecuted(target, opt)
	}
	return InteractionResult{
		Executed: true,
		Action:   opt.Label,
		Target:   target,
		Option:   opt,
	}, nil
}

func (c *Controller) carryTargetPos(cam CameraPose) engine.Vector3 {
	dir := normalizeVec3(cam.Forward)
	dist := c.CarryDistance
	if dist <= 0 {
		dist = DefaultCarryDistance
	}
	return engine.NewVector3(
		cam.Origin.X+dir.X*dist,
		cam.Origin.Y+dir.Y*dist,
		cam.Origin.Z+dir.Z*dist,
	)
}

func isGrabVerb(comp *spatial.InteractiveComponent) bool {
	if comp == nil {
		return false
	}
	if comp.DefaultVerb.Verb == spatial.VerbGrab {
		return true
	}
	if comp.IsDynamicProp && comp.DefaultVerb.LuaHook == "" && comp.Body != nil && comp.Body.BodyType == engine.BodyDynamic {
		return true
	}
	return false
}

func normalizeVec3(v engine.Vector3) engine.Vector3 {
	mag := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if mag < 1e-6 {
		return engine.Vector3{}
	}
	return engine.NewVector3(v.X/mag, v.Y/mag, v.Z/mag)
}
