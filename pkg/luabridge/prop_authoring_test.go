package luabridge

import (
	"path/filepath"
	"testing"
)

func TestPropServiceAuthoring(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()
	svc := b.PropEditor()
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)

	prop, err := svc.GetProp("mug")
	if err != nil {
		t.Fatalf("GetProp: %v", err)
	}
	if prop.ID != "mug" {
		t.Fatalf("expected mug, got %s", prop.ID)
	}

	if err := svc.SetSelection("mug", "root", "type", "ceramic"); err != nil {
		t.Fatalf("SetSelection: %v", err)
	}
	res, err := svc.ResolveProp("mug", ResolutionOptions{})
	if err != nil {
		t.Fatalf("ResolveProp after pin: %v", err)
	}
	found := false
	for _, leaf := range res.Leaves {
		if leaf.NodeID == "root" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pinned selection not reflected in resolve: %+v", res.Leaves)
	}

	newPath, err := svc.CloneProp("mug", "mug_v2")
	if err != nil {
		t.Fatalf("CloneProp: %v", err)
	}
	if newPath == "" {
		t.Fatal("expected clone path")
	}
	props, err := svc.ListProps("")
	if err != nil {
		t.Fatalf("ListProps after clone: %v", err)
	}
	if len(props) != 2 {
		t.Fatalf("expected 2 props after clone, got %d", len(props))
	}
	if err := svc.SaveProp("mug_v2"); err != nil {
		t.Fatalf("SaveProp: %v", err)
	}
}
