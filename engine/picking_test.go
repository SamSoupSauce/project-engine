package engine

import (
	"math"
	"testing"
)

func TestScreenPointToRayAndSceneObjectPicking(t *testing.T) {
	cam := Camera3D{
		Position:   NewVector3(0, 0, 5),
		Target:     NewVector3(0, 0, 0),
		Up:         NewVector3(0, 1, 0),
		Fovy:       60,
		Projection: CameraPerspective,
	}

	centerRay := ScreenPointToRay(cam, 640, 360, 1280, 720)
	if math.Abs(float64(centerRay.Direction.Z+1.0)) > 1e-3 {
		t.Fatalf("expected center ray along -Z, got %+v", centerRay.Direction)
	}

	scene := NewBaseScene("pick_scene")
	cube := NewGameObject("cube_1", "RainbowCube")
	cube.EnsureTransform().SetPosition(NewVector3(0, 0, 0))
	scene.AddObject(cube)

	farObj := NewGameObject("far_cube", "FarCube")
	farObj.EnsureTransform().SetPosition(NewVector3(0, 0, -6))
	scene.AddObject(farObj)

	// Pure 2D HUD object at origin must not block 3D picking.
	hud := NewGameObject("hud_overlay", "HUD")
	hud.EnsureTransform().SetPosition(NewVector3(0, 0, 2))
	hud.AddComponent(NewRenderable2D(0, 100, 20))
	scene.AddObject(hud)

	res := PickSceneObject(scene, centerRay, nil)
	if !res.Hit || res.Object != cube {
		t.Fatalf("expected closest 3D cube picked (ignoring HUD), got %+v", res)
	}

	// Far off-axis click should miss all scene objects.
	missRay := ScreenPointToRay(cam, 10, 10, 1280, 720)
	miss := PickSceneObject(scene, missRay, nil)
	if miss.Hit {
		t.Fatalf("expected corner ray to miss centered cubes, got %+v", miss)
	}
}

type mockPickerBackend struct {
	lines    []DebugConsoleLine
	open     bool
	pickMode bool
	pickedID string
}

func (m *mockPickerBackend) ExecuteCommand(string) string          { return "" }
func (m *mockPickerBackend) GetOutputLines() []DebugConsoleLine    { return m.lines }
func (m *mockPickerBackend) ClearOutput()                          { m.lines = nil }
func (m *mockPickerBackend) IsConsoleOpen() bool                   { return m.open }
func (m *mockPickerBackend) SetConsoleOpen(v bool)                 { m.open = v }
func (m *mockPickerBackend) ToggleConsole()                        { m.open = !m.open }
func (m *mockPickerBackend) AppendLog(t string, k DebugConsoleLineKind) {
	m.lines = append(m.lines, DebugConsoleLine{Text: t, Kind: k})
}
func (m *mockPickerBackend) GetHistory() []string { return nil }
func (m *mockPickerBackend) IsPickMode() bool     { return m.pickMode }
func (m *mockPickerBackend) SetPickMode(v bool)   { m.pickMode = v }
func (m *mockPickerBackend) GetPickedSummary() (string, string, bool) {
	if m.pickedID == "" {
		return "", "", false
	}
	return m.pickedID, "Cube", true
}
func (m *mockPickerBackend) ClearPicked() { m.pickedID = "" }

func TestDebugConsoleWindowRendersPickerUIHeadless(t *testing.T) {
	overlay := NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()

	backend := &mockPickerBackend{open: true, pickMode: true, pickedID: "cube_1"}
	overlay.SetConsole(backend)

	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()
}
