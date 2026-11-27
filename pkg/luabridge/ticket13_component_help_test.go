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
	cube.AddComponent(engine.NewTransformComponent())
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
		"Components (3):",
		"Transform (*engine.TransformComponent)",
		"Renderable (*engine.Renderable)",
		"InteractiveComponent (*spatial.InteractiveComponent)",
	} {
		if !strings.Contains(objHelp, want) {
			t.Fatalf("expected help(selected) output to contain %q, got:\n%s", want, objHelp)
		}
	}
	if strings.Contains(objHelp, "Properties: id, name, active, scene, position") {
		t.Fatalf("expected GameObject Properties not to list default transform/position fields:\n%s", objHelp)
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

	// 1. Under Strict ECS, GameObjects start with zero components; attach Transform explicitly from Lua.
	hasTr := console.ExecuteCommand(`return selected:HasComponent("Transform")`)
	if hasTr != "false" {
		t.Fatalf("expected new GameObject HasComponent('Transform') == false, got %q", hasTr)
	}
	console.ExecuteCommand(`
		local tc = selected:AddComponent("Transform")
		tc:SetPosition(11, 22, 33)
		tc.x = 15
	`)
	pos := obj.GetWorldTransform().Position
	if pos.X != 15 || pos.Y != 22 || pos.Z != 33 {
		t.Fatalf("expected Transform Lua mutation synced to GameObject, got %+v", pos)
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

	// 3. MeshRenderable enforces both Transform and MeshComponent constraints; Mesh & MeshRenderable class constants + help work.
	meshErr := console.ExecuteCommand(`selected:AddComponent(MeshRenderable)`)
	if !strings.Contains(meshErr, "Error:") {
		t.Fatalf("expected AddComponent(MeshRenderable) error without MeshComponent, got %q", meshErr)
	}
	meshHelp := console.ExecuteCommand(`help(Mesh)`)
	transformClassHelp := console.ExecuteCommand(`help(Transform)`)
	meshRendHelp := console.ExecuteCommand(`help(MeshRenderable)`)
	if !strings.Contains(meshHelp, "Component Help: Mesh (*engine.MeshComponent)") ||
		!strings.Contains(meshHelp, "GetTriangleCount()") ||
		!strings.Contains(transformClassHelp, "Component Help: Transform (*engine.TransformComponent)") ||
		!strings.Contains(meshRendHelp, "Component Help: MeshRenderable (*engine.MeshRenderable)") {
		t.Fatalf("expected class-constant help output, got:\n%s\n%s\n%s", meshHelp, transformClassHelp, meshRendHelp)
	}

	// Read-only constant & unknown class/variable error checks:
	roRes := console.ExecuteCommand(`return pcall(function() Mesh.name = "hack" end)`)
	if !strings.HasPrefix(roRes, "false") {
		t.Fatalf("expected ComponentClass constant to reject mutation, got %q", roRes)
	}
	for _, unknownCmd := range []string{`help(UnknownClass)`, `help("UnknownClass")`, `help UnknownClass`} {
		unkOut := console.ExecuteCommand(unknownCmd)
		if !strings.Contains(unkOut, "Error:") || !strings.Contains(unkOut, `no class name or variable exists named "UnknownClass"`) {
			t.Fatalf("expected explicit missing class/variable error for %q, got %q", unknownCmd, unkOut)
		}
	}
	ccHelp := console.ExecuteCommand(`help(CharacterController)`)
	if !strings.Contains(ccHelp, "Component Help: CharacterController (*spatial.CharacterController)") {
		t.Fatalf("expected help(CharacterController) component menu, got:\n%s", ccHelp)
	}
	varHelp := console.ExecuteCommand(`myProbe = { a = 1 }; return help(myProbe)`)
	if strings.Contains(varHelp, "Error:") {
		t.Fatalf("expected help(myProbe) for defined variable not to error, got %q", varHelp)
	}

	// 4. selected:GetComponent(<Class>) returns JSON dump if present or Nil if missing.
	trJSON := console.ExecuteCommand(`selected:GetComponent(Transform)`)
	if !strings.Contains(trJSON, `"name": "Transform"`) || !strings.Contains(trJSON, `"type": "*engine.TransformComponent"`) {
		t.Fatalf("expected JSON dump from selected:GetComponent(Transform), got:\n%s", trJSON)
	}
	missingMesh := console.ExecuteCommand(`selected:GetComponent(Mesh)`)
	if missingMesh != "Nil" {
		t.Fatalf("expected Nil when selected has no Mesh component, got %q", missingMesh)
	}
	console.ExecuteCommand(`selected:AddComponent(Mesh, "../../assets/models/props/rainbow_cube.gltf")`)
	if console.ExecuteCommand(`return selected:HasComponent(Mesh)`) != "true" {
		t.Fatalf("expected HasComponent(Mesh) == true after AddComponent(Mesh, ...)")
	}
	meshJSON := console.ExecuteCommand(`selected:GetComponent(Mesh)`)
	if !strings.Contains(meshJSON, `"name": "Mesh"`) ||
		!strings.Contains(meshJSON, `"type": "*engine.MeshComponent"`) ||
		!strings.Contains(meshJSON, `"triangles": 12`) ||
		!strings.Contains(meshJSON, `"loaded": true`) {
		t.Fatalf("expected Mesh component JSON dump from selected:GetComponent(Mesh), got:\n%s", meshJSON)
	}

	// 5. Removing Transform violates Renderable constraint and blocks adding another Renderable.
	console.ExecuteCommand(`selected:RemoveComponent(Renderable); selected:RemoveComponent(Transform)`)
	errOut := console.ExecuteCommand(`selected:AddComponent(Renderable, "mesh", "mat")`)
	if !strings.Contains(errOut, "Error:") || !strings.Contains(errOut, "Transform") {
		t.Fatalf("expected AddComponent(Renderable) error when Transform missing, got %q", errOut)
	}
}
