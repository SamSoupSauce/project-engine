package luabridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/gorilla/websocket"
)

func setupTestAgentServer(t *testing.T, token string) (*AgentServer, *engine.BaseScene, *LuaBridge, *Console) {
	t.Helper()
	prefabsDir := filepath.Join(t.TempDir(), "prefabs")

	bridge := NewLuaBridge()
	t.Cleanup(bridge.Close)
	bridge.PrefabEditor().SetPrefabsDir(prefabsDir)

	scene := engine.NewBaseScene("AgentServerTestScene")
	bridge.BindScene(scene)

	cube := engine.NewGameObject("cube_1", "TestCube")
	cube.AddComponent(engine.NewTransformComponent())
	scene.AddObject(cube)
	bridge.RegisterGameObject("cube", cube)
	scene.Init()

	console := NewConsole(bridge)
	console.BindScene(scene)

	if _, err := bridge.PrefabEditor().SaveGameObjectAsPrefab(cube, "test_cube"); err != nil {
		t.Fatalf("save test prefab failed: %v", err)
	}

	srv := NewAgentServer(bridge, console, scene, AgentServerConfig{
		Address:    "127.0.0.1:0",
		Token:      token,
		PingPeriod: 40 * time.Millisecond,
		PongWait:   300 * time.Millisecond,
		WriteWait:  2 * time.Second,
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("AgentServer.Start failed: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return srv, scene, bridge, console
}

func postRPC(t *testing.T, url string, headers map[string]string, payload any) (*http.Response, rpcResponse) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal rpc payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header[http.CanonicalHeaderKey(k)] = []string{v}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	var decoded rpcResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode rpcResponse: %v", err)
		}
	}
	return resp, decoded
}

func TestTicket20AuthEnforcementEmptyAndNonEmptyTokens(t *testing.T) {
	t.Run("non_empty_token", func(t *testing.T) {
		srv, scene, _, _ := setupTestAgentServer(t, "top-secret")
		stopTick := make(chan struct{})
		go func() {
			tick := time.NewTicker(5 * time.Millisecond)
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

		rpcURL := "http://" + srv.BoundAddress() + "/rpc"
		wsBase := "ws://" + srv.BoundAddress() + "/ws"

		// Missing token -> 401
		r1, _ := postRPC(t, rpcURL, nil, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "engine.ping"})
		if r1.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing token, got %d", r1.StatusCode)
		}
		// Wrong token -> 401
		r2, _ := postRPC(t, rpcURL, map[string]string{"Authorization": "Bearer wrong"}, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "engine.ping"})
		if r2.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong token, got %d", r2.StatusCode)
		}
		// Valid Bearer token -> 200
		r3, dec3 := postRPC(t, rpcURL, map[string]string{"Authorization": "Bearer top-secret"}, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "engine.ping"})
		if r3.StatusCode != http.StatusOK || dec3.Result == nil || dec3.Result.Status != "success" {
			t.Fatalf("expected 200 success with Bearer token, got status=%d resp=%+v", r3.StatusCode, dec3)
		}
		// Valid X-Engine-Token -> 200
		r4, _ := postRPC(t, rpcURL, map[string]string{"X-Engine-Token": "top-secret"}, map[string]any{"jsonrpc": "2.0", "id": 4, "method": "engine.ping"})
		if r4.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 with X-Engine-Token, got %d", r4.StatusCode)
		}

		// WS without token -> 401
		_, wsResp, err := websocket.DefaultDialer.Dial(wsBase, nil)
		if err == nil || wsResp == nil || wsResp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected WS 401 without token, got resp=%v err=%v", wsResp, err)
		}
		// WS with ?token=top-secret -> 101
		conn, _, err := websocket.DefaultDialer.Dial(wsBase+"?token=top-secret", nil)
		if err != nil {
			t.Fatalf("expected WS dial success with token, got %v", err)
		}
		_ = conn.Close()
	})

	t.Run("empty_token_still_requires_field", func(t *testing.T) {
		srv, scene, _, _ := setupTestAgentServer(t, "")
		stopTick := make(chan struct{})
		go func() {
			tick := time.NewTicker(5 * time.Millisecond)
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

		rpcURL := "http://" + srv.BoundAddress() + "/rpc"
		wsBase := "ws://" + srv.BoundAddress() + "/ws"

		// Missing token field -> 401 even when configured token is ""
		r1, _ := postRPC(t, rpcURL, nil, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "engine.ping"})
		if r1.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 when token header omitted even with empty server token, got %d", r1.StatusCode)
		}

		// Explicit empty X-Engine-Token -> 200
		r2, dec2 := postRPC(t, rpcURL, map[string]string{"X-Engine-Token": ""}, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "engine.ping"})
		if r2.StatusCode != http.StatusOK || dec2.Result == nil {
			t.Fatalf("expected 200 with explicit empty X-Engine-Token, got status=%d", r2.StatusCode)
		}

		// Explicit empty Bearer header -> 200
		r3, _ := postRPC(t, rpcURL, map[string]string{"Authorization": "Bearer "}, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "engine.ping"})
		if r3.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 with explicit empty Bearer token, got %d", r3.StatusCode)
		}

		// WS missing ?token= -> 401; WS with ?token= -> 101
		_, wsResp, err := websocket.DefaultDialer.Dial(wsBase, nil)
		if err == nil || wsResp == nil || wsResp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected WS 401 when ?token omitted, got resp=%v err=%v", wsResp, err)
		}
		conn, _, err := websocket.DefaultDialer.Dial(wsBase+"?token=", nil)
		if err != nil {
			t.Fatalf("expected WS connect with ?token=, got %v", err)
		}
		_ = conn.Close()
	})
}

