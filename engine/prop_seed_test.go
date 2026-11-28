package engine

import "testing"

// TestResolve_SeededSelectionIsDeterministic verifies that the same seed picks
// the same option from a random-mode pool across repeated resolutions.
func TestResolve_SeededSelectionIsDeterministic(t *testing.T) {
	prop := &Prop{
		ID: "mug",
		Root: &Node{
			ID: "root",
			TypePool: &Pool{
				Name: "type",
				Mode: PoolModeRandom,
				Options: []PoolOption{
					{ID: "ceramic", Payload: OptionPayload{Model: "ceramic.gltf"}},
					{ID: "glass", Payload: OptionPayload{Model: "glass.gltf"}},
					{ID: "metal", Payload: OptionPayload{Model: "metal.gltf"}},
				},
			},
		},
	}

	first, err := Resolve(prop, ResolveOptions{Seed: 42})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	second, err := Resolve(prop, ResolveOptions{Seed: 42})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(first.Leaves) != 1 || len(second.Leaves) != 1 {
		t.Fatalf("expected one leaf each, got %d and %d", len(first.Leaves), len(second.Leaves))
	}
	if first.Leaves[0].Model != second.Leaves[0].Model {
		t.Fatalf("same seed gave different models: %q vs %q", first.Leaves[0].Model, second.Leaves[0].Model)
	}
}
