package spatial

import (
	"math"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestPhysicsWorldFixedTimestep60HzAccumulator(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	// Less than 1/60s (~16.67ms) -> 0 steps
	if steps := world.StepSimulation(0.008); steps != 0 {
		t.Fatalf("expected 0 steps for 8ms, got %d", steps)
	}
	// Another 9ms -> total 17ms -> 1 fixed step
	if steps := world.StepSimulation(0.009); steps != 1 {
		t.Fatalf("expected 1 step at 17ms total, got %d", steps)
	}
	// 50ms -> 3 fixed steps
	if steps := world.StepSimulation(0.050); steps != 3 {
		t.Fatalf("expected 3 steps for 50ms, got %d", steps)
	}
	if world.StepCount() != 4 {
		t.Fatalf("expected total StepCount 4, got %d", world.StepCount())
	}
}

func TestDynamicRigidBodyCollisionSleepAndRenderableSync(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	// Static floor box centered at y=-0.5, top surface at y=0
	floorObj := engine.NewGameObject("floor", "Floor")
	floorObj.Transform.Position = engine.NewVector3(0, -0.5, 0)
	floorRB := NewRigidBody("floor", world, engine.BodyStatic, BoxShapeDesc(20, 0.5, 20))
	floorObj.AddComponent(floorRB)

	// Dynamic prop (thrown box) with Renderable + child Renderable to verify hierarchy sync
	propObj := engine.NewGameObject("prop", "AlarmClock")
	propObj.Transform.Position = engine.NewVector3(0, 3.0, 0)
	propRB := NewRigidBody("prop", world, engine.BodyDynamic, BoxShapeDesc(0.5, 0.5, 0.5))
	propRB.Restitution = 0.05
	propRend := engine.NewRenderable("alarm_clock", "default")
	propObj.AddComponent(propRB)
	propObj.AddComponent(propRend)

	childObj := engine.NewGameObject("prop_bell", "Bell")
	childObj.Transform.Position = engine.NewVector3(0, 0.6, 0)
	childObj.AddComponent(engine.NewRenderable("bell", "metal"))
	propObj.AddChild(childObj)

	world.OptimizeBroadPhase()

	// Step for up to 5 seconds at 60 Hz; prop must hit floor (y~0.5) and go to sleep
	for i := 0; i < 300; i++ {
		world.StepSimulation(DefaultFixedTimestep)
	}

	pos := propObj.Transform.Position
	if pos.Y < 0.45 || pos.Y > 0.65 {
		t.Fatalf("expected prop resting near y=0.5, got y=%f", pos.Y)
	}
	if propRB.IsActive() {
		t.Errorf("expected resting prop body to enter sleep state")
	}

	// Verify RenderPassBuffer extracts synchronized world transform matrices for prop and child
	passBuf := engine.NewRenderPassBuffer(16)
	passBuf.ExtractFromHierarchy(propObj)
	if len(passBuf.Commands) != 2 {
		t.Fatalf("expected 2 Renderable DrawCommands, got %d", len(passBuf.Commands))
	}
	childWorldY := passBuf.Commands[1].WorldTransform.Position.Y
	if math.Abs(float64(childWorldY-(pos.Y+0.6))) > 0.02 {
		t.Errorf("expected child Renderable world Y ~ %f, got %f", pos.Y+0.6, childWorldY)
	}
}

func TestCCDPreventsTunnelingThroughThinWall(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	// Thin 0.2m wall at z = 0
	wallObj := engine.NewGameObject("wall", "ThinWall")
	wallObj.Transform.Position = engine.NewVector3(0, 1, 0)
	wallObj.AddComponent(NewRigidBody("wall", world, engine.BodyStatic, BoxShapeDesc(10, 5, 0.1)))

	// High-velocity projectile with CCD enabled moving at 240 m/s (4m per 1/60s step!)
	projObj := engine.NewGameObject("proj", "Projectile")
	projObj.Transform.Position = engine.NewVector3(0, 1, -2.0)
	projRB := NewRigidBody("proj", world, engine.BodyDynamic, SphereShapeDesc(0.2))
	projRB.UseCCD = true
	projRB.GravityFactor = 0
	projObj.AddComponent(projRB)

	world.OptimizeBroadPhase()
	projRB.SetVelocity(engine.NewVector3(0, 0, 240))

	for i := 0; i < 5; i++ {
		world.StepSimulation(DefaultFixedTimestep)
	}

	finalZ := projObj.Transform.Position.Z
	if finalZ > 0.1 {
		t.Fatalf("CCD projectile tunneled past thin wall at z=0: final z=%f", finalZ)
	}
}

type mockMapColliderSource struct {
	bounds [][2][3]float32
}

func (m mockMapColliderSource) ExportStaticColliderBounds() [][2][3]float32 {
	return m.bounds
}

func TestWorldMapStaticCompoundColliderAndCharacterStairsCorners(t *testing.T) {
	const floorH float32 = 0.08
	const wallH float32 = 2.2
	const wallT float32 = 0.2

	var bounds [][2][3]float32
	// 6x1 floor slabs at x = 0..5 (1x1m each, aligned seams)
	for col := 0; col < 6; col++ {
		x0 := float32(col) - 0.5
		x1 := float32(col) + 0.5
		bounds = append(bounds, [2][3]float32{
			{x0, 0, -1.5},
			{x1, floorH, 1.5},
		})
	}
	// Corner walls at col=0 (North -Z and West -X)
	bounds = append(bounds,
		[2][3]float32{{-0.5, floorH, -1.5}, {0.5, floorH + wallH, -1.5 + wallT}},
		[2][3]float32{{-0.5, floorH, -1.5}, {-0.5 + wallT, floorH + wallH, 1.5}},
	)
	// 4-step staircase ascending East (+X) across col=3 (x in [2.5, 3.5]) plus upper landing at col=4..5
	for s := 0; s < 4; s++ {
		x0 := 2.5 + float32(s)*0.25
		x1 := x0 + 0.25
		topY := floorH + float32(s+1)*0.2
		bounds = append(bounds, [2][3]float32{
			{x0, floorH, -1.5},
			{x1, topY, 1.5},
		})
	}
	// Upper landing at y = floorH + 0.8
	bounds = append(bounds, [2][3]float32{
		{3.5, floorH, -1.5},
		{5.5, floorH + 0.8, 1.5},
	})

	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	mapBodyID, err := BuildWorldMapCollider(world, mockMapColliderSource{bounds: bounds})
	if err != nil {
		t.Fatalf("BuildWorldMapCollider failed: %v", err)
	}
	if mapBodyID == InvalidBodyID {
		t.Fatalf("expected valid static compound body ID")
	}

	// Verify raycast hits the North corner wall
	wallHit := world.Raycast(engine.Ray{
		Position:  engine.NewVector3(0.0, 1.0, 0.0),
		Direction: engine.NewVector3(0, 0, -1),
	}, 5.0)
	if !wallHit.Hit || wallHit.Distance > 1.5 {
		t.Fatalf("expected raycast to hit corner North wall, got %+v", wallHit)
	}

	// Spawn CharacterController at x=0.5 (across floor seams) and walk East (+X) up the stairs!
	playerObj := engine.NewGameObject("player", "Player")
	playerObj.Transform.Position = engine.NewVector3(0.5, floorH+0.02, 0)
	ctrl := NewCharacterController(world, DefaultCharacterConfig())
	playerObj.AddComponent(ctrl)

	for i := 0; i < 15; i++ {
		world.StepSimulation(DefaultFixedTimestep)
	}
	if !ctrl.IsGrounded() {
		t.Fatalf("expected character to be grounded on tile floor, state=%v pos=%+v", ctrl.GroundState(), ctrl.GetPosition())
	}

	// Walk East (+X) across floor seams and up the 4-step staircase
	ctrl.SetDesiredVelocity(engine.NewVector3(3.0, 0, 0))
	for i := 0; i < 70; i++ {
		world.StepSimulation(DefaultFixedTimestep)
	}

	endPos := playerObj.Transform.Position
	if endPos.X <= 3.0 {
		t.Fatalf("expected character to advance smoothly across seams/stairs, endX=%f", endPos.X)
	}
	if endPos.Y <= floorH+0.4 {
		t.Fatalf("expected character to climb stairs above floor height %f, got Y=%f", floorH, endPos.Y)
	}
}

func TestStaticMeshColliderRaycast(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	verts := []engine.Vector3{
		engine.NewVector3(-5, 0, -5),
		engine.NewVector3(5, 0, -5),
		engine.NewVector3(5, 0, 5),
		engine.NewVector3(-5, 0, 5),
	}
	indices := []uint32{0, 2, 1, 0, 3, 2}
	bodyID, err := BuildStaticMeshCollider(world, verts, indices)
	if err != nil || bodyID == InvalidBodyID {
		t.Fatalf("BuildStaticMeshCollider failed: id=%d err=%v", bodyID, err)
	}

	hit := world.Raycast(engine.Ray{
		Position:  engine.NewVector3(0, 4, 0),
		Direction: engine.NewVector3(0, -1, 0),
	}, 10.0)
	if !hit.Hit || math.Abs(float64(hit.Distance-4.0)) > 0.05 {
		t.Fatalf("expected mesh raycast hit at distance 4.0, got %+v", hit)
	}
}

func TestRaycastMaskAndEntityUserData(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	// Static wall at z=8 on LayerNonMoving with user_data=1001
	wallObj := engine.NewGameObject("wall", "StaticWall")
	wallObj.Transform.Position = engine.NewVector3(0, 1, 8)
	wallRB := NewRigidBody("wall", world, engine.BodyStatic, BoxShapeDesc(2, 2, 0.5))
	wallRB.UserData = 1001
	wallObj.AddComponent(wallRB)

	// Interactable prop at z=4 on LayerInteractable with user_data=2042
	propObj := engine.NewGameObject("door_knob", "InteractableProp")
	propObj.Transform.Position = engine.NewVector3(0, 1, 4)
	propRB := NewRigidBody("door_knob", world, engine.BodyDynamic, BoxShapeDesc(0.5, 0.5, 0.5))
	propRB.GravityFactor = 0
	propRB.UserData = 2042
	propRB.SetObjectLayer(LayerInteractable)
	propObj.AddComponent(propRB)

	world.OptimizeBroadPhase()

	ray := engine.Ray{
		Position:  engine.NewVector3(0, 1, 0),
		Direction: engine.NewVector3(0, 0, 1),
	}

	// All layers -> hits closer interactable prop at z=3.5
	allHit := world.Raycast(ray, 20.0)
	if !allHit.Hit || allHit.UserData != 2042 || math.Abs(float64(allHit.Distance-3.5)) > 0.05 {
		t.Fatalf("expected all-layer raycast to hit prop user_data=2042 at d=3.5, got %+v", allHit)
	}

	// LayerMaskNonMoving -> ignores prop and hits wall at z=7.5
	wallHit := world.RaycastMask(ray, 20.0, LayerMaskNonMoving)
	if !wallHit.Hit || wallHit.UserData != 1001 || math.Abs(float64(wallHit.Distance-7.5)) > 0.05 {
		t.Fatalf("expected non-moving layer raycast to hit wall user_data=1001 at d=7.5, got %+v", wallHit)
	}

	// LayerMaskMoving -> misses both static wall and interactable prop
	movingHit := world.RaycastMask(ray, 20.0, LayerMaskMoving)
	if movingHit.Hit {
		t.Fatalf("expected moving-only layer mask to miss, got %+v", movingHit)
	}
}
