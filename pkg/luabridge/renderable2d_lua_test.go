package luabridge

import (
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestLuaRenderable2DMutations(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	scene := engine.NewBaseScene("HUDLuaScene")

	healthBarObj := engine.NewGameObject("hp_bar", "HealthBar")
	healthR2D := engine.NewRenderable2D(1, 200, 20)
	healthR2D.ZIndex = 10
	healthBarObj.AddComponent(healthR2D)
	bridge.RegisterGameObject("HealthBar", healthBarObj)

	crosshairObj := engine.NewGameObject("crosshair", "Crosshair")
	crosshairR2D := engine.NewRenderable2D(2, 16, 16)
	crosshairR2D.Anchor = engine.AnchorCenter
	crosshairR2D.ZIndex = 50
	crosshairObj.AddComponent(crosshairR2D)

	scriptPath := writeTempLuaScript(t, "hud_controller.lua", `
function spawn(self, sender)
	local r = self:GetRenderable2D()
	r:SetVisible(false)
	r:SetColor(0, 255, 128, 200)
	r:SetUV(0.25, 0.25, 0.5, 0.5)
end

function event(name, self, sender, data)
	if name == "toggle_crosshair" then
		local r = self:GetRenderable2D()
		r:SetVisible(not r:IsVisible())
	end
end
`)
	sc, err := bridge.NewScriptComponent(scriptPath)
	if err != nil {
		t.Fatalf("failed loading hud script: %v", err)
	}
	crosshairObj.AddComponent(sc)

	scene.AddObject(healthBarObj)
	scene.AddObject(crosshairObj)
	scene.Init()
	defer scene.Unload()

	// Crosshair script hid itself on spawn and updated color/UV.
	if crosshairR2D.IsVisible() {
		t.Errorf("expected crosshair to be hidden after Lua spawn")
	}
	if crosshairR2D.Color != engine.NewColor(0, 255, 128, 200) {
		t.Errorf("unexpected crosshair color: %+v", crosshairR2D.Color)
	}
	if crosshairR2D.SourceRect != (engine.Rect2D{U: 0.25, V: 0.25, W: 0.5, H: 0.5}) {
		t.Errorf("unexpected crosshair UV: %+v", crosshairR2D.SourceRect)
	}

	// Mutate registered HealthBar via global helpers in LuaBridge.
	if _, err := bridge.Execute(`set_ui_size("HealthBar", 120, 20); set_ui_color("HealthBar", 220, 40, 40, 255)`); err != nil {
		t.Fatalf("Lua global UI mutation failed: %v", err)
	}
	if healthR2D.Size.X != 120 || healthR2D.Color.R != 220 {
		t.Errorf("expected health bar width=120 R=220, got size=%+v color=%+v", healthR2D.Size, healthR2D.Color)
	}

	// Toggle crosshair back on via event bus.
	healthBarObj.Emit("toggle_crosshair", nil)
	if !crosshairR2D.IsVisible() {
		t.Errorf("expected crosshair to become visible after toggle_crosshair event")
	}

	cmds := engine.Extract2DFromHierarchy(scene.GetRoot(), 1280, 720)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 extracted 2D HUD commands, got %d", len(cmds))
	}
	if cmds[0].Width != 120 || cmds[1].ZIndex != 50 {
		t.Errorf("unexpected extracted commands: %+v", cmds)
	}
}
