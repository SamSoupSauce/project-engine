package engine

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
	GetPosition() Vector3
	SetPosition(pos Vector3)
	GetRotation() Vector3
	SetRotation(rot Vector3)
	GetVelocity() Vector3
	SetVelocity(vel Vector3)
	IsGrounded() bool
	SetGrounded(grounded bool)
	GetBoundingBox() BoundingBox
}

// RaycastHit describes the outcome of an abstract raycast query.
type RaycastHit struct {
	Hit      bool
	Distance float32
	Point    Vector3
	Normal   Vector3
	BodyID   string
	UserData uint32
}

// PhysicsWorldHandle defines an abstract interface for a 3D physics environment.
type PhysicsWorldHandle interface {
	Step(dt float32)
	AddBody(body PhysicsBodyHandle)
	RemoveBody(id string)
	GetBody(id string) PhysicsBodyHandle
	Raycast(ray Ray, maxDistance float32) RaycastHit
	SetGravity(gravity Vector3)
	GetGravity() Vector3
}
