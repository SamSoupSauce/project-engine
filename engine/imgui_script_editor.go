package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
)

// ScriptEditorDocument holds the tri-pane authoring state for a single Lua script.
type ScriptEditorDocument struct {
	Name         string
	FilePath     string
	DataJSON     string
	HooksJSON    string
	FunctionsLua string
	ContentHash  string
}

// ScriptEditorBackend decouples the Dear ImGui ScriptEditorWindow from pkg/luabridge
// to prevent circular package imports.
type ScriptEditorBackend interface {
	OpenScript(target string) (ScriptEditorDocument, error)
	ValidateScript(doc ScriptEditorDocument) error
	CommitScript(doc ScriptEditorDocument) (ScriptEditorDocument, error)
	IsEditorOpen() bool
	SetEditorOpen(open bool)
	GetActiveDocument() ScriptEditorDocument
	Revision() uint64
}

// ScriptEditorWindow renders the Dear ImGui tri-pane script authoring modal/window
// (Data JSON, Hooks JSON, and Functions Lua) invoked via console `edit(...)`.
type ScriptEditorWindow struct {
	Backend     ScriptEditorBackend
	IsOpen      bool
	Title       string
	Doc         ScriptEditorDocument
	OriginalDoc ScriptEditorDocument
	StatusMsg   string
	StatusIsErr bool
	Dirty       bool

	lastRevision uint64
}

// NewScriptEditorWindow creates a ScriptEditorWindow backed by the given ScriptEditorBackend.
func NewScriptEditorWindow(backend ScriptEditorBackend) *ScriptEditorWindow {
	return &ScriptEditorWindow{
		Backend: backend,
		Title:   "Script Editor",
	}
}

// Open loads target into the editor window and marks it visible.
// An empty target closes the editor window.
func (w *ScriptEditorWindow) Open(target string) error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: script editor backend is nil")
	}
	if strings.TrimSpace(target) == "" {
		w.Close()
		return nil
	}
	doc, err := w.Backend.OpenScript(target)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.Doc = doc
	w.OriginalDoc = doc
	w.IsOpen = true
	w.Dirty = false
	w.StatusMsg = fmt.Sprintf("Loaded %s", doc.FilePath)
	w.StatusIsErr = false
	w.lastRevision = w.Backend.Revision()
	return nil
}

// Close hides the editor window and clears the backend open flag.
func (w *ScriptEditorWindow) Close() {
	if w == nil {
		return
	}
	w.IsOpen = false
	if w.Backend != nil {
		w.Backend.SetEditorOpen(false)
		w.lastRevision = w.Backend.Revision()
	}
}

// Revert restores all three buffers to the last loaded/committed snapshot.
func (w *ScriptEditorWindow) Revert() {
	if w == nil {
		return
	}
	w.Doc = w.OriginalDoc
	w.Dirty = false
	w.StatusMsg = "Reverted unsaved changes."
	w.StatusIsErr = false
}

// FormatJSON validates and pretty-prints both DataJSON and HooksJSON buffers.
func (w *ScriptEditorWindow) FormatJSON() error {
	if w == nil {
		return nil
	}
	formattedData, err := formatJSONObject(w.Doc.DataJSON, "Data JSON")
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	formattedHooks, err := formatJSONObject(w.Doc.HooksJSON, "Hooks JSON")
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.Doc.DataJSON = formattedData
	w.Doc.HooksJSON = formattedHooks
	w.Dirty = w.Doc != w.OriginalDoc
	w.StatusMsg = "Formatted JSON."
	w.StatusIsErr = false
	return nil
}

// Validate checks JSON syntax and Lua compilation without writing to disk.
func (w *ScriptEditorWindow) Validate() error {
	if w == nil || w.Backend == nil {
		return nil
	}
	if err := w.Backend.ValidateScript(w.Doc); err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.StatusIsErr = false
	return nil
}

// Commit validates, writes to disk, and hot-reloads active instances via Backend.CommitScript.
func (w *ScriptEditorWindow) Commit() error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: script editor backend is nil")
	}
	updated, err := w.Backend.CommitScript(w.Doc)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.Doc = updated
	w.OriginalDoc = updated
	w.Dirty = false
	hashPreview := updated.ContentHash
	if len(hashPreview) > 12 {
		hashPreview = hashPreview[:12]
	}
	w.StatusMsg = fmt.Sprintf("Committed %s (sha256:%s)", updated.FilePath, hashPreview)
	w.StatusIsErr = false
	w.lastRevision = w.Backend.Revision()
	return nil
}

