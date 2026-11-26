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
	parentTC := parent.EnsureTransform()
	parentTC.SetPosition(NewVector3(10, 0, 0))
	parentTC.SetScale(NewVector3(2, 2, 2))

	child := NewGameObject("child", "ChildNode")
	child.EnsureTransform().SetPosition(NewVector3(5, 0, 0))
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
	root.AddComponent(NewTransformComponent())
	rootRenderable := NewRenderable("mesh_root", "mat_root")
	rootRenderable.Flags = FlagVisible | FlagCastShadow
	root.AddComponent(rootRenderable)

	childA := NewGameObject("child_a", "ChildA")
	childA.AddComponent(NewTransformComponent())
	childARenderable := NewRenderable("mesh_child_a", "mat_child_a")
	childARenderable.Flags = FlagVisible | FlagWireframe
	childA.AddComponent(childARenderable)
	root.AddChild(childA)

	childB := NewGameObject("child_b", "ChildB")
	childB.AddComponent(NewTransformComponent())
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

type mockScriptComponent struct {
	owner   *GameObject
	loads   int
	unloads int
	spawns  []any
	events  []string
	ticks   []int
	despawn []any
}

func (m *mockScriptComponent) Init(owner *GameObject) { m.owner = owner }
func (m *mockScriptComponent) Update(dt float32)      {}
func (m *mockScriptComponent) Destroy()               { m.owner = nil }
func (m *mockScriptComponent) OnLoad(s Scene)         { m.loads++ }
func (m *mockScriptComponent) OnUnload(s Scene)       { m.unloads++ }
func (m *mockScriptComponent) OnSpawn(sender any)     { m.spawns = append(m.spawns, sender) }
func (m *mockScriptComponent) OnEvent(name string, sender *GameObject, data any) {
	m.events = append(m.events, name)
}
func (m *mockScriptComponent) OnTick(dtMs int) { m.ticks = append(m.ticks, dtMs) }
func (m *mockScriptComponent) OnDespawn(sender *GameObject, data any) {
	m.despawn = append(m.despawn, sender)
}

func TestBaseSceneLifecycleAndTickAccumulator(t *testing.T) {
	scene := NewBaseScene("TestScene")
	scene.TickIntervalMs = 20

	initialObj := NewGameObject("init_1", "Initial")
	initScript := &mockScriptComponent{}
	initialObj.AddComponent(initScript)
	scene.AddObject(initialObj)

	if scene.FindByID("init_1") != initialObj || scene.FindByName("Initial") != initialObj {
		t.Fatalf("expected scene queries to locate initial object")
	}

	scene.Init()
	if initScript.loads != 1 {
		t.Fatalf("expected 1 OnLoad call, got %d", initScript.loads)
	}
	if len(initScript.spawns) != 1 || initScript.spawns[0] != scene {
		t.Fatalf("expected initial spawn sender to be scene, got %+v", initScript.spawns)
	}

	// Accumulate 10ms (no tick yet), then 30ms (total 40ms -> 2 ticks of 20ms)
	scene.Update(0.010)
	if len(initScript.ticks) != 0 {
		t.Fatalf("expected 0 ticks after 10ms, got %d", len(initScript.ticks))
	}
	scene.Update(0.030)
	if len(initScript.ticks) != 2 || initScript.ticks[0] != 20 || initScript.ticks[1] != 20 {
		t.Fatalf("expected two 20ms ticks, got %+v", initScript.ticks)
	}

	// Dynamic runtime spawn with GameObject sender
	dynObj := NewGameObject("dyn_1", "Dynamic")
	dynScript := &mockScriptComponent{}
	dynObj.AddComponent(dynScript)
	scene.Spawn(dynObj, initialObj)

	if len(dynScript.spawns) != 1 || dynScript.spawns[0] != initialObj {
		t.Fatalf("expected dynamic spawn sender to be initialObj, got %+v", dynScript.spawns)
	}

	// Event bus dispatch
	initialObj.Emit("ping", map[string]any{"x": 1})
	if len(initScript.events) != 1 || len(dynScript.events) != 1 {
		t.Fatalf("expected both objects to receive scene event, got init=%v dyn=%v", initScript.events, dynScript.events)
	}

	// Despawn dynObj and ensure ticks no longer fire on it
	scene.Despawn(dynObj, initialObj, nil)
	if len(dynScript.despawn) != 1 || dynScript.despawn[0] != initialObj {
		t.Fatalf("expected OnDespawn with initialObj sender, got %+v", dynScript.despawn)
	}
	if scene.FindByID("dyn_1") != nil {
		t.Fatalf("expected despawned object to be removed from scene")
	}

	scene.Update(0.020)
	if len(dynScript.ticks) != 0 {
		t.Fatalf("expected despawned object not to receive ticks, got %+v", dynScript.ticks)
	}

	scene.Unload()
	if initScript.unloads != 1 {
		t.Fatalf("expected OnUnload on remaining object, got %d", initScript.unloads)
	}
}
