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
}
