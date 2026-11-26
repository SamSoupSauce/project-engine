package spatial

/*
#include "joltc.h"
#include "jolt_ffi.h"
*/
import "C"

import (
	"math"
	"unsafe"

	"github.com/SamSoupSauce/project-engine/engine"
)

// ShapeKind identifies primitive collision geometries supported by RigidBody.
type ShapeKind uint8

const (
	ShapeBox ShapeKind = iota
	ShapeSphere
	ShapeCapsule
)

// ShapeDesc describes collision dimensions for a RigidBody component.
type ShapeDesc struct {
	Kind        ShapeKind
	HalfExtents engine.Vector3 // Used by ShapeBox
	Radius      float32        // Used by ShapeSphere and ShapeCapsule
	HalfHeight  float32        // Used by ShapeCapsule (cylinder half-height)
}

// BoxShapeDesc creates a box ShapeDesc from half-extents.
func BoxShapeDesc(hx, hy, hz float32) ShapeDesc {
	return ShapeDesc{Kind: ShapeBox, HalfExtents: engine.NewVector3(hx, hy, hz)}
}

// SphereShapeDesc creates a sphere ShapeDesc from radius.
func SphereShapeDesc(radius float32) ShapeDesc {
	return ShapeDesc{Kind: ShapeSphere, Radius: radius}
}

// CapsuleShapeDesc creates a capsule ShapeDesc from cylinder half-height and radius.
func CapsuleShapeDesc(halfHeight, radius float32) ShapeDesc {
	return ShapeDesc{Kind: ShapeCapsule, HalfHeight: halfHeight, Radius: radius}
}

// RigidBody is an engine.Component and engine.PhysicsBodyHandle backed by a Jolt rigid body.
// After each physics tick, its Jolt transform synchronizes back to Owner.Transform.
type RigidBody struct {
	ID             string
	World          *PhysicsWorld
	Owner          *engine.GameObject
	BodyID         uint32
	BodyType       engine.PhysicsBodyType
	Shape          ShapeDesc
	Mass           float32
	Friction       float32
	Restitution    float32
	LinearDamping  float32
	AngularDamping float32
	GravityFactor  float32
	UseCCD         bool
	UserData       uint32
	ObjectLayer    uint32
	HasCustomLayer bool

	initialPos engine.Vector3
	initialRot engine.Vector3
}

// NewRigidBody constructs a RigidBody component configured with sensible defaults.
func NewRigidBody(id string, world *PhysicsWorld, bodyType engine.PhysicsBodyType, shape ShapeDesc) *RigidBody {
	return &RigidBody{
		ID:             id,
		World:          world,
		BodyID:         InvalidBodyID,
		BodyType:       bodyType,
		Shape:          shape,
		Mass:           1.0,
		Friction:       0.5,
		Restitution:    0.1,
		LinearDamping:  0.05,
		AngularDamping: 0.05,
		GravityFactor:  1.0,
	}
}

// Init registers the owning GameObject and spawns the Jolt body if a PhysicsWorld is configured.
func (rb *RigidBody) Init(owner *engine.GameObject) {
	rb.Owner = owner
	if owner != nil {
		rb.initialPos = owner.Transform.Position
		rb.initialRot = owner.Transform.Rotation
	}
	if rb.World != nil && rb.BodyID == InvalidBodyID {
		rb.AttachToWorld(rb.World)
	}
}

// Update satisfies engine.Component; simulation stepping occurs in PhysicsWorld.StepSimulation.
func (rb *RigidBody) Update(dt float32) {}

// Destroy removes and destroys the underlying Jolt rigid body from PhysicsWorld.
func (rb *RigidBody) Destroy() {
	if rb.World != nil && rb.BodyID != InvalidBodyID {
		w := rb.World
		w.mu.Lock()
		if !w.closed && w.bodyIface != nil {
			if bool(C.JPH_BodyInterface_IsAdded(w.bodyIface, C.JPH_BodyID(rb.BodyID))) {
				C.JPH_BodyInterface_RemoveAndDestroyBody(w.bodyIface, C.JPH_BodyID(rb.BodyID))
			}
			delete(w.bodies, rb.BodyID)
			if rb.ID != "" {
				delete(w.bodiesByID, rb.ID)
			}
		}
		w.mu.Unlock()
	}
	rb.BodyID = InvalidBodyID
	rb.World = nil
	rb.Owner = nil
}

