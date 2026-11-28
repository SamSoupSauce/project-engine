package luabridge

import (
	"encoding/json"

	"github.com/SamSoupSauce/project-engine/engine"
	"path/filepath"
	"testing"
)

func TestBridgePropEndpoints(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()
	svc := b.PropEditor()
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)

	server := &AgentServer{Bridge: b}
	params, _ := json.Marshal(map[string]string{"query": ""})
	got, rpcErr := server.dispatchMethod("agent:test", "engine.query_props", params)
	if rpcErr != nil {
		t.Fatalf("query_props: %v", rpcErr)
	}
	obj := got.(map[string]any)
	if obj["count"].(int) != 1 {
		t.Fatalf("expected 1 prop, got %v", obj["count"])
	}

	params, _ = json.Marshal(map[string]any{"id": "mug", "selections": map[string]string{"root": "ceramic", "style": "classic"}})
	got, rpcErr = server.dispatchMethod("agent:test", "engine.resolve_prop", params)
	if rpcErr != nil {
		t.Fatalf("resolve_prop: %v", rpcErr)
	}
	res := got.(*engine.ResolvedProp)
	if len(res.Leaves) != 1 || res.Leaves[0].Model != "mug_classic.gltf" {
		t.Fatalf("unexpected resolve result: %+v", res)
	}
}
