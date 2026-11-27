package luabridge

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

// ScriptSearchResult describes a Lua script discovered under ScriptsDir.
type ScriptSearchResult struct {
	Name          string
	FilePath      string
	RelativePath  string
	Category      string
	ComponentName string
	Hooks         []string
}

// ComponentSearchResult describes a registered built-in or custom Lua component class.
type ComponentSearchResult struct {
	Name        string
	GoType      string
	Custom      bool
	Description string
	Fields      []string
	Methods     []string
	SourcePath  string
}

func (b *LuaBridge) effectiveScriptsDir() string {
	if b != nil && b.scriptEditor != nil {
		b.scriptEditor.mu.RLock()
		dir := b.scriptEditor.ScriptsDir
		b.scriptEditor.mu.RUnlock()
		if strings.TrimSpace(dir) != "" {
			return dir
		}
	}
	return defaultEditorScriptsDir
}

// SearchScripts scans ScriptsDir for *.lua files matching query across filename, relative path,
// category, component name, and declared hook/function names.
func (b *LuaBridge) SearchScripts(query string) ([]ScriptSearchResult, error) {
	dir := b.effectiveScriptsDir()
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scripts: stat %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scripts: %q is not a directory", dir)
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var results []ScriptSearchResult
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".lua") {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		relSlash := filepath.ToSlash(rel)
		category := ""
		if idx := strings.IndexByte(relSlash, '/'); idx > 0 {
			category = relSlash[:idx]
		}
		stem := stemFromPath(path)

		raw, _ := os.ReadFile(path)
		src := string(raw)
		compName, hooks := extractScriptMetadata(src)

		if q != "" {
			matched := strings.Contains(strings.ToLower(stem), q) ||
				strings.Contains(strings.ToLower(relSlash), q) ||
				strings.Contains(strings.ToLower(category), q) ||
				strings.Contains(strings.ToLower(compName), q) ||
				strings.Contains(strings.ToLower(src), q)
			if !matched {
				return nil
			}
		}

		results = append(results, ScriptSearchResult{
			Name:          stem,
			FilePath:      filepath.Clean(path),
			RelativePath:  relSlash,
			Category:      category,
			ComponentName: compName,
			Hooks:         hooks,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scripts: walk %q: %w", dir, err)
	}
	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].RelativePath) < strings.ToLower(results[j].RelativePath)
	})
	return results, nil
}

func extractScriptMetadata(src string) (componentName string, hooks []string) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	hookSet := make(map[string]bool)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- @component:") {
			componentName = strings.TrimSpace(strings.TrimPrefix(trimmed, "-- @component:"))
		} else if strings.HasPrefix(trimmed, "-- @hooks:") {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- @hooks:"))
			if m, _, err := parseHooksJSONObject(raw); err == nil {
				for h := range m {
					hookSet[h] = true
				}
			}
		} else if strings.HasPrefix(trimmed, "function ") {
			sig := strings.TrimSpace(strings.TrimPrefix(trimmed, "function "))
			if paren := strings.IndexByte(sig, '('); paren > 0 {
				fnName := strings.TrimSpace(sig[:paren])
				if colon := strings.IndexAny(fnName, ":."); colon > 0 {
					fnName = fnName[colon+1:]
				}
				if fnName != "" {
					hookSet[fnName] = true
				}
			}
		}
	}
	for h := range hookSet {
		hooks = append(hooks, h)
	}
	sort.Strings(hooks)
	return componentName, hooks
}