// AttachToWorld creates the Jolt body inside the provided PhysicsWorld and registers transform sync.
func (rb *RigidBody) AttachToWorld(world *PhysicsWorld) {
	if world == nil {
		return
	}
	world.mu.Lock()
	if world.closed || world.bodyIface == nil {
		world.mu.Unlock()
		return
	}
	if rb.BodyID != InvalidBodyID {
		world.mu.Unlock()
		return
	}
	iface := world.bodyIface
	world.mu.Unlock()

	jShape := rb.createJoltShape()
	if jShape == nil {
		return
	}
	defer C.JPH_Shape_Destroy(jShape)

	pos := rb.initialPos
	rotDeg := rb.initialRot
	if rb.Owner != nil {
		pos = rb.Owner.Transform.Position
		rotDeg = rb.Owner.Transform.Rotation
	}

	jPos := C.JPH_RVec3{x: C.float(pos.X), y: C.float(pos.Y), z: C.float(pos.Z)}
	jRot := eulerDegToJoltQuat(rotDeg)

	motionType := C.JPH_MotionType_Dynamic
	layer := LayerMoving
	activation := C.JPH_Activation_Activate
	switch rb.BodyType {
	case engine.BodyStatic:
		motionType = C.JPH_MotionType_Static
		layer = LayerNonMoving
		activation = C.JPH_Activation_DontActivate
	case engine.BodyKinematic:
		motionType = C.JPH_MotionType_Kinematic
		layer = LayerMoving
	}
	if rb.HasCustomLayer {
		layer = rb.ObjectLayer
	}

	bcs := C.JPH_BodyCreationSettings_Create3(jShape, &jPos, &jRot, C.JPH_MotionType(motionType), C.JPH_ObjectLayer(layer))
	if bcs == nil {
		return
	}
	defer C.JPH_BodyCreationSettings_Destroy(bcs)

	C.JPH_BodyCreationSettings_SetUserData(bcs, C.uint64_t(rb.UserData))
	C.JPH_BodyCreationSettings_SetFriction(bcs, C.float(rb.Friction))
	C.JPH_BodyCreationSettings_SetRestitution(bcs, C.float(rb.Restitution))
	C.JPH_BodyCreationSettings_SetLinearDamping(bcs, C.float(rb.LinearDamping))
	C.JPH_BodyCreationSettings_SetAngularDamping(bcs, C.float(rb.AngularDamping))
	C.JPH_BodyCreationSettings_SetGravityFactor(bcs, C.float(rb.GravityFactor))
	if rb.UseCCD {
		C.JPH_BodyCreationSettings_SetMotionQuality(bcs, C.JPH_MotionQuality_LinearCast)
	}
	if rb.Mass > 0 && rb.BodyType == engine.BodyDynamic {
		var mp C.JPH_MassProperties
		C.JPH_Shape_GetMassProperties(jShape, &mp)
		C.JPH_MassProperties_ScaleToMass(&mp, C.float(rb.Mass))
		C.JPH_BodyCreationSettings_SetOverrideMassProperties(bcs, C.JPH_OverrideMassProperties_CalculateInertia)
		C.JPH_BodyCreationSettings_SetMassPropertiesOverride(bcs, &mp)
	}

	bodyID := uint32(C.JPH_BodyInterface_CreateAndAddBody(iface, bcs, C.JPH_Activation(activation)))
	if bodyID == InvalidBodyID {
		return
	}

	rb.World = world
	rb.BodyID = bodyID

	world.mu.Lock()
	if !world.closed {
		world.bodies[bodyID] = rb
		if rb.ID != "" {
			world.bodiesByID[rb.ID] = rb
		}
	}
	world.mu.Unlock()
}

func (rb *RigidBody) createJoltShape() *C.JPH_Shape {
	switch rb.Shape.Kind {
	case ShapeSphere:
		r := rb.Shape.Radius
		if r <= 0 {
			r = 0.5
		}
		return (*C.JPH_Shape)(unsafe.Pointer(C.JPH_SphereShape_Create(C.float(r))))
	case ShapeCapsule:
		hh := rb.Shape.HalfHeight
		r := rb.Shape.Radius
		if hh <= 0 {
			hh = 0.5
		}
		if r <= 0 {
			r = 0.25
		}
		return (*C.JPH_Shape)(unsafe.Pointer(C.JPH_CapsuleShape_Create(C.float(hh), C.float(r))))
	default:
		hx, hy, hz := rb.Shape.HalfExtents.X, rb.Shape.HalfExtents.Y, rb.Shape.HalfExtents.Z
		if hx <= 0 {
			hx = 0.5
		}
		if hy <= 0 {
			hy = 0.5
		}
		if hz <= 0 {
			hz = 0.5
		}
		convexR := float32(C.JPH_DEFAULT_CONVEX_RADIUS)
		minDim := hx
		if hy < minDim {
			minDim = hy
		}
		if hz < minDim {
			minDim = hz
		}
		if minDim < convexR {
			convexR = minDim * 0.5
		}
		half := C.JPH_Vec3{x: C.float(hx), y: C.float(hy), z: C.float(hz)}
		return (*C.JPH_Shape)(unsafe.Pointer(C.JPH_BoxShape_Create(&half, C.float(convexR))))
	}
}

