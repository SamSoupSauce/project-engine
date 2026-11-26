package luabridge

import (
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

func TestHelpIntrospectionOnSelectedObjectAndComponents(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()
	console := NewConsole(bridge)

	cube := engine.NewGameObject("cube_01", "RainbowCube")
	cube.AddComponent(engine.NewRenderable("cube_mesh", "rainbow_mat"))
	cube.AddComponent(spatial.NewInteractiveComponent("RainbowCube", true))
	bridge.RegisterGameObject("cube", cube)
	console.PickObject(cube)

	// 1. help(selected) enumerates GameObject methods AND lists attached components & their Go types.
	objHelp := console.ExecuteCommand("help(selected)")
	for _, want := range []string{
		"GameObject Help: RainbowCube (cube_01)",
		"GetComponent",
		"AddComponent",
		"SetPosition",
		"Components (3):",
		"Transform (*engine.TransformComponent)",
		"Renderable (*engine.Renderable)",
		"InteractiveComponent (*spatial.InteractiveComponent)",
	} {
		if !strings.Contains(objHelp, want) {
			t.Fatalf("expected help(selected) output to contain %q, got:\n%s", want, objHelp)
		}
	}

	// 2. CLI command form `help selected` also works.
	cliSelectedHelp := console.ExecuteCommand("help selected")
	if !strings.Contains(cliSelectedHelp, "Transform (*engine.TransformComponent)") {
		t.Fatalf("expected `help selected` command output to list components, got:\n%s", cliSelectedHelp)
	}

	// 3. help(comp) on a component userdata prints methods/fields/constraints.
	compHelp := console.ExecuteCommand(`help(selected:GetComponent("Renderable"))`)
	for _, want := range []string{
		"Component Help: Renderable (*engine.Renderable)",
		"Constraint:",
		"Transform component",
		"ValidateConstraint()",
		"SetOutlined(bool)",
	} {
		if !strings.Contains(compHelp, want) {
			t.Fatalf("expected help(Renderable comp) to contain %q, got:\n%s", want, compHelp)
		}
	}

	// 4. help("ComponentName") works by type name string (including aliases).
	nameHelp := console.ExecuteCommand(`help("Transform")`)
	if !strings.Contains(nameHelp, "Component Help: Transform (*engine.TransformComponent)") ||
		!strings.Contains(nameHelp, "Translate(dx, dy, dz)") {
		t.Fatalf("expected help('Transform') component menu, got:\n%s", nameHelp)
	}
	cliCompHelp := console.ExecuteCommand("help RigidBody")
	if !strings.Contains(cliCompHelp, "Component Help: RigidBody (*spatial.RigidBody)") ||
		!strings.Contains(cliCompHelp, "AddImpulse(") {
		t.Fatalf("expected `help RigidBody` menu, got:\n%s", cliCompHelp)
	}
}

func TestLuaGenericComponentAccessAndRenderableConstraint(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()
	console := NewConsole(bridge)

	obj := engine.NewGameObject("prop_01", "Prop")
	bridge.RegisterGameObject("prop", obj)
	console.PickObject(obj)

	// 1. Access default Transform component from Lua and mutate 3D location.
	hasTr := console.ExecuteCommand(`return selected:HasComponent("Transform")`)
	if hasTr != "true" {
		t.Fatalf("expected HasComponent('Transform') == true, got %q", hasTr)
	}
	console.ExecuteCommand(`
		local tc = selected:GetComponent("Transform")
		tc:SetPosition(11, 22, 33)
		tc.x = 15
	`)
	if obj.Transform.Position.X != 15 || obj.Transform.Position.Y != 22 || obj.Transform.Position.Z != 33 {
		t.Fatalf("expected Transform Lua mutation synced to GameObject, got %+v", obj.Transform.Position)
	}

	// 2. Add Renderable from Lua (passes Transform constraint) and enumerate via GetComponents().
	console.ExecuteCommand(`
		local r = selected:AddComponent("Renderable", "crate_mesh", "wood")
		assert(r:ValidateConstraint())
		assert(r:GetTransform() ~= nil)
	`)
	count := console.ExecuteCommand(`return #selected:GetComponents()`)
	if count != "2" {
		t.Fatalf("expected 2 components in GetComponents(), got %q", count)
	}

	// 3. Removing Transform violates Renderable constraint and blocks adding another Renderable.
	console.ExecuteCommand(`selected:RemoveComponent("Renderable"); selected:RemoveComponent("Transform")`)
	errOut := console.ExecuteCommand(`selected:AddComponent("Renderable", "mesh", "mat")`)
	if !strings.Contains(errOut, "Error:") || !strings.Contains(errOut, "Transform") {
		t.Fatalf("expected AddComponent('Renderable') error when Transform missing, got %q", errOut)
	}
}