func TestTicket20JSONRPCMethodsAndFrameCapture(t *testing.T) {
	srv, scene, bridge, _ := setupTestAgentServer(t, "rpc-token")

	// Inject synthetic 2x2 bottom-up RGBA buffer for engine.capture_frame test.
	srv.FrameCapturer = func(format string) (FrameCaptureResult, error) {
		pixels := []byte{
			255, 0, 0, 255, 0, 255, 0, 255,
			0, 0, 255, 255, 255, 255, 255, 255,
		}
		b64, normFmt, err := engine.EncodeRGBABufferBase64(pixels, 2, 2, format)
		if err != nil {
			return FrameCaptureResult{}, err
		}
		return FrameCaptureResult{Format: normFmt, Width: 2, Height: 2, Data: b64}, nil
	}

	stopTick := make(chan struct{})
	go func() {
		tick := time.NewTicker(5 * time.Millisecond)
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

	rpcURL := "http://" + srv.BoundAddress() + "/rpc"
	hdr := map[string]string{"Authorization": "Bearer rpc-token"}

	// 1. engine.console_exec mutates scene on Tick() and returns captured logs.
	_, execResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-1",
		"method":  "engine.console_exec",
		"params": map[string]any{
			"cmd": `print("hello_rpc"); pick("cube"):GetComponent(Transform):SetPosition(4, 5, 6); return "moved"`,
		},
	})
	if execResp.Error != nil || execResp.Result == nil {
		t.Fatalf("console_exec failed: %+v", execResp.Error)
	}
	if !strings.Contains(strings.Join(execResp.Result.Logs, "\n"), "hello_rpc") {
		t.Fatalf("expected logs to include 'hello_rpc', got %v", execResp.Result.Logs)
	}
	if execResp.Result.Tick == 0 {
		t.Fatalf("expected non-zero tick in result")
	}
	pos := bridge.GetGameObject("cube").GetTransformComponent().GetPosition()
	if pos.X != 4 || pos.Y != 5 || pos.Z != 6 {
		t.Fatalf("expected cube moved to (4,5,6), got %+v", pos)
	}

	// Lua error produces JSON-RPC error with traceback.
	_, errResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-err",
		"method":  "engine.console_exec",
		"params":  map[string]any{"cmd": `error("boom_lua")`},
	})
	if errResp.Error == nil || !strings.Contains(errResp.Error.Message, "boom_lua") {
		t.Fatalf("expected JSON-RPC error containing 'boom_lua', got %+v", errResp)
	}

	// 2. engine.query_prefabs
	_, prefResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-2",
		"method":  "engine.query_prefabs",
		"params":  map[string]any{"query": "test_cube"},
	})
	if prefResp.Error != nil || prefResp.Result == nil {
		t.Fatalf("query_prefabs failed: %+v", prefResp.Error)
	}
	dataMap := prefResp.Result.Data.(map[string]any)
	if dataMap["count"].(float64) != 1 {
		t.Fatalf("expected 1 prefab match, got %+v", dataMap)
	}

	// 3. engine.query_components
	_, compResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-3",
		"method":  "engine.query_components",
		"params":  map[string]any{"query": "Transform"},
	})
	if compResp.Error != nil || compResp.Result == nil {
		t.Fatalf("query_components failed: %+v", compResp.Error)
	}

	// 4. engine.spawn_prefab
	_, spawnResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-4",
		"method":  "engine.spawn_prefab",
		"params": map[string]any{
			"name": "test_cube",
			"id":   "spawned_rpc_cube",
			"x":    11, "y": 22, "z": 33,
		},
	})
	if spawnResp.Error != nil || spawnResp.Result == nil {
		t.Fatalf("spawn_prefab failed: %+v", spawnResp.Error)
	}
	spawnedObj := bridge.GetGameObject("spawned_rpc_cube")
	if spawnedObj == nil {
		t.Fatalf("expected spawned_rpc_cube in LuaBridge")
	}
	spPos := spawnedObj.GetTransformComponent().GetPosition()
	if spPos.X != 11 || spPos.Y != 22 || spPos.Z != 33 {
		t.Fatalf("expected spawned position (11,22,33), got %+v", spPos)
	}

	// 5. engine.capture_frame (default PNG)
	_, frameResp := postRPC(t, rpcURL, hdr, map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-5",
		"method":  "engine.capture_frame",
	})
	if frameResp.Error != nil || frameResp.Result == nil {
		t.Fatalf("capture_frame failed: %+v", frameResp.Error)
	}
	fMap := frameResp.Result.Data.(map[string]any)
	rawImg, err := base64.StdEncoding.DecodeString(fMap["data"].(string))
	if err != nil {
		t.Fatalf("decode base64 frame: %v", err)
	}
	decodedPNG, err := png.Decode(bytes.NewReader(rawImg))
	if err != nil || decodedPNG.Bounds().Dx() != 2 || decodedPNG.Bounds().Dy() != 2 {
		t.Fatalf("expected valid 2x2 PNG, got err=%v", err)
	}
}

