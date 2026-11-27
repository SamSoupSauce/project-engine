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
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

const defaultPrefabsDir = "prefabs"

// PrefabSpawnOptions configures how a PrefabSpec is instantiated into a GameObject.
type PrefabSpawnOptions struct {
	ID          string
	Name        string
	Position    *engine.Vector3
	AutoAdd     bool
	PickSpawned bool
}

// PrefabService manages JSON Prefab storage under PrefabsDir, GameObject serialization/spawning,
// and Dear ImGui PrefabEditorWindow synchronization.
type PrefabService struct {
	Bridge     *LuaBridge
	PrefabsDir string

	mu        sync.RWMutex
	open      bool
	activeDoc engine.PrefabDocument
	revision  uint64
	spawnSeq  int
}

// NewPrefabService creates a PrefabService bound to bridge.
func NewPrefabService(bridge *LuaBridge) *PrefabService {
	return &PrefabService{
		Bridge:     bridge,
		PrefabsDir: defaultPrefabsDir,
	}
}

// PrefabEditor returns the LuaBridge's PrefabService for attaching to engine.ImGuiOverlay.
func (b *LuaBridge) PrefabEditor() *PrefabService {
	if b == nil {
		return nil
	}
	return b.prefabService
}

// SetPrefabsDir overrides the storage directory used for prefabs (defaults to "prefabs").
func (s *PrefabService) SetPrefabsDir(dir string) {
	if s == nil || strings.TrimSpace(dir) == "" {
		return
	}
	s.mu.Lock()
	s.PrefabsDir = dir
	s.mu.Unlock()
}

// GetPrefabsDir returns the configured prefab storage directory.
func (s *PrefabService) GetPrefabsDir() string {
	if s == nil {
		return defaultPrefabsDir
	}
	s.mu.RLock()
	dir := s.PrefabsDir
	s.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		return defaultPrefabsDir
	}
	return dir
}

// IsPrefabEditorOpen implements engine.PrefabEditorBackend and engine.DebugConsolePrefabUI.
func (s *PrefabService) IsPrefabEditorOpen() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.open
}

// SetPrefabEditorOpen implements engine.PrefabEditorBackend and engine.DebugConsolePrefabUI.
func (s *PrefabService) SetPrefabEditorOpen(open bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.open = open
	s.revision++
	s.mu.Unlock()
}

// GetActivePrefabDocument implements engine.PrefabEditorBackend.
func (s *PrefabService) GetActivePrefabDocument() engine.PrefabDocument {
	if s == nil {
		return engine.PrefabDocument{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeDoc
}

// Revision implements engine.PrefabEditorBackend.
func (s *PrefabService) Revision() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

// HasSelectedObject implements engine.PrefabEditorBackend.
func (s *PrefabService) HasSelectedObject() (string, string, bool) {
	if s == nil || s.Bridge == nil {
		return "", "", false
	}
	obj := s.Bridge.GetPickedObject()
	if obj == nil {
		return "", "", false
	}
	return obj.ID, obj.Name, true
}

// AvailableComponentNames implements engine.PrefabEditorBackend.
func (s *PrefabService) AvailableComponentNames() []string {
	if s == nil || s.Bridge == nil {
		return nil
	}
	results := s.Bridge.SearchComponents("")
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.Name)
	}
	return names
}

// SearchPrefabs scans PrefabsDir for *.json prefabs matching query across name, path, and components.
func (s *PrefabService) SearchPrefabs(query string) ([]engine.PrefabSummary, error) {
	dir := s.GetPrefabsDir()
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("prefab: stat %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("prefab: %q is not a directory", dir)
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var results []engine.PrefabSummary
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var spec engine.PrefabSpec
		if unmarshalErr := json.Unmarshal(raw, &spec); unmarshalErr != nil {
			return nil
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = stemFromPath(path)
		}
		compNames := make([]string, 0, len(spec.Components))
		for _, c := range spec.Components {
			if c.Name != "" {
				compNames = append(compNames, c.Name)
			} else if c.Type != "" {
				compNames = append(compNames, c.Type)
			}
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		relSlash := filepath.ToSlash(rel)

		if q != "" {
			matched := strings.Contains(strings.ToLower(name), q) ||
				strings.Contains(strings.ToLower(stemFromPath(path)), q) ||
				strings.Contains(strings.ToLower(relSlash), q)
			if !matched {
				for _, cn := range compNames {
					if strings.Contains(strings.ToLower(cn), q) {
						matched = true
						break
					}
				}
			}
			if !matched {
				return nil
			}
		}

		results = append(results, engine.PrefabSummary{
			Name:           name,
			FilePath:       filepath.Clean(path),
			RelativePath:   relSlash,
			Active:         spec.Active,
			ComponentNames: compNames,
			ChildCount:     len(spec.Children),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prefab: walk %q: %w", dir, err)
	}
	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].RelativePath) < strings.ToLower(results[j].RelativePath)
	})
	return results, nil
}

