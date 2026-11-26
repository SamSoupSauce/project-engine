package props

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/components"
	"github.com/SamSoupSauce/project-engine/engine/hazards"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

// withRepoRoot changes CWD temporarily to project root so manifest paths ("assets/models/props/*.gltf") resolve naturally.
func withRepoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(%q): %v", root, err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func TestLuaGLTFPropArchitectureEndToEnd(t *testing.T) {
	withRepoRoot(t)

	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld: %v", err)
	}
	defer world.Destroy()

	scene := engine.NewBaseScene("prop_arch_test")
	hazardSys := hazards.NewHazardSystem(world, scene)
	zombie := &hazards.ZombieListener{ID: "zombie_kitchen", Position: engine.NewVector3(0, 0, -14)}
	hazardSys.RegisterZombie(zombie)

	L := lua.NewState()
	defer L.Close()
	reg := NewPropRegistry(L, world, scene, hazardSys)

	// 1. LoadDirectory discovers and validates all Lua manifests and .gltf assets.
	if err := reg.LoadDirectory("scripts/props"); err != nil {
		t.Fatalf("LoadDirectory failed: %v", err)
	}
	for _, id := range []string{"kitchen_toaster", "nightstand_drawer", "lamp_ceramic"} {
		if _, ok := reg.Manifests[id]; !ok {
			t.Fatalf("expected manifest %q loaded from scripts/props", id)
		}
	}

	// 2. Kitchen Toaster: convex_hull from GLTF + high impulse collision triggers acoustic alert & alerts zombie.
	toasterID, err := reg.SpawnProp("kitchen_toaster", engine.NewVector3(0, 1, -3))
	if err != nil {
		t.Fatalf("SpawnProp(kitchen_toaster): %v", err)
	}
	toasterObj := reg.Entities[toasterID]
	toasterMesh := toasterObj.GetComponent(reflect.TypeOf((*components.MeshRendererComponent)(nil))).(*components.MeshRendererComponent)
	if toasterMesh.Asset == nil || toasterMesh.Asset.FilePath != "assets/models/props/kitchen_toaster.gltf" {
		t.Fatalf("expected toaster MeshRendererComponent loaded from GLTF, got %+v", toasterMesh.Asset)
	}
	toasterRB := toasterObj.GetComponent(reflect.TypeOf((*components.RigidBodyComponent)(nil))).(*components.RigidBodyComponent)
	if toasterRB.Body == nil || toasterRB.Body.Shape.Kind != spatial.ShapeConvexHull {
		t.Fatalf("expected Jolt ConvexHull shape on toaster RigidBodyComponent, got %+v", toasterRB.Body)
	}
	if err := reg.DispatchCollision(toasterID, engine.NewVector3(0, 0, -3), engine.NewVector3(0, 1, 0), 10.0); err != nil {
		t.Fatalf("DispatchCollision(toaster): %v", err)
	}
	if len(reg.AcousticAlerts) != 1 || reg.AcousticAlerts[0].SoundProfile != "metal_clatter_loud" || !zombie.Alerted {
		t.Fatalf("expected loud metal clatter acoustic alert to wake zombie: alerts=%+v zombie=%+v", reg.AcousticAlerts, zombie)
	}

	// 3. Nightstand Drawer: prismatic ConstraintComponent break triggers on_joint_break velocity & splinters on collision.
	drawerID, err := reg.SpawnProp("nightstand_drawer", engine.NewVector3(2, 1, -2))
	if err != nil {
		t.Fatalf("SpawnProp(nightstand_drawer): %v", err)
	}
	drawerObj := reg.Entities[drawerID]
	drawerCC := drawerObj.GetComponent(reflect.TypeOf((*components.ConstraintComponent)(nil))).(*components.ConstraintComponent)
	if drawerCC.BreakForce != 120.0 || drawerCC.MaxLimit < 0.4 {
		t.Fatalf("expected drawer ConstraintComponent configured from Lua, got %+v", drawerCC)
	}

	reg.HandVelocity = engine.NewVector3(0, 0, 5.5)
	if err := reg.DispatchJointBreak(drawerID, "nightstand_frame"); err != nil {
		t.Fatalf("DispatchJointBreak(drawer): %v", err)
	}
	drawerRB := drawerObj.GetComponent(reflect.TypeOf((*components.RigidBodyComponent)(nil))).(*components.RigidBodyComponent)
	if vel := drawerRB.GetLinearVelocity(); vel.Z < 4.5 {
		t.Fatalf("expected Lua on_joint_break to apply Player hand velocity, got %+v", vel)
	}

	if err := reg.DispatchCollision(drawerID, engine.NewVector3(2, 0.2, -2), engine.NewVector3(0, 1, 0), 18.0); err != nil {
		t.Fatalf("DispatchCollision(drawer): %v", err)
	}
	if len(reg.SplinterSpawns) != 1 || reg.SplinterSpawns[0].Count != 8 || reg.Entities[drawerID] != nil {
		t.Fatalf("expected drawer splinter debris spawn and entity destroy, splinters=%+v", reg.SplinterSpawns)
	}

	// 4. Ceramic Lamp: collision >= shatter_impulse dynamically creates FloorShardHazard with HazardComponent in Lua.
	lampID, err := reg.SpawnProp("lamp_ceramic", engine.NewVector3(0, 1.5, -5))
	if err != nil {
		t.Fatalf("SpawnProp(lamp_ceramic): %v", err)
	}
	if err := reg.DispatchCollision(lampID, engine.NewVector3(0, 0.05, -5), engine.NewVector3(0, 1, 0), 15.0); err != nil {
		t.Fatalf("DispatchCollision(lamp): %v", err)
	}
	if reg.Entities[lampID] != nil || hazardSys.ActiveShardCount() < 1 {
		t.Fatalf("expected shattered lamp removed & Lua FloorShardHazard active (shards=%d)", hazardSys.ActiveShardCount())
	}

	// Barefoot actor crossing the Lua-spawned FloorShardHazard takes laceration & bleeds.
	actor := &hazards.TraversalActor{
		EntityID:       "player",
		Position:       engine.NewVector3(0, 0.05, -5),
		Footwear:       hazards.FootwearBarefoot,
		Health:         100,
		LimpSpeedScale: 1.0,
	}
	trav := hazardSys.EvaluateTraversal(actor)
	if !trav.Damaged || !trav.BleedingApplied || !trav.Screamed {
		t.Fatalf("expected Lua FloorShardHazard traversal laceration, got %+v", trav)
	}
}
