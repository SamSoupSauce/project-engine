package engine

import "testing"

// TestResolve_CycleDoesNotHang verifies that resolution stops at a cycle
// rather than recursing forever.
func TestResolve_CycleDoesNotHang(t *testing.T) {
	root := &Node{
		ID:       "root",
		TypePool: &Pool{Name: "type", Options: []PoolOption{{ID: "mug", Payload: OptionPayload{Model: "mug.gltf"}}}},
	}
	child := &Node{
		ID:       "child",
		TypePool: &Pool{Name: "style", Options: []PoolOption{{ID: "ceramic", Payload: OptionPayload{Model: "child.gltf"}}}},
	}
	root.Children = []*Node{child}
	child.Children = []*Node{root}

	resolved, err := Resolve(&Prop{ID: "mug", Root: root}, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved.Leaves) != 2 {
		t.Fatalf("expected 2 leaves before cycle cutoff, got %d", len(resolved.Leaves))
	}
}