// SearchComponents searches all built-in and custom Lua component classes by query across
// name, GoType, description, fields, and methods.
func (b *LuaBridge) SearchComponents(query string) []ComponentSearchResult {
	q := strings.ToLower(strings.TrimSpace(query))
	seen := make(map[string]bool)
	var results []ComponentSearchResult

	// 1. Built-in documented components
	builtinOrder := []string{
		"Transform", "Mesh", "MeshRenderable", "Renderable", "Renderable2D",
		"RigidBody", "CharacterController", "InteractiveComponent", "FractureComponent", "ScriptComponent",
	}
	for _, name := range builtinOrder {
		doc, ok := componentDocs[name]
		if !ok {
			continue
		}
		seen[strings.ToLower(doc.Name)] = true
		item := ComponentSearchResult{
			Name:        doc.Name,
			GoType:      doc.GoType,
			Custom:      false,
			Description: doc.Description,
			Fields:      append([]string(nil), doc.Fields...),
			Methods:     append([]string(nil), doc.Methods...),
		}
		if matchesComponentQuery(item, q) {
			results = append(results, item)
		}
	}

	// 2. Custom Lua components registered via Component(...)
	if b != nil {
		b.dataMu.RLock()
		customList := make([]*LuaComponentDef, 0, len(b.customComponents))
		for _, def := range b.customComponents {
			if def != nil {
				customList = append(customList, def)
			}
		}
		b.dataMu.RUnlock()

		sort.Slice(customList, func(i, j int) bool {
			return strings.ToLower(customList[i].Name) < strings.ToLower(customList[j].Name)
		})
		for _, def := range customList {
			if seen[strings.ToLower(def.Name)] {
				continue
			}
			seen[strings.ToLower(def.Name)] = true
			methods := collectProtoMethodNames(def.Proto)
			fields := append([]string(nil), def.FieldNames...)
			desc := "Lua custom component"
			if len(def.HookNames) > 0 {
				desc += " (hooks: " + strings.Join(def.HookNames, ", ") + ")"
			}
			item := ComponentSearchResult{
				Name:        def.Name,
				GoType:      "*luabridge.LuaComponent",
				Custom:      true,
				Description: desc,
				Fields:      fields,
				Methods:     methods,
				SourcePath:  def.SourcePath,
			}
			if matchesComponentQuery(item, q) {
				results = append(results, item)
			}
		}
	}
	return results
}

func matchesComponentQuery(item ComponentSearchResult, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(item.Name), q) ||
		strings.Contains(strings.ToLower(item.GoType), q) ||
		strings.Contains(strings.ToLower(item.Description), q) ||
		strings.Contains(strings.ToLower(item.SourcePath), q) {
		return true
	}
	for _, f := range item.Fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	for _, m := range item.Methods {
		if strings.Contains(strings.ToLower(m), q) {
			return true
		}
	}
	return false
}

