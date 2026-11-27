package engine

import (
	"reflect"
	"sync"
)

// GameObject represents a distinct spatial entity in the scene graph hierarchy.
// It contains strictly spatial identity, hierarchy links, active status, and attached components.
// Zero dependency on gameplay logic or concrete physics engines.
type GameObject struct {
	ID         string
	Name       string
	Active     bool
	Parent     *GameObject
	Children   []*GameObject
	Components []Component
	Scene      Scene

	spawned bool
	mu      sync.RWMutex
}

// NewGameObject initializes an empty spatial entity with zero default components.
func NewGameObject(id, name string) *GameObject {
	return &GameObject{
		ID:         id,
		Name:       name,
		Active:     true,
		Children:   make([]*GameObject, 0),
		Components: make([]Component, 0),
	}
}

// AddChild links a child GameObject into this entity's hierarchy.
func (go_ *GameObject) AddChild(child *GameObject) {
	if child == nil || child == go_ {
		return
	}
	if child.Parent == go_ {
		return
	}
	if child.Parent != nil {
		child.Parent.RemoveChild(child)
	}

	go_.mu.Lock()
	child.Parent = go_
	go_.Children = append(go_.Children, child)
	scene := go_.Scene
	go_.mu.Unlock()

	if scene != nil {
		child.SetSceneRecursive(scene)
	}
}

// RemoveChild unlinks a child GameObject from this entity.
func (go_ *GameObject) RemoveChild(child *GameObject) bool {
	if child == nil {
		return false
	}
	go_.mu.Lock()
	defer go_.mu.Unlock()

	for i, c := range go_.Children {
		if c == child {
			child.Parent = nil
			go_.Children = append(go_.Children[:i], go_.Children[i+1:]...)
			return true
		}
	}
	return false
}

// AddComponent attaches a component and invokes its Init lifecycle hook.
func (go_ *GameObject) AddComponent(c Component) {
	if c == nil {
		return
	}
	go_.mu.Lock()
	go_.Components = append(go_.Components, c)
	go_.mu.Unlock()

	c.Init(go_)
}

// GetComponent finds the first attached component of the matching type pointer.
func (go_ *GameObject) GetComponent(targetType reflect.Type) Component {
	go_.mu.RLock()
	defer go_.mu.RUnlock()

	if targetType == nil {
		return nil
	}
	isIface := targetType.Kind() == reflect.Interface
	for _, c := range go_.Components {
		if c == nil {
			continue
		}
		cType := reflect.TypeOf(c)
		if cType == targetType || (isIface && cType.Implements(targetType)) {
			return c
		}
	}
	return nil
}

// RemoveComponent detaches the first component matching targetType and calls Destroy().
func (go_ *GameObject) RemoveComponent(targetType reflect.Type) bool {
	if targetType == nil {
		return false
	}
	isIface := targetType.Kind() == reflect.Interface
	return go_.RemoveComponentWhere(func(c Component) bool {
		cType := reflect.TypeOf(c)
		return cType == targetType || (isIface && cType.Implements(targetType))
	})
}

// RemoveComponentWhere detaches the first component satisfying match and calls Destroy().
func (go_ *GameObject) RemoveComponentWhere(match func(Component) bool) bool {
	if go_ == nil || match == nil {
		return false
	}
	go_.mu.Lock()
	var removed Component
	for i, c := range go_.Components {
		if c == nil {
			continue
		}
		if match(c) {
			removed = c
			go_.Components = append(go_.Components[:i], go_.Components[i+1:]...)
			break
		}
	}
	go_.mu.Unlock()

	if removed != nil {
		removed.Destroy()
		return true
	}
	return false
}

// GetWorldTransform evaluates the cascaded world-space transform from attached TransformComponents.
func (go_ *GameObject) GetWorldTransform() Transform {
	if go_ == nil {
		return NewTransform()
	}
	local := NewTransform()
	if tc := go_.GetTransformComponent(); tc != nil {
		local = tc.Value()
	}
	if go_.Parent == nil {
		return local
	}
	parentWorld := go_.Parent.GetWorldTransform()
	return CombineTransforms(parentWorld, local)
}

// Update propagates delta time across all attached components and active child nodes.
func (go_ *GameObject) Update(dt float32) {
	if !go_.Active {
		return
	}

	go_.mu.RLock()
	// Update attached components
	for _, c := range go_.Components {
		c.Update(dt)
	}

	// Recurse down children
	for _, child := range go_.Children {
		child.Update(dt)
	}
	go_.mu.RUnlock()
}

// Destroy cleans up all attached components and recurses into children.
func (go_ *GameObject) Destroy() {
	go_.Active = false
	go_.mu.Lock()
	components := make([]Component, len(go_.Components))
	copy(components, go_.Components)
	children := make([]*GameObject, len(go_.Children))
	copy(children, go_.Children)
	go_.spawned = false
	go_.mu.Unlock()

	for _, c := range components {
		c.Destroy()
	}
	for _, child := range children {
		child.Destroy()
	}
}

// SetScene assigns the owning scene on this GameObject.
func (go_ *GameObject) SetScene(s Scene) {
	go_.mu.Lock()
	go_.Scene = s
	go_.mu.Unlock()
}