// GetID returns the string identifier of this rigid body.
func (rb *RigidBody) GetID() string { return rb.ID }

// SetUserData assigns the 32-bit entity user_data associated with this Jolt body.
func (rb *RigidBody) SetUserData(userData uint32) {
	rb.UserData = userData
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface != nil {
		C.JPH_BodyInterface_SetUserData(iface, C.JPH_BodyID(rb.BodyID), C.uint64_t(userData))
	}
}

// GetUserData returns the 32-bit entity user_data stored on the Jolt body.
func (rb *RigidBody) GetUserData() uint32 {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return rb.UserData
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return rb.UserData
	}
	return uint32(C.JPH_BodyInterface_GetUserData(iface, C.JPH_BodyID(rb.BodyID)))
}

// SetObjectLayer configures the collision object layer (updating the live Jolt body if attached).
func (rb *RigidBody) SetObjectLayer(layer uint32) {
	rb.ObjectLayer = layer
	rb.HasCustomLayer = true
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface != nil {
		C.JPH_BodyInterface_SetObjectLayer(iface, C.JPH_BodyID(rb.BodyID), C.JPH_ObjectLayer(layer))
	}
}

// GetBodyType returns whether this body is static, kinematic, or dynamic.
func (rb *RigidBody) GetBodyType() engine.PhysicsBodyType { return rb.BodyType }

// GetPosition returns the current Jolt world-space position of the rigid body.
func (rb *RigidBody) GetPosition() engine.Vector3 {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return rb.initialPos
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return rb.initialPos
	}
	var pos C.JPH_RVec3
	C.JPH_BodyInterface_GetPosition(iface, C.JPH_BodyID(rb.BodyID), &pos)
	return engine.NewVector3(float32(pos.x), float32(pos.y), float32(pos.z))
}

// SetPosition teleports the Jolt rigid body to the target world-space position.
func (rb *RigidBody) SetPosition(pos engine.Vector3) {
	rb.initialPos = pos
	if rb.Owner != nil {
		rb.Owner.Transform.Position = pos
	}
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return
	}
	jp := C.JPH_RVec3{x: C.float(pos.X), y: C.float(pos.Y), z: C.float(pos.Z)}
	C.JPH_BodyInterface_SetPosition(iface, C.JPH_BodyID(rb.BodyID), &jp, C.JPH_Activation_Activate)
}

// GetRotation returns the current body rotation in Euler degrees.
func (rb *RigidBody) GetRotation() engine.Vector3 {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return rb.initialRot
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return rb.initialRot
	}
	var q C.JPH_Quat
	C.JPH_BodyInterface_GetRotation(iface, C.JPH_BodyID(rb.BodyID), &q)
	var eulerRad C.JPH_Vec3
	C.JPH_Quat_GetEulerAngles(&q, &eulerRad)
	const rad2deg = 180.0 / math.Pi
	return engine.NewVector3(
		float32(float64(eulerRad.x)*rad2deg),
		float32(float64(eulerRad.y)*rad2deg),
		float32(float64(eulerRad.z)*rad2deg),
	)
}

// SetRotation sets the rigid body orientation in Euler degrees.
func (rb *RigidBody) SetRotation(rot engine.Vector3) {
	rb.initialRot = rot
	if rb.Owner != nil {
		rb.Owner.Transform.Rotation = rot
	}
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return
	}
	jq := eulerDegToJoltQuat(rot)
	C.JPH_BodyInterface_SetRotation(iface, C.JPH_BodyID(rb.BodyID), &jq, C.JPH_Activation_Activate)
}

// GetVelocity returns the linear velocity of the body.
func (rb *RigidBody) GetVelocity() engine.Vector3 {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return engine.Vector3{}
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return engine.Vector3{}
	}
	var vel C.JPH_Vec3
	C.JPH_BodyInterface_GetLinearVelocity(iface, C.JPH_BodyID(rb.BodyID), &vel)
	return engine.NewVector3(float32(vel.x), float32(vel.y), float32(vel.z))
}

