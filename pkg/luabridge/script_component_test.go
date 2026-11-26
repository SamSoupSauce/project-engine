package luabridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func writeTempLuaScript(t *testing.T, filename, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp lua script %s: %v", path, err)
	}
	return path
}

func TestScriptComponentAllSixLifecycleHooks(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	var scriptErrors []string
	bridge.OnScriptError = func(path, hook string, err error) {
		scriptErrors = append(scriptErrors, hook+": "+err.Error())
	}

	scriptSrc := `
log = {}
local_ticks = {}
before_despawn_found = false

function load(scene)
	table.insert(log, "load:" .. scene:GetName() .. ":" .. tostring(scene:IsLoaded()) .. ":" .. tostring(scene:ObjectCount()))
end

function unload(scene)
	table.insert(log, "unload:" .. scene:GetName())
end

function spawn(self, sender)
	local senderKind = "unknown"
	if sender == self:GetScene() then
		senderKind = "scene:" .. sender:GetName()
	elseif sender ~= nil then
		senderKind = "go:" .. sender:GetID()
	end
	table.insert(log, "spawn:" .. self:GetID() .. ":" .. senderKind)
end

function event(name, self, sender, data)
	local payload = "nil"
	if data ~= nil then
		payload = tostring(data.msg) .. "#" .. tostring(data.count)
	end
	table.insert(log, "event:" .. name .. ":" .. self:GetID() .. ":" .. sender:GetID() .. ":" .. payload)
end

function tick(self, dt)
	local isInt = (math.floor(dt) == dt)
	table.insert(local_ticks, dt)
	table.insert(log, "tick:" .. self:GetID() .. ":" .. tostring(dt) .. ":" .. tostring(isInt))
end

function despawn(self, sender, data)
	local sc = self:GetScene()
	before_despawn_found = (sc ~= nil and sc:FindByID(self:GetID()) == self and self:IsActive())
	local senderID = "nil"
	if sender ~= nil then
		senderID = sender:GetID()
	end
	local reason = "nil"
	if data ~= nil then
		reason = tostring(data.reason)
	end
	table.insert(log, "despawn:" .. self:GetID() .. ":" .. senderID .. ":" .. reason)
end
`
	scriptPath := writeTempLuaScript(t, "full_lifecycle.lua", scriptSrc)

	scene := engine.NewBaseScene("MainArena")
	scene.TickIntervalMs = 16

	hero := engine.NewGameObject("hero_1", "Hero")
	hero.EnsureTransform().SetPosition(engine.NewVector3(1, 2, 3))
	heroScript, err := bridge.NewScriptComponent(scriptPath)
	if err != nil {
		t.Fatalf("failed to load script component: %v", err)
	}
	hero.AddComponent(heroScript)
	scene.AddObject(hero)

	// 1. Scene Init -> triggers load(scene) then spawn(self, scene)
	scene.Init()

	// 2. Manual runtime spawn -> triggers spawn(self, senderGameObject)
	minion := engine.NewGameObject("minion_1", "Minion")
	minionScript, err := bridge.NewScriptComponent(scriptPath)
	if err != nil {
		t.Fatalf("failed to load minion script: %v", err)
	}
	minion.AddComponent(minionScript)
	scene.Spawn(minion, hero)

	// 3. Emit event from Lua on hero -> both hero and minion receive event(name, self, sender, data)
	if _, err := bridge.Execute(""); err != nil {
		t.Fatalf("unexpected bridge execute error: %v", err)
	}
	hero.Emit("battle_cry", map[string]any{"msg": "charge", "count": 3})
	// Emit with nil data
	minion.Emit("ping", nil)

	// 4. Simulation steps -> tick(self, dt) with integer ms intervals (32ms -> 2 ticks of 16ms)
	scene.Update(0.032)

	// 5. Despawn minion -> despawn(self, sender, data) fires before removal/cleanup
	scene.Despawn(minion, hero, map[string]any{"reason": "expired"})

	if minion.Active || minion.GetScene() != nil || scene.FindByID("minion_1") != nil {
		t.Fatalf("expected minion to be deactivated and removed after despawn")
	}

	// Subsequent ticks must only hit hero, not despawned minion
	scene.Update(0.016)

	// 6. Unload scene -> unload(scene) fires on remaining hero
	scene.Unload()

	if len(scriptErrors) > 0 {
		t.Fatalf("unexpected Lua script errors: %v", scriptErrors)
	}

	// Inspect hero isolated log via Lua
	bridge.stateMu.Lock()
	bridge.L.SetGlobal("__hero_env", heroScript.Env())
	bridge.L.SetGlobal("__minion_env", minionScript.Env())
	bridge.stateMu.Unlock()

	out, err := bridge.Execute(`return table.concat(__hero_env.log, "|")`)
	if err != nil {
		t.Fatalf("failed reading hero log: %v", err)
	}
	expectedHero := "load:MainArena:true:1|spawn:hero_1:scene:MainArena|event:battle_cry:hero_1:hero_1:charge#3|event:ping:hero_1:minion_1:nil|tick:hero_1:16:true|tick:hero_1:16:true|tick:hero_1:16:true|unload:MainArena"
	if out != expectedHero {
		t.Errorf("hero log mismatch:\n got: %s\nwant: %s", out, expectedHero)
	}

	minionOut, err := bridge.Execute(`return table.concat(__minion_env.log, "|") .. "::found=" .. tostring(__minion_env.before_despawn_found)`)
	if err != nil {
		t.Fatalf("failed reading minion log: %v", err)
	}
	expectedMinion := "spawn:minion_1:go:hero_1|event:battle_cry:minion_1:hero_1:charge#3|event:ping:minion_1:minion_1:nil|tick:minion_1:16:true|tick:minion_1:16:true|despawn:minion_1:hero_1:expired::found=true"
	if minionOut != expectedMinion {
		t.Errorf("minion log mismatch:\n got: %s\nwant: %s", minionOut, expectedMinion)
	}
}

