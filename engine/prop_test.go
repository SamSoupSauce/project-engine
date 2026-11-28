package engine

import "testing"

// TestResolve_SelectsExplicitNodeType verifies that pinning an option in a node's
// type pool resolves that node to a concrete model leaf.
func TestResolve_SelectsExplicitNodeType(t *testing.T) {
	prop := &Prop{
		ID: "mug",
		Root: &Node{
			ID: "root",
			TypePool: &Pool{
				Name: "type",
				Mode: PoolModeExplicit,
				Options: []PoolOption{
					{ID: "ceramic", Label: "Ceramic", Payload: OptionPayload{Model: "assets/models/props/mug_ceramic_a.gltf"}},
					{ID: "glass", Label: "Glass", Payload: OptionPayload{Model: "assets/models/props/mug_glass_a.gltf"}},
				},
			},
		},
	}

	resolved, err := Resolve(prop, ResolveOptions{
		Selections: map[string]string{"root": "ceramic"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved == nil {
		t.Fatal("Resolve returned nil result")
	}
	if len(resolved.Leaves) != 1 {
		t.Fatalf("expected 1 resolved leaf, got %d", len(resolved.Leaves))
	}
	if got := resolved.Leaves[0].Model; got != "assets/models/props/mug_ceramic_a.gltf" {
		t.Fatalf("expected ceramic model, got %q", got)
	}
	if resolved.Leaves[0].NodeID != "root" {
		t.Fatalf("expected leaf node id 'root', got %q", resolved.Leaves[0].NodeID)
	}
}