// SetVelocity assigns linear velocity directly on the Jolt body.
func (rb *RigidBody) SetVelocity(vel engine.Vector3) {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return
	}
	jv := C.JoltVec3{x: C.float(vel.X), y: C.float(vel.Y), z: C.float(vel.Z)}
	C.Jolt_SetVelocityVec3(unsafe.Pointer(iface), C.uint32_t(rb.BodyID), &jv)
}

// AddImpulse applies an instantaneous linear impulse to the Jolt body.
func (rb *RigidBody) AddImpulse(impulse engine.Vector3) {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return
	}
	ji := C.JoltVec3{x: C.float(impulse.X), y: C.float(impulse.Y), z: C.float(impulse.Z)}
	C.Jolt_AddImpulseVec3(unsafe.Pointer(iface), C.uint32_t(rb.BodyID), &ji)
}

// IsActive reports whether the Jolt body is currently awake (not sleeping).
func (rb *RigidBody) IsActive() bool {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return false
	}
	rb.World.mu.Lock()
	iface := rb.World.bodyIface
	rb.World.mu.Unlock()
	if iface == nil {
		return false
	}
	return bool(C.JPH_BodyInterface_IsActive(iface, C.JPH_BodyID(rb.BodyID)))
}

// IsGrounded checks whether the body is resting or supported by geometry directly underneath.
func (rb *RigidBody) IsGrounded() bool {
	if rb.World == nil || rb.BodyID == InvalidBodyID {
		return false
	}
	pos := rb.GetPosition()
	bb := rb.GetBoundingBox()
	halfH := (bb.Max.Y - bb.Min.Y) * 0.5
	hit := rb.World.Raycast(engine.Ray{
		Position:  pos,
		Direction: engine.NewVector3(0, -1, 0),
	}, halfH+0.15)
	return hit.Hit
}

// SetGrounded satisfies engine.PhysicsBodyHandle.
func (rb *RigidBody) SetGrounded(_ bool) {}

// GetBoundingBox returns an axis-aligned world-space bounding box around the body.
func (rb *RigidBody) GetBoundingBox() engine.BoundingBox {
	pos := rb.GetPosition()
	var hx, hy, hz float32
	switch rb.Shape.Kind {
	case ShapeSphere:
		hx, hy, hz = rb.Shape.Radius, rb.Shape.Radius, rb.Shape.Radius
	case ShapeCapsule:
		hx, hz = rb.Shape.Radius, rb.Shape.Radius
		hy = rb.Shape.HalfHeight + rb.Shape.Radius
	default:
		hx, hy, hz = rb.Shape.HalfExtents.X, rb.Shape.HalfExtents.Y, rb.Shape.HalfExtents.Z
	}
	return engine.NewBoundingBox(
		engine.NewVector3(pos.X-hx, pos.Y-hy, pos.Z-hz),
		engine.NewVector3(pos.X+hx, pos.Y+hy, pos.Z+hz),
	)
}

// CharacterGroundState mirrors Jolt's JPH_GroundState.
type CharacterGroundState uint8

const (
	GroundOnGround CharacterGroundState = iota
	GroundOnSteepGround
	GroundNotSupported
	GroundInAir
)

// CharacterConfig configures a first-person JPH_CharacterVirtual controller.
type CharacterConfig struct {
	Radius               float32
	HalfHeight           float32
	MaxSlopeAngleDeg     float32
	Mass                 float32
	MaxStrength          float32
	StepUpHeight         float32
	StickToFloorStepDown float32
	Gravity              float32
}

// DefaultCharacterConfig returns standard first-person capsule dimensions and stair-stepping defaults.
func DefaultCharacterConfig() CharacterConfig {
	return CharacterConfig{
		Radius:               0.35,
		HalfHeight:           0.55,
		MaxSlopeAngleDeg:     45.0,
		Mass:                 70.0,
		MaxStrength:          100.0,
		StepUpHeight:         0.35,
		StickToFloorStepDown: 0.45,
		Gravity:              -9.81,
	}
}

// CharacterController is an engine.Component wrapping JPH_CharacterVirtual for smooth
// first-person stair climbing, slope handling, and corner-seam traversal.
type CharacterController struct {
	World     *PhysicsWorld
	Owner     *engine.GameObject
	Config    CharacterConfig
	character *C.JPH_CharacterVirtual
	shape     *C.JPH_Shape

	desiredVel  engine.Vector3
	verticalVel float32
	jumpImpulse float32
	initialPos  engine.Vector3
}

