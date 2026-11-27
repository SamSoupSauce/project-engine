package luabridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
)

const defaultEditorScriptsDir = "scripts"

// ScriptEditorService implements engine.ScriptEditorBackend over LuaBridge,
// supporting `-- @data:` / `-- @hooks:` single-file authoring, JSON + Lua validation,
// SHA-256 tracking, and live entity component hot-reload.
type ScriptEditorService struct {
	Bridge     *LuaBridge
	ScriptsDir string

	mu        sync.RWMutex
	open      bool
	activeDoc engine.ScriptEditorDocument
	revision  uint64
	hashes    map[string]string
}

// NewScriptEditorService creates a ScriptEditorService bound to bridge.
func NewScriptEditorService(bridge *LuaBridge) *ScriptEditorService {
	return &ScriptEditorService{
		Bridge:     bridge,
		ScriptsDir: defaultEditorScriptsDir,
		hashes:     make(map[string]string),
	}
}

func (b *LuaBridge) registerScriptEditorAPIs() {
	if b.scriptEditor == nil {
		b.scriptEditor = NewScriptEditorService(b)
	}

	openFn := b.L.NewFunction(func(L *lua.LState) int {
		if L.GetTop() == 0 || L.Get(1) == lua.LNil {
			b.scriptEditor.SetEditorOpen(false)
			return 0
		}
		target := strings.TrimSpace(L.CheckString(1))
		if target == "" {
			b.scriptEditor.SetEditorOpen(false)
			return 0
		}
		doc, err := b.scriptEditor.OpenScript(target)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.EmitConsole(fmt.Sprintf("[ScriptEditor] Opened %s", doc.FilePath), LineSystem)
		return 0
	})

	b.L.SetGlobal("__engine_open_editor", openFn)
	b.L.SetGlobal("edit", openFn)

	b.RegisterCommand("edit", func(args []string) (string, error) {
		if len(args) == 0 {
			b.scriptEditor.SetEditorOpen(false)
			return "Script editor closed.", nil
		}
		target := strings.TrimSpace(strings.Join(args, " "))
		doc, err := b.scriptEditor.OpenScript(target)
		if err != nil {
			return "", err
		}
		msg := fmt.Sprintf("[ScriptEditor] Opened %s", doc.FilePath)
		b.EmitConsole(msg, LineSystem)
		return msg, nil
	})
}

// ScriptEditor returns the LuaBridge's ScriptEditorService for attaching to engine.ImGuiOverlay.
func (b *LuaBridge) ScriptEditor() *ScriptEditorService {
	if b == nil {
		return nil
	}
	return b.scriptEditor
}

// SetScriptsDir overrides the root directory searched by edit("Name") and LoadScriptsDir.
func (s *ScriptEditorService) SetScriptsDir(dir string) {
	if s == nil || strings.TrimSpace(dir) == "" {
		return
	}
	s.mu.Lock()
	s.ScriptsDir = dir
	s.mu.Unlock()
}

// IsEditorOpen reports whether the script editor window should be visible.
func (s *ScriptEditorService) IsEditorOpen() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.open
}

// SetEditorOpen updates the editor visibility and bumps Revision.
func (s *ScriptEditorService) SetEditorOpen(open bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.open = open
	s.revision++
	s.mu.Unlock()
}

