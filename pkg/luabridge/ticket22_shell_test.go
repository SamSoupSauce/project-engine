package luabridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTicket22ShellScriptsLifecycleAndLuaRegistration(t *testing.T) {
	scriptsDir := t.TempDir()
	toolsPath := filepath.Join(scriptsDir, "test_tools.lua")
	initialScript := `local M = {}

function M.load(scene)
  _G.test_tools_loaded_count = (_G.test_tools_loaded_count or 0) + 1
end

function M.unload(scene)
  _G.test_tools_unloaded_count = (_G.test_tools_unloaded_count or 0) + 1
end

shell.register("spawn_box", {
  description = "Spawns a physics-active test crate at target coordinates",
  usage = "spawn_box <x> <y> <z> [mass]",
  examples = { "spawn_box 0 5 0 2.5" },
  handler = function(ctx, args)
    if #args < 3 then
      return false, "Error: Missing x y z coordinates"
    end
    local x, y, z = tonumber(args[1]), tonumber(args[2]), tonumber(args[3])
    local mass = tonumber(args[4]) or 1.0
    if not x or not y or not z then
      return false, "Error: Invalid numeric coordinates"
    end
    return true, string.format("sender=%s pos=(%.1f,%.1f,%.1f) mass=%.1f", ctx.sender, x, y, z, mass)
  end
})

return M
`
	if err := os.WriteFile(toolsPath, []byte(initialScript), 0o644); err != nil {
		t.Fatalf("write test_tools.lua failed: %v", err)
	}

	bridge := NewLuaBridge()
	defer bridge.Close()
	bridge.ScriptEditor().SetScriptsDir(scriptsDir)
	console := NewConsole(bridge)

	// 1. scripts.list shows available script before run
	lsBefore := console.ExecuteCommand("scripts.list")
	if !strings.Contains(lsBefore, "[available] test_tools") {
		t.Fatalf("expected [available] test_tools in scripts.list, got:\n%s", lsBefore)
	}

	// 2. scripts.run loads script, runs M.load(), and binds spawn_box
	runOut := console.ExecuteCommand("scripts.run test_tools.lua")
	if !strings.Contains(runOut, "spawn_box") {
		t.Fatalf("expected scripts.run to mention bound spawn_box command, got %q", runOut)
	}
	if loaded := console.ExecuteCommand("return _G.test_tools_loaded_count"); loaded != "1" {
		t.Fatalf("expected M.load() to run once, got %q", loaded)
	}

	// scripts.list now shows [LOADED]
	lsAfter := console.ExecuteCommand("scripts.list")
	if !strings.Contains(lsAfter, "[LOADED] test_tools") {
		t.Fatalf("expected [LOADED] test_tools in scripts.list, got:\n%s", lsAfter)
	}

	// 3. help and help spawn_box reflect dynamic Lua command
	helpAll := console.ExecuteCommand("help")
	if !strings.Contains(helpAll, "spawn_box") || !strings.Contains(helpAll, "scripts.list") {
		t.Fatalf("expected help output to include spawn_box and scripts.list, got:\n%s", helpAll)
	}
	helpCmd := console.ExecuteCommand("help spawn_box")
	if !strings.Contains(helpCmd, "spawn_box <x> <y> <z> [mass]") {
		t.Fatalf("expected usage in help spawn_box, got:\n%s", helpCmd)
	}

	// 4. Invoke spawn_box with valid args and invalid args
	okOut := console.ExecuteCommand(`spawn_box 1 2 3 "4.5"`)
	if okOut != "sender=user pos=(1.0,2.0,3.0) mass=4.5" {
		t.Fatalf("unexpected spawn_box output: %q", okOut)
	}
	errOut := console.ExecuteCommand(`spawn_box 1`)
	if !strings.Contains(errOut, "Error: Missing x y z coordinates") {
		t.Fatalf("expected structured error on missing args, got %q", errOut)
	}

	// 5. Modify script on disk and run scripts.reload -> re-runs unload + load and updates command
	updatedScript := strings.ReplaceAll(initialScript, "mass=%.1f", "reloaded_mass=%.1f")
	if err := os.WriteFile(toolsPath, []byte(updatedScript), 0o644); err != nil {
		t.Fatalf("update test_tools.lua failed: %v", err)
	}
	if reloadOut := console.ExecuteCommand("scripts.reload test_tools"); strings.Contains(reloadOut, "Error:") {
		t.Fatalf("scripts.reload failed: %s", reloadOut)
	}
	if loaded := console.ExecuteCommand("return _G.test_tools_loaded_count"); loaded != "2" {
		t.Fatalf("expected load count 2 after reload, got %q", loaded)
	}
	if unloaded := console.ExecuteCommand("return _G.test_tools_unloaded_count"); unloaded != "1" {
		t.Fatalf("expected unload count 1 after reload, got %q", unloaded)
	}
	if relExec := console.ExecuteCommand("spawn_box 0 5 0 2"); !strings.Contains(relExec, "reloaded_mass=2.0") {
		t.Fatalf("expected reloaded handler output, got %q", relExec)
	}

	// 6. scripts.unload invokes unload hook and automatically unbinds spawn_box
	unloadOut := console.ExecuteCommand("scripts.unload test_tools")
	if !strings.Contains(unloadOut, "unbound 1 command(s)") {
		t.Fatalf("expected unbound 1 command on scripts.unload, got %q", unloadOut)
	}
	if unloaded := console.ExecuteCommand("return _G.test_tools_unloaded_count"); unloaded != "2" {
		t.Fatalf("expected unload count 2 after scripts.unload, got %q", unloaded)
	}
	if _, exists := bridge.Shell().Get("spawn_box"); exists {
		t.Fatalf("expected spawn_box removed from ShellRegistry after unload")
	}
}

