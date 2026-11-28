package luabridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
	"github.com/SamSoupSauce/project-engine/pkg/tileimporter"
	lua "github.com/yuin/gopher-lua"
)

const (
	defaultTileMapRelPath = "assets/maps/kitchen_sink.tmx"
	tileMapTileSize       = float32(1.25)
	tileMapBaseY          = float32(-0.08)
)

// TileMapSummary describes a discovered .tmx file.
type TileMapSummary struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size_bytes"`
}

// TileMapInfo summarizes the active WorldMap3D.
type TileMapInfo struct {
	Path        string  `json:"path"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	TileWidth   int     `json:"tilewidth"`
	TileHeight  int     `json:"tileheight"`
	TileCount   int     `json:"tile_count"`
	LayerCount  int     `json:"layer_count"`
	ObjectCount int     `json:"object_count"`
	TileSize    float32 `json:"tile_size"`
	BaseY       float32 `json:"base_y"`
}

// TileMapLayerInfo describes a single layer.
type TileMapLayerInfo struct {
	Name      string  `json:"name"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Elevation float32 `json:"elevation"`
	Visible   bool    `json:"visible"`
}

// TileMapTileInfo describes a single instantiated tile.
type TileMapTileInfo struct {
	Layer     string  `json:"layer"`
	Col       int     `json:"col"`
	Row       int     `json:"row"`
	GID       uint32  `json:"gid"`
	Type      string  `json:"type"`
	Name      string  `json:"name"`
	Solid     bool    `json:"solid"`
	Elevation float32 `json:"elevation"`
	Walls     uint8   `json:"walls"`
	StairDir  int     `json:"stair_dir"`
}