func TestScriptEnvironmentIsolationAndSharedBindings(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	// Two entities share the exact same Lua script file using global variables,
	// yet maintain completely isolated state while accessing shared engine bindings.
	scriptPath := writeTempLuaScript(t, "isolated.lua", `
counter = 0
last_pos = nil

function spawn(self, sender)
	counter = counter + 1
	-- Access shared engine binding registered in _G
	last_pos = get_entity_position(self:GetName())
end

function tick(self, dt)
	counter = counter + 10
end
`)

	scene := engine.NewBaseScene("IsolationScene")
	scene.TickIntervalMs = 10

	objA := engine.NewGameObject("a", "Alpha")
	objA.EnsureTransform().SetPosition(engine.NewVector3(5, 0, 0))
	bridge.RegisterGameObject("Alpha", objA)

	objB := engine.NewGameObject("b", "Beta")
	objB.EnsureTransform().SetPosition(engine.NewVector3(99, 0, 0))
	bridge.RegisterGameObject("Beta", objB)

	scA, err := NewScriptComponent(bridge, scriptPath)
	if err != nil {
		t.Fatalf("load scA failed: %v", err)
	}
	scB, err := NewScriptComponent(bridge, scriptPath)
	if err != nil {
		t.Fatalf("load scB failed: %v", err)
	}

	objA.AddComponent(scA)
	objB.AddComponent(scB)
	scene.AddObject(objA)
	scene.Init()

	// Tick objA (plus objB is spawned later so objA gets 2 ticks, objB gets 1 tick)
	scene.Tick(10)
	scene.AddObject(objB)
	scene.Tick(10)

	bridge.stateMu.Lock()
	bridge.L.SetGlobal("__envA", scA.Env())
	bridge.L.SetGlobal("__envB", scB.Env())
	bridge.stateMu.Unlock()

	res, err := bridge.Execute(`return string.format("A=%d(%.0f) B=%d(%.0f) G=%s", __envA.counter, __envA.last_pos.x, __envB.counter, __envB.last_pos.x, tostring(counter))`)
	if err != nil {
		t.Fatalf("failed inspecting isolated envs: %v", err)
	}
	// A: spawn(+1) + tick(+10) + tick(+10) = 21, pos.x = 5
	// B: spawn(+1) + tick(+10) = 11, pos.x = 99
	// _G.counter must remain nil (not polluted by either script)
	if res != "A=21(5) B=11(99) G=nil" {
		t.Errorf("isolation check failed: got %q", res)
	}
}

