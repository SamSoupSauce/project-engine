package luabridge

import (
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
	"github.com/SamSoupSauce/project-engine/pkg/interaction"
)

func TestLuaHazardsFractureSweepKickAndHeavyCarryDamping(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()

	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	// Heavy prop two-arm carry + camera rotational damping test via Interaction Controller.
	desk := engine.NewGameObject("oak_desk", "Oak Desk")
	desk.EnsureTransform().SetPosition(engine.NewVector3(0, 1, -2))
	deskRB := spatial.NewRigidBody("desk_rb", world, engine.BodyDynamic, spatial.BoxShapeDesc(0.7, 0.5, 0.4))
	desk.AddComponent(deskRB)
	desk.AddComponent(spatial.NewInteractiveComponent("Oak Desk", true))
	deskFC := spatial.NewFractureComponent(spatial.FractureSplinter, spatial.IntegrityHeavy, 140.0)
	desk.AddComponent(deskFC)
	world.OptimizeBroadPhase()

	ctrl := interaction.NewController(world, nil)
	if err := ctrl.GrabEntity(desk, interaction.CameraPose{
		Origin:  engine.NewVector3(0, 1, 0),
		Forward: engine.NewVector3(0, 0, -1),
	}); err != nil {
		t.Fatalf("GrabEntity heavy prop failed: %v", err)
	}
	if ctrl.State.CarryHands != 2 {
		t.Fatalf("expected 2-arm carry mode for heavy desk, got %d", ctrl.State.CarryHands)
	}
	dx, dy := ctrl.ApplyCameraLookDelta(10.0, 4.0)
	if dx > 3.0 || dy > 1.5 {
		t.Fatalf("expected heavy camera rotational damping (<=0.25), got dx=%.2f dy=%.2f", dx, dy)
	}

	// Lua FractureComponent & HazardSystem + on_sweep / kick_hazards APIs.
	pushGameObject(b.L, desk)
	b.L.SetGlobal("lua_prop", b.L.Get(-1))
	b.L.Pop(1)

	script := `
local fc = lua_prop:GetFracture()
hands = fc:GetRequiredCarryHands()
damping = fc:GetCameraLookDamping()

local hs = new_hazard_system()
hs:SpawnShard(0, 0, -2, 0.5, 20)
hs:RegisterZombie("z1", 0, 0, -10)
hs:RegisterZombie("z_far", 0, 0, -30)

bare_res = hs:EvaluateTraversal("p1", 0, 0, -2, Footwear.Barefoot, 100)
boot_res = hs:EvaluateTraversal("p2", 0, 0, -2, Footwear.Boots, 100)
z1_alert = hs:IsZombieAlerted("z1")
z_far_alert = hs:IsZombieAlerted("z_far")

kicked_cnt = kick_hazards(hs, 0, 0, -1, 0, 0, -1, 3.0, 8.0)
swept_cnt = on_sweep(hs, 0, 0, -1, 0, 0, -1, 5.0, 6.0)
remaining = hs:ActiveShardCount()
`
	if err := b.L.DoString(script); err != nil {
		t.Fatalf("Lua hazard script failed: %v", err)
	}
	if b.L.GetGlobal("hands").String() != "2" {
		t.Fatalf("expected Lua hands=2, got %s", b.L.GetGlobal("hands").String())
	}
	if b.L.GetGlobal("z1_alert").String() != "true" || b.L.GetGlobal("z_far_alert").String() != "false" {
		t.Fatalf("unexpected zombie alert states: z1=%s z_far=%s",
			b.L.GetGlobal("z1_alert").String(), b.L.GetGlobal("z_far_alert").String())
	}
	if b.L.GetGlobal("swept_cnt").String() != "1" || b.L.GetGlobal("remaining").String() != "0" {
		t.Fatalf("expected on_sweep to clear shard: swept=%s remaining=%s",
			b.L.GetGlobal("swept_cnt").String(), b.L.GetGlobal("remaining").String())
	}
}
