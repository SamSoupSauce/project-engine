package luabridge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestGracefulScriptLoadAndRuntimeErrors(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	compDir := filepath.Join(scriptsDir, "components")
	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write one broken script and one valid script in alphabetical order (a_broken runs first).
	brokenPath := filepath.Join(compDir, "a_broken.lua")
	if err := os.WriteFile(brokenPath, []byte(`this is invalid lua syntax !!!`), 0o644); err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(compDir, "b_good.lua")
	validSrc := `
GoodSpinner = Component("GoodSpinner", { speed = 2.0 }, { tick = { "number" } })
function GoodSpinner:tick(dt)
	self.speed = self.speed + dt
end
`
	if err := os.WriteFile(validPath, []byte(validSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	bridge := NewLuaBridge()
	defer bridge.Close()
	console := NewConsole(bridge)

	var recorded []string
	bridge.OnScriptError = func(path, hook string, err error) {
		recorded = append(recorded, filepath.Base(path)+":"+hook)
	}

	loaded, err := bridge.LoadScriptsDir(scriptsDir)
	if err == nil {
		t.Fatalf("expected ScriptLoadErrors when a_broken.lua fails")
	}
	var loadErrs ScriptLoadErrors
	if !errors.As(err, &loadErrs) {
		t.Fatalf("expected ScriptLoadErrors, got %T (%v)", err, err)
	}
	if len(loadErrs) != 1 {
		t.Fatalf("expected 1 failed script, got %d", len(loadErrs))
	}
	if len(loaded) != 1 || filepath.Clean(loaded[0]) != filepath.Clean(validPath) {
		t.Fatalf("expected b_good.lua to still load despite a_broken.lua failure, got loaded=%v", loaded)
	}
	if len(recorded) == 0 || recorded[0] != "a_broken.lua:load" {
		t.Fatalf("expected OnScriptError callback for a_broken.lua:load, got %v", recorded)
	}

	// Valid component should be registered and usable.
	if comps := bridge.SearchComponents("GoodSpinner"); len(comps) != 1 {
		t.Fatalf("expected GoodSpinner component to be registered, got %v", comps)
	}

	// Verify script error history and console output capture.
	errs := bridge.GetScriptErrors()
	if len(errs) == 0 {
		t.Fatalf("expected GetScriptErrors to record failure")
	}
	foundConsoleErr := false
	for _, line := range console.Output {
		if line.Type == LineError && strings.Contains(line.Text, "a_broken.lua") {
			foundConsoleErr = true
			break
		}
	}
	if !foundConsoleErr {
		t.Fatalf("expected console to receive LineError for a_broken.lua")
	}

	bridge.ClearScriptErrors()
	if len(bridge.GetScriptErrors()) != 0 {
		t.Fatalf("expected ClearScriptErrors to empty error list")
	}
}

func TestBridgeWorkspacePathsAndPrefabAssetResolution(t *testing.T) {
	tmp := t.TempDir()
	customRoot := filepath.Join(tmp, "game_root")
	paths := engine.ResolvedWorkspacePaths{
		RootDir:    customRoot,
		AssetsDir:  filepath.Join(customRoot, "custom_assets"),
		ScriptsDir: filepath.Join(customRoot, "scripts"),
		PrefabsDir: filepath.Join(customRoot, "prefabs"),
		ConfigDir:  filepath.Join(customRoot, "config"),
	}
	if _, err := engine.InitWorkspaceDirs(paths); err != nil {
		t.Fatal(err)
	}

	// Copy rainbow_cube.gltf into custom_assets/models/props/rainbow_cube.gltf.
	repoCube, err := os.ReadFile(filepath.Join("..", "..", "assets", "models", "props", "rainbow_cube.gltf"))
	if err != nil {
		t.Fatal(err)
	}
	targetModelDir := filepath.Join(paths.AssetsDir, "models", "props")
	if err := os.MkdirAll(targetModelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetModelDir, "rainbow_cube.gltf"), repoCube, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a prefab referencing "assets/models/props/rainbow_cube.gltf" inside custom prefabs/.
	repoPrefab, err := os.ReadFile(filepath.Join("..", "..", "prefabs", "rainbow_cube.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.PrefabsDir, "rainbow_cube.json"), repoPrefab, 0o644); err != nil {
		t.Fatal(err)
	}

	bridge := NewLuaBridge()
	defer bridge.Close()
	bridge.SetWorkspacePaths(paths)

	out, err := bridge.Execute(`local wp = workspace_paths(); return wp.assets`)
	if err != nil {
		t.Fatalf("workspace_paths() failed: %v", err)
	}
	if filepath.Clean(out) != filepath.Clean(paths.AssetsDir) {
		t.Fatalf("expected workspace_paths().assets=%q, got %q", paths.AssetsDir, out)
	}

	scene := engine.NewBaseScene("CustomRootScene")
	bridge.BindScene(scene)
	obj, err := bridge.PrefabEditor().SpawnPrefabObject("rainbow_cube", PrefabSpawnOptions{AutoAdd: true})
	if err != nil {
		t.Fatalf("SpawnPrefabObject under custom root failed: %v", err)
	}
	if obj.GetMeshComponent() == nil || obj.GetMeshComponent().Asset == nil {
		t.Fatalf("expected MeshComponent to load GLTF from custom assets_dir")
	}
}

func TestLuaExamplesExecuteCleanly(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	_, paths, err := engine.LoadWorkspaceConfig(repoRoot, true, "")
	if err != nil {
		t.Fatal(err)
	}

	bridge := NewLuaBridge()
	defer bridge.Close()
	bridge.SetWorkspacePaths(paths)
	scene := engine.NewBaseScene("LuaExamplesScene")
	bridge.BindScene(scene)

	examples := []string{
		filepath.Join(repoRoot, "examples", "lua", "01_hello_shell_commands.lua"),
		filepath.Join(repoRoot, "examples", "lua", "02_custom_component.lua"),
		filepath.Join(repoRoot, "examples", "lua", "03_prefabs_and_events.lua"),
		filepath.Join(repoRoot, "examples", "lua", "04_jolt_physics_and_raycast.lua"),
	}
	for _, ex := range examples {
		if _, err := bridge.Execute("scripts.run " + filepath.ToSlash(ex)); err != nil {
			t.Fatalf("failed executing Lua example %s: %v", ex, err)
		}
	}

	// Verify dynamic shell commands from 01 and 03 work.
	out, err := bridge.Execute("greet EngineTester")
	if err != nil || !strings.Contains(out, "Hello, EngineTester!") {
		t.Fatalf("expected greet output, got out=%q err=%v", out, err)
	}
	out, err = bridge.Execute("spawn_showcase_cube 1 2 3")
	if err != nil || !strings.Contains(out, "Spawned RainbowCube") {
		t.Fatalf("expected spawn_showcase_cube output, got out=%q err=%v", out, err)
	}

	// Verify 05_zerocopy_hud_luajit.lua loads and renders cleanly under LuaJITBridge.
	jitBridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge failed: %v", err)
	}
	defer jitBridge.Close()
	hudPath := filepath.Join(repoRoot, "examples", "lua", "05_zerocopy_hud_luajit.lua")
	if err := jitBridge.DoFile(hudPath); err != nil {
		t.Fatalf("failed loading LuaJIT HUD example %s: %v", hudPath, err)
	}
	if err := jitBridge.CallRenderPtr("render_example_hud", nil, 1280, 720); err != nil {
		t.Fatalf("render_example_hud failed: %v", err)
	}
}
