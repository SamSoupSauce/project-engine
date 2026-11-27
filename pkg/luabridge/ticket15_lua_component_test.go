package luabridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestTicket15LuaComponentFactoryFieldsMethodsHooksAndHelp(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()
	console := NewConsole(bridge)

	scene := engine.NewBaseScene("ComponentTestScene")
	bridge.BindScene(scene)

	crate := engine.NewGameObject("crate_01", "Crate")
	barrel := engine.NewGameObject("barrel_01", "Barrel")
	torch := engine.NewGameObject("torch_01", "Torch")
	scene.AddObject(crate)
	scene.AddObject(barrel)
	scene.AddObject(torch)
	bridge.RegisterGameObject("crate", crate)
	bridge.RegisterGameObject("barrel", barrel)
	bridge.RegisterGameObject("torch", torch)
	scene.Init()
	console.PickObject(crate)

	// 1. Define custom Lua Component with fields, hooks (both string name and GameObject token), and methods.
	out := console.ExecuteCommand(`
		Flammable = Component("Flammable", {
			Lit = false,
			BurnTime = 10.0,
			Tags = { "wood" },
		}, {
			Ignite = { "GameObject" },
			Extinguish = {},
		})

		function Flammable:Ignite(sender)
			if not self.Lit then
				self.Lit = true
				self.IgnitedBy = sender:GetID()
			end
		end

		function Flammable:Extinguish()
			self.Lit = false
		end

		Wettable = Component("Wettable", { Wet = false }, { Soak = { GameObject } })
		function Wettable:Soak(sender)
			self.Wet = true
			self.SoakedBy = sender:GetID()
		end
	`)
	if strings.Contains(out, "Error:") {
		t.Fatalf("unexpected error defining custom components: %s", out)
	}

	// 2. Attach to multiple entities, verify isolated defaults + overrides + nested table cloning.
	res := console.ExecuteCommand(`
		crate = pick("crate")
		barrel = pick("barrel")
		torch = pick("torch")
		pick("crate")
		local c1 = crate:AddComponent(Flammable)
		local c2 = barrel:AddComponent("Flammable", { BurnTime = 25.0 })
		local w1 = crate:AddComponent(Wettable)

		assert(crate:HasComponent(Flammable) == true)
		assert(crate:HasComponent("Wettable") == true)
		assert(c1.Lit == false and c1.BurnTime == 10.0)
		assert(c2.Lit == false and c2.BurnTime == 25.0)

		table.insert(c1.Tags, "charred")
		assert(#c1.Tags == 2 and #c2.Tags == 1, "nested default tables must be cloned per instance")

		crate:CallHook("Ignite", torch)
		assert(c1.Lit == true and c1.IgnitedBy == "torch_01")
		assert(c2.Lit == false, "instance states must remain isolated")

		-- EmitTo dispatches matching hook on LuaComponent too.
		torch:EmitTo(crate, "Soak", nil)
		assert(w1.Wet == true and w1.SoakedBy == "torch_01")

		return #crate:GetComponents()
	`)
	if res != "2" {
		t.Fatalf("expected 2 components on crate, got %q", res)
	}

	// 3. Hook signature validation rejects nil or mismatched argument types cleanly.
	badNil := console.ExecuteCommand(`return pcall(function() crate:CallHook("Ignite") end)`)
	if !strings.HasPrefix(badNil, "false") || !strings.Contains(badNil, "GameObject") {
		t.Fatalf("expected CallHook('Ignite') with nil arg to fail validation, got %q", badNil)
	}
	badStr := console.ExecuteCommand(`return pcall(function() crate:CallHook("Ignite", "not_a_go") end)`)
	if !strings.HasPrefix(badStr, "false") || !strings.Contains(badStr, "GameObject") {
		t.Fatalf("expected CallHook('Ignite', 'not_a_go') to fail validation, got %q", badStr)
	}

	// 4. Dynamic methods declared on built-in ComponentClass types (e.g., Transform) work on instances.
	builtInRes := console.ExecuteCommand(`
		function Transform:MoveToOrigin()
			self:SetPosition(0, 0, 0)
		end
		local tr = crate:AddComponent(Transform, 5, 6, 7)
		assert(tr.x == 5)
		tr:MoveToOrigin()
		local x, y, z = tr:GetPosition()
		return string.format("%.0f,%.0f,%.0f", x, y, z)
	`)
	if builtInRes != "0,0,0" {
		t.Fatalf("expected dynamic method on Transform component to move to 0,0,0, got %q", builtInRes)
	}

	// 5. help(...) and JSON serialization support custom Lua components and GameObject token.
	classHelp := console.ExecuteCommand(`help(Flammable)`)
	for _, want := range []string{
		"Component Help: Flammable (*luabridge.LuaComponent)",
		"BurnTime",
		"Lit",
		"Ignite(GameObject)",
		"Extinguish()",
	} {
		if !strings.Contains(classHelp, want) {
			t.Fatalf("expected help(Flammable) to contain %q, got:\n%s", want, classHelp)
		}
	}
	goHelp := console.ExecuteCommand(`help(GameObject)`)
	if !strings.Contains(goHelp, "Type Help: GameObject (*engine.GameObject)") || !strings.Contains(goHelp, "CallHook") {
		t.Fatalf("expected help(GameObject) output, got:\n%s", goHelp)
	}
	compJSON := console.ExecuteCommand(`crate:GetComponent(Flammable)`)
	if !strings.Contains(compJSON, `"name": "Flammable"`) || !strings.Contains(compJSON, `"IgnitedBy": "torch_01"`) {
		t.Fatalf("expected JSON dump for Flammable instance, got:\n%s", compJSON)
	}

	// 6. RemoveComponent removes only the targeted custom component type.
	if rem := console.ExecuteCommand(`return crate:RemoveComponent(Wettable), crate:HasComponent(Wettable), crate:HasComponent(Flammable)`); !strings.Contains(rem, "true\nfalse\ntrue") {
		t.Fatalf("expected RemoveComponent(Wettable) to keep Flammable intact, got %q", rem)
	}
}

func TestTicket15LoadScriptsDirSkipsUIAndProps(t *testing.T) {
	tmpDir := t.TempDir()
	for _, sub := range []string{"components", "ui", "props"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", sub, err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "components", "Conductive.lua"), []byte(`
Conductive = Component("Conductive", { Voltage = 0 }, { Shock = { GameObject } })
function Conductive:Shock(sender)
	self.Voltage = 220
end
`), 0o644); err != nil {
		t.Fatalf("write component script failed: %v", err)
	}
	// Put an FFI script in ui/ and a prop manifest in props/ to verify both are skipped.
	if err := os.WriteFile(filepath.Join(tmpDir, "ui", "hud.lua"), []byte(`local ffi = require("ffi")`), 0o644); err != nil {
		t.Fatalf("write ui script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "props", "prop.lua"), []byte(`error("props should be skipped")`), 0o644); err != nil {
		t.Fatalf("write prop script failed: %v", err)
	}

	bridge := NewLuaBridge()
	defer bridge.Close()
	loaded, err := bridge.LoadScriptsDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadScriptsDir failed: %v", err)
	}
	if len(loaded) != 1 || !strings.HasSuffix(loaded[0], "Conductive.lua") {
		t.Fatalf("expected only Conductive.lua loaded, got %v", loaded)
	}
	out, err := bridge.Execute(`return Conductive.name`)
	if err != nil || out != "Conductive" {
		t.Fatalf("expected Conductive global registered, got %q (err=%v)", out, err)
	}
}
