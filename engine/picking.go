package engine

import (
	"math"
	"reflect"
)

// PickResult describes the closest GameObject hit by a viewport or world raycast.
type PickResult struct {
	Hit      bool
	Object   *GameObject
	Distance float32
	Point    Vector3
}

// ScreenPointToRay converts a screen-space cursor coordinate (top-left origin) into a world-space Ray
// using the given 3D camera orientation and viewport dimensions.
func ScreenPointToRay(cam Camera3D, mouseX, mouseY, viewportW, viewportH float32) Ray {
	if viewportW <= 0 {
		viewportW = 1280
	}
	if viewportH <= 0 {
		viewportH = 720
	}

	fwd := subVec3(cam.Target, cam.Position)
	fwd = normalizePickVec3(fwd)
	if fwd.X == 0 && fwd.Y == 0 && fwd.Z == 0 {
		fwd = NewVector3(0, 0, -1)
	}

	worldUp := normalizePickVec3(cam.Up)
	if worldUp.X == 0 && worldUp.Y == 0 && worldUp.Z == 0 {
		worldUp = NewVector3(0, 1, 0)
	}
	right := normalizePickVec3(crossVec3(fwd, worldUp))
	if right.X == 0 && right.Y == 0 && right.Z == 0 {
		right = NewVector3(1, 0, 0)
	}
	up := normalizePickVec3(crossVec3(right, fwd))

	ndcX := (2.0*mouseX)/viewportW - 1.0
	ndcY := 1.0 - (2.0*mouseY)/viewportH
	aspect := viewportW / viewportH

	fovy := cam.Fovy
	if fovy <= 0 {
		fovy = 50.0
	}

	if cam.Projection == CameraOrthographic {
		halfH := fovy * 0.5
		halfW := halfH * aspect
		origin := Vector3{
			X: cam.Position.X + right.X*(ndcX*halfW) + up.X*(ndcY*halfH),
			Y: cam.Position.Y + right.Y*(ndcX*halfW) + up.Y*(ndcY*halfH),
			Z: cam.Position.Z + right.Z*(ndcX*halfW) + up.Z*(ndcY*halfH),
		}
		return Ray{Position: origin, Direction: fwd}
	}

	tanHalf := float32(math.Tan(float64(fovy) * (math.Pi / 180.0) * 0.5))
	dir := Vector3{
		X: fwd.X + right.X*(ndcX*aspect*tanHalf) + up.X*(ndcY*tanHalf),
		Y: fwd.Y + right.Y*(ndcX*aspect*tanHalf) + up.Y*(ndcY*tanHalf),
		Z: fwd.Z + right.Z*(ndcX*aspect*tanHalf) + up.Z*(ndcY*tanHalf),
	}
	return Ray{
		Position:  cam.Position,
		Direction: normalizePickVec3(dir),
	}
}

// IntersectRayBoundingBox performs slab intersection between ray and axis-aligned box.
func IntersectRayBoundingBox(ray Ray, box BoundingBox) (bool, float32) {
	tMin := float32(0.0)
	tMax := float32(math.MaxFloat32)

	checkAxis := func(orig, dir, bMin, bMax float32) bool {
		if math.Abs(float64(dir)) < 1e-6 {
			return orig >= bMin && orig <= bMax
		}
		inv := 1.0 / dir
		t1 := (bMin - orig) * inv
		t2 := (bMax - orig) * inv
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tMin {
			tMin = t1
		}
		if t2 < tMax {
			tMax = t2
		}
		return tMin <= tMax
	}

	if !checkAxis(ray.Position.X, ray.Direction.X, box.Min.X, box.Max.X) {
		return false, 0
	}
	if !checkAxis(ray.Position.Y, ray.Direction.Y, box.Min.Y, box.Max.Y) {
		return false, 0
	}
	if !checkAxis(ray.Position.Z, ray.Direction.Z, box.Min.Z, box.Max.Z) {
		return false, 0
	}
	if tMax < 0 {
		return false, 0
	}
	return true, tMin
}

