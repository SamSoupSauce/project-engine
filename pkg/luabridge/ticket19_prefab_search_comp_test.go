package luabridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

func TestTicket19PrefabsSaveSearchSpawnAndEditorUI(t *testing.T) {
	prefabsDir := filepath.Join(t.TempDir(), "prefabs")

	bridge := NewLuaBridge()
	defer bridge.Close()
	bridge.PrefabEditor().SetPrefabsDir(prefabsDir)

	scene := engine.NewBaseScene("PrefabTestScene")
	bridge.BindScene(scene)
	console := NewConsole(bridge)

	overlay := engine.NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()
	overlay.SetConsole(console)
	overlay.SetPrefabEditor(bridge.PrefabEditor())

	// Build a GameObject with Transform, InteractiveComponent, FractureComponent, and a child node.
	crate := engine.NewGameObject("crate_proto", "WoodenCrate")
	tc := engine.NewTransformComponentAt(engine.NewVector3(2, 3, 4))
	tc.SetRotation(engine.NewVector3(0, 45, 0))
	tc.SetScale(engine.NewVector3(1.5, 1.5, 1.5))
	crate.AddComponent(tc)
	ic := spatial.NewInteractiveComponent("Crate", false)
	crate.AddComponent(ic)
	fc := spatial.NewFractureComponent(spatial.FractureSplinter, spatial.IntegrityStandard, 30)
	crate.AddComponent(fc)

	child := engine.NewGameObject("crate_lid", "CrateLid")
	child.AddComponent(engine.NewTransformComponentAt(engine.NewVector3(0, 0.5, 0)))
	crate.AddChild(child)

	scene.AddObject(crate)
	bridge.RegisterGameObject("crate_proto", crate)
	scene.Init()
	console.PickObject(crate)

	// 1. Save selected GameObject as JSON prefab into dedicated prefabs folder.
	out := console.ExecuteCommand(`save_prefab("wooden_crate")`)
	expectedPath := filepath.Join(prefabsDir, "wooden_crate.json")
	if filepath.Clean(out) != filepath.Clean(expectedPath) {
		t.Fatalf("expected saved path %q, got %q", expectedPath, out)
	}
	raw, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("expected prefab JSON on disk: %v", err)
	}
	var parsed engine.PrefabSpec
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid prefab JSON: %v", err)
	}
	if parsed.Name != "WoodenCrate" || len(parsed.Components) != 3 || len(parsed.Children) != 1 {
		t.Fatalf("unexpected parsed prefab JSON: %+v", parsed)
	}

	// Also save a second prefab via CLI command `prefab save`.
	cliSave := console.ExecuteCommand(`prefab save crate_variant crate_proto`)
	if strings.Contains(cliSave, "Error:") {
		t.Fatalf("prefab save CLI failed: %s", cliSave)
	}

	// 2. Search prefabs via Lua `search_prefabs` and CLI `prefabs` / `prefab search`.
	luaCount := console.ExecuteCommand(`return #search_prefabs("wooden_crate")`)
	if luaCount != "1" {
		t.Fatalf("expected 1 match for search_prefabs('wooden_crate'), got %q", luaCount)
	}
	woodenNameCount := console.ExecuteCommand(`return #search_prefabs("WoodenCrate")`)
	if woodenNameCount != "2" {
		t.Fatalf("expected 2 matches for search_prefabs('WoodenCrate'), got %q", woodenNameCount)
	}
	compMatch := console.ExecuteCommand(`return #search_prefabs("FractureComponent")`)
	if compMatch != "2" {
		t.Fatalf("expected 2 matches for component query 'FractureComponent', got %q", compMatch)
	}
	cliSearch := console.ExecuteCommand(`prefabs variant`)
	if !strings.Contains(cliSearch, "crate_variant.json") || strings.Contains(cliSearch, "wooden_crate.json") {
		t.Fatalf("unexpected CLI prefabs search output: %s", cliSearch)
	}

	// 3. Spawn prefab into scene and verify Transform, components, and children are reconstructed.
	spawnOut := console.ExecuteCommand(`
		local spawned = spawn_prefab("wooden_crate", { position = { x = 10, y = 20, z = 30 } })
		local pos = spawned:GetComponent(Transform):GetLocation()
		local inter = spawned:GetComponent(InteractiveComponent)
		local frac = spawned:GetComponent(FractureComponent)
		local kids = spawned:GetChildren()
		return string.format("%s|%.0f,%.0f,%.0f|%s|%.0f|%d",
			spawned:GetName(), pos.x, pos.y, pos.z, inter:GetDisplayName(), frac:GetHealth(), #kids)
	`)
	if spawnOut != "WoodenCrate|10,20,30|Crate|30|1" {
		t.Fatalf("unexpected spawned prefab state: %q", spawnOut)
	}

	// 4. Open ImGui Prefab Manager via console `edit_prefab("wooden_crate")`, edit & save, then spawn.
	uiOut := console.ExecuteCommand(`edit_prefab("wooden_crate")`)
	if strings.Contains(uiOut, "Error:") {
		t.Fatalf("edit_prefab failed: %s", uiOut)
	}
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	pWin := overlay.PrefabEditor
	if !pWin.IsOpen || pWin.Doc.Name != "WoodenCrate" {
		t.Fatalf("expected PrefabEditorWindow open with WoodenCrate, open=%v name=%q", pWin.IsOpen, pWin.Doc.Name)
	}
	pWin.Doc.Spec.Name = "ReinforcedCrate"
	pWin.SyncJSONFromSpec()
	if err := pWin.Save(); err != nil {
		t.Fatalf("PrefabEditorWindow.Save failed: %v", err)
	}
	spawnedID, err := pWin.Spawn()
	if err != nil || spawnedID == "" {
		t.Fatalf("PrefabEditorWindow.Spawn failed: id=%q err=%v", spawnedID, err)
	}
	if obj := bridge.GetGameObject(spawnedID); obj == nil || obj.Name != "ReinforcedCrate" {
		t.Fatalf("expected spawned GameObject named 'ReinforcedCrate', got %+v", obj)
	}
}

