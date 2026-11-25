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
	Transform  Transform
	Parent     *GameObject
	Children   []*GameObject
	Components []Component

	mu sync.RWMutex
}

// NewGameObject initializes an active spatial entity with identity transform.
func NewGameObject(id, name string) *GameObject {
	return &GameObject{
		ID:         id,
		Name:       name,
		Active:     true,
		Transform:  NewTransform(),
		Children:   make([]*GameObject, 0),
		Components: make([]Component, 0),
	}
}

// AddChild links a child GameObject into this entity's hierarchy.
func (go_ *GameObject) AddChild(child *GameObject) {
	if child == nil || child == go_ {
		return
	}
	go_.mu.Lock()
	defer go_.mu.Unlock()

	// If child already has a parent, remove it first
	if child.Parent != nil {
		child.Parent.RemoveChild(child)
	}

	child.Parent = go_
	go_.Children = append(go_.Children, child)
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

	for _, c := range go_.Components {
		if reflect.TypeOf(c) == targetType || reflect.TypeOf(c).Implements(targetType) {
			return c
		}
	}
	return nil
}

// GetWorldTransform evaluates the cascaded world-space transform from root to this entity.
func (go_ *GameObject) GetWorldTransform() Transform {
	if go_.Parent == nil {
		return go_.Transform
	}
	parentWorld := go_.Parent.GetWorldTransform()
	return CombineTransforms(parentWorld, go_.Transform)
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
	defer go_.mu.Unlock()

	for _, c := range go_.Components {
		c.Destroy()
	}
	for _, child := range go_.Children {
		child.Destroy()
	}
}
