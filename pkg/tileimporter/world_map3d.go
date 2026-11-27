package tileimporter

import (
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"

	"github.com/SamSoupSauce/project-engine/engine"
)

// Tile3D represents a single tile instantiated in 3D world space.
type Tile3D struct {
	GID           uint32
	LayerName     string
	Col           int
	Row           int
	Position      engine.Vector3
	Size          engine.Vector3
	Color         engine.Color
	WireColor     engine.Color
	IsSolid       bool
	Type          TileType
	Name          string
	IsExterior    bool
	IsInterior    bool
	ExteriorFaces uint8

	HasFlooring    bool
	FloorGID       uint32
	FloorColor     engine.Color
	FloorWireColor engine.Color
	FloorPosition  engine.Vector3
	FloorSize      engine.Vector3

	Walls        uint8
	WallSegments []WallSegment

	IsStairs    bool
	StairDir    StairDirection
	StepCount   int
	StairHeight float32
	StartHeight float32
	Steps       []StairStep
	StepBoxes   []engine.BoundingBox

	HasBottomCollision bool
	BottomCollisionBox engine.BoundingBox
	HasTopCollision    bool
	TopCollisionBox    engine.BoundingBox

	Elevation float32
}

// WallCount returns the number of active wall segments (0 to 4).
func (t *Tile3D) WallCount() int {
	return bits.OnesCount8(t.Walls)
}

// HasWall returns true if the specified wall edge bitmask is set.
func (t *Tile3D) HasWall(edge uint8) bool {
	return t.Walls&edge != 0
}

// IsCorner returns true if this tile has two perpendicular wall edges (NW, NE, SW, SE)
// or is explicitly named a corner.
func (t *Tile3D) IsCorner() bool {
	if strings.Contains(strings.ToLower(t.Name), "corner") {
		return true
	}
	if t.WallCount() == 2 {
		hasNorthOrSouth := (t.Walls&WallEdgeNorth != 0) || (t.Walls&WallEdgeSouth != 0)
		hasEastOrWest := (t.Walls&WallEdgeEast != 0) || (t.Walls&WallEdgeWest != 0)
		return hasNorthOrSouth && hasEastOrWest
	}
	return false
}

// GetBottomBoundingBox returns the bottom floor slab collision bounding box.
func (t *Tile3D) GetBottomBoundingBox() engine.BoundingBox {
	if t.HasBottomCollision {
		return t.BottomCollisionBox
	}
	ts := float32(1.0)
	if t.FloorSize.X > 0 {
		ts = t.FloorSize.X
	} else if t.Size.X > 0 {
		ts = t.Size.X
	}
	worldX := t.Position.X
	worldZ := t.Position.Z
	floorTopY := t.Position.Y
	if t.HasFlooring && t.FloorSize.Y > 0 {
		worldX = t.FloorPosition.X
		worldZ = t.FloorPosition.Z
		floorTopY = t.FloorPosition.Y + t.FloorSize.Y/2.0
	}
	return engine.NewBoundingBox(
		engine.NewVector3(worldX-ts/2.0, floorTopY-FloorHeight, worldZ-ts/2.0),
		engine.NewVector3(worldX+ts/2.0, floorTopY, worldZ+ts/2.0),
	)
}

// GetTopBoundingBox returns the top ceiling / cap slab collision bounding box.
func (t *Tile3D) GetTopBoundingBox() engine.BoundingBox {
	if t.HasTopCollision {
		return t.TopCollisionBox
	}
	ts := float32(1.0)
	if t.FloorSize.X > 0 {
		ts = t.FloorSize.X
	} else if t.Size.X > 0 {
		ts = t.Size.X
	}
	worldX := t.Position.X
	worldZ := t.Position.Z
	floorTopY := t.Position.Y
	if t.HasFlooring && t.FloorSize.Y > 0 {
		worldX = t.FloorPosition.X
		worldZ = t.FloorPosition.Z
		floorTopY = t.FloorPosition.Y + t.FloorSize.Y/2.0
	}
	wallH := DefaultWallHeight
	wallTopY := floorTopY + wallH
	return engine.NewBoundingBox(
		engine.NewVector3(worldX-ts/2.0, wallTopY-FloorHeight, worldZ-ts/2.0),
		engine.NewVector3(worldX+ts/2.0, wallTopY, worldZ+ts/2.0),
	)
}

