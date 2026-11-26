package luabridge

import (
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
	"github.com/SamSoupSauce/project-engine/pkg/interaction"
)

func TestLuaInteractiveBindingsAndVerbDispatch(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()

	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	scene := engine.NewBaseScene("lua_interact")
	crate := engine.NewGameObject("door_01", "Blast Door")
	crate.EnsureTransform().SetPosition(engine.NewVector3(0, 1, -3))
	rend := engine.NewRenderable("door_mesh", "steel")
	crate.AddComponent(rend)
	rb := spatial.NewRigidBody("door_body", world, engine.BodyStatic, spatial.BoxShapeDesc(0.5, 1.0, 0.2))
	rb.ObjectLayer = spatial.LayerInteractable
	rb.HasCustomLayer = true
	crate.AddComponent(rb)
	scene.AddObject(crate)
	world.OptimizeBroadPhase()

	pushGameObject(b.L, crate)
	b.L.SetGlobal("door", b.L.Get(-1))
	b.L.Pop(1)

	script := `
primary_calls = 0
secondary_calls = 0
last_verb_label = ""

function on_toggle_door(self, label, verb)
	primary_calls = primary_calls + 1
	last_verb_label = label
end

function on_hack_door(self, label, verb)
	secondary_calls = secondary_calls + 1
	last_verb_label = label
end

local ic = door:AddInteractive("Blast Door", false)
ic:SetHighlightTint(0.1, 0.8, 1.0, 1.0)
ic:SetDefaultVerb(InteractionVerb.ToggleState, "Toggle Door", "on_toggle_door")
ic:AddContextVerb(InteractionVerb.Custom, "Hack Lock", "on_hack_door")

local r = door:GetRenderable()
r:SetOutlineColor(0.1, 0.8, 1.0, 1.0)
`
	if err := b.L.DoString(script); err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}

	ctrl := interaction.NewController(world, scene)
	b.BindInteractionController(ctrl)

	cam := interaction.CameraPose{
		Origin:  engine.NewVector3(0, 1, 0),
		Forward: engine.NewVector3(0, 0, -1),
	}

	// Hover highlights the Renderable via InteractiveComponent.
	ctrl.Update(cam, 0.016)
	if !rend.IsOutlined() {
		t.Fatalf("expected Blast Door Renderable outlined on hover")
	}

	// Quick tap runs on_toggle_door.
	ctrl.OnInteractPressed()
	ctrl.Update(cam, 0.080)
	if res, err := ctrl.OnInteractReleased(cam); err != nil || !res.Executed {
		t.Fatalf("expected primary Lua verb execution, got res=%+v err=%v", res, err)
	}
	if got := b.L.GetGlobal("primary_calls").String(); got != "1" {
		t.Fatalf("expected primary_calls=1, got %s", got)
	}

	// Hold > 250ms opens context menu, release on index 0 triggers on_hack_door.
	ctrl.OnInteractPressed()
	ctrl.Update(cam, 0.300)
	if !ctrl.State.IsMenuOpen {
		t.Fatalf("expected context menu open on hold")
	}
	ctrl.SetHoveredMenuOption(0)
	if res, err := ctrl.OnInteractReleased(cam); err != nil || !res.Executed {
		t.Fatalf("expected secondary Lua verb execution, got res=%+v err=%v", res, err)
	}
	if got := b.L.GetGlobal("secondary_calls").String(); got != "1" {
		t.Fatalf("expected secondary_calls=1, got %s", got)
	}
	if got := b.L.GetGlobal("last_verb_label").String(); got != "Hack Lock" {
		t.Fatalf("expected last_verb_label='Hack Lock', got %q", got)
	}
}
