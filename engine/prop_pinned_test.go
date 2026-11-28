package engine

import (
	"path/filepath"
	"testing"
)

func TestPinnedSelectionRoundTrip(t *testing.T) {
	prop := testPropTree()
	if err := SetSelection(prop, "root/body", "style", "ceramic"); err != nil {
		t.Fatalf("SetSelection: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mug.prop.json")
	if err := SavePropManifest(path, prop); err != nil {
		t.Fatalf("SavePropManifest: %v", err)
	}
	loaded, err := LoadPropManifest(path)
	if err != nil {
		t.Fatalf("LoadPropManifest: %v", err)
	}
	if len(loaded.Pinned) == 0 {
		t.Fatal("expected pinned selections to survive round-trip")
	}
	// Resolve with no explicit selections must use the pinned value.
	res, err := Resolve(loaded, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	found := false
	for _, leaf := range res.Leaves {
		if leaf.NodeID == "body" && leaf.Model == "ceramic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pinned selection not applied: %+v", res.Leaves)
	}
}

func TestExplicitSelectionsOverridePinned(t *testing.T) {
	prop := testPropTree()
	if err := SetOption(prop.Root.Children[0].TypePool, "glass", "Glass", OptionPayload{Model: "glass.gltf"}); err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if err := SetSelection(prop, "root/body", "style", "ceramic"); err != nil {
		t.Fatalf("SetSelection: %v", err)
	}
	res, err := Resolve(prop, ResolveOptions{Selections: map[string]string{"body": "glass"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	found := false
	for _, leaf := range res.Leaves {
		if leaf.NodeID == "body" && leaf.Model == "glass.gltf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit selection did not override pinned: %+v", res.Leaves)
	}
}
