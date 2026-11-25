package luabridge

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/SamSoupSauce/project-engine/engine"
)

func TestImGuiDebugConsoleLuaBridgeIntegration(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	console := NewConsole(bridge)
	overlay := engine.NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()

	overlay.SetConsole(console)

	var leakedKeys int
	overlay.OnGameKey = func(_ glfw.Key, _ int, _ glfw.Action, _ glfw.ModifierKey) {
		leakedKeys++
	}

	if overlay.Console.IsOpen() {
		t.Fatalf("expected console to start closed")
	}

	// Toggle open with backtick/tilde
	if !overlay.HandleKey(glfw.KeyGraveAccent, 0, glfw.Press, 0) {
		t.Fatalf("expected KeyGraveAccent to toggle console")
	}
	if !overlay.Console.IsOpen() || !console.IsOpen {
		t.Fatalf("expected console to be open")
	}
	if leakedKeys != 0 {
		t.Fatalf("expected toggle key not to leak to game key handler")
	}

	// Execute Lua command via ImGui DebugConsoleWindow -> Console -> LuaBridge
	res := overlay.Console.Submit("return 6 * 7")
	if res != "42" {
		t.Fatalf("expected Lua result '42', got %q", res)
	}

	lines := console.GetOutputLines()
	if len(lines) < 3 {
		t.Fatalf("expected system, input, and output lines in console, got %d", len(lines))
	}
	last := lines[len(lines)-1]
	if last.Text != "42" || last.Kind != engine.ConsoleLineOutput {
		t.Errorf("unexpected last console line: %+v", last)
	}

	// Draw & render the open console window headlessly
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	// Toggle closed
	overlay.HandleKey(glfw.KeyGraveAccent, 0, glfw.Press, 0)
	if overlay.Console.IsOpen() {
		t.Fatalf("expected console to close on second toggle key press")
	}
}
