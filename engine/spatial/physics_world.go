package spatial

/*
#cgo CFLAGS: -I${SRCDIR}/thirdparty/joltc/include
#cgo windows,amd64 LDFLAGS: ${SRCDIR}/thirdparty/joltc/lib/windows/amd64/libjoltc.a -lstdc++ -lm
#cgo linux,amd64 LDFLAGS: -ljoltc -lstdc++ -lm -lpthread
#cgo darwin LDFLAGS: -ljoltc -lstdc++ -lm

#include <stdlib.h>
#include "joltc.h"
#include "jolt_ffi.h"
*/
import "C"

import (
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/SamSoupSauce/project-engine/engine"
)

// Collision object layers used across static world geometry and dynamic/character entities.
const (
	LayerNonMoving  uint32 = 0
	LayerMoving     uint32 = 1
	numObjectLayers uint32 = 2

	broadPhaseNonMoving uint8 = 0
	broadPhaseMoving    uint8 = 1
	numBroadPhaseLayers uint32 = 2

	InvalidBodyID uint32 = 0xFFFFFFFF
)

// DefaultFixedTimestep is the deterministic 60 Hz simulation step (1/60s).
const DefaultFixedTimestep float32 = 1.0 / 60.0

var (
	jphInitMu   sync.Mutex
	jphRefCount int
)

func acquireJolt() error {
	jphInitMu.Lock()
	defer jphInitMu.Unlock()
	if jphRefCount == 0 {
		if !bool(C.JPH_Init()) {
			return fmt.Errorf("spatial: JPH_Init failed")
		}
	}
	jphRefCount++
	return nil
}

func releaseJolt() {
	jphInitMu.Lock()
	defer jphInitMu.Unlock()
	if jphRefCount > 0 {
		jphRefCount--
		if jphRefCount == 0 {
			C.JPH_Shutdown()
		}
	}
}

// PhysicsWorldConfig configures Jolt Physics system limits, gravity, and fixed-step accumulator behavior.
type PhysicsWorldConfig struct {
	MaxBodies             uint32
	NumBodyMutexes        uint32
	MaxBodyPairs          uint32
	MaxContactConstraints uint32
	Gravity               engine.Vector3
	FixedTimestep         float32
	MaxSubSteps           int
}

// DefaultPhysicsWorldConfig returns standard 60 Hz 3D physics configuration.
func DefaultPhysicsWorldConfig() PhysicsWorldConfig {
	return PhysicsWorldConfig{
		MaxBodies:             10240,
		NumBodyMutexes:        0,
		MaxBodyPairs:          65536,
		MaxContactConstraints: 10240,
		Gravity:               engine.NewVector3(0, -9.81, 0),
		FixedTimestep:         DefaultFixedTimestep,
		MaxSubSteps:           4,
	}
}

// PhysicsWorld wraps JPH_PhysicsSystem, JPH_BodyInterface, and JPH_JobSystem with
// deterministic 60 Hz fixed-timestep stepping and GameObject transform synchronization.
type PhysicsWorld struct {
	system      *C.JPH_PhysicsSystem
	bodyIface   *C.JPH_BodyInterface
	jobSystem   *C.JPH_JobSystem
	FixedStep   float32
	MaxSubSteps int

	accumulator float32
	stepCount   uint64

	bodies       map[uint32]*RigidBody
	bodiesByID   map[string]*RigidBody
	staticBodies []uint32
	characters   []*CharacterController
	syncTargets  map[uint32]*engine.GameObject

	mu     sync.Mutex
	closed bool
}