// GetBoundingBox returns the encompassing 3D collision bounding box for this tile.
func (t *Tile3D) GetBoundingBox() engine.BoundingBox {
	if (t.Type == TileStairs || t.IsStairs) && len(t.StepBoxes) > 0 {
		bb := t.StepBoxes[0]
		for _, sb := range t.StepBoxes[1:] {
			if sb.Min.X < bb.Min.X { bb.Min.X = sb.Min.X }
			if sb.Min.Y < bb.Min.Y { bb.Min.Y = sb.Min.Y }
			if sb.Min.Z < bb.Min.Z { bb.Min.Z = sb.Min.Z }
			if sb.Max.X > bb.Max.X { bb.Max.X = sb.Max.X }
			if sb.Max.Y > bb.Max.Y { bb.Max.Y = sb.Max.Y }
			if sb.Max.Z > bb.Max.Z { bb.Max.Z = sb.Max.Z }
		}
		return bb
	}
	if len(t.WallSegments) > 0 {
		bb := t.WallSegments[0].GetBoundingBox()
		for _, ws := range t.WallSegments[1:] {
			wsBB := ws.GetBoundingBox()
			if wsBB.Min.X < bb.Min.X { bb.Min.X = wsBB.Min.X }
			if wsBB.Min.Y < bb.Min.Y { bb.Min.Y = wsBB.Min.Y }
			if wsBB.Min.Z < bb.Min.Z { bb.Min.Z = wsBB.Min.Z }
			if wsBB.Max.X > bb.Max.X { bb.Max.X = wsBB.Max.X }
			if wsBB.Max.Y > bb.Max.Y { bb.Max.Y = wsBB.Max.Y }
			if wsBB.Max.Z > bb.Max.Z { bb.Max.Z = wsBB.Max.Z }
		}
		if t.HasBottomCollision {
			if t.BottomCollisionBox.Min.X < bb.Min.X { bb.Min.X = t.BottomCollisionBox.Min.X }
			if t.BottomCollisionBox.Min.Y < bb.Min.Y { bb.Min.Y = t.BottomCollisionBox.Min.Y }
			if t.BottomCollisionBox.Min.Z < bb.Min.Z { bb.Min.Z = t.BottomCollisionBox.Min.Z }
			if t.BottomCollisionBox.Max.X > bb.Max.X { bb.Max.X = t.BottomCollisionBox.Max.X }
			if t.BottomCollisionBox.Max.Y > bb.Max.Y { bb.Max.Y = t.BottomCollisionBox.Max.Y }
			if t.BottomCollisionBox.Max.Z > bb.Max.Z { bb.Max.Z = t.BottomCollisionBox.Max.Z }
		}
		if t.HasTopCollision {
			if t.TopCollisionBox.Min.X < bb.Min.X { bb.Min.X = t.TopCollisionBox.Min.X }
			if t.TopCollisionBox.Min.Y < bb.Min.Y { bb.Min.Y = t.TopCollisionBox.Min.Y }
			if t.TopCollisionBox.Min.Z < bb.Min.Z { bb.Min.Z = t.TopCollisionBox.Min.Z }
			if t.TopCollisionBox.Max.X > bb.Max.X { bb.Max.X = t.TopCollisionBox.Max.X }
			if t.TopCollisionBox.Max.Y > bb.Max.Y { bb.Max.Y = t.TopCollisionBox.Max.Y }
			if t.TopCollisionBox.Max.Z > bb.Max.Z { bb.Max.Z = t.TopCollisionBox.Max.Z }
		}
		return bb
	}
	halfW := t.Size.X / 2.0
	halfH := t.Size.Y / 2.0
	halfD := t.Size.Z / 2.0
	return engine.NewBoundingBox(
		engine.NewVector3(t.Position.X-halfW, t.Position.Y-halfH, t.Position.Z-halfD),
		engine.NewVector3(t.Position.X+halfW, t.Position.Y+halfH, t.Position.Z+halfD),
	)
}

