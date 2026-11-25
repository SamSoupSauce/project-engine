package luabridge

import (
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestLuaBridgeCommandRegistration(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	commandRun := false
	bridge.RegisterCommand("ping", func(args []string) (string, error) {
		commandRun = true
		return "pong " + strings.Join(args, ","), nil
	})

	out, err := bridge.Execute("ping a b c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !commandRun {
		t.Errorf("registered command was not executed")
	}
	if out != "pong a,b,c" {
		t.Errorf("expected 'pong a,b,c', got '%s'", out)
	}
}

func TestLuaBridgeInputHook(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	hookCalled := false
	bridge.AddInputHook(func(input string) (bool, string, error) {
		if strings.HasPrefix(input, "!") {
			hookCalled = true
			return true, "HOOKED: " + input[1:], nil
		}
		return false, "", nil
	})

	out, err := bridge.Execute("!special_action")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hookCalled {
		t.Errorf("input hook was not called")
	}
	if out != "HOOKED: special_action" {
		t.Errorf("expected 'HOOKED: special_action', got '%s'", out)
	}

	// Normal input should bypass the hook
	res, err := bridge.Execute("return 40 + 2")
	if err != nil {
		t.Fatalf("unexpected error on Lua: %v", err)
	}
	if res != "42" {
		t.Errorf("expected '42', got '%s'", res)
	}
}

func TestLuaBridgeGameObjectBinding(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	obj := engine.NewGameObject("player_1", "Player")
	obj.Transform.Position = engine.NewVector3(12.5, 3.0, -8.0)
	obj.Active = true

	bridge.RegisterGameObject("Player", obj)

	// Query position via Lua script
	luaScript := `
		local pos = get_entity_position("Player")
		if pos == nil then return "nil" end
		return string.format("pos:%.1f,%.1f,%.1f", pos.x, pos.y, pos.z)
	`
	out, err := bridge.Execute(luaScript)
	if err != nil {
		t.Fatalf("Lua execution failed: %v", err)
	}
	if out != "pos:12.5,3.0,-8.0" {
		t.Errorf("expected 'pos:12.5,3.0,-8.0', got '%s'", out)
	}

	// Mutate active state via Lua script
	_, err = bridge.Execute("set_entity_active('Player', false)")
	if err != nil {
		t.Fatalf("set_entity_active failed: %v", err)
	}
	if obj.Active {
		t.Errorf("expected GameObject.Active to be false after Lua call")
	}
}

func TestConsoleInteraction(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()

	console := NewConsole(bridge)
	if console.IsOpen {
		t.Errorf("console should start closed")
	}
	console.Toggle()
	if !console.IsOpen {
		t.Errorf("console should be open after toggle")
	}

	console.InputBuffer = []rune("return 'hello console'")
	res := console.SubmitInput()
	if res != "hello console" {
		t.Errorf("expected 'hello console', got '%s'", res)
	}
	if len(console.History) != 1 {
		t.Errorf("expected history length 1, got %d", len(console.History))
	}
}
