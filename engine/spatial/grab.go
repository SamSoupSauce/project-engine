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

// PhysicsGrab manages a Jolt kinematic carry anchor and JPH_PointConstraint for holding
// and throwing dynamic RigidBody props in front of the player camera.
type PhysicsGrab struct {
	world        *PhysicsWorld
	anchorBodyID uint32
	heldBody     *RigidBody
	constraint   *C.JPH_Constraint
	targetPos    engine.Vector3
	holdSmooth   float32
}

// FindBodyByRayHit resolves a *RigidBody matching a RaycastHit by string ID, numeric BodyID, or UserData.
func (w *PhysicsWorld) FindBodyByRayHit(hit engine.RaycastHit) *RigidBody {
	if w == nil || !hit.Hit {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if rb, ok := w.bodiesByID[hit.BodyID]; ok && rb != nil {
		return rb
	}
	for _, rb := range w.bodies {
		if rb == nil {
			continue
		}
		if hit.UserData != 0 && rb.UserData == hit.UserData {
			return rb
		}
		if fmt.Sprintf("%d", rb.BodyID) == hit.BodyID {
			return rb
		}
	}
	return nil
}

// GetSyncTarget returns the GameObject synchronized to a numeric Jolt BodyID, or nil.
func (w *PhysicsWorld) GetSyncTarget(bodyID uint32) *engine.GameObject {
	if w == nil || bodyID == InvalidBodyID {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.syncTargets[bodyID]
}

// GrabBody attaches a dynamic RigidBody to a Jolt kinematic carry anchor at targetPos.
func (w *PhysicsWorld) GrabBody(body *RigidBody, targetPos engine.Vector3) error {
	if w == nil {
		return fmt.Errorf("spatial: nil PhysicsWorld")
	}
	if body == nil || body.BodyID == InvalidBodyID {
		return fmt.Errorf("spatial: invalid RigidBody for grab")
	}
	if body.BodyType != engine.BodyDynamic {
		return fmt.Errorf("spatial: cannot physics-grab non-dynamic body %q", body.ID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.system == nil || w.bodyIface == nil {
		return fmt.Errorf("spatial: PhysicsWorld is closed")
	}

	if w.activeGrab == nil {
		w.activeGrab = &PhysicsGrab{
			world:        w,
			anchorBodyID: InvalidBodyID,
			holdSmooth:   14.0,
		}
	}
	return w.activeGrab.startLocked(body, targetPos)
}

// UpdateGrabTarget updates the world-space carry target position for the currently held prop.
func (w *PhysicsWorld) UpdateGrabTarget(targetPos engine.Vector3) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.activeGrab != nil && w.activeGrab.heldBody != nil {
		w.activeGrab.targetPos = targetPos
	}
}

// ReleaseGrab gently releases any active physics grab constraint without throwing impulse.
func (w *PhysicsWorld) ReleaseGrab() *RigidBody {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.activeGrab == nil {
		return nil
	}
	return w.activeGrab.releaseLocked(engine.Vector3{}, 0)
}

// ThrowGrab releases any active physics grab constraint and applies linear throw impulse along dir.
func (w *PhysicsWorld) ThrowGrab(dir engine.Vector3, impulse float32) *RigidBody {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.activeGrab == nil {
		return nil
	}
	return w.activeGrab.releaseLocked(dir, impulse)
}

// HeldBody returns the currently grabbed *RigidBody, or nil if nothing is held.
func (w *PhysicsWorld) HeldBody() *RigidBody {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.activeGrab == nil {
		return nil
	}
	return w.activeGrab.heldBody
}

func (g *PhysicsGrab) ensureAnchorLocked(initialPos engine.Vector3) error {
	if g.anchorBodyID != InvalidBodyID {
		return nil
	}
	shape := C.JPH_SphereShape_Create(C.float(0.05))
	if shape == nil {
		return fmt.Errorf("spatial: failed to create grab anchor shape")
	}
	defer C.JPH_Shape_Destroy((*C.JPH_Shape)(unsafe.Pointer(shape)))

	jPos := C.JPH_RVec3{x: C.float(initialPos.X), y: C.float(initialPos.Y), z: C.float(initialPos.Z)}
	jRot := C.JPH_Quat{x: 0, y: 0, z: 0, w: 1}
	bcs := C.JPH_BodyCreationSettings_Create3(
		(*C.JPH_Shape)(unsafe.Pointer(shape)),
		&jPos,
		&jRot,
		C.JPH_MotionType_Kinematic,
		C.JPH_ObjectLayer(LayerMoving),
	)
	if bcs == nil {
		return fmt.Errorf("spatial: failed to create grab anchor body settings")
	}
	defer C.JPH_BodyCreationSettings_Destroy(bcs)

	// Carry anchor must be a sensor so it never collides physically with world geometry.
	C.JPH_BodyCreationSettings_SetIsSensor(bcs, C.bool(true))
	id := uint32(C.JPH_BodyInterface_CreateAndAddBody(g.world.bodyIface, bcs, C.JPH_Activation_Activate))
	if id == InvalidBodyID {
		return fmt.Errorf("spatial: failed to spawn grab anchor body")
	}
	g.anchorBodyID = id
	return nil
}

func (g *PhysicsGrab) startLocked(body *RigidBody, targetPos engine.Vector3) error {
	if g.heldBody != nil {
		g.releaseLocked(engine.Vector3{}, 0)
	}

	var propPos C.JPH_RVec3
	C.JPH_BodyInterface_GetPosition(g.world.bodyIface, C.JPH_BodyID(body.BodyID), &propPos)

	if err := g.ensureAnchorLocked(engine.NewVector3(float32(propPos.x), float32(propPos.y), float32(propPos.z))); err != nil {
		return err
	}

	jZeroRot := C.JPH_Quat{x: 0, y: 0, z: 0, w: 1}
	C.JPH_BodyInterface_SetPositionAndRotation(
		g.world.bodyIface,
		C.JPH_BodyID(g.anchorBodyID),
		&propPos,
		&jZeroRot,
		C.JPH_Activation_Activate,
	)

	// Wake prop, zero residual spin, and suspend gravity while tethered cleanly to the anchor.
	C.JPH_BodyInterface_ActivateBody(g.world.bodyIface, C.JPH_BodyID(body.BodyID))
	C.JPH_BodyInterface_SetGravityFactor(g.world.bodyIface, C.JPH_BodyID(body.BodyID), C.float(0))
	zeroVec := C.JPH_Vec3{x: 0, y: 0, z: 0}
	C.JPH_BodyInterface_SetAngularVelocity(g.world.bodyIface, C.JPH_BodyID(body.BodyID), &zeroVec)

	lockIface := C.JPH_PhysicsSystem_GetBodyLockInterface(g.world.system)
	pair := [2]C.JPH_BodyID{C.JPH_BodyID(g.anchorBodyID), C.JPH_BodyID(body.BodyID)}
	multiLock := C.JPH_BodyLockInterface_LockMultiWrite(lockIface, &pair[0], 2)
	if multiLock == nil {
		return fmt.Errorf("spatial: LockMultiWrite failed for grab constraint")
	}

	b1 := C.JPH_BodyLockMultiWrite_GetBody(multiLock, 0)
	b2 := C.JPH_BodyLockMultiWrite_GetBody(multiLock, 1)
	if b1 == nil || b2 == nil {
		C.JPH_BodyLockMultiWrite_Destroy(multiLock)
		return fmt.Errorf("spatial: failed to lock bodies for grab constraint")
	}

	var settings C.JPH_PointConstraintSettings
	C.JPH_PointConstraintSettings_Init(&settings)
	settings.space = C.JPH_ConstraintSpace_WorldSpace
	settings.point1 = propPos
	settings.point2 = propPos

	ptConstraint := C.JPH_PointConstraint_Create(&settings, b1, b2)
	C.JPH_BodyLockMultiWrite_Destroy(multiLock)
	if ptConstraint == nil {
		return fmt.Errorf("spatial: JPH_PointConstraint_Create failed")
	}

	constraint := (*C.JPH_Constraint)(unsafe.Pointer(ptConstraint))
	C.JPH_PhysicsSystem_AddConstraint(g.world.system, constraint)

	g.constraint = constraint
	g.heldBody = body
	g.targetPos = targetPos
	return nil
}

func (g *PhysicsGrab) stepLocked(dt float32) {
	if g == nil || g.heldBody == nil || g.anchorBodyID == InvalidBodyID || g.world == nil || g.world.bodyIface == nil || dt <= 0 {
		return
	}

	// Move kinematic anchor smoothly toward carry target each substep.
	var anchorPos C.JPH_RVec3
	C.JPH_BodyInterface_GetPosition(g.world.bodyIface, C.JPH_BodyID(g.anchorBodyID), &anchorPos)

	alpha := g.holdSmooth * dt
	if alpha > 1.0 {
		alpha = 1.0
	}
	nextPos := C.JPH_RVec3{
		x: anchorPos.x + (C.float(g.targetPos.X)-anchorPos.x)*C.float(alpha),
		y: anchorPos.y + (C.float(g.targetPos.Y)-anchorPos.y)*C.float(alpha),
		z: anchorPos.z + (C.float(g.targetPos.Z)-anchorPos.z)*C.float(alpha),
	}
	rot := C.JPH_Quat{x: 0, y: 0, z: 0, w: 1}
	C.JPH_BodyInterface_MoveKinematic(g.world.bodyIface, C.JPH_BodyID(g.anchorBodyID), &nextPos, &rot, C.float(dt))
	C.JPH_BodyInterface_ActivateBody(g.world.bodyIface, C.JPH_BodyID(g.heldBody.BodyID))
}

func (g *PhysicsGrab) releaseLocked(throwDir engine.Vector3, impulse float32) *RigidBody {
	if g == nil {
		return nil
	}
	if g.constraint != nil && g.world != nil && g.world.system != nil {
		C.JPH_PhysicsSystem_RemoveConstraint(g.world.system, g.constraint)
		C.JPH_Constraint_Destroy(g.constraint)
		g.constraint = nil
	}
	released := g.heldBody
	g.heldBody = nil

	if released != nil && released.BodyID != InvalidBodyID && g.world != nil && g.world.bodyIface != nil {
		gravFactor := released.GravityFactor
		if gravFactor == 0 {
			gravFactor = 1.0
		}
		C.JPH_BodyInterface_SetGravityFactor(g.world.bodyIface, C.JPH_BodyID(released.BodyID), C.float(gravFactor))
		C.JPH_BodyInterface_ActivateBody(g.world.bodyIface, C.JPH_BodyID(released.BodyID))

		if impulse > 0 {
			mag := float32(math.Sqrt(float64(throwDir.X*throwDir.X + throwDir.Y*throwDir.Y + throwDir.Z*throwDir.Z)))
			if mag > 1e-6 {
				nx, ny, nz := throwDir.X/mag, throwDir.Y/mag, throwDir.Z/mag
				jImp := C.JPH_Vec3{
					x: C.float(nx * impulse),
					y: C.float(ny * impulse),
					z: C.float(nz * impulse),
				}
				C.JPH_BodyInterface_AddImpulse(g.world.bodyIface, C.JPH_BodyID(released.BodyID), &jImp)
			}
		}
	}
	return released
}

func (g *PhysicsGrab) destroyLocked() {
	if g == nil {
		return
	}
	g.releaseLocked(engine.Vector3{}, 0)
	if g.anchorBodyID != InvalidBodyID && g.world != nil && g.world.bodyIface != nil {
		if bool(C.JPH_BodyInterface_IsAdded(g.world.bodyIface, C.JPH_BodyID(g.anchorBodyID))) {
			C.JPH_BodyInterface_RemoveAndDestroyBody(g.world.bodyIface, C.JPH_BodyID(g.anchorBodyID))
		}
		g.anchorBodyID = InvalidBodyID
	}
}