// rebuildWallSegments updates the 0.2m thick wall segments based on t.Walls.
func (t *Tile3D) rebuildWallSegments(ts, baseY float32) {
	t.WallSegments = make([]WallSegment, 0, 4)
	if t.Walls == 0 {
		if t.Type == TileWallExterior || t.Type == TileWallInterior || t.Type == TileWall {
			t.IsSolid = false
		}
		t.HasBottomCollision = false
		t.HasTopCollision = false
		return
	}
	t.IsSolid = true
	wallH := DefaultWallHeight
	floorTopY := baseY + FloorHeight
	if t.HasFlooring && t.FloorSize.Y > 0 {
		floorTopY = t.FloorPosition.Y + t.FloorSize.Y/2.0
	}
	centerY := floorTopY + wallH/2.0

	worldX := t.Position.X
	worldZ := t.Position.Z
	if t.HasFlooring {
		worldX = t.FloorPosition.X
		worldZ = t.FloorPosition.Z
	}

	edges := []uint8{WallEdgeNorth, WallEdgeSouth, WallEdgeEast, WallEdgeWest}
	for _, edge := range edges {
		if t.Walls&edge == 0 {
			continue
		}
		var pos, size engine.Vector3
		var extFaces uint8 = 0
		switch edge {
		case WallEdgeNorth:
			pos = engine.NewVector3(worldX, centerY, worldZ-ts/2.0+WallThickness/2.0)
			size = engine.NewVector3(ts, wallH, WallThickness)
			if t.IsExterior || t.Type == TileWallExterior { extFaces |= FaceNorth }
		case WallEdgeSouth:
			pos = engine.NewVector3(worldX, centerY, worldZ+ts/2.0-WallThickness/2.0)
			size = engine.NewVector3(ts, wallH, WallThickness)
			if t.IsExterior || t.Type == TileWallExterior { extFaces |= FaceSouth }
		case WallEdgeWest:
			pos = engine.NewVector3(worldX-ts/2.0+WallThickness/2.0, centerY, worldZ)
			size = engine.NewVector3(WallThickness, wallH, ts)
			if t.IsExterior || t.Type == TileWallExterior { extFaces |= FaceWest }
		case WallEdgeEast:
			pos = engine.NewVector3(worldX+ts/2.0-WallThickness/2.0, centerY, worldZ)
			size = engine.NewVector3(WallThickness, wallH, ts)
			if t.IsExterior || t.Type == TileWallExterior { extFaces |= FaceEast }
		}

		ws := WallSegment{
			Edge:          edge,
			Type:          t.Type,
			Position:      pos,
			Size:          size,
			Color:         t.Color,
			WireColor:     t.WireColor,
			IsSolid:       true,
			IsExterior:    t.IsExterior || t.Type == TileWallExterior,
			IsInterior:    t.IsInterior || t.Type == TileWallInterior,
			ExteriorFaces: extFaces,
		}
		t.WallSegments = append(t.WallSegments, ws)
	}

	if len(t.WallSegments) > 0 {
		t.IsSolid = true
		t.Position = t.WallSegments[0].Position
		t.Size = t.WallSegments[0].Size
	}

	// Corner tiles with floors have collision meshes on top and bottom
	if t.IsCorner() && t.HasFlooring {
		t.HasBottomCollision = true
		halfW := ts / 2.0
		halfD := ts / 2.0
		if t.FloorSize.X > 0 { halfW = t.FloorSize.X / 2.0 }
		if t.FloorSize.Z > 0 { halfD = t.FloorSize.Z / 2.0 }
		wallTopY := floorTopY + wallH
		t.BottomCollisionBox = engine.NewBoundingBox(
			engine.NewVector3(worldX-halfW, floorTopY-FloorHeight, worldZ-halfD),
			engine.NewVector3(worldX+halfW, floorTopY, worldZ+halfD),
		)
		t.HasTopCollision = true
		t.TopCollisionBox = engine.NewBoundingBox(
			engine.NewVector3(worldX-halfW, wallTopY-FloorHeight, worldZ-halfD),
			engine.NewVector3(worldX+halfW, wallTopY, worldZ+halfD),
		)
	} else {
		t.HasBottomCollision = false
		t.HasTopCollision = false
	}
}

// MapSpawnPoint represents an entity or trigger extracted from an objectgroup.
type MapSpawnPoint struct {
	ID         int
	Name       string
	Type       string
	Position   engine.Vector3
	Width      float32
	Height     float32
	Properties map[string]string
}

// WorldMap3D manages 3D level geometry and spatial entities instantiated from a TMX map.
type WorldMap3D struct {
	SourceMap *Map
	TileSize  float32
	BaseY     float32
	Tiles     []Tile3D
	Spawns    []MapSpawnPoint
}

// NewWorldMap3D creates an empty 3D world map container.
func NewWorldMap3D(m *Map, tileSize, baseY float32) *WorldMap3D {
	if tileSize <= 0 { tileSize = 1.0 }
	m3d := &WorldMap3D{
		SourceMap: m,
		TileSize:  tileSize,
		BaseY:     baseY,
		Tiles:     make([]Tile3D, 0),
		Spawns:    make([]MapSpawnPoint, 0),
	}
	m3d.Build()
	return m3d
}

