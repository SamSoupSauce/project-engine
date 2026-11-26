package interaction

import (
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

func TestControllerTapHoldGrabThrowAndHighlightClear(t *testing.T) {
	world, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig())
	if err != nil {
		t.Fatalf("NewPhysicsWorld failed: %v", err)
	}
	defer world.Destroy()

	scene := engine.NewBaseScene("interaction_test")

	// Static cabinet with primary LuaHook & secondary ContextVerbs.
	cabinet := engine.NewGameObject("cabinet", "Supply Cabinet")
	cabinet.Transform.Position = engine.NewVector3(0, 1, -3)
	cabinetRend := engine.NewRenderable("cabinet_mesh", "metal")
	cabinet.AddComponent(cabinetRend)
	cabinetRB := spatial.NewRigidBody("cabinet_body", world, engine.BodyStatic, spatial.BoxShapeDesc(0.6, 1.0, 0.5))
	cabinetRB.ObjectLayer = spatial.LayerInteractable
	cabinetRB.HasCustomLayer = true
	cabinet.AddComponent(cabinetRB)
	cabinetIC := spatial.NewInteractiveComponent("Supply Cabinet", false)
	cabinetIC.SetDefaultVerb(spatial.VerbOpenInventory, "Open", "on_open_cabinet")
	cabinetIC.AddContextVerb(spatial.VerbStashInventory, "Stash in Backpack", "on_stash_cabinet")
	cabinetIC.AddContextVerb(spatial.VerbDismantle, "Dismantle", "on_dismantle_cabinet")
	cabinet.AddComponent(cabinetIC)
	scene.AddObject(cabinet)

	// Dynamic loose prop positioned to the right (x=5, y=2, z=-3).
	crate := engine.NewGameObject("crate", "Small Crate")
	crate.Transform.Position = engine.NewVector3(5, 2, -3)
	crateRend := engine.NewRenderable("crate_mesh", "wood")
	crate.AddComponent(crateRend)
	crateRB := spatial.NewRigidBody("crate_body", world, engine.BodyDynamic, spatial.BoxShapeDesc(0.35, 0.35, 0.35))
	crateRB.ObjectLayer = spatial.LayerProp
	crateRB.HasCustomLayer = true
	crateRB.GravityFactor = 0 // Keep steady until grabbed/thrown for ray aim.
	crate.AddComponent(crateRB)
	crateIC := spatial.NewInteractiveComponent("Small Crate", true)
	crateIC.AddContextVerb(spatial.VerbStashInventory, "Stash in Backpack", "on_stash_crate")
	crate.AddComponent(crateIC)
	scene.AddObject(crate)

	world.OptimizeBroadPhase()

	var firedHooks []string
	ctrl := NewController(world, scene)
	ctrl.OnLuaVerb = func(target *engine.GameObject, opt spatial.InteractionOption) error {
		firedHooks = append(firedHooks, target.ID+":"+opt.LuaHook)
		return nil
	}

	camCabinet := CameraPose{
		Origin:    engine.NewVector3(0, 1, 0),
		Forward:   engine.NewVector3(0, 0, -1),
		ViewportW: 1280,
		ViewportH: 720,
	}

	// 1. Reticle hits cabinet -> FlagOutline applied.
	ctrl.Update(camCabinet, 0.016)
	if ctrl.State.HoveredEntity != cabinet || !cabinetRend.IsOutlined() {
		t.Fatalf("expected cabinet highlighted on hover")
	}

	// 2. Quick Tap (< 250ms) executes primary LuaHook.
	ctrl.OnInteractPressed()
	ctrl.Update(camCabinet, 0.100)
	res, err := ctrl.OnInteractReleased(camCabinet)
	if err != nil || !res.Executed {
		t.Fatalf("expected quick tap to execute DefaultVerb, got res=%+v err=%v", res, err)
	}
	if len(firedHooks) != 1 || firedHooks[0] != "cabinet:on_open_cabinet" {
		t.Fatalf("expected cabinet:on_open_cabinet, got %v", firedHooks)
	}

	// 3. Long Press (> 250ms) opens context menu without firing primary tap.
	ctrl.OnInteractPressed()
	menuRes := ctrl.Update(camCabinet, 0.300)
	if !menuRes.MenuOpened || !ctrl.State.IsMenuOpen || len(ctrl.State.MenuOptions) != 2 {
		t.Fatalf("expected context menu open with 2 options, got res=%+v state=%+v", menuRes, ctrl.State)
	}
	ctrl.SetHoveredMenuOption(0)
	relRes, err := ctrl.OnInteractReleased(camCabinet)
	if err != nil || !relRes.Executed || ctrl.State.IsMenuOpen {
		t.Fatalf("expected selected menu option executed & menu closed, got %+v err=%v open=%v", relRes, err, ctrl.State.IsMenuOpen)
	}
	if len(firedHooks) != 2 || firedHooks[1] != "cabinet:on_stash_cabinet" {
		t.Fatalf("expected cabinet:on_stash_cabinet, got %v", firedHooks)
	}

	// 4. Moving crosshair away immediately clears highlight & dismisses open menu.
	ctrl.OnInteractPressed()
	ctrl.Update(camCabinet, 0.300)
	if !ctrl.State.IsMenuOpen {
		t.Fatalf("expected menu open before look-away")
	}
	camAway := CameraPose{
		Origin:  engine.NewVector3(0, 1, 0),
		Forward: engine.NewVector3(0, 1, 0), // Look straight up at sky.
	}
	ctrl.Update(camAway, 0.016)
	if ctrl.State.IsMenuOpen || ctrl.State.HoveredEntity != nil || cabinetRend.IsOutlined() {
		t.Fatalf("expected highlight and menu cleared when looking away: open=%v hovered=%v outlined=%v",
			ctrl.State.IsMenuOpen, ctrl.State.HoveredEntity, cabinetRend.IsOutlined())
	}
	_, _ = ctrl.OnInteractReleased(camAway)

	// 5. Dynamic prop: quick tap grabs, second quick tap throws along camera forward.
	camCrate := CameraPose{
		Origin:  engine.NewVector3(5, 2, 0),
		Forward: engine.NewVector3(0, 0, -1),
	}
	ctrl.Update(camCrate, 0.016)
	if ctrl.State.HoveredEntity != crate || !crateRend.IsOutlined() {
		t.Fatalf("expected crate highlighted on reticle hit")
	}
	ctrl.OnInteractPressed()
	ctrl.Update(camCrate, 0.050)
	grabRes, err := ctrl.OnInteractReleased(camCrate)
	if err != nil || !grabRes.Executed || grabRes.GrabbedProp != crate || ctrl.State.ActiveHeldProp != crate {
		t.Fatalf("expected quick tap grab on dynamic prop, got %+v err=%v", grabRes, err)
	}

	// While carrying crate, holding interact (> 250ms) pops its held context menu.
	ctrl.OnInteractPressed()
	heldMenuRes := ctrl.Update(camCrate, 0.300)
	if !heldMenuRes.MenuOpened || !ctrl.State.IsMenuOpen || ctrl.State.MenuTarget != crate {
		t.Fatalf("expected held prop context menu open on hold, got res=%+v state=%+v", heldMenuRes, ctrl.State)
	}
	ctrl.SetHoveredMenuOption(0)
	stashRes, err := ctrl.OnInteractReleased(camCrate)
	if err != nil || !stashRes.Executed || ctrl.State.IsMenuOpen {
		t.Fatalf("expected held prop context option executed, got res=%+v err=%v", stashRes, err)
	}

	// Second quick tap throws the held prop forward.
	ctrl.OnInteractPressed()
	ctrl.Update(camCrate, 0.050)
	throwRes, err := ctrl.OnInteractReleased(camCrate)
	if err != nil || !throwRes.Executed || throwRes.ThrownProp != crate || ctrl.State.ActiveHeldProp != nil {
		t.Fatalf("expected second quick tap throw on held prop, got %+v err=%v", throwRes, err)
	}
}

func TestImGuiContextMenuHeadlessRenderAndRadialLayout(t *testing.T) {
	slots := ComputeRadialLayout(640, 360, 100, []spatial.InteractionOption{
		{Verb: spatial.VerbOpenInventory, Label: "Inspect"},
		{Verb: spatial.VerbStashInventory, Label: "Stash"},
		{Verb: spatial.VerbDismantle, Label: "Dismantle"},
	})
	if len(slots) != 3 {
		t.Fatalf("expected 3 radial slots, got %d", len(slots))
	}

	overlay := engine.NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()

	ctrl := NewController(nil, nil)
	target := engine.NewGameObject("box", "Toolbox")
	ic := spatial.NewInteractiveComponent("Toolbox", true)
	ic.AddContextVerb(spatial.VerbStashInventory, "Stash in Backpack", "on_stash")
	target.AddComponent(ic)
	ctrl.SetHoverTargetDirectly(target)

	overlay.DrawUI = func() {
		ctrl.DrawImGui(1280, 720)
	}

	// Frame 1: draw hover prompt.
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	// Frame 2: open context menu and draw ImGui menu.
	ctrl.openMenu(target, ic.ContextVerbs)
	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()
}