func TestLuaUserdataEmitSpawnDespawnAndQueries(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	emitterScriptPath := writeTempLuaScript(t, "emitter.lua", `
received = {}

function spawn(self, sender)
	local tc = self:AddComponent("Transform", 4, 5, 6)
	self.active = true
	-- Emit directly from Lua with payload table
	self:Emit("lua_hello", { sender_name = self:GetName(), power = 42, tags = {"fire", "aoe"} })
end

function event(name, self, sender, data)
	if name == "lua_hello" then
		local pos = self:GetComponent("Transform"):GetLocation()
		table.insert(received, string.format("%s:%s->%s:%.0f:%s:%s",
			name, sender:GetID(), self:GetID(), data.power, data.tags[1], tostring(pos.x)))
	end
end
`)

	scene := engine.NewBaseScene("UserdataScene")
	obj := engine.NewGameObject("e1", "Emitter")
	sc, err := bridge.NewScriptComponent(emitterScriptPath)
	if err != nil {
		t.Fatalf("failed loading emitter script: %v", err)
	}
	obj.AddComponent(sc)
	scene.AddObject(obj)
	scene.Init()

	if pos := obj.GetWorldTransform().Position; pos.X != 4 || pos.Y != 5 || pos.Z != 6 {
		t.Errorf("expected position (4,5,6) from Lua Transform component, got %+v", pos)
	}

	bridge.stateMu.Lock()
	bridge.L.SetGlobal("__emit_env", sc.Env())
	bridge.stateMu.Unlock()

	out, err := bridge.Execute(`return table.concat(__emit_env.received, ",")`)
	if err != nil {
		t.Fatalf("failed reading received events: %v", err)
	}
	if out != "lua_hello:e1->e1:42:fire:4" {
		t.Errorf("unexpected Lua event output: %q", out)
	}

	// Verify Scene userdata queries from Lua
	bridge.stateMu.Lock()
	pushScene(bridge.L, scene)
	bridge.L.SetGlobal("__test_scene", bridge.L.Get(-1))
	bridge.L.Pop(1)
	bridge.stateMu.Unlock()

	queryOut, err := bridge.Execute(`
		local found = __test_scene:FindByName("Emitter")
		local list = __test_scene:GetObjects()
		return found:GetID() .. ":" .. tostring(#list) .. ":" .. tostring(__test_scene:ObjectCount())
	`)
	if err != nil {
		t.Fatalf("scene query from Lua failed: %v", err)
	}
	if queryOut != "e1:1:1" {
		t.Errorf("unexpected scene query result: %q", queryOut)
	}
}

func TestScriptMissingHooksGracefulNoOp(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	// Empty script (no hooks defined at all)
	emptyPath := writeTempLuaScript(t, "empty.lua", `-- no hooks defined`)
	// Partial script (only tick defined; non-function global named 'event' must also safely no-op)
	partialPath := writeTempLuaScript(t, "partial.lua", `
event = "not_a_function"
ticks = 0
function tick(self, dt)
	ticks = ticks + 1
end
`)

	scene := engine.NewBaseScene("GracefulScene")
	scene.TickIntervalMs = 16

	objEmpty := engine.NewGameObject("empty_1", "Empty")
	scEmpty, err := bridge.NewScriptComponent(emptyPath)
	if err != nil {
		t.Fatalf("failed loading empty script: %v", err)
	}
	objEmpty.AddComponent(scEmpty)

	objPartial := engine.NewGameObject("partial_1", "Partial")
	scPartial, err := bridge.NewScriptComponent(partialPath)
	if err != nil {
		t.Fatalf("failed loading partial script: %v", err)
	}
	objPartial.AddComponent(scPartial)

	scene.AddObject(objEmpty)
	scene.AddObject(objPartial)

	scene.Init()
	scene.Update(0.032)
	objPartial.Emit("ignored", map[string]any{"ok": true})
	scene.Despawn(objEmpty, objPartial, nil)
	scene.Unload()

	if scEmpty.LastError() != nil {
		t.Errorf("expected nil error on empty script, got %v", scEmpty.LastError())
	}
	if scPartial.LastError() != nil {
		t.Errorf("expected nil error on partial script, got %v", scPartial.LastError())
	}

	bridge.stateMu.Lock()
	bridge.L.SetGlobal("__partial_env", scPartial.Env())
	bridge.stateMu.Unlock()

	ticksOut, err := bridge.Execute(`return tostring(__partial_env.ticks)`)
	if err != nil {
		t.Fatalf("failed reading partial ticks: %v", err)
	}
	if ticksOut != "2" {
		t.Errorf("expected 2 ticks on partial script, got %s", ticksOut)
	}
}
