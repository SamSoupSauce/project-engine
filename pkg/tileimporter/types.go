package tileimporter

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Architectural constants for walls and flooring
const (
	WallThickness     float32 = 0.2
	FloorHeight       float32 = 0.08
	DefaultWallHeight float32 = 2.2
)

// TileType classifies semantic behavior of tiles in 3D world space.
type TileType int

const (
	TileEmpty TileType = iota
	TileFloor
	TileWall
	TileWallExterior
	TileWallInterior
	TileBarricade
	TilePillar
	TileWater
	TileGrass
	TileStairs
)

// Wall edge bitmasks representing which edge(s) of a square tile have walls (0 to 4 walls per tile).
const (
	WallEdgeNone  uint8 = 0
	WallEdgeNorth uint8 = 1 << 0 // -Z edge (facing North)
	WallEdgeSouth uint8 = 1 << 1 // +Z edge (facing South)
	WallEdgeEast  uint8 = 1 << 2 // +X edge (facing East)
	WallEdgeWest  uint8 = 1 << 3 // -X edge (facing West)
)

// Face direction masks for wall texturing and exterior siding
const (
	FaceNorth uint8 = 1 << 0 // -Z direction
	FaceSouth uint8 = 1 << 1 // +Z direction
	FaceEast  uint8 = 1 << 2 // +X direction
	FaceWest  uint8 = 1 << 3 // -X direction
)

// StairDirection indicates which direction a staircase ascends.
type StairDirection int

const (
	StairAscendNorth StairDirection = iota // Low at South (+Z), High at North (-Z)
	StairAscendSouth                       // Low at North (-Z), High at South (+Z)
	StairAscendEast                        // Low at West (-X), High at East (+X)
	StairAscendWest                        // Low at East (+X), High at West (-X)
)

// WallSegment represents a single ~0.2m thick wall along one edge of a square tile.
type WallSegment struct {
	Edge          uint8
	Type          TileType
	Position      rl.Vector3
	Size          rl.Vector3
	Color         rl.Color
	WireColor     rl.Color
	IsSolid       bool
	IsExterior    bool
	IsInterior    bool
	ExteriorFaces uint8
}

// GetBoundingBox returns the exact 3D collision bounding box for this 0.2m wall segment.
func (ws *WallSegment) GetBoundingBox() rl.BoundingBox {
	halfW := ws.Size.X / 2.0
	halfH := ws.Size.Y / 2.0
	halfD := ws.Size.Z / 2.0
	return rl.NewBoundingBox(
		rl.NewVector3(ws.Position.X-halfW, ws.Position.Y-halfH, ws.Position.Z-halfD),
		rl.NewVector3(ws.Position.X+halfW, ws.Position.Y+halfH, ws.Position.Z+halfD),
	)
}

// StairStep represents a single step within a 3D staircase.
type StairStep struct {
	Index       int
	Position    rl.Vector3
	Size        rl.Vector3
	BoundingBox rl.BoundingBox
}
