package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
)

// PrefabComponentData describes a single serialized component inside a Prefab JSON.
type PrefabComponentData struct {
	Name    string         `json:"name,omitempty"`
	Type    string         `json:"type,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// PrefabSpec is the canonical JSON schema describing a GameObject Prefab.
type PrefabSpec struct {
	ID         string                `json:"id,omitempty"`
	Name       string                `json:"name"`
	Active     bool                  `json:"active"`
	Components []PrefabComponentData `json:"components"`
	Children   []PrefabSpec          `json:"children,omitempty"`
}

// UnmarshalJSON parses a PrefabSpec JSON object, defaulting Active to true when omitted
// and gracefully ignoring string child IDs if copied from runtime GameObject JSON.
func (p *PrefabSpec) UnmarshalJSON(raw []byte) error {
	type rawPrefab struct {
		ID         string                `json:"id"`
		Name       string                `json:"name"`
		Active     *bool                 `json:"active"`
		Components []PrefabComponentData `json:"components"`
		Children   []json.RawMessage     `json:"children"`
	}
	var dec rawPrefab
	if err := json.Unmarshal(raw, &dec); err != nil {
		return err
	}
	p.ID = dec.ID
	p.Name = dec.Name
	if dec.Active != nil {
		p.Active = *dec.Active
	} else {
		p.Active = true
	}
	p.Components = dec.Components
	if p.Components == nil {
		p.Components = []PrefabComponentData{}
	}
	p.Children = nil
	for _, chRaw := range dec.Children {
		trimmed := bytes.TrimSpace(chRaw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			continue
		}
		var child PrefabSpec
		if err := json.Unmarshal(trimmed, &child); err != nil {
			return fmt.Errorf("invalid child prefab: %w", err)
		}
		p.Children = append(p.Children, child)
	}
	return nil
}

// PrefabSummary contains lightweight metadata for listing and searching prefabs.
type PrefabSummary struct {
	Name           string
	FilePath       string
	RelativePath   string
	Active         bool
	ComponentNames []string
	ChildCount     int
}

// PrefabDocument holds the editable state of a loaded or newly created Prefab JSON file.
type PrefabDocument struct {
	Name        string
	FilePath    string
	RawJSON     string
	ContentHash string
	Spec        PrefabSpec
}

// PrefabEditorBackend decouples PrefabEditorWindow from pkg/luabridge.
type PrefabEditorBackend interface {
	IsPrefabEditorOpen() bool
	SetPrefabEditorOpen(open bool)
	SearchPrefabs(query string) ([]PrefabSummary, error)
	OpenPrefab(target string) (PrefabDocument, error)
	ValidatePrefab(doc PrefabDocument) error
	CommitPrefab(doc PrefabDocument) (PrefabDocument, error)
	DeletePrefab(target string) error
	SpawnPrefab(target string) (string, error)
	CreatePrefabFromSelected(name string) (PrefabDocument, error)
	HasSelectedObject() (id string, name string, ok bool)
	AvailableComponentNames() []string
	GetActivePrefabDocument() PrefabDocument
	Revision() uint64
}

// PrefabEditorWindow renders the Dear ImGui Prefab Browser, Viewer, and Editor window.
type PrefabEditorWindow struct {
	Backend     PrefabEditorBackend
	IsOpen      bool
	Title       string
	SearchQuery string
	NewName     string
	Prefabs     []PrefabSummary
	Doc         PrefabDocument
	OriginalDoc PrefabDocument
	StatusMsg   string
	StatusIsErr bool
	Dirty       bool

	addCompIdx   int
	lastRevision uint64
	listLoaded   bool
}

// NewPrefabEditorWindow creates a PrefabEditorWindow bound to backend.
func NewPrefabEditorWindow(backend PrefabEditorBackend) *PrefabEditorWindow {
	return &PrefabEditorWindow{
		Backend: backend,
		Title:   "Prefab Manager",
	}
}

// Open opens the Prefab Editor window, refreshes the prefab list, and optionally loads target.
func (w *PrefabEditorWindow) Open(target string) error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prefab editor backend is nil")
	}
	w.IsOpen = true
	w.Backend.SetPrefabEditorOpen(true)
	_ = w.RefreshList()

	if strings.TrimSpace(target) != "" {
		doc, err := w.Backend.OpenPrefab(target)
		if err != nil {
			w.StatusMsg = err.Error()
			w.StatusIsErr = true
			return err
		}
		w.setLoadedDoc(doc, fmt.Sprintf("Loaded %s", doc.FilePath))
	} else {
		w.lastRevision = w.Backend.Revision()
	}
	return nil
}

// Close hides the Prefab Editor window and updates backend visibility.
func (w *PrefabEditorWindow) Close() {
	if w == nil {
		return
	}
	w.IsOpen = false
	if w.Backend != nil {
		w.Backend.SetPrefabEditorOpen(false)
		w.lastRevision = w.Backend.Revision()
	}
}

// RefreshList updates w.Prefabs using the current SearchQuery.
func (w *PrefabEditorWindow) RefreshList() error {
	if w == nil || w.Backend == nil {
		return nil
	}
	list, err := w.Backend.SearchPrefabs(w.SearchQuery)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.Prefabs = list
	w.listLoaded = true
	return nil
}

// SelectPrefab loads target into the editor panes.
func (w *PrefabEditorWindow) SelectPrefab(target string) error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prefab editor backend is nil")
	}
	doc, err := w.Backend.OpenPrefab(target)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.setLoadedDoc(doc, fmt.Sprintf("Loaded %s", doc.FilePath))
	return nil
}

// Revert restores RawJSON and Spec to the last loaded/saved snapshot.
func (w *PrefabEditorWindow) Revert() {
	if w == nil {
		return
	}
	w.Doc = w.OriginalDoc
	w.Dirty = false
	w.StatusMsg = "Reverted unsaved changes."
	w.StatusIsErr = false
}

// FormatJSON validates and pretty-prints RawJSON and syncs Spec.
func (w *PrefabEditorWindow) FormatJSON() error {
	if w == nil {
		return nil
	}
	trimmed := strings.TrimSpace(w.Doc.RawJSON)
	if trimmed == "" {
		trimmed = "{}"
	}
	if !json.Valid([]byte(trimmed)) {
		w.StatusMsg = "Prefab JSON: invalid JSON syntax"
		w.StatusIsErr = true
		return fmt.Errorf("%s", w.StatusMsg)
	}
	var spec PrefabSpec
	if err := json.Unmarshal([]byte(trimmed), &spec); err != nil {
		w.StatusMsg = fmt.Sprintf("Prefab JSON: %v", err)
		w.StatusIsErr = true
		return err
	}
	if spec.Name == "" {
		spec.Name = w.Doc.Name
	}
	pretty, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.Doc.Spec = spec
	w.Doc.RawJSON = string(pretty) + "\n"
	w.Dirty = w.Doc.RawJSON != w.OriginalDoc.RawJSON
	w.StatusMsg = "Formatted Prefab JSON."
	w.StatusIsErr = false
	return nil
}

// SyncJSONFromSpec serializes the structured Spec inspector edits back into RawJSON.
func (w *PrefabEditorWindow) SyncJSONFromSpec() {
	if w == nil {
		return
	}
	if w.Doc.Spec.Components == nil {
		w.Doc.Spec.Components = []PrefabComponentData{}
	}
	pretty, err := json.MarshalIndent(w.Doc.Spec, "", "  ")
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return
	}
	w.Doc.RawJSON = string(pretty) + "\n"
	w.Dirty = w.Doc.RawJSON != w.OriginalDoc.RawJSON
	if err := w.Validate(); err == nil {
		w.StatusMsg = "Updated prefab (unsaved changes)"
	}
}

// SyncSpecFromJSON parses RawJSON into Spec when RawJSON is edited directly.
func (w *PrefabEditorWindow) SyncSpecFromJSON() {
	if w == nil {
		return
	}
	var spec PrefabSpec
	if err := json.Unmarshal([]byte(w.Doc.RawJSON), &spec); err == nil {
		w.Doc.Spec = spec
	}
	w.Dirty = w.Doc.RawJSON != w.OriginalDoc.RawJSON
	if err := w.Validate(); err == nil {
		w.StatusMsg = "JSON valid (unsaved changes)"
	}
}

// Validate runs backend schema validation without writing to disk.
func (w *PrefabEditorWindow) Validate() error {
	if w == nil || w.Backend == nil {
		return nil
	}
	if err := w.Backend.ValidatePrefab(w.Doc); err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.StatusIsErr = false
	return nil
}

// Save commits the current prefab document to disk and refreshes the list.
func (w *PrefabEditorWindow) Save() error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prefab editor backend is nil")
	}
	updated, err := w.Backend.CommitPrefab(w.Doc)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.setLoadedDoc(updated, fmt.Sprintf("Saved %s", updated.FilePath))
	_ = w.RefreshList()
	return nil
}

// Spawn instantiates the currently selected/open prefab into the scene.
func (w *PrefabEditorWindow) Spawn() (string, error) {
	if w == nil || w.Backend == nil {
		return "", fmt.Errorf("engine: prefab editor backend is nil")
	}
	target := w.Doc.FilePath
	if target == "" {
		target = w.Doc.Name
	}
	if strings.TrimSpace(target) == "" {
		err := fmt.Errorf("no prefab selected to spawn")
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return "", err
	}
	spawnedID, err := w.Backend.SpawnPrefab(target)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return "", err
	}
	w.StatusMsg = fmt.Sprintf("Spawned %q from %s", spawnedID, w.Doc.Name)
	w.StatusIsErr = false
	return spawnedID, nil
}

// Delete deletes the current prefab from disk and refreshes the list.
func (w *PrefabEditorWindow) Delete() error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prefab editor backend is nil")
	}
	target := w.Doc.FilePath
	if target == "" {
		target = w.Doc.Name
	}
	if err := w.Backend.DeletePrefab(target); err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.Doc = PrefabDocument{}
	w.OriginalDoc = PrefabDocument{}
	w.Dirty = false
	w.StatusMsg = fmt.Sprintf("Deleted %s", target)
	w.StatusIsErr = false
	_ = w.RefreshList()
	return nil
}

// SaveSelected snapshots the currently picked GameObject into a prefab named w.NewName (or object name).
func (w *PrefabEditorWindow) SaveSelected() error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prefab editor backend is nil")
	}
	doc, err := w.Backend.CreatePrefabFromSelected(w.NewName)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.setLoadedDoc(doc, fmt.Sprintf("Saved selected object as %s", doc.FilePath))
	_ = w.RefreshList()
	return nil
}

func (w *PrefabEditorWindow) setLoadedDoc(doc PrefabDocument, status string) {
	w.Doc = doc
	w.OriginalDoc = doc
	w.Dirty = false
	w.StatusMsg = status
	w.StatusIsErr = false
	if w.Backend != nil {
		w.lastRevision = w.Backend.Revision()
	}
}

// SyncFromBackend synchronizes window state when Lua/console commands open, close, or save prefabs.
func (w *PrefabEditorWindow) SyncFromBackend() {
	if w == nil || w.Backend == nil {
		return
	}
	rev := w.Backend.Revision()
	if rev == w.lastRevision {
		return
	}
	w.lastRevision = rev
	if !w.Backend.IsPrefabEditorOpen() {
		w.IsOpen = false
		return
	}
	w.IsOpen = true
	doc := w.Backend.GetActivePrefabDocument()
	if doc.FilePath != "" || doc.RawJSON != "" {
		w.Doc = doc
		w.OriginalDoc = doc
		w.Dirty = false
		w.StatusMsg = fmt.Sprintf("Viewing %s", doc.FilePath)
		w.StatusIsErr = false
	}
	_ = w.RefreshList()
}

// Draw renders the Dear ImGui Prefab Manager/Editor window when open.
func (w *PrefabEditorWindow) Draw() {
	if w == nil {
		return
	}
	w.SyncFromBackend()
	if !w.IsOpen {
		return
	}
	if !w.listLoaded {
		_ = w.RefreshList()
	}

	title := w.Title
	if title == "" {
		title = "Prefab Manager"
	}
	if w.Doc.Name != "" {
		title = fmt.Sprintf("Prefab Manager: %s###PrefabManagerWindow", w.Doc.Name)
	} else {
		title = "Prefab Manager###PrefabManagerWindow"
	}

	open := true
	imgui.SetNextWindowSizeV(imgui.NewVec2(840, 560), imgui.CondFirstUseEver)
	if imgui.BeginV(title, &open, imgui.WindowFlagsNone) {
		avail := imgui.ContentRegionAvail()
		leftW := float32(250)
		if avail.X < 560 {
			leftW = avail.X * 0.35
		}

		// Left browser pane: Search + New + Save Selected + Prefab list
		if imgui.BeginChildStrV("##PrefabBrowserPane", imgui.NewVec2(leftW, 0), imgui.ChildFlagsBorders, imgui.WindowFlagsNone) {
			imgui.TextUnformatted("Prefabs")
			imgui.SetNextItemWidth(-1)
			if imgui.InputTextWithHint("##PrefabSearch", "Search prefabs...", &w.SearchQuery, imgui.InputTextFlagsNone, nil) {
				_ = w.RefreshList()
			}

			imgui.SetNextItemWidth(-1)
			imgui.InputTextWithHint("##PrefabNewName", "New prefab name...", &w.NewName, imgui.InputTextFlagsNone, nil)
			if imgui.Button("New") {
				target := strings.TrimSpace(w.NewName)
				if target == "" {
					target = "NewPrefab"
				}
				_ = w.SelectPrefab(target)
				_ = w.RefreshList()
			}
			imgui.SameLine()
			if imgui.Button("Refresh") {
				_ = w.RefreshList()
			}
			if selID, selName, hasSel := w.Backend.HasSelectedObject(); hasSel {
				if imgui.Button(fmt.Sprintf("Save Selected (%s)", selName)) {
					_ = w.SaveSelected()
				}
				_ = selID
			}
			imgui.Separator()

			if imgui.BeginChildStrV("##PrefabListScroll", imgui.NewVec2(0, 0), imgui.ChildFlagsNone, imgui.WindowFlagsNone) {
				for _, p := range w.Prefabs {
					selected := p.FilePath != "" && p.FilePath == w.Doc.FilePath
					label := fmt.Sprintf("%s (%d)", p.Name, len(p.ComponentNames))
					if imgui.SelectableBoolV(label, selected, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
						_ = w.SelectPrefab(p.FilePath)
					}
					if imgui.IsItemHovered() && len(p.ComponentNames) > 0 {
						imgui.SetTooltip(fmt.Sprintf("%s\nComponents: %s", p.RelativePath, strings.Join(p.ComponentNames, ", ")))
					}
				}
				if len(w.Prefabs) == 0 {
					imgui.TextDisabled("(no matching prefabs)")
				}
			}
			imgui.EndChild()
		}
		imgui.EndChild()

		imgui.SameLine()

		// Right view/modify pane
		if imgui.BeginChildStrV("##PrefabDetailPane", imgui.NewVec2(0, 0), imgui.ChildFlagsBorders, imgui.WindowFlagsNone) {
			if w.Doc.FilePath == "" && w.Doc.RawJSON == "" {
				imgui.TextDisabled("Select a prefab from the list, click 'New', or save the selected object.")
			} else {
				imgui.TextColored(imgui.NewVec4(0.7, 0.85, 1.0, 1.0), fmt.Sprintf("File: %s", w.Doc.FilePath))
				if w.Dirty {
					imgui.SameLine()
					imgui.TextColored(imgui.NewVec4(1.0, 0.85, 0.35, 1.0), " (modified)")
				}

				footerH := imgui.FrameHeightWithSpacing()*2.2 + 12
				bodyH := imgui.ContentRegionAvail().Y - footerH
				if bodyH < 180 {
					bodyH = 180
				}

				if imgui.BeginTabBar("##PrefabTabs") {
					if imgui.BeginTabItem("Inspector (View / Modify)") {
						if imgui.BeginChildStrV("##PrefabInspectorScroll", imgui.NewVec2(0, bodyH-30), imgui.ChildFlagsNone, imgui.WindowFlagsNone) {
							w.drawInspectorPane()
						}
						imgui.EndChild()
						imgui.EndTabItem()
					}
					if imgui.BeginTabItem("JSON") {
						if imgui.InputTextMultiline("##PrefabRawJSON", &w.Doc.RawJSON, imgui.NewVec2(-1, bodyH-30), imgui.InputTextFlagsAllowTabInput, nil) {
							w.SyncSpecFromJSON()
						}
						imgui.EndTabItem()
					}
					imgui.EndTabBar()
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
				if imgui.Button("Save") {
					_ = w.Save()
				}
				imgui.SameLine()
				if imgui.Button("Spawn") {
					_, _ = w.Spawn()
				}
				imgui.SameLine()
				if imgui.Button("Delete") {
					_ = w.Delete()
				}
			}
		}
		imgui.EndChild()
	}
	imgui.End()

	if !open {
		w.Close()
	}
}

func (w *PrefabEditorWindow) drawInspectorPane() {
	changed := false
	if imgui.InputTextWithHint("Name", "GameObject name", &w.Doc.Spec.Name, imgui.InputTextFlagsNone, nil) {
		changed = true
	}
	if imgui.Checkbox("Active", &w.Doc.Spec.Active) {
		changed = true
	}
	if len(w.Doc.Spec.Children) > 0 {
		imgui.TextDisabled(fmt.Sprintf("Children: %d", len(w.Doc.Spec.Children)))
	}

	imgui.Separator()
	imgui.TextUnformatted(fmt.Sprintf("Components (%d)", len(w.Doc.Spec.Components)))

	removeIdx := -1
	for i := range w.Doc.Spec.Components {
		comp := &w.Doc.Spec.Components[i]
		label := comp.Name
		if label == "" {
			label = comp.Type
		}
		if label == "" {
			label = fmt.Sprintf("Component_%d", i)
		}
		header := fmt.Sprintf("%s##comp_%d", label, i)
		if imgui.CollapsingHeaderBoolPtrV(header, nil, imgui.TreeNodeFlagsDefaultOpen) {
			imgui.PushIDInt(int32(i))
			if drawComponentDetailsEditor(comp) {
				changed = true
			}
			if imgui.SmallButton("Remove Component") {
				removeIdx = i
			}
			imgui.PopID()
		}
	}
	if removeIdx >= 0 && removeIdx < len(w.Doc.Spec.Components) {
		w.Doc.Spec.Components = append(w.Doc.Spec.Components[:removeIdx], w.Doc.Spec.Components[removeIdx+1:]...)
		changed = true
	}

	imgui.Separator()
	availComps := w.Backend.AvailableComponentNames()
	if len(availComps) > 0 {
		if w.addCompIdx < 0 || w.addCompIdx >= len(availComps) {
			w.addCompIdx = 0
		}
		if imgui.BeginCombo("##AddCompCombo", availComps[w.addCompIdx]) {
			for idx, name := range availComps {
				isSel := idx == w.addCompIdx
				if imgui.SelectableBoolV(name, isSel, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
					w.addCompIdx = idx
				}
			}
			imgui.EndCombo()
		}
		imgui.SameLine()
		if imgui.Button("Add Component") {
			compName := availComps[w.addCompIdx]
			w.Doc.Spec.Components = append(w.Doc.Spec.Components, defaultPrefabComponentSpec(compName))
			changed = true
		}
	}

	if changed {
		w.SyncJSONFromSpec()
	}
}

func drawComponentDetailsEditor(comp *PrefabComponentData) bool {
	if comp.Details == nil {
		comp.Details = make(map[string]any)
	}
	changed := false
	keys := make([]string, 0, len(comp.Details))
	for k := range comp.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		val := comp.Details[k]
		switch tv := val.(type) {
		case bool:
			b := tv
			if imgui.Checkbox(k, &b) {
				comp.Details[k] = b
				changed = true
			}
		case float64:
			f := float32(tv)
			if imgui.DragFloatV(k, &f, 0.1, 0, 0, "%.3f", imgui.SliderFlagsNone) {
				comp.Details[k] = float64(f)
				changed = true
			}
		case string:
			s := tv
			if imgui.InputTextWithHint(k, "", &s, imgui.InputTextFlagsNone, nil) {
				comp.Details[k] = s
				changed = true
			}
		case map[string]any:
			if hasXYZ(tv) {
				vec := [3]float32{asFloat32(tv["x"]), asFloat32(tv["y"]), asFloat32(tv["z"])}
				if imgui.DragFloat3V(k, &vec, 0.1, 0, 0, "%.3f", imgui.SliderFlagsNone) {
					tv["x"] = float64(vec[0])
					tv["y"] = float64(vec[1])
					tv["z"] = float64(vec[2])
					changed = true
				}
			} else {
				raw, _ := json.Marshal(tv)
				imgui.TextDisabled(fmt.Sprintf("%s: %s", k, string(raw)))
			}
		default:
			raw, _ := json.Marshal(tv)
			imgui.TextDisabled(fmt.Sprintf("%s: %s", k, string(raw)))
		}
	}
	return changed
}

func hasXYZ(m map[string]any) bool {
	_, hx := m["x"]
	_, hy := m["y"]
	_, hz := m["z"]
	return hx && hy && hz
}

func asFloat32(v any) float32 {
	switch n := v.(type) {
	case float64:
		return float32(n)
	case float32:
		return n
	case int:
		return float32(n)
	default:
		return 0
	}
}

func defaultPrefabComponentSpec(compName string) PrefabComponentData {
	switch strings.ToLower(strings.TrimSpace(compName)) {
	case "transform":
		return PrefabComponentData{
			Name: "Transform",
			Type: "*engine.TransformComponent",
			Details: map[string]any{
				"position": map[string]any{"x": 0.0, "y": 0.0, "z": 0.0},
				"rotation": map[string]any{"x": 0.0, "y": 0.0, "z": 0.0},
				"scale":    map[string]any{"x": 1.0, "y": 1.0, "z": 1.0},
			},
		}
	case "mesh":
		return PrefabComponentData{
			Name: "Mesh",
			Type: "*engine.MeshComponent",
			Details: map[string]any{
				"file_path": "",
			},
		}
	case "meshrenderable":
		return PrefabComponentData{
			Name: "MeshRenderable",
			Type: "*engine.MeshRenderable",
			Details: map[string]any{
				"visible":   true,
				"wireframe": false,
			},
		}
	case "renderable":
		return PrefabComponentData{
			Name: "Renderable",
			Type: "*engine.Renderable",
			Details: map[string]any{
				"mesh_id":     "cube",
				"material_id": "default",
				"visible":     true,
				"outlined":    false,
			},
		}
	case "interactivecomponent":
		return PrefabComponentData{
			Name: "InteractiveComponent",
			Type: "*spatial.InteractiveComponent",
			Details: map[string]any{
				"display_name": "Prop",
				"enabled":      true,
			},
		}
	case "fracturecomponent":
		return PrefabComponentData{
			Name: "FractureComponent",
			Type: "*spatial.FractureComponent",
			Details: map[string]any{
				"health":      15.0,
				"carry_hands": 1.0,
				"broken":      false,
			},
		}
	default:
		return PrefabComponentData{
			Name:    compName,
			Details: map[string]any{},
		}
	}
}
