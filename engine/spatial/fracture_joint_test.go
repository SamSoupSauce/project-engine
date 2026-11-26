package spatial

import (
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestBreakableSliderJointSpillsItemsAndAttachesArmSocket(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	scene := engine.NewBaseScene("drawer_test")

	cabinet := engine.NewGameObject("nightstand", "Nightstand")
	cabinet.Transform.Position = engine.NewVector3(0, 1, -2)
	cabinetRB := NewRigidBody("nightstand_body", world, engine.BodyStatic, BoxShapeDesc(0.5, 0.6, 0.4))
	cabinet.AddComponent(cabinetRB)
	scene.AddObject(cabinet)

	drawer := engine.NewGameObject("drawer", "Nightstand Drawer")
	drawer.Transform.Position = engine.NewVector3(0, 1, -1.6)
	drawerRB := NewRigidBody("drawer_body", world, engine.BodyDynamic, BoxShapeDesc(0.35, 0.12, 0.3))
	drawerRB.Mass = 1.5
	drawer.AddComponent(drawerRB)
	cabinet.AddChild(drawer)
	world.RegisterSyncTarget(drawerRB.BodyID, drawer)

	// Loose items resting inside the drawer.
	watch := engine.NewGameObject("watch", "Pocket Watch")
	watch.Transform.Position = engine.NewVector3(0, 1.05, -1.6)
	watchRB := NewRigidBody("watch_body", world, engine.BodyDynamic, SphereShapeDesc(0.08))
	watchRB.Mass = 0.2
	watch.AddComponent(watchRB)
	drawer.AddChild(watch)
	world.RegisterSyncTarget(watchRB.BodyID, watch)

	world.OptimizeBroadPhase()

	joint, err := world.CreateBreakableJoint(cabinetRB, drawerRB, JointDesc{
		ID:         "drawer_slide",
		Kind:       JointSlider,
		Anchor:     engine.NewVector3(0, 1, -1.6),
		Axis:       engine.NewVector3(0, 0, 1), // Slide out toward +Z (player)
		MinLimit:   0.0,
		MaxLimit:   0.25,
		BreakForce: 150.0,
	})
	if err != nil {
		t.Fatalf("CreateBreakableJoint failed: %v", err)
	}
	joint.AddContainedProp(watchRB)
	joint.SetArmSocket(true, engine.NewVector3(0, 1.2, -0.5))

	var breakEvent JointBreakEvent
	joint.OnBreak = func(ev JointBreakEvent) {
		breakEvent = ev
	}

	// Yank hard along +Z until drawer hits stop and snaps prismatic joint cleanly.
	pullDir := engine.NewVector3(0, 0, 1)
	for i := 0; i < 30 && !joint.Broken; i++ {
		joint.ApplyPullForce(pullDir, 450.0, DefaultFixedTimestep)
		world.StepSimulation(DefaultFixedTimestep)
	}

	if !joint.Broken {
		t.Fatalf("expected drawer slider joint to break under pull tension (lastForce=%.2f)", joint.LastForce)
	}
	if !breakEvent.AttachedToArmSocket || world.HeldBody() != drawerRB {
		t.Fatalf("expected detached drawer transferred to player arm socket grab, attached=%v held=%v",
			breakEvent.AttachedToArmSocket, world.HeldBody())
	}
	if len(breakEvent.SpilledBodies) != 1 || breakEvent.SpilledBodies[0] != watchRB {
		t.Fatalf("expected loose internal watch to spill, got %+v", breakEvent.SpilledBodies)
	}
	watchVel := watchRB.GetVelocity()
	if watchVel.Z <= 0.5 {
		t.Fatalf("expected spilled item flying forward along +Z pull vector, got vel=%+v", watchVel)
	}
}

func TestBreakableHingeJointTorqueSnap(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	frameRB := NewRigidBody("door_frame", world, engine.BodyStatic, BoxShapeDesc(0.2, 1.0, 0.2))
	frameObj := engine.NewGameObject("door_frame", "Frame")
	frameObj.AddComponent(frameRB)

	doorObj := engine.NewGameObject("cabinet_door", "Cabinet Door")
	doorObj.Transform.Position = engine.NewVector3(0.5, 0, 0)
	doorRB := NewRigidBody("cabinet_door_rb", world, engine.BodyDynamic, BoxShapeDesc(0.4, 0.9, 0.08))
	doorRB.Mass = 3.0
	doorObj.AddComponent(doorRB)
	world.OptimizeBroadPhase()

	hinge, err := world.CreateBreakableJoint(frameRB, doorRB, JointDesc{
		ID:          "door_hinge",
		Kind:        JointHinge,
		Anchor:      engine.NewVector3(0.1, 0, 0),
		Axis:        engine.NewVector3(0, 1, 0),
		MinLimit:    -0.5,
		MaxLimit:    0.5,
		BreakTorque: 80.0,
	})
	if err != nil {
		t.Fatalf("CreateBreakableJoint(hinge) failed: %v", err)
	}

	for i := 0; i < 30 && !hinge.Broken; i++ {
		hinge.ApplyPullForce(engine.NewVector3(0, 0, 1), 600.0, DefaultFixedTimestep)
		world.StepSimulation(DefaultFixedTimestep)
	}
	if !hinge.Broken {
		t.Fatalf("expected hinge joint to break under excessive torque (lastForce=%.2f lastTorque=%.2f)",
			hinge.LastForce, hinge.LastTorque)
	}
}

func TestFractureLowVsHeavyIntegrityStrikesAndMatchwood(t *testing.T) {
	world, err := NewPhysicsWorld(DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	scene := engine.NewBaseScene("fracture_test")

	// Enemy body target.
	enemy := engine.NewGameObject("zombie", "Zombie")
	enemy.Transform.Position = engine.NewVector3(0, 1, -3)
	enemyRB := NewRigidBody("zombie_rb", world, engine.BodyDynamic, CapsuleShapeDesc(0.6, 0.3))
	enemyRB.Mass = 70.0
	enemyRB.GravityFactor = 0
	enemy.AddComponent(enemyRB)
	scene.AddObject(enemy)

	// 1. Flimsy wooden drawer striking zombie obliterates into matchwood splinters (~0.075 kg).
	drawer := engine.NewGameObject("flimsy_drawer", "Wooden Drawer")
	drawer.Transform.Position = engine.NewVector3(0, 1, -2)
	drawerRB := NewRigidBody("flimsy_drawer_rb", world, engine.BodyDynamic, BoxShapeDesc(0.3, 0.1, 0.25))
	drawerRB.Mass = 1.0
	drawer.AddComponent(drawerRB)
	drawerFC := NewFractureComponent(FractureSplinter, IntegrityLow, 8.0)
	drawerFC.Scene = scene
	drawer.AddComponent(drawerFC)
	scene.AddObject(drawer)

	world.OptimizeBroadPhase()

	lightStrike := drawerFC.StrikeEntity(enemyRB, nil, engine.NewVector3(0, 0, -1), 10.0)
	if !lightStrike.PropFractured || !drawerFC.Broken {
		t.Fatalf("expected lightweight drawer to shatter on strike, got %+v", lightStrike)
	}
	if lightStrike.Knockdown || lightStrike.StaggerFrames > 2 || lightStrike.AppliedImpulse > 2.0 {
		t.Fatalf("expected minimal stagger/impulse from flimsy drawer, got %+v", lightStrike)
	}
	if len(lightStrike.FractureEvent.Debris) != drawerFC.DebrisPrefab.Count {
		t.Fatalf("expected %d matchwood fragments, got %d", drawerFC.DebrisPrefab.Count, len(lightStrike.FractureEvent.Debris))
	}
	for _, debRB := range lightStrike.FractureEvent.DebrisBodies {
		if debRB.Mass < 0.05 || debRB.Mass > 0.1 {
			t.Fatalf("expected splinter mass in 0.05..0.1 kg, got %.3f", debRB.Mass)
		}
	}

	// 2. Heavy nightstand enforces 2-arm carry, heavy look damping, and knocks down enemy without breaking.
	nightstand := engine.NewGameObject("heavy_nightstand", "Oak Nightstand")
	nightstand.Transform.Position = engine.NewVector3(2, 1, -2)
	nsRB := NewRigidBody("heavy_ns_rb", world, engine.BodyDynamic, BoxShapeDesc(0.5, 0.6, 0.4))
	nsRB.Mass = 32.0
	nightstand.AddComponent(nsRB)
	nsFC := NewFractureComponent(FractureSplinter, IntegrityHeavy, 150.0)
	nightstand.AddComponent(nsFC)
	scene.AddObject(nightstand)
	world.OptimizeBroadPhase()

	if nsFC.RequiredCarryHands != 2 || nsFC.CameraLookDamping >= 0.5 {
		t.Fatalf("expected 2-arm carry and heavy camera damping, got hands=%d damping=%.2f",
			nsFC.RequiredCarryHands, nsFC.CameraLookDamping)
	}
	heavyStrike := nsFC.StrikeEntity(enemyRB, nil, engine.NewVector3(0, 0, -1), 10.0)
	if heavyStrike.PropFractured || nsFC.Broken {
		t.Fatalf("expected heavy nightstand NOT to self-destruct on strike")
	}
	if !heavyStrike.Knockdown || heavyStrike.StaggerFrames < 10 || heavyStrike.AppliedImpulse < 15.0 {
		t.Fatalf("expected heavy knockdown impulse & stagger, got %+v", heavyStrike)
	}
}