// GetScene returns the owning scene of this GameObject, if any.
func (go_ *GameObject) GetScene() Scene {
	go_.mu.RLock()
	defer go_.mu.RUnlock()
	return go_.Scene
}

// SetSceneRecursive assigns the owning scene on this GameObject and all descendants.
func (go_ *GameObject) SetSceneRecursive(s Scene) {
	go_.SetScene(s)
	go_.mu.RLock()
	children := make([]*GameObject, len(go_.Children))
	copy(children, go_.Children)
	go_.mu.RUnlock()

	for _, child := range children {
		child.SetSceneRecursive(s)
	}
}

// IsSpawned reports whether this GameObject is currently spawned in an active scene.
func (go_ *GameObject) IsSpawned() bool {
	go_.mu.RLock()
	defer go_.mu.RUnlock()
	return go_.spawned
}

// SetSpawned updates the spawned state of this GameObject.
func (go_ *GameObject) SetSpawned(spawned bool) {
	go_.mu.Lock()
	go_.spawned = spawned
	go_.mu.Unlock()
}

// Emit broadcasts a named event with optional payload data across the owning scene's event bus,
// or directly to this GameObject subtree when not attached to a scene.
func (go_ *GameObject) Emit(name string, data any) {
	scene := go_.GetScene()
	if scene != nil {
		if bus, ok := scene.(EventBus); ok {
			bus.Emit(name, go_, data)
			return
		}
	}
	go_.dispatchEventTree(name, go_, data)
}

// EmitTo sends a named event directly to a specific target GameObject with this object as sender.
func (go_ *GameObject) EmitTo(target *GameObject, name string, data any) {
	if target == nil || !target.Active {
		return
	}
	target.DispatchEvent(name, go_, data)
}

func (go_ *GameObject) dispatchEventTree(name string, sender *GameObject, data any) {
	if !go_.Active {
		return
	}
	go_.DispatchEvent(name, sender, data)
	go_.mu.RLock()
	children := make([]*GameObject, len(go_.Children))
	copy(children, go_.Children)
	go_.mu.RUnlock()

	for _, child := range children {
		child.dispatchEventTree(name, sender, data)
	}
}

func (go_ *GameObject) forEachScript(fn func(ScriptLifecycle)) {
	go_.mu.RLock()
	scripts := make([]ScriptLifecycle, 0, len(go_.Components))
	for _, c := range go_.Components {
		if sl, ok := c.(ScriptLifecycle); ok {
			scripts = append(scripts, sl)
		}
	}
	go_.mu.RUnlock()

	for _, s := range scripts {
		fn(s)
	}
}

// DispatchLoad triggers OnLoad(scene) on all attached ScriptLifecycle components.
func (go_ *GameObject) DispatchLoad(scene Scene) {
	go_.forEachScript(func(s ScriptLifecycle) {
		s.OnLoad(scene)
	})
}

// DispatchUnload triggers OnUnload(scene) on all attached ScriptLifecycle components.
func (go_ *GameObject) DispatchUnload(scene Scene) {
	go_.forEachScript(func(s ScriptLifecycle) {
		s.OnUnload(scene)
	})
}

// DispatchSpawn triggers OnSpawn(sender) on all attached ScriptLifecycle components.
func (go_ *GameObject) DispatchSpawn(sender any) {
	go_.forEachScript(func(s ScriptLifecycle) {
		s.OnSpawn(sender)
	})
}

// DispatchEvent triggers OnEvent(name, sender, data) on all attached ScriptLifecycle components.
func (go_ *GameObject) DispatchEvent(name string, sender *GameObject, data any) {
	go_.forEachScript(func(s ScriptLifecycle) {
		s.OnEvent(name, sender, data)
	})
}

// DispatchTick triggers OnTick(dtMs) on all attached ScriptLifecycle components.
func (go_ *GameObject) DispatchTick(dtMs int) {
	go_.forEachScript(func(s ScriptLifecycle) {
		s.OnTick(dtMs)
	})
}

// DispatchDespawn triggers OnDespawn(sender, data) on all attached ScriptLifecycle components.
func (go_ *GameObject) DispatchDespawn(sender *GameObject, data any) {
	go_.forEachScript(func(s ScriptLifecycle) {
		s.OnDespawn(sender, data)
	})
}

// DispatchRender triggers OnRenderWithScriptName / OnRender on all attached RenderLifecycle
// components when this GameObject is active and spawned.
func (go_ *GameObject) DispatchRender(gfx *Renderer, scriptName string) {
	if gfx == nil || !go_.Active || !go_.IsSpawned() {
		return
	}
	go_.mu.RLock()
	targets := make([]RenderLifecycle, 0, len(go_.Components))
	for _, c := range go_.Components {
		if rl, ok := c.(RenderLifecycle); ok {
			targets = append(targets, rl)
		}
	}
	go_.mu.RUnlock()

	for _, rl := range targets {
		if nrl, ok := rl.(NamedRenderLifecycle); ok {
			nrl.OnRenderWithScriptName(gfx, scriptName)
		} else {
			rl.OnRender(gfx)
		}
	}
}
