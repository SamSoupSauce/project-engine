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

func TestTicket23DynamicDocsRegistryAndTransports(t *testing.T) {
	scriptsDir := t.TempDir()
	toolsPath := filepath.Join(scriptsDir, "test_tools.lua")
	scriptSrc := `shell.register("spawn_box", {
  description = "Spawns a physics-active test crate at target coordinates",
  usage = "spawn_box <x> <y> <z> [mass]",
  examples = { "spawn_box 0 5 0 2.5" },
  handler = function(ctx, args)
    return true, "ok"
  end
})
`
	if err := os.WriteFile(toolsPath, []byte(scriptSrc), 0o644); err != nil {
		t.Fatalf("write test_tools.lua failed: %v", err)
	}

	srv, scene, bridge, console := setupTestAgentServer(t, "docs-tok")
	bridge.ScriptEditor().SetScriptsDir(scriptsDir)

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

	// 1. `docs` with no args lists subsystems, native commands, components, and hooks.
	idxOut := console.ExecuteCommand("docs")
	for _, want := range []string{"jolt", "shell.register", "prefab.format", "scripts.list", "Transform", "tick(self, dtMs)"} {
		if !strings.Contains(idxOut, want) {
			t.Fatalf("expected docs index to contain %q, got:\n%s", want, idxOut)
		}
	}

	// 2. Built-in subsystem topic queries (`docs jolt`, `docs shell.register`, `docs prefab.format`, `docs hooks`)
	for _, topic := range []string{"jolt", "shell.register", "prefab.format", "hooks", "components"} {
		out := console.ExecuteCommand("docs " + topic)
		if strings.Contains(out, "Error:") || !strings.Contains(strings.ToLower(out), strings.ToLower(topic)) {
			t.Fatalf("expected valid docs for %q, got:\n%s", topic, out)
		}
	}

	// 3. Dynamically loading script makes `spawn_box` immediately visible in `docs` and `engine.query_docs`.
	if runOut := console.ExecuteCommand("scripts.run test_tools.lua"); strings.Contains(runOut, "Error:") {
		t.Fatalf("scripts.run failed: %s", runOut)
	}
	dynIdx := console.ExecuteCommand("docs")
	if !strings.Contains(dynIdx, "spawn_box") {
		t.Fatalf("expected dynamic Lua command spawn_box in docs index, got:\n%s", dynIdx)
	}

	rpcURL := "http://" + srv.BoundAddress() + "/rpc"
	hdr := map[string]string{
		"X-Engine-Token": "docs-tok",
		"X-Client-ID":    "gemini",
	}

	// JSON-RPC `engine.query_docs` for specific Lua command
	_, docResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "agy-docs-1",
		"method":  "engine.query_docs",
		"params":  map[string]any{"topic": "spawn_box"},
	})
	if docResp.Error != nil || docResp.Result == nil {
		t.Fatalf("engine.query_docs failed: %+v", docResp.Error)
	}
	dataMap := docResp.Result.Data.(map[string]any)
	if dataMap["topic"] != "spawn_box" ||
		dataMap["type"] != "lua_command" ||
		dataMap["usage"] != "spawn_box <x> <y> <z> [mass]" ||
		!strings.Contains(dataMap["source"].(string), "test_tools.lua") {
		t.Fatalf("unexpected engine.query_docs payload: %+v", dataMap)
	}

	// JSON-RPC `engine.query_docs` for index
	_, idxResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "agy-docs-idx",
		"method":  "engine.query_docs",
		"params":  map[string]any{},
	})
	if idxResp.Error != nil || idxResp.Result == nil {
		t.Fatalf("engine.query_docs index failed: %+v", idxResp.Error)
	}
	idxMap := idxResp.Result.Data.(map[string]any)
	if idxMap["topic"] != "index" || idxMap["type"] != "index" {
		t.Fatalf("unexpected index payload: %+v", idxMap)
	}

	// 4. WebSocket `op: "shell", cmd: "docs"` mirrors `[agent:gemini] docs` into Console scrollback.
	wsURL := "ws://" + srv.BoundAddress() + "/ws?token=docs-tok&client_id=gemini"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"id":  11,
		"op":  "shell",
		"cmd": "docs",
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
			if m["ok"] != true {
				t.Fatalf("expected ok=true for WS docs, got %+v", m)
			}
			gotWSReply = true
		}
	}

	sawMirrored := false
	for _, l := range console.GetOutputLines() {
		if strings.Contains(l.Text, "[agent:gemini] docs") {
			sawMirrored = true
			break
		}
	}
	if !sawMirrored {
		t.Fatalf("expected '[agent:gemini] docs' mirrored into Console scrollback")
	}

	// 5. Unloading script removes dynamic command from `docs`.
	if ulOut := console.ExecuteCommand("scripts.unload test_tools"); strings.Contains(ulOut, "Error:") {
		t.Fatalf("scripts.unload failed: %s", ulOut)
	}
	if afterUnload := console.ExecuteCommand("docs spawn_box"); !strings.Contains(afterUnload, "not found") {
		t.Fatalf("expected spawn_box docs removed after unload, got:\n%s", afterUnload)
	}
}
