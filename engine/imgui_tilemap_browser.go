package engine

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
)

// TileMapBrowserBackend decouples TileMapBrowserWindow from pkg/luabridge.
// Implementations return plain data types so engine never imports luabridge.
type TileMapBrowserBackend interface {
	IsTileMapBrowserOpen() bool
	SetTileMapBrowserOpen(open bool)
	ListTileMaps(query string) ([]TileMapBrowserMap, error)
	LoadTileMap(path string) error
	ReloadTileMap() error
	ActiveTileMapInfo() (TileMapBrowserInfo, error)
	TileMapLayers() ([]TileMapBrowserLayer, error)
	TileMapTiles(layer, query string) ([]TileMapBrowserTile, error)
	TileGridToWorld(col, row int) (float32, float32, error)
}

// TileMapBrowserMap is a discovered map entry for the browser UI.
type TileMapBrowserMap struct {
	Path string
	Name string
	Size int64
}

// TileMapBrowserInfo summarizes the active map for the browser UI.
type TileMapBrowserInfo struct {
	Path        string
	Width       int
	Height      int
	TileWidth   int
	TileHeight  int
	TileCount   int
	LayerCount  int
	ObjectCount int
}

// TileMapBrowserLayer describes a layer for the browser UI.
type TileMapBrowserLayer struct {
	Name      string
	Width     int
	Height    int
	Elevation float32
	Visible   bool
}

// TileMapBrowserTile describes a tile for the browser UI.
type TileMapBrowserTile struct {
	Layer     string
	Col       int
	Row       int
	GID       uint32
	Type      string
	Name      string
	Solid     bool
	Elevation float32
	Walls     uint8
}

// TileMapBrowserWindow renders the map browser, layer selector, tile grid, and tile inspector.
type TileMapBrowserWindow struct {
	Backend       TileMapBrowserBackend
	IsOpen        bool
	Title         string
	SearchQuery   string
	TileQuery     string
	Maps          []TileMapBrowserMap
	Layers        []TileMapBrowserLayer
	Tiles         []TileMapBrowserTile
	SelectedMap   string
	SelectedLayer int
	SelectedTile  *TileMapBrowserTile
	StatusMsg     string
	StatusIsErr   bool
	listLoaded    bool
}

// NewTileMapBrowserWindow creates a browser window bound to backend.
func NewTileMapBrowserWindow(backend TileMapBrowserBackend) *TileMapBrowserWindow {
	return &TileMapBrowserWindow{Backend: backend, Title: "Tile Map Browser"}
}

// Open shows the window and refreshes the map list.
func (w *TileMapBrowserWindow) Open() error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: tile map browser backend is nil")
	}
	w.IsOpen = true
	w.Backend.SetTileMapBrowserOpen(true)
	return w.RefreshMaps()
}

// Close hides the window.
func (w *TileMapBrowserWindow) Close() {
	if w == nil {
		return
	}
	w.IsOpen = false
	if w.Backend != nil {
		w.Backend.SetTileMapBrowserOpen(false)
	}
}

// RefreshMaps reloads the discovered map list from the backend.
func (w *TileMapBrowserWindow) RefreshMaps() error {
	if w == nil || w.Backend == nil {
		return nil
	}
	maps, err := w.Backend.ListTileMaps(w.SearchQuery)
	if err != nil {
		w.setStatus(err.Error(), true)
		return err
	}
	w.Maps = maps
	w.listLoaded = true
	return nil
}

func (w *TileMapBrowserWindow) refreshActive() {
	if w == nil || w.Backend == nil {
		return
	}
	w.Layers, _ = w.Backend.TileMapLayers()
	layer := ""
	if w.SelectedLayer >= 0 && w.SelectedLayer < len(w.Layers) {
		layer = w.Layers[w.SelectedLayer].Name
	}
	w.Tiles, _ = w.Backend.TileMapTiles(layer, w.TileQuery)
}

func (w *TileMapBrowserWindow) setStatus(msg string, isErr bool) {
	w.StatusMsg = msg
	w.StatusIsErr = isErr
}