// NewPhysicsWorld allocates and initializes a Jolt PhysicsWorld with deterministic 60 Hz settings.
func NewPhysicsWorld(cfg PhysicsWorldConfig) (*PhysicsWorld, error) {
	if cfg.MaxBodies == 0 {
		cfg.MaxBodies = 10240
	}
	if cfg.MaxBodyPairs == 0 {
		cfg.MaxBodyPairs = 65536
	}
	if cfg.MaxContactConstraints == 0 {
		cfg.MaxContactConstraints = 10240
	}
	if cfg.FixedTimestep <= 0 {
		cfg.FixedTimestep = DefaultFixedTimestep
	}
	if cfg.MaxSubSteps <= 0 {
		cfg.MaxSubSteps = 4
	}
	if cfg.Gravity == (engine.Vector3{}) {
		cfg.Gravity = engine.NewVector3(0, -9.81, 0)
	}

	if err := acquireJolt(); err != nil {
		return nil, err
	}

	objPairFilter := C.JPH_ObjectLayerPairFilterTable_Create(C.uint32_t(numObjectLayers))
	C.JPH_ObjectLayerPairFilterTable_EnableCollision(objPairFilter, C.JPH_ObjectLayer(LayerNonMoving), C.JPH_ObjectLayer(LayerMoving))
	C.JPH_ObjectLayerPairFilterTable_EnableCollision(objPairFilter, C.JPH_ObjectLayer(LayerMoving), C.JPH_ObjectLayer(LayerNonMoving))
	C.JPH_ObjectLayerPairFilterTable_EnableCollision(objPairFilter, C.JPH_ObjectLayer(LayerMoving), C.JPH_ObjectLayer(LayerMoving))

	bpLayer := C.JPH_BroadPhaseLayerInterfaceTable_Create(C.uint32_t(numObjectLayers), C.uint32_t(numBroadPhaseLayers))
	C.JPH_BroadPhaseLayerInterfaceTable_MapObjectToBroadPhaseLayer(bpLayer, C.JPH_ObjectLayer(LayerNonMoving), C.JPH_BroadPhaseLayer(broadPhaseNonMoving))
	C.JPH_BroadPhaseLayerInterfaceTable_MapObjectToBroadPhaseLayer(bpLayer, C.JPH_ObjectLayer(LayerMoving), C.JPH_BroadPhaseLayer(broadPhaseMoving))

	objVsBpFilter := C.JPH_ObjectVsBroadPhaseLayerFilterTable_Create(
		bpLayer, C.uint32_t(numBroadPhaseLayers),
		objPairFilter, C.uint32_t(numObjectLayers),
	)

	var settings C.JPH_PhysicsSystemSettings
	settings.maxBodies = C.uint32_t(cfg.MaxBodies)
	settings.numBodyMutexes = C.uint32_t(cfg.NumBodyMutexes)
	settings.maxBodyPairs = C.uint32_t(cfg.MaxBodyPairs)
	settings.maxContactConstraints = C.uint32_t(cfg.MaxContactConstraints)
	settings.broadPhaseLayerInterface = bpLayer
	settings.objectLayerPairFilter = objPairFilter
	settings.objectVsBroadPhaseLayerFilter = objVsBpFilter

	sys := C.JPH_PhysicsSystem_Create(&settings)
	if sys == nil {
		releaseJolt()
		return nil, fmt.Errorf("spatial: JPH_PhysicsSystem_Create failed")
	}

	jobSys := C.JPH_JobSystemThreadPool_Create(nil)
	if jobSys == nil {
		C.JPH_PhysicsSystem_Destroy(sys)
		releaseJolt()
		return nil, fmt.Errorf("spatial: JPH_JobSystemThreadPool_Create failed")
	}

	jGrav := C.JPH_Vec3{x: C.float(cfg.Gravity.X), y: C.float(cfg.Gravity.Y), z: C.float(cfg.Gravity.Z)}
	C.JPH_PhysicsSystem_SetGravity(sys, &jGrav)

	return &PhysicsWorld{
		system:      sys,
		bodyIface:   C.JPH_PhysicsSystem_GetBodyInterface(sys),
		jobSystem:   jobSys,
		FixedStep:   cfg.FixedTimestep,
		MaxSubSteps: cfg.MaxSubSteps,
		bodies:      make(map[uint32]*RigidBody),
		bodiesByID:  make(map[string]*RigidBody),
		syncTargets: make(map[uint32]*engine.GameObject),
	}, nil
}

// Destroy releases all registered bodies, characters, Jolt physics system, job system, and global state ref.
func (w *PhysicsWorld) Destroy() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true

	chars := make([]*CharacterController, len(w.characters))
	copy(chars, w.characters)
	w.characters = nil

	bodies := make([]*RigidBody, 0, len(w.bodies))
	for _, b := range w.bodies {
		bodies = append(bodies, b)
	}
	w.bodies = nil
	w.bodiesByID = nil
	w.syncTargets = nil

	staticIDs := make([]uint32, len(w.staticBodies))
	copy(staticIDs, w.staticBodies)
	w.staticBodies = nil

	sys := w.system
	iface := w.bodyIface
	jobSys := w.jobSystem
	w.system = nil
	w.bodyIface = nil
	w.jobSystem = nil
	w.mu.Unlock()

	for _, c := range chars {
		c.destroyNative()
	}
	for _, b := range bodies {
		if iface != nil && b.BodyID != InvalidBodyID {
			if bool(C.JPH_BodyInterface_IsAdded(iface, C.JPH_BodyID(b.BodyID))) {
				C.JPH_BodyInterface_RemoveAndDestroyBody(iface, C.JPH_BodyID(b.BodyID))
			}
		}
		b.BodyID = InvalidBodyID
		b.World = nil
	}
	for _, id := range staticIDs {
		if iface != nil && id != InvalidBodyID {
			if bool(C.JPH_BodyInterface_IsAdded(iface, C.JPH_BodyID(id))) {
				C.JPH_BodyInterface_RemoveAndDestroyBody(iface, C.JPH_BodyID(id))
			}
		}
	}

	if jobSys != nil {
		C.JPH_JobSystem_Destroy(jobSys)
	}
	if sys != nil {
		C.JPH_PhysicsSystem_Destroy(sys)
	}
	releaseJolt()
}

