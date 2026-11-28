package luabridge

import (
	"path/filepath"
	"testing"
)

func TestPropLuaMutationAPI(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()
	svc := b.PropEditor()
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)

	if err := b.L.DoString(`
		local p = props.get("mug")
		assert(p.id == "mug", "get failed")

		props.select("mug", "root", "type", "ceramic")
		local r = props.resolve("mug", {})
		assert(#r.leaves > 0, "resolve after select failed")

		props.clone("mug", "mug_v2")
		local list = props.list()
		assert(#list == 2, "expected 2 props after clone, got " .. #list)

		props.save("mug_v2")
		props.delete("mug_v2")
		list = props.list()
		assert(#list == 1, "expected 1 prop after delete, got " .. #list)
	`); err != nil {
		t.Fatalf("Lua mutation script failed: %v", err)
	}
}

func TestPropMutationShellCommands(t *testing.T) {
	b := NewLuaBridge()
	defer b.Close()
	svc := b.PropEditor()
	dir := t.TempDir()
	writeTestPropManifest(t, filepath.Join(dir, "mug.prop.json"), &createTestProp)
	svc.SetPropsDir(dir)

	for _, cmd := range []string{"prop_get mug", "prop_select mug root type ceramic", "prop_clone mug mug_v2", "prop_save mug_v2"} {
		out, err := b.ExecuteWithContext(ExecutionContext{Sender: "test"}, cmd)
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if out == "" {
			t.Fatalf("%s produced no output", cmd)
		}
	}
}
