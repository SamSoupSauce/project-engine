package engine

import "testing"

type editingPropBackend struct {
	*fakePropBackend
	selected string
	added    string
	removed  string
	pinned   string
}

func (e *editingPropBackend) AddOption(id, path, pool, option, model string) error {
	e.added = option
	return nil
}
func (e *editingPropBackend) RemoveOption(id, path, pool, option string) error {
	e.removed = option
	return nil
}
func (e *editingPropBackend) PinSelection(id, path, pool, option string) error {
	e.pinned = option
	return nil
}
func (e *editingPropBackend) PropTree(id string) ([]PropTreeNode, error) {
	return []PropTreeNode{
		{ID: "root", PoolName: "type", OptionCount: 1, Depth: 0},
		{ID: "body", PoolName: "style", OptionCount: 2, Depth: 1},
	}, nil
}

func TestPropBrowserTreeEditing(t *testing.T) {
	backend := &editingPropBackend{fakePropBackend: &fakePropBackend{props: []PropBrowserSummary{{ID: "mug"}}}}
	w := NewPropBrowserWindow(backend)
	if err := w.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	w.SelectedID = "mug"
	nodes, err := w.LoadTree("mug")
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	if len(nodes) != 2 || nodes[1].ID != "body" {
		t.Fatalf("unexpected tree: %+v", nodes)
	}
	if err := w.AddOption("mug", "root", "type", "metal", "metal.gltf"); err != nil {
		t.Fatalf("AddOption: %v", err)
	}
	if backend.added != "metal" {
		t.Fatalf("backend did not receive add: %q", backend.added)
	}
	if err := w.RemoveOption("mug", "root", "type", "metal"); err != nil {
		t.Fatalf("RemoveOption: %v", err)
	}
	if backend.removed != "metal" {
		t.Fatalf("backend did not receive remove: %q", backend.removed)
	}
	if err := w.PinSelection("mug", "root", "type", "glass"); err != nil {
		t.Fatalf("PinSelection: %v", err)
	}
	if backend.pinned != "glass" {
		t.Fatalf("backend did not receive pin: %q", backend.pinned)
	}
}
