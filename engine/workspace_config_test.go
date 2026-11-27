package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceConfigAndVerification(t *testing.T) {
	tmp := t.TempDir()

	// Empty directory must fail verification cleanly with actionable missing paths.
	_, emptyPaths, err := LoadWorkspaceConfig(tmp, true, "")
	if err != nil {
		t.Fatalf("LoadWorkspaceConfig failed: %v", err)
	}
	verErr := VerifyWorkspaceStructure(emptyPaths)
	if verErr == nil {
		t.Fatalf("expected verification error on empty root %q", tmp)
	}
	var wve *WorkspaceVerificationError
	if !errors.As(verErr, &wve) {
		t.Fatalf("expected *WorkspaceVerificationError, got %T", verErr)
	}
	if len(wve.MissingDirs) != 4 {
		t.Fatalf("expected 4 missing subdirs (assets, scripts, prefabs, config), got %v", wve.MissingDirs)
	}
	if !strings.Contains(verErr.Error(), "--init-dirs") {
		t.Fatalf("expected error message to mention --init-dirs, got %q", verErr.Error())
	}

	// InitWorkspaceDirs should scaffold all required directories and pass verification.
	created, err := InitWorkspaceDirs(emptyPaths)
	if err != nil {
		t.Fatalf("InitWorkspaceDirs failed: %v", err)
	}
	if len(created) == 0 {
		t.Fatalf("expected directories to be created")
	}
	if err := VerifyWorkspaceStructure(emptyPaths); err != nil {
		t.Fatalf("expected verification to pass after InitWorkspaceDirs, got %v", err)
	}
}

func TestWorkspaceConfigOverridesFromFileAndCLI(t *testing.T) {
	tmp := t.TempDir()
	customRoot := filepath.Join(tmp, "my_game")
	if err := os.MkdirAll(filepath.Join(customRoot, "data_assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(customRoot, "lua_scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(customRoot, "game_prefabs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(customRoot, "user_cfg"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(customRoot, "engine.json")
	cfgJSON := `{
  "assets_dir": "data_assets",
  "scripts_dir": "lua_scripts",
  "prefabs_dir": "game_prefabs",
  "config_dir": "user_cfg",
  "window_title": "Custom Game Title"
}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Explicit --config without --root infers root from config directory.
	cfg, paths, err := LoadWorkspaceConfig(".", false, cfgPath)
	if err != nil {
		t.Fatalf("LoadWorkspaceConfig with --config failed: %v", err)
	}
	if !paths.ConfigLoaded || cfg.WindowTitle != "Custom Game Title" {
		t.Fatalf("expected config to load window title, got %+v / %+v", cfg, paths)
	}
	if filepath.Clean(paths.RootDir) != filepath.Clean(customRoot) {
		t.Fatalf("expected inferred root %q, got %q", customRoot, paths.RootDir)
	}
	if err := VerifyWorkspaceStructure(paths); err != nil {
		t.Fatalf("VerifyWorkspaceStructure failed: %v", err)
	}

	resolvedModel := paths.ResolvePath("assets/models/props/rainbow_cube.gltf")
	expectedModel := filepath.Join(customRoot, "data_assets", "models", "props", "rainbow_cube.gltf")
	if resolvedModel != expectedModel {
		t.Fatalf("expected resolved asset %q, got %q", expectedModel, resolvedModel)
	}

	// Explicit --config pointing to nonexistent file fails fast.
	if _, _, err := LoadWorkspaceConfig(".", false, filepath.Join(tmp, "missing.json")); err == nil {
		t.Fatalf("expected error for missing explicit --config file")
	}
}