// NewCharacterController constructs a CharacterController bound to world with the given config.
func NewCharacterController(world *PhysicsWorld, cfg CharacterConfig) *CharacterController {
	if cfg.Radius <= 0 {
		cfg.Radius = 0.35
	}
	if cfg.HalfHeight <= 0 {
		cfg.HalfHeight = 0.55
	}
	if cfg.MaxSlopeAngleDeg <= 0 {
		cfg.MaxSlopeAngleDeg = 45.0
	}
	if cfg.Mass <= 0 {
		cfg.Mass = 70.0
	}
	if cfg.MaxStrength <= 0 {
		cfg.MaxStrength = 100.0
	}
	if cfg.Gravity == 0 {
		cfg.Gravity = -9.81
	}
	return &CharacterController{
		World:  world,
		Config: cfg,
	}
}

// Init registers the owning GameObject and creates the JPH_CharacterVirtual instance.
func (cc *CharacterController) Init(owner *engine.GameObject) {
	cc.Owner = owner
	if owner != nil {
		cc.initialPos = owner.Transform.Position
	}
	if cc.World != nil && cc.character == nil {
		cc.AttachToWorld(cc.World)
	}
}

// Update satisfies engine.Component.
func (cc *CharacterController) Update(dt float32) {}

// Destroy removes the CharacterController from PhysicsWorld and frees Jolt resources.
func (cc *CharacterController) Destroy() {
	if cc.World != nil {
		w := cc.World
		w.mu.Lock()
		for i, c := range w.characters {
			if c == cc {
				w.characters = append(w.characters[:i], w.characters[i+1:]...)
				break
			}
		}
		w.mu.Unlock()
	}
	cc.destroyNative()
	cc.World = nil
	cc.Owner = nil
}

func (cc *CharacterController) destroyNative() {
	if cc.character != nil {
		C.JPH_CharacterBase_Destroy((*C.JPH_CharacterBase)(unsafe.Pointer(cc.character)))
		cc.character = nil
	}
	if cc.shape != nil {
		C.JPH_Shape_Destroy(cc.shape)
		cc.shape = nil
	}
}

// AttachToWorld instantiates the JPH_CharacterVirtual inside world.
func (cc *CharacterController) AttachToWorld(world *PhysicsWorld) {
	if world == nil {
		return
	}
	world.mu.Lock()
	if world.closed || world.system == nil {
		world.mu.Unlock()
		return
	}
	if cc.character != nil {
		world.mu.Unlock()
		return
	}
	sys := world.system
	world.mu.Unlock()

	capsule := C.JPH_CapsuleShape_Create(C.float(cc.Config.HalfHeight), C.float(cc.Config.Radius))
	if capsule == nil {
		return
	}
	// Offset capsule so character position is at the feet (y = 0 at bottom of capsule).
	offset := C.JPH_Vec3{x: 0, y: C.float(cc.Config.HalfHeight + cc.Config.Radius), z: 0}
	rotTrans := C.JPH_RotatedTranslatedShape_Create(&offset, nil, (*C.JPH_Shape)(unsafe.Pointer(capsule)))
	C.JPH_Shape_Destroy((*C.JPH_Shape)(unsafe.Pointer(capsule)))
	if rotTrans == nil {
		return
	}
	cc.shape = (*C.JPH_Shape)(unsafe.Pointer(rotTrans))

	var settings C.JPH_CharacterVirtualSettings
	C.JPH_CharacterVirtualSettings_Init(&settings)
	settings.base.shape = cc.shape
	settings.base.maxSlopeAngle = C.float(float64(cc.Config.MaxSlopeAngleDeg) * math.Pi / 180.0)
	settings.base.enhancedInternalEdgeRemoval = C.bool(true)
	settings.base.supportingVolume = C.JPH_Plane{
		normal:   C.JPH_Vec3{x: 0, y: 1, z: 0},
		distance: C.float(-cc.Config.Radius),
	}
	settings.mass = C.float(cc.Config.Mass)
	settings.maxStrength = C.float(cc.Config.MaxStrength)

	pos := cc.initialPos
	if cc.Owner != nil {
		pos = cc.Owner.Transform.Position
	}
	jPos := C.JPH_RVec3{x: C.float(pos.X), y: C.float(pos.Y), z: C.float(pos.Z)}
	jRot := C.JPH_Quat{x: 0, y: 0, z: 0, w: 1}

	ch := C.JPH_CharacterVirtual_Create(&settings, &jPos, &jRot, 0, sys)
	if ch == nil {
		C.JPH_Shape_Destroy(cc.shape)
		cc.shape = nil
		return
	}

	cc.World = world
	cc.character = ch

	world.mu.Lock()
	if !world.closed {
		world.characters = append(world.characters, cc)
	}
	world.mu.Unlock()
}

