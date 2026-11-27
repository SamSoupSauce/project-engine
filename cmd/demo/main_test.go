package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestParseDemoServerFlags(t *testing.T) {
	// Default without --serve succeeds without --token.
	opts, err := parseDemoServerFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error without --serve: %v", err)
	}
	if opts.Serve || opts.Address != "127.0.0.1:4242" || opts.Root != "." || opts.RootExplicit || opts.InitDirs || opts.Version {
		t.Fatalf("unexpected default opts: %+v", opts)
	}

	opts, err = parseDemoServerFlags([]string{"--version"})
	if err != nil || !opts.Version {
		t.Fatalf("expected --version flag to parse cleanly, got %+v, err=%v", opts, err)
	}

	// --root, --config, --init-dirs parsing.
	opts, err = parseDemoServerFlags([]string{"--root=examples/custom-game-root", "--config=custom.json", "--init-dirs"})
	if err != nil {
		t.Fatalf("unexpected error with root/config/init-dirs: %v", err)
	}
	if !opts.RootExplicit || opts.Root != "examples/custom-game-root" || opts.Config != "custom.json" || !opts.InitDirs {
		t.Fatalf("unexpected root/config opts: %+v", opts)
	}

	// --serve without --token fails fast.
	if _, err := parseDemoServerFlags([]string{"--serve"}); err == nil {
		t.Fatalf("expected error when --serve is set without --token")
	}

	// --serve with explicit empty --token="" succeeds.
	opts, err = parseDemoServerFlags([]string{"--serve", "--token="})
	if err != nil {
		t.Fatalf("unexpected error with --token=: %v", err)
	}
	if !opts.Serve || opts.Token != "" {
		t.Fatalf("unexpected opts with empty token: %+v", opts)
	}

	// --serve with custom --address and --token succeeds.
	opts, err = parseDemoServerFlags([]string{"--serve", "--address=0.0.0.0:9000", "--token=abc123"})
	if err != nil {
		t.Fatalf("unexpected error with custom flags: %v", err)
	}
	if !opts.Serve || opts.Address != "0.0.0.0:9000" || opts.Token != "abc123" {
		t.Fatalf("unexpected opts: %+v", opts)
	}

	// --tunnel implies --serve and requires --token.
	if _, err := parseDemoServerFlags([]string{"--tunnel"}); err == nil {
		t.Fatalf("expected error when --tunnel is set without --token")
	}
	opts, err = parseDemoServerFlags([]string{"--tunnel", "--token="})
	if err != nil {
		t.Fatalf("unexpected error with --tunnel --token=: %v", err)
	}
	if !opts.Serve || !opts.Tunnel {
		t.Fatalf("expected --tunnel to enable Serve and Tunnel, got %+v", opts)
	}
}

func TestParseDemoServerFlagsHeadless(t *testing.T) {
	opts, err := parseDemoServerFlags(nil)
	if err != nil || opts.Headless {
		t.Fatalf("headless should be off by default, got %+v, err=%v", opts, err)
	}

	opts, err = parseDemoServerFlags([]string{"--headless"})
	if err != nil || !opts.Headless {
		t.Fatalf("expected --headless to be parsed, got %+v, err=%v", opts, err)
	}

	opts, err = parseDemoServerFlags([]string{"--headless", "--serve", "--token="})
	if err != nil || !opts.Headless || !opts.Serve {
		t.Fatalf("expected --headless --serve to be accepted, got %+v, err=%v", opts, err)
	}
}

func TestRunHeadlessLoopTicksSceneUntilCancel(t *testing.T) {
	scene := engine.NewBaseScene("headless-test")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runHeadlessLoop(ctx, scene, 5*time.Millisecond)
		close(done)
	}()

	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runHeadlessLoop did not stop after context cancellation")
	}

	if got := scene.GetTickCount(); got == 0 {
		t.Fatal("expected headless loop to dispatch fixed ticks")
	}
	if scene.IsLoaded() {
		t.Fatal("expected headless loop to unload the scene on exit")
	}
}

func TestRunDemoHeadlessServesAgentRPC(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve loopback port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	opts := demoServerOptions{
		Root:         repoRoot,
		RootExplicit: true,
		Headless:     true,
		Serve:        true,
		Address:      addr,
		Token:        "headless-test-token",
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runDemo(ctx, opts) }()

	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/rpc",
			strings.NewReader(`{"jsonrpc":"2.0","id":"t1","method":"engine.ping","params":{}}`))
		if reqErr != nil {
			t.Fatalf("failed to build RPC request: %v", reqErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Engine-Token", opts.Token)
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("headless agent bridge did not come up: %v", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("failed to read RPC response: %v", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("engine.ping status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("engine.ping body missing status ok: %s", body)
	}

	cancel()
	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatalf("runDemo returned error: %v", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDemo did not shut down after context cancellation")
	}
}

func TestKitchenSinkSceneLoadsAllPropsAndTiles(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	_, paths, err := engine.LoadWorkspaceConfig(repoRoot, true, "")
	if err != nil {
		t.Fatalf("LoadWorkspaceConfig failed: %v", err)
	}
	if err := engine.VerifyWorkspaceStructure(paths); err != nil {
		t.Fatalf("VerifyWorkspaceStructure failed: %v", err)
	}

	scene := newKitchenSinkDemoScene(nil, paths)
	defer scene.Unload()
	scene.Init()

	if !scene.tileStats.MapLoaded || scene.tileStats.TotalTiles == 0 {
		t.Fatalf("expected kitchen_sink.tmx tiles to load, got %+v (warnings=%v)", scene.tileStats, scene.warnings)
	}
	if scene.tileStats.FloorCount == 0 || scene.tileStats.WallCount == 0 || scene.tileStats.PillarCount == 0 || scene.tileStats.StairCount == 0 {
		t.Fatalf("expected floors, walls, pillars, and stairs in tile stats, got %+v", scene.tileStats)
	}
	if scene.tileMapObj == nil || scene.tileMapObj.GetMeshComponent() == nil || scene.tileMapObj.GetMeshComponent().Asset == nil {
		t.Fatalf("expected procedural tile map GLTF asset on tileMapObj")
	}
	for _, spec := range kitchenSinkPropSpecs {
		obj := scene.propObjects[spec.ID]
		if obj == nil {
			t.Fatalf("missing showcase prop %q", spec.ID)
		}
		mc := obj.GetMeshComponent()
		if mc == nil || mc.Asset == nil || mc.LoadErr != nil {
			t.Fatalf("expected loaded glTF mesh on prop %q (mc=%v)", spec.ID, mc)
		}
	}
	if len(scene.warnings) > 0 {
		t.Fatalf("expected zero warnings in kitchen sink demo, got %v", scene.warnings)
	}

	// Exercise interactive fracture & reset without GL window.
	scene.FractureDemoLamp()
	if !scene.lampFractured {
		t.Fatalf("expected FractureDemoLamp to mark lampFractured=true")
	}
	scene.ResetShowcaseProps()
	if scene.lampFractured {
		t.Fatalf("expected ResetShowcaseProps to restore lampFractured=false")
	}
}
