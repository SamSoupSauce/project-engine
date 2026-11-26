package luabridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestTicket17ScriptEditorEditFormatValidateAndHotReload(t *testing.T) {
	tmpDir := t.TempDir()
	compDir := filepath.Join(tmpDir, "components")
	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	// Initial single-file component script on disk with -- @data and -- @hooks headers.
	flammablePath := filepath.Join(compDir, "Flammable.lua")
	initialSrc := `-- @data: {"Lit":false,"BurnTime":10}
-- @hooks: {"Ignite":["GameObject"]}
-- @component: Flammable
Flammable = Component("Flammable", { ["BurnTime"] = 10, ["Lit"] = false }, { ["Ignite"] = { "GameObject" } })
-- @end-component

function Flammable:Ignite(sender)
  if not self.Lit then
    self.Lit = true
  end
end
`
	if err := os.WriteFile(flammablePath, []byte(initialSrc), 0o644); err != nil {
		t.Fatalf("write initial script failed: %v", err)
	}

	bridge := NewLuaBridge()
	defer bridge.Close()
	bridge.ScriptEditor().SetScriptsDir(tmpDir)
	if _, err := bridge.LoadScriptsDir(tmpDir); err != nil {
		t.Fatalf("LoadScriptsDir failed: %v", err)
	}

	console := NewConsole(bridge)
	overlay := engine.NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()
	overlay.SetConsole(console)
	overlay.SetScriptEditor(bridge.ScriptEditor())

	// Create a live entity with Flammable and mutate its live state before hot-reload.
	scene := engine.NewBaseScene("EditorHotReloadScene")
	bridge.BindScene(scene)
	crate := engine.NewGameObject("crate_1", "WoodenCrate")
	torch := engine.NewGameObject("torch_1", "Torch")
	scene.AddObject(crate)
	scene.AddObject(torch)
	bridge.RegisterGameObject("crate", crate)
	bridge.RegisterGameObject("torch", torch)
	scene.Init()

	console.ExecuteCommand(`
		crate = pick("crate")
		torch = pick("torch")
		local c = crate:AddComponent(Flammable)
		crate:CallHook("Ignite", torch)
		assert(c.Lit == true)
	`)

	// 1. Running `edit("Flammable")` in the dev console opens the Dear ImGui ScriptEditorWindow.
	out := console.ExecuteCommand(`edit("Flammable")`)
	if strings.Contains(out, "Error:") {
		t.Fatalf("edit('Flammable') returned error: %s", out)
	}
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	edWin := overlay.ScriptEditor
	if !edWin.IsOpen {
		t.Fatalf("expected ScriptEditorWindow to be open after edit('Flammable')")
	}
	if filepath.Clean(edWin.Doc.FilePath) != filepath.Clean(flammablePath) {
		t.Fatalf("expected FilePath %q, got %q", flammablePath, edWin.Doc.FilePath)
	}
	if !strings.Contains(edWin.Doc.DataJSON, `"Lit": false`) || !strings.Contains(edWin.Doc.HooksJSON, `"Ignite"`) {
		t.Fatalf("unexpected parsed Data/Hooks:\nData: %s\nHooks: %s", edWin.Doc.DataJSON, edWin.Doc.HooksJSON)
	}
	if strings.Contains(edWin.Doc.FunctionsLua, "@component:") || !strings.Contains(edWin.Doc.FunctionsLua, "function Flammable:Ignite") {
		t.Fatalf("expected clean FunctionsLua without @component header, got:\n%s", edWin.Doc.FunctionsLua)
	}

	// 2. Malformed JSON or invalid Lua syntax fails Commit cleanly without overwriting disk.
	edWin.Doc.DataJSON = `{ "Lit": `
	if err := edWin.Commit(); err == nil || !edWin.StatusIsErr {
		t.Fatalf("expected malformed DataJSON commit to fail")
	}
	edWin.Revert()

	edWin.Doc.HooksJSON = `{ "Ignite": 42 }`
	if err := edWin.Commit(); err == nil || !edWin.StatusIsErr {
		t.Fatalf("expected invalid HooksJSON commit to fail")
	}
	edWin.Revert()

	edWin.Doc.FunctionsLua = "function Flammable:Ignite(sender\n  end"
	if err := edWin.Commit(); err == nil || !edWin.StatusIsErr {
		t.Fatalf("expected Lua syntax error commit to fail")
	}
	edWin.Revert()

	// Verify disk content remained untouched after failed commits.
	afterBadRaw, _ := os.ReadFile(flammablePath)
	if string(afterBadRaw) != initialSrc {
		t.Fatalf("expected disk file unchanged after invalid commits")
	}

	// 3. Valid edit adds new field `Fuel = 50`, new hook `Extinguish = {"GameObject"}`, and new method.
	edWin.Doc.DataJSON = `{
  "Lit": false,
  "BurnTime": 10,
  "Fuel": 50
}`
	edWin.Doc.HooksJSON = `{
  "Ignite": ["GameObject"],
  "Extinguish": ["GameObject"]
}`
	edWin.Doc.FunctionsLua = `function Flammable:Ignite(sender)
  if not self.Lit then
    self.Lit = true
  end
end

function Flammable:Extinguish(sender)
  self.Lit = false
  self.ExtinguishedBy = sender:GetID()
end
`
	if err := edWin.Commit(); err != nil {
		t.Fatalf("expected valid Commit to succeed, got %v", err)
	}
	if edWin.Doc.ContentHash == "" || bridge.ScriptEditor().LastHash(flammablePath) != edWin.Doc.ContentHash {
		t.Fatalf("expected SHA-256 content hash recorded on Commit")
	}

	// 4. Verify live entity preserved `Lit == true`, merged new default `Fuel == 50`,
	//    and hot-reloaded the new `Extinguish` hook & method!
	res := console.ExecuteCommand(`
		local c = crate:GetComponent(Flammable)
		assert(c.Lit == true, "expected live Lit=true preserved across hot-reload")
		assert(c.Fuel == 50, "expected new default Fuel=50 merged into live component")
		crate:CallHook("Extinguish", torch)
		assert(c.Lit == false, "expected hot-reloaded Extinguish hook to set Lit=false")
		return c.ExtinguishedBy
	`)
	if res != "torch_1" {
		t.Fatalf("expected hot-reloaded Extinguish hook to set ExtinguishedBy='torch_1', got %q", res)
	}

	// 5. Calling `edit()` with no args closes the ImGui window.
	console.ExecuteCommand(`edit()`)
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()
	if edWin.IsOpen {
		t.Fatalf("expected edit() with no arguments to close ScriptEditorWindow")
	}

	// 6. Opening an unknown short name creates a new template under <scriptsDir>/components/<Name>.lua on Commit.
	console.ExecuteCommand(`edit("Wettable")`)
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()
	if !edWin.IsOpen || !strings.HasSuffix(filepath.ToSlash(edWin.Doc.FilePath), "/components/Wettable.lua") {
		t.Fatalf("expected new template path under components/Wettable.lua, got %q (open=%v)", edWin.Doc.FilePath, edWin.IsOpen)
	}
	if err := edWin.Commit(); err != nil {
		t.Fatalf("commit new Wettable.lua failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(compDir, "Wettable.lua")); err != nil {
		t.Fatalf("expected Wettable.lua created on disk: %v", err)
	}
	if console.ExecuteCommand(`return Wettable ~= nil and Wettable.name`) != "Wettable" {
		t.Fatalf("expected newly committed Wettable component registered in LuaBridge")
	}
}