// SetDesiredVelocity sets the target horizontal/3D movement velocity (m/s) for the character.
func (cc *CharacterController) SetDesiredVelocity(vel engine.Vector3) {
	cc.desiredVel = vel
}

// Jump triggers an upward velocity impulse on the next physics step if grounded.
func (cc *CharacterController) Jump(speed float32) {
	cc.jumpImpulse = speed
}

// SetPosition teleports the CharacterVirtual to the specified world position.
func (cc *CharacterController) SetPosition(pos engine.Vector3) {
	cc.initialPos = pos
	if cc.Owner != nil {
		cc.Owner.Transform.Position = pos
	}
	if cc.character != nil {
		jp := C.JPH_RVec3{x: C.float(pos.X), y: C.float(pos.Y), z: C.float(pos.Z)}
		C.JPH_CharacterVirtual_SetPosition(cc.character, &jp)
	}
}

// GetPosition returns the character's current world-space position.
func (cc *CharacterController) GetPosition() engine.Vector3 {
	if cc.character == nil {
		return cc.initialPos
	}
	return cc.getPositionNative()
}

func (cc *CharacterController) getPositionNative() engine.Vector3 {
	var pos C.JPH_RVec3
	C.JPH_CharacterVirtual_GetPosition(cc.character, &pos)
	return engine.NewVector3(float32(pos.x), float32(pos.y), float32(pos.z))
}

// GetVelocity returns the character's current linear velocity.
func (cc *CharacterController) GetVelocity() engine.Vector3 {
	if cc.character == nil {
		return engine.Vector3{}
	}
	var v C.JPH_Vec3
	C.JPH_CharacterVirtual_GetLinearVelocity(cc.character, &v)
	return engine.NewVector3(float32(v.x), float32(v.y), float32(v.z))
}

// GroundState returns the character's current ground contact state.
func (cc *CharacterController) GroundState() CharacterGroundState {
	if cc.character == nil {
		return GroundInAir
	}
	gs := C.JPH_CharacterBase_GetGroundState((*C.JPH_CharacterBase)(unsafe.Pointer(cc.character)))
	return CharacterGroundState(gs)
}

// IsGrounded reports whether the character is supported on walkable ground.
func (cc *CharacterController) IsGrounded() bool {
	return cc.GroundState() == GroundOnGround
}

func (cc *CharacterController) stepLocked(sys *C.JPH_PhysicsSystem, dt float32) {
	if cc.character == nil || sys == nil {
		return
	}

	C.JPH_CharacterVirtual_UpdateGroundVelocity(cc.character)
	gs := cc.GroundState()

	if gs == GroundOnGround {
		if cc.jumpImpulse > 0 {
			cc.verticalVel = cc.jumpImpulse
			cc.jumpImpulse = 0
		} else {
			cc.verticalVel = 0
		}
	} else {
		cc.jumpImpulse = 0
		cc.verticalVel += cc.Config.Gravity * dt
	}

	vel := C.JPH_Vec3{
		x: C.float(cc.desiredVel.X),
		y: C.float(cc.verticalVel),
		z: C.float(cc.desiredVel.Z),
	}
	C.JPH_CharacterVirtual_SetLinearVelocity(cc.character, &vel)

	var ext C.JPH_ExtendedUpdateSettings
	ext.stickToFloorStepDown = C.JPH_Vec3{x: 0, y: C.float(-cc.Config.StickToFloorStepDown), z: 0}
	ext.walkStairsStepUp = C.JPH_Vec3{x: 0, y: C.float(cc.Config.StepUpHeight), z: 0}
	ext.walkStairsMinStepForward = C.float(0.02)
	ext.walkStairsStepForwardTest = C.float(0.15)
	ext.walkStairsCosAngleForwardContact = C.float(0.75)
	ext.walkStairsStepDownExtra = C.JPH_Vec3{x: 0, y: 0, z: 0}

	C.JPH_CharacterVirtual_ExtendedUpdate(
		cc.character,
		C.float(dt),
		&ext,
		C.JPH_ObjectLayer(LayerMoving),
		sys,
		nil,
		nil,
	)
}
