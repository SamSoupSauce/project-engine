package luabridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/SamSoupSauce/project-engine/engine/ui"
)

func TestLuaJITFFIDrawListPassThrough(t *testing.T) {
	bridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "hud_overlay.lua")); err != nil {
		t.Fatalf("DoFile hud_overlay.lua failed: %v", err)
	}

	// Headless ImGui frame to receive DrawList FFI calls directly into ImDrawList
	ctx := imgui.CreateContext()
	defer imgui.DestroyContextV(ctx)
	io := imgui.CurrentIO()
	io.SetBackendFlags(imgui.BackendFlagsRendererHasTextures)
	io.SetDisplaySize(imgui.NewVec2(1280, 720))
	io.SetDeltaTime(1.0 / 60.0)
	io.Fonts().AddFontDefault()
	imgui.InternalImFontAtlasBuildMain(io.Fonts())

	var recorded []DrawListCall
	SetDrawListRecorder(&recorded)
	defer SetDrawListRecorder(nil)

	imgui.NewFrame()
	dl := imgui.ForegroundDrawListViewportPtr()
	if err := bridge.CallTick("run_ui_tick", 1.0/60.0); err != nil {
		t.Fatalf("CallTick failed: %v", err)
	}
	if err := bridge.CallRender("render_ui_frame", dl, 1280, 720); err != nil {
		t.Fatalf("CallRender failed: %v", err)
	}
	imgui.Render()

	dd := imgui.CurrentDrawData()
	if dd == nil || !dd.Valid() || dd.TotalVtxCount() <= 0 {
		t.Fatalf("expected non-empty ImDrawData after LuaJIT DrawList FFI pass")
	}
	if len(recorded) == 0 {
		t.Fatalf("expected recorded DrawList FFI calls from LuaJIT")
	}
}

func TestLuaJITPointerDispatchLatency(t *testing.T) {
	bridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "hud_overlay.lua")); err != nil {
		t.Fatalf("DoFile hud_overlay.lua failed: %v", err)
	}

	root, err := ui.LoadUIJSON([]byte(`{
		"id": "root",
		"style": { "direction": "row", "padding": 16 },
		"children": [
			{ "id": "play_btn", "style": { "width": 200, "height": 60 }, "on_click": "on_play_click" }
		]
	}`))
	if err != nil {
		t.Fatalf("LoadUIJSON failed: %v", err)
	}
	ui.ComputeLayout(root, 1280, 720)

	dispatcher := &ui.PointerEventDispatcher{}
	bridge.BindPointerDispatcher(dispatcher)

	// Warmup JIT traces
	for i := 0; i < 100; i++ {
		_, handled := dispatcher.DispatchClick(root, 40, 40)
		if !handled {
			t.Fatalf("expected click to be handled during warmup")
		}
	}

	const iterations = 1000
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, handled := dispatcher.DispatchClick(root, 40, 40)
		if !handled {
			t.Fatalf("expected click to be handled")
		}
	}
	elapsed := time.Since(start)
	avg := elapsed / iterations
	t.Logf("LuaJIT pointer dispatch average latency: %v (%d iterations in %v)", avg, iterations, elapsed)
	if avg >= 50*time.Microsecond {
		t.Fatalf("pointer dispatch exceeded 0.05ms budget: avg=%v", avg)
	}
}

func TestLuaJITTraceAuditZeroNYIAborts(t *testing.T) {
	bridge, err := NewLuaJITBridge()
	if err != nil {
		t.Fatalf("NewLuaJITBridge failed: %v", err)
	}
	defer bridge.Close()

	if err := bridge.DoFile(filepath.Join("..", "..", "scripts", "ui", "hud_overlay.lua")); err != nil {
		t.Fatalf("DoFile hud_overlay.lua failed: %v", err)
	}

	dumpPath := filepath.Join(t.TempDir(), "jit_trace_dump.log")
	if err := bridge.StartJITDump("bitmsx", dumpPath); err != nil {
		t.Fatalf("StartJITDump failed: %v", err)
	}

	ctx := imgui.CreateContext()
	defer imgui.DestroyContextV(ctx)
	io := imgui.CurrentIO()
	io.SetBackendFlags(imgui.BackendFlagsRendererHasTextures)
	io.SetDisplaySize(imgui.NewVec2(1280, 720))
	io.SetDeltaTime(1.0 / 60.0)
	io.Fonts().AddFontDefault()
	imgui.InternalImFontAtlasBuildMain(io.Fonts())

	// Execute continuous 60+ FPS interaction loop (tick, hover, click, render)
	for i := 0; i < 240; i++ {
		imgui.NewFrame()
		dl := imgui.ForegroundDrawListViewportPtr()
		if err := bridge.CallTick("run_ui_tick", 1.0/60.0); err != nil {
			t.Fatalf("tick %d failed: %v", i, err)
		}
		if _, err := bridge.CallPointerHandler("on_panel_hover", "panel", float32(40+i%50), 30); err != nil {
			t.Fatalf("hover %d failed: %v", i, err)
		}
		if err := bridge.CallRender("render_ui_frame", dl, 1280, 720); err != nil {
			t.Fatalf("render %d failed: %v", i, err)
		}
		imgui.Render()
	}

	if err := bridge.StopJITDump(); err != nil {
		t.Fatalf("StopJITDump failed: %v", err)
	}

	raw, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("failed reading jit.dump output %q: %v", dumpPath, err)
	}
	dumpText := string(raw)
	if !strings.Contains(dumpText, "TRACE") {
		t.Fatalf("expected compiled TRACE entries in jit.dump output, got:\n%s", dumpText)
	}
	if strings.Contains(dumpText, "NYI") {
		t.Fatalf("detected NYI abort in jit.dump output:\n%s", dumpText)
	}
}
