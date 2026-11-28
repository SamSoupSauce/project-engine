package tileimporter

import (
	"math"

	"github.com/SamSoupSauce/project-engine/engine"
)

// BuildGLTFAsset converts a WorldMap3D into an indexed/flat colored GLTFAsset
// renderable by engine.MeshRenderable in OpenGL 3.3 Core without extra shader pipelines.
func BuildGLTFAsset(m3d *WorldMap3D) *engine.GLTFAsset {
	if m3d == nil || len(m3d.Tiles) == 0 {
		return nil
	}
	prim := engine.GLTFPrimitive{MaterialIndex: -1}
	minB := engine.NewVector3(math.MaxFloat32, math.MaxFloat32, math.MaxFloat32)
	maxB := engine.NewVector3(-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32)

	appendBox := func(cx, cy, cz, sx, sy, sz float32, baseCol [3]float32) {
		if sx <= 0 || sy <= 0 || sz <= 0 {
			return
		}
		hx, hy, hz := sx*0.5, sy*0.5, sz*0.5
		x0, x1 := cx-hx, cx+hx
		y0, y1 := cy-hy, cy+hy
		z0, z1 := cz-hz, cz+hz
		if x0 < minB.X {
			minB.X = x0
		}
		if y0 < minB.Y {
			minB.Y = y0
		}
		if z0 < minB.Z {
			minB.Z = z0
		}
		if x1 > maxB.X {
			maxB.X = x1
		}
		if y1 > maxB.Y {
			maxB.Y = y1
		}
		if z1 > maxB.Z {
			maxB.Z = z1
		}

		addQuad := func(a, b, c, d engine.Vector3, shade float32) {
			col := [4]float32{baseCol[0] * shade, baseCol[1] * shade, baseCol[2] * shade, 1.0}
			prim.Positions = append(prim.Positions, a, b, c, a, c, d)
			for i := 0; i < 6; i++ {
				prim.Colors = append(prim.Colors, col)
			}
		}
		p000 := engine.NewVector3(x0, y0, z0)
		p100 := engine.NewVector3(x1, y0, z0)
		p110 := engine.NewVector3(x1, y1, z0)
		p010 := engine.NewVector3(x0, y1, z0)
		p001 := engine.NewVector3(x0, y0, z1)
		p101 := engine.NewVector3(x1, y0, z1)
		p111 := engine.NewVector3(x1, y1, z1)
		p011 := engine.NewVector3(x0, y1, z1)

		addQuad(p011, p111, p110, p010, 1.00)
		addQuad(p000, p100, p101, p001, 0.55)
		addQuad(p001, p101, p111, p011, 0.85)
		addQuad(p100, p000, p010, p110, 0.72)
		addQuad(p101, p100, p110, p111, 0.78)
		addQuad(p000, p001, p011, p010, 0.66)
	}

	for i := range m3d.Tiles {
		t := &m3d.Tiles[i]
		if t.HasFlooring {
			col := [3]float32{0.23, 0.27, 0.34}
			if (t.Col+t.Row)%2 == 1 {
				col = [3]float32{0.28, 0.32, 0.40}
			}
			if t.Type == TileGrass {
				col = [3]float32{0.22, 0.48, 0.25}
				if (t.Col+t.Row)%2 == 1 {
					col = [3]float32{0.26, 0.55, 0.29}
				}
			}
			appendBox(t.FloorPosition.X, t.FloorPosition.Y, t.FloorPosition.Z,
				t.FloorSize.X, t.FloorSize.Y, t.FloorSize.Z, col)
		}
		for _, ws := range t.WallSegments {
			col := [3]float32{0.56, 0.53, 0.48}
			if ws.IsInterior {
				col = [3]float32{0.72, 0.68, 0.60}
			}
			appendBox(ws.Position.X, ws.Position.Y, ws.Position.Z, ws.Size.X, ws.Size.Y, ws.Size.Z, col)
		}
		if t.Type == TilePillar {
			px, pz := t.FloorPosition.X, t.FloorPosition.Z
			baseTop := t.FloorPosition.Y + t.FloorSize.Y*0.5
			h := DefaultWallHeight
			appendBox(px, baseTop+h*0.5, pz, m3d.TileSize*0.42, h, m3d.TileSize*0.42, [3]float32{0.58, 0.44, 0.30})
		}
		if t.Type == TileStairs || t.IsStairs {
			for _, step := range t.Steps {
				appendBox(step.Position.X, step.Position.Y, step.Position.Z,
					step.Size.X, step.Size.Y, step.Size.Z, [3]float32{0.64, 0.46, 0.28})
			}
		}
	}

	if len(prim.Positions) == 0 {
		return nil
	}
	return &engine.GLTFAsset{
		FilePath: "procedural://tilemap_world",
		Version:  "2.0",
		Meshes: []engine.GLTFMesh{
			{Name: "TileMapWorldMesh", Primitives: []engine.GLTFPrimitive{prim}},
		},
		Bounds: engine.BoundingBox{Min: minB, Max: maxB},
	}
}