// GetActiveDocument returns the currently loaded ScriptEditorDocument.
func (s *ScriptEditorService) GetActiveDocument() engine.ScriptEditorDocument {
	if s == nil {
		return engine.ScriptEditorDocument{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeDoc
}

// Revision returns a monotonically increasing counter bumped whenever editor state changes from Go/Lua.
func (s *ScriptEditorService) Revision() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

// LastHash returns the last committed SHA-256 hex digest for filePath, if recorded.
func (s *ScriptEditorService) LastHash(filePath string) string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hashes[filepath.Clean(filePath)]
}

// OpenScript resolves target (component name, script stem, or path), loads/parses it into
// the tri-pane document buffers, and flags the editor open.
func (s *ScriptEditorService) OpenScript(target string) (engine.ScriptEditorDocument, error) {
	if s == nil || s.Bridge == nil {
		return engine.ScriptEditorDocument{}, fmt.Errorf("luabridge: script editor bridge is nil")
	}
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		s.SetEditorOpen(false)
		return engine.ScriptEditorDocument{}, nil
	}

	resolvedPath, stemName, exists := s.resolveTarget(trimmed)
	var doc engine.ScriptEditorDocument
	if exists {
		raw, err := os.ReadFile(resolvedPath)
		if err != nil {
			return engine.ScriptEditorDocument{}, fmt.Errorf("script editor: read %q: %w", resolvedPath, err)
		}
		doc = s.parseScriptDocument(stemName, resolvedPath, raw)
	} else {
		doc = s.newScriptTemplate(stemName, resolvedPath)
	}

	s.mu.Lock()
	s.activeDoc = doc
	s.open = true
	if doc.ContentHash != "" {
		s.hashes[filepath.Clean(doc.FilePath)] = doc.ContentHash
	}
	s.revision++
	s.mu.Unlock()

	return doc, nil
}

// ValidateScript verifies DataJSON, HooksJSON, and FunctionsLua (plus synthesized source)
// without writing to disk or mutating runtime state.
func (s *ScriptEditorService) ValidateScript(doc engine.ScriptEditorDocument) error {
	if s == nil || s.Bridge == nil || s.Bridge.L == nil {
		return fmt.Errorf("luabridge: bridge not initialized")
	}
	dataMap, compactData, err := parseJSONObject(doc.DataJSON, "Data (JSON)")
	if err != nil {
		return err
	}
	hooksMap, compactHooks, err := parseHooksJSONObject(doc.HooksJSON)
	if err != nil {
		return err
	}

	s.Bridge.stateMu.Lock()
	defer s.Bridge.stateMu.Unlock()

	if err := compileLuaChunk(s.Bridge.L, doc.FunctionsLua, "Functions (Lua)"); err != nil {
		return err
	}
	fullSource := s.synthesizeScriptSource(doc, compactData, compactHooks, dataMap, hooksMap)
	if err := compileLuaChunk(s.Bridge.L, fullSource, doc.FilePath); err != nil {
		return err
	}
	return nil
}

// CommitScript validates, writes the single-file `.lua` script to disk, updates its SHA-256 hash,
// reloads it in LuaBridge, and hot-reloads live entities preserving existing matching field state.
func (s *ScriptEditorService) CommitScript(doc engine.ScriptEditorDocument) (engine.ScriptEditorDocument, error) {
	if s == nil || s.Bridge == nil || s.Bridge.L == nil {
		return doc, fmt.Errorf("luabridge: bridge not initialized")
	}
	if strings.TrimSpace(doc.FilePath) == "" {
		return doc, fmt.Errorf("script editor: missing target file path")
	}

	dataMap, compactData, err := parseJSONObject(doc.DataJSON, "Data (JSON)")
	if err != nil {
		return doc, err
	}
	hooksMap, compactHooks, err := parseHooksJSONObject(doc.HooksJSON)
	if err != nil {
		return doc, err
	}

	fullSource := s.synthesizeScriptSource(doc, compactData, compactHooks, dataMap, hooksMap)

	s.Bridge.stateMu.Lock()
	if err := compileLuaChunk(s.Bridge.L, doc.FunctionsLua, "Functions (Lua)"); err != nil {
		s.Bridge.stateMu.Unlock()
		return doc, err
	}
	if err := compileLuaChunk(s.Bridge.L, fullSource, doc.FilePath); err != nil {
		s.Bridge.stateMu.Unlock()
		return doc, err
	}
	s.Bridge.stateMu.Unlock()

	cleanPath := filepath.Clean(doc.FilePath)
	if dir := filepath.Dir(cleanPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return doc, fmt.Errorf("script editor: create dir %q: %w", dir, err)
		}
	}
	rawBytes := []byte(fullSource)
	if err := os.WriteFile(cleanPath, rawBytes, 0o644); err != nil {
		return doc, fmt.Errorf("script editor: write %q: %w", cleanPath, err)
	}

	sum := sha256.Sum256(rawBytes)
	hashHex := hex.EncodeToString(sum[:])

	if err := s.hotReloadFile(cleanPath, doc.Name, dataMap, hooksMap); err != nil {
		return doc, err
	}

	prettyData, _ := formatJSONMap(dataMap)
	prettyHooks, _ := formatJSONMap(hooksMap)
	updated := engine.ScriptEditorDocument{
		Name:         doc.Name,
		FilePath:     cleanPath,
		DataJSON:     prettyData,
		HooksJSON:    prettyHooks,
		FunctionsLua: doc.FunctionsLua,
		ContentHash:  hashHex,
	}

	s.mu.Lock()
	s.activeDoc = updated
	s.hashes[cleanPath] = hashHex
	s.revision++
	s.mu.Unlock()

	return updated, nil
}

