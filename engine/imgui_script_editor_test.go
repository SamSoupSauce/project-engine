package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AllenDang/cimgui-go/imgui"
)

type mockScriptEditorBackend struct {
	open     bool
	doc      ScriptEditorDocument
	revision uint64
	failNext error
	commits  int
}

func (m *mockScriptEditorBackend) OpenScript(target string) (ScriptEditorDocument, error) {
	if m.failNext != nil {
		err := m.failNext
		m.failNext = nil
		return ScriptEditorDocument{}, err
	}
	m.doc = ScriptEditorDocument{
		Name:         target,
		FilePath:     "scripts/components/" + target + ".lua",
		DataJSON:     `{"hp": 100}`,
		HooksJSON:    `{"Update": ["number"]}`,
		FunctionsLua: fmt.Sprintf("function %s:Update(dt)\nend\n", target),
		ContentHash:  "initialhash1234",
	}
	m.open = true
	m.revision++
	return m.doc, nil
}

func (m *mockScriptEditorBackend) ValidateScript(doc ScriptEditorDocument) error {
	if _, err := formatJSONObject(doc.DataJSON, "Data (JSON)"); err != nil {
		return err
	}
	if _, err := formatJSONObject(doc.HooksJSON, "Hooks (JSON)"); err != nil {
		return err
	}
	if strings.Contains(doc.FunctionsLua, "SYNTAX_ERR") {
		return fmt.Errorf("Lua syntax error near SYNTAX_ERR")
	}
	return nil
}

func (m *mockScriptEditorBackend) CommitScript(doc ScriptEditorDocument) (ScriptEditorDocument, error) {
	if err := m.ValidateScript(doc); err != nil {
		return doc, err
	}
	m.commits++
	doc.ContentHash = fmt.Sprintf("abcdef123456%02d", m.commits)
	m.doc = doc
	m.revision++
	return doc, nil
}

func (m *mockScriptEditorBackend) IsEditorOpen() bool { return m.open }
func (m *mockScriptEditorBackend) SetEditorOpen(open bool) {
	m.open = open
	m.revision++
}
func (m *mockScriptEditorBackend) GetActiveDocument() ScriptEditorDocument { return m.doc }
func (m *mockScriptEditorBackend) Revision() uint64                      { return m.revision }

func TestScriptEditorWindowLifecycleAndHeadlessImGui(t *testing.T) {
	backend := &mockScriptEditorBackend{}
	overlay := NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()
	overlay.SetScriptEditor(backend)

	// Trigger open from backend (as Lua `edit("MyScript")` does).
	if _, err := backend.OpenScript("MyScript"); err != nil {
		t.Fatalf("OpenScript failed: %v", err)
	}

	var drawDataValid bool
	overlay.onHeadlessDraw = func(dd *imgui.DrawData) {
		drawDataValid = dd != nil && dd.Valid()
	}

	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	if !overlay.ScriptEditor.IsOpen {
		t.Fatalf("expected ScriptEditor window to sync open from backend")
	}
	if !drawDataValid {
		t.Fatalf("expected valid ImGui DrawData with ScriptEditor open")
	}
	if overlay.ScriptEditor.Doc.Name != "MyScript" {
		t.Fatalf("expected active doc Name 'MyScript', got %q", overlay.ScriptEditor.Doc.Name)
	}

	// FormatJSON prettifies valid JSON and flags invalid JSON without crashing.
	overlay.ScriptEditor.Doc.DataJSON = `{"hp":250,"speed":6.5}`
	if err := overlay.ScriptEditor.FormatJSON(); err != nil {
		t.Fatalf("FormatJSON failed: %v", err)
	}
	if !strings.Contains(overlay.ScriptEditor.Doc.DataJSON, "\n") {
		t.Fatalf("expected pretty-printed multiline JSON, got %q", overlay.ScriptEditor.Doc.DataJSON)
	}

	overlay.ScriptEditor.Doc.DataJSON = `{bad json`
	if err := overlay.ScriptEditor.FormatJSON(); err == nil || !overlay.ScriptEditor.StatusIsErr {
		t.Fatalf("expected FormatJSON error on malformed JSON")
	}

	// Revert restores original snapshot.
	overlay.ScriptEditor.Revert()
	if overlay.ScriptEditor.Doc.DataJSON != `{"hp": 100}` || overlay.ScriptEditor.StatusIsErr {
		t.Fatalf("expected Revert to restore original DataJSON and clear error, got %q", overlay.ScriptEditor.Doc.DataJSON)
	}

	// Invalid Lua syntax fails Commit safely without crashing.
	overlay.ScriptEditor.Doc.FunctionsLua = "SYNTAX_ERR"
	if err := overlay.ScriptEditor.Commit(); err == nil || !overlay.ScriptEditor.StatusIsErr {
		t.Fatalf("expected Commit to fail on invalid Lua syntax")
	}
	if backend.commits != 0 {
		t.Fatalf("expected 0 commits on validation error, got %d", backend.commits)
	}

	// Valid edit commits cleanly and updates OriginalDoc & ContentHash.
	overlay.ScriptEditor.Doc.FunctionsLua = "function MyScript:Update(dt)\n  -- ok\nend\n"
	if err := overlay.ScriptEditor.Commit(); err != nil {
		t.Fatalf("expected valid Commit to succeed, got %v", err)
	}
	if overlay.ScriptEditor.StatusIsErr || backend.commits != 1 {
		t.Fatalf("expected 1 successful commit, err=%v msg=%q", overlay.ScriptEditor.StatusIsErr, overlay.ScriptEditor.StatusMsg)
	}
	if !strings.Contains(overlay.ScriptEditor.StatusMsg, "abcdef123456") {
		t.Fatalf("expected SHA-256 hash prefix in status message, got %q", overlay.ScriptEditor.StatusMsg)
	}

	// Closing via backend (`edit()` with no args) syncs window closed.
	backend.SetEditorOpen(false)
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()
	if overlay.ScriptEditor.IsOpen {
		t.Fatalf("expected ScriptEditor window to close after backend SetEditorOpen(false)")
	}
}
