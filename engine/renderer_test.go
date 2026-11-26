package engine

import "testing"

type mockRenderScriptComponent struct {
	owner       *GameObject
	renderCalls int
	scriptNames []string
	onRenderFn  func(owner *GameObject, gfx *Renderer, scriptName string)
}

func (m *mockRenderScriptComponent) Init(owner *GameObject) { m.owner = owner }
func (m *mockRenderScriptComponent) Update(dt float32)      {}
func (m *mockRenderScriptComponent) Destroy()               { m.owner = nil }
func (m *mockRenderScriptComponent) OnRender(gfx *Renderer) {
	m.OnRenderWithScriptName(gfx, "")
}
func (m *mockRenderScriptComponent) OnRenderWithScriptName(gfx *Renderer, scriptName string) {
	m.renderCalls++
	m.scriptNames = append(m.scriptNames, scriptName)
	if m.onRenderFn != nil {
		m.onRenderFn(m.owner, gfx, scriptName)
	}
}

func TestRendererBuffersAndSorting(t *testing.T) {
	r := NewRenderer(800, 600)
	w, h := r.ViewportSize()
	if w != 800 || h != 600 {
		t.Fatalf("expected viewport 800x600, got %.0fx%.0f", w, h)
	}

	r.DrawMesh("cube", "lit", NewTransform(), White, FlagVisible|FlagCastShadow)
	if len(r.Commands3D()) != 1 || r.Commands3D()[0].MeshID != "cube" {
		t.Fatalf("expected 1 3D DrawCommand with MeshID=cube, got %+v", r.Commands3D())
	}

	r.DrawRect(10, 20, 100, 40, NewColor(255, 0, 0, 255), 5)
	r.DrawTexturedQuad(0, 0, 32, 32, 7, Rect2D{U: 0, V: 0, W: 0.5, H: 0.5}, White, 1, BlendAdditive)
	r.DrawRect(5, 5, 10, 10, Black, 5)

	sorted := r.SortedCommands2D()
	if len(sorted) != 3 {
		t.Fatalf("expected 3 2D commands, got %d", len(sorted))
	}
	if sorted[0].ZIndex != 1 || sorted[0].TextureID != 7 {
		t.Errorf("expected z=1 tex=7 first, got %+v", sorted[0])
	}
	if sorted[1].ZIndex != 5 || sorted[1].X != 10 {
		t.Errorf("expected first z=5 rect at x=10 second, got %+v", sorted[1])
	}
	if sorted[2].ZIndex != 5 || sorted[2].X != 5 {
		t.Errorf("expected second z=5 rect at x=5 third, got %+v", sorted[2])
	}

	r.Reset(1920, 1080)
	w, h = r.ViewportSize()
	if w != 1920 || h != 1080 {
		t.Errorf("expected viewport 1920x1080 after Reset, got %.0fx%.0f", w, h)
	}
	if len(r.Commands3D()) != 0 || len(r.Commands2D) != 0 {
		t.Errorf("expected empty 3D and 2D buffers after Reset")
	}
}

