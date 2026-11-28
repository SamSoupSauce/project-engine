package luabridge

import (
	"path/filepath"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestPropLuaAPI(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()
	svc := b.PropEditor()
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)
	svc.registerPropLuaAPIs()

	L := b.L
	if err := L.DoString(`
		local list = props.list()
		assert(#list == 1, "expected 1 prop")
		assert(list[1].id == "mug", "expected mug id")

		local res = props.resolve("mug", { root = "ceramic", style = "classic" })
		assert(#res.leaves == 1, "expected 1 leaf")
		assert(res.leaves[1].model == "mug_classic.gltf", "wrong model")

		local errs = props.validate("mug")
		assert(#errs == 0, "expected valid prop")
	`); err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
	_ = lua.LNil
}

func TestPropShellCommands(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()
	svc := b.PropEditor()
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)
	svc.registerPropLuaAPIs()

	out, err := b.ExecuteWithContext(ExecutionContext{Sender: "test"}, "props")
	if err != nil {
		t.Fatalf("props command: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty props output")
	}
	out, err = b.ExecuteWithContext(ExecutionContext{Sender: "test"}, "prop_resolve mug")
	if err != nil {
		t.Fatalf("prop resolve command: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty resolve output")
	}
}
