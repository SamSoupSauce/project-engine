package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPropManifestRoundTrip verifies that a Prop tree survives save/load,
// including nested pools and seeded resolution producing identical leaves.
func TestPropManifestRoundTrip(t *testing.T) {
	original := &Prop{
		ID: "mug",
		Root: &Node{
			ID: "root",
			TypePool: &Pool{
				Name: "type", Mode: PoolModeExplicit,
				Options: []PoolOption{{
					ID: "ceramic", Label: "Ceramic",
					Payload: OptionPayload{
						SubPools: []*Pool{{
							Name: "style", Mode: PoolModeExplicit,
							Options: []PoolOption{{ID: "classic", Payload: OptionPayload{Model: "mug_classic.gltf"}}},
						}},
					},
				}},
			},
		},
	}
	path := filepath.Join(t.TempDir(), "mug.prop.json")
	if err := SavePropManifest(path, original); err != nil {
		t.Fatalf("SavePropManifest: %v", err)
	}
	loaded, err := LoadPropManifest(path)
	if err != nil {
		t.Fatalf("LoadPropManifest: %v", err)
	}
	if loaded.ID != original.ID {
		t.Fatalf("expected id %q, got %q", original.ID, loaded.ID)
	}
	if len(loaded.Root.TypePool.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(loaded.Root.TypePool.Options))
	}
	subs := loaded.Root.TypePool.Options[0].Payload.SubPools
	if len(subs) != 1 || subs[0].Name != "style" {
		t.Fatalf("expected 'style' sub-pool, got %+v", subs)
	}
	res, err := Resolve(loaded, ResolveOptions{Selections: map[string]string{"root": "ceramic", "style": "classic"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Leaves) != 1 || res.Leaves[0].Model != "mug_classic.gltf" {
		t.Fatalf("unexpected resolved leaves: %+v", res.Leaves)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("manifest file missing: %v", err)
	}
}
