package engine

import (
	"testing"
)

type mockComponent struct {
	inited    bool
	updated   bool
	destroyed bool
	owner     *GameObject
}

func (m *mockComponent) Init(owner *GameObject) {
	m.inited = true
	m.owner = owner
}

func (m *mockComponent) Update(dt float32) {
	m.updated = true
}

func (m *mockComponent) Destroy() {
	m.destroyed = true
}

func TestGameObjectHierarchyAndTransforms(t *testing.T) {
	parent := NewGameObject("parent", "ParentNode")
	parent.Transform.Position = NewVector3(10, 0, 0)
	parent.Transform.Scale = NewVector3(2, 2, 2)

	child := NewGameObject("child", "ChildNode")
	child.Transform.Position = NewVector3(5, 0, 0)
	parent.AddChild(child)

	if child.Parent != parent {
		t.Fatalf("expected child parent to be parent node")
	}
	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(parent.Children))
	}

	// World transform: parent (10, 0, 0) + child (5, 0, 0) * scale(2, 2, 2) = (20, 0, 0)
	childWorld := child.GetWorldTransform()
	if childWorld.Position.X != 20 {
		t.Errorf("expected child world X to be 20, got %f", childWorld.Position.X)
	}

	// Test Component Lifecycle
	comp := &mockComponent{}
	child.AddComponent(comp)
	if !comp.inited || comp.owner != child {
		t.Errorf("component was not initialized with owner")
	}

	parent.Update(0.016)
	if !comp.updated {
		t.Errorf("component update was not propagated down hierarchy")
	}

	parent.Destroy()
	if !comp.destroyed {
		t.Errorf("component destroy was not invoked")
	}
	if parent.Active || child.Active {
		t.Errorf("expected nodes to be inactive after Destroy")
	}
}

func TestRenderableCascadeEvaluation(t *testing.T) {
	root := NewGameObject("root", "Root")
	rootRenderable := NewRenderable("mesh_root", "mat_root")
	rootRenderable.Flags = FlagVisible | FlagCastShadow
	root.AddComponent(rootRenderable)

	childA := NewGameObject("child_a", "ChildA")
	childARenderable := NewRenderable("mesh_child_a", "mat_child_a")
	childARenderable.Flags = FlagVisible | FlagWireframe
	childA.AddComponent(childARenderable)
	root.AddChild(childA)

	childB := NewGameObject("child_b", "ChildB")
	childBRenderable := NewRenderable("mesh_child_b", "mat_child_b")
	// childB overrides parent and sets itself invisible
	childBRenderable.Flags = FlagOverrideParent // FlagVisible not set
	childB.AddComponent(childBRenderable)
	root.AddChild(childB)

	buffer := NewRenderPassBuffer(16)
	buffer.ExtractFromHierarchy(root)

	// We expect 2 draw commands: root and childA (childB is overridden to invisible)
	if len(buffer.Commands) != 2 {
		t.Fatalf("expected 2 draw commands, got %d", len(buffer.Commands))
	}

	cmdRoot := buffer.Commands[0]
	if cmdRoot.MeshID != "mesh_root" {
		t.Errorf("expected root mesh_root, got %s", cmdRoot.MeshID)
	}

	cmdA := buffer.Commands[1]
	if cmdA.MeshID != "mesh_child_a" {
		t.Errorf("expected childA mesh_child_a, got %s", cmdA.MeshID)
	}
	// ChildA should inherit parent's FlagCastShadow AND have its own FlagWireframe
	if !cmdA.Flags.Has(FlagCastShadow) {
		t.Errorf("childA should have inherited FlagCastShadow from parent")
	}
	if !cmdA.Flags.Has(FlagWireframe) {
		t.Errorf("childA should retain its own FlagWireframe")
	}

	// Now hide root: childA should cascade to invisible, but childB with FlagOverrideParent + FlagVisible could be visible
	rootRenderable.Flags &^= FlagVisible // Root is now hidden
	childBRenderable.Flags = FlagOverrideParent | FlagVisible // ChildB forces visibility

	buffer.Reset()
	buffer.ExtractFromHierarchy(root)

	// Since root is hidden, root is omitted and childA inherits hidden state.
	// Only childB with FlagOverrideParent | FlagVisible should be drawn!
	if len(buffer.Commands) != 1 {
		t.Fatalf("expected 1 draw command after root hidden, got %d", len(buffer.Commands))
	}
	if buffer.Commands[0].MeshID != "mesh_child_b" {
		t.Errorf("expected childB to override parent hiding, got %s", buffer.Commands[0].MeshID)
	}
}
