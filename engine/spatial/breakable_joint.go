package spatial

/*
#include <stdlib.h>
#include "joltc.h"
*/
import "C"

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/SamSoupSauce/project-engine/engine"
)

// JointKind distinguishes prismatic slider joints (drawers) from revolute hinge joints (doors/lids).
type JointKind uint8

const (
	JointSlider JointKind = iota
	JointHinge
)

// JointDesc configures a breakable Jolt slider or hinge constraint between two bodies.
type JointDesc struct {
	ID          string
	Kind        JointKind
	Anchor      engine.Vector3
	Axis        engine.Vector3
	NormalAxis  engine.Vector3
	MinLimit    float32
	MaxLimit    float32
	MaxFriction float32
	BreakForce  float32 // Newtons threshold before constraint breaks (<= 0 disables)
	BreakTorque float32 // N*m threshold before constraint breaks (<= 0 disables)
}

// ArmSocketBinding describes a player's active hand/arm socket pulling or holding a jointed part.
type ArmSocketBinding struct {
	Active            bool
	CarryTarget       engine.Vector3
	AutoAttachOnBreak bool
}

// JointBreakEvent is emitted when a BreakableJoint exceeds BreakForce or BreakTorque and snaps.
type JointBreakEvent struct {
	Joint               *BreakableJoint
	ParentBody          *RigidBody
	ChildBody           *RigidBody
	PullVector          engine.Vector3
	SpilledBodies       []*RigidBody
	AttachedToArmSocket bool
	Force               float32
	Torque              float32
}

// BreakableJoint wraps a JPH_SliderConstraint or JPH_HingeConstraint with dynamic fracture limits.
type BreakableJoint struct {
	ID             string
	Kind           JointKind
	World          *PhysicsWorld
	ParentBody     *RigidBody
	ChildBody      *RigidBody
	BreakForce     float32
	BreakTorque    float32
	Broken         bool
	LastForce      float32
	LastTorque     float32
	Axis           engine.Vector3
	ContainedProps []*RigidBody
	ArmSocket      ArmSocketBinding
	OnBreak        func(ev JointBreakEvent)

	constraint    *C.JPH_Constraint
	rawSlider     *C.JPH_SliderConstraint
	rawHinge      *C.JPH_HingeConstraint
	pendingPull   engine.Vector3
	preSnapLinVel engine.Vector3
	preSnapAngVel engine.Vector3
}

