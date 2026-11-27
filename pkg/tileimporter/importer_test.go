package tileimporter

import (
	"math"
	"testing"
)

const sampleTMX = `<?xml version="1.0" encoding="UTF-8"?>
<map version="1.10" orientation="orthogonal" width="4" height="4" tilewidth="32" tileheight="32">
 <tileset firstgid="1" name="test_tiles" tilewidth="32" tilecount="10">
  <tile id="0">
   <properties>
    <property name="name" value="Wood Floor"/>
    <property name="solid" type="bool" value="false"/>
   </properties>
  </tile>
  <tile id="1">
   <properties>
    <property name="name" value="House Corner (NW)"/>
    <property name="solid" type="bool" value="true"/>
    <property name="wall_type" value="exterior"/>
    <property name="walls" value="north,west"/>
   </properties>
  </tile>
  <tile id="2">
   <properties>
    <property name="name" value="House Wall (North)"/>
    <property name="solid" type="bool" value="true"/>
    <property name="wall_type" value="exterior"/>
    <property name="walls" value="north"/>
   </properties>
  </tile>
 </tileset>
 <layer id="1" name="Ground" width="4" height="4">
  <data encoding="csv">
2,3,3,2,
1,1,1,1,
1,1,1,1,
2,3,3,2
  </data>
 </layer>
 <layer id="2" name="SecondFloor" width="4" height="4">
  <properties>
   <property name="elevation" type="float" value="2.2"/>
  </properties>
  <data encoding="csv">
2,0,0,2,
0,0,0,0,
0,0,0,0,
2,0,0,2
  </data>
 </layer>
 <objectgroup id="3" name="Spawns">
  <object id="1" name="Hero" type="player" x="32" y="32" width="32" height="32">
   <properties>
    <property name="class" value="ranger"/>
   </properties>
  </object>
 </objectgroup>
</map>`

func TestTMXParsing(t *testing.T) {
	m, err := ParseTMX([]byte(sampleTMX))
	if err != nil {
		t.Fatalf("failed to parse TMX: %v", err)
	}

	if m.Width != 4 || m.Height != 4 {
		t.Fatalf("expected 4x4 map, got %dx%d", m.Width, m.Height)
	}
	if len(m.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(m.Layers))
	}
	if len(m.ObjectGroups) != 1 {
		t.Fatalf("expected 1 object group, got %d", len(m.ObjectGroups))
	}

	ground := &m.Layers[0]
	if len(ground.Tiles) != 4 || len(ground.Tiles[0]) != 4 {
		t.Fatalf("ground layer dimensions mismatch: %dx%d", len(ground.Tiles), len(ground.Tiles[0]))
	}
}

func TestCoordinateTransformations(t *testing.T) {
	m, err := ParseTMX([]byte(sampleTMX))
	if err != nil {
		t.Fatalf("failed to parse TMX: %v", err)
	}

	worldMap := NewWorldMap3D(m, 1.0, -2.2)

	// For a 4x4 map with tileSize 1.0 centered at (0, 0):
	// mapW = 4, mapH = 4
	// Col 0: (0 - 2 + 0.5) * 1.0 = -1.5
	// Col 3: (3 - 2 + 0.5) * 1.0 = +1.5
	wx0, wz0 := worldMap.GridToWorld(0, 0)
	if wx0 != -1.5 || wz0 != -1.5 {
		t.Errorf("expected GridToWorld(0,0) = (-1.5, -1.5), got (%f, %f)", wx0, wz0)
	}

	wx3, wz3 := worldMap.GridToWorld(3, 3)
	if wx3 != 1.5 || wz3 != 1.5 {
		t.Errorf("expected GridToWorld(3,3) = (1.5, 1.5), got (%f, %f)", wx3, wz3)
	}

	// Round-trip test WorldToGrid
	c0, r0 := worldMap.WorldToGrid(wx0, wz0)
	if c0 != 0 || r0 != 0 {
		t.Errorf("expected WorldToGrid(-1.5, -1.5) = (0, 0), got (%d, %d)", c0, r0)
	}

	c3, r3 := worldMap.WorldToGrid(wx3, wz3)
	if c3 != 3 || r3 != 3 {
		t.Errorf("expected WorldToGrid(1.5, 1.5) = (3, 3), got (%d, %d)", c3, r3)
	}
}

