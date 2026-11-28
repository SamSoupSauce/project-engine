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
