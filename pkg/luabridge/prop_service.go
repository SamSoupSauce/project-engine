package luabridge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/SamSoupSauce/project-engine/engine"
)

// PropSummary describes a discovered prop manifest.
type PropSummary struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
}

// ResolutionOptions mirrors engine.ResolveOptions for bridge callers.
type ResolutionOptions = engine.ResolveOptions

// PropService owns prop manifest discovery, loading, resolution, and validation.
type PropService struct {
	Bridge *LuaBridge

	mu              sync.RWMutex
	propsDir        string
	cache           map[string]*engine.Prop
	propBrowserOpen bool
}

// NewPropService creates a service bound to bridge.
func NewPropService(bridge *LuaBridge) *PropService {
	return &PropService{Bridge: bridge, cache: make(map[string]*engine.Prop)}
}

// PropEditor returns the bridge's PropService.
func (b *LuaBridge) PropEditor() *PropService {
	if b == nil {
		return nil
	}
	return b.propService
}

// SetPropsDir sets the directory scanned for *.prop.json manifests.
func (s *PropService) SetPropsDir(dir string) {
	if s == nil || strings.TrimSpace(dir) == "" {
		return
	}
	s.mu.Lock()
	s.propsDir = filepath.Clean(dir)
	s.cache = make(map[string]*engine.Prop)
	s.mu.Unlock()
}

func (s *PropService) resolvePropsDir() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	dir := s.propsDir
	s.mu.RUnlock()
	if strings.TrimSpace(dir) != "" {
		return dir
	}
	if s.Bridge != nil {
		wp := s.Bridge.WorkspacePaths()
		return filepath.Join(wp.ScriptsDir, "props")
	}
	return filepath.Join("scripts", "props")
}

