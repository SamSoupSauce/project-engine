package spatial

import (
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestInteractiveComponentHoverHighlightAndRaycastResolve(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	scene := engine.NewBaseScene("test")
	cabinet := engine.NewGameObject("cabinet_01", "Filing Cabinet")
	cabinet.EnsureTransform().SetPosition(engine.NewVector3(0, 1, -4))

	rend := engine.NewRenderable("cabinet_mesh", "steel")
	cabinet.AddComponent(rend)

	rb := NewRigidBody("cabinet_body", world, engine.BodyStatic, BoxShapeDesc(0.5, 1.0, 0.5))
	rb.ObjectLayer = LayerInteractable
	rb.HasCustomLayer = true
	rb.UserData = 1001
	cabinet.AddComponent(rb)

	ic := NewInteractiveComponent("Filing Cabinet", false)
	ic.SetDefaultVerb(VerbOpenInventory, "Open Cabinet", "on_open_cabinet")
	ic.AddContextVerb(VerbStashInventory, "Loot All", "on_loot_cabinet")
	cabinet.AddComponent(ic)
	scene.AddObject(cabinet)

	world.OptimizeBroadPhase()

	hit := world.RaycastMask(
		engine.Ray{
			Position:  engine.NewVector3(0, 1, 0),
			Direction: engine.NewVector3(0, 0, -1),
		},
		10.0,
		1<<LayerInteractable,
	)
	if !hit.Hit || hit.BodyID != "cabinet_body" {
		t.Fatalf("expected raycast hit on cabinet_body, got %+v", hit)
	}

	resolvedObj, resolvedIC := ResolveInteractiveFromHit(world, scene, hit)
	if resolvedObj != cabinet || resolvedIC != ic {
		t.Fatalf("ResolveInteractiveFromHit mismatch: obj=%v ic=%v", resolvedObj, resolvedIC)
	}

	resolvedIC.SetHovered(true)
	if !rend.IsOutlined() {
		t.Fatalf("expected Renderable FlagOutline set on hover")
	}
	resolvedIC.SetHovered(false)
	if rend.IsOutlined() {
		t.Fatalf("expected Renderable FlagOutline cleared when hover ends")
	}
}

func TestPhysicsGrabCarryAndThrow(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	crate := engine.NewGameObject("crate_01", "Wooden Crate")
	crate.EnsureTransform().SetPosition(engine.NewVector3(0, 2, -2))
	rb := NewRigidBody("crate_body", world, engine.BodyDynamic, BoxShapeDesc(0.4, 0.4, 0.4))
	rb.Mass = 2.0
	crate.AddComponent(rb)
	world.RegisterSyncTarget(rb.BodyID, crate)
	world.OptimizeBroadPhase()

	targetCarry := engine.NewVector3(2.0, 3.0, -1.0)
	if err := world.GrabBody(rb, targetCarry); err != nil {
		t.Fatalf("GrabBody failed: %v", err)
	}
	if world.HeldBody() != rb {
		t.Fatalf("expected HeldBody=%v, got %v", rb, world.HeldBody())
	}

	for i := 0; i < 45; i++ {
		world.UpdateGrabTarget(targetCarry)
		world.StepSimulation(DefaultFixedTimestep)
	}

	pos := rb.GetPosition()
	dx := pos.X - targetCarry.X
	dy := pos.Y - targetCarry.Y
	dz := pos.Z - targetCarry.Z
	distSq := dx*dx + dy*dy + dz*dz
	if distSq > 0.35 {
		t.Fatalf("expected held prop pulled near %+v, got %+v (distSq=%.3f)", targetCarry, pos, distSq)
	}

	world.ThrowGrab(engine.NewVector3(0, 0, -1), 16.0)
	if world.HeldBody() != nil {
		t.Fatalf("expected HeldBody=nil after ThrowGrab")
	}
	world.StepSimulation(DefaultFixedTimestep)
	vel := rb.GetVelocity()
	if vel.Z >= -2.0 {
		t.Fatalf("expected negative Z velocity after throwing forward, got %+v", vel)
	}
}
