package main

import (
	"path/filepath"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestParseDemoServerFlags(t *testing.T) {
	// Default without --serve succeeds without --token.
	opts, err := parseDemoServerFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error without --serve: %v", err)
	}
	if opts.Serve || opts.Address != "127.0.0.1:4242" || opts.Root != "." || opts.RootExplicit || opts.InitDirs {
		t.Fatalf("unexpected default opts: %+v", opts)
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