func TestBaseSceneRenderDispatchGatingAndCascade(t *testing.T) {
	scene := NewBaseScene("RenderGateScene")

	// 1. Spawned object WITH Renderable -> should render
	hero := NewGameObject("hero", "Hero")
	heroR := NewRenderable("hero_mesh", "default")
	heroR.ScriptName = "HeroScript"
	heroScript := &mockRenderScriptComponent{
		onRenderFn: func(owner *GameObject, gfx *Renderer, _ string) {
			gfx.DrawRect(1, 2, 10, 10, White, 0)
		},
	}
	hero.AddComponent(heroR)
	hero.AddComponent(heroScript)
	scene.AddObject(hero)

	// 2. Spawned object WITHOUT Renderable -> must NOT trigger render hook
	logicOnly := NewGameObject("logic", "LogicOnly")
	logicScript := &mockRenderScriptComponent{}
	logicOnly.AddComponent(logicScript)
	scene.AddObject(logicOnly)

	// 3. Object with Renderable BEFORE scene.Init() (not spawned yet) -> must NOT render
	gfx := NewRenderer(1280, 720)
	DispatchRenderFromHierarchy(scene.Root, gfx)
	if heroScript.renderCalls != 0 {
		t.Fatalf("expected 0 render calls before spawn, got %d", heroScript.renderCalls)
	}

	// Init spawns initial objects
	scene.Init()
	scene.Draw3D()

	if heroScript.renderCalls != 1 {
		t.Fatalf("expected 1 render call on spawned Renderable hero, got %d", heroScript.renderCalls)
	}
	if heroScript.scriptNames[0] != "HeroScript" {
		t.Errorf("expected ScriptName HeroScript, got %q", heroScript.scriptNames[0])
	}
	if logicScript.renderCalls != 0 {
		t.Errorf("expected 0 render calls on object without Renderable, got %d", logicScript.renderCalls)
	}
	if len(scene.GetRenderer().Commands2D) != 1 {
		t.Errorf("expected 1 2D command submitted by hero render hook, got %d", len(scene.GetRenderer().Commands2D))
	}

	// 4. Child cascade & FlagOverrideParent
	childHidden := NewGameObject("child_hidden", "ChildHidden")
	childHiddenR := NewRenderable("c1", "default")
	childHiddenScript := &mockRenderScriptComponent{}
	childHidden.AddComponent(childHiddenR)
	childHidden.AddComponent(childHiddenScript)
	hero.AddChild(childHidden)
	scene.Spawn(childHidden, hero)

	childOverride := NewGameObject("child_override", "ChildOverride")
	childOverrideR := NewRenderable("c2", "default")
	childOverrideR.Flags = FlagOverrideParent | FlagVisible
	childOverrideScript := &mockRenderScriptComponent{}
	childOverride.AddComponent(childOverrideR)
	childOverride.AddComponent(childOverrideScript)
	hero.AddChild(childOverride)
	scene.Spawn(childOverride, hero)

	// Hide hero: hero & childHidden must not render, childOverride MUST render
	heroR.SetVisible(false)
	scene.Draw3D()
	if heroScript.renderCalls != 1 {
		t.Errorf("expected hidden hero not to render again, calls=%d", heroScript.renderCalls)
	}
	if childHiddenScript.renderCalls != 0 {
		t.Errorf("expected childHidden to inherit hidden parent, calls=%d", childHiddenScript.renderCalls)
	}
	if childOverrideScript.renderCalls != 1 {
		t.Errorf("expected childOverride with FlagOverrideParent|FlagVisible to render, calls=%d", childOverrideScript.renderCalls)
	}

	// 5. Inactive GameObject must not render even with FlagOverrideParent|FlagVisible
	childOverride.Active = false
	scene.Draw3D()
	if childOverrideScript.renderCalls != 1 {
		t.Errorf("expected inactive childOverride not to render, calls=%d", childOverrideScript.renderCalls)
	}

	// 6. Re-enable hero and test Despawn + Unload gating
	heroR.SetVisible(true)
	scene.Despawn(childHidden, hero, nil)
	scene.Unload()
	scene.Draw3D() // Scene is unloaded -> must not render
	if heroScript.renderCalls != 1 {
		t.Errorf("expected unloaded scene Draw3D to no-op, calls=%d", heroScript.renderCalls)
	}
}

func TestBaseSceneDraw2DFlushesRendererQuads(t *testing.T) {
	api := newMockBatcherGLAPI()
	batcher := NewOrthoBatcher2D(32)
	batcher.SetGLAPI(api)
	if err := batcher.Init(); err != nil {
		t.Fatalf("batcher.Init failed: %v", err)
	}
	defer batcher.Destroy()

	scene := NewBaseScene("Flush2DScene")
	scene.SetViewportSize(1024, 768)
	scene.SetBatcher2D(batcher)

	obj := NewGameObject("hud_custom", "HUDCustom")
	obj.AddComponent(NewRenderable("quad", "ui"))
	obj.AddComponent(&mockRenderScriptComponent{
		onRenderFn: func(_ *GameObject, gfx *Renderer, _ string) {
			gfx.DrawRect(10, 10, 50, 20, White, 2)
			gfx.DrawRect(70, 10, 50, 20, Black, 1)
		},
	})
	scene.AddObject(obj)
	scene.Init()

	scene.Draw3D()
	scene.Draw2D()

	if batcher.Stats.DrawCalls != 1 || batcher.Stats.QuadsRendered != 2 {
		t.Fatalf("expected 1 batched draw call and 2 quads rendered, got %+v", batcher.Stats)
	}
}
