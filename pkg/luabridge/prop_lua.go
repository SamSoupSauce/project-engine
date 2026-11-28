package luabridge

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// registerPropLuaAPIs exposes the `props` Lua table and prop shell commands.
func (s *PropService) registerPropLuaAPIs() {
	if s == nil || s.Bridge == nil {
		return
	}
	L := s.Bridge.L
	tbl := L.NewTable()
	set := func(name string, fn lua.LGFunction) { L.SetField(tbl, name, L.NewFunction(fn)) }

	set("list", func(L *lua.LState) int {
		q := L.OptString(1, "")
		props, err := s.ListProps(q)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		t := L.NewTable()
		for _, p := range props {
			item := L.NewTable()
			L.SetField(item, "id", lua.LString(p.ID))
			L.SetField(item, "path", lua.LString(p.Path))
			L.SetField(item, "name", lua.LString(p.Name))
			t.Append(item)
		}
		L.Push(t)
		return 1
	})
	set("resolve", func(L *lua.LState) int {
		id := L.CheckString(1)
		opts := ResolutionOptions{Selections: map[string]string{}}
		if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTTable {
			sel := L.CheckTable(2)
			sel.ForEach(func(k, v lua.LValue) {
				opts.Selections[k.String()] = v.String()
			})
		}
		res, err := s.ResolveProp(id, opts)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		t := L.NewTable()
		leaves := L.NewTable()
		for _, leaf := range res.Leaves {
			item := L.NewTable()
			L.SetField(item, "node_id", lua.LString(leaf.NodeID))
			L.SetField(item, "model", lua.LString(leaf.Model))
			L.SetField(item, "material", lua.LString(leaf.Material))
			leaves.Append(item)
		}
		L.SetField(t, "leaves", leaves)
		L.Push(t)
		return 1
	})
	set("get", func(L *lua.LState) int {
		id := L.CheckString(1)
		prop, err := s.GetProp(id)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		t := L.NewTable()
		L.SetField(t, "id", lua.LString(prop.ID))
		L.Push(t)
		return 1
	})
	set("select", func(L *lua.LState) int {
		id, path, pool, optionID := L.CheckString(1), L.CheckString(2), L.CheckString(3), L.CheckString(4)
		if err := s.SetSelection(id, path, pool, optionID); err != nil {
			L.RaiseError("%s", err.Error())
		}
		return 0
	})
	set("clone", func(L *lua.LState) int {
		source, newID := L.CheckString(1), L.CheckString(2)
		path, err := s.CloneProp(source, newID)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(lua.LString(path))
		return 1
	})
	set("save", func(L *lua.LState) int {
		id := L.CheckString(1)
		if err := s.SaveProp(id); err != nil {
			L.RaiseError("%s", err.Error())
		}
		return 0
	})
	set("delete", func(L *lua.LState) int {
		id := L.CheckString(1)
		if err := s.DeleteProp(id); err != nil {
			L.RaiseError("%s", err.Error())
		}
		return 0
	})
	set("validate", func(L *lua.LState) int {
		id := L.CheckString(1)
		errs, err := s.ValidateProp(id)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		t := L.NewTable()
		for _, e := range errs {
			t.Append(lua.LString(e))
		}
		L.Push(t)
		return 1
	})
	L.SetGlobal("props", tbl)

	// Shell commands
	s.Bridge.Shell().RegisterNative(ShellCommandDef{
		Name:        "props",
		Description: "Lists prop manifests.",
		Usage:       "props [query]",
		Examples:    []string{"props", "props mug"},
		GoHandler: func(ctx ExecutionContext) (string, error) {
			q := ""
			if len(ctx.Args) > 1 {
				q = ctx.Args[1]
			}
			list, err := s.ListProps(q)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Props (%d):\n", len(list))
			for _, p := range list {
				fmt.Fprintf(&b, "  %s\n", p.ID)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		},
	})
	s.Bridge.Shell().RegisterNative(ShellCommandDef{
		Name:        "prop_resolve",
		Description: "Resolves a prop into concrete leaves.",
		Usage:       "prop_resolve <id>",
		Examples:    []string{"prop_resolve mug"},
		GoHandler: func(ctx ExecutionContext) (string, error) {
			if len(ctx.Args) < 2 {
				return "", fmt.Errorf("prop_resolve requires an id")
			}
			res, err := s.ResolveProp(ctx.Args[1], ResolutionOptions{Selections: map[string]string{}})
			if err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Resolved %s (%d leaves):\n", res.PropID, len(res.Leaves))
			for _, leaf := range res.Leaves {
				fmt.Fprintf(&b, "  %s -> %s\n", leaf.NodeID, leaf.Model)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		},
	})

	registerSimple := func(name, usage, desc string, handler func([]string) (string, error)) {
		s.Bridge.Shell().RegisterNative(ShellCommandDef{
			Name: name, Description: desc, Usage: usage,
			Examples: []string{usage}, GoHandler: func(ctx ExecutionContext) (string, error) {
				return handler(ctx.Args[1:])
			},
		})
	}
	registerSimple("prop_get", "prop_get <id>", "Shows a prop manifest summary.", func(a []string) (string, error) {
		if len(a) < 1 { return "", fmt.Errorf("prop_get requires id") }
		p, err := s.GetProp(a[0])
		if err != nil { return "", err }
		return "Prop: " + p.ID, nil
	})
	registerSimple("prop_select", "prop_select <id> <path> <pool> <option>", "Pins a prop option.", func(a []string) (string, error) {
		if len(a) < 4 { return "", fmt.Errorf("prop_select requires id, path, pool, option") }
		if err := s.SetSelection(a[0], a[1], a[2], a[3]); err != nil { return "", err }
		return "Pinned " + a[0], nil
	})
	registerSimple("prop_clone", "prop_clone <source> <newId>", "Clones a prop.", func(a []string) (string, error) {
		if len(a) < 2 { return "", fmt.Errorf("prop_clone requires source and new id") }
		path, err := s.CloneProp(a[0], a[1])
		if err != nil { return "", err }
		return "Cloned to " + path, nil
	})
	registerSimple("prop_save", "prop_save <id>", "Saves a prop manifest.", func(a []string) (string, error) {
		if len(a) < 1 { return "", fmt.Errorf("prop_save requires id") }
		if err := s.SaveProp(a[0]); err != nil { return "", err }
		return "Saved " + a[0], nil
	})

}
