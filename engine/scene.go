package engine

import (
	"sync"
)

// DefaultTickIntervalMs is the default fixed simulation tick interval in milliseconds.
const DefaultTickIntervalMs = 16

// BaseScene provides a default Scene implementation with a hierarchical GameObject graph,
// fixed-rate tick dispatching, entity spawn/despawn management, and an event bus.
type BaseScene struct {
	Name           string
	Root           *GameObject
	Camera         Camera3D
	TickIntervalMs int

	loaded      bool
	tickAccumMs float64
	mu          sync.RWMutex
}

// NewBaseScene creates an initialized BaseScene with an empty root node and default 3D camera.
func NewBaseScene(name string) *BaseScene {
	root := NewGameObject("__root__", name+"_root")
	s := &BaseScene{
		Name:           name,
		Root:           root,
		TickIntervalMs: DefaultTickIntervalMs,
		Camera: Camera3D{
			Position:   NewVector3(0, 10, 10),
			Target:     NewVector3(0, 0, 0),
			Up:         NewVector3(0, 1, 0),
			Fovy:       45,
			Projection: CameraPerspective,
		},
	}
	root.SetScene(s)
	return s
}

// GetName returns the scene's name.
func (s *BaseScene) GetName() string {
	return s.Name
}

// IsLoaded reports whether the scene has been initialized and not yet unloaded.
func (s *BaseScene) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// GetRoot returns the root GameObject of the scene graph.
func (s *BaseScene) GetRoot() *GameObject {
	return s.Root
}

// Init loads the scene, triggers load(scene) on all attached scripts, and spawns initial objects.
func (s *BaseScene) Init() {
	s.mu.Lock()
	s.loaded = true
	s.tickAccumMs = 0
	s.mu.Unlock()

	s.Root.SetSceneRecursive(s)
	objects := s.collectAllObjects(true)

	for _, obj := range objects {
		obj.DispatchLoad(s)
	}

	for _, obj := range objects {
		if obj.Active {
			obj.SetSpawned(true)
			obj.DispatchSpawn(s)
		}
	}
}

// Update advances component updates and steps fixed-rate simulation ticks.
func (s *BaseScene) Update(dt float32) {
	if s.Root != nil {
		s.Root.Update(dt)
	}

	if !s.IsLoaded() || dt <= 0 {
		return
	}

	interval := s.effectiveTickIntervalMs()
	s.mu.Lock()
	s.tickAccumMs += float64(dt) * 1000.0
	ticks := 0
	for s.tickAccumMs+1e-4 >= float64(interval) {
		s.tickAccumMs -= float64(interval)
		if s.tickAccumMs < 0 {
			s.tickAccumMs = 0
		}
		ticks++
	}
	s.mu.Unlock()

	for i := 0; i < ticks; i++ {
		s.Tick(interval)
	}
}

// Tick executes a single fixed-rate simulation step in milliseconds across all spawned objects.
func (s *BaseScene) Tick(dtMs int) {
	if !s.IsLoaded() {
		return
	}
	for _, obj := range s.collectAllObjects(true) {
		if obj.Active && obj.IsSpawned() {
			obj.DispatchTick(dtMs)
		}
	}
}

// Draw3D provides a default no-op 3D render pass.
func (s *BaseScene) Draw3D() {}

// Draw2D provides a default no-op 2D render pass.
func (s *BaseScene) Draw2D() {}

// GetCamera returns the scene's active 3D camera.
func (s *BaseScene) GetCamera() Camera3D {
	return s.Camera
}

// Unload triggers unload(scene) across all scene objects before destroying the scene graph.
func (s *BaseScene) Unload() {
	s.mu.Lock()
	if !s.loaded {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	objects := s.collectAllObjects(true)
	for _, obj := range objects {
		obj.DispatchUnload(s)
	}

	s.mu.Lock()
	s.loaded = false
	s.mu.Unlock()

	for _, obj := range objects {
		obj.SetSpawned(false)
	}
	if s.Root != nil {
		s.Root.Destroy()
	}
}

// AddObject attaches a GameObject to the scene graph. If the scene is already loaded,
// it spawns the object with the scene as the sender.
func (s *BaseScene) AddObject(obj *GameObject) {
	if obj == nil {
		return
	}
	if s.IsLoaded() {
		s.Spawn(obj, s)
		return
	}
	if obj.Parent == nil && obj != s.Root {
		s.Root.AddChild(obj)
	}
	obj.SetSceneRecursive(s)
}

// Spawn attaches an object to the active scene and dispatches spawn(self, sender).
// If sender is nil, the scene instance is passed as sender.
func (s *BaseScene) Spawn(obj *GameObject, sender any) {
	if obj == nil {
		return
	}
	if obj.Parent == nil && obj != s.Root {
		s.Root.AddChild(obj)
	}
	obj.SetSceneRecursive(s)

	if sender == nil {
		sender = s
	}

	subtree := collectSubtree(obj)
	for _, node := range subtree {
		node.Active = true
		node.SetSpawned(true)
	}

	if s.IsLoaded() {
		for _, node := range subtree {
			node.DispatchSpawn(sender)
		}
	}
}

// Despawn invokes despawn(self, sender, data) immediately before removing and destroying the object.
func (s *BaseScene) Despawn(obj *GameObject, sender *GameObject, data any) {
	if obj == nil || obj == s.Root {
		return
	}

	subtree := collectSubtree(obj)
	for _, node := range subtree {
		node.DispatchDespawn(sender, data)
	}

	for _, node := range subtree {
		node.SetSpawned(false)
		node.SetScene(nil)
	}

	if obj.Parent != nil {
		obj.Parent.RemoveChild(obj)
	}
	obj.Destroy()
}

// Emit broadcasts an event across all active objects currently in the scene.
func (s *BaseScene) Emit(name string, sender *GameObject, data any) {
	for _, obj := range s.collectAllObjects(false) {
		if obj.Active {
			obj.DispatchEvent(name, sender, data)
		}
	}
}

// FindByID returns the first GameObject in the scene with the matching ID, or nil.
func (s *BaseScene) FindByID(id string) *GameObject {
	for _, obj := range s.collectAllObjects(false) {
		if obj.ID == id {
			return obj
		}
	}
	return nil
}

// FindByName returns the first GameObject in the scene with the matching Name, or nil.
func (s *BaseScene) FindByName(name string) *GameObject {
	for _, obj := range s.collectAllObjects(false) {
		if obj.Name == name {
			return obj
		}
	}
	return nil
}

// GetObjects returns a snapshot of all GameObjects attached under the scene root.
func (s *BaseScene) GetObjects() []*GameObject {
	return s.collectAllObjects(false)
}

// ObjectCount returns the number of GameObjects attached under the scene root.
func (s *BaseScene) ObjectCount() int {
	return len(s.collectAllObjects(false))
}

func (s *BaseScene) effectiveTickIntervalMs() int {
	if s.TickIntervalMs <= 0 {
		return DefaultTickIntervalMs
	}
	return s.TickIntervalMs
}

func (s *BaseScene) collectAllObjects(includeRoot bool) []*GameObject {
	if s.Root == nil {
		return nil
	}
	all := collectSubtree(s.Root)
	if includeRoot || len(all) == 0 {
		return all
	}
	return all[1:]
}

func collectSubtree(root *GameObject) []*GameObject {
	if root == nil {
		return nil
	}
	out := []*GameObject{root}
	root.mu.RLock()
	children := make([]*GameObject, len(root.Children))
	copy(children, root.Children)
	root.mu.RUnlock()

	for _, c := range children {
		out = append(out, collectSubtree(c)...)
	}
	return out
}
