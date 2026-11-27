package engine

import (
	"errors"
	"reflect"
	"runtime"
	"strings"
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

func TestGLContextOpenReturnsErrorOnGLFWPlatformPanic(t *testing.T) {
	origDisplay := hasDisplayServer
	hasDisplayServer = func() bool { return true }
	defer func() { hasDisplayServer = origDisplay }()

	original := glfwInit
	glfwInit = func() error {
		panic("PlatformError: X11: The DISPLAY environment variable is missing")
	}
	defer func() { glfwInit = original }()

	ctx := NewGLContext(DefaultGLWindowConfig())
	err := ctx.Open()
	if err == nil {
		t.Fatal("expected Open to return an error instead of propagating a GLFW panic")
	}
	if !strings.Contains(err.Error(), "DISPLAY") {
		t.Fatalf("expected error to carry the GLFW platform failure, got %v", err)
	}
	if ctx.Ready || ctx.Window != nil {
		t.Fatalf("expected failed Open to leave context unready, got Ready=%v Window=%v", ctx.Ready, ctx.Window)
	}
}

func TestGLContextOpenSkipsGLFWInitWithoutDisplay(t *testing.T) {
	origDisplay := hasDisplayServer
	hasDisplayServer = func() bool { return false }
	defer func() { hasDisplayServer = origDisplay }()

	origInit := glfwInit
	glfwInitCalled := false
	glfwInit = func() error {
		glfwInitCalled = true
		return nil
	}
	defer func() { glfwInit = origInit }()

	ctx := NewGLContext(DefaultGLWindowConfig())
	err := ctx.Open()
	if !errors.Is(err, ErrNoDisplayServer) {
		t.Fatalf("expected ErrNoDisplayServer, got %v", err)
	}
	if glfwInitCalled {
		t.Fatal("expected glfwInit not to be called when no display server is available")
	}
	if ctx.Ready || ctx.Window != nil {
		t.Fatalf("expected failed Open to leave context unready, got Ready=%v Window=%v", ctx.Ready, ctx.Window)
	}
}

func TestHasDisplayServerForEnv(t *testing.T) {
	makeEnv := func(vals map[string]string) func(string) string {
		return func(k string) string { return vals[k] }
	}
	if hasDisplayServerForEnv("linux", makeEnv(nil)) {
		t.Fatal("expected linux with empty DISPLAY and WAYLAND_DISPLAY to report no display server")
	}
	if !hasDisplayServerForEnv("linux", makeEnv(map[string]string{"DISPLAY": ":0"})) {
		t.Fatal("expected linux with DISPLAY=:0 to report display server available")
	}
	if !hasDisplayServerForEnv("linux", makeEnv(map[string]string{"WAYLAND_DISPLAY": "wayland-0"})) {
		t.Fatal("expected linux with WAYLAND_DISPLAY to report display server available")
	}
	if !hasDisplayServerForEnv("windows", makeEnv(nil)) {
		t.Fatal("expected windows to report display server available")
	}
	if !hasDisplayServerForEnv("darwin", makeEnv(nil)) {
		t.Fatal("expected darwin to report display server available")
	}
}

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
