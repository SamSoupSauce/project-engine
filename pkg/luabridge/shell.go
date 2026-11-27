package luabridge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ExecutionContext carries mandatory origin attribution and parsed arguments for
// every command invocation across the Dear ImGui console, HTTP JSON-RPC, WebSocket, and Lua scripts.
type ExecutionContext struct {
	Sender    string    // e.g. "user", "agent:agy", "agent:ws:127.0.0.1", "script:trap_sys"
	Timestamp time.Time // Exact engine/tick invocation timestamp
	Raw       string    // Raw unparsed command line
	Args      []string  // Whitespace-split arguments honoring quoted substrings
}

// ShellCommandDef describes a native Go or dynamically registered Lua shell command.
type ShellCommandDef struct {
	Name        string
	Description string
	Usage       string
	Examples    []string
	Source      string
	IsLua       bool
	GoHandler   func(ctx ExecutionContext) (string, error)
	LuaHandler  *lua.LFunction
}

// LoadedScriptRecord tracks scripts run or loaded through the shell lifecycle.
type LoadedScriptRecord struct {
	Path               string
	Name               string
	Loaded             bool
	LastLoadedAt       time.Time
	ModuleTable        *lua.LTable
	RegisteredCommands []string
}

// ShellRegistry manages all native Go and dynamic Lua shell commands as well as loaded script metadata.
type ShellRegistry struct {
	mu             sync.RWMutex
	commands       map[string]*ShellCommandDef
	scriptCommands map[string][]string
	loadedScripts  map[string]*LoadedScriptRecord
}

// NewShellRegistry creates an initialized ShellRegistry.
func NewShellRegistry() *ShellRegistry {
	return &ShellRegistry{
		commands:       make(map[string]*ShellCommandDef),
		scriptCommands: make(map[string][]string),
		loadedScripts:  make(map[string]*LoadedScriptRecord),
	}
}

// RegisterNative registers or updates a native Go shell command.
func (r *ShellRegistry) RegisterNative(def ShellCommandDef) {
	key := strings.ToLower(strings.TrimSpace(def.Name))
	if key == "" {
		return
	}
	def.Name = key
	def.IsLua = false
	if def.Source == "" {
		def.Source = "native"
	}
	r.mu.Lock()
	r.commands[key] = &def
	r.mu.Unlock()
}

// RegisterLua registers a Lua shell command bound to source (script path or inline sender).
func (r *ShellRegistry) RegisterLua(def ShellCommandDef) {
	key := strings.ToLower(strings.TrimSpace(def.Name))
	if key == "" {
		return
	}
	def.Name = key
	def.IsLua = true
	sourceKey := normalizeScriptKey(def.Source)
	if sourceKey == "" {
		sourceKey = "inline"
	}
	def.Source = sourceKey

	r.mu.Lock()
	r.commands[key] = &def
	if !containsString(r.scriptCommands[sourceKey], key) {
		r.scriptCommands[sourceKey] = append(r.scriptCommands[sourceKey], key)
	}
	if rec, ok := r.loadedScripts[sourceKey]; ok && rec != nil {
		if !containsString(rec.RegisteredCommands, key) {
			rec.RegisteredCommands = append(rec.RegisteredCommands, key)
		}
	}
	r.mu.Unlock()
}

// Unregister removes a shell command by name.
func (r *ShellRegistry) Unregister(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	def, ok := r.commands[key]
	if !ok {
		return false
	}
	delete(r.commands, key)
	if def.IsLua && def.Source != "" {
		r.scriptCommands[def.Source] = removeString(r.scriptCommands[def.Source], key)
	}
	return true
}