// Draw renders the window. Call each frame from ImGuiOverlay.
func (w *TileMapBrowserWindow) Draw() {
	if w == nil || w.Backend == nil {
		return
	}
	backendOpen := w.Backend.IsTileMapBrowserOpen()
	if backendOpen != w.IsOpen {
		w.IsOpen = backendOpen
	}
	if !w.IsOpen {
		return
	}
	if !w.listLoaded {
		_ = w.RefreshMaps()
	}

	open := true
	imgui.SetNextWindowSizeV(imgui.NewVec2(960, 600), imgui.CondFirstUseEver)
	if !imgui.BeginV("Tile Map Browser###TileMapBrowserWindow", &open, imgui.WindowFlagsNone) {
		imgui.End()
		return
	}
	if !open {
		w.Close()
		imgui.End()
		return
	}

	avail := imgui.ContentRegionAvail()
	leftW := float32(240)
	if avail.X < 640 {
		leftW = avail.X * 0.35
	}

	// Left: map list
	imgui.BeginChildStrV("##TileMapMapPane", imgui.NewVec2(leftW, 0), imgui.ChildFlagsBorders, imgui.WindowFlagsNone)
	imgui.TextUnformatted("Maps")
	imgui.InputTextWithHint("##TileMapSearch", "Search maps...", &w.SearchQuery, imgui.InputTextFlagsNone, nil)
	if imgui.Button("Refresh") {
		_ = w.RefreshMaps()
	}
	imgui.BeginChildStrV("##TileMapListScroll", imgui.NewVec2(0, 0), imgui.ChildFlagsNone, imgui.WindowFlagsNone)
	for i := range w.Maps {
		m := &w.Maps[i]
		selected := w.SelectedMap == m.Path
		label := fmt.Sprintf("%s##map_%d", m.Name, i)
		if imgui.SelectableBoolV(label, selected, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
			w.SelectedMap = m.Path
		}
		if imgui.IsItemClicked() {
			if err := w.Backend.LoadTileMap(m.Path); err != nil {
				w.setStatus(err.Error(), true)
			} else {
				w.setStatus(fmt.Sprintf("Loaded %s", m.Path), false)
				w.refreshActive()
			}
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip(m.Path)
		}
	}
	if len(w.Maps) == 0 {
		imgui.TextDisabled("(no .tmx maps found)")
	}
	imgui.EndChild()
	imgui.EndChild()

	imgui.SameLine()

	// Right: layers + tiles + inspector
	imgui.BeginChildStrV("##TileMapDetailPane", imgui.NewVec2(0, 0), imgui.ChildFlagsBorders, imgui.WindowFlagsNone)
	info, err := w.Backend.ActiveTileMapInfo()
	if err != nil {
		imgui.TextColored(imgui.NewVec4(1.0, 0.65, 0.4, 1.0), "No active tile map. Select a map on the left to load it.")
	} else {
		imgui.TextColored(imgui.NewVec4(0.7, 0.85, 1.0, 1.0), "Active: "+info.Path)
		imgui.TextUnformatted(fmt.Sprintf("%dx%d tiles (tile %dx%d) | %d tiles | %d layers | %d objects",
			info.Width, info.Height, info.TileWidth, info.TileHeight, info.TileCount, info.LayerCount, info.ObjectCount))

		if imgui.Button("Reload From Disk") {
			if err := w.Backend.ReloadTileMap(); err != nil {
				w.setStatus(err.Error(), true)
			} else {
				w.setStatus("Reloaded active map", false)
				w.refreshActive()
			}
		}

		imgui.Separator()
		imgui.TextUnformatted("Layers")
		if len(w.Layers) == 0 {
			w.refreshActive()
		}
		for i := range w.Layers {
			l := &w.Layers[i]
			label := fmt.Sprintf("%s  (%dx%d, elev %.2f)##layer_%d", l.Name, l.Width, l.Height, l.Elevation, i)
			if imgui.SelectableBoolV(label, w.SelectedLayer == i, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
				w.SelectedLayer = i
				w.refreshActive()
			}
		}

		imgui.Separator()
		imgui.TextUnformatted("Tiles")
		imgui.InputTextWithHint("##TileMapTileSearch", "Filter tiles (name/type)...", &w.TileQuery, imgui.InputTextFlagsNone, nil)
		imgui.BeginChildStrV("##TileMapTileScroll", imgui.NewVec2(0, 160), imgui.ChildFlagsBorders, imgui.WindowFlagsNone)
		for i := range w.Tiles {
			t := &w.Tiles[i]
			label := fmt.Sprintf("[%s] (%d,%d) gid=%d %s %s##tile_%d",
				t.Layer, t.Col, t.Row, t.GID, t.Type, t.Name, i)
			if imgui.SelectableBoolV(label, w.SelectedTile != nil && w.SelectedTile.Col == t.Col && w.SelectedTile.Row == t.Row && w.SelectedTile.Layer == t.Layer, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
				copyTile := *t
				w.SelectedTile = &copyTile
			}
		}
		if len(w.Tiles) == 0 {
			imgui.TextDisabled("(no tiles match filter)")
		}
		imgui.EndChild()

		if w.SelectedTile != nil {
			imgui.Separator()
			imgui.TextUnformatted("Tile Inspector")
			t := w.SelectedTile
			imgui.TextUnformatted(fmt.Sprintf("Layer: %s", t.Layer))
			imgui.TextUnformatted(fmt.Sprintf("Grid: (%d, %d)", t.Col, t.Row))
			imgui.TextUnformatted(fmt.Sprintf("GID: %d  Type: %s", t.GID, t.Type))
			imgui.TextUnformatted(fmt.Sprintf("Name: %s", t.Name))
			imgui.TextUnformatted(fmt.Sprintf("Solid: %v  Elevation: %.2f  Walls: %08b", t.Solid, t.Elevation, t.Walls))
			if x, z, err := w.Backend.TileGridToWorld(t.Col, t.Row); err == nil {
				imgui.TextUnformatted(fmt.Sprintf("World: (%.2f, %.2f)", x, z))
			}
		}
	}

	imgui.Separator()
	if w.StatusIsErr {
		imgui.TextColored(imgui.NewVec4(1.0, 0.42, 0.42, 1.0), w.StatusMsg)
	} else if strings.TrimSpace(w.StatusMsg) != "" {
		imgui.TextColored(imgui.NewVec4(0.55, 0.95, 0.65, 1.0), w.StatusMsg)
	} else {
		imgui.TextDisabled("Ready")
	}
	imgui.End()
}