func TestTicket22UnifiedPipelineAndSenderMirroring(t *testing.T) {
	srv, scene, bridge, console := setupTestAgentServer(t, "pipe-tok")

	bridge.Shell().RegisterNative(ShellCommandDef{
		Name:        "who_am_i",
		Description: "Reports ExecutionContext.Sender and args",
		Usage:       "who_am_i [arg]",
		GoHandler: func(ctx ExecutionContext) (string, error) {
			return ctx.Sender + "|" + strings.Join(ctx.Args[1:], ","), nil
		},
	})

	stopTick := make(chan struct{})
	go func() {
		tick := time.NewTicker(4 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				scene.Tick(16)
			case <-stopTick:
				return
			}
		}
	}()
	defer close(stopTick)

	// 1. JSON-RPC with explicit client_id="agy" mirrors `[agent:agy] who_am_i "hello world"` into Console
	rpcURL := "http://" + srv.BoundAddress() + "/rpc"
	hdr := map[string]string{
		"X-Engine-Token": "pipe-tok",
		"X-Client-ID":    "agy",
	}
	_, rpcResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "pipe-1",
		"method":  "engine.console_exec",
		"params": map[string]any{
			"cmd": `who_am_i "hello world" 42`,
		},
	})
	if rpcResp.Error != nil || rpcResp.Result == nil {
		t.Fatalf("RPC console_exec failed: %+v", rpcResp.Error)
	}
	dataMap := rpcResp.Result.Data.(map[string]any)
	if dataMap["output"] != "agent:agy|hello world,42" {
		t.Fatalf("unexpected RPC output: %v", dataMap["output"])
	}

	// Verify Console output scrollback contains `[agent:agy] who_am_i "hello world" 42` and output
	lines := console.GetOutputLines()
	sawMirroredCmd := false
	sawMirroredOut := false
	for _, l := range lines {
		if strings.Contains(l.Text, `[agent:agy] who_am_i "hello world" 42`) {
			sawMirroredCmd = true
		}
		if strings.Contains(l.Text, `agent:agy|hello world,42`) {
			sawMirroredOut = true
		}
	}
	if !sawMirroredCmd || !sawMirroredOut {
		t.Fatalf("expected Console to mirror [agent:agy] cmd and output, got lines=%+v", lines)
	}

	// 2. WebSocket `op: "shell"` with ?client_id=gemini routes through same funnel
	wsURL := "ws://" + srv.BoundAddress() + "/ws?token=pipe-tok&client_id=gemini"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"id":  9,
		"op":  "shell",
		"cmd": "who_am_i fast",
	}); err != nil {
		t.Fatalf("WS WriteJSON failed: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	gotWSReply := false
	for !gotWSReply {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("WS ReadMessage failed: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if m["id"] != nil {
			if m["ok"] != true || m["res"] != "agent:gemini|fast" {
				t.Fatalf("unexpected WS shell reply: %+v", m)
			}
			gotWSReply = true
		}
	}
}