// UnbindScriptCommands removes every Lua shell command owned by scriptPath.
func (r *ShellRegistry) UnbindScriptCommands(scriptPath string) []string {
	sourceKey := normalizeScriptKey(scriptPath)
	if sourceKey == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	names := append([]string(nil), r.scriptCommands[sourceKey]...)
	for _, name := range names {
		if def, ok := r.commands[name]; ok && def.IsLua && normalizeScriptKey(def.Source) == sourceKey {
			delete(r.commands, name)
		}
	}
	delete(r.scriptCommands, sourceKey)
	if rec, ok := r.loadedScripts[sourceKey]; ok && rec != nil {
		rec.RegisteredCommands = nil
	}
	return names
}

// Get returns a snapshot of a registered ShellCommandDef.
func (r *ShellRegistry) Get(name string) (*ShellCommandDef, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	def, ok := r.commands[key]
	r.mu.RUnlock()
	if !ok || def == nil {
		return nil, false
	}
	cp := *def
	return &cp, true
}

// List returns all registered shell commands sorted alphabetically by name.
func (r *ShellRegistry) List() []*ShellCommandDef {
	r.mu.RLock()
	out := make([]*ShellCommandDef, 0, len(r.commands))
	for _, def := range r.commands {
		if def != nil {
			cp := *def
			out = append(out, &cp)
		}
	}
	r.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// RecordScriptLoaded marks scriptPath as loaded in the registry.
func (r *ShellRegistry) RecordScriptLoaded(scriptPath string, mod *lua.LTable) {
	key := normalizeScriptKey(scriptPath)
	if key == "" {
		return
	}
	r.mu.Lock()
	rec, ok := r.loadedScripts[key]
	if !ok || rec == nil {
		rec = &LoadedScriptRecord{
			Path: key,
			Name: stemFromPath(key),
		}
		r.loadedScripts[key] = rec
	}
	rec.Loaded = true
	rec.LastLoadedAt = time.Now()
	rec.ModuleTable = mod
	rec.RegisteredCommands = append([]string(nil), r.scriptCommands[key]...)
	r.mu.Unlock()
}

// MarkScriptUnloaded marks scriptPath as unloaded and clears its ModuleTable reference.
func (r *ShellRegistry) MarkScriptUnloaded(scriptPath string) *LoadedScriptRecord {
	key := normalizeScriptKey(scriptPath)
	if key == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.loadedScripts[key]
	if rec == nil {
		return nil
	}
	cp := *rec
	rec.Loaded = false
	rec.ModuleTable = nil
	rec.RegisteredCommands = nil
	return &cp
}

// IsScriptLoaded reports whether scriptPath is currently marked loaded.
func (r *ShellRegistry) IsScriptLoaded(scriptPath string) bool {
	key := normalizeScriptKey(scriptPath)
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec := r.loadedScripts[key]
	return rec != nil && rec.Loaded
}

// GetScriptRecord returns a copy of the LoadedScriptRecord for scriptPath, if any.
func (r *ShellRegistry) GetScriptRecord(scriptPath string) (*LoadedScriptRecord, bool) {
	key := normalizeScriptKey(scriptPath)
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.loadedScripts[key]
	if !ok || rec == nil {
		return nil, false
	}
	cp := *rec
	return &cp, true
}

func normalizeScriptKey(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(trimmed))
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func removeString(list []string, target string) []string {
	out := list[:0]
	for _, s := range list {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}

// ParseShellArgs splits a command string on whitespace while preserving quoted substrings
// (`"..."` or `'...'`) and backslash escapes inside double quotes.
func ParseShellArgs(raw string) []string {
	var args []string
	var cur strings.Builder
	inToken := false
	var quote rune

	runes := []rune(raw)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			if r == '\\' && quote == '"' && i+1 < len(runes) {
				i++
				cur.WriteRune(runes[i])
				continue
			}
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
			continue
		}

		switch r {
		case '"', '\'':
			quote = r
			inToken = true
		case ' ', '\t', '\r', '\n':
			if inToken {
				args = append(args, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if inToken {
		args = append(args, cur.String())
	}
	return args
}

// Shell returns the LuaBridge's unified ShellRegistry.
func (b *LuaBridge) Shell() *ShellRegistry {
	if b == nil {
		return nil
	}
	return b.shell
}

// ExecuteWithContext is the singular execution funnel for all user and remote agent commands.
// It validates and parses arguments into ExecutionContext, mirrors sender-attributed input/output
// into the Dear ImGui console when attached, and executes shell verbs or Lua code on the active state.
func (b *LuaBridge) ExecuteWithContext(ctx ExecutionContext, rawCommand string) (string, error) {
	trimmed := strings.TrimSpace(rawCommand)
	if trimmed == "" {
		return "", nil
	}

	if strings.TrimSpace(ctx.Sender) == "" {
		ctx.Sender = "user"
	}
	if ctx.Timestamp.IsZero() {
		ctx.Timestamp = time.Now()
	}
	if strings.TrimSpace(ctx.Raw) == "" {
		ctx.Raw = trimmed
	}
	ctx.Args = ParseShellArgs(trimmed)

	b.dataMu.RLock()
	c := b.console
	b.dataMu.RUnlock()

	if c != nil {
		if strings.HasPrefix(ctx.Sender, "agent:") {
			c.Print(fmt.Sprintf("[%s] %s", ctx.Sender, trimmed), LineInput)
		} else if ctx.Sender == "user" {
			c.Print("> "+ctx.Raw, LineInput)
		}
	}

	out, streamed, err := b.executeWithStreamFlagContext(ctx, trimmed)
	if err != nil {
		if c != nil {
			c.Print("Error: "+err.Error(), LineError)
		}
		return "", err
	}
	if out != "" && !streamed && c != nil {
		c.Print(out, LineOutput)
	}
	return out, nil
}

func (b *LuaBridge) registerShellAPIs() {
	if b.shell == nil {
		b.shell = NewShellRegistry()
	}

	// Register Lua global `shell` API: shell.register, shell.unregister, shell.list
	shellTbl := b.L.NewTable()
	b.L.SetField(shellTbl, "register", b.L.NewFunction(func(L *lua.LState) int {
		name := strings.TrimSpace(L.CheckString(1))
		if name == "" {
			L.ArgError(1, "command name must not be empty")
			return 0
		}
		opts := L.CheckTable(2)
		fnVal := opts.RawGetString("handler")
		fn, ok := fnVal.(*lua.LFunction)
		if !ok || fn == nil {
			L.ArgError(2, "shell.register requires a handler function")
			return 0
		}
		desc := ""
		if d := opts.RawGetString("description"); d != lua.LNil {
			desc = d.String()
		}
		usage := ""
		if u := opts.RawGetString("usage"); u != lua.LNil {
			usage = u.String()
		}
		var examples []string
		if exTbl, ok := opts.RawGetString("examples").(*lua.LTable); ok && exTbl != nil {
			for i := 1; i <= exTbl.Len(); i++ {
				if v := exTbl.RawGetInt(i); v != lua.LNil {
					examples = append(examples, v.String())
				}
			}
		}

		source := strings.TrimSpace(b.loadingScriptPath)
		if source == "" {
			if b.activeExecCtx != nil && b.activeExecCtx.Sender != "" {
				source = "inline:" + b.activeExecCtx.Sender
			} else {
				source = "inline"
			}
		}

		b.shell.RegisterLua(ShellCommandDef{
			Name:        name,
			Description: desc,
			Usage:       usage,
			Examples:    examples,
			Source:      source,
			LuaHandler:  fn,
		})
		return 0
	}))

	b.L.SetField(shellTbl, "unregister", b.L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		L.Push(lua.LBool(b.shell.Unregister(name)))
		return 1
	}))

	b.L.SetField(shellTbl, "list", b.L.NewFunction(func(L *lua.LState) int {
		cmds := b.shell.List()
		tbl := L.NewTable()
		for i, cmd := range cmds {
			entry := L.NewTable()
			entry.RawSetString("name", lua.LString(cmd.Name))
			entry.RawSetString("description", lua.LString(cmd.Description))
			entry.RawSetString("usage", lua.LString(cmd.Usage))
			entry.RawSetString("source", lua.LString(cmd.Source))
			entry.RawSetString("is_lua", lua.LBool(cmd.IsLua))
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		return 1
	}))

	b.L.SetGlobal("shell", shellTbl)

	// Built-in core shell commands (scripts.list, scripts.run, scripts.reload, scripts.unload)
	b.shell.RegisterNative(ShellCommandDef{
		Name:        "scripts.list",
		Description: "Scans and outputs all .lua files in the scripts directory along with their loaded status.",
		Usage:       "scripts.list [query]",
		Examples:    []string{"scripts.list", "scripts.list components"},
		GoHandler: func(ctx ExecutionContext) (string, error) {
			query := strings.TrimSpace(strings.Join(ctx.Args[1:], " "))
			results, err := b.SearchScripts(query)
			if err != nil {
				return "", err
			}
			out := b.formatShellScriptsList(query, results)
			b.EmitConsole(out, LineInfo)
			return out, nil
		},
	})

	b.shell.RegisterNative(ShellCommandDef{
		Name:        "scripts.run",
		Description: "Executes a Lua script file within the global Lua environment under the current sender context.",
		Usage:       "scripts.run <path_or_name> [args...]",
		Examples:    []string{"scripts.run scripts/test_tools.lua", "scripts.run Spinner"},
		GoHandler: func(ctx ExecutionContext) (string, error) {
			if len(ctx.Args) < 2 {
				return "", fmt.Errorf("usage: scripts.run <path_or_name> [args...]")
			}
			return b.runScriptFile(ctx.Args[1], ctx.Args[2:], false)
		},
	})

	b.shell.RegisterNative(ShellCommandDef{
		Name:        "scripts.reload",
		Description: "Purges a script's cached chunk/commands and re-runs its initialization hook live.",
		Usage:       "scripts.reload <script_name_or_path>",
		Examples:    []string{"scripts.reload test_tools", "scripts.reload scripts/components/Spinner.lua"},
		GoHandler: func(ctx ExecutionContext) (string, error) {
			if len(ctx.Args) < 2 {
				return "", fmt.Errorf("usage: scripts.reload <script_name_or_path>")
			}
			return b.reloadScriptFile(ctx.Args[1], ctx.Args[2:])
		},
	})

	b.shell.RegisterNative(ShellCommandDef{
		Name:        "scripts.unload",
		Description: "Invokes a script's unload lifecycle hook and strips its shell commands and hooks from the active registry.",
		Usage:       "scripts.unload <script_name_or_path>",
		Examples:    []string{"scripts.unload test_tools"},
		GoHandler: func(ctx ExecutionContext) (string, error) {
			if len(ctx.Args) < 2 {
				return "", fmt.Errorf("usage: scripts.unload <script_name_or_path>")
			}
			return b.unloadScriptFile(ctx.Args[1])
		},
	})
}

func (b *LuaBridge) formatShellScriptsList(query string, results []ScriptSearchResult) string {
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
		loaded := b.isScriptActive(r.FilePath, r.ComponentName)
		status := "available"
		if loaded {
			status = "LOADED"
		}
		extra := ""
		if r.ComponentName != "" {
			extra += " component=" + r.ComponentName
		}
		if rec, ok := b.shell.GetScriptRecord(r.FilePath); ok && len(rec.RegisteredCommands) > 0 {
			extra += " cmds=[" + strings.Join(rec.RegisteredCommands, ", ") + "]"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s (%s)%s\n", status, r.Name, r.RelativePath, extra))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (b *LuaBridge) isScriptActive(filePath, compName string) bool {
	if b.shell != nil && b.shell.IsScriptLoaded(filePath) {
		return true
	}
	norm := normalizeScriptKey(filePath)
	b.dataMu.RLock()
	for _, def := range b.customComponents {
		if def != nil && (normalizeScriptKey(def.SourcePath) == norm || (compName != "" && strings.EqualFold(def.Name, compName))) {
			b.dataMu.RUnlock()
			return true
		}
	}
	b.dataMu.RUnlock()

	for _, obj := range b.collectAllTrackedObjects() {
		for _, c := range obj.Components {
			if sc, ok := c.(*ScriptComponent); ok && normalizeScriptKey(sc.ScriptPath) == norm && sc.loaded {
				return true
			}
		}
	}
	return false
}

func (b *LuaBridge) resolveScriptPath(target string) (string, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", fmt.Errorf("script target must not be empty")
	}
	scriptsDir := b.effectiveScriptsDir()

	candidates := []string{filepath.Clean(trimmed)}
	if !strings.HasSuffix(strings.ToLower(trimmed), ".lua") {
		candidates = append(candidates, filepath.Clean(trimmed+".lua"))
	}
	candidates = append(candidates, filepath.Join(scriptsDir, trimmed))
	if !strings.HasSuffix(strings.ToLower(trimmed), ".lua") {
		candidates = append(candidates, filepath.Join(scriptsDir, trimmed+".lua"))
	}
	for _, cand := range candidates {
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return filepath.Clean(cand), nil
		}
	}
	if found, ok := findScriptByStem(scriptsDir, trimmed); ok {
		return filepath.Clean(found), nil
	}
	return "", fmt.Errorf("script %q not found", target)
}

func (b *LuaBridge) runScriptFile(target string, args []string, isReload bool) (string, error) {
	resolved, err := b.resolveScriptPath(target)
	if err != nil {
		return "", err
	}

	// Clean up any commands previously owned by this script so re-runs don't leave stale bindings.
	b.shell.UnbindScriptCommands(resolved)

	b.stateMu.Lock()
	defer b.stateMu.Unlock()

	prevLoading := b.loadingScriptPath
	b.loadingScriptPath = resolved
	defer func() { b.loadingScriptPath = prevLoading }()

	prevArg := b.L.GetGlobal("arg")
	argTbl := b.L.NewTable()
	argTbl.RawSetInt(0, lua.LString(resolved))
	for i, a := range args {
		argTbl.RawSetInt(i+1, lua.LString(a))
	}
	b.L.SetGlobal("arg", argTbl)
	defer b.L.SetGlobal("arg", prevArg)

	fn, err := b.L.LoadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("scripts.run %q: %w", resolved, err)
	}
	baseTop := b.L.GetTop()
	b.L.Push(fn)
	callArgs := make([]lua.LValue, 0, len(args))
	for _, a := range args {
		callArgs = append(callArgs, lua.LString(a))
	}
	for _, ca := range callArgs {
		b.L.Push(ca)
	}
	if err := b.L.PCall(len(callArgs), lua.MultRet, nil); err != nil {
		b.L.SetTop(baseTop)
		return "", fmt.Errorf("scripts.run %q: %w", resolved, err)
	}

	var modTbl *lua.LTable
	if b.L.GetTop() > baseTop {
		if tbl, ok := b.L.Get(baseTop + 1).(*lua.LTable); ok {
			modTbl = tbl
		}
	}
	b.L.SetTop(baseTop)

	b.shell.RecordScriptLoaded(resolved, modTbl)

	// Invoke optional module load(scene) hook if returned by the script.
	b.dataMu.RLock()
	scene := b.scene
	b.dataMu.RUnlock()
	if modTbl != nil {
		if loadFn, ok := modTbl.RawGetString("load").(*lua.LFunction); ok && loadFn != nil {
			b.L.Push(loadFn)
			if scene != nil {
				pushScene(b.L, scene)
			} else {
				b.L.Push(lua.LNil)
			}
			if err := b.L.PCall(1, 0, nil); err != nil {
				return "", fmt.Errorf("scripts.run %q load hook: %w", resolved, err)
			}
		}
	}

	action := "Executed"
	if isReload {
		action = "Reloaded"
	}
	rec, _ := b.shell.GetScriptRecord(resolved)
	cmdSuffix := ""
	if rec != nil && len(rec.RegisteredCommands) > 0 {
		cmdSuffix = fmt.Sprintf(" (registered commands: %s)", strings.Join(rec.RegisteredCommands, ", "))
	}
	msg := fmt.Sprintf("[Script] %s %s%s", action, filepath.ToSlash(resolved), cmdSuffix)
	b.EmitConsole(msg, LineSystem)
	return msg, nil
}

func (b *LuaBridge) reloadScriptFile(target string, args []string) (string, error) {
	resolved, err := b.resolveScriptPath(target)
	if err != nil {
		return "", err
	}
	_, _ = b.unloadScriptByResolvedPath(resolved, true)
	msg, err := b.runScriptFile(resolved, args, true)
	if err != nil {
		return "", err
	}

	// Reload any matching ScriptComponents attached to live scene GameObjects.
	for _, obj := range b.collectAllTrackedObjects() {
		for _, comp := range obj.Components {
			if sc, ok := comp.(*ScriptComponent); ok && normalizeScriptKey(sc.ScriptPath) == normalizeScriptKey(resolved) {
				if reloadErr := sc.loadFromFile(resolved); reloadErr != nil {
					return "", reloadErr
				}
			}
		}
	}
	return msg, nil
}

func (b *LuaBridge) unloadScriptFile(target string) (string, error) {
	resolved, err := b.resolveScriptPath(target)
	if err != nil {
		// Allow unloading a previously loaded script even if file was deleted from disk.
		norm := normalizeScriptKey(target)
		if _, ok := b.shell.GetScriptRecord(norm); ok {
			resolved = norm
		} else {
			return "", err
		}
	}
	return b.unloadScriptByResolvedPath(resolved, false)
}

func (b *LuaBridge) unloadScriptByResolvedPath(resolved string, silent bool) (string, error) {
	rec, hasRec := b.shell.GetScriptRecord(resolved)

	b.stateMu.Lock()
	b.dataMu.RLock()
	scene := b.scene
	b.dataMu.RUnlock()

	if hasRec && rec.ModuleTable != nil {
		if unloadFn, ok := rec.ModuleTable.RawGetString("unload").(*lua.LFunction); ok && unloadFn != nil {
			b.L.Push(unloadFn)
			if scene != nil {
				pushScene(b.L, scene)
			} else {
				b.L.Push(lua.LNil)
			}
			_ = b.L.PCall(1, 0, nil)
		}
	}

	// Trigger unload on any matching live ScriptComponents.
	for _, obj := range b.collectAllTrackedObjects() {
		for _, comp := range obj.Components {
			if sc, ok := comp.(*ScriptComponent); ok && normalizeScriptKey(sc.ScriptPath) == normalizeScriptKey(resolved) {
				sc.OnUnload(scene)
			}
		}
	}

	// Clear package.loaded cache for the script stem / slash path.
	stem := stemFromPath(resolved)
	if pkgTbl, ok := b.L.GetGlobal("package").(*lua.LTable); ok && pkgTbl != nil {
		if loadedTbl, ok := pkgTbl.RawGetString("loaded").(*lua.LTable); ok && loadedTbl != nil {
			loadedTbl.RawSetString(stem, lua.LNil)
			loadedTbl.RawSetString(filepath.ToSlash(resolved), lua.LNil)
		}
	}
	b.stateMu.Unlock()

	// Unbind all shell commands registered by this script and unhook custom component listeners if owned by this script.
	unbound := b.shell.UnbindScriptCommands(resolved)
	b.shell.MarkScriptUnloaded(resolved)

	norm := normalizeScriptKey(resolved)
	b.dataMu.RLock()
	var matchingDefs []*LuaComponentDef
	for _, def := range b.customComponents {
		if def != nil && normalizeScriptKey(def.SourcePath) == norm {
			matchingDefs = append(matchingDefs, def)
		}
	}
	b.dataMu.RUnlock()

	for _, def := range matchingDefs {
		for _, obj := range b.collectAllTrackedObjects() {
			for _, c := range obj.Components {
				if lc, ok := c.(*LuaComponent); ok && strings.EqualFold(lc.TypeName, def.Name) {
					b.unregisterLuaComponentHooks(obj, lc)
				}
			}
		}
	}

	if silent {
		return "", nil
	}
	msg := fmt.Sprintf("[Script] Unloaded %s (unbound %d command(s))", filepath.ToSlash(resolved), len(unbound))
	b.EmitConsole(msg, LineSystem)
	return msg, nil
}

func (b *LuaBridge) invokeShellCommandLocked(def *ShellCommandDef, ctx ExecutionContext) (string, error) {
	if def == nil {
		return "", fmt.Errorf("nil shell command")
	}
	prevCtx := b.activeExecCtx
	b.activeExecCtx = &ctx
	defer func() { b.activeExecCtx = prevCtx }()

	if !def.IsLua && def.GoHandler != nil {
		return def.GoHandler(ctx)
	}
	if def.IsLua && def.LuaHandler != nil {
		ctxTbl := b.L.NewTable()
		ctxTbl.RawSetString("sender", lua.LString(ctx.Sender))
		ctxTbl.RawSetString("timestamp", lua.LString(ctx.Timestamp.Format(time.RFC3339Nano)))
		ctxTbl.RawSetString("raw", lua.LString(ctx.Raw))

		argsTbl := b.L.NewTable()
		argSlice := []string{}
		if len(ctx.Args) > 1 {
			argSlice = ctx.Args[1:]
		}
		for i, a := range argSlice {
			argsTbl.RawSetInt(i+1, lua.LString(a))
		}
		ctxTbl.RawSetString("args", argsTbl)

		baseTop := b.L.GetTop()
		b.L.Push(def.LuaHandler)
		b.L.Push(ctxTbl)
		b.L.Push(argsTbl)
		if err := b.L.PCall(2, lua.MultRet, nil); err != nil {
			b.L.SetTop(baseTop)
			return "", err
		}

		nret := b.L.GetTop() - baseTop
		if nret <= 0 {
			b.L.SetTop(baseTop)
			return "", nil
		}

		first := b.L.Get(baseTop + 1)
		if nret >= 2 && (first == lua.LFalse || first == lua.LTrue) {
			second := b.L.Get(baseTop + 2)
			msg := ""
			if second != nil && second != lua.LNil {
				msg = SerializeLuaValue(b.L, second)
			}
			b.L.SetTop(baseTop)
			if first == lua.LFalse {
				if msg == "" {
					msg = fmt.Sprintf("command %q failed", def.Name)
				}
				return "", fmt.Errorf("%s", msg)
			}
			return msg, nil
		}

		parts := make([]string, 0, nret)
		for i := baseTop + 1; i <= b.L.GetTop(); i++ {
			parts = append(parts, SerializeLuaValue(b.L, b.L.Get(i)))
		}
		b.L.SetTop(baseTop)
		return strings.Join(parts, "\n"), nil
	}
	return "", fmt.Errorf("shell command %q has no handler", def.Name)
}

func formatShellCommandHelp(def *ShellCommandDef) string {
	var sb strings.Builder
	kind := "native"
	if def.IsLua {
		kind = "lua"
	}
	sb.WriteString(fmt.Sprintf("=== Command Help: %s (%s) ===\n", def.Name, kind))
	if def.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", def.Description))
	}
	if def.Usage != "" {
		sb.WriteString(fmt.Sprintf("Usage:       %s\n", def.Usage))
	}
	if def.Source != "" {
		sb.WriteString(fmt.Sprintf("Source:      %s\n", def.Source))
	}
	if len(def.Examples) > 0 {
		sb.WriteString("Examples:\n")
		for _, ex := range def.Examples {
			sb.WriteString("  " + ex + "\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