// SyncFromBackend synchronizes window visibility and active document with backend state
// (for example, when Lua `edit(...)` opens or closes the editor).
func (w *ScriptEditorWindow) SyncFromBackend() {
	if w == nil || w.Backend == nil {
		return
	}
	rev := w.Backend.Revision()
	if rev == w.lastRevision {
		return
	}
	w.lastRevision = rev
	if !w.Backend.IsEditorOpen() {
		w.IsOpen = false
		return
	}
	doc := w.Backend.GetActiveDocument()
	w.Doc = doc
	w.OriginalDoc = doc
	w.IsOpen = true
	w.Dirty = false
	w.StatusMsg = fmt.Sprintf("Editing %s", doc.FilePath)
	w.StatusIsErr = false
}

// Draw renders the Dear ImGui tri-pane script editor window when open.
func (w *ScriptEditorWindow) Draw() {
	if w == nil {
		return
	}
	w.SyncFromBackend()
	if !w.IsOpen {
		return
	}

	name := w.Doc.Name
	if name == "" {
		name = "Untitled.lua"
	}
	title := fmt.Sprintf("Script Editor: %s###ScriptEditorWindow", name)

	open := true
	imgui.SetNextWindowSizeV(imgui.NewVec2(760, 540), imgui.CondFirstUseEver)
	if imgui.BeginV(title, &open, imgui.WindowFlagsNone) {
		if w.Doc.FilePath != "" {
			imgui.TextColored(imgui.NewVec4(0.7, 0.8, 0.9, 1.0), fmt.Sprintf("File: %s", w.Doc.FilePath))
		}
		if w.Dirty {
			imgui.SameLine()
			imgui.TextColored(imgui.NewVec4(1.0, 0.85, 0.35, 1.0), " (modified)")
		}

		avail := imgui.ContentRegionAvail()
		topPaneH := avail.Y * 0.36
		if topPaneH < 110 {
			topPaneH = 110
		}
		footerH := imgui.FrameHeightWithSpacing()*2.2 + 12
		bottomPaneH := avail.Y - topPaneH - footerH - 36
		if bottomPaneH < 120 {
			bottomPaneH = 120
		}

		edited := false
		if imgui.BeginTable("##ScriptEditorTopPanes", 2) {
			imgui.TableNextRow()
			imgui.TableSetColumnIndex(0)
			imgui.TextUnformatted("Pane 1: Data (JSON)")
			if imgui.InputTextMultiline("##ScriptDataJSON", &w.Doc.DataJSON, imgui.NewVec2(-1, topPaneH), imgui.InputTextFlagsAllowTabInput, nil) {
				edited = true
			}

			imgui.TableSetColumnIndex(1)
			imgui.TextUnformatted("Pane 2: Hooks (JSON)")
			if imgui.InputTextMultiline("##ScriptHooksJSON", &w.Doc.HooksJSON, imgui.NewVec2(-1, topPaneH), imgui.InputTextFlagsAllowTabInput, nil) {
				edited = true
			}
			imgui.EndTable()
		}

		imgui.TextUnformatted("Pane 3: Functions (Lua Logic)")
		if imgui.InputTextMultiline("##ScriptFunctionsLua", &w.Doc.FunctionsLua, imgui.NewVec2(-1, bottomPaneH), imgui.InputTextFlagsAllowTabInput, nil) {
			edited = true
		}

		if edited {
			w.Dirty = w.Doc != w.OriginalDoc
			_ = w.Validate()
			if !w.StatusIsErr {
				w.StatusMsg = "Syntax OK (unsaved changes)"
			}
		}

		imgui.Separator()
		if w.StatusMsg != "" {
			col := imgui.NewVec4(0.55, 0.95, 0.65, 1.0)
			if w.StatusIsErr {
				col = imgui.NewVec4(1.0, 0.4, 0.4, 1.0)
			}
			imgui.TextColored(col, w.StatusMsg)
		} else {
			imgui.TextUnformatted("Ready")
		}

		if imgui.Button("Revert") {
			w.Revert()
		}
		imgui.SameLine()
		if imgui.Button("Format JSON") {
			_ = w.FormatJSON()
		}
		imgui.SameLine()
		if imgui.Button("Commit") {
			_ = w.Commit()
		}
	}
	imgui.End()

	if !open {
		w.Close()
	}
}

func formatJSONObject(raw, label string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}", nil
	}
	if !json.Valid([]byte(trimmed)) {
		return "", fmt.Errorf("%s: invalid JSON syntax", label)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return "", fmt.Errorf("%s: expected JSON object: %w", label, err)
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
