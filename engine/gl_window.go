package engine

import (
	"fmt"
	"runtime"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// GLWindowConfig configures a GLFW + OpenGL 3.3 Core window for hosting scenes
// and the Dear ImGui debug overlay.
type GLWindowConfig struct {
	Width     int
	Height    int
	Title     string
	Visible   bool
	Resizable bool
	VSync     bool
	ClearRGB  [4]float32
}

// DefaultGLWindowConfig returns standard OpenGL 3.3 Core window defaults.
func DefaultGLWindowConfig() GLWindowConfig {
	return GLWindowConfig{
		Width:     1280,
		Height:    720,
		Title:     "Project Engine - OpenGL 3 Runtime",
		Visible:   true,
		Resizable: true,
		VSync:     true,
		ClearRGB:  [4]float32{0.04, 0.055, 0.086, 1.0},
	}
}

// GLApp hosts a GLFW window, OpenGL 3.3 Core context, active Scene, and ImGuiOverlay.
type GLApp struct {
	Config  GLWindowConfig
	Window  *glfw.Window
	Overlay *ImGuiOverlay
	Scene   Scene
}

// NewGLApp creates an initialized GLApp wrapper ready to open a GLFW window.
func NewGLApp(cfg GLWindowConfig, scene Scene, overlay *ImGuiOverlay) *GLApp {
	if overlay == nil {
		overlay = NewImGuiOverlay()
	}
	return &GLApp{
		Config:  cfg,
		Overlay: overlay,
		Scene:   scene,
	}
}

// InitWindow initializes GLFW, creates an OpenGL 3.3 Core context, initializes GL
// bindings, and attaches the ImGuiOverlay. Call Close() when done.
func (a *GLApp) InitWindow() error {
	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		return fmt.Errorf("engine: glfw.Init failed: %w", err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	if a.Config.Visible {
		glfw.WindowHint(glfw.Visible, glfw.True)
	} else {
		glfw.WindowHint(glfw.Visible, glfw.False)
	}
	if a.Config.Resizable {
		glfw.WindowHint(glfw.Resizable, glfw.True)
	} else {
		glfw.WindowHint(glfw.Resizable, glfw.False)
	}

	w, err := glfw.CreateWindow(a.Config.Width, a.Config.Height, a.Config.Title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("engine: glfw.CreateWindow failed: %w", err)
	}
	a.Window = w
	w.MakeContextCurrent()

	if a.Config.VSync {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
	}

	if a.Overlay != nil {
		if err := a.Overlay.Init(w); err != nil {
			w.Destroy()
			glfw.Terminate()
			a.Window = nil
			return err
		}
	} else if err := gl.Init(); err != nil {
		w.Destroy()
		glfw.Terminate()
		a.Window = nil
		return fmt.Errorf("engine: gl.Init failed: %w", err)
	}

	w.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		gl.Viewport(0, 0, int32(width), int32(height))
	})

	return nil
}

// SceneMetadata is optionally implemented by scenes to report name and object count
// to the debug metrics overlay.
type SceneMetadata interface {
	GetName() string
	ObjectCount() int
}

// StepFrame executes a single update, 3D scene pass, ImGui overlay pass, buffer swap, and event poll.
func (a *GLApp) StepFrame(dt float32) {
	if a.Window == nil {
		return
	}

	fbW, fbH := a.Window.GetFramebufferSize()
	gl.Viewport(0, 0, int32(fbW), int32(fbH))
	gl.Enable(gl.DEPTH_TEST)
	gl.ClearColor(a.Config.ClearRGB[0], a.Config.ClearRGB[1], a.Config.ClearRGB[2], a.Config.ClearRGB[3])
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	if a.Scene != nil {
		a.Scene.Update(dt)
		a.Scene.Draw3D()
		a.Scene.Draw2D()
	}

	if a.Overlay != nil {
		if sc, ok := a.Scene.(SceneMetadata); ok {
			a.Overlay.SceneName = sc.GetName()
			a.Overlay.ObjectCount = sc.ObjectCount()
		}
		a.Overlay.NewFrame()
		a.Overlay.DrawDebugWindows()
		a.Overlay.Render()
	}

	a.Window.SwapBuffers()
	glfw.PollEvents()
}

// Run opens the GLFW window and drives the main loop until the window closes.
func (a *GLApp) Run() error {
	if err := a.InitWindow(); err != nil {
		return err
	}
	defer a.Close()

	if a.Scene != nil {
		a.Scene.Init()
		defer a.Scene.Unload()
	}

	lastTime := glfw.GetTime()
	for !a.Window.ShouldClose() {
		now := glfw.GetTime()
		dt := float32(now - lastTime)
		lastTime = now
		a.StepFrame(dt)
	}
	return nil
}

// Close shuts down the ImGui overlay, destroys the GLFW window, and terminates GLFW.
func (a *GLApp) Close() {
	if a.Overlay != nil {
		a.Overlay.Shutdown()
	}
	if a.Window != nil {
		a.Window.Destroy()
		a.Window = nil
	}
	glfw.Terminate()
}