func TestCornerTilesTopAndBottomCollision(t *testing.T) {
	m, err := ParseTMX([]byte(sampleTMX))
	if err != nil {
		t.Fatalf("failed to parse TMX: %v", err)
	}

	worldMap := NewWorldMap3D(m, 1.0, -2.2)

	// Find the NW corner tile at (0, 0) on Ground layer
	var cornerTile *Tile3D
	for i := range worldMap.Tiles {
		tile := &worldMap.Tiles[i]
		if tile.Col == 0 && tile.Row == 0 && tile.LayerName == "Ground" {
			cornerTile = tile
			break
		}
	}

	if cornerTile == nil {
		t.Fatalf("ground corner tile at (0,0) not found")
	}

	if !cornerTile.IsCorner() {
		t.Errorf("expected tile at (0,0) to be a corner")
	}
	if !cornerTile.HasBottomCollision {
		t.Errorf("expected corner tile to have HasBottomCollision == true")
	}
	if !cornerTile.HasTopCollision {
		t.Errorf("expected corner tile to have HasTopCollision == true")
	}

	// Bottom collision box Y should be [-2.20, -2.12]
	bBox := cornerTile.GetBottomBoundingBox()
	if math.Abs(float64(bBox.Min.Y-(-2.20))) > 0.01 || math.Abs(float64(bBox.Max.Y-(-2.12))) > 0.01 {
		t.Errorf("bottom collision box Y expected [-2.20, -2.12], got [%f, %f]", bBox.Min.Y, bBox.Max.Y)
	}

	// Top collision box Y should be [0.00, 0.08]
	tBox := cornerTile.GetTopBoundingBox()
	if math.Abs(float64(tBox.Min.Y-0.00)) > 0.01 || math.Abs(float64(tBox.Max.Y-0.08)) > 0.01 {
		t.Errorf("top collision box Y expected [0.00, 0.08], got [%f, %f]", tBox.Min.Y, tBox.Max.Y)
	}

	// Verify solid boxes extraction
	boxes := worldMap.GetSolidBoxes()
	if len(boxes) == 0 {
		t.Errorf("expected non-empty solid boxes list")
	}
}

func TestContiguousStitchingAndMultiLayer(t *testing.T) {
	m, err := ParseTMX([]byte(sampleTMX))
	if err != nil {
		t.Fatalf("failed to parse TMX: %v", err)
	}

	worldMap := NewWorldMap3D(m, 1.0, -2.2)

	// Find Ground tile at (0,0) and SecondFloor tile at (0,0)
	var gTile, uTile *Tile3D
	for i := range worldMap.Tiles {
		tile := &worldMap.Tiles[i]
		if tile.Col == 0 && tile.Row == 0 {
			if tile.LayerName == "Ground" {
				gTile = tile
			} else if tile.LayerName == "SecondFloor" {
				uTile = tile
			}
		}
	}

	if gTile == nil || uTile == nil {
		t.Fatalf("expected both ground and second-floor tiles at (0,0)")
	}

	// Second floor should have elevation 2.2m
	if uTile.Elevation != 2.2 {
		t.Errorf("expected second floor elevation 2.2, got %f", uTile.Elevation)
	}

	// Ground ceiling [0.00, 0.08] should match SecondFloor floor [0.00, 0.08]
	gCeil := gTile.GetTopBoundingBox()
	uFloor := uTile.GetBottomBoundingBox()
	if math.Abs(float64(gCeil.Min.Y-uFloor.Min.Y)) > 0.001 || math.Abs(float64(gCeil.Max.Y-uFloor.Max.Y)) > 0.001 {
		t.Errorf("contiguous vertical stitching mismatch: ground ceil [%f, %f] vs upper floor [%f, %f]",
			gCeil.Min.Y, gCeil.Max.Y, uFloor.Min.Y, uFloor.Max.Y)
	}

	bounds := worldMap.ExportStaticColliderBounds()
	if len(bounds) < len(worldMap.Tiles) {
		t.Errorf("expected ExportStaticColliderBounds to emit at least one bound per tile (%d), got %d", len(worldMap.Tiles), len(bounds))
	}
}

func TestStairsAndPillarTileProperties(t *testing.T) {
	tmx := `<?xml version="1.0" encoding="UTF-8"?>
<map version="1.10" orientation="orthogonal" width="3" height="1" tilewidth="32" tileheight="32">
 <tileset firstgid="1" name="demo" tilewidth="32" tilecount="4">
  <tile id="0">
   <properties>
    <property name="name" value="Pillar"/>
    <property name="type" value="pillar"/>
    <property name="solid" type="bool" value="true"/>
   </properties>
  </tile>
  <tile id="1">
   <properties>
    <property name="name" value="Stairs"/>
    <property name="type" value="stairs"/>
    <property name="stair_dir" value="north"/>
    <property name="step_count" type="int" value="4"/>
    <property name="stair_height" type="float" value="1.2"/>
   </properties>
  </tile>
 </tileset>
 <layer id="1" name="Ground" width="3" height="1">
  <data encoding="csv">1,2,0</data>
 </layer>
</map>`
	m, err := ParseTMX([]byte(tmx))
	if err != nil {
		t.Fatalf("ParseTMX failed: %v", err)
	}
	w := NewWorldMap3D(m, 1.0, 0.0)
	if len(w.Tiles) != 2 {
		t.Fatalf("expected 2 tiles, got %d", len(w.Tiles))
	}
	if w.Tiles[0].Type != TilePillar || !w.Tiles[0].IsSolid {
		t.Fatalf("expected tile 0 to be solid pillar, got %+v", w.Tiles[0])
	}
	if w.Tiles[1].Type != TileStairs || !w.Tiles[1].IsStairs || len(w.Tiles[1].Steps) != 4 {
		t.Fatalf("expected tile 1 to be 4-step stairs, got type=%v steps=%d", w.Tiles[1].Type, len(w.Tiles[1].Steps))
	}
}
