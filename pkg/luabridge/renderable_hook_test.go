package luabridge

import (
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestScriptNameRenderHookAndRendererLuaAPI(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	var scriptErrors []string
	bridge.OnScriptError = func(path, hook string, err error) {
		scriptErrors = append(scriptErrors, path+":"+hook+":"+err.Error())
	}

	// 1. Script deriving prefix from filename ("hero_sprite.lua" -> hero_sprite_render)
	heroPath := writeTempLuaScript(t, "hero_sprite.lua", `
rendered = 0
last_viewport = nil

function hero_sprite_render(self, gfx)
	rendered = rendered + 1
	local w, h = gfx:GetViewportSize()
	last_viewport = string.format("%.0fx%.0f", w, h)
	local pos = self:GetPosition()
	gfx:DrawRect(pos.x, pos.y, 40, 20, 255, 128, 0, 255, 3)
	gfx:DrawQuad(0, 0, 16, 16, 42, 0, 0, 0.5, 0.5, 255, 255, 255, 255, 1)
	gfx:DrawMesh("hero_mesh", "pbr", pos.x, pos.y, pos.z, 200, 210, 220, 255)
	gfx:SubmitDrawCommand({
		mesh_id = "aura_ring",
		material_id = "additive",
		position = { x = pos.x, y = pos.y + 1, z = pos.z },
		color = { r = 0, g = 255, b = 128, a = 255 },
	})
end
`)

	// 2. Script using Renderable.ScriptName override ("BossOverlay_render")
	bossPath := writeTempLuaScript(t, "generic_boss.lua", `
boss_calls = 0

function BossOverlay_render(self, gfx)
	boss_calls = boss_calls + 1
	gfx:DrawRect(100, 10, 200, 24, { r = 200, g = 20, b = 20, a = 255 }, 10)
end
`)

	// 3. Script using unprefixed isolated render(self, gfx) fallback (two isolated instances)
	minionPath := writeTempLuaScript(t, "minion.lua", `
calls = 0

function render(self, gfx)
	calls = calls + 1
	gfx:DrawRect(calls * 10, 50, 8, 8, 255, 255, 255, 255, 0)
end
`)

	scene := engine.NewBaseScene("RenderHookScene")
	scene.SetViewportSize(1280, 720)

	hero := engine.NewGameObject("hero_1", "Hero")
	hero.Transform.Position = engine.NewVector3(12, 34, 56)
	heroScript, err := bridge.NewScriptComponent(heroPath)
	if err != nil {
		t.Fatalf("failed loading hero script: %v", err)
	}
	hero.AddComponent(engine.NewRenderable("base_cube", "default"))
	hero.AddComponent(heroScript)
	scene.AddObject(hero)

	boss := engine.NewGameObject("boss_1", "Boss")
	bossRenderable := engine.NewRenderable("boss_mesh", "lit")
	bossRenderable.ScriptName = "BossOverlay"
	bossScript, err := bridge.NewScriptComponent(bossPath)
	if err != nil {
		t.Fatalf("failed loading boss script: %v", err)
	}
	boss.AddComponent(bossRenderable)
	boss.AddComponent(bossScript)
	scene.AddObject(boss)

	minionA := engine.NewGameObject("m_a", "MinionA")
	minionAScript, err := bridge.NewScriptComponent(minionPath)
	if err != nil {
		t.Fatalf("failed loading minionA script: %v", err)
	}
	minionA.AddComponent(engine.NewRenderable("m", "default"))
	minionA.AddComponent(minionAScript)

	minionB := engine.NewGameObject("m_b", "MinionB")
	minionBScript, err := bridge.NewScriptComponent(minionPath)
	if err != nil {
		t.Fatalf("failed loading minionB script: %v", err)
	}
	minionB.AddComponent(engine.NewRenderable("m", "default"))
	minionB.AddComponent(minionBScript)

	// Before Init -> unspawned objects must not invoke render
	scene.Draw3D()
	if len(scene.GetRenderer().Commands2D) != 0 {
		t.Fatalf("expected 0 draw commands before scene.Init()")
	}

	scene.Init()
	scene.Spawn(minionA, hero)
	scene.Spawn(minionB, hero)

	// Run 1 render frame
	scene.Draw3D()
	// Run a 2nd frame for minionA only (hide minionB via Lua Renderable binding)
	bridge.stateMu.Lock()
	bridge.L.SetGlobal("__minionB", gameObjectToUserData(bridge.L, minionB))
	bridge.stateMu.Unlock()
	if _, err := bridge.Execute(`__minionB:GetRenderable():SetVisible(false)`); err != nil {
		t.Fatalf("failed hiding minionB from Lua: %v", err)
	}
	scene.Draw3D()

	if len(scriptErrors) > 0 {
		t.Fatalf("unexpected Lua script errors: %v", scriptErrors)
	}

	// Verify isolated script state
	bridge.stateMu.Lock()
	bridge.L.SetGlobal("__hero_env", heroScript.Env())
	bridge.L.SetGlobal("__boss_env", bossScript.Env())
	bridge.L.SetGlobal("__mA_env", minionAScript.Env())
	bridge.L.SetGlobal("__mB_env", minionBScript.Env())
	bridge.stateMu.Unlock()

	summary, err := bridge.Execute(`return string.format("hero=%d(%s) boss=%d mA=%d mB=%d", __hero_env.rendered, __hero_env.last_viewport, __boss_env.boss_calls, __mA_env.calls, __mB_env.calls)`)
	if err != nil {
		t.Fatalf("failed inspecting Lua script state: %v", err)
	}
	if summary != "hero=2(1280x720) boss=2 mA=2 mB=1" {
		t.Fatalf("unexpected render call counts: %s", summary)
	}

	// On frame 2: hero (2 quads + 2 custom 3D + 1 passive) + boss (1 quad + 1 passive) + minionA (1 quad + 1 passive)
	// minionB is hidden so omitted on frame 2.
	gfx := scene.GetRenderer()
	if len(gfx.Commands2D) != 4 {
		t.Fatalf("expected 4 2D commands on frame 2, got %d", len(gfx.Commands2D))
	}
	sorted := gfx.SortedCommands2D()
	if sorted[0].ZIndex != 0 || sorted[1].ZIndex != 1 || sorted[2].ZIndex != 3 || sorted[3].ZIndex != 10 {
		t.Errorf("unexpected ZIndex sort order: %d, %d, %d, %d", sorted[0].ZIndex, sorted[1].ZIndex, sorted[2].ZIndex, sorted[3].ZIndex)
	}
	if sorted[1].TextureID != 42 || sorted[1].SourceRect.W != 0.5 {
		t.Errorf("expected textured quad from DrawQuad, got %+v", sorted[1])
	}

	// 3 passive Renderables (hero, boss, minionA) + 2 script-submitted 3D meshes (hero_mesh, aura_ring) = 5
	cmds3D := gfx.Commands3D()
	if len(cmds3D) != 5 {
		t.Fatalf("expected 5 3D DrawCommands on frame 2, got %d", len(cmds3D))
	}
	foundAura := false
	for _, c := range cmds3D {
		if c.MeshID == "aura_ring" && c.WorldTransform.Position.Y == 35 && c.Color.G == 255 {
			foundAura = true
		}
	}
	if !foundAura {
		t.Errorf("expected aura_ring DrawCommand submitted from Lua, got %+v", cmds3D)
	}
}

func TestRenderHookGatingMissingHookGlobalAndErrorHandling(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	var recordedErr string
	bridge.OnScriptError = func(path, hook string, err error) {
		recordedErr = hook + ":" + err.Error()
	}

	// Global <ScriptName>_render function in _G alongside a ScriptComponent without its own render
	if _, err := bridge.Execute(`
global_render_count = 0
function GlobalWidget_render(self, gfx)
	global_render_count = global_render_count + 1
	gfx:DrawRect(0, 0, gfx.width, 4, 255, 255, 255, 255, 0)
end
`); err != nil {
		t.Fatalf("failed registering global render function: %v", err)
	}

	scene := engine.NewBaseScene("GateEdgeScene")

	// 1. Script without Renderable -> must NOT call render
	noRendPath := writeTempLuaScript(t, "no_rend.lua", `
called = 0
function no_rend_render(self, gfx) called = called + 1 end
function render(self, gfx) called = called + 1 end
`)
	noRendObj := engine.NewGameObject("no_rend", "NoRend")
	noRendSC, err := bridge.NewScriptComponent(noRendPath)
	if err != nil {
		t.Fatalf("failed loading no_rend.lua: %v", err)
	}
	noRendObj.AddComponent(noRendSC)
	scene.AddObject(noRendObj)

	// 2. Renderable with ScriptComponent that omits render hook -> clean no-op
	emptyPath := writeTempLuaScript(t, "empty_rend.lua", `x = 1`)
	emptyObj := engine.NewGameObject("empty", "Empty")
	emptySC, err := bridge.NewScriptComponent(emptyPath)
	if err != nil {
		t.Fatalf("failed loading empty_rend.lua: %v", err)
	}
	emptyObj.AddComponent(engine.NewRenderable("box", "default"))
	emptyObj.AddComponent(emptySC)
	scene.AddObject(emptyObj)

	// 3. Renderable with ScriptName pointing to global function GlobalWidget_render
	globalObj := engine.NewGameObject("glob", "GlobalWidgetObj")
	globalSC, err := bridge.NewScriptComponent(emptyPath)
	if err != nil {
		t.Fatalf("failed loading script for globalObj: %v", err)
	}
	globalRend := engine.NewRenderable("quad", "ui")
	globalRend.ScriptName = "GlobalWidget"
	globalObj.AddComponent(globalRend)
	globalObj.AddComponent(globalSC)
	scene.AddObject(globalObj)

	// 4. Script with runtime error in render hook -> caught cleanly
	errPath := writeTempLuaScript(t, "broken.lua", `
function broken_render(self, gfx)
	error("intentional render boom")
end
`)
	errObj := engine.NewGameObject("err_obj", "ErrObj")
	errSC, err := bridge.NewScriptComponent(errPath)
	if err != nil {
		t.Fatalf("failed loading broken.lua: %v", err)
	}
	errObj.AddComponent(engine.NewRenderable("box", "default"))
	errObj.AddComponent(errSC)
	scene.AddObject(errObj)

	scene.Init()
	scene.Draw3D()

	// Despawn globalObj and verify it stops rendering on next frame
	scene.Despawn(globalObj, nil, nil)
	scene.Draw3D()

	bridge.stateMu.Lock()
	bridge.L.SetGlobal("__no_rend_env", noRendSC.Env())
	bridge.stateMu.Unlock()

	out, err := bridge.Execute(`return tostring(__no_rend_env.called) .. ":" .. tostring(global_render_count)`)
	if err != nil {
		t.Fatalf("failed reading counts: %v", err)
	}
	if out != "0:1" {
		t.Errorf("expected no_rend=0 and global_render_count=1 (stopped after despawn), got %s", out)
	}

	if emptySC.LastError() != nil {
		t.Errorf("expected nil LastError for missing render hook, got %v", emptySC.LastError())
	}
	if errSC.LastError() == nil || !strings.Contains(errSC.LastError().Error(), "intentional render boom") {
		t.Errorf("expected captured render error on broken.lua, got %v", errSC.LastError())
	}
	if !strings.HasPrefix(recordedErr, "broken_render:") {
		t.Errorf("expected OnScriptError hook name 'broken_render', got %q", recordedErr)
	}
}