// EstimateGameObjectWorldBounds computes a world-space AABB for 3D ray picking fallback.
func EstimateGameObjectWorldBounds(obj *GameObject) BoundingBox {
	if obj == nil {
		return BoundingBox{}
	}
	wt := obj.GetWorldTransform()
	half := NewVector3(0.5, 0.5, 0.5)
	if mc := obj.GetMeshComponent(); mc != nil && mc.Asset != nil {
		b := mc.Asset.Bounds
		hx := (b.Max.X - b.Min.X) * 0.5
		hy := (b.Max.Y - b.Min.Y) * 0.5
		hz := (b.Max.Z - b.Min.Z) * 0.5
		if hx > 0.01 && hy > 0.01 && hz > 0.01 {
			half = NewVector3(hx, hy, hz)
		}
	}
	if comp := obj.GetComponent(reflect.TypeOf((*Renderable)(nil))); comp != nil {
		if r, ok := comp.(*Renderable); ok && r != nil && r.CustomSize != nil {
			if r.CustomSize.X > 0 {
				half.X = r.CustomSize.X * 0.5
			}
			if r.CustomSize.Y > 0 {
				half.Y = r.CustomSize.Y * 0.5
			}
			if r.CustomSize.Z > 0 {
				half.Z = r.CustomSize.Z * 0.5
			}
		}
	}
	sx := float32(math.Abs(float64(wt.Scale.X)))
	sy := float32(math.Abs(float64(wt.Scale.Y)))
	sz := float32(math.Abs(float64(wt.Scale.Z)))
	if sx == 0 {
		sx = 1
	}
	if sy == 0 {
		sy = 1
	}
	if sz == 0 {
		sz = 1
	}
	hx, hy, hz := half.X*sx, half.Y*sy, half.Z*sz
	return BoundingBox{
		Min: NewVector3(wt.Position.X-hx, wt.Position.Y-hy, wt.Position.Z-hz),
		Max: NewVector3(wt.Position.X+hx, wt.Position.Y+hy, wt.Position.Z+hz),
	}
}

// PickSceneObject finds the closest active 3D GameObject intersected by ray using physicsWorld
// if present and falling back to scene AABB ray intersection.
func PickSceneObject(scene *BaseScene, ray Ray, physicsWorld PhysicsWorldHandle) PickResult {
	var best PickResult
	bestDist := float32(math.MaxFloat32)

	if physicsWorld != nil && scene != nil {
		hit := physicsWorld.Raycast(ray, 500.0)
		if hit.Hit && hit.BodyID != "" {
			if obj := scene.FindByID(hit.BodyID); obj != nil && obj != scene.Root && obj.Active {
				best = PickResult{
					Hit:      true,
					Object:   obj,
					Distance: hit.Distance,
					Point:    hit.Point,
				}
				bestDist = hit.Distance
			}
		}
	}

	if scene == nil || scene.Root == nil {
		return best
	}
	for _, obj := range scene.collectAllObjects(false) {
		if obj == nil || obj == scene.Root || !obj.Active {
			continue
		}
		// Skip pure 2D screen-space HUD objects that have Renderable2D without 3D Renderable/Mesh.
		has2D := obj.GetComponent(reflect.TypeOf((*Renderable2D)(nil))) != nil
		has3D := obj.GetComponent(reflect.TypeOf((*Renderable)(nil))) != nil || obj.GetMeshRenderable() != nil || obj.GetMeshComponent() != nil
		if has2D && !has3D {
			continue
		}
		box := EstimateGameObjectWorldBounds(obj)
		hit, dist := IntersectRayBoundingBox(ray, box)
		if hit && dist < bestDist {
			bestDist = dist
			best = PickResult{
				Hit:      true,
				Object:   obj,
				Distance: dist,
				Point: Vector3{
					X: ray.Position.X + ray.Direction.X*dist,
					Y: ray.Position.Y + ray.Direction.Y*dist,
					Z: ray.Position.Z + ray.Direction.Z*dist,
				},
			}
		}
	}
	return best
}

func subVec3(a, b Vector3) Vector3 {
	return Vector3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z}
}

func crossVec3(a, b Vector3) Vector3 {
	return Vector3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}

func normalizePickVec3(v Vector3) Vector3 {
	m := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if m < 1e-6 {
		return Vector3{}
	}
	return Vector3{X: v.X / m, Y: v.Y / m, Z: v.Z / m}
}
