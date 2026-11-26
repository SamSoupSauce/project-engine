package spatial

/*
#include "joltc.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/SamSoupSauce/project-engine/engine"
)

// MapColliderSource is implemented by map/room geometry exporters (such as
// tileimporter.WorldMap3D) to emit static AABB [min, max] XYZ bounds without
// introducing direct graphics/importer package dependencies.
type MapColliderSource interface {
	ExportStaticColliderBounds() [][2][3]float32
}

type boxSubShape struct {
	center      engine.Vector3
	halfExtents engine.Vector3
}

func boxFromBounds(min, max engine.Vector3) (boxSubShape, bool) {
	hx := (max.X - min.X) * 0.5
	hy := (max.Y - min.Y) * 0.5
	hz := (max.Z - min.Z) * 0.5
	if hx <= 1e-4 || hy <= 1e-4 || hz <= 1e-4 {
		return boxSubShape{}, false
	}
	cx := (min.X + max.X) * 0.5
	cy := (min.Y + max.Y) * 0.5
	cz := (min.Z + max.Z) * 0.5
	return boxSubShape{
		center:      engine.NewVector3(cx, cy, cz),
		halfExtents: engine.NewVector3(hx, hy, hz),
	}, true
}

// BuildWorldMapCollider converts a MapColliderSource (such as tileimporter.WorldMap3D)
// into a single Jolt StaticCompoundShape static body containing all floor slabs,
// walls, corner seams, and staircase steps.
func BuildWorldMapCollider(world *PhysicsWorld, src MapColliderSource) (uint32, error) {
	return BuildWorldMapColliderWithUserData(world, src, 0)
}

// BuildWorldMapColliderWithUserData converts a MapColliderSource into a single Jolt
// StaticCompoundShape static body and attaches entity userData for raycast identification.
func BuildWorldMapColliderWithUserData(world *PhysicsWorld, src MapColliderSource, userData uint32) (uint32, error) {
	if world == nil {
		return InvalidBodyID, fmt.Errorf("spatial: physics world must not be nil")
	}
	if src == nil {
		return InvalidBodyID, fmt.Errorf("spatial: map collider source must not be nil")
	}
	return buildWorldMapColliderBounds(world, src.ExportStaticColliderBounds(), userData)
}

// BuildWorldMapColliderFromBounds compiles [min, max] XYZ bounds slices into a single
// Jolt StaticCompoundShape static body in PhysicsWorld.
func BuildWorldMapColliderFromBounds(world *PhysicsWorld, bounds [][2][3]float32) (uint32, error) {
	return buildWorldMapColliderBounds(world, bounds, 0)
}

func buildWorldMapColliderBounds(world *PhysicsWorld, bounds [][2][3]float32, userData uint32) (uint32, error) {
	if world == nil {
		return InvalidBodyID, fmt.Errorf("spatial: physics world must not be nil")
	}
	shapes := make([]boxSubShape, 0, len(bounds))
	for _, b := range bounds {
		minV := engine.NewVector3(b[0][0], b[0][1], b[0][2])
		maxV := engine.NewVector3(b[1][0], b[1][1], b[1][2])
		if sub, ok := boxFromBounds(minV, maxV); ok {
			shapes = append(shapes, sub)
		}
	}
	return buildStaticCompoundSubShapes(world, shapes, userData)
}

// BuildStaticCompoundFromBoxes compiles arbitrary axis-aligned BoundingBoxes into a single
// Jolt StaticCompoundShape static body in the PhysicsWorld.
func BuildStaticCompoundFromBoxes(world *PhysicsWorld, boxes []engine.BoundingBox) (uint32, error) {
	if world == nil {
		return InvalidBodyID, fmt.Errorf("spatial: physics world must not be nil")
	}
	shapes := make([]boxSubShape, 0, len(boxes))
	for _, bb := range boxes {
		if b, ok := boxFromBounds(bb.Min, bb.Max); ok {
			shapes = append(shapes, b)
		}
	}
	return buildStaticCompoundSubShapes(world, shapes, 0)
}

func buildStaticCompoundSubShapes(world *PhysicsWorld, shapes []boxSubShape, userData uint32) (uint32, error) {
	if len(shapes) == 0 {
		return InvalidBodyID, fmt.Errorf("spatial: no valid collider shapes generated")
	}

	world.mu.Lock()
	if world.closed || world.bodyIface == nil {
		world.mu.Unlock()
		return InvalidBodyID, fmt.Errorf("spatial: physics world is closed")
	}
	iface := world.bodyIface
	world.mu.Unlock()

	compoundSettings := C.JPH_StaticCompoundShapeSettings_Create()
	if compoundSettings == nil {
		return InvalidBodyID, fmt.Errorf("spatial: JPH_StaticCompoundShapeSettings_Create failed")
	}
	defer C.JPH_ShapeSettings_Destroy((*C.JPH_ShapeSettings)(unsafe.Pointer(compoundSettings)))

	identityRot := C.JPH_Quat{x: 0, y: 0, z: 0, w: 1}
	boxSettingsList := make([]*C.JPH_BoxShapeSettings, 0, len(shapes))
	defer func() {
		for _, bs := range boxSettingsList {
			C.JPH_ShapeSettings_Destroy((*C.JPH_ShapeSettings)(unsafe.Pointer(bs)))
		}
	}()

	for i, s := range shapes {
		half := C.JPH_Vec3{x: C.float(s.halfExtents.X), y: C.float(s.halfExtents.Y), z: C.float(s.halfExtents.Z)}
		// Use small convex radius for thin floor/wall slabs (0.08m floor -> <= 0.02m convex radius)
		convexR := float32(C.JPH_DEFAULT_CONVEX_RADIUS)
		minDim := s.halfExtents.X
		if s.halfExtents.Y < minDim {
			minDim = s.halfExtents.Y
		}
		if s.halfExtents.Z < minDim {
			minDim = s.halfExtents.Z
		}
		if minDim < convexR {
			convexR = minDim * 0.5
		}
		bs := C.JPH_BoxShapeSettings_Create(&half, C.float(convexR))
		boxSettingsList = append(boxSettingsList, bs)

		pos := C.JPH_Vec3{x: C.float(s.center.X), y: C.float(s.center.Y), z: C.float(s.center.Z)}
		C.JPH_CompoundShapeSettings_AddShape(
			(*C.JPH_CompoundShapeSettings)(unsafe.Pointer(compoundSettings)),
			&pos,
			&identityRot,
			(*C.JPH_ShapeSettings)(unsafe.Pointer(bs)),
			C.uint32_t(i),
		)
	}

	compoundShape := C.JPH_StaticCompoundShape_Create(compoundSettings)
	if compoundShape == nil {
		return InvalidBodyID, fmt.Errorf("spatial: JPH_StaticCompoundShape_Create failed")
	}
	defer C.JPH_Shape_Destroy((*C.JPH_Shape)(unsafe.Pointer(compoundShape)))

	zeroPos := C.JPH_RVec3{x: 0, y: 0, z: 0}
	bodySettings := C.JPH_BodyCreationSettings_Create3(
		(*C.JPH_Shape)(unsafe.Pointer(compoundShape)),
		&zeroPos,
		&identityRot,
		C.JPH_MotionType_Static,
		C.JPH_ObjectLayer(LayerNonMoving),
	)
	if bodySettings == nil {
		return InvalidBodyID, fmt.Errorf("spatial: JPH_BodyCreationSettings_Create3 failed")
	}
	defer C.JPH_BodyCreationSettings_Destroy(bodySettings)

	C.JPH_BodyCreationSettings_SetUserData(bodySettings, C.uint64_t(userData))
	bodyID := uint32(C.JPH_BodyInterface_CreateAndAddBody(iface, bodySettings, C.JPH_Activation_DontActivate))
	if bodyID == InvalidBodyID {
		return InvalidBodyID, fmt.Errorf("spatial: failed adding static compound collider body")
	}

	world.mu.Lock()
	if !world.closed {
		world.staticBodies = append(world.staticBodies, bodyID)
	}
	world.mu.Unlock()

	world.OptimizeBroadPhase()
	return bodyID, nil
}

// BuildStaticMeshCollider creates a static Jolt MeshShape rigid body from an indexed triangle list.
func BuildStaticMeshCollider(world *PhysicsWorld, vertices []engine.Vector3, indices []uint32) (uint32, error) {
	if world == nil {
		return InvalidBodyID, fmt.Errorf("spatial: physics world must not be nil")
	}
	if len(vertices) < 3 || len(indices) < 3 || len(indices)%3 != 0 {
		return InvalidBodyID, fmt.Errorf("spatial: invalid static mesh geometry (verts=%d indices=%d)", len(vertices), len(indices))
	}

	world.mu.Lock()
	if world.closed || world.bodyIface == nil {
		world.mu.Unlock()
		return InvalidBodyID, fmt.Errorf("spatial: physics world is closed")
	}
	iface := world.bodyIface
	world.mu.Unlock()

	jVerts := make([]C.JPH_Vec3, len(vertices))
	for i, v := range vertices {
		jVerts[i] = C.JPH_Vec3{x: C.float(v.X), y: C.float(v.Y), z: C.float(v.Z)}
	}
	triCount := len(indices) / 3
	jTris := make([]C.JPH_IndexedTriangle, triCount)
	for i := 0; i < triCount; i++ {
		jTris[i] = C.JPH_IndexedTriangle{
			i1: C.uint32_t(indices[i*3+0]),
			i2: C.uint32_t(indices[i*3+1]),
			i3: C.uint32_t(indices[i*3+2]),
		}
	}

	meshSettings := C.JPH_MeshShapeSettings_Create2(
		&jVerts[0], C.uint32_t(len(jVerts)),
		&jTris[0], C.uint32_t(triCount),
	)
	if meshSettings == nil {
		return InvalidBodyID, fmt.Errorf("spatial: JPH_MeshShapeSettings_Create2 failed")
	}
	defer C.JPH_ShapeSettings_Destroy((*C.JPH_ShapeSettings)(unsafe.Pointer(meshSettings)))

	meshShape := C.JPH_MeshShapeSettings_CreateShape(meshSettings)
	if meshShape == nil {
		return InvalidBodyID, fmt.Errorf("spatial: JPH_MeshShapeSettings_CreateShape failed")
	}
	defer C.JPH_Shape_Destroy((*C.JPH_Shape)(unsafe.Pointer(meshShape)))

	zeroPos := C.JPH_RVec3{x: 0, y: 0, z: 0}
	identityRot := C.JPH_Quat{x: 0, y: 0, z: 0, w: 1}
	bodySettings := C.JPH_BodyCreationSettings_Create3(
		(*C.JPH_Shape)(unsafe.Pointer(meshShape)),
		&zeroPos,
		&identityRot,
		C.JPH_MotionType_Static,
		C.JPH_ObjectLayer(LayerNonMoving),
	)
	defer C.JPH_BodyCreationSettings_Destroy(bodySettings)

	bodyID := uint32(C.JPH_BodyInterface_CreateAndAddBody(iface, bodySettings, C.JPH_Activation_DontActivate))
	if bodyID == InvalidBodyID {
		return InvalidBodyID, fmt.Errorf("spatial: failed adding static mesh collider body")
	}

	world.mu.Lock()
	if !world.closed {
		world.staticBodies = append(world.staticBodies, bodyID)
	}
	world.mu.Unlock()

	world.OptimizeBroadPhase()
	return bodyID, nil
}