func TestTicket19ScriptSearchAndComponentCommand(t *testing.T) {
	scriptsDir := t.TempDir()
	compDir := filepath.Join(scriptsDir, "components")
	propsDir := filepath.Join(scriptsDir, "props")
	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatalf("mkdir components failed: %v", err)
	}
	if err := os.MkdirAll(propsDir, 0o755); err != nil {
		t.Fatalf("mkdir props failed: %v", err)
	}

	spinnerPath := filepath.Join(compDir, "Spinner.lua")
	spinnerSrc := `-- @data: {"speed":3.5,"active":true}
-- @hooks: {"Spin":["number"]}
-- @component: Spinner
Spinner = Component("Spinner", { ["active"] = true, ["speed"] = 3.5 }, { ["Spin"] = { "number" } })
-- @end-component

function Spinner:Spin(dt)
  self.speed = self.speed + dt
  return self.speed
end
`
	if err := os.WriteFile(spinnerPath, []byte(spinnerSrc), 0o644); err != nil {
		t.Fatalf("write Spinner.lua failed: %v", err)
	}
	toasterSrc := "return { meta = { name = 'Toaster' } }\n"
	if err := os.WriteFile(filepath.Join(propsDir, "toaster.lua"), []byte(toasterSrc), 0o644); err != nil {
		t.Fatalf("write toaster.lua failed: %v", err)
	}

	bridge := NewLuaBridge()
	defer bridge.Close()
	bridge.ScriptEditor().SetScriptsDir(scriptsDir)
	if _, err := bridge.LoadScriptsDir(scriptsDir); err != nil {
		t.Fatalf("LoadScriptsDir failed: %v", err)
	}
	console := NewConsole(bridge)

	// 1. Script search functions (Lua & CLI)
	allScripts := console.ExecuteCommand(`return #search_scripts()`)
	if allScripts != "2" {
		t.Fatalf("expected 2 scripts total, got %q", allScripts)
	}
	spinMatch := console.ExecuteCommand(`
		local res = search_scripts("Spin")
		return res[1].name .. "|" .. res[1].component
	`)
	if spinMatch != "Spinner|Spinner" {
		t.Fatalf("expected Spinner script search match, got %q", spinMatch)
	}
	cliScripts := console.ExecuteCommand(`scripts toaster`)
	if !strings.Contains(cliScripts, "toaster") || strings.Contains(cliScripts, "Spinner") {
		t.Fatalf("unexpected CLI scripts search output: %s", cliScripts)
	}

	// 2. Component search functions (Lua & CLI)
	compSearch := console.ExecuteCommand(`comp search Spinner`)
	if !strings.Contains(compSearch, "Spinner [custom]") {
		t.Fatalf("expected Spinner in comp search output, got:\n%s", compSearch)
	}
	luaCompSearch := console.ExecuteCommand(`
		local res = search_components("Transform")
		return res[1].name
	`)
	if luaCompSearch != "Transform" {
		t.Fatalf("expected first match for search_components('Transform') to be 'Transform', got %q", luaCompSearch)
	}

	// 3. Component interaction via easy `comp` command on picked object
	box := engine.NewGameObject("box_1", "TestBox")
	bridge.RegisterGameObject("box_1", box)
	console.PickObject(box)

	if out := console.ExecuteCommand(`comp add Transform 1 2 3`); strings.Contains(out, "Error:") {
		t.Fatalf("comp add Transform failed: %s", out)
	}
	if out := console.ExecuteCommand(`comp set Transform position 7 8 9`); strings.Contains(out, "Error:") {
		t.Fatalf("comp set Transform position failed: %s", out)
	}
	if got := console.ExecuteCommand(`comp get Transform x`); got != "7" {
		t.Fatalf("expected Transform.x == 7, got %q", got)
	}

	if out := console.ExecuteCommand(`comp add Spinner`); strings.Contains(out, "Error:") {
		t.Fatalf("comp add Spinner failed: %s", out)
	}
	if out := console.ExecuteCommand(`comp set Spinner speed 10`); strings.Contains(out, "Error:") {
		t.Fatalf("comp set Spinner speed failed: %s", out)
	}
	if callOut := console.ExecuteCommand(`comp call Spinner Spin 2.5`); callOut != "12.5" {
		t.Fatalf("expected comp call Spinner Spin 2.5 -> 12.5, got %q", callOut)
	}
	if listOut := console.ExecuteCommand(`comp list`); !strings.Contains(listOut, "Transform") || !strings.Contains(listOut, "Spinner") {
		t.Fatalf("expected comp list to show Transform and Spinner, got:\n%s", listOut)
	}
	if rmOut := console.ExecuteCommand(`comp remove Spinner`); strings.Contains(rmOut, "Error:") {
		t.Fatalf("comp remove Spinner failed: %s", rmOut)
	}
	if has := console.ExecuteCommand(`return selected:HasComponent(Spinner)`); has != "false" {
		t.Fatalf("expected Spinner removed, got HasComponent=%q", has)
	}
}
