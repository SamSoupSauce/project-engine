package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/AllenDang/cimgui-go/imgui"
)

type mockPrefabEditorBackend struct {
	open         bool
	prefabs      map[string]PrefabDocument
	revision     uint64
	selectedID   string
	selectedName string
	hasSelected  bool
	spawned      []string
	activeDoc    PrefabDocument
}

func newMockPrefabEditorBackend() *mockPrefabEditorBackend {
	cubeSpec := PrefabSpec{
		Name:   "RainbowCube",
		Active: true,
		Components: []PrefabComponentData{
			{
				Name: "Transform",
				Type: "*engine.TransformComponent",
				Details: map[string]any{
					"position": map[string]any{"x": 0.0, "y": 1.0, "z": 0.0},
					"rotation": map[string]any{"x": 0.0, "y": 0.0, "z": 0.0},
					"scale":    map[string]any{"x": 1.0, "y": 1.0, "z": 1.0},
				},
			},
			{
				Name:    "MeshRenderable",
				Type:    "*engine.MeshRenderable",
				Details: map[string]any{"visible": true, "wireframe": false},
			},
		},
	}
	raw, _ := json.MarshalIndent(cubeSpec, "", "  ")
	doc := PrefabDocument{
		Name:        "RainbowCube",
		FilePath:    "prefabs/rainbow_cube.json",
		RawJSON:     string(raw) + "\n",
		ContentHash: "hash001",
		Spec:        cubeSpec,
	}
	return &mockPrefabEditorBackend{
		prefabs: map[string]PrefabDocument{
			doc.FilePath: doc,
		},
		selectedID:   "cube_1",
		selectedName: "CubeObj",
		hasSelected:  true,
	}
}

func (m *mockPrefabEditorBackend) IsPrefabEditorOpen() bool { return m.open }
func (m *mockPrefabEditorBackend) SetPrefabEditorOpen(open bool) {
	m.open = open
	m.revision++
}
func (m *mockPrefabEditorBackend) SearchPrefabs(query string) ([]PrefabSummary, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []PrefabSummary
	for path, doc := range m.prefabs {
		if q != "" && !strings.Contains(strings.ToLower(doc.Name), q) && !strings.Contains(strings.ToLower(path), q) {
			continue
		}
		var names []string
		for _, c := range doc.Spec.Components {
			names = append(names, c.Name)
		}
		out = append(out, PrefabSummary{
			Name:           doc.Name,
			FilePath:       path,
			RelativePath:   strings.TrimPrefix(path, "prefabs/"),
			Active:         doc.Spec.Active,
			ComponentNames: names,
		})
	}
	return out, nil
}
func (m *mockPrefabEditorBackend) OpenPrefab(target string) (PrefabDocument, error) {
	if doc, ok := m.prefabs[target]; ok {
		m.activeDoc = doc
		m.open = true
		m.revision++
		return doc, nil
	}
	for _, doc := range m.prefabs {
		if strings.EqualFold(doc.Name, target) {
			m.activeDoc = doc
			m.open = true
			m.revision++
			return doc, nil
		}
	}
	spec := PrefabSpec{
		Name:   target,
		Active: true,
		Components: []PrefabComponentData{
			defaultPrefabComponentSpec("Transform"),
		},
	}
	raw, _ := json.MarshalIndent(spec, "", "  ")
	doc := PrefabDocument{
		Name:     target,
		FilePath: "prefabs/" + target + ".json",
		RawJSON:  string(raw) + "\n",
		Spec:     spec,
	}
	m.activeDoc = doc
	m.open = true
	m.revision++
	return doc, nil
}
func (m *mockPrefabEditorBackend) ValidatePrefab(doc PrefabDocument) error {
	if !json.Valid([]byte(doc.RawJSON)) {
		return fmt.Errorf("invalid JSON syntax")
	}
	var spec PrefabSpec
	return json.Unmarshal([]byte(doc.RawJSON), &spec)
}
func (m *mockPrefabEditorBackend) CommitPrefab(doc PrefabDocument) (PrefabDocument, error) {
	if err := m.ValidatePrefab(doc); err != nil {
		return doc, err
	}
	var spec PrefabSpec
	_ = json.Unmarshal([]byte(doc.RawJSON), &spec)
	doc.Spec = spec
	doc.Name = spec.Name
	doc.ContentHash = "savedhash"
	m.prefabs[doc.FilePath] = doc
	m.activeDoc = doc
	m.revision++
	return doc, nil
}
func (m *mockPrefabEditorBackend) DeletePrefab(target string) error {
	delete(m.prefabs, target)
	m.revision++
	return nil
}
func (m *mockPrefabEditorBackend) SpawnPrefab(target string) (string, error) {
	id := fmt.Sprintf("spawned_%d", len(m.spawned)+1)
	m.spawned = append(m.spawned, id)
	return id, nil
}
func (m *mockPrefabEditorBackend) CreatePrefabFromSelected(name string) (PrefabDocument, error) {
	if !m.hasSelected {
		return PrefabDocument{}, fmt.Errorf("nothing selected")
	}
	if name == "" {
		name = m.selectedName
	}
	return m.OpenPrefab(name)
}
func (m *mockPrefabEditorBackend) HasSelectedObject() (string, string, bool) {
	return m.selectedID, m.selectedName, m.hasSelected
}
func (m *mockPrefabEditorBackend) AvailableComponentNames() []string {
	return []string{"Transform", "Mesh", "MeshRenderable", "Renderable"}
}
func (m *mockPrefabEditorBackend) GetActivePrefabDocument() PrefabDocument { return m.activeDoc }
func (m *mockPrefabEditorBackend) Revision() uint64                      { return m.revision }