// GridToWorld converts 2D grid coordinates (col, row) into 3D world coordinates (X, Z).
func (m3d *WorldMap3D) GridToWorld(col, row int) (float32, float32) {
	mapW := float32(16)
	mapH := float32(16)
	if m3d.SourceMap != nil {
		if m3d.SourceMap.Width > 0 { mapW = float32(m3d.SourceMap.Width) }
		if m3d.SourceMap.Height > 0 { mapH = float32(m3d.SourceMap.Height) }
	}
	worldX := (float32(col) - mapW/2.0 + 0.5) * m3d.TileSize
	worldZ := (float32(row) - mapH/2.0 + 0.5) * m3d.TileSize
	return worldX, worldZ
}

// WorldToGrid converts continuous 3D world coordinates (X, Z) into discrete 2D grid coordinates (col, row).
func (m3d *WorldMap3D) WorldToGrid(x, z float32) (int, int) {
	mapW := float32(16)
	mapH := float32(16)
	if m3d.SourceMap != nil {
		if m3d.SourceMap.Width > 0 { mapW = float32(m3d.SourceMap.Width) }
		if m3d.SourceMap.Height > 0 { mapH = float32(m3d.SourceMap.Height) }
	}
	col := int(math.Floor(float64(x/m3d.TileSize + mapW/2.0)))
	row := int(math.Floor(float64(z/m3d.TileSize + mapH/2.0)))
	return col, row
}

// Build constructs 3D tiles, walls, and spawn points from the loaded TMX map.
func (m3d *WorldMap3D) Build() {
	if m3d.SourceMap == nil {
		return
	}
	m := m3d.SourceMap
	ts := m3d.TileSize
	m3d.Tiles = make([]Tile3D, 0)
	m3d.Spawns = make([]MapSpawnPoint, 0)

	for _, layer := range m.Layers {
		if !layer.IsVisible() {
			continue
		}

		layerElevation := float32(0.0)
		if elevStr := layer.GetProperty("elevation", ""); elevStr != "" {
			if v, err := strconv.ParseFloat(elevStr, 32); err == nil {
				layerElevation = float32(v)
			}
		}

		for r := 0; r < len(layer.Tiles); r++ {
			for c := 0; c < len(layer.Tiles[r]); c++ {
				gid := layer.Tiles[r][c]
				if gid == 0 {
					continue
				}

				worldX, worldZ := m3d.GridToWorld(c, r)

				tile := Tile3D{
					GID:           gid,
					LayerName:     layer.Name,
					Col:           c,
					Row:           r,
					Position:      engine.NewVector3(worldX, m3d.BaseY+layerElevation+1.0/2.0, worldZ),
					Size:          engine.NewVector3(ts, 1.0, ts),
					IsSolid:       true,
					Type:          TileWall,
					Name:          fmt.Sprintf("Tile GID %d", gid),
					HasFlooring:   true,
					FloorPosition: engine.NewVector3(worldX, m3d.BaseY+layerElevation+FloorHeight/2.0, worldZ),
					FloorSize:     engine.NewVector3(ts, FloorHeight, ts),
					Elevation:     layerElevation,
				}

				// Check custom properties from tileset
				m3d.applyTileProperties(&tile)

				if tile.Type == TileStairs || tile.IsStairs {
					tile.rebuildStairSteps(ts, m3d.BaseY+layerElevation)
				} else {
					tile.rebuildWallSegments(ts, m3d.BaseY+layerElevation)
				}
				m3d.Tiles = append(m3d.Tiles, tile)
			}
		}
	}

	// Extract spawn points
	for _, og := range m.ObjectGroups {
		for _, obj := range og.Objects {
			col, row := m3d.WorldToGrid(obj.X, obj.Y)
			worldX, worldZ := m3d.GridToWorld(col, row)
			props := make(map[string]string)
			for _, p := range obj.Properties {
				props[p.Name] = p.GetVal()
			}
			m3d.Spawns = append(m3d.Spawns, MapSpawnPoint{
				ID:         obj.ID,
				Name:       obj.Name,
				Type:       obj.Type,
				Position:   engine.NewVector3(worldX, m3d.BaseY, worldZ),
				Width:      obj.Width,
				Height:     obj.Height,
				Properties: props,
			})
		}
	}
}

