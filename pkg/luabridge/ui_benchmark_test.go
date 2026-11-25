package luabridge

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestUIZeroAllocationBenchmarkAndJITVerbose(t *testing.T) {
	// Phase 1: Run benchmark script cleanly without jit.v hook overhead.
	bridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "hud_overlay.lua")); err != nil {
		t.Fatalf("DoFile hud_overlay.lua failed: %v", err)
	}
	if err := bridge.DoFile(filepath.Join("..", "..", "tests", "ui_benchmark.lua")); err != nil {
		t.Fatalf("ui_benchmark.lua failed: %v", err)
	}

	// Phase 2: Verify jit.v trace output across a fresh LuaJIT state running the same UI loop.
	jitBridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge (jit.v) failed: %v", err)
	}
	defer jitBridge.Close()

	if err := jitBridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "hud_overlay.lua")); err != nil {
		t.Fatalf("DoFile hud_overlay.lua failed: %v", err)
	}

	jitVLog := filepath.Join(t.TempDir(), "jit_v_phase2.log")
	if err := jitBridge.StartJITVerbose(jitVLog); err != nil {
		t.Fatalf("StartJITVerbose failed: %v", err)
	}
	if err := jitBridge.DoFile(filepath.Join("..", "..", "tests", "ui_benchmark.lua")); err != nil {
		t.Fatalf("ui_benchmark.lua (jit.v) failed: %v", err)
	}
	if err := jitBridge.StopJITVerbose(); err != nil {
		t.Fatalf("StopJITVerbose failed: %v", err)
	}

	raw, err := os.ReadFile(jitVLog)
	if err != nil {
		t.Fatalf("failed reading jit.v log: %v", err)
	}
	logText := string(raw)
	if !strings.Contains(logText, "[TRACE") {
		t.Fatalf("expected compiled [TRACE entries in jit.v log, got:\n%s", logText)
	}
	if strings.Contains(logText, "NYI") {
		t.Fatalf("detected NYI bailout in jit.v log:\n%s", logText)
	}
}

func TestUIScriptsStaticZeroAllocAudit(t *testing.T) {
	scriptsDir := filepath.Join("..", "..", "scripts", "ui")
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		t.Fatalf("failed reading %s: %v", scriptsDir, err)
	}

	forbidden := []string{
		"pairs(",
		"ipairs(",
		"table.insert",
		"loadstring",
		"string.dump",
		"..",
	}
	tableCtorRe := regexp.MustCompile(`\{[^}]*\}`)

	luaCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		luaCount++
		fullPath := filepath.Join(scriptsDir, entry.Name())
		data, rerr := os.ReadFile(fullPath)
		if rerr != nil {
			t.Fatalf("read %s failed: %v", fullPath, rerr)
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "--") {
				continue
			}
			for _, pat := range forbidden {
				if strings.Contains(trimmed, pat) {
					t.Errorf("%s:%d contains forbidden LuaJIT hot-path construct %q", entry.Name(), lineNo+1, pat)
				}
			}
			if tableCtorRe.MatchString(trimmed) {
				t.Errorf("%s:%d contains forbidden Lua table constructor %q", entry.Name(), lineNo+1, trimmed)
			}
		}
	}
	if luaCount == 0 {
		t.Fatalf("expected at least 1 Lua UI script in %s", scriptsDir)
	}
}
