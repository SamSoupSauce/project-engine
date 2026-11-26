package hazards

import (
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

func TestCeramicShatterFootwearTraversalAlertsAndSweepKick(t *testing.T) {
	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	scene := engine.NewBaseScene("hazard_test")
	hs := NewHazardSystem(world, scene)

	// Static floor beneath dropped ceramic lamp.
	floorRB := spatial.NewRigidBody("floor", world, engine.BodyStatic, spatial.BoxShapeDesc(10, 0.5, 10))
	floorObj := engine.NewGameObject("floor", "ConcreteFloor")
	floorObj.Transform.Position = engine.NewVector3(0, -0.5, -2)
	floorObj.AddComponent(floorRB)
	scene.AddObject(floorObj)

	// Fragile ceramic table lamp.
	lamp := engine.NewGameObject("lamp_01", "Ceramic Table Lamp")
	lamp.Transform.Position = engine.NewVector3(0, 0.2, -2)
	lampRB := spatial.NewRigidBody("lamp_rb", world, engine.BodyDynamic, spatial.BoxShapeDesc(0.18, 0.25, 0.18))
	lampRB.Mass = 1.8
	lamp.AddComponent(lampRB)
	lampFC := spatial.NewFractureComponent(spatial.FractureShatter, spatial.IntegrityLow, 5.0)
	lampFC.Scene = scene
	lamp.AddComponent(lampFC)
	hs.WireFracture(lampFC)
	scene.AddObject(lamp)
	world.OptimizeBroadPhase()

	// Drop/throw lamp into floor -> shatters into persistent HazardShards.
	ev, broke := lampFC.ApplyImpact(14.0, engine.NewVector3(0, 0.05, -2), engine.NewVector3(0, 1, 0))
	if !broke || ev.Mode != spatial.FractureShatter {
		t.Fatalf("expected ceramic lamp shatter fracture, broke=%v mode=%v", broke, ev.Mode)
	}
	if hs.ActiveShardCount() < 1 {
		t.Fatalf("expected active HazardShard colliders spawned on floor")
	}
	for _, s := range hs.Shards {
		if s.Tag != HazardShardTag || s.Owner.Name != HazardShardTag {
			t.Fatalf("expected HazardShard tag on spawned collider, got %+v", s)
		}
	}

	// Register zombies inside and outside 15.0m crunch radius.
	nearZombie := &ZombieListener{ID: "zombie_near", Position: engine.NewVector3(0, 0, -12)} // ~10m from (0,0,-2)
	farZombie := &ZombieListener{ID: "zombie_far", Position: engine.NewVector3(0, 0, -25)}   // ~23m from (0,0,-2)
	hs.RegisterZombie(nearZombie)
	hs.RegisterZombie(farZombie)

	// 1. Barefoot player steps over shards -> laceration, bleed, limp penalty, vocal scream.
	barefootActor := &TraversalActor{
		EntityID:       "player_barefoot",
		Position:       engine.NewVector3(0, 0.1, -2),
		Footwear:       FootwearBarefoot,
		Health:         100.0,
		LimpSpeedScale: 1.0,
	}
	bareRes := hs.EvaluateTraversal(barefootActor)
	if !bareRes.Damaged || !bareRes.BleedingApplied || !bareRes.Limping || !bareRes.Screamed {
		t.Fatalf("expected laceration, bleeding, limp, and vocal scream for barefoot player: %+v", bareRes)
	}
	if barefootActor.Health >= 100.0 || barefootActor.LimpSpeedScale >= 1.0 {
		t.Fatalf("expected reduced health and limp speed scale, got hp=%.1f limp=%.2f",
			barefootActor.Health, barefootActor.LimpSpeedScale)
	}
	if nearZombie.Alerted {
		t.Fatalf("barefoot step should emit vocal scream, not booted crunch alert")
	}

	// 2. Booted player steps over shards -> zero damage, crunch sound radius 15.0m alerts nearZombie only.
	bootedActor := &TraversalActor{
		EntityID:       "player_boots",
		Position:       engine.NewVector3(0, 0.1, -2),
		Footwear:       FootwearBoots,
		Health:         100.0,
		LimpSpeedScale: 1.0,
	}
	bootRes := hs.EvaluateTraversal(bootedActor)
	if bootRes.Damaged || bootRes.Limping || bootRes.Screamed || !bootRes.CrunchAlerted {
		t.Fatalf("expected booted traversal to prevent damage and crunch loudly: %+v", bootRes)
	}
	if bootedActor.Health != 100.0 || !nearZombie.Alerted || farZombie.Alerted {
		t.Fatalf("expected 100 HP and nearZombie alerted (far ignored): hp=%.1f near=%v far=%v",
			bootedActor.Health, nearZombie.Alerted, farZombie.Alerted)
	}

	// 3. Physical kick scatters shards/debris; OnSweep clears shards cleanly from chokepoint.
	kicked := hs.Kick(engine.NewVector3(0, 0.1, -1.2), engine.NewVector3(0, 0, -1), 3.0, 9.0)
	if kicked == 0 {
		t.Fatalf("expected Kick to affect shards/debris")
	}
	world.StepSimulation(spatial.DefaultFixedTimestep)

	swept := hs.OnSweep(engine.NewVector3(0, 0.1, -1.2), engine.NewVector3(0, 0, -1), 5.0, 7.0)
	if swept == 0 || hs.ActiveShardCount() != 0 {
		t.Fatalf("expected OnSweep to clear chokepoint shards (swept=%d remaining=%d)", swept, hs.ActiveShardCount())
	}
}