func (m3d *WorldMap3D) applyTileProperties(t *Tile3D) {
	if m3d.SourceMap == nil {
		return
	}
	gid := t.GID
	for _, tsSet := range m3d.SourceMap.Tilesets {
		if gid >= tsSet.FirstGID && gid < tsSet.FirstGID+uint32(tsSet.TileCount) {
			tileID := gid - tsSet.FirstGID
			for _, tt := range tsSet.Tiles {
				if tt.ID == tileID {
					for _, p := range tt.Props {
						pName := strings.ToLower(p.Name)
						pVal := strings.ToLower(p.GetVal())
						switch pName {
						case "name":
							t.Name = p.GetVal()
						case "solid":
							t.IsSolid = (pVal == "true" || pVal == "1")
						case "walls":
							t.Walls = parseWallMask(pVal)
						case "wall_type":
							if pVal == "exterior" {
								t.Type = TileWallExterior
								t.IsExterior = true
							} else if pVal == "interior" {
								t.Type = TileWallInterior
								t.IsInterior = true
							}
						case "type":
							switch pVal {
							case "floor":
								t.Type = TileFloor
								t.IsSolid = false
							case "pillar":
								t.Type = TilePillar
								t.IsSolid = true
							case "barricade":
								t.Type = TileBarricade
								t.IsSolid = true
							case "stairs":
								t.Type = TileStairs
								t.IsStairs = true
								t.IsSolid = true
							case "grass":
								t.Type = TileGrass
								t.IsSolid = false
							case "water":
								t.Type = TileWater
								t.IsSolid = false
							}
						case "stair_dir":
							switch pVal {
							case "north", "n":
								t.StairDir = StairAscendNorth
							case "south", "s":
								t.StairDir = StairAscendSouth
							case "east", "e":
								t.StairDir = StairAscendEast
							case "west", "w":
								t.StairDir = StairAscendWest
							}
						case "step_count":
							if v, err := strconv.Atoi(pVal); err == nil && v > 0 {
								t.StepCount = v
							}
						case "stair_height":
							if v, err := strconv.ParseFloat(pVal, 32); err == nil && v > 0 {
								t.StairHeight = float32(v)
							}
						}
					}
				}
			}
		}
	}
}

func (t *Tile3D) rebuildStairSteps(ts, baseY float32) {
	t.IsStairs = true
	t.Type = TileStairs
	t.IsSolid = true
	steps := t.StepCount
	if steps <= 0 {
		steps = 5
		t.StepCount = steps
	}
	stairH := t.StairHeight
	if stairH <= 0 {
		stairH = DefaultWallHeight
		t.StairHeight = stairH
	}
	floorTopY := baseY + FloorHeight
	if t.HasFlooring && t.FloorSize.Y > 0 {
		floorTopY = t.FloorPosition.Y + t.FloorSize.Y/2.0
	}
	t.StartHeight = floorTopY

	worldX := t.Position.X
	worldZ := t.Position.Z
	if t.HasFlooring {
		worldX = t.FloorPosition.X
		worldZ = t.FloorPosition.Z
	}

	t.Steps = make([]StairStep, 0, steps)
	t.StepBoxes = make([]engine.BoundingBox, 0, steps)
	stepSpan := ts / float32(steps)
	stepRise := stairH / float32(steps)

	for i := 0; i < steps; i++ {
		h := stepRise * float32(i+1)
		centerY := floorTopY + h*0.5
		var pos, size engine.Vector3
		offset := -ts*0.5 + stepSpan*(float32(i)+0.5)
		switch t.StairDir {
		case StairAscendNorth:
			// Low at South (+Z), high at North (-Z)
			pos = engine.NewVector3(worldX, centerY, worldZ-offset)
			size = engine.NewVector3(ts, h, stepSpan)
		case StairAscendSouth:
			// Low at North (-Z), high at South (+Z)
			pos = engine.NewVector3(worldX, centerY, worldZ+offset)
			size = engine.NewVector3(ts, h, stepSpan)
		case StairAscendEast:
			// Low at West (-X), high at East (+X)
			pos = engine.NewVector3(worldX+offset, centerY, worldZ)
			size = engine.NewVector3(stepSpan, h, ts)
		case StairAscendWest:
			// Low at East (+X), high at West (-X)
			pos = engine.NewVector3(worldX-offset, centerY, worldZ)
			size = engine.NewVector3(stepSpan, h, ts)
		}
		bb := engine.NewBoundingBox(
			engine.NewVector3(pos.X-size.X*0.5, pos.Y-size.Y*0.5, pos.Z-size.Z*0.5),
			engine.NewVector3(pos.X+size.X*0.5, pos.Y+size.Y*0.5, pos.Z+size.Z*0.5),
		)
		t.Steps = append(t.Steps, StairStep{
			Index:       i,
			Position:    pos,
			Size:        size,
			BoundingBox: bb,
		})
		t.StepBoxes = append(t.StepBoxes, bb)
	}
}

