package engine

import (
	"reflect"
	"runtime"
	"testing"
)

type stubGLScene struct {
	*BaseScene
	draw3DCalls int
	draw2DCalls int
}

func newStubGLScene() *stubGLScene {
	return &stubGLScene{
		BaseScene: NewBaseScene("StubGLScene"),
	}
}

func (s *stubGLScene) Draw3D() { s.draw3DCalls++ }
func (s *stubGLScene) Draw2D() { s.draw2DCalls++ }

func TestGLContext_ComponentLifecycleWithoutOpen(t *testing.T) {
	cfg := DefaultGLWindowConfig()
	cfg.Width = 800
	cfg.Height = 600
	cfg.Visible = false

	ctx := NewGLContext(cfg)
	if ctx.AutoOpen {
		t.Fatalf("expected AutoOpen to default to false")
	}

	obj := NewGameObject("ctx_obj", "WindowContext")
	obj.AddComponent(ctx)

	if ctx.Owner != obj {
		t.Fatalf("expected Owner to be registered via Component.Init")
	}
	if ctx.Ready {
		t.Fatalf("expected context not to open when AutoOpen is false")
	}

	found := obj.GetComponent(reflect.TypeOf((*GLContext)(nil)))
	if found != ctx {
		t.Fatalf("expected GetComponent to return attached *GLContext")
	}

	obj.Update(0.02)
	obj.Update(0.02)
	if ctx.FrameCount != 2 {
		t.Fatalf("expected FrameCount=2 after two updates, got %d", ctx.FrameCount)
	}
	if ctx.DeltaTime != 0.02 {
		t.Fatalf("expected DeltaTime=0.02, got %v", ctx.DeltaTime)
	}
	if ctx.FPS <= 0 {
		t.Fatalf("expected positive smoothed FPS, got %v", ctx.FPS)
	}

	obj.Destroy()
	if ctx.Owner != nil {
		t.Fatalf("expected Destroy to clear Owner")
	}
	if !ctx.ShouldClose() {
		t.Fatalf("expected closed/unopened context to report ShouldClose()=true")
	}
}

func TestGLContext_LiveWindowAndGLApp(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cfg := DefaultGLWindowConfig()
	cfg.Width = 320
	cfg.Height = 240
	cfg.Title = "GLContext Hidden Test"
	cfg.Visible = false
	cfg.VSync = false

	ctxObj := NewGameObject("live_ctx", "LiveGLContext")
	glCtx := NewGLContext(cfg)
	glCtx.AutoOpen = true
	ctxObj.AddComponent(glCtx)

	if glCtx.InitErr != nil {
		t.Skipf("skipping live GLFW/OpenGL test without display/driver: %v", glCtx.InitErr)
	}
	defer ctxObj.Destroy()

	if !glCtx.Ready || glCtx.Window == nil {
		t.Fatalf("expected GLContext to be Ready with a non-nil GLFW window")
	}
	if glCtx.FramebufferW <= 0 || glCtx.FramebufferH <= 0 {
		t.Fatalf("expected positive framebuffer size, got %dx%d", glCtx.FramebufferW, glCtx.FramebufferH)
	}

	glCtx.LockCursor()
	if !glCtx.CursorLocked {
		t.Fatalf("expected CursorLocked=true after LockCursor")
	}
	glCtx.UnlockCursor()
	if glCtx.CursorLocked {
		t.Fatalf("expected CursorLocked=false after UnlockCursor")
	}

	glCtx.BeginFrame()
	glCtx.EndFrame()

	scene := newStubGLScene()
	hudObj := NewGameObject("hud_test", "HudQuad")
	hudObj.AddComponent(NewRenderable2D(0, 32, 16))
	scene.AddObject(hudObj)

	overlay := NewImGuiOverlay()
	app := NewGLApp(cfg, scene, overlay)
	app.Context = glCtx
	if err := app.InitWindow(); err != nil {
		t.Fatalf("GLApp.InitWindow failed with existing GLContext: %v", err)
	}
	defer app.Close()

	scene.Init()
	app.StepFrame(0.016)
	scene.Unload()

	if scene.draw3DCalls != 1 || scene.draw2DCalls != 1 {
		t.Fatalf("expected 1 Draw3D and 1 Draw2D call, got 3D=%d 2D=%d", scene.draw3DCalls, scene.draw2DCalls)
	}
	if glCtx.FrameCount == 0 {
		t.Fatalf("expected GLContext.FrameCount to advance during GLApp.StepFrame")
	}
}
