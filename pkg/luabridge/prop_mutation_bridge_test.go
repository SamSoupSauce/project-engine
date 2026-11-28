package luabridge

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestBridgePropMutationEndpoints(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()
	svc := b.PropEditor()
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)
	server := &AgentServer{Bridge: b}

	call := func(method string, p any) (any, *rpcError) {
		b, _ := json.Marshal(p)
		return server.dispatchMethod("agent:test", method, b)
	}

	if _, err := call("engine.set_prop_selection", map[string]string{
		"id": "mug", "path": "root", "pool": "type", "option": "ceramic",
	}); err != nil {
		t.Fatalf("set_prop_selection: %v", err)
	}

	got, err := call("engine.get_prop", map[string]string{"id": "mug"})
	if err != nil {
		t.Fatalf("get_prop: %v", err)
	}
	if got == nil {
		t.Fatal("get_prop returned nil")
	}

	if _, err := call("engine.clone_prop", map[string]string{"source": "mug", "id": "mug_v2"}); err != nil {
		t.Fatalf("clone_prop: %v", err)
	}
	got, err = call("engine.query_props", map[string]string{"query": ""})
	if err != nil {
		t.Fatalf("query_props: %v", err)
	}
	obj := got.(map[string]any)
	if obj["count"].(int) != 2 {
		t.Fatalf("expected 2 props after clone, got %v", obj["count"])
	}

	got, err = call("engine.query_prop_tree", map[string]string{"id": "mug", "path": "root"})
	if err != nil {
		t.Fatalf("query_prop_tree: %v", err)
	}
	if got == nil {
		t.Fatal("query_prop_tree returned nil")
	}
}
