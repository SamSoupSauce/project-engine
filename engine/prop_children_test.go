package engine

import "testing"

// TestResolve_MultipleChildNodesProduceMultipleLeaves verifies that a node tree
// with several sibling nodes resolves each node into its own concrete leaf.
func TestResolve_MultipleChildNodesProduceMultipleLeaves(t *testing.T) {
	prop := &Prop{
		ID: "mug",
		Root: &Node{
			ID: "root",
			TypePool: &Pool{
				Name:    "type",
				Mode:    PoolModeExplicit,
				Options: []PoolOption{{ID: "mug", Payload: OptionPayload{}}},
			},
			Children: []*Node{
				{
					ID: "body",
					TypePool: &Pool{
						Name:    "style",
						Mode:    PoolModeExplicit,
						Options: []PoolOption{{ID: "ceramic", Payload: OptionPayload{Model: "body_ceramic.gltf"}}},
					},
				},
				{
					ID: "handle",
					TypePool: &Pool{
						Name:    "style",
						Mode:    PoolModeExplicit,
						Options: []PoolOption{{ID: "loop", Payload: OptionPayload{Model: "handle_loop.gltf"}}},
					},
				},
			},
		},
	}

	resolved, err := Resolve(prop, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved.Leaves) != 2 {
		t.Fatalf("expected 2 leaves (body + handle), got %d", len(resolved.Leaves))
	}
	models := map[string]bool{}
	for _, leaf := range resolved.Leaves {
		models[leaf.Model] = true
	}
	if !models["body_ceramic.gltf"] || !models["handle_loop.gltf"] {
		t.Fatalf("unexpected resolved models: %v", models)
	}
}
