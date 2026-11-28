package luabridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/pkg/tileimporter"
)

func newTestTileMapService(t *testing.T) *TileMapService {
	t.Helper()
	b := NewLuaBridge()
	t.Cleanup(func() { b.Close() })
	svc := b.TileMapEditor()
	if svc == nil {
		t.Fatal("TileMapEditor service not initialized")
	}
	// Use the repo's assets/maps dir (tests run from pkg/luabridge).
	svc.SetMapsDir(filepath.Join("..", "..", "assets", "maps"))
	return svc
}

func TestTileMapDiscovery(t *testing.T) {
	svc := newTestTileMapService(t)
	maps, err := svc.ListMaps("")
	if err != nil {
		t.Fatalf("ListMaps: %v", err)
	}
	found := false
	for _, m := range maps {
		if filepath.ToSlash(m.Path) == "assets/maps/kitchen_sink.tmx" ||
			filepath.Base(m.Path) == "kitchen_sink.tmx" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected kitchen_sink.tmx in discovered maps: %+v", maps)
	}
}

func TestTileMapLoadAndInfo(t *testing.T) {
	svc := newTestTileMapService(t)
	path := filepath.Join("..", "..", "assets", "maps", "kitchen_sink.tmx")
	res, err := svc.LoadMap(path)
	if err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	if res.Width != 8 || res.Height != 8 {
		t.Fatalf("expected 8x8 map, got %dx%d", res.Width, res.Height)
	}
	if res.LayerCount != 2 {
		t.Fatalf("expected 2 layers, got %d", res.LayerCount)
	}
	info, err := svc.Info()
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.TileCount == 0 {
		t.Fatal("expected non-zero tile count")
	}
	if info.ObjectCount != 4 {
		t.Fatalf("expected 4 prop spawns, got %d", info.ObjectCount)
	}
	if info.TileSize != 1.25 || info.BaseY != -0.08 {
		t.Fatalf("unexpected tileSize/baseY: %v/%v", info.TileSize, info.BaseY)
	}
}

func TestTileMapLayersAndElevation(t *testing.T) {
	svc := newTestTileMapService(t)
	if _, err := svc.LoadMap(filepath.Join("..", "..", "assets", "maps", "kitchen_sink.tmx")); err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	layers, err := svc.Layers()
	if err != nil {
		t.Fatalf("Layers: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	var mezz *TileMapLayerInfo
	for i := range layers {
		if layers[i].Name == "Mezzanine" {
			mezz = &layers[i]
		}
	}
	if mezz == nil {
		t.Fatal("Mezzanine layer not found")
	}
	if mezz.Elevation != 1.4 {
		t.Fatalf("expected Mezzanine elevation 1.4, got %v", mezz.Elevation)
	}
}

func TestTileMapTilesAndLookup(t *testing.T) {
	svc := newTestTileMapService(t)
	if _, err := svc.LoadMap(filepath.Join("..", "..", "assets", "maps", "kitchen_sink.tmx")); err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	tiles, err := svc.Tiles("", "")
	if err != nil {
		t.Fatalf("Tiles: %v", err)
	}
	if len(tiles) == 0 {
		t.Fatal("expected tiles")
	}
	// tile id 10 in TMX = gid 11 = Wood Stairs North
	foundStairs := false
	for _, tile := range tiles {
		if tile.Type == "stairs" {
			foundStairs = true
		}
	}
	if !foundStairs {
		t.Fatal("expected at least one stairs tile")
	}
	if _, err := svc.TileAt(2, 2, "Ground"); err != nil {
		t.Fatalf("TileAt(2,2,Ground): %v", err)
	}
}

func TestTileMapGridWorldRoundTrip(t *testing.T) {
	svc := newTestTileMapService(t)
	if _, err := svc.LoadMap(filepath.Join("..", "..", "assets", "maps", "kitchen_sink.tmx")); err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	x, z, err := svc.GridToWorld(3, 4)
	if err != nil {
		t.Fatalf("GridToWorld: %v", err)
	}
	c, r, err := svc.WorldToGrid(x, z)
	if err != nil {
		t.Fatalf("WorldToGrid: %v", err)
	}
	if c != 3 || r != 4 {
		t.Fatalf("round trip mismatch: expected (3,4) got (%d,%d)", c, r)
	}
}

func TestTileMapCollidersAndRegistry(t *testing.T) {
	svc := newTestTileMapService(t)
	if _, err := svc.LoadMap(filepath.Join("..", "..", "assets", "maps", "kitchen_sink.tmx")); err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	bounds, err := svc.Colliders()
	if err != nil {
		t.Fatalf("Colliders: %v", err)
	}
	if len(bounds) == 0 {
		t.Fatal("expected solid collider bounds from kitchen_sink map")
	}
	reg, err := svc.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if len(reg) == 0 {
		t.Fatal("expected non-empty tile registry")
	}
}

func TestTileMapServiceBrowserBackend(t *testing.T) {
	svc := newTestTileMapService(t)
	var _ engine.TileMapBrowserBackend = svc
	if _, err := svc.ActiveTileMapInfo(); err == nil {
		t.Fatal("expected error before any map loaded")
	}
	if _, err := svc.LoadMap(filepath.Join("..", "..", "assets", "maps", "kitchen_sink.tmx")); err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	if _, err := svc.ActiveTileMapInfo(); err != nil {
		t.Fatalf("ActiveTileMapInfo after load: %v", err)
	}
}

func TestTileMapMissingMapErrors(t *testing.T) {
	svc := newTestTileMapService(t)
	if _, err := svc.LoadMap(filepath.Join(os.TempDir(), "does-not-exist.tmx")); err == nil {
		t.Fatal("expected error loading missing map")
	}
	if _, err := svc.TileAt(-1, -1, ""); err == nil {
		t.Fatal("expected error on TileAt before load")
	}
}

func TestBuildGLTFAsset(t *testing.T) {
	svc := newTestTileMapService(t)
	if _, err := svc.LoadMap(filepath.Join("..", "..", "assets", "maps", "kitchen_sink.tmx")); err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	w := svc.ActiveWorld()
	if w == nil {
		t.Fatal("no active world")
	}
	asset := tileimporter.BuildGLTFAsset(w)
	if asset == nil {
		t.Fatal("expected GLTF asset from kitchen_sink world")
	}
	if len(asset.Meshes) == 0 || len(asset.Meshes[0].Primitives) == 0 {
		t.Fatal("asset has no mesh/primitives")
	}
	if len(asset.Meshes[0].Primitives[0].Positions) == 0 {
		t.Fatal("asset has no vertex positions")
	}
}
