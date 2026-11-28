package engine

import "testing"

// TestResolve_ResolvesNestedSubPools verifies that selecting a node type can
// reveal a sub-pool of options, which is resolved recursively into child leaves.
func TestResolve_ResolvesNestedSubPools(t *testing.T) {
	prop := &Prop{
		ID: "mug",
		Root: &Node{
			ID: "root",
			TypePool: &Pool{
				Name: "type",
				Mode: PoolModeExplicit,
				Options: []PoolOption{
					{
						ID:    "ceramic",
						Label: "Ceramic",
						Payload: OptionPayload{
							SubPools: []*Pool{
								{
									Name: "style",
									Mode: PoolModeExplicit,
									Options: []PoolOption{
										{ID: "classic", Label: "Classic", Payload: OptionPayload{Model: "mug_ceramic_classic.gltf"}},
										{ID: "tall", Label: "Tall", Payload: OptionPayload{Model: "mug_ceramic_tall.gltf"}},
									},
								},
							},
						},
					},
					{ID: "glass", Label: "Glass", Payload: OptionPayload{Model: "mug_glass.gltf"}},
				},
			},
		},
	}

	resolved, err := Resolve(prop, ResolveOptions{
		Selections: map[string]string{"root": "ceramic", "style": "tall"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved.Leaves) != 1 {
		t.Fatalf("expected 1 leaf (root contributes model via style), got %d", len(resolved.Leaves))
	}
	if got := resolved.Leaves[0].Model; got != "mug_ceramic_tall.gltf" {
		t.Fatalf("expected tall ceramic model, got %q", got)
	}
}
