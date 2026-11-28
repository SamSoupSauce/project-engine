package engine

import "testing"

func TestClonePropIsDeepCopy(t *testing.T) {
	source := testPropTree()
	clone := CloneProp(source, "mug_v2")
	if clone == nil {
		t.Fatal("CloneProp returned nil")
	}
	if clone.ID != "mug_v2" {
		t.Fatalf("expected id mug_v2, got %q", clone.ID)
	}
	if clone.Root == source.Root {
		t.Fatal("clone shares root pointer with source")
	}
	// Mutate the clone's nested tree; source must be unaffected.
	clone.Root.Children[0].Children[0].ID = "handle_v2"
	if source.Root.Children[0].Children[0].ID != "handle" {
		t.Fatal("source was mutated through clone")
	}
	if err := SetOption(clone.Root.Children[0].TypePool, "glass", "Glass", OptionPayload{Model: "glass.gltf"}); err != nil {
		t.Fatalf("SetOption on clone: %v", err)
	}
	if len(source.Root.Children[0].TypePool.Options) != 1 {
		t.Fatal("source options mutated through clone")
	}
}