// NativePointers returns the underlying JPH_PhysicsSystem* and JPH_BodyInterface* C pointers
// for direct zero-overhead LuaJIT FFI binding.
func (w *PhysicsWorld) NativePointers() (physicsSystem unsafe.Pointer, bodyInterface unsafe.Pointer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return unsafe.Pointer(w.system), unsafe.Pointer(w.bodyIface)
}

// Step implements engine.PhysicsWorldHandle using the 60 Hz fixed-timestep accumulator.
func (w *PhysicsWorld) Step(dt float32) {
	w.StepSimulation(dt)
}

// StepSimulation advances the physics simulation in fixed 1/60s steps using an accumulator,
// updates all CharacterControllers, and synchronizes body transforms to GameObjects / Renderables.
// Returns the number of fixed physics steps executed.
func (w *PhysicsWorld) StepSimulation(dt float32) int {
	if dt < 0 {
		return 0
	}

	w.mu.Lock()
	if w.closed || w.system == nil || w.jobSystem == nil {
		w.mu.Unlock()
		return 0
	}

	fixed := w.FixedStep
	if fixed <= 0 {
		fixed = DefaultFixedTimestep
	}
	maxSub := w.MaxSubSteps
	if maxSub <= 0 {
		maxSub = 4
	}

	w.accumulator += dt
	steps := 0
	const eps float32 = 1e-6

	for w.accumulator+eps >= fixed && steps < maxSub {
		w.accumulator -= fixed
		if w.accumulator < 0 {
			w.accumulator = 0
		}

		for _, c := range w.characters {
			c.stepLocked(w.system, fixed)
		}

		C.JPH_PhysicsSystem_Update(w.system, C.float(fixed), 1, w.jobSystem)
		w.stepCount++
		steps++
	}

	if steps > 0 {
		w.syncTransformsLocked()
	}
	w.mu.Unlock()
	return steps
}

// StepCount returns the total number of fixed simulation steps executed.
func (w *PhysicsWorld) StepCount() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stepCount
}

// Accumulator returns the remaining unconsumed time in the fixed-step accumulator.
func (w *PhysicsWorld) Accumulator() float32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.accumulator
}

// OptimizeBroadPhase rebuilds Jolt's broadphase acceleration structures after batch body insertions.
func (w *PhysicsWorld) OptimizeBroadPhase() {
	w.mu.Lock()
	sys := w.system
	w.mu.Unlock()
	if sys != nil {
		C.JPH_PhysicsSystem_OptimizeBroadPhase(sys)
	}
}

// SetGravity updates the physics world gravity vector.
func (w *PhysicsWorld) SetGravity(gravity engine.Vector3) {
	w.mu.Lock()
	sys := w.system
	w.mu.Unlock()
	if sys == nil {
		return
	}
	g := C.JPH_Vec3{x: C.float(gravity.X), y: C.float(gravity.Y), z: C.float(gravity.Z)}
	C.JPH_PhysicsSystem_SetGravity(sys, &g)
}

// GetGravity retrieves the current physics world gravity vector.
func (w *PhysicsWorld) GetGravity() engine.Vector3 {
	w.mu.Lock()
	sys := w.system
	w.mu.Unlock()
	if sys == nil {
		return engine.Vector3{}
	}
	var g C.JPH_Vec3
	C.JPH_PhysicsSystem_GetGravity(sys, &g)
	return engine.NewVector3(float32(g.x), float32(g.y), float32(g.z))
}

// Raycast casts a ray through the Jolt narrowphase query and returns the closest hit.
func (w *PhysicsWorld) Raycast(ray engine.Ray, maxDistance float32) engine.RaycastHit {
	w.mu.Lock()
	sys := w.system
	w.mu.Unlock()
	if sys == nil || maxDistance <= 0 {
		return engine.RaycastHit{}
	}

	orig := C.JoltVec3{x: C.float(ray.Position.X), y: C.float(ray.Position.Y), z: C.float(ray.Position.Z)}
	dir := C.JoltVec3{x: C.float(ray.Direction.X), y: C.float(ray.Direction.Y), z: C.float(ray.Direction.Z)}
	var res C.JoltHitResult
	if !bool(C.Jolt_RaycastOut(unsafe.Pointer(sys), &orig, &dir, C.float(maxDistance), &res)) {
		return engine.RaycastHit{}
	}

	bodyIDStr := fmt.Sprintf("%d", uint32(res.body_id))
	w.mu.Lock()
	if rb, ok := w.bodies[uint32(res.body_id)]; ok && rb.ID != "" {
		bodyIDStr = rb.ID
	}
	w.mu.Unlock()

	return engine.RaycastHit{
		Hit:      true,
		Distance: float32(res.distance),
		Point:    engine.NewVector3(float32(res.point_x), float32(res.point_y), float32(res.point_z)),
		Normal:   engine.NewVector3(float32(res.normal_x), float32(res.normal_y), float32(res.normal_z)),
		BodyID:   bodyIDStr,
	}
}