func (b *LuaBridge) registerSearchAndComponentCommands() {
	// Script search Lua functions
	searchScriptsFn := b.L.NewFunction(func(L *lua.LState) int {
		query := L.OptString(1, "")
		results, err := b.SearchScripts(query)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		out := formatScriptSearchLines(query, results)
		b.EmitConsole(out, LineInfo)

		tbl := L.NewTable()
		for i, item := range results {
			entry := L.NewTable()
			entry.RawSetString("name", lua.LString(item.Name))
			entry.RawSetString("path", lua.LString(item.FilePath))
			entry.RawSetString("relative_path", lua.LString(item.RelativePath))
			entry.RawSetString("category", lua.LString(item.Category))
			entry.RawSetString("component", lua.LString(item.ComponentName))
			hooksTbl := L.NewTable()
			for j, h := range item.Hooks {
				hooksTbl.RawSetInt(j+1, lua.LString(h))
			}
			entry.RawSetString("hooks", hooksTbl)
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		return 1
	})
	b.L.SetGlobal("search_scripts", searchScriptsFn)
	b.L.SetGlobal("find_scripts", searchScriptsFn)
	b.L.SetGlobal("list_scripts", b.L.NewFunction(func(L *lua.LState) int {
		results, err := b.SearchScripts("")
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.EmitConsole(formatScriptSearchLines("", results), LineInfo)
		tbl := L.NewTable()
		for i, item := range results {
			tbl.RawSetInt(i+1, lua.LString(item.RelativePath))
		}
		L.Push(tbl)
		return 1
	}))

	// Script CLI commands
	b.RegisterCommand("scripts", func(args []string) (string, error) {
		q := strings.TrimSpace(strings.Join(args, " "))
		results, err := b.SearchScripts(q)
		if err != nil {
			return "", err
		}
		out := formatScriptSearchLines(q, results)
		b.EmitConsole(out, LineInfo)
		return out, nil
	})
	b.RegisterCommand("script", func(args []string) (string, error) {
		if len(args) == 0 {
			results, err := b.SearchScripts("")
			if err != nil {
				return "", err
			}
			out := formatScriptSearchLines("", results)
			b.EmitConsole(out, LineInfo)
			return out, nil
		}
		sub := strings.ToLower(args[0])
		rest := args[1:]
		switch sub {
		case "search", "find", "list":
			q := strings.TrimSpace(strings.Join(rest, " "))
			results, err := b.SearchScripts(q)
			if err != nil {
				return "", err
			}
			out := formatScriptSearchLines(q, results)
			b.EmitConsole(out, LineInfo)
			return out, nil
		case "edit", "open":
			if len(rest) == 0 {
				return "", fmt.Errorf("usage: script edit <nameOrPath>")
			}
			doc, err := b.scriptEditor.OpenScript(strings.Join(rest, " "))
			if err != nil {
				return "", err
			}
			msg := fmt.Sprintf("[ScriptEditor] Opened %s", doc.FilePath)
			b.EmitConsole(msg, LineSystem)
			return msg, nil
		default:
			q := strings.TrimSpace(strings.Join(args, " "))
			results, err := b.SearchScripts(q)
			if err != nil {
				return "", err
			}
			out := formatScriptSearchLines(q, results)
			b.EmitConsole(out, LineInfo)
			return out, nil
		}
	})

	// Component search Lua functions
	searchCompsFn := b.L.NewFunction(func(L *lua.LState) int {
		query := L.OptString(1, "")
		results := b.SearchComponents(query)
		out := formatComponentSearchLines(query, results, b.GetPickedObject())
		b.EmitConsole(out, LineInfo)

		tbl := L.NewTable()
		for i, item := range results {
			entry := L.NewTable()
			entry.RawSetString("name", lua.LString(item.Name))
			entry.RawSetString("go_type", lua.LString(item.GoType))
			entry.RawSetString("custom", lua.LBool(item.Custom))
			entry.RawSetString("description", lua.LString(item.Description))
			if item.SourcePath != "" {
				entry.RawSetString("source_path", lua.LString(item.SourcePath))
			}
			fTbl := L.NewTable()
			for j, f := range item.Fields {
				fTbl.RawSetInt(j+1, lua.LString(f))
			}
			entry.RawSetString("fields", fTbl)
			mTbl := L.NewTable()
			for j, m := range item.Methods {
				mTbl.RawSetInt(j+1, lua.LString(m))
			}
			entry.RawSetString("methods", mTbl)
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		return 1
	})
	b.L.SetGlobal("search_components", searchCompsFn)
	b.L.SetGlobal("find_components", searchCompsFn)
	b.L.SetGlobal("list_components", b.L.NewFunction(func(L *lua.LState) int {
		var target *engine.GameObject
		if L.GetTop() >= 1 && L.Get(1) != lua.LNil {
			if s, ok := L.Get(1).(lua.LString); ok {
				target = b.findSceneOrRegisteredObject(string(s))
			} else {
				target = checkGameObject(L, 1)
			}
		} else {
			target = b.GetPickedObject()
		}
		if target != nil {
			out := formatObjectComponentsList(target)
			b.EmitConsole(out, LineInfo)
			tbl := L.NewTable()
			for i, c := range target.Components {
				if c != nil {
					tbl.RawSetInt(i+1, lua.LString(componentCanonicalName(c)))
				}
			}
			L.Push(tbl)
			return 1
		}
		results := b.SearchComponents("")
		b.EmitConsole(formatComponentSearchLines("", results, nil), LineInfo)
		tbl := L.NewTable()
		for i, item := range results {
			tbl.RawSetInt(i+1, lua.LString(item.Name))
		}
		L.Push(tbl)
		return 1
	}))

	// Easy Lua `comp(...)` wrapper forwarding to the CLI command handler.
	b.L.SetGlobal("comp", b.L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		args := make([]string, 0, top)
		for i := 1; i <= top; i++ {
			v := L.Get(i)
			if ud, ok := v.(*lua.LUserData); ok && ud != nil {
				if obj, ok := ud.Value.(*engine.GameObject); ok && obj != nil {
					args = append(args, obj.ID)
					continue
				}
				if cc, ok := ud.Value.(*ComponentClass); ok && cc != nil {
					args = append(args, cc.Name)
					continue
				}
			}
			args = append(args, v.String())
		}
		out, err := b.handleComponentCommand(args)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(lua.LString(out))
		return 1
	}))

	compCmd := func(args []string) (string, error) {
		return b.handleComponentCommand(args)
	}
	b.RegisterCommand("comp", compCmd)
	b.RegisterCommand("component", compCmd)
	b.RegisterCommand("components", compCmd)
}

