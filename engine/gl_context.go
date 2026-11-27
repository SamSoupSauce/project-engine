package engine

import (
	"fmt"
	"runtime"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// GLWindowConfig configures a GLFW + OpenGL 3.3 Core window and rendering context.
type GLWindowConfig struct {
	Width      int
	Height     int
	Title      string
	Visible    bool
	Resizable  bool
	VSync      bool
	EnableMSAA bool
	Samples    int
	Fullscreen bool
	ClearRGB   [4]float32
}

// DefaultGLWindowConfig returns standard OpenGL 3.3 Core window defaults.
func DefaultGLWindowConfig() GLWindowConfig {
	return GLWindowConfig{
		Width:      1280,
		Height:     720,
		Title:      "Project Engine - OpenGL 3 Runtime",
		Visible:    true,
		Resizable:  true,
		VSync:      true,
		EnableMSAA: true,
		Samples:    4,
		ClearRGB:   [4]float32{0.04, 0.055, 0.086, 1.0},
	}
}

// GLContext is an engine.Component that owns and manages a GLFW window,
// OpenGL 3.3 Core context, MSAA/VSync hints, fullscreen toggling, framebuffer
// resize tracking, and cursor capture/confinement.
type GLContext struct {
	Config   GLWindowConfig
	AutoOpen bool

	Owner        *GameObject
	Window       *glfw.Window
	Ready        bool
	InitErr      error
	IsFullscreen bool
	CursorLocked bool

	Width        int
	Height       int
	FramebufferW int
	FramebufferH int

	DeltaTime  float32
	FPS        float32
	FrameCount uint64

	OnResize func(fbWidth, fbHeight int)

	windowedX  int
	windowedY  int
	windowedW  int
	windowedH  int
	lastTime   float64
	wasFocused bool
	ownsGLFW   bool
}

// NewGLContext creates a GLContext component with the provided window configuration.
// AutoOpen defaults to false so callers can open explicitly via Open() or opt into
// lazy open on AddComponent by setting AutoOpen = true.
func NewGLContext(cfg GLWindowConfig) *GLContext {
	if cfg.Width <= 0 {
		cfg.Width = 1280
	}
	if cfg.Height <= 0 {
		cfg.Height = 720
	}
	if cfg.Samples <= 0 && cfg.EnableMSAA {
		cfg.Samples = 4
	}
	return &GLContext{
		Config:     cfg,
		Width:      cfg.Width,
		Height:     cfg.Height,
		wasFocused: true,
	}
}

// Init registers the owning GameObject and opens the GLFW/OpenGL context if AutoOpen is enabled.
func (c *GLContext) Init(owner *GameObject) {
	c.Owner = owner
	if c.AutoOpen && !c.Ready {
		c.InitErr = c.Open()
	}
}

// Update advances per-frame timing and releases cursor capture if the window loses focus.
func (c *GLContext) Update(dt float32) {
	c.DeltaTime = dt
	if dt > 0 {
		instFPS := 1.0 / dt
		if c.FPS == 0 {
			c.FPS = instFPS
		} else {
			c.FPS = c.FPS*0.9 + instFPS*0.1
		}
	}
	c.FrameCount++

	if !c.Ready || c.Window == nil {
		return
	}

	focused := c.Window.GetAttrib(glfw.Focused) == glfw.True
	if c.wasFocused && !focused && c.CursorLocked {
		c.UnlockCursor()
	}
	c.wasFocused = focused
}

// Destroy releases cursor capture, closes the GLFW window, and clears Owner.
func (c *GLContext) Destroy() {
	c.Close()
	c.Owner = nil
}

// Open initializes GLFW on the calling OS thread, creates an OpenGL 3.3 Core window,
// makes its context current, and initializes OpenGL bindings.
func (c *GLContext) Open() error {
	if c.Ready && c.Window != nil {
		return nil
	}

	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		return fmt.Errorf("engine: glfw.Init failed: %w", err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	if c.Config.Visible {
		glfw.WindowHint(glfw.Visible, glfw.True)
	} else {
		glfw.WindowHint(glfw.Visible, glfw.False)
	}
	if c.Config.Resizable {
		glfw.WindowHint(glfw.Resizable, glfw.True)
	} else {
		glfw.WindowHint(glfw.Resizable, glfw.False)
	}
	if c.Config.EnableMSAA && c.Config.Samples > 0 {
		glfw.WindowHint(glfw.Samples, c.Config.Samples)
	}

	var monitor *glfw.Monitor
	w, h := c.Config.Width, c.Config.Height
	if c.Config.Fullscreen {
		monitor = glfw.GetPrimaryMonitor()
		if monitor != nil {
			mode := monitor.GetVideoMode()
			if mode != nil {
				w, h = mode.Width, mode.Height
			}
		}
	}

	win, err := glfw.CreateWindow(w, h, c.Config.Title, monitor, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("engine: glfw.CreateWindow failed: %w", err)
	}

	c.Window = win
	c.ownsGLFW = true
	c.IsFullscreen = monitor != nil
	win.MakeContextCurrent()

	if c.Config.VSync {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
	}

	if err := gl.Init(); err != nil {
		win.Destroy()
		glfw.Terminate()
		c.Window = nil
		c.ownsGLFW = false
		return fmt.Errorf("engine: gl.Init failed: %w", err)
	}

	if c.Config.EnableMSAA {
		gl.Enable(gl.MULTISAMPLE)
	}
	gl.Enable(gl.DEPTH_TEST)

	c.Width, c.Height = win.GetSize()
	c.FramebufferW, c.FramebufferH = win.GetFramebufferSize()
	c.windowedX, c.windowedY = win.GetPos()
	c.windowedW, c.windowedH = c.Config.Width, c.Config.Height
	c.lastTime = glfw.GetTime()
	c.wasFocused = true
	c.Ready = true

	win.SetFramebufferSizeCallback(func(_ *glfw.Window, fbW, fbH int) {
		c.FramebufferW = fbW
		c.FramebufferH = fbH
		c.Width, c.Height = win.GetSize()
		gl.Viewport(0, 0, int32(fbW), int32(fbH))
		if c.OnResize != nil {
			c.OnResize(fbW, fbH)
		}
	})

	return nil
}

// Close releases cursor capture, destroys the GLFW window, and terminates GLFW.
func (c *GLContext) Close() {
	if c.CursorLocked {
		c.UnlockCursor()
	}
	if c.Window != nil {
		c.Window.Destroy()
		c.Window = nil
	}
	if c.ownsGLFW {
		glfw.Terminate()
		c.ownsGLFW = false
	}
	c.Ready = false
}

// MakeCurrent binds this context's OpenGL context to the calling OS thread.
func (c *GLContext) MakeCurrent() {
	if c.Window != nil {
		c.Window.MakeContextCurrent()
	}
}

// ShouldClose reports whether the window close flag has been set.
func (c *GLContext) ShouldClose() bool {
	if c.Window == nil {
		return true
	}
	return c.Window.ShouldClose()
}

// StepDelta computes elapsed seconds since the previous frame using GLFW's monotonic clock.
func (c *GLContext) StepDelta() float32 {
	now := glfw.GetTime()
	dt := float32(now - c.lastTime)
	c.lastTime = now
	if dt < 0 {
		dt = 0
	}
	c.Update(dt)
	return dt
}

// BeginFrame sets the OpenGL viewport, enables depth testing, and clears color/depth buffers.
func (c *GLContext) BeginFrame() {
	if c.Window == nil {
		return
	}
	c.Width, c.Height = c.Window.GetSize()
	c.FramebufferW, c.FramebufferH = c.Window.GetFramebufferSize()
	gl.Viewport(0, 0, int32(c.FramebufferW), int32(c.FramebufferH))
	gl.Enable(gl.DEPTH_TEST)
	gl.ClearColor(c.Config.ClearRGB[0], c.Config.ClearRGB[1], c.Config.ClearRGB[2], c.Config.ClearRGB[3])
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
}

// EndFrame swaps the front and back buffers and polls GLFW window events.
func (c *GLContext) EndFrame() {
	if c.Window == nil {
		return
	}
	c.Window.SwapBuffers()
	glfw.PollEvents()
}

// ToggleFullscreen switches between windowed mode and borderless/primary-monitor fullscreen.
func (c *GLContext) ToggleFullscreen() {
	if !c.Ready || c.Window == nil {
		return
	}
	if !c.IsFullscreen {
		c.windowedX, c.windowedY = c.Window.GetPos()
		c.windowedW, c.windowedH = c.Window.GetSize()
		monitor := glfw.GetPrimaryMonitor()
		if monitor == nil {
			return
		}
		mode := monitor.GetVideoMode()
		if mode == nil {
			return
		}
		c.Window.SetMonitor(monitor, 0, 0, mode.Width, mode.Height, mode.RefreshRate)
		c.IsFullscreen = true
	} else {
		w, h := c.windowedW, c.windowedH
		if w <= 0 {
			w = 1280
		}
		if h <= 0 {
			h = 720
		}
		c.Window.SetMonitor(nil, c.windowedX, c.windowedY, w, h, 0)
		c.IsFullscreen = false
	}
	c.Width, c.Height = c.Window.GetSize()
	c.FramebufferW, c.FramebufferH = c.Window.GetFramebufferSize()
}

// LockCursor captures and hides the OS cursor for first-person mouselook, enabling raw mouse motion when supported.
func (c *GLContext) LockCursor() {
	c.CursorLocked = true
	if !c.Ready || c.Window == nil {
		return
	}
	c.Window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
	if glfw.RawMouseMotionSupported() {
		c.Window.SetInputMode(glfw.RawMouseMotion, glfw.True)
	}
}

// UnlockCursor restores normal OS cursor visibility and movement for UI/console interaction.
func (c *GLContext) UnlockCursor() {
	c.CursorLocked = false
	if !c.Ready || c.Window == nil {
		return
	}
	if glfw.RawMouseMotionSupported() {
		c.Window.SetInputMode(glfw.RawMouseMotion, glfw.False)
	}
	c.Window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
}