// OpenPrefab resolves target (prefab name, stem, or .json path), loads it or creates a new template,
// and marks the PrefabEditorWindow open.
func (s *PrefabService) OpenPrefab(target string) (engine.PrefabDocument, error) {
	if s == nil {
		return engine.PrefabDocument{}, fmt.Errorf("prefab: service is nil")
	}
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		s.SetPrefabEditorOpen(true)
		return s.GetActivePrefabDocument(), nil
	}

	resolvedPath, stem, exists := s.resolvePrefabTarget(trimmed)
	var doc engine.PrefabDocument
	if exists {
		raw, err := os.ReadFile(resolvedPath)
		if err != nil {
			return engine.PrefabDocument{}, fmt.Errorf("prefab: read %q: %w", resolvedPath, err)
		}
		parsed, err := parsePrefabDocument(stem, resolvedPath, raw)
		if err != nil {
			return engine.PrefabDocument{}, err
		}
		doc = parsed
	} else {
		spec := defaultNewPrefabSpec(stem)
		doc = buildPrefabDocument(stem, resolvedPath, spec, "")
	}

	s.mu.Lock()
	s.activeDoc = doc
	s.open = true
	s.revision++
	s.mu.Unlock()
	return doc, nil
}

// ValidatePrefab validates doc.RawJSON against JSON syntax and PrefabSpec schema.
func (s *PrefabService) ValidatePrefab(doc engine.PrefabDocument) error {
	trimmed := strings.TrimSpace(doc.RawJSON)
	if trimmed == "" {
		return fmt.Errorf("prefab: JSON cannot be empty")
	}
	if !json.Valid([]byte(trimmed)) {
		return fmt.Errorf("prefab: invalid JSON syntax")
	}
	var spec engine.PrefabSpec
	if err := json.Unmarshal([]byte(trimmed), &spec); err != nil {
		return fmt.Errorf("prefab: invalid GameObject JSON: %w", err)
	}
	if strings.TrimSpace(spec.Name) == "" && strings.TrimSpace(doc.Name) == "" {
		return fmt.Errorf("prefab: GameObject name must not be empty")
	}
	return nil
}

// CommitPrefab validates and writes doc.RawJSON to disk under PrefabsDir.
func (s *PrefabService) CommitPrefab(doc engine.PrefabDocument) (engine.PrefabDocument, error) {
	if s == nil {
		return doc, fmt.Errorf("prefab: service is nil")
	}
	if err := s.ValidatePrefab(doc); err != nil {
		return doc, err
	}

	var spec engine.PrefabSpec
	if err := json.Unmarshal([]byte(doc.RawJSON), &spec); err != nil {
		return doc, fmt.Errorf("prefab: parse JSON: %w", err)
	}
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = doc.Name
	}
	if spec.Name == "" {
		spec.Name = stemFromPath(doc.FilePath)
	}
	if spec.Components == nil {
		spec.Components = []engine.PrefabComponentData{}
	}

	targetPath := strings.TrimSpace(doc.FilePath)
	if targetPath == "" {
		targetPath = filepath.Join(s.GetPrefabsDir(), sanitizeFileStem(spec.Name)+".json")
	}
	cleanPath := filepath.Clean(targetPath)

	if dir := filepath.Dir(cleanPath); dir != "" && dir != "." {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return doc, fmt.Errorf("prefab: create dir %q: %w", dir, err)
			}
		}
	}

	pretty, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return doc, fmt.Errorf("prefab: encode JSON: %w", err)
	}
	rawBytes := append(pretty, '\n')
	if err := os.WriteFile(cleanPath, rawBytes, 0o644); err != nil {
		return doc, fmt.Errorf("prefab: write %q: %w", cleanPath, err)
	}

	sum := sha256.Sum256(rawBytes)
	hashHex := hex.EncodeToString(sum[:])
	updated := buildPrefabDocument(stemFromPath(cleanPath), cleanPath, spec, hashHex)

	s.mu.Lock()
	s.activeDoc = updated
	s.revision++
	s.mu.Unlock()
	return updated, nil
}

// DeletePrefab removes a prefab JSON file from PrefabsDir.
func (s *PrefabService) DeletePrefab(target string) error {
	if s == nil {
		return fmt.Errorf("prefab: service is nil")
	}
	resolvedPath, _, exists := s.resolvePrefabTarget(strings.TrimSpace(target))
	if !exists {
		return fmt.Errorf("prefab %q not found", target)
	}
	if err := os.Remove(resolvedPath); err != nil {
		return fmt.Errorf("prefab: delete %q: %w", resolvedPath, err)
	}

	s.mu.Lock()
	if filepath.Clean(s.activeDoc.FilePath) == filepath.Clean(resolvedPath) {
		s.activeDoc = engine.PrefabDocument{}
	}
	s.revision++
	s.mu.Unlock()
	return nil
}

