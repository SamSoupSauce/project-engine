package luabridge

import (
	"fmt"
	"strings"

	"github.com/SamSoupSauce/project-engine/engine"
)

// BindScene attaches an engine.BaseScene to both Console and LuaBridge for object picking and discovery.
func (c *Console) BindScene(scene *engine.BaseScene) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.Scene = scene
	c.mu.Unlock()
	if c.Bridge != nil {
		c.Bridge.BindScene(scene)
	}
}

// BindPhysics attaches an optional PhysicsWorldHandle used first during viewport raycast picking.
func (c *Console) BindPhysics(world engine.PhysicsWorldHandle) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.Physics = world
	c.mu.Unlock()
}

// PickObject binds obj as the console's active picked target, updates Lua globals `selected` / `picked`,
// and prints a structured selection log entry.
func (c *Console) PickObject(obj *engine.GameObject) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.PickedObject = obj
	c.mu.Unlock()

	if c.Bridge != nil {
		c.Bridge.SetPickedObject(obj)
	}
	if obj == nil {
		c.Print("[Unpicked] Selection cleared.", LineInfo)
		return
	}
	pos := obj.GetWorldTransform().Position
	c.Print(fmt.Sprintf("[Picked] %s (%s) @ (%.2f, %.2f, %.2f) -> bound to `selected`", obj.Name, obj.ID, pos.X, pos.Y, pos.Z), LineSystem)
}

// PickByNameOrID resolves a registered or scene GameObject by ID or Name and selects it.
func (c *Console) PickByNameOrID(query string) (*engine.GameObject, error) {
	if c == nil || c.Bridge == nil {
		return nil, fmt.Errorf("console has no LuaBridge")
	}
	obj := c.Bridge.findSceneOrRegisteredObject(query)
	if obj == nil {
		return nil, fmt.Errorf("object %q not found", query)
	}
	c.PickObject(obj)
	return obj, nil
}

// PickAtScreenPos casts a ray from camera screen coordinates into the bound Scene/PhysicsWorld
// and selects the intersected GameObject if hit.
func (c *Console) PickAtScreenPos(cam engine.Camera3D, mouseX, mouseY, viewportW, viewportH float32) engine.PickResult {
	if c == nil {
		return engine.PickResult{}
	}
	c.mu.RLock()
	scene := c.Scene
	phys := c.Physics
	c.mu.RUnlock()

	ray := engine.ScreenPointToRay(cam, mouseX, mouseY, viewportW, viewportH)
	res := engine.PickSceneObject(scene, ray, phys)
	if res.Hit && res.Object != nil {
		c.PickObject(res.Object)
	} else {
		c.Print("[Pick] No object hit under cursor.", LineInfo)
	}
	return res
}

// IsPickMode implements engine.DebugConsolePicker.
func (c *Console) IsPickMode() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PickMode
}

// SetPickMode implements engine.DebugConsolePicker.
func (c *Console) SetPickMode(enabled bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.PickMode = enabled
	c.mu.Unlock()
	if enabled {
		c.Print("[Pick Mode] Click any 3D object in the viewport to select it.", LineInfo)
	}
}

// GetPickedSummary implements engine.DebugConsolePicker.
func (c *Console) GetPickedSummary() (string, string, bool) {
	if c == nil {
		return "", "", false
	}
	c.mu.RLock()
	obj := c.PickedObject
	c.mu.RUnlock()
	if obj == nil {
		return "", "", false
	}
	return obj.ID, obj.Name, true
}

// ClearPicked implements engine.DebugConsolePicker.
func (c *Console) ClearPicked() {
	c.PickObject(nil)
}

func (b *LuaBridge) findSceneOrRegisteredObject(query string) *engine.GameObject {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	b.dataMu.RLock()
	if obj, ok := b.objects[q]; ok && obj != nil {
		b.dataMu.RUnlock()
		return obj
	}
	scene := b.scene
	b.dataMu.RUnlock()

	if scene != nil {
		if byID := scene.FindByID(q); byID != nil {
			return byID
		}
		if byName := scene.FindByName(q); byName != nil {
			return byName
		}
	}

	// Case-insensitive scan fallback across registered objects and scene nodes.
	qLower := strings.ToLower(q)
	b.dataMu.RLock()
	for name, obj := range b.objects {
		if obj != nil && (strings.ToLower(name) == qLower || strings.ToLower(obj.ID) == qLower || strings.ToLower(obj.Name) == qLower) {
			b.dataMu.RUnlock()
			return obj
		}
	}
	b.dataMu.RUnlock()

	if scene != nil && scene.Root != nil {
		return findInSubtreeCaseInsensitive(scene.Root, qLower)
	}
	return nil
}

func findInSubtreeCaseInsensitive(node *engine.GameObject, qLower string) *engine.GameObject {
	if node == nil {
		return nil
	}
	if node.ID != "__root__" && (strings.ToLower(node.ID) == qLower || strings.ToLower(node.Name) == qLower) {
		return node
	}
	for _, ch := range node.Children {
		if found := findInSubtreeCaseInsensitive(ch, qLower); found != nil {
			return found
		}
	}
	return nil
}
