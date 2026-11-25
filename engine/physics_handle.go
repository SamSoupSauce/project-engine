package engine

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// PhysicsBodyType distinguishes dynamic, kinematic, and static bodies.
type PhysicsBodyType int

const (
	BodyStatic PhysicsBodyType = iota
	BodyKinematic
	BodyDynamic
)

// PhysicsBodyHandle defines an abstract handle for interacting with physics simulations
// (such as Jolt Physics, PhysX, or lightweight custom solvers) without concrete coupling.
type PhysicsBodyHandle interface {
	GetID() string
	GetBodyType() PhysicsBodyType
	GetPosition() rl.Vector3
	SetPosition(pos rl.Vector3)
	GetRotation() rl.Vector3
	SetRotation(rot rl.Vector3)
	GetVelocity() rl.Vector3
	SetVelocity(vel rl.Vector3)
	IsGrounded() bool
	SetGrounded(grounded bool)
	GetBoundingBox() rl.BoundingBox
}

// RaycastHit describes the outcome of an abstract raycast query.
type RaycastHit struct {
	Hit      bool
	Distance float32
	Point    rl.Vector3
	Normal   rl.Vector3
	BodyID   string
}

// PhysicsWorldHandle defines an abstract interface for a 3D physics environment.
type PhysicsWorldHandle interface {
	Step(dt float32)
	AddBody(body PhysicsBodyHandle)
	RemoveBody(id string)
	GetBody(id string) PhysicsBodyHandle
	Raycast(ray rl.Ray, maxDistance float32) RaycastHit
	SetGravity(gravity rl.Vector3)
	GetGravity() rl.Vector3
}
