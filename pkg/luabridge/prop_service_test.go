package luabridge

import (
	"path/filepath"
	"testing"
)

func newTestPropService(t *testing.T) *PropService {
	t.Helper()
	b := NewLuaBridge()
	t.Cleanup(func() { b.Close() })
	svc := b.PropEditor()
	if svc == nil {
		t.Fatal("PropEditor service not initialized")
	}
	svc.SetPropsDir(filepath.Join("..", "..", "assets", "props"))
	return svc
}

func TestPropServiceDiscovery(t *testing.T) {
	svc := newTestPropService(t)
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)

	props, err := svc.ListProps("")
	if err != nil {
		t.Fatalf("ListProps: %v", err)
	}
	if len(props) != 1 || props[0].ID != "mug" {
		t.Fatalf("expected one 'mug' prop, got %+v", props)
	}
}

func TestPropServiceResolve(t *testing.T) {
	svc := newTestPropService(t)
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)

	resolved, err := svc.ResolveProp("mug", ResolutionOptions{
		Selections: map[string]string{"root": "ceramic", "style": "classic"},
	})
	if err != nil {
		t.Fatalf("ResolveProp: %v", err)
	}
	if len(resolved.Leaves) != 1 || resolved.Leaves[0].Model != "mug_classic.gltf" {
		t.Fatalf("unexpected resolved leaves: %+v", resolved.Leaves)
	}
}

func TestPropServiceValidation(t *testing.T) {
	svc := newTestPropService(t)
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "bad.prop.json"), &badTestProp)
	svc.SetPropsDir(dir)
	errs, err := svc.ValidateProp("bad")
	if err != nil {
		t.Fatalf("ValidateProp: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation errors for bad prop")
	}
}

func TestPropServiceMissingProp(t *testing.T) {
	svc := newTestPropService(t)
	svc.SetPropsDir(t.TempDir())
	if _, err := svc.ResolveProp("missing", ResolutionOptions{}); err == nil {
		t.Fatal("expected error resolving missing prop")
	}
}