// AddBody registers a PhysicsBodyHandle (specifically *RigidBody) with this PhysicsWorld.
func (w *PhysicsWorld) AddBody(body engine.PhysicsBodyHandle) {
	if rb, ok := body.(*RigidBody); ok && rb != nil {
		rb.AttachToWorld(w)
	}
}

// RemoveBody removes a registered RigidBody by its string ID from the simulation.
func (w *PhysicsWorld) RemoveBody(id string) {
	w.mu.Lock()
	rb := w.bodiesByID[id]
	w.mu.Unlock()
	if rb != nil {
		rb.Destroy()
	}
}

// GetBody looks up a registered RigidBody by its string ID.
func (w *PhysicsWorld) GetBody(id string) engine.PhysicsBodyHandle {
	w.mu.Lock()
	defer w.mu.Unlock()
	rb := w.bodiesByID[id]
	if rb == nil {
		return nil
	}
	return rb
}

// RegisterSyncTarget links a raw Jolt BodyID to an engine.GameObject so its Transform
// (and any child Renderables) stays synchronized after every physics step.
func (w *PhysicsWorld) RegisterSyncTarget(bodyID uint32, target *engine.GameObject) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || bodyID == InvalidBodyID || target == nil {
		return
	}
	w.syncTargets[bodyID] = target
}

// SyncTransforms synchronizes all active RigidBody, CharacterController, and raw sync-target
// transforms into their owning GameObject.Transform (which feeds Renderable world matrices).
func (w *PhysicsWorld) SyncTransforms() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.syncTransformsLocked()
	}
}

func (w *PhysicsWorld) syncTransformsLocked() {
	if w.bodyIface == nil {
		return
	}
	for _, rb := range w.bodies {
		if rb.BodyID == InvalidBodyID || rb.Owner == nil {
			continue
		}
		if rb.BodyType == engine.BodyStatic {
			continue
		}
		syncBodyToGameObject(w.bodyIface, rb.BodyID, rb.Owner)
	}
	for bodyID, go_ := range w.syncTargets {
		if bodyID == InvalidBodyID || go_ == nil {
			continue
		}
		syncBodyToGameObject(w.bodyIface, bodyID, go_)
	}
	for _, c := range w.characters {
		if c.character != nil && c.Owner != nil {
			c.Owner.Transform.Position = c.getPositionNative()
		}
	}
}

func syncBodyToGameObject(iface *C.JPH_BodyInterface, bodyID uint32, go_ *engine.GameObject) {
	var pos C.JPH_RVec3
	var rot C.JPH_Quat
	C.JPH_BodyInterface_GetPositionAndRotation(iface, C.JPH_BodyID(bodyID), &pos, &rot)
	var eulerRad C.JPH_Vec3
	C.JPH_Quat_GetEulerAngles(&rot, &eulerRad)

	const rad2deg = 180.0 / math.Pi
	go_.Transform.Position = engine.NewVector3(float32(pos.x), float32(pos.y), float32(pos.z))
	go_.Transform.Rotation = engine.NewVector3(
		float32(float64(eulerRad.x)*rad2deg),
		float32(float64(eulerRad.y)*rad2deg),
		float32(float64(eulerRad.z)*rad2deg),
	)
}

func eulerDegToJoltQuat(deg engine.Vector3) C.JPH_Quat {
	const deg2rad = math.Pi / 180.0
	hx := float64(deg.X) * deg2rad * 0.5
	hy := float64(deg.Y) * deg2rad * 0.5
	hz := float64(deg.Z) * deg2rad * 0.5

	cx, sx := math.Cos(hx), math.Sin(hx)
	cy, sy := math.Cos(hy), math.Sin(hy)
	cz, sz := math.Cos(hz), math.Sin(hz)

	return C.JPH_Quat{
		x: C.float(sx*cy*cz - cx*sy*sz),
		y: C.float(cx*sy*cz + sx*cy*sz),
		z: C.float(cx*cy*sz - sx*sy*cz),
		w: C.float(cx*cy*cz + sx*sy*sz),
	}
}

// BodyCount returns the total number of bodies currently registered in the Jolt PhysicsSystem.
func (w *PhysicsWorld) BodyCount() uint32 {
	w.mu.Lock()
	sys := w.system
	w.mu.Unlock()
	if sys == nil {
		return 0
	}
	return uint32(C.JPH_PhysicsSystem_GetNumBodies(sys))
}