func TestPrefabEditorWindowLifecycleAndHeadlessImGui(t *testing.T) {
	backend := newMockPrefabEditorBackend()
	overlay := NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()
	overlay.SetPrefabEditor(backend)

	if err := overlay.PrefabEditor.Open("RainbowCube"); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	var drawDataValid bool
	overlay.onHeadlessDraw = func(dd *imgui.DrawData) {
		drawDataValid = dd != nil && dd.Valid()
	}

	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	if !overlay.PrefabEditor.IsOpen || !drawDataValid {
		t.Fatalf("expected PrefabEditorWindow open with valid DrawData (open=%v, valid=%v)", overlay.PrefabEditor.IsOpen, drawDataValid)
	}
	if len(overlay.PrefabEditor.Prefabs) != 1 || overlay.PrefabEditor.Doc.Name != "RainbowCube" {
		t.Fatalf("unexpected prefabs list or active doc: %+v", overlay.PrefabEditor.Prefabs)
	}

	// Search filtering
	overlay.PrefabEditor.SearchQuery = "nonexistent"
	_ = overlay.PrefabEditor.RefreshList()
	if len(overlay.PrefabEditor.Prefabs) != 0 {
		t.Fatalf("expected 0 prefabs for query 'nonexistent', got %d", len(overlay.PrefabEditor.Prefabs))
	}
	overlay.PrefabEditor.SearchQuery = "rainbow"
	_ = overlay.PrefabEditor.RefreshList()
	if len(overlay.PrefabEditor.Prefabs) != 1 {
		t.Fatalf("expected 1 prefab for query 'rainbow', got %d", len(overlay.PrefabEditor.Prefabs))
	}

	// Modify Spec -> SyncJSONFromSpec -> Save
	overlay.PrefabEditor.Doc.Spec.Active = false
	overlay.PrefabEditor.SyncJSONFromSpec()
	if !overlay.PrefabEditor.Dirty || !strings.Contains(overlay.PrefabEditor.Doc.RawJSON, `"active": false`) {
		t.Fatalf("expected SyncJSONFromSpec to update RawJSON and Dirty flag")
	}
	if err := overlay.PrefabEditor.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if overlay.PrefabEditor.Dirty || overlay.PrefabEditor.StatusIsErr {
		t.Fatalf("expected clean state after Save, got dirty=%v err=%v msg=%q",
			overlay.PrefabEditor.Dirty, overlay.PrefabEditor.StatusIsErr, overlay.PrefabEditor.StatusMsg)
	}

	// Invalid JSON validation & Revert
	overlay.PrefabEditor.Doc.RawJSON = "{broken"
	if err := overlay.PrefabEditor.FormatJSON(); err == nil || !overlay.PrefabEditor.StatusIsErr {
		t.Fatalf("expected FormatJSON error on malformed JSON")
	}
	overlay.PrefabEditor.Revert()
	if overlay.PrefabEditor.StatusIsErr || !strings.Contains(overlay.PrefabEditor.Doc.RawJSON, "RainbowCube") {
		t.Fatalf("expected Revert to restore saved JSON")
	}

	// Spawn
	id, err := overlay.PrefabEditor.Spawn()
	if err != nil || id != "spawned_1" {
		t.Fatalf("expected Spawn to return spawned_1, got %q err=%v", id, err)
	}

	// SaveSelected
	overlay.PrefabEditor.NewName = "FromSelected"
	if err := overlay.PrefabEditor.SaveSelected(); err != nil {
		t.Fatalf("SaveSelected failed: %v", err)
	}
	if overlay.PrefabEditor.Doc.Name != "FromSelected" {
		t.Fatalf("expected SaveSelected doc Name 'FromSelected', got %q", overlay.PrefabEditor.Doc.Name)
	}

	// Delete
	if err := overlay.PrefabEditor.Delete(); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}