func formatScriptSearchLines(query string, results []ScriptSearchResult) string {
	if len(results) == 0 {
		if query == "" {
			return "(no Lua scripts found in scripts/)"
		}
		return fmt.Sprintf("(no scripts matching %q)", query)
	}
	var sb strings.Builder
	header := fmt.Sprintf("Scripts (%d):", len(results))
	if query != "" {
		header = fmt.Sprintf("Scripts matching %q (%d):", query, len(results))
	}
	sb.WriteString(header + "\n")
	for _, r := range results {
		meta := ""
		if r.ComponentName != "" {
			meta += " component=" + r.ComponentName
		}
		if len(r.Hooks) > 0 {
			meta += " hooks=[" + strings.Join(r.Hooks, ", ") + "]"
		}
		sb.WriteString(fmt.Sprintf("  %s (%s)%s\n", r.Name, r.RelativePath, meta))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatComponentSearchLines(query string, results []ComponentSearchResult, selected *engine.GameObject) string {
	var sb strings.Builder
	if len(results) == 0 {
		sb.WriteString(fmt.Sprintf("(no components matching %q)", query))
	} else {
		header := fmt.Sprintf("Component Classes (%d):", len(results))
		if query != "" {
			header = fmt.Sprintf("Components matching %q (%d):", query, len(results))
		}
		sb.WriteString(header + "\n")
		for _, r := range results {
			tag := "builtin"
			if r.Custom {
				tag = "custom"
			}
			sb.WriteString(fmt.Sprintf("  %s [%s] (%s) - %s\n", r.Name, tag, r.GoType, r.Description))
		}
	}
	if selected != nil {
		var attached []string
		q := strings.ToLower(strings.TrimSpace(query))
		for _, c := range selected.Components {
			if c == nil {
				continue
			}
			cn := componentCanonicalName(c)
			if q == "" || strings.Contains(strings.ToLower(cn), q) {
				summary := summarizeComponentShort(c)
				if summary != "" {
					attached = append(attached, fmt.Sprintf("%s (%s)", cn, summary))
				} else {
					attached = append(attached, cn)
				}
			}
		}
		if len(attached) > 0 {
			sb.WriteString(fmt.Sprintf("On selected (%s): %s\n", selected.Name, strings.Join(attached, "; ")))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatObjectComponentsList(obj *engine.GameObject) string {
	if obj == nil {
		return "(nil GameObject)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Components on %s (%s) [%d]:\n", obj.Name, obj.ID, len(obj.Components)))
	if len(obj.Components) == 0 {
		sb.WriteString("  (none)")
		return sb.String()
	}
	for i, c := range obj.Components {
		if c == nil {
			continue
		}
		canon := componentCanonicalName(c)
		goType := reflect.TypeOf(c).String()
		summary := summarizeComponentShort(c)
		if summary != "" {
			sb.WriteString(fmt.Sprintf("  %d. %s (%s) - %s\n", i+1, canon, goType, summary))
		} else {
			sb.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1, canon, goType))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (b *LuaBridge) handleComponentCommand(args []string) (string, error) {
	if len(args) == 0 {
		results := b.SearchComponents("")
		out := formatComponentSearchLines("", results, b.GetPickedObject())
		b.EmitConsole(out, LineInfo)
		return out, nil
	}

	sub := strings.ToLower(args[0])
	rest := args[1:]

	switch sub {
	case "search", "find":
		q := strings.TrimSpace(strings.Join(rest, " "))
		results := b.SearchComponents(q)
		out := formatComponentSearchLines(q, results, b.GetPickedObject())
		b.EmitConsole(out, LineInfo)
		return out, nil

	case "list", "ls":
		var obj *engine.GameObject
		if len(rest) > 0 {
			if strings.EqualFold(rest[0], "selected") || strings.EqualFold(rest[0], "picked") {
				obj = b.GetPickedObject()
			} else {
				obj = b.findSceneOrRegisteredObject(rest[0])
			}
			if obj == nil {
				return "", fmt.Errorf("object %q not found", rest[0])
			}
			out := formatObjectComponentsList(obj)
			b.EmitConsole(out, LineInfo)
			return out, nil
		}
		if sel := b.GetPickedObject(); sel != nil {
			out := formatObjectComponentsList(sel)
			b.EmitConsole(out, LineInfo)
			return out, nil
		}
		results := b.SearchComponents("")
		out := formatComponentSearchLines("", results, nil)
		b.EmitConsole(out, LineInfo)
		return out, nil

	case "help", "info":
		if len(rest) == 0 {
			return "", fmt.Errorf("usage: comp help <ComponentType>")
		}
		typeName := strings.Join(rest, " ")
		txt, ok := b.formatNamedTypeHelp(typeName)
		if !ok {
			return "", fmt.Errorf("no component type found named %q", typeName)
		}
		b.EmitConsole(txt, LineInfo)
		return txt, nil

	case "edit":
		if len(rest) == 0 {
			return "", fmt.Errorf("usage: comp edit <CustomComponent>")
		}
		doc, err := b.scriptEditor.OpenScript(strings.Join(rest, " "))
		if err != nil {
			return "", err
		}
		msg := fmt.Sprintf("[ScriptEditor] Opened %s", doc.FilePath)
		b.EmitConsole(msg, LineSystem)
		return msg, nil

	case "add":
		obj, rem, err := b.resolveCompTargetObject(rest)
		if err != nil {
			return "", err
		}
		if len(rem) == 0 {
			return "", fmt.Errorf("usage: comp add [object] <ComponentType> [args...]")
		}
		compType := rem[0]
		extra := rem[1:]
		comp, err := b.addComponentByCommand(obj, compType, extra)
		if err != nil {
			return "", err
		}
		msg := fmt.Sprintf("[Comp] Added %s to %s (%s)", componentCanonicalName(comp), obj.Name, obj.ID)
		b.EmitConsole(msg, LineSystem)
		return msg, nil

	case "remove", "rm":
		obj, rem, err := b.resolveCompTargetObject(rest)
		if err != nil {
			return "", err
		}
		if len(rem) == 0 {
			return "", fmt.Errorf("usage: comp remove [object] <ComponentType>")
		}
		compType := rem[0]
		removed := false
		if rt := componentReflectType(compType); rt != nil {
			removed = obj.RemoveComponent(rt)
		} else if def := b.getCustomComponentDef(compType); def != nil {
			removed = obj.RemoveComponentWhere(func(c engine.Component) bool {
				lc, ok := c.(*LuaComponent)
				return ok && strings.EqualFold(lc.TypeName, def.Name)
			})
		}
		if !removed {
			return "", fmt.Errorf("%s (%s) has no component %q", obj.Name, obj.ID, compType)
		}
		msg := fmt.Sprintf("[Comp] Removed %s from %s (%s)", compType, obj.Name, obj.ID)
		b.EmitConsole(msg, LineSystem)
		return msg, nil

	case "get":
		obj, rem, err := b.resolveCompTargetObject(rest)
		if err != nil {
			return "", err
		}
		if len(rem) == 0 {
			return "", fmt.Errorf("usage: comp get [object] <ComponentType> [field]")
		}
		comp := findComponentByName(obj, rem[0])
		if comp == nil {
			return "", fmt.Errorf("%s (%s) has no component %q", obj.Name, obj.ID, rem[0])
		}
		if len(rem) == 1 {
			out := SerializeComponentJSON(comp)
			b.EmitConsole(out, LineOutput)
			return out, nil
		}
		val, err := b.getComponentField(comp, rem[1])
		if err != nil {
			return "", err
		}
		b.EmitConsole(val, LineOutput)
		return val, nil

	case "set":
		obj, rem, err := b.resolveCompTargetObject(rest)
		if err != nil {
			return "", err
		}
		if len(rem) < 3 {
			return "", fmt.Errorf("usage: comp set [object] <ComponentType> <field> <value...>")
		}
		comp := findComponentByName(obj, rem[0])
		if comp == nil {
			return "", fmt.Errorf("%s (%s) has no component %q", obj.Name, obj.ID, rem[0])
		}
		if err := b.setComponentField(comp, rem[1], rem[2:]); err != nil {
			return "", err
		}
		msg := fmt.Sprintf("[Comp] Set %s.%s on %s (%s)", componentCanonicalName(comp), rem[1], obj.Name, obj.ID)
		b.EmitConsole(msg, LineSystem)
		return msg, nil

	case "call", "invoke":
		obj, rem, err := b.resolveCompTargetObject(rest)
		if err != nil {
			return "", err
		}
		if len(rem) < 2 {
			return "", fmt.Errorf("usage: comp call [object] <ComponentType> <method> [args...]")
		}
		comp := findComponentByName(obj, rem[0])
		if comp == nil {
			return "", fmt.Errorf("%s (%s) has no component %q", obj.Name, obj.ID, rem[0])
		}
		out, err := b.callComponentMethod(comp, rem[1], rem[2:])
		if err != nil {
			return "", err
		}
		if out == "" {
			out = "Nil"
		}
		b.EmitConsole(out, LineOutput)
		return out, nil

	default:
		// If arg0 names a GameObject (or "selected"), show its components; otherwise search component classes.
		if len(args) == 1 {
			if strings.EqualFold(args[0], "selected") || strings.EqualFold(args[0], "picked") {
				if obj := b.GetPickedObject(); obj != nil {
					out := formatObjectComponentsList(obj)
					b.EmitConsole(out, LineInfo)
					return out, nil
				}
			}
			if obj := b.findSceneOrRegisteredObject(args[0]); obj != nil {
				out := formatObjectComponentsList(obj)
				b.EmitConsole(out, LineInfo)
				return out, nil
			}
		}
		q := strings.TrimSpace(strings.Join(args, " "))
		results := b.SearchComponents(q)
		out := formatComponentSearchLines(q, results, b.GetPickedObject())
		b.EmitConsole(out, LineInfo)
		return out, nil
	}
}

func (b *LuaBridge) resolveCompTargetObject(args []string) (*engine.GameObject, []string, error) {
	if len(args) >= 1 {
		if strings.EqualFold(args[0], "selected") || strings.EqualFold(args[0], "picked") {
			obj := b.GetPickedObject()
			if obj == nil {
				return nil, nil, fmt.Errorf("no GameObject currently selected (use pick() first)")
			}
			return obj, args[1:], nil
		}
		// Only treat args[0] as an object if it is not itself a known component name (or if args[1] is a component name).
		if obj := b.findSceneOrRegisteredObject(args[0]); obj != nil {
			isCompName := componentReflectType(args[0]) != nil || b.getCustomComponentDef(args[0]) != nil
			if !isCompName || (len(args) >= 2 && (componentReflectType(args[1]) != nil || b.getCustomComponentDef(args[1]) != nil)) {
				return obj, args[1:], nil
			}
		}
	}
	if obj := b.GetPickedObject(); obj != nil {
		return obj, args, nil
	}
	return nil, nil, fmt.Errorf("no GameObject selected or specified (pass object name/ID or run pick() first)")
}

func (b *LuaBridge) addComponentByCommand(obj *engine.GameObject, compType string, extra []string) (engine.Component, error) {
	canon := resolveComponentAlias(compType)
	switch canon {
	case "Transform":
		if existing := obj.GetTransformComponent(); existing != nil {
			return existing, nil
		}
		tc := engine.NewTransformComponent()
		if len(extra) >= 3 {
			if vec, ok := parseConsoleVec3Args(extra); ok {
				tc.SetPosition(vec)
			}
		}
		obj.AddComponent(tc)
		return tc, nil
	case "Mesh":
		path := ""
		if len(extra) > 0 {
			path = strings.Join(extra, " ")
		}
		mc := engine.NewMeshComponent(path)
		if mc.LoadErr != nil {
			return nil, mc.LoadErr
		}
		obj.AddComponent(mc)
		return mc, nil
	case "MeshRenderable":
		if obj.GetTransformComponent() == nil {
			return nil, fmt.Errorf("MeshRenderable requires an attached Transform component on %q", obj.ID)
		}
		if obj.GetMeshComponent() == nil || obj.GetMeshComponent().Asset == nil {
			return nil, fmt.Errorf("MeshRenderable requires an attached loaded Mesh component on %q", obj.ID)
		}
		mr := engine.NewMeshRenderable()
		obj.AddComponent(mr)
		return mr, nil
	case "Renderable":
		if obj.GetTransformComponent() == nil {
			return nil, fmt.Errorf("Renderable requires an attached Transform component on %q", obj.ID)
		}
		meshID := "cube"
		matID := "default"
		if len(extra) >= 1 {
			meshID = extra[0]
		}
		if len(extra) >= 2 {
			matID = extra[1]
		}
		r := engine.NewRenderable(meshID, matID)
		obj.AddComponent(r)
		return r, nil
	case "Renderable2D":
		r2d := engine.NewRenderable2D(0, 64, 64)
		obj.AddComponent(r2d)
		return r2d, nil
	case "InteractiveComponent":
		disp := obj.Name
		if len(extra) >= 1 {
			disp = strings.Join(extra, " ")
		}
		ic := spatial.NewInteractiveComponent(disp, false)
		obj.AddComponent(ic)
		return ic, nil
	case "FractureComponent":
		hp := float32(15.0)
		if len(extra) >= 1 {
			if f, err := strconv.ParseFloat(extra[0], 32); err == nil {
				hp = float32(f)
			}
		}
		fc := spatial.NewFractureComponent(spatial.FractureSplinter, spatial.IntegrityStandard, hp)
		obj.AddComponent(fc)
		return fc, nil
	default:
		if def := b.getCustomComponentDef(compType); def != nil {
			lc := b.newLuaComponent(def, nil)
			obj.AddComponent(lc)
			return lc, nil
		}
		return nil, fmt.Errorf("unsupported component type %q", compType)
	}
}

func (b *LuaBridge) getComponentField(comp engine.Component, field string) (string, error) {
	if lc, ok := comp.(*LuaComponent); ok {
		if lc.Fields == nil {
			return "Nil", nil
		}
		v := lc.Fields.RawGetString(field)
		return SerializeLuaValue(b.L, v), nil
	}
	pushEngineComponent(b.L, comp)
	ud := b.L.Get(-1)
	b.L.Pop(1)
	if ud == nil || ud == lua.LNil {
		return "", fmt.Errorf("component %T does not expose Lua fields", comp)
	}
	val := b.L.GetField(ud, field)
	return SerializeLuaValue(b.L, val), nil
}

func (b *LuaBridge) setComponentField(comp engine.Component, field string, valTokens []string) error {
	rawVal := strings.Join(valTokens, " ")
	fLower := strings.ToLower(strings.TrimSpace(field))

	switch c := comp.(type) {
	case *engine.TransformComponent:
		switch fLower {
		case "position", "location":
			v, ok := parseConsoleVec3Args(valTokens)
			if !ok {
				return fmt.Errorf("expected 3 numbers for %s", field)
			}
			c.SetPosition(v)
			return nil
		case "rotation":
			v, ok := parseConsoleVec3Args(valTokens)
			if !ok {
				return fmt.Errorf("expected 3 numbers for rotation")
			}
			c.SetRotation(v)
			return nil
		case "scale":
			v, ok := parseConsoleVec3Args(valTokens)
			if !ok {
				return fmt.Errorf("expected 3 numbers for scale")
			}
			c.SetScale(v)
			return nil
		case "x", "y", "z":
			n, err := strconv.ParseFloat(valTokens[0], 32)
			if err != nil {
				return fmt.Errorf("expected float for %s: %w", field, err)
			}
			p := c.GetPosition()
			switch fLower {
			case "x":
				p.X = float32(n)
			case "y":
				p.Y = float32(n)
			case "z":
				p.Z = float32(n)
			}
			c.SetPosition(p)
			return nil
		}
	case *engine.MeshComponent:
		if fLower == "file_path" || fLower == "filepath" || fLower == "path" {
			return c.Load(rawVal)
		}
	case *engine.MeshRenderable:
		switch fLower {
		case "visible":
			bv, err := strconv.ParseBool(valTokens[0])
			if err != nil {
				return err
			}
			c.Visible = bv
			return nil
		case "wireframe":
			bv, err := strconv.ParseBool(valTokens[0])
			if err != nil {
				return err
			}
			c.Wireframe = bv
			return nil
		}
	case *engine.Renderable:
		switch fLower {
		case "visible":
			bv, err := strconv.ParseBool(valTokens[0])
			if err != nil {
				return err
			}
			c.SetVisible(bv)
			return nil
		case "outlined":
			bv, err := strconv.ParseBool(valTokens[0])
			if err != nil {
				return err
			}
			c.SetOutlined(bv)
			return nil
		case "mesh_id", "meshid":
			c.MeshID = rawVal
			return nil
		case "material_id", "materialid":
			c.MaterialID = rawVal
			return nil
		case "script_name", "scriptname":
			c.ScriptName = rawVal
			return nil
		}
	case *spatial.InteractiveComponent:
		switch fLower {
		case "enabled":
			bv, err := strconv.ParseBool(valTokens[0])
			if err != nil {
				return err
			}
			c.Enabled = bv
			return nil
		case "display_name", "displayname":
			c.DisplayName = rawVal
			return nil
		}
	case *spatial.FractureComponent:
		switch fLower {
		case "health":
			n, err := strconv.ParseFloat(valTokens[0], 32)
			if err != nil {
				return err
			}
			c.Health = float32(n)
			return nil
		case "broken":
			bv, err := strconv.ParseBool(valTokens[0])
			if err != nil {
				return err
			}
			c.Broken = bv
			return nil
		}
	case *LuaComponent:
		if c.Fields == nil {
			c.Fields = b.L.NewTable()
		}
		c.Fields.RawSetString(field, parseConsoleScalarToLua(b.L, rawVal))
		return nil
	}
	return fmt.Errorf("cannot set field %q on %s", field, componentCanonicalName(comp))
}

func (b *LuaBridge) callComponentMethod(comp engine.Component, method string, argTokens []string) (string, error) {
	pushEngineComponent(b.L, comp)
	ud := b.L.Get(-1)
	b.L.Pop(1)
	if ud == nil || ud == lua.LNil {
		return "", fmt.Errorf("component %T cannot be invoked from Lua", comp)
	}
	fnVal := b.L.GetField(ud, method)
	fn, ok := fnVal.(*lua.LFunction)
	if !ok || fn == nil {
		return "", fmt.Errorf("component %s has no method %q", componentCanonicalName(comp), method)
	}

	baseTop := b.L.GetTop()
	b.L.Push(fn)
	b.L.Push(ud)
	for _, tok := range argTokens {
		b.L.Push(parseConsoleScalarToLua(b.L, tok))
	}
	if err := b.L.PCall(1+len(argTokens), lua.MultRet, nil); err != nil {
		b.L.SetTop(baseTop)
		return "", err
	}
	top := b.L.GetTop()
	if top <= baseTop {
		return "", nil
	}
	parts := make([]string, 0, top-baseTop)
	for i := baseTop + 1; i <= top; i++ {
		parts = append(parts, SerializeLuaValue(b.L, b.L.Get(i)))
	}
	b.L.SetTop(baseTop)
	return strings.Join(parts, ", "), nil
}

func parseConsoleVec3Args(tokens []string) (engine.Vector3, bool) {
	if len(tokens) == 1 {
		cleaned := strings.Trim(tokens[0], "(){}[]")
		parts := strings.Split(cleaned, ",")
		if len(parts) == 3 {
			tokens = parts
		}
	}
	if len(tokens) < 3 {
		return engine.Vector3{}, false
	}
	x, err1 := strconv.ParseFloat(strings.TrimSpace(tokens[0]), 32)
	y, err2 := strconv.ParseFloat(strings.TrimSpace(tokens[1]), 32)
	z, err3 := strconv.ParseFloat(strings.TrimSpace(tokens[2]), 32)
	if err1 != nil || err2 != nil || err3 != nil {
		return engine.Vector3{}, false
	}
	return engine.NewVector3(float32(x), float32(y), float32(z)), true
}

func parseConsoleScalarToLua(L *lua.LState, raw string) lua.LValue {
	trimmed := strings.TrimSpace(raw)
	if strings.EqualFold(trimmed, "true") {
		return lua.LTrue
	}
	if strings.EqualFold(trimmed, "false") {
		return lua.LFalse
	}
	if strings.EqualFold(trimmed, "nil") {
		return lua.LNil
	}
	if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return lua.LNumber(num)
	}
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		trimmed = trimmed[1 : len(trimmed)-1]
	}
	return lua.LString(trimmed)
}
