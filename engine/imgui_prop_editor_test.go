package engine

import "testing"

type fakePropBackend struct {
	open  bool
	props []PropBrowserSummary
}

func (f *fakePropBackend) IsPropBrowserOpen() bool   { return f.open }
func (f *fakePropBackend) SetPropBrowserOpen(v bool) { f.open = v }
func (f *fakePropBackend) ListProps(q string) ([]PropBrowserSummary, error) {
	return f.props, nil
}
func (f *fakePropBackend) ResolveProp(id string, seed int64) (PropBrowserResolution, error) {
	return PropBrowserResolution{PropID: id, LeafCount: 2, Leaves: []PropBrowserLeaf{
		{NodeID: "body", Model: "body_ceramic.gltf"},
		{NodeID: "handle", Model: "handle_loop.gltf"},
	}}, nil
}

func (f *fakePropBackend) PropTree(id string) ([]PropTreeNode, error) { return nil, nil }
func (f *fakePropBackend) AddOption(id, path, pool, option, model string) error { return nil }
func (f *fakePropBackend) RemoveOption(id, path, pool, option string) error { return nil }
func (f *fakePropBackend) PinSelection(id, path, pool, option string) error { return nil }

func TestPropBrowserWindowOpenClose(t *testing.T) {
	backend := &fakePropBackend{}
	w := NewPropBrowserWindow(backend)
	if err := w.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !w.IsOpen || !backend.open {
		t.Fatal("expected window and backend open")
	}
	w.Close()
	if w.IsOpen || backend.open {
		t.Fatal("expected window and backend closed")
	}
}

func TestPropBrowserWindowResolve(t *testing.T) {
	backend := &fakePropBackend{props: []PropBrowserSummary{{ID: "mug"}}}
	w := NewPropBrowserWindow(backend)
	if err := w.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	res, err := w.Resolve("mug", 42)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.LeafCount != 2 || len(res.Leaves) != 2 {
		t.Fatalf("unexpected resolution: %+v", res)
	}
}