func (s *ScriptEditorService) resolveTarget(target string) (resolvedPath string, stem string, exists bool) {
	s.mu.RLock()
	scriptsDir := s.ScriptsDir
	s.mu.RUnlock()
	if scriptsDir == "" {
		scriptsDir = defaultEditorScriptsDir
	}

	isExplicitPath := strings.ContainsAny(target, `/\`) || strings.HasSuffix(strings.ToLower(target), ".lua")
	if isExplicitPath {
		clean := filepath.Clean(target)
		if !strings.HasSuffix(strings.ToLower(clean), ".lua") {
			clean += ".lua"
		}
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			return clean, stemFromPath(clean), true
		}
		joined := filepath.Join(scriptsDir, clean)
		if info, err := os.Stat(joined); err == nil && !info.IsDir() {
			return joined, stemFromPath(joined), true
		}
		return clean, stemFromPath(clean), false
	}

	if s.Bridge != nil {
		if def := s.Bridge.getCustomComponentDef(target); def != nil && strings.TrimSpace(def.SourcePath) != "" {
			if info, err := os.Stat(def.SourcePath); err == nil && !info.IsDir() {
				return filepath.Clean(def.SourcePath), def.Name, true
			}
		}
	}

	if found, ok := findScriptByStem(scriptsDir, target); ok {
		return found, stemFromPath(found), true
	}

	defaultPath := filepath.Join(scriptsDir, "components", target+".lua")
	return defaultPath, target, false
}

func findScriptByStem(root, targetStem string) (string, bool) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", false
	}
	want := strings.TrimSuffix(strings.TrimSpace(targetStem), ".lua")
	var matches []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".lua") {
			return nil
		}
		if strings.EqualFold(stemFromPath(path), want) {
			matches = append(matches, filepath.Clean(path))
		}
		return nil
	})
	if len(matches) == 0 {
		return "", false
	}
	sort.Strings(matches)
	return matches[0], true
}

func stemFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (s *ScriptEditorService) parseScriptDocument(stem, path string, raw []byte) engine.ScriptEditorDocument {
	sum := sha256.Sum256(raw)
	hashHex := hex.EncodeToString(sum[:])

	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	var dataJSON, hooksJSON string
	var bodyLines []string
	inComponentBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- @data:") {
			rawJSON := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- @data:"))
			if formatted, err := formatJSONString(rawJSON); err == nil {
				dataJSON = formatted
			} else {
				dataJSON = rawJSON
			}
			continue
		}
		if strings.HasPrefix(trimmed, "-- @hooks:") {
			rawJSON := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- @hooks:"))
			if formatted, err := formatJSONString(rawJSON); err == nil {
				hooksJSON = formatted
			} else {
				hooksJSON = rawJSON
			}
			continue
		}
		if strings.HasPrefix(trimmed, "-- @component:") {
			inComponentBlock = true
			continue
		}
		if inComponentBlock {
			if trimmed == "-- @end-component" {
				inComponentBlock = false
			}
			continue
		}
		bodyLines = append(bodyLines, line)
	}

	for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[0]) == "" {
		bodyLines = bodyLines[1:]
	}
	functionsLua := strings.Join(bodyLines, "\n")

	if dataJSON == "" || hooksJSON == "" {
		if def := s.Bridge.getCustomComponentDef(stem); def != nil {
			if dataJSON == "" {
				dataJSON = s.defDefaultsToJSON(def)
			}
			if hooksJSON == "" {
				hooksJSON = s.defHooksToJSON(def)
			}
		}
	}
	if dataJSON == "" {
		dataJSON = "{}"
	}
	if hooksJSON == "" {
		hooksJSON = "{}"
	}

	return engine.ScriptEditorDocument{
		Name:         stem,
		FilePath:     filepath.Clean(path),
		DataJSON:     dataJSON,
		HooksJSON:    hooksJSON,
		FunctionsLua: functionsLua,
		ContentHash:  hashHex,
	}
}

func (s *ScriptEditorService) newScriptTemplate(stem, path string) engine.ScriptEditorDocument {
	dataJSON := "{}"
	hooksJSON := "{\n  \"Update\": [\n    \"number\"\n  ]\n}"
	functionsLua := fmt.Sprintf("function %s:Update(dt)\n  -- runtime update\nend\n", sanitizeLuaIdent(stem))

	if def := s.Bridge.getCustomComponentDef(stem); def != nil {
		dataJSON = s.defDefaultsToJSON(def)
		hooksJSON = s.defHooksToJSON(def)
		methods := collectProtoMethodNames(def.Proto)
		if len(methods) > 0 {
			var sb strings.Builder
			for _, m := range methods {
				sb.WriteString(fmt.Sprintf("-- Registered method on %s: %s\n", def.Name, m))
			}
			functionsLua = sb.String()
		}
	}

	return engine.ScriptEditorDocument{
		Name:         stem,
		FilePath:     filepath.Clean(path),
		DataJSON:     dataJSON,
		HooksJSON:    hooksJSON,
		FunctionsLua: functionsLua,
	}
}

func (s *ScriptEditorService) defDefaultsToJSON(def *LuaComponentDef) string {
	if def == nil || len(def.FieldNames) == 0 {
		return "{}"
	}
	m := make(map[string]any, len(def.FieldNames))
	for _, k := range def.FieldNames {
		m[k] = luaValueToGo(def.Defaults[k])
	}
	out, err := formatJSONMap(m)
	if err != nil {
		return "{}"
	}
	return out
}

func (s *ScriptEditorService) defHooksToJSON(def *LuaComponentDef) string {
	if def == nil || len(def.HookNames) == 0 {
		return "{}"
	}
	m := make(map[string]any, len(def.HookNames))
	for _, h := range def.HookNames {
		m[h] = def.Hooks[h]
	}
	out, err := formatJSONMap(m)
	if err != nil {
		return "{}"
	}
	return out
}

func (s *ScriptEditorService) synthesizeScriptSource(
	doc engine.ScriptEditorDocument,
	compactData, compactHooks string,
	dataMap, hooksMap map[string]any,
) string {
	var sb strings.Builder
	sb.WriteString("-- @data: " + compactData + "\n")
	sb.WriteString("-- @hooks: " + compactHooks + "\n")

	stem := sanitizeLuaIdent(doc.Name)
	if stem == "" {
		stem = sanitizeLuaIdent(stemFromPath(doc.FilePath))
	}

	needsComponentHeader := s.shouldEmitComponentHeader(doc, stem)
	if needsComponentHeader && stem != "" {
		sb.WriteString("-- @component: " + stem + "\n")
		sb.WriteString(fmt.Sprintf("%s = Component(%q, %s, %s)\n",
			stem, stem, goMapToLuaTableLiteral(dataMap, false), hooksMapToLuaTableLiteral(hooksMap)))
		sb.WriteString("-- @end-component\n")
	}

	sb.WriteString("\n")
	body := strings.TrimLeft(strings.ReplaceAll(doc.FunctionsLua, "\r\n", "\n"), "\n")
	sb.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}

func (s *ScriptEditorService) shouldEmitComponentHeader(doc engine.ScriptEditorDocument, stem string) bool {
	body := doc.FunctionsLua
	if strings.Contains(body, "Component(") {
		return false
	}
	if strings.Contains(body, "local "+stem+" ") || strings.Contains(body, "local "+stem+"=") {
		return false
	}
	// Module-style scripts (`local M = {} ... return M`) manage their own table.
	if strings.Contains(body, "\nreturn ") || strings.HasPrefix(strings.TrimSpace(body), "return ") {
		return false
	}
	if stem != "" && (strings.Contains(body, "function "+stem+":") || strings.Contains(body, "function "+stem+".")) {
		return true
	}
	clean := filepath.ToSlash(strings.ToLower(doc.FilePath))
	if strings.Contains(clean, "/components/") || strings.HasPrefix(clean, "components/") {
		return true
	}
	return s.Bridge != nil && s.Bridge.getCustomComponentDef(stem) != nil
}

func (s *ScriptEditorService) hotReloadFile(path, name string, dataMap, hooksMap map[string]any) error {
	b := s.Bridge
	b.stateMu.Lock()
	prevLoading := b.loadingScriptPath
	b.loadingScriptPath = path
	err := b.L.DoFile(path)
	b.loadingScriptPath = prevLoading
	if err != nil {
		b.stateMu.Unlock()
		return fmt.Errorf("script editor: execute %q: %w", path, err)
	}

	compName := strings.TrimSpace(name)
	if compName == "" {
		compName = stemFromPath(path)
	}
	def := b.getCustomComponentDef(compName)
	if def != nil {
		def.SourcePath = path
		syncDefFromEditorMaps(b.L, def, dataMap, hooksMap)
	}

	allObjects := b.collectAllTrackedObjects()
	for _, obj := range allObjects {
		for _, comp := range obj.Components {
			switch c := comp.(type) {
			case *LuaComponent:
				if def != nil && strings.EqualFold(c.TypeName, def.Name) {
					mergeLuaComponentDefaults(b.L, c, def)
					b.unregisterLuaComponentHooks(obj, c)
					for _, h := range def.HookNames {
						b.registerLuaComponentHook(obj, h, c)
					}
				}
			case *ScriptComponent:
				if filepath.Clean(c.ScriptPath) == filepath.Clean(path) {
					_ = c.loadFromFile(path)
				}
			}
		}
	}
	b.stateMu.Unlock()
	return nil
}

func (b *LuaBridge) collectAllTrackedObjects() []*engine.GameObject {
	seen := make(map[*engine.GameObject]bool)
	var out []*engine.GameObject

	b.dataMu.RLock()
	for _, obj := range b.objects {
		if obj != nil && !seen[obj] {
			seen[obj] = true
			out = append(out, obj)
		}
	}
	scene := b.scene
	b.dataMu.RUnlock()

	if scene != nil {
		for _, obj := range scene.GetObjects() {
			if obj != nil && !seen[obj] {
				seen[obj] = true
				out = append(out, obj)
			}
		}
	}
	return out
}

func syncDefFromEditorMaps(L *lua.LState, def *LuaComponentDef, dataMap, hooksMap map[string]any) {
	defaults := make(map[string]lua.LValue, len(dataMap))
	fieldNames := make([]string, 0, len(dataMap))
	for k, v := range dataMap {
		defaults[k] = goValueToLua(L, v)
		fieldNames = append(fieldNames, k)
	}
	sort.Strings(fieldNames)

	hooks := make(map[string][]string, len(hooksMap))
	hookNames := make([]string, 0, len(hooksMap))
	for hookName, rawSig := range hooksMap {
		switch sv := rawSig.(type) {
		case bool:
			if !sv {
				continue
			}
			hooks[hookName] = []string{}
			hookNames = append(hookNames, hookName)
		case []any:
			sig := make([]string, 0, len(sv))
			for _, item := range sv {
				if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
					sig = append(sig, canonicalHookTypeName(str))
				}
			}
			hooks[hookName] = sig
			hookNames = append(hookNames, hookName)
		}
	}
	sort.Strings(hookNames)

	def.Defaults = defaults
	def.FieldNames = fieldNames
	def.Hooks = hooks
	def.HookNames = hookNames
}

func mergeLuaComponentDefaults(L *lua.LState, lc *LuaComponent, def *LuaComponentDef) {
	if lc == nil || def == nil {
		return
	}
	if lc.Fields == nil {
		lc.Fields = L.NewTable()
	}
	for _, k := range def.FieldNames {
		if lc.Fields.RawGetString(k) == lua.LNil {
			lc.Fields.RawSetString(k, cloneLuaValue(L, def.Defaults[k]))
		}
	}
}

func compileLuaChunk(L *lua.LState, source, chunkName string) error {
	fn, err := L.LoadString(source)
	if err != nil {
		return fmt.Errorf("%s: Lua syntax error: %w", chunkName, err)
	}
	if fn == nil {
		return fmt.Errorf("%s: failed compiling Lua chunk", chunkName)
	}
	return nil
}

func parseJSONObject(raw, label string) (map[string]any, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}, "{}", nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, "", fmt.Errorf("%s: invalid JSON syntax", label)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil || m == nil {
		return nil, "", fmt.Errorf("%s: expected JSON object", label)
	}
	compact, err := json.Marshal(m)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", label, err)
	}
	return m, string(compact), nil
}

func parseHooksJSONObject(raw string) (map[string]any, string, error) {
	m, compact, err := parseJSONObject(raw, "Hooks (JSON)")
	if err != nil {
		return nil, "", err
	}
	for hookName, val := range m {
		switch tv := val.(type) {
		case bool:
			// valid boolean toggle
		case []any:
			for i, item := range tv {
				if _, ok := item.(string); !ok {
					return nil, "", fmt.Errorf("Hooks (JSON): hook %q arg %d must be a type name string", hookName, i+1)
				}
			}
		default:
			return nil, "", fmt.Errorf("Hooks (JSON): hook %q must be boolean or array of type name strings", hookName)
		}
	}
	return m, compact, nil
}

func formatJSONString(raw string) (string, error) {
	m, _, err := parseJSONObject(raw, "JSON")
	if err != nil {
		return "", err
	}
	return formatJSONMap(m)
}

func formatJSONMap(m map[string]any) (string, error) {
	if m == nil {
		return "{}", nil
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func goMapToLuaTableLiteral(m map[string]any, hooksMode bool) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("[%q] = %s", k, goValueToLuaLiteral(m[k])))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func hooksMapToLuaTableLiteral(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		switch v := m[k].(type) {
		case bool:
			if !v {
				continue
			}
			parts = append(parts, fmt.Sprintf("[%q] = {}", k))
		case []any:
			parts = append(parts, fmt.Sprintf("[%q] = %s", k, goValueToLuaLiteral(v)))
		}
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func goValueToLuaLiteral(val any) string {
	switch v := val.(type) {
	case nil:
		return "nil"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case string:
		return fmt.Sprintf("%q", v)
	case []any:
		elems := make([]string, 0, len(v))
		for _, item := range v {
			elems = append(elems, goValueToLuaLiteral(item))
		}
		return "{ " + strings.Join(elems, ", ") + " }"
	case map[string]any:
		return goMapToLuaTableLiteral(v, false)
	default:
		return fmt.Sprintf("%q", fmt.Sprint(v))
	}
}

func sanitizeLuaIdent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var sb strings.Builder
	for i, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isLetter {
			sb.WriteByte('_')
			if isDigit {
				sb.WriteRune(r)
			}
			continue
		}
		if isLetter || isDigit {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	return sb.String()
}