// LoadPrefabSpec reads and parses a prefab JSON file by name, stem, or path.
func (s *PrefabService) LoadPrefabSpec(target string) (engine.PrefabSpec, string, error) {
	resolvedPath, _, exists := s.resolvePrefabTarget(strings.TrimSpace(target))
	if !exists {
		return engine.PrefabSpec{}, "", fmt.Errorf("prefab %q not found", target)
	}
	raw, err := os.ReadFile(resolvedPath)
	if err != nil {
		return engine.PrefabSpec{}, "", fmt.Errorf("prefab: read %q: %w", resolvedPath, err)
	}
	var spec engine.PrefabSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return engine.PrefabSpec{}, "", fmt.Errorf("prefab: parse %q: %w", resolvedPath, err)
	}
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = stemFromPath(resolvedPath)
	}
	return spec, filepath.Clean(resolvedPath), nil
}

// SaveGameObjectAsPrefab serializes obj into a JSON prefab file under PrefabsDir.
func (s *PrefabService) SaveGameObjectAsPrefab(obj *engine.GameObject, name string) (engine.PrefabDocument, error) {
	if s == nil {
		return engine.PrefabDocument{}, fmt.Errorf("prefab: service is nil")
	}
	if obj == nil {
		return engine.PrefabDocument{}, fmt.Errorf("prefab: GameObject must not be nil")
	}
	spec := GameObjectToPrefabSpec(obj)
	prefabName := strings.TrimSpace(name)
	if prefabName == "" {
		prefabName = strings.TrimSpace(obj.Name)
	}
	if prefabName == "" {
		prefabName = strings.TrimSpace(obj.ID)
	}
	if prefabName == "" {
		prefabName = "Prefab"
	}

	resolvedPath, stem, _ := s.resolvePrefabTarget(prefabName)
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = stem
	}
	doc := buildPrefabDocument(stem, resolvedPath, spec, "")
	return s.CommitPrefab(doc)
}

// CreatePrefabFromSelected implements engine.PrefabEditorBackend.
func (s *PrefabService) CreatePrefabFromSelected(name string) (engine.PrefabDocument, error) {
	if s == nil || s.Bridge == nil {
		return engine.PrefabDocument{}, fmt.Errorf("prefab: bridge is nil")
	}
	obj := s.Bridge.GetPickedObject()
	if obj == nil {
		return engine.PrefabDocument{}, fmt.Errorf("no GameObject currently selected (use pick() first)")
	}
	return s.SaveGameObjectAsPrefab(obj, name)
}

// SpawnPrefab implements engine.PrefabEditorBackend, instantiating target into the scene.
func (s *PrefabService) SpawnPrefab(target string) (string, error) {
	obj, err := s.SpawnPrefabObject(target, PrefabSpawnOptions{AutoAdd: true})
	if err != nil {
		return "", err
	}
	return obj.ID, nil
}

// SpawnPrefabObject loads target prefab JSON and instantiates a live GameObject (and children).
func (s *PrefabService) SpawnPrefabObject(target string, opts PrefabSpawnOptions) (*engine.GameObject, error) {
	if s == nil || s.Bridge == nil {
		return nil, fmt.Errorf("prefab: bridge is nil")
	}
	spec, _, err := s.LoadPrefabSpec(target)
	if err != nil {
		return nil, err
	}
	return s.instantiateSpecRecursive(spec, opts, true)
}

func (s *PrefabService) instantiateSpecRecursive(spec engine.PrefabSpec, opts PrefabSpawnOptions, isRoot bool) (*engine.GameObject, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" || !isRoot {
		name = strings.TrimSpace(spec.Name)
	}
	if name == "" {
		name = "PrefabObject"
	}

	id := strings.TrimSpace(opts.ID)
	if id == "" || !isRoot {
		id = s.nextUniqueID(name, spec.ID)
	}

	obj := engine.NewGameObject(id, name)
	obj.Active = spec.Active

	ordered := orderPrefabComponents(spec.Components)
	for _, compSpec := range ordered {
		if err := s.attachComponentFromSpec(obj, compSpec); err != nil {
			return nil, fmt.Errorf("prefab %q component %q: %w", name, compSpec.Name, err)
		}
	}

	if isRoot && opts.Position != nil {
		obj.EnsureTransform().SetPosition(*opts.Position)
	}

	for _, childSpec := range spec.Children {
		child, err := s.instantiateSpecRecursive(childSpec, PrefabSpawnOptions{}, false)
		if err != nil {
			return nil, err
		}
		obj.AddChild(child)
	}

	if isRoot && opts.AutoAdd {
		s.Bridge.RegisterGameObject(obj.ID, obj)
		s.Bridge.dataMu.RLock()
		scene := s.Bridge.scene
		s.Bridge.dataMu.RUnlock()
		if scene != nil {
			scene.AddObject(obj)
			if scene.IsLoaded() && obj.Active {
				scene.Spawn(obj, nil)
			}
		}
		if opts.PickSpawned {
			s.Bridge.dataMu.RLock()
			console := s.Bridge.console
			s.Bridge.dataMu.RUnlock()
			if console != nil {
				console.PickObject(obj)
			} else {
				s.Bridge.SetPickedObject(obj)
			}
		}
	}
	return obj, nil
}