// CreateBreakableJoint creates and registers a Jolt slider or hinge constraint with tensile/torque limits.
func (w *PhysicsWorld) CreateBreakableJoint(parent, child *RigidBody, desc JointDesc) (*BreakableJoint, error) {
	if w == nil {
		return nil, fmt.Errorf("spatial: nil PhysicsWorld")
	}
	if parent == nil || parent.BodyID == InvalidBodyID || child == nil || child.BodyID == InvalidBodyID {
		return nil, fmt.Errorf("spatial: invalid parent or child RigidBody for joint %q", desc.ID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.system == nil || w.bodyIface == nil {
		return nil, fmt.Errorf("spatial: PhysicsWorld is closed")
	}

	axis := normVec3(desc.Axis)
	if axis.X == 0 && axis.Y == 0 && axis.Z == 0 {
		axis = engine.NewVector3(0, 0, 1)
	}
	normal := desc.NormalAxis
	if normal.X == 0 && normal.Y == 0 && normal.Z == 0 {
		normal = orthogonalUnitVec3(axis)
	} else {
		normal = orthogonalizeAgainst(normVec3(normal), axis)
	}

	lockIface := C.JPH_PhysicsSystem_GetBodyLockInterface(w.system)
	pair := [2]C.JPH_BodyID{C.JPH_BodyID(parent.BodyID), C.JPH_BodyID(child.BodyID)}
	multiLock := C.JPH_BodyLockInterface_LockMultiWrite(lockIface, &pair[0], 2)
	if multiLock == nil {
		return nil, fmt.Errorf("spatial: LockMultiWrite failed for joint %q", desc.ID)
	}

	b1 := C.JPH_BodyLockMultiWrite_GetBody(multiLock, 0)
	b2 := C.JPH_BodyLockMultiWrite_GetBody(multiLock, 1)
	if b1 == nil || b2 == nil {
		C.JPH_BodyLockMultiWrite_Destroy(multiLock)
		return nil, fmt.Errorf("spatial: failed to lock bodies for joint %q", desc.ID)
	}

	var constraint *C.JPH_Constraint
	var sliderPtr *C.JPH_SliderConstraint
	var hingePtr *C.JPH_HingeConstraint

	anchor := C.JPH_RVec3{x: C.float(desc.Anchor.X), y: C.float(desc.Anchor.Y), z: C.float(desc.Anchor.Z)}
	jAxis := C.JPH_Vec3{x: C.float(axis.X), y: C.float(axis.Y), z: C.float(axis.Z)}
	jNorm := C.JPH_Vec3{x: C.float(normal.X), y: C.float(normal.Y), z: C.float(normal.Z)}

	switch desc.Kind {
	case JointSlider:
		var settings C.JPH_SliderConstraintSettings
		C.JPH_SliderConstraintSettings_Init(&settings)
		settings.space = C.JPH_ConstraintSpace_WorldSpace
		settings.autoDetectPoint = C.bool(false)
		settings.point1 = anchor
		settings.point2 = anchor
		C.JPH_SliderConstraintSettings_SetSliderAxis(&settings, &jAxis)
		if desc.MaxLimit > desc.MinLimit {
			settings.limitsMin = C.float(desc.MinLimit)
			settings.limitsMax = C.float(desc.MaxLimit)
		}
		if desc.MaxFriction > 0 {
			settings.maxFrictionForce = C.float(desc.MaxFriction)
		}
		sliderPtr = C.JPH_SliderConstraint_Create(&settings, b1, b2)
		if sliderPtr != nil {
			constraint = (*C.JPH_Constraint)(unsafe.Pointer(sliderPtr))
		}
	case JointHinge:
		var settings C.JPH_HingeConstraintSettings
		C.JPH_HingeConstraintSettings_Init(&settings)
		settings.space = C.JPH_ConstraintSpace_WorldSpace
		settings.point1 = anchor
		settings.hingeAxis1 = jAxis
		settings.normalAxis1 = jNorm
		settings.point2 = anchor
		settings.hingeAxis2 = jAxis
		settings.normalAxis2 = jNorm
		if desc.MaxLimit > desc.MinLimit {
			settings.limitsMin = C.float(desc.MinLimit)
			settings.limitsMax = C.float(desc.MaxLimit)
		}
		if desc.MaxFriction > 0 {
			settings.maxFrictionTorque = C.float(desc.MaxFriction)
		}
		hingePtr = C.JPH_HingeConstraint_Create(&settings, b1, b2)
		if hingePtr != nil {
			constraint = (*C.JPH_Constraint)(unsafe.Pointer(hingePtr))
		}
	default:
		C.JPH_BodyLockMultiWrite_Destroy(multiLock)
		return nil, fmt.Errorf("spatial: unknown JointKind %d", desc.Kind)
	}

	C.JPH_BodyLockMultiWrite_Destroy(multiLock)
	if constraint == nil {
		return nil, fmt.Errorf("spatial: failed to create Jolt constraint for joint %q", desc.ID)
	}

	C.JPH_PhysicsSystem_AddConstraint(w.system, constraint)
	C.JPH_BodyInterface_ActivateBody(w.bodyIface, C.JPH_BodyID(child.BodyID))

	j := &BreakableJoint{
		ID:          desc.ID,
		Kind:        desc.Kind,
		World:       w,
		ParentBody:  parent,
		ChildBody:   child,
		BreakForce:  desc.BreakForce,
		BreakTorque: desc.BreakTorque,
		Axis:        axis,
		constraint:  constraint,
		rawSlider:   sliderPtr,
		rawHinge:    hingePtr,
	}
	w.joints = append(w.joints, j)
	return j, nil
}

// GetBreakableJoint looks up a joint registered on this PhysicsWorld by ID.
func (w *PhysicsWorld) GetBreakableJoint(id string) *BreakableJoint {
	if w == nil || id == "" {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, j := range w.joints {
		if j != nil && j.ID == id {
			return j
		}
	}
	return nil
}

// AddContainedProp registers a loose internal RigidBody whose momentum is preserved and spilled when the joint snaps.
func (j *BreakableJoint) AddContainedProp(rb *RigidBody) {
	if j == nil || rb == nil {
		return
	}
	j.ContainedProps = append(j.ContainedProps, rb)
}

// SetArmSocket configures active player arm-socket attachment when this joint breaks mid-pull.
func (j *BreakableJoint) SetArmSocket(active bool, carryTarget engine.Vector3) {
	if j == nil {
		return
	}
	j.ArmSocket = ArmSocketBinding{
		Active:            active,
		CarryTarget:       carryTarget,
		AutoAttachOnBreak: active,
	}
}

// ApplyPullForce applies a pulling force/impulse to the child part along pullDir and tracks the pull vector.
func (j *BreakableJoint) ApplyPullForce(pullDir engine.Vector3, forceNewtons float32, dt float32) {
	if j == nil || j.Broken || j.World == nil || j.ChildBody == nil || j.ChildBody.BodyID == InvalidBodyID {
		return
	}
	dir := normVec3(pullDir)
	if dir.X == 0 && dir.Y == 0 && dir.Z == 0 {
		return
	}
	if dt <= 0 {
		dt = DefaultFixedTimestep
	}

	j.World.mu.Lock()
	defer j.World.mu.Unlock()
	if j.World.closed || j.World.bodyIface == nil || j.Broken {
		return
	}

	j.pendingPull = dir
	j.capturePreSnapVelocitiesLocked()

	C.JPH_BodyInterface_ActivateBody(j.World.bodyIface, C.JPH_BodyID(j.ChildBody.BodyID))
	impulseMag := forceNewtons * dt
	jImp := C.JPH_Vec3{
		x: C.float(dir.X * impulseMag),
		y: C.float(dir.Y * impulseMag),
		z: C.float(dir.Z * impulseMag),
	}
	C.JPH_BodyInterface_AddImpulse(j.World.bodyIface, C.JPH_BodyID(j.ChildBody.BodyID), &jImp)
}

// Break explicitly snaps the constraint, converts ChildBody to a free dynamic body, transfers to arm socket if held,
// and spills contained internal items preserving velocity along pullDir.
func (j *BreakableJoint) Break(pullDir engine.Vector3) JointBreakEvent {
	if j == nil || j.World == nil {
		return JointBreakEvent{}
	}
	j.World.mu.Lock()
	defer j.World.mu.Unlock()
	return j.breakLocked(pullDir, j.LastForce, j.LastTorque)
}

func (j *BreakableJoint) capturePreSnapVelocitiesLocked() {
	if j == nil || j.World == nil || j.World.bodyIface == nil || j.ChildBody == nil || j.ChildBody.BodyID == InvalidBodyID {
		return
	}
	var lv, av C.JPH_Vec3
	C.JPH_BodyInterface_GetLinearVelocity(j.World.bodyIface, C.JPH_BodyID(j.ChildBody.BodyID), &lv)
	C.JPH_BodyInterface_GetAngularVelocity(j.World.bodyIface, C.JPH_BodyID(j.ChildBody.BodyID), &av)
	j.preSnapLinVel = engine.NewVector3(float32(lv.x), float32(lv.y), float32(lv.z))
	j.preSnapAngVel = engine.NewVector3(float32(av.x), float32(av.y), float32(av.z))
}

func (j *BreakableJoint) evaluateTensionLocked(dt float32) {
	if j == nil || j.Broken || j.constraint == nil || dt <= 0 {
		return
	}

	var force, torque float32
	switch j.Kind {
	case JointSlider:
		if j.rawSlider != nil {
			var pos [2]C.float
			C.JPH_SliderConstraint_GetTotalLambdaPosition(j.rawSlider, &pos[0])
			posLimit := float32(C.JPH_SliderConstraint_GetTotalLambdaPositionLimits(j.rawSlider))
			var rot C.JPH_Vec3
			C.JPH_SliderConstraint_GetTotalLambdaRotation(j.rawSlider, &rot)
			p0, p1 := float32(pos[0]), float32(pos[1])
			force = float32(math.Sqrt(float64(p0*p0+p1*p1+posLimit*posLimit))) / dt
			rx, ry, rz := float32(rot.x), float32(rot.y), float32(rot.z)
			torque = float32(math.Sqrt(float64(rx*rx+ry*ry+rz*rz))) / dt
		}
	case JointHinge:
		if j.rawHinge != nil {
			var pos C.JPH_Vec3
			C.JPH_HingeConstraint_GetTotalLambdaPosition(j.rawHinge, &pos)
			var rot [2]C.float
			C.JPH_HingeConstraint_GetTotalLambdaRotation(j.rawHinge, &rot[0])
			rotLimit := float32(C.JPH_HingeConstraint_GetTotalLambdaRotationLimits(j.rawHinge))
			px, py, pz := float32(pos.x), float32(pos.y), float32(pos.z)
			force = float32(math.Sqrt(float64(px*px+py*py+pz*pz))) / dt
			r0, r1 := float32(rot[0]), float32(rot[1])
			torque = float32(math.Sqrt(float64(r0*r0+r1*r1+rotLimit*rotLimit))) / dt
		}
	}

	j.LastForce = force
	j.LastTorque = torque

	overForce := j.BreakForce > 0 && force >= j.BreakForce
	overTorque := j.BreakTorque > 0 && torque >= j.BreakTorque
	if overForce || overTorque {
		j.breakLocked(j.pendingPull, force, torque)
	}
}

func (j *BreakableJoint) breakLocked(pullDir engine.Vector3, force, torque float32) JointBreakEvent {
	if j == nil || j.Broken {
		return JointBreakEvent{}
	}
	j.Broken = true

	if j.constraint != nil && j.World != nil && j.World.system != nil {
		C.JPH_PhysicsSystem_RemoveConstraint(j.World.system, j.constraint)
		C.JPH_Constraint_Destroy(j.constraint)
		j.constraint = nil
		j.rawSlider = nil
		j.rawHinge = nil
	}

	dir := normVec3(pullDir)
	if dir.X == 0 && dir.Y == 0 && dir.Z == 0 {
		dir = normVec3(j.preSnapLinVel)
	}
	if dir.X == 0 && dir.Y == 0 && dir.Z == 0 {
		dir = normVec3(j.Axis)
	}

	// Ensure child drawer/door becomes a free dynamic body and detach hierarchy parenting cleanly.
	if j.ChildBody != nil && j.World != nil && j.World.bodyIface != nil && j.ChildBody.BodyID != InvalidBodyID {
		j.ChildBody.BodyType = engine.BodyDynamic
		C.JPH_BodyInterface_SetMotionType(
			j.World.bodyIface,
			C.JPH_BodyID(j.ChildBody.BodyID),
			C.JPH_MotionType_Dynamic,
			C.JPH_Activation_Activate,
		)
		C.JPH_BodyInterface_ActivateBody(j.World.bodyIface, C.JPH_BodyID(j.ChildBody.BodyID))

		if j.ChildBody.Owner != nil && j.ParentBody != nil && j.ParentBody.Owner != nil &&
			j.ChildBody.Owner.Parent == j.ParentBody.Owner {
			worldT := j.ChildBody.Owner.GetWorldTransform()
			j.ParentBody.Owner.RemoveChild(j.ChildBody.Owner)
			j.ChildBody.Owner.EnsureTransform().Transform = worldT
		}
	}

	// Preserve and propagate momentum onto any contained loose dynamic items so they spill naturally.
	spillVel := j.preSnapLinVel
	if spillVel.X == 0 && spillVel.Y == 0 && spillVel.Z == 0 {
		spillVel = engine.NewVector3(dir.X*4.5, dir.Y*4.5+0.8, dir.Z*4.5)
	}
	spilled := make([]*RigidBody, 0, len(j.ContainedProps))
	for i, item := range j.ContainedProps {
		if item == nil || item.BodyID == InvalidBodyID || j.World == nil || j.World.bodyIface == nil {
			continue
		}
		if item.Owner != nil && j.ChildBody != nil && j.ChildBody.Owner != nil && item.Owner.Parent == j.ChildBody.Owner {
			worldT := item.Owner.GetWorldTransform()
			j.ChildBody.Owner.RemoveChild(item.Owner)
			item.Owner.EnsureTransform().Transform = worldT
		}
		item.BodyType = engine.BodyDynamic
		C.JPH_BodyInterface_SetMotionType(
			j.World.bodyIface,
			C.JPH_BodyID(item.BodyID),
			C.JPH_MotionType_Dynamic,
			C.JPH_Activation_Activate,
		)
		C.JPH_BodyInterface_ActivateBody(j.World.bodyIface, C.JPH_BodyID(item.BodyID))

		var curLv C.JPH_Vec3
		C.JPH_BodyInterface_GetLinearVelocity(j.World.bodyIface, C.JPH_BodyID(item.BodyID), &curLv)
		// Preserve existing velocity if significant; add parent pull velocity & scatter.
		scatter := float32((i%3)-1) * 0.65
		outVel := C.JPH_Vec3{
			x: C.float(float32(curLv.x) + spillVel.X + scatter),
			y: C.float(float32(curLv.y) + spillVel.Y + 0.5),
			z: C.float(float32(curLv.z) + spillVel.Z),
		}
		C.JPH_BodyInterface_SetLinearVelocity(j.World.bodyIface, C.JPH_BodyID(item.BodyID), &outVel)
		C.JPH_BodyInterface_SetAngularVelocity(j.World.bodyIface, C.JPH_BodyID(item.BodyID), &C.JPH_Vec3{
			x: C.float(j.preSnapAngVel.X + scatter),
			y: C.float(j.preSnapAngVel.Y + 1.2),
			z: C.float(j.preSnapAngVel.Z),
		})
		spilled = append(spilled, item)
	}

	// If held via player's active arm socket when snapping, attach detached ChildBody to arm socket carry anchor.
	attachedArm := false
	if j.ArmSocket.Active && j.ArmSocket.AutoAttachOnBreak && j.ChildBody != nil && j.World != nil {
		if j.World.activeGrab == nil {
			j.World.activeGrab = &PhysicsGrab{
				world:        j.World,
				anchorBodyID: InvalidBodyID,
				holdSmooth:   14.0,
			}
		}
		if err := j.World.activeGrab.startLocked(j.ChildBody, j.ArmSocket.CarryTarget); err == nil {
			attachedArm = true
		}
	}

	ev := JointBreakEvent{
		Joint:               j,
		ParentBody:          j.ParentBody,
		ChildBody:           j.ChildBody,
		PullVector:          dir,
		SpilledBodies:       spilled,
		AttachedToArmSocket: attachedArm,
		Force:               force,
		Torque:              torque,
	}
	if j.OnBreak != nil {
		j.OnBreak(ev)
	}
	return ev
}

func normVec3(v engine.Vector3) engine.Vector3 {
	m := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if m < 1e-6 {
		return engine.Vector3{}
	}
	return engine.NewVector3(v.X/m, v.Y/m, v.Z/m)
}

func orthogonalUnitVec3(axis engine.Vector3) engine.Vector3 {
	up := engine.NewVector3(0, 1, 0)
	if math.Abs(float64(axis.Y)) > 0.9 {
		up = engine.NewVector3(1, 0, 0)
	}
	return orthogonalizeAgainst(up, axis)
}

func orthogonalizeAgainst(v, axis engine.Vector3) engine.Vector3 {
	dot := v.X*axis.X + v.Y*axis.Y + v.Z*axis.Z
	perp := engine.NewVector3(v.X-axis.X*dot, v.Y-axis.Y*dot, v.Z-axis.Z*dot)
	return normVec3(perp)
}