// TileMapLoadResult is returned by load/swap operations.
type TileMapLoadResult struct {
	Path       string `json:"path"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	TileCount  int    `json:"tile_count"`
	LayerCount int    `json:"layer_count"`
}

// TileMapService owns tile map discovery, the active WorldMap3D, and exposes
// Lua/shell/bridge introspection and runtime loading. It is decoupled from
// rendering: a registered ReloadScene callback rebuilds scene geometry/colliders.
type TileMapService struct {
	Bridge *LuaBridge

	mu         sync.RWMutex
	mapsDir    string
	activePath string
	active     *tileimporter.WorldMap3D
	open       bool

	// ReloadScene is called after a successful load/reload so the host can rebuild
	// the renderable mesh and static collider for the new world.
	ReloadScene func(path string, world *tileimporter.WorldMap3D) error
}

// NewTileMapService creates a service bound to bridge.
func NewTileMapService(bridge *LuaBridge) *TileMapService {
	return &TileMapService{Bridge: bridge}
}

// TileMapEditor returns the bridge's TileMapService.
func (b *LuaBridge) TileMapEditor() *TileMapService {
	if b == nil {
		return nil
	}
	return b.tileMapService
}

// SetMapsDir sets the directory scanned for .tmx maps.
func (s *TileMapService) SetMapsDir(dir string) {
	if s == nil || strings.TrimSpace(dir) == "" {
		return
	}
	s.mu.Lock()
	s.mapsDir = filepath.Clean(dir)
	s.mu.Unlock()
}

func (s *TileMapService) resolveMapsDir() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	dir := s.mapsDir
	s.mu.RUnlock()
	if strings.TrimSpace(dir) != "" {
		return dir
	}
	if s.Bridge != nil {
		wp := s.Bridge.WorkspacePaths()
		return filepath.Join(wp.AssetsDir, "maps")
	}
	return filepath.Join("assets", "maps")
}

// ListMaps discovers .tmx files under the configured maps directory.
func (s *TileMapService) ListMaps(query string) ([]TileMapSummary, error) {
	dir := s.resolveMapsDir()
	var out []TileMapSummary
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".tmx") {
			return nil
		}
		st, stErr := os.Stat(path)
		size := int64(0)
		if stErr == nil {
			size = st.Size()
		}
		out = append(out, TileMapSummary{
			Path: filepath.ToSlash(path),
			Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
			Size: size,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q != "" {
		filtered := out[:0]
		for _, m := range out {
			if strings.Contains(strings.ToLower(m.Path), q) || strings.Contains(strings.ToLower(m.Name), q) {
				filtered = append(filtered, m)
			}
		}
		out = filtered
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if out == nil {
		out = []TileMapSummary{}
	}
	return out, nil
}

// LoadMap parses and activates a TMX map at the given path, then invokes ReloadScene.
func (s *TileMapService) LoadMap(path string) (TileMapLoadResult, error) {
	var result TileMapLoadResult
	if s == nil || strings.TrimSpace(path) == "" {
		return result, fmt.Errorf("tilemap.load requires a non-empty path")
	}
	clean := s.resolveMapPath(path)
	raw, err := os.ReadFile(clean)
	if err != nil {
		return result, fmt.Errorf("read tile map %q: %w", filepath.ToSlash(clean), err)
	}
	parsed, err := tileimporter.ParseTMX(raw)
	if err != nil {
		return result, fmt.Errorf("parse tile map %q: %w", filepath.ToSlash(clean), err)
	}
	world := tileimporter.NewWorldMap3D(parsed, tileMapTileSize, tileMapBaseY)

	s.mu.Lock()
	prev := s.active
	s.active = world
	s.activePath = filepath.ToSlash(clean)
	s.mu.Unlock()

	if s.ReloadScene != nil {
		if err := s.ReloadScene(filepath.ToSlash(clean), world); err != nil {
			s.mu.Lock()
			s.active = prev
			s.activePath = func() string {
				if prev != nil {
					return filepath.ToSlash(clean)
				}
				return ""
			}()
			s.mu.Unlock()
			return result, fmt.Errorf("rebuild scene for tile map: %w", err)
		}
	}

	result = TileMapLoadResult{
		Path:       filepath.ToSlash(clean),
		Width:      parsed.Width,
		Height:     parsed.Height,
		TileCount:  len(world.Tiles),
		LayerCount: len(parsed.Layers),
	}
	return result, nil
}

// resolveMapPath resolves a path that may be absolute, workspace-relative, or maps-dir-relative.
func (s *TileMapService) resolveMapPath(path string) string {
	p := strings.TrimSpace(path)
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	// Try workspace-relative first (e.g. assets/maps/foo.tmx).
	if s.Bridge != nil {
		resolved := s.Bridge.ResolveWorkspacePath(p)
		if _, err := os.Stat(resolved); err == nil {
			return resolved
		}
	}
	// Then maps-dir-relative (e.g. foo.tmx).
	candidate := filepath.Join(s.resolveMapsDir(), p)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if s.Bridge != nil {
		return s.Bridge.ResolveWorkspacePath(p)
	}
	return filepath.Clean(p)
}

// Reload reloads the currently active map from disk.
func (s *TileMapService) Reload() (TileMapLoadResult, error) {
	s.mu.RLock()
	p := s.activePath
	s.mu.RUnlock()
	if strings.TrimSpace(p) == "" {
		return TileMapLoadResult{}, fmt.Errorf("no active tile map to reload")
	}
	return s.LoadMap(p)
}

// ActiveWorld returns the active world (or nil).
func (s *TileMapService) ActiveWorld() *tileimporter.WorldMap3D {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// ActivePath returns the active map path (or empty).
func (s *TileMapService) ActivePath() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activePath
}

// Info summarizes the active world.
func (s *TileMapService) Info() (TileMapInfo, error) {
	w := s.ActiveWorld()
	if w == nil || w.SourceMap == nil {
		return TileMapInfo{}, fmt.Errorf("no active tile map loaded")
	}
	m := w.SourceMap
	objects := 0
	for _, g := range m.ObjectGroups {
		objects += len(g.Objects)
	}
	return TileMapInfo{
		Path:        s.ActivePath(),
		Width:       m.Width,
		Height:      m.Height,
		TileWidth:   m.TileWidth,
		TileHeight:  m.TileHeight,
		TileCount:   len(w.Tiles),
		LayerCount:  len(m.Layers),
		ObjectCount: objects,
		TileSize:    w.TileSize,
		BaseY:       w.BaseY,
	}, nil
}

// Layers lists the active world's layers.
func (s *TileMapService) Layers() ([]TileMapLayerInfo, error) {
	w := s.ActiveWorld()
	if w == nil || w.SourceMap == nil {
		return nil, fmt.Errorf("no active tile map loaded")
	}
	out := make([]TileMapLayerInfo, 0, len(w.SourceMap.Layers))
	for _, l := range w.SourceMap.Layers {
		elev := float32(0)
		if v, err := parseFloat(l.GetProperty("elevation", "0")); err == nil {
			elev = v
		}
		out = append(out, TileMapLayerInfo{
			Name:      l.Name,
			Width:     l.Width,
			Height:    l.Height,
			Elevation: elev,
			Visible:   l.IsVisible(),
		})
	}
	return out, nil
}

// Tiles lists tiles in the active world, optionally filtered by layer name and query.
func (s *TileMapService) Tiles(layerFilter, query string) ([]TileMapTileInfo, error) {
	w := s.ActiveWorld()
	if w == nil {
		return nil, fmt.Errorf("no active tile map loaded")
	}
	q := strings.ToLower(strings.TrimSpace(query))
	lq := strings.ToLower(strings.TrimSpace(layerFilter))
	out := make([]TileMapTileInfo, 0, len(w.Tiles))
	for i := range w.Tiles {
		t := &w.Tiles[i]
		if lq != "" && !strings.Contains(strings.ToLower(t.LayerName), lq) {
			continue
		}
		ti := tileInfoFrom(t)
		if q != "" && !strings.Contains(strings.ToLower(ti.Name), q) &&
			!strings.Contains(strings.ToLower(ti.Type), q) {
			continue
		}
		out = append(out, ti)
	}
	return out, nil
}

// TileAt returns a single tile at (col,row), optionally on a named layer.
func (s *TileMapService) TileAt(col, row int, layer string) (TileMapTileInfo, error) {
	w := s.ActiveWorld()
	if w == nil {
		return TileMapTileInfo{}, fmt.Errorf("no active tile map loaded")
	}
	for i := range w.Tiles {
		t := &w.Tiles[i]
		if t.Col == col && t.Row == row && (layer == "" || strings.EqualFold(t.LayerName, layer)) {
			return tileInfoFrom(t), nil
		}
	}
	return TileMapTileInfo{}, fmt.Errorf("no tile at (%d,%d)", col, row)
}

func tileInfoFrom(t *tileimporter.Tile3D) TileMapTileInfo {
	return TileMapTileInfo{
		Layer:     t.LayerName,
		Col:       t.Col,
		Row:       t.Row,
		GID:       t.GID,
		Type:      tileTypeName(t.Type),
		Name:      t.Name,
		Solid:     t.IsSolid,
		Elevation: t.Elevation,
		Walls:     t.Walls,
		StairDir:  int(t.StairDir),
	}
}

func tileTypeName(t tileimporter.TileType) string {
	switch t {
	case tileimporter.TileFloor:
		return "floor"
	case tileimporter.TileWall:
		return "wall"
	case tileimporter.TileWallExterior:
		return "exterior_wall"
	case tileimporter.TileWallInterior:
		return "interior_wall"
	case tileimporter.TileBarricade:
		return "barricade"
	case tileimporter.TilePillar:
		return "pillar"
	case tileimporter.TileWater:
		return "water"
	case tileimporter.TileGrass:
		return "grass"
	case tileimporter.TileStairs:
		return "stairs"
	default:
		return "empty"
	}
}

// Colliders returns active world static collider bounds.
func (s *TileMapService) Colliders() ([][2][3]float32, error) {
	w := s.ActiveWorld()
	if w == nil {
		return nil, fmt.Errorf("no active tile map loaded")
	}
	return w.ExportStaticColliderBounds(), nil
}

// Registry returns the active tileset registry as a JSON-shaped map.
func (s *TileMapService) Registry() (map[string]any, error) {
	w := s.ActiveWorld()
	if w == nil || w.SourceMap == nil {
		return nil, fmt.Errorf("no active tile map loaded")
	}
	reg := make(map[string]any)
	for _, ts := range w.SourceMap.Tilesets {
		for _, tt := range ts.Tiles {
			gid := ts.FirstGID + tt.ID
			props := make(map[string]string)
			for _, p := range tt.Props {
				props[p.Name] = p.GetVal()
			}
			reg[fmt.Sprintf("%d", gid)] = props
		}
	}
	return reg, nil
}

// GridToWorld / WorldToGrid wrap WorldMap3D coordinate conversion.
func (s *TileMapService) GridToWorld(col, row int) (float32, float32, error) {
	w := s.ActiveWorld()
	if w == nil {
		return 0, 0, fmt.Errorf("no active tile map loaded")
	}
	x, z := w.GridToWorld(col, row)
	return x, z, nil
}
func (s *TileMapService) WorldToGrid(x, z float32) (int, int, error) {
	w := s.ActiveWorld()
	if w == nil {
		return 0, 0, fmt.Errorf("no active tile map loaded")
	}
	c, r := w.WorldToGrid(x, z)
	return c, r, nil
}

// JSON convenience helpers used by bridge endpoints.
func (s *TileMapService) InfoJSON() (any, error)             { return s.Info() }
func (s *TileMapService) LayersJSON() (any, error)           { return s.Layers() }
func (s *TileMapService) TilesJSON(l, q string) (any, error) { return s.Tiles(l, q) }
func (s *TileMapService) CollidersJSON() (any, error)        { return s.Colliders() }
func (s *TileMapService) RegistryJSON() (any, error)         { return s.Registry() }

// MarshalMap marshals any value to a generic structure for RPC transport.
func MarshalMap(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// --- Lua table + shell commands ---

func (s *TileMapService) registerLuaAPIs() {
	if s == nil || s.Bridge == nil {
		return
	}
	L := s.Bridge.L
	tbl := L.NewTable()

	set := func(name string, fn lua.LGFunction) { L.SetField(tbl, name, L.NewFunction(fn)) }

	set("list", func(L *lua.LState) int {
		q := L.OptString(1, "")
		maps, err := s.ListMaps(q)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		t := L.NewTable()
		for _, m := range maps {
			item := L.NewTable()
			L.SetField(item, "path", lua.LString(m.Path))
			L.SetField(item, "name", lua.LString(m.Name))
			L.SetField(item, "size_bytes", lua.LNumber(m.Size))
			t.Append(item)
		}
		L.Push(t)
		return 1
	})
	set("load", func(L *lua.LState) int {
		p := L.CheckString(1)
		res, err := s.LoadMap(p)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(tileLoadResultToLua(L, res))
		return 1
	})
	set("reload", func(L *lua.LState) int {
		res, err := s.Reload()
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(tileLoadResultToLua(L, res))
		return 1
	})
	set("current", func(L *lua.LState) int {
		w := s.ActiveWorld()
		if w == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(s.ActivePath()))
		return 1
	})
	set("info", func(L *lua.LState) int {
		info, err := s.Info()
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(structToLuaTable(L, info))
		return 1
	})
	set("layers", func(L *lua.LState) int {
		layers, err := s.Layers()
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(sliceToLuaTable(L, layers, func(v any) lua.LValue {
			return structToLuaTable(L, v)
		}))
		return 1
	})
	set("tiles", func(L *lua.LState) int {
		layer := L.OptString(1, "")
		q := L.OptString(2, "")
		tiles, err := s.Tiles(layer, q)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(sliceToLuaTable(L, tiles, func(v any) lua.LValue {
			return structToLuaTable(L, v)
		}))
		return 1
	})
	set("tile_at", func(L *lua.LState) int {
		col, row := L.CheckInt(1), L.CheckInt(2)
		layer := L.OptString(3, "")
		tile, err := s.TileAt(col, row, layer)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(structToLuaTable(L, tile))
		return 1
	})
	set("world_to_grid", func(L *lua.LState) int {
		x, z := float32(L.CheckNumber(1)), float32(L.CheckNumber(2))
		c, r, err := s.WorldToGrid(x, z)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(lua.LNumber(c))
		L.Push(lua.LNumber(r))
		return 2
	})
	set("grid_to_world", func(L *lua.LState) int {
		c, r := L.CheckInt(1), L.CheckInt(2)
		x, z, err := s.GridToWorld(c, r)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(lua.LNumber(x))
		L.Push(lua.LNumber(z))
		return 2
	})
	set("colliders", func(L *lua.LState) int {
		bounds, err := s.Colliders()
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		t := L.NewTable()
		for _, b := range bounds {
			min := L.NewTable()
			max := L.NewTable()
			min.Append(lua.LNumber(b[0][0]))
			min.Append(lua.LNumber(b[0][1]))
			min.Append(lua.LNumber(b[0][2]))
			max.Append(lua.LNumber(b[1][0]))
			max.Append(lua.LNumber(b[1][1]))
			max.Append(lua.LNumber(b[1][2]))
			pair := L.NewTable()
			pair.Append(min)
			pair.Append(max)
			t.Append(pair)
		}
		L.Push(t)
		return 1
	})
	set("registry", func(L *lua.LState) int {
		reg, err := s.Registry()
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(structToLuaTable(L, reg))
		return 1
	})

	L.SetGlobal("tilemap", tbl)

	// Shell commands via the unified registry (docs-visible).
	reg := func(name, desc, usage string, examples []string, handler func([]string) (string, error)) {
		s.Bridge.shell.RegisterNative(ShellCommandDef{
			Name: name, Description: desc, Usage: usage, Examples: examples,
			Source: "native",
			GoHandler: func(ctx ExecutionContext) (string, error) {
				out, err := handler(ctx.Args[1:])
				if err == nil {
					s.Bridge.EmitConsole(out, LineInfo)
				}
				return out, err
			},
		})
	}
	reg("tilemaps", "Lists discovered .tmx tile maps.", "tilemaps [query]", []string{"tilemaps", "tilemaps kitchen"},
		func(args []string) (string, error) {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			maps, err := s.ListMaps(q)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Tile maps (%d):\n", len(maps))
			for _, m := range maps {
				fmt.Fprintf(&b, "  %s\n", m.Path)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})
	reg("tilemap.load", "Loads a .tmx map at runtime.", "tilemap.load <path>", []string{"tilemap.load assets/maps/kitchen_sink.tmx"},
		func(args []string) (string, error) {
			if len(args) < 1 {
				return "", fmt.Errorf("tilemap.load requires a path")
			}
			res, err := s.LoadMap(args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("loaded %s (%dx%d, %d tiles, %d layers)", res.Path, res.Width, res.Height, res.TileCount, res.LayerCount), nil
		})
	reg("tilemap.reload", "Reloads the active map from disk.", "tilemap.reload", []string{"tilemap.reload"},
		func(args []string) (string, error) {
			res, err := s.Reload()
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("reloaded %s (%d tiles)", res.Path, res.TileCount), nil
		})
	reg("tilemap.info", "Shows active tile map summary.", "tilemap.info", []string{"tilemap.info"},
		func(args []string) (string, error) {
			info, err := s.Info()
			if err != nil {
				return "", err
			}
			return info.Formatted(), nil
		})
	reg("tilemap.layers", "Lists active map layers.", "tilemap.layers", []string{"tilemap.layers"},
		func(args []string) (string, error) {
			layers, err := s.Layers()
			if err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Layers (%d):\n", len(layers))
			for _, l := range layers {
				fmt.Fprintf(&b, "  %s  %dx%d  elev=%.2f  visible=%v\n", l.Name, l.Width, l.Height, l.Elevation, l.Visible)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})
	reg("tilemap.tiles", "Browses active map tiles.", "tilemap.tiles [layer] [query]", []string{"tilemap.tiles", "tilemap.tiles Ground pillar"},
		func(args []string) (string, error) {
			layer, q := "", ""
			if len(args) > 0 {
				layer = args[0]
			}
			if len(args) > 1 {
				q = args[1]
			}
			tiles, err := s.Tiles(layer, q)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Tiles (%d):\n", len(tiles))
			for _, t := range tiles {
				fmt.Fprintf(&b, "  [%s] (%d,%d) gid=%d %s %q solid=%v elev=%.2f\n",
					t.Layer, t.Col, t.Row, t.GID, t.Type, t.Name, t.Solid, t.Elevation)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})
}

func tileLoadResultToLua(L *lua.LState, r TileMapLoadResult) lua.LValue {
	t := L.NewTable()
	L.SetField(t, "path", lua.LString(r.Path))
	L.SetField(t, "width", lua.LNumber(r.Width))
	L.SetField(t, "height", lua.LNumber(r.Height))
	L.SetField(t, "tile_count", lua.LNumber(r.TileCount))
	L.SetField(t, "layer_count", lua.LNumber(r.LayerCount))
	return t
}

// structToLuaTable converts a struct or map to a Lua table via JSON round-trip (lower-case field names).
func structToLuaTable(L *lua.LState, v any) lua.LValue {
	b, err := json.Marshal(v)
	if err != nil {
		return L.NewTable()
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return L.NewTable()
	}
	return anyToLua(L, m)
}

func sliceToLuaTable(L *lua.LState, v any, conv func(any) lua.LValue) lua.LValue {
	t := L.NewTable()
	b, err := json.Marshal(v)
	if err != nil {
		return t
	}
	var items []any
	if err := json.Unmarshal(b, &items); err != nil {
		return t
	}
	for _, item := range items {
		t.Append(conv(item))
	}
	return t
}

func anyToLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case string:
		return lua.LString(x)
	case float64:
		return lua.LNumber(x)
	case int:
		return lua.LNumber(x)
	case int64:
		return lua.LNumber(x)
	case []any:
		t := L.NewTable()
		for _, it := range x {
			t.Append(anyToLua(L, it))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, it := range x {
			L.SetField(t, k, anyToLua(L, it))
		}
		return t
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

func parseFloat(s string) (float32, error) {
	var f float64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%f", &f)
	return float32(f), err
}

// Formatted returns a human-readable summary for the console.
func (i TileMapInfo) Formatted() string {
	return fmt.Sprintf("Tile map %s: %dx%d (tile %dx%d), %d tiles, %d layers, %d objects, tileSize=%.2f baseY=%.2f",
		i.Path, i.Width, i.Height, i.TileWidth, i.TileHeight, i.TileCount, i.LayerCount, i.ObjectCount, i.TileSize, i.BaseY)
}

// RebuildTileMapScene builds a renderable mesh asset and static collider for a
// loaded world, swapping them on the registered tile GameObject.
func RebuildTileMapScene(scene *engine.BaseScene, physics *spatial.PhysicsWorld, path string, world *tileimporter.WorldMap3D) error {
	if scene == nil || world == nil {
		return fmt.Errorf("scene and world required")
	}
	obj := scene.FindByName("TileMapWorld")
	if obj == nil {
		obj = scene.FindByName("kitchen_sink_tiles")
	}
	if obj == nil {
		return fmt.Errorf("tile map GameObject not found in active scene")
	}
	asset := tileimporter.BuildGLTFAsset(world)
	mc := obj.GetMeshComponent()
	if mc == nil {
		return fmt.Errorf("tile map GameObject has no MeshComponent")
	}
	mc.Asset = asset
	mc.FilePath = filepath.ToSlash(path)

	if physics != nil {
		if _, err := spatial.BuildWorldMapCollider(physics, world); err != nil {
			return fmt.Errorf("world map collider: %w", err)
		}
	}
	return nil
}

// --- TileMapBrowserBackend implementation (engine interface) ---

func (s *TileMapService) IsTileMapBrowserOpen() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.open
}
func (s *TileMapService) SetTileMapBrowserOpen(open bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.open = open
	s.mu.Unlock()
}
func (s *TileMapService) ListTileMaps(query string) ([]engine.TileMapBrowserMap, error) {
	maps, err := s.ListMaps(query)
	if err != nil {
		return nil, err
	}
	out := make([]engine.TileMapBrowserMap, 0, len(maps))
	for _, m := range maps {
		out = append(out, engine.TileMapBrowserMap{Path: m.Path, Name: m.Name, Size: m.Size})
	}
	return out, nil
}
func (s *TileMapService) LoadTileMap(path string) error {
	_, err := s.LoadMap(path)
	return err
}
func (s *TileMapService) ReloadTileMap() error {
	_, err := s.Reload()
	return err
}
func (s *TileMapService) ActiveTileMapInfo() (engine.TileMapBrowserInfo, error) {
	i, err := s.Info()
	if err != nil {
		return engine.TileMapBrowserInfo{}, err
	}
	return engine.TileMapBrowserInfo{
		Path: i.Path, Width: i.Width, Height: i.Height,
		TileWidth: i.TileWidth, TileHeight: i.TileHeight,
		TileCount: i.TileCount, LayerCount: i.LayerCount, ObjectCount: i.ObjectCount,
	}, nil
}
func (s *TileMapService) TileMapLayers() ([]engine.TileMapBrowserLayer, error) {
	layers, err := s.Layers()
	if err != nil {
		return nil, err
	}
	out := make([]engine.TileMapBrowserLayer, 0, len(layers))
	for _, l := range layers {
		out = append(out, engine.TileMapBrowserLayer{Name: l.Name, Width: l.Width, Height: l.Height, Elevation: l.Elevation, Visible: l.Visible})
	}
	return out, nil
}
func (s *TileMapService) TileMapTiles(layer, query string) ([]engine.TileMapBrowserTile, error) {
	tiles, err := s.Tiles(layer, query)
	if err != nil {
		return nil, err
	}
	out := make([]engine.TileMapBrowserTile, 0, len(tiles))
	for _, t := range tiles {
		out = append(out, engine.TileMapBrowserTile{
			Layer: t.Layer, Col: t.Col, Row: t.Row, GID: t.GID,
			Type: t.Type, Name: t.Name, Solid: t.Solid, Elevation: t.Elevation, Walls: t.Walls,
		})
	}
	return out, nil
}
func (s *TileMapService) TileGridToWorld(col, row int) (float32, float32, error) {
	return s.GridToWorld(col, row)
}