func (s *PrefabService) nextUniqueID(name, preferredID string) string {
	base := sanitizeFileStem(preferredID)
	if base == "" {
		base = sanitizeFileStem(name)
	}
	if base == "" {
		base = "prefab"
	}
	base = strings.ToLower(base)

	s.mu.Lock()
	s.spawnSeq++
	seq := s.spawnSeq
	s.mu.Unlock()

	candidate := fmt.Sprintf("%s_%d", base, seq)
	for s.Bridge.findSceneOrRegisteredObject(candidate) != nil {
		s.mu.Lock()
		s.spawnSeq++
		seq = s.spawnSeq
		s.mu.Unlock()
		candidate = fmt.Sprintf("%s_%d", base, seq)
	}
	return candidate
}

func (s *PrefabService) attachComponentFromSpec(obj *engine.GameObject, spec engine.PrefabComponentData) error {
	nameHint := strings.TrimSpace(spec.Name)
	if nameHint == "" {
		nameHint = strings.TrimSpace(spec.Type)
	}
	canon := resolveComponentAlias(nameHint)
	details := spec.Details

	switch canon {
	case "Transform":
		tc := obj.GetTransformComponent()
		if tc == nil {
			tc = engine.NewTransformComponent()
			obj.AddComponent(tc)
		}
		if p, ok := detailVec3(details, "position"); ok {
			tc.SetPosition(p)
		}
		if r, ok := detailVec3(details, "rotation"); ok {
			tc.SetRotation(r)
		}
		if sc, ok := detailVec3(details, "scale"); ok {
			tc.SetScale(sc)
		}
		return nil

	case "Mesh":
		path := detailString(details, "file_path")
		mc := engine.NewMeshComponent(path)
		if mc.LoadErr != nil {
			return mc.LoadErr
		}
		obj.AddComponent(mc)
		return nil

	case "MeshRenderable":
		if obj.GetTransformComponent() == nil {
			obj.AddComponent(engine.NewTransformComponent())
		}
		mr := engine.NewMeshRenderable()
		if v, ok := detailBool(details, "visible"); ok {
			mr.Visible = v
		}
		if wf, ok := detailBool(details, "wireframe"); ok {
			mr.Wireframe = wf
		}
		obj.AddComponent(mr)
		return nil

	case "Renderable":
		if obj.GetTransformComponent() == nil {
			obj.AddComponent(engine.NewTransformComponent())
		}
		meshID := detailString(details, "mesh_id")
		if meshID == "" {
			meshID = "cube"
		}
		matID := detailString(details, "material_id")
		if matID == "" {
			matID = "default"
		}
		r := engine.NewRenderable(meshID, matID)
		if sn := detailString(details, "script_name"); sn != "" {
			r.ScriptName = sn
		}
		if v, ok := detailBool(details, "visible"); ok {
			r.SetVisible(v)
		}
		if o, ok := detailBool(details, "outlined"); ok {
			r.SetOutlined(o)
		}
		if f, ok := detailFloat(details, "flags"); ok {
			r.Flags = engine.RenderFlags(uint32(f))
		}
		obj.AddComponent(r)
		return nil

	case "Renderable2D":
		texID := uint32(detailFloatDefault(details, "texture_id", 0))
		w := float32(detailFloatDefault(details, "width", 64))
		h := float32(detailFloatDefault(details, "height", 64))
		r2d := engine.NewRenderable2D(texID, w, h)
		if z, ok := detailFloat(details, "z_index"); ok {
			r2d.ZIndex = int32(z)
		}
		if v, ok := detailBool(details, "visible"); ok {
			r2d.SetVisible(v)
		}
		obj.AddComponent(r2d)
		return nil

	case "InteractiveComponent":
		disp := detailString(details, "display_name")
		if disp == "" {
			disp = obj.Name
		}
		ic := spatial.NewInteractiveComponent(disp, false)
		if en, ok := detailBool(details, "enabled"); ok {
			ic.Enabled = en
		}
		obj.AddComponent(ic)
		return nil

	case "FractureComponent":
		mode := spatial.FractureMode(int(detailFloatDefault(details, "mode", float64(spatial.FractureSplinter))))
		hp := float32(detailFloatDefault(details, "health", 15.0))
		fc := spatial.NewFractureComponent(mode, spatial.IntegrityStandard, hp)
		if hands, ok := detailFloat(details, "carry_hands"); ok {
			fc.RequiredCarryHands = int(hands)
		}
		if broken, ok := detailBool(details, "broken"); ok {
			fc.Broken = broken
		}
		obj.AddComponent(fc)
		return nil

	case "ScriptComponent":
		scriptPath := detailString(details, "script_path")
		if scriptPath == "" {
			return fmt.Errorf("ScriptComponent requires details.script_path")
		}
		sc, err := s.Bridge.NewScriptComponent(scriptPath)
		if err != nil {
			return err
		}
		if sn := detailString(details, "script_name"); sn != "" {
			sc.ScriptName = sn
		}
		obj.AddComponent(sc)
		return nil

	default:
		if s.Bridge != nil {
			def := s.Bridge.getCustomComponentDef(nameHint)
			if def == nil {
				// Attempt lazy load from scriptsDir if script exists on disk.
				if scriptsDir := s.Bridge.effectiveScriptsDir(); scriptsDir != "" {
					if foundPath, ok := findScriptByStem(scriptsDir, nameHint); ok {
						s.Bridge.stateMu.Lock()
						prevLoading := s.Bridge.loadingScriptPath
						s.Bridge.loadingScriptPath = foundPath
						_ = s.Bridge.L.DoFile(foundPath)
						s.Bridge.loadingScriptPath = prevLoading
						s.Bridge.stateMu.Unlock()
						def = s.Bridge.getCustomComponentDef(nameHint)
					}
				}
			}
			if def != nil {
				s.Bridge.stateMu.Lock()
				var overrides *lua.LTable
				if len(details) > 0 {
					overrides = s.Bridge.L.NewTable()
					for k, v := range details {
						overrides.RawSetString(k, goValueToLua(s.Bridge.L, v))
					}
				}
				lc := s.Bridge.newLuaComponent(def, overrides)
				s.Bridge.stateMu.Unlock()
				obj.AddComponent(lc)
				return nil
			}
		}
		return fmt.Errorf("unsupported or unknown component %q", nameHint)
	}
}