// ListProps discovers *.prop.json manifests.
func (s *PropService) ListProps(query string) ([]PropSummary, error) {
	dir := s.resolvePropsDir()
	var out []PropSummary
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".prop.json") {
			return nil
		}
		prop, err := engine.LoadPropManifest(path)
		if err != nil {
			return nil
		}
		out = append(out, PropSummary{ID: prop.ID, Path: filepath.ToSlash(path), Name: info.Name()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q != "" {
		filtered := out[:0]
		for _, p := range out {
			if strings.Contains(strings.ToLower(p.ID), q) || strings.Contains(strings.ToLower(p.Path), q) {
				filtered = append(filtered, p)
			}
		}
		out = filtered
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if out == nil {
		out = []PropSummary{}
	}
	return out, nil
}

func (s *PropService) loadCached(id string) (*engine.Prop, error) {
	s.mu.RLock()
	prop, ok := s.cache[id]
	s.mu.RUnlock()
	if ok {
		return prop, nil
	}
	// discover by id
	props, err := s.ListProps("")
	if err != nil {
		return nil, err
	}
	for _, p := range props {
		if p.ID == id {
			prop, err := engine.LoadPropManifest(p.Path)
			if err != nil {
				return nil, err
			}
			s.mu.Lock()
			s.cache[id] = prop
			s.mu.Unlock()
			return prop, nil
		}
	}
	return nil, fmt.Errorf("prop %q not found", id)
}

// ResolveProp resolves a named prop into concrete leaves.
func (s *PropService) ResolveProp(id string, opts ResolutionOptions) (*engine.ResolvedProp, error) {
	prop, err := s.loadCached(id)
	if err != nil {
		return nil, err
	}
	return engine.Resolve(prop, opts)
}

// ValidateProp returns validation errors for a named prop.
func (s *PropService) ValidateProp(id string) ([]string, error) {
	prop, err := s.loadCached(id)
	if err != nil {
		return nil, err
	}
	return engine.ValidateProp(prop), nil
}

// --- PropBrowserBackend implementation ---

func (s *PropService) IsPropBrowserOpen() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.propBrowserOpen
}
func (s *PropService) SetPropBrowserOpen(open bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.propBrowserOpen = open
	s.mu.Unlock()
}
func (s *PropService) ListPropsBrowser(query string) ([]engine.PropBrowserSummary, error) {
	list, err := s.ListProps(query)
	if err != nil {
		return nil, err
	}
	out := make([]engine.PropBrowserSummary, 0, len(list))
	for _, p := range list {
		out = append(out, engine.PropBrowserSummary{ID: p.ID, Name: p.Name, Path: p.Path})
	}
	return out, nil
}
func (s *PropService) ResolvePropBrowser(id string, seed int64) (engine.PropBrowserResolution, error) {
	res, err := s.ResolveProp(id, ResolutionOptions{Seed: seed, Selections: map[string]string{}})
	if err != nil {
		return engine.PropBrowserResolution{}, err
	}
	leaves := make([]engine.PropBrowserLeaf, 0, len(res.Leaves))
	for _, l := range res.Leaves {
		leaves = append(leaves, engine.PropBrowserLeaf{NodeID: l.NodeID, Model: l.Model, Material: l.Material})
	}
	return engine.PropBrowserResolution{PropID: res.PropID, LeafCount: len(leaves), Leaves: leaves}, nil
}

// GetProp returns a loaded prop tree by id.
func (s *PropService) GetProp(id string) (*engine.Prop, error) {
	return s.loadCached(id)
}

// propManifestPath finds the manifest path for an id.
func (s *PropService) propManifestPath(id string) (string, error) {
	props, err := s.ListProps("")
	if err != nil {
		return "", err
	}
	for _, p := range props {
		if p.ID == id {
			return p.Path, nil
		}
	}
	return "", fmt.Errorf("prop %q not found", id)
}

func (s *PropService) invalidate(id string) {
	s.mu.Lock()
	delete(s.cache, id)
	s.mu.Unlock()
}

// SetSelection pins an option for a node path, validates, and persists.
func (s *PropService) SetSelection(id, path, poolName, optionID string) error {
	prop, err := s.GetProp(id)
	if err != nil {
		return err
	}
	if err := engine.SetSelection(prop, path, poolName, optionID); err != nil {
		return err
	}
	if errs := engine.ValidateProp(prop); len(errs) > 0 {
		return fmt.Errorf("prop validation failed: %s", strings.Join(errs, "; "))
	}
	manifest, err := s.propManifestPath(id)
	if err != nil {
		return err
	}
	if err := engine.SavePropManifest(manifest, prop); err != nil {
		return err
	}
	s.invalidate(id)
	return nil
}

// CloneProp deep-copies a prop to a new id, persists it, and returns the new manifest path.
func (s *PropService) CloneProp(sourceID, newID string) (string, error) {
	prop, err := s.GetProp(sourceID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(newID) == "" {
		return "", fmt.Errorf("new prop id is empty")
	}
	clone := engine.CloneProp(prop, newID)
	if clone == nil {
		return "", fmt.Errorf("failed to clone prop %q", sourceID)
	}
	path := filepath.Join(s.resolvePropsDir(), newID+".prop.json")
	if err := engine.SavePropManifest(path, clone); err != nil {
		return "", err
	}
	s.invalidate(newID)
	return filepath.ToSlash(path), nil
}

// SaveProp persists the cached prop tree to its manifest.
func (s *PropService) SaveProp(id string) error {
	prop, err := s.GetProp(id)
	if err != nil {
		return err
	}
	if errs := engine.ValidateProp(prop); len(errs) > 0 {
		return fmt.Errorf("prop validation failed: %s", strings.Join(errs, "; "))
	}
	manifest, err := s.propManifestPath(id)
	if err != nil {
		return err
	}
	return engine.SavePropManifest(manifest, prop)
}

// DeleteProp removes a prop manifest by id.
func (s *PropService) DeleteProp(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("prop id is empty")
	}
	manifest, err := s.propManifestPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(manifest); err != nil {
		return fmt.Errorf("delete prop manifest: %w", err)
	}
	s.invalidate(id)
	return nil
}

// PropTreeBrowser returns a flat tree representation for the editor.
func (s *PropService) PropTreeBrowser(id string) ([]engine.PropTreeNode, error) {
	prop, err := s.GetProp(id)
	if err != nil { return nil, err }
	var out []engine.PropTreeNode
	var visit func(*engine.Node, int)
	visit = func(n *engine.Node, depth int) {
		if n == nil { return }
		name, count := "", 0
		if n.TypePool != nil { name, count = n.TypePool.Name, len(n.TypePool.Options) }
		out = append(out, engine.PropTreeNode{ID: n.ID, PoolName: name, OptionCount: count, Depth: depth})
		for _, c := range n.Children { visit(c, depth+1) }
	}
	visit(prop.Root, 0)
	return out, nil
}

func (s *PropService) AddPropOptionBrowser(id, path, pool, option, model string) error {
	prop, err := s.GetProp(id)
	if err != nil { return err }
	node, err := engine.FindNode(prop.Root, path)
	if err != nil { return err }
	var target *engine.Pool
	if node.TypePool != nil && (pool == "" || node.TypePool.Name == pool) { target = node.TypePool }
	if target == nil {
		for i := range node.TypePool.Options {
			sub := node.TypePool.Options[i].Payload.SubPools
			for j := range sub { if sub[j].Name == pool { target = sub[j] } }
		}
	}
	if target == nil { return fmt.Errorf("pool %q not found at node %q", pool, path) }
	if err := engine.SetOption(target, option, option, engine.OptionPayload{Model: model}); err != nil { return err }
	if errs := engine.ValidateProp(prop); len(errs) > 0 { return fmt.Errorf("validation failed: %s", strings.Join(errs, "; ")) }
	manifest, err := s.propManifestPath(id)
	if err != nil { return err }
	if err := engine.SavePropManifest(manifest, prop); err != nil { return err }
	s.invalidate(id)
	return nil
}

func (s *PropService) RemovePropOptionBrowser(id, path, pool, option string) error {
	prop, err := s.GetProp(id)
	if err != nil { return err }
	node, err := engine.FindNode(prop.Root, path)
	if err != nil { return err }
	target := node.TypePool
	if target == nil { return fmt.Errorf("no pool at node %q", path) }
	if err := engine.RemoveOption(target, option); err != nil { return err }
	if errs := engine.ValidateProp(prop); len(errs) > 0 { return fmt.Errorf("validation failed: %s", strings.Join(errs, "; ")) }
	manifest, err := s.propManifestPath(id)
	if err != nil { return err }
	if err := engine.SavePropManifest(manifest, prop); err != nil { return err }
	s.invalidate(id)
	return nil
}

func (s *PropService) PinPropSelectionBrowser(id, path, pool, option string) error {
	return s.SetSelection(id, path, pool, option)
}