func TestTicket20WebSocketStreamingAndConcurrencyRace(t *testing.T) {
	srv, scene, _, console := setupTestAgentServer(t, "ws-secret")

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

	wsURL := "ws://" + srv.BoundAddress() + "/ws?token=ws-secret"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial failed: %v", err)
	}
	defer conn.Close()

	// Send rapid exec command via WS slim schema
	if err := conn.WriteJSON(map[string]any{
		"id":  1,
		"op":  "exec",
		"cmd": `print("ws_stream_check"); return 40 + 2`,
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	gotReply := false
	gotLog := false
	deadline := time.Now().Add(2 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	for !gotReply || !gotLog {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("WS ReadMessage failed (gotReply=%v gotLog=%v): %v", gotReply, gotLog, err)
		}
		var m map[string]any
		if err := json.Unmarshal(msgBytes, &m); err != nil {
			t.Fatalf("unmarshal WS frame: %v", err)
		}
		if m["event"] == "log" {
			if msgStr, _ := m["msg"].(string); strings.Contains(msgStr, "ws_stream_check") {
				gotLog = true
			}
		} else if m["id"] != nil {
			if m["ok"] != true || m["res"] != "42" {
				t.Fatalf("unexpected WS reply frame: %+v", m)
			}
			gotReply = true
		}
	}

	// Emit an asynchronous console log line and verify WS receives it.
	console.Print("[physics] joint snapped on entity_42", LineSystem)
	gotPhysicsLog := false
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for !gotPhysicsLog {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for physics log: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(msgBytes, &m)
		if m["event"] == "log" && strings.Contains(fmt.Sprint(m["msg"]), "joint snapped on entity_42") {
			gotPhysicsLog = true
		}
	}

	// Concurrent RPC stress under -race across 8 goroutines.
	rpcURL := "http://" + srv.BoundAddress() + "/rpc"
	hdr := map[string]string{"X-Engine-Token": "ws-secret"}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				_, resp := postRPC(t, rpcURL, hdr, map[string]any{
					"jsonrpc": "2.0",
					"id":      fmt.Sprintf("w%d-%d", worker, i),
					"method":  "engine.console_exec",
					"params":  map[string]any{"cmd": fmt.Sprintf("return %d", worker*10+i)},
				})
				if resp.Error != nil || resp.Result == nil {
					t.Errorf("concurrent rpc failed: %+v", resp)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