func (s *PrefabService) resolvePrefabTarget(target string) (resolvedPath string, stem string, exists bool) {
	dir := s.GetPrefabsDir()
	isPath := strings.ContainsAny(target, `/\`) || strings.HasSuffix(strings.ToLower(target), ".json")
	if isPath {
		clean := filepath.Clean(target)
		if !strings.HasSuffix(strings.ToLower(clean), ".json") {
			clean += ".json"
		}
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			return clean, stemFromPath(clean), true
		}
		joined := filepath.Join(dir, clean)
		if info, err := os.Stat(joined); err == nil && !info.IsDir() {
			return joined, stemFromPath(joined), true
		}
		if !filepath.IsAbs(clean) && !strings.HasPrefix(filepath.ToSlash(clean), filepath.ToSlash(filepath.Clean(dir))+"/") {
			return filepath.Clean(joined), stemFromPath(joined), false
		}
		return clean, stemFromPath(clean), false
	}

	if found, ok := findPrefabByStem(dir, target); ok {
		return found, stemFromPath(found), true
	}

	fileName := sanitizeFileStem(target)
	if fileName == "" {
		fileName = "prefab"
	}
	return filepath.Join(dir, fileName+".json"), target, false
}

func findPrefabByStem(dir, target string) (string, bool) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	want := strings.TrimSuffix(strings.TrimSpace(target), ".json")
	var matches []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
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

// GameObjectToPrefabSpec converts a live GameObject hierarchy into a serializable PrefabSpec.
func GameObjectToPrefabSpec(obj *engine.GameObject) engine.PrefabSpec {
	if obj == nil {
		return engine.PrefabSpec{Active: true, Components: []engine.PrefabComponentData{}}
	}
	comps := make([]engine.PrefabComponentData, 0, len(obj.Components))
	for _, c := range obj.Components {
		if c == nil {
			continue
		}
		summary := buildComponentSummaryJSON(c)
		comps = append(comps, engine.PrefabComponentData{
			Name:    summary.Name,
			Type:    summary.Type,
			Details: summary.Details,
		})
	}
	children := make([]engine.PrefabSpec, 0, len(obj.Children))
	for _, ch := range obj.Children {
		if ch != nil {
			children = append(children, GameObjectToPrefabSpec(ch))
		}
	}
	return engine.PrefabSpec{
		Name:       obj.Name,
		Active:     obj.Active,
		Components: comps,
		Children:   children,
	}
}

func parsePrefabDocument(stem, path string, raw []byte) (engine.PrefabDocument, error) {
	var spec engine.PrefabSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return engine.PrefabDocument{}, fmt.Errorf("prefab: parse %q: %w", path, err)
	}
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = stem
	}
	sum := sha256.Sum256(raw)
	return buildPrefabDocument(stem, path, spec, hex.EncodeToString(sum[:])), nil
}

func buildPrefabDocument(stem, path string, spec engine.PrefabSpec, hashHex string) engine.PrefabDocument {
	if spec.Components == nil {
		spec.Components = []engine.PrefabComponentData{}
	}
	pretty, err := json.MarshalIndent(spec, "", "  ")
	rawJSON := "{}\n"
	if err == nil {
		rawJSON = string(pretty) + "\n"
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = stem
	}
	return engine.PrefabDocument{
		Name:        name,
		FilePath:    filepath.Clean(path),
		RawJSON:     rawJSON,
		ContentHash: hashHex,
		Spec:        spec,
	}
}

func defaultNewPrefabSpec(name string) engine.PrefabSpec {
	if strings.TrimSpace(name) == "" {
		name = "NewPrefab"
	}
	return engine.PrefabSpec{
		Name:   name,
		Active: true,
		Components: []engine.PrefabComponentData{
			{
				Name: "Transform",
				Type: "*engine.TransformComponent",
				Details: map[string]any{
					"position": map[string]any{"x": 0.0, "y": 0.0, "z": 0.0},
					"rotation": map[string]any{"x": 0.0, "y": 0.0, "z": 0.0},
					"scale":    map[string]any{"x": 1.0, "y": 1.0, "z": 1.0},
				},
			},
		},
	}
}

func orderPrefabComponents(comps []engine.PrefabComponentData) []engine.PrefabComponentData {
	priority := func(c engine.PrefabComponentData) int {
		name := c.Name
		if name == "" {
			name = c.Type
		}
		switch resolveComponentAlias(name) {
		case "Transform":
			return 0
		case "Mesh":
			return 1
		case "MeshRenderable", "Renderable":
			return 3
		default:
			return 2
		}
	}
	ordered := make([]engine.PrefabComponentData, len(comps))
	copy(ordered, comps)
	sort.SliceStable(ordered, func(i, j int) bool {
		return priority(ordered[i]) < priority(ordered[j])
	})
	return ordered
}

func sanitizeFileStem(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			sb.WriteRune(r)
		} else if r == ' ' {
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

func detailVec3(details map[string]any, key string) (engine.Vector3, bool) {
	if details == nil {
		return engine.Vector3{}, false
	}
	raw, ok := details[key]
	if !ok || raw == nil {
		return engine.Vector3{}, false
	}
	switch v := raw.(type) {
	case map[string]any:
		return engine.NewVector3(
			float32(toFloat64(v["x"])),
			float32(toFloat64(v["y"])),
			float32(toFloat64(v["z"])),
		), true
	case []any:
		if len(v) >= 3 {
			return engine.NewVector3(
				float32(toFloat64(v[0])),
				float32(toFloat64(v[1])),
				float32(toFloat64(v[2])),
			), true
		}
	}
	return engine.Vector3{}, false
}

func detailString(details map[string]any, key string) string {
	if details == nil {
		return ""
	}
	if s, ok := details[key].(string); ok {
		return s
	}
	return ""
}

func detailBool(details map[string]any, key string) (bool, bool) {
	if details == nil {
		return false, false
	}
	b, ok := details[key].(bool)
	return b, ok
}

func detailFloat(details map[string]any, key string) (float64, bool) {
	if details == nil {
		return 0, false
	}
	v, ok := details[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func detailFloatDefault(details map[string]any, key string, def float64) float64 {
	if v, ok := detailFloat(details, key); ok {
		return v
	}
	return def
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func (b *LuaBridge) registerPrefabAPIs() {
	if b.prefabService == nil {
		b.prefabService = NewPrefabService(b)
	}

	searchPrefabsFn := b.L.NewFunction(func(L *lua.LState) int {
		query := L.OptString(1, "")
		results, err := b.prefabService.SearchPrefabs(query)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.emitPrefabSearchResults(query, results)
		tbl := L.NewTable()
		for i, item := range results {
			entry := L.NewTable()
			entry.RawSetString("name", lua.LString(item.Name))
			entry.RawSetString("path", lua.LString(item.FilePath))
			entry.RawSetString("relative_path", lua.LString(item.RelativePath))
			entry.RawSetString("active", lua.LBool(item.Active))
			compsTbl := L.NewTable()
			for j, cn := range item.ComponentNames {
				compsTbl.RawSetInt(j+1, lua.LString(cn))
			}
			entry.RawSetString("components", compsTbl)
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		return 1
	})
	b.L.SetGlobal("search_prefabs", searchPrefabsFn)
	b.L.SetGlobal("find_prefabs", searchPrefabsFn)
	b.L.SetGlobal("list_prefabs", b.L.NewFunction(func(L *lua.LState) int {
		results, err := b.prefabService.SearchPrefabs("")
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.emitPrefabSearchResults("", results)
		tbl := L.NewTable()
		for i, item := range results {
			tbl.RawSetInt(i+1, lua.LString(item.Name))
		}
		L.Push(tbl)
		return 1
	}))

	savePrefabFn := b.L.NewFunction(func(L *lua.LState) int {
		var obj *engine.GameObject
		var name string
		if L.GetTop() == 1 {
			if s, ok := L.Get(1).(lua.LString); ok {
				obj = b.GetPickedObject()
				name = string(s)
			} else {
				obj = checkGameObject(L, 1)
				name = obj.Name
			}
		} else {
			if s, ok := L.Get(1).(lua.LString); ok {
				obj = b.findSceneOrRegisteredObject(string(s))
			} else {
				obj = checkGameObject(L, 1)
			}
			name = L.OptString(2, "")
		}
		if obj == nil {
			L.RaiseError("save_prefab: target GameObject not found (or nothing selected)")
			return 0
		}
		doc, err := b.prefabService.SaveGameObjectAsPrefab(obj, name)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.EmitConsole(fmt.Sprintf("[Prefab] Saved %q -> %s", doc.Name, doc.FilePath), LineSystem)
		L.Push(lua.LString(doc.FilePath))
		return 1
	})
	b.L.SetGlobal("save_prefab", savePrefabFn)
	b.L.SetGlobal("create_prefab", savePrefabFn)

	b.L.SetGlobal("load_prefab", b.L.NewFunction(func(L *lua.LState) int {
		target := L.CheckString(1)
		spec, path, err := b.prefabService.LoadPrefabSpec(target)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		raw, _ := json.Marshal(spec)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		m["file_path"] = path
		L.Push(goValueToLua(L, m))
		return 1
	}))

	spawnPrefabFn := b.L.NewFunction(func(L *lua.LState) int {
		target := L.CheckString(1)
		opts := PrefabSpawnOptions{AutoAdd: true}
		if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
			switch v := L.Get(2).(type) {
			case lua.LString:
				opts.ID = string(v)
			case *lua.LTable:
				if id := v.RawGetString("id"); id != lua.LNil {
					opts.ID = id.String()
				}
				if nm := v.RawGetString("name"); nm != lua.LNil {
					opts.Name = nm.String()
				}
				if posTbl, ok := v.RawGetString("position").(*lua.LTable); ok {
					p := tableToVec3(L, posTbl)
					opts.Position = &p
				}
				if pk := v.RawGetString("pick"); pk != lua.LNil {
					opts.PickSpawned = lua.LVAsBool(pk)
				}
			}
		}
		if L.GetTop() >= 4 {
			p := engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4)))
			opts.Position = &p
		}
		obj, err := b.prefabService.SpawnPrefabObject(target, opts)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.EmitConsole(fmt.Sprintf("[Prefab] Spawned %s (%s)", obj.Name, obj.ID), LineSystem)
		pushGameObject(L, obj)
		return 1
	})
	b.L.SetGlobal("spawn_prefab", spawnPrefabFn)
	b.L.SetGlobal("instantiate_prefab", spawnPrefabFn)

	b.L.SetGlobal("delete_prefab", b.L.NewFunction(func(L *lua.LState) int {
		target := L.CheckString(1)
		if err := b.prefabService.DeletePrefab(target); err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.EmitConsole(fmt.Sprintf("[Prefab] Deleted %q", target), LineSystem)
		L.Push(lua.LTrue)
		return 1
	}))

	openPrefabUIFn := b.L.NewFunction(func(L *lua.LState) int {
		if L.GetTop() >= 1 && L.Get(1) == lua.LFalse {
			b.prefabService.SetPrefabEditorOpen(false)
			return 0
		}
		target := ""
		if L.GetTop() >= 1 && L.Get(1) != lua.LNil {
			target = strings.TrimSpace(L.CheckString(1))
		}
		doc, err := b.prefabService.OpenPrefab(target)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		if doc.FilePath != "" {
			b.EmitConsole(fmt.Sprintf("[PrefabUI] Opened %s", doc.FilePath), LineSystem)
		} else {
			b.EmitConsole("[PrefabUI] Opened Prefab Manager", LineSystem)
		}
		return 0
	})
	b.L.SetGlobal("edit_prefab", openPrefabUIFn)
	b.L.SetGlobal("view_prefabs", openPrefabUIFn)
	b.L.SetGlobal("prefabs_ui", openPrefabUIFn)
	b.L.SetGlobal("prefab_editor", openPrefabUIFn)

	b.RegisterCommand("prefabs", func(args []string) (string, error) {
		query := strings.TrimSpace(strings.Join(args, " "))
		results, err := b.prefabService.SearchPrefabs(query)
		if err != nil {
			return "", err
		}
		out := formatPrefabSearchLines(query, results)
		b.EmitConsole(out, LineInfo)
		return out, nil
	})

	b.RegisterCommand("prefab", func(args []string) (string, error) {
		if len(args) == 0 {
			results, err := b.prefabService.SearchPrefabs("")
			if err != nil {
				return "", err
			}
			out := formatPrefabSearchLines("", results)
			b.EmitConsole(out, LineInfo)
			return out, nil
		}
		sub := strings.ToLower(args[0])
		rest := args[1:]
		switch sub {
		case "search", "find", "list":
			q := strings.TrimSpace(strings.Join(rest, " "))
			results, err := b.prefabService.SearchPrefabs(q)
			if err != nil {
				return "", err
			}
			out := formatPrefabSearchLines(q, results)
			b.EmitConsole(out, LineInfo)
			return out, nil
		case "view", "show":
			if len(rest) == 0 {
				return "", fmt.Errorf("usage: prefab view <name>")
			}
			doc, err := b.prefabService.OpenPrefab(strings.Join(rest, " "))
			if err != nil {
				return "", err
			}
			b.EmitConsole(doc.RawJSON, LineInfo)
			return doc.RawJSON, nil
		case "save", "create":
			if len(rest) == 0 {
				return "", fmt.Errorf("usage: prefab save <name> [object]")
			}
			name := rest[0]
			var obj *engine.GameObject
			if len(rest) >= 2 {
				obj = b.findSceneOrRegisteredObject(rest[1])
			} else {
				obj = b.GetPickedObject()
			}
			if obj == nil {
				return "", fmt.Errorf("target GameObject not found (pick an object or pass its ID/name)")
			}
			doc, err := b.prefabService.SaveGameObjectAsPrefab(obj, name)
			if err != nil {
				return "", err
			}
			msg := fmt.Sprintf("[Prefab] Saved %q -> %s", doc.Name, doc.FilePath)
			b.EmitConsole(msg, LineSystem)
			return msg, nil
		case "spawn", "instantiate":
			if len(rest) == 0 {
				return "", fmt.Errorf("usage: prefab spawn <name> [id]")
			}
			opts := PrefabSpawnOptions{AutoAdd: true}
			if len(rest) >= 2 {
				opts.ID = rest[1]
			}
			obj, err := b.prefabService.SpawnPrefabObject(rest[0], opts)
			if err != nil {
				return "", err
			}
			msg := fmt.Sprintf("[Prefab] Spawned %s (%s)", obj.Name, obj.ID)
			b.EmitConsole(msg, LineSystem)
			return msg, nil
		case "edit", "ui", "open":
			target := strings.TrimSpace(strings.Join(rest, " "))
			doc, err := b.prefabService.OpenPrefab(target)
			if err != nil {
				return "", err
			}
			msg := "[PrefabUI] Opened Prefab Manager"
			if doc.FilePath != "" {
				msg = fmt.Sprintf("[PrefabUI] Opened %s", doc.FilePath)
			}
			b.EmitConsole(msg, LineSystem)
			return msg, nil
		case "delete", "rm":
			if len(rest) == 0 {
				return "", fmt.Errorf("usage: prefab delete <name>")
			}
			target := strings.Join(rest, " ")
			if err := b.prefabService.DeletePrefab(target); err != nil {
				return "", err
			}
			msg := fmt.Sprintf("[Prefab] Deleted %q", target)
			b.EmitConsole(msg, LineSystem)
			return msg, nil
		default:
			// Treat `prefab <query>` as search shortcut.
			q := strings.TrimSpace(strings.Join(args, " "))
			results, err := b.prefabService.SearchPrefabs(q)
			if err != nil {
				return "", err
			}
			out := formatPrefabSearchLines(q, results)
			b.EmitConsole(out, LineInfo)
			return out, nil
		}
	})
}

func (b *LuaBridge) emitPrefabSearchResults(query string, results []engine.PrefabSummary) {
	b.EmitConsole(formatPrefabSearchLines(query, results), LineInfo)
}

func formatPrefabSearchLines(query string, results []engine.PrefabSummary) string {
	if len(results) == 0 {
		if query == "" {
			return "(no prefabs found in prefabs/)"
		}
		return fmt.Sprintf("(no prefabs matching %q)", query)
	}
	var sb strings.Builder
	header := fmt.Sprintf("Prefabs (%d):", len(results))
	if query != "" {
		header = fmt.Sprintf("Prefabs matching %q (%d):", query, len(results))
	}
	sb.WriteString(header + "\n")
	for _, r := range results {
		comps := "(no components)"
		if len(r.ComponentNames) > 0 {
			comps = strings.Join(r.ComponentNames, ", ")
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] - components: %s\n", r.Name, r.RelativePath, comps))
	}
	return strings.TrimRight(sb.String(), "\n")
}
