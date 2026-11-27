package engine

import (
	"path/filepath"
	"testing"

	"github.com/AllenDang/cimgui-go/imgui"
)

func TestTutorialStatePersistenceAndHeadlessWindow(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "config", "tutorial_state.json")

	// First run: missing file opens tutorial by default.
	tw := NewTutorialWindow(statePath)
	if !tw.IsOpen() {
		t.Fatalf("expected tutorial window to be open on first run")
	}
	if tw.DoNotShowAgain {
		t.Fatalf("expected DoNotShowAgain to default to false")
	}

	// Render a headless ImGui frame with TutorialWindow attached.
	overlay := NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()
	overlay.Tutorial = tw

	var drawDataValid bool
	var vtxCount int32
	overlay.onHeadlessDraw = func(dd *imgui.DrawData) {
		if dd != nil {
			drawDataValid = dd.Valid()
			vtxCount = dd.TotalVtxCount()
		}
	}
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()
	if !drawDataValid || vtxCount <= 0 {
		t.Fatalf("expected valid DrawData with non-zero vertices when tutorial window is open (valid=%v, vtx=%d)", drawDataValid, vtxCount)
	}

	// Persist "Do not show again" and verify next launch keeps tutorial closed.
	if err := tw.SetDoNotShowAgain(true); err != nil {
		t.Fatalf("SetDoNotShowAgain failed: %v", err)
	}
	twNext := NewTutorialWindow(statePath)
	if twNext.IsOpen() {
		t.Fatalf("expected tutorial window to remain closed when DoNotShowAgain=true")
	}
	if !twNext.DoNotShowAgain {
		t.Fatalf("expected DoNotShowAgain=true to persist across runs")
	}

	// Manual Show() reopens it even when DoNotShowAgain=true.
	twNext.Show()
	if !twNext.IsOpen() {
		t.Fatalf("expected Show() to open tutorial window on demand")
	}
}