func parseWallMask(val string) uint8 {
	var mask uint8 = 0
	tokens := strings.FieldsFunc(strings.ToLower(val), func(r rune) bool {
		return r == ',' || r == '|' || r == ' '
	})
	for _, token := range tokens {
		switch token {
		case "north", "n": mask |= WallEdgeNorth
		case "south", "s": mask |= WallEdgeSouth
		case "east", "e":   mask |= WallEdgeEast
		case "west", "w":   mask |= WallEdgeWest
		case "all", "4":    mask = WallEdgeNorth | WallEdgeSouth | WallEdgeEast | WallEdgeWest
		}
	}
	return mask
}

// GetSolidBoxes returns the bounding boxes of all solid environment elements in the map.
func (m3d *WorldMap3D) GetSolidBoxes() []engine.BoundingBox {
	var boxes []engine.BoundingBox
	for _, t := range m3d.Tiles {
		if !t.IsSolid {
			continue
		}
		if len(t.WallSegments) > 0 {
			for _, ws := range t.WallSegments {
				if ws.IsSolid {
					boxes = append(boxes, ws.GetBoundingBox())
				}
			}
			if t.HasBottomCollision {
				boxes = append(boxes, t.BottomCollisionBox)
			}
			if t.HasTopCollision {
				boxes = append(boxes, t.TopCollisionBox)
			}
		} else {
			boxes = append(boxes, t.GetBoundingBox())
			if t.HasBottomCollision {
				boxes = append(boxes, t.BottomCollisionBox)
			}
			if t.HasTopCollision {
				boxes = append(boxes, t.TopCollisionBox)
			}
		}
	}
	return boxes
}

// ExportStaticColliderBounds exports all static floor slabs, corner top/bottom seams,
// solid wall segments, stair step boxes, and solid tile bounds as [min, max] XYZ pairs
// compatible with engine/spatial.MapColliderSource without introducing package coupling.
func (m3d *WorldMap3D) ExportStaticColliderBounds() [][2][3]float32 {
	if m3d == nil {
		return nil
	}
	var out [][2][3]float32
	appendBox := func(minX, minY, minZ, maxX, maxY, maxZ float32) {
		out = append(out, [2][3]float32{
			{minX, minY, minZ},
			{maxX, maxY, maxZ},
		})
	}
	appendCenterSize := func(pos, size engine.Vector3) {
		if size.X <= 0 || size.Y <= 0 || size.Z <= 0 {
			return
		}
		hx, hy, hz := size.X*0.5, size.Y*0.5, size.Z*0.5
		appendBox(pos.X-hx, pos.Y-hy, pos.Z-hz, pos.X+hx, pos.Y+hy, pos.Z+hz)
	}

	for _, tile := range m3d.Tiles {
		if tile.HasBottomCollision {
			b := tile.BottomCollisionBox
			appendBox(b.Min.X, b.Min.Y, b.Min.Z, b.Max.X, b.Max.Y, b.Max.Z)
		} else if tile.HasFlooring {
			appendCenterSize(tile.FloorPosition, tile.FloorSize)
		}

		if tile.HasTopCollision {
			b := tile.TopCollisionBox
			appendBox(b.Min.X, b.Min.Y, b.Min.Z, b.Max.X, b.Max.Y, b.Max.Z)
		}

		if tile.Type == TileStairs || tile.IsStairs {
			if len(tile.Steps) > 0 {
				for _, step := range tile.Steps {
					appendCenterSize(step.Position, step.Size)
				}
			} else {
				for _, sb := range tile.StepBoxes {
					appendBox(sb.Min.X, sb.Min.Y, sb.Min.Z, sb.Max.X, sb.Max.Y, sb.Max.Z)
				}
			}
			continue
		}

		if tile.IsSolid {
			if len(tile.WallSegments) > 0 {
				for _, ws := range tile.WallSegments {
					if ws.IsSolid {
						appendCenterSize(ws.Position, ws.Size)
					}
				}
			} else if !tile.HasFlooring {
				bb := tile.GetBoundingBox()
				appendBox(bb.Min.X, bb.Min.Y, bb.Min.Z, bb.Max.X, bb.Max.Y, bb.Max.Z)
			}
		}
	}
	return out
}
