package engine

import (
	"github.com/go-gl/glfw/v3.3/glfw"
)

// GLApp hosts a GLContext component, OpenGL 3.3 Core window, active Scene,
// 2D orthographic batcher, and ImGuiOverlay.
type GLApp struct {
	Config    GLWindowConfig
	Context   *GLContext
	Window    *glfw.Window
	Batcher2D *OrthoBatcher2D
	Overlay   *ImGuiOverlay
	Scene     Scene
}

// NewGLApp creates an initialized GLApp wrapper ready to open a GLFW window.
func NewGLApp(cfg GLWindowConfig, scene Scene, overlay *ImGuiOverlay) *GLApp {
	if overlay == nil {
		overlay = NewImGuiOverlay()
	}
	ctx := NewGLContext(cfg)
	return &GLApp{
		Config:    cfg,
		Context:   ctx,
		Batcher2D: NewOrthoBatcher2D(DefaultBatcher2DCapacity),
		Overlay:   overlay,
		Scene:     scene,
	}
}

// InitWindow initializes GLFW via GLContext, creates an OpenGL 3.3 Core context,
// initializes GL bindings, Batcher2D, and attaches the ImGuiOverlay. Call Close() when done.
func (a *GLApp) InitWindow() error {
	if a.Context == nil {
		a.Context = NewGLContext(a.Config)
	} else {
		a.Context.Config = a.Config
	}

	if err := a.Context.Open(); err != nil {
		return err
	}
	a.Window = a.Context.Window

	if a.Overlay != nil {
		if err := a.Overlay.Init(a.Window); err != nil {
			a.Context.Close()
			a.Window = nil
			return err
		}
	}

	if a.Batcher2D != nil {
		if err := a.Batcher2D.Init(); err != nil {
			if a.Overlay != nil {
				a.Overlay.Shutdown()
			}
			a.Context.Close()
			a.Window = nil
			return err
		}
	}

	return nil
}

// SceneMetadata is optionally implemented by scenes to report name and object count
// to the debug metrics overlay.
type SceneMetadata interface {
	GetName() string
	ObjectCount() int
}

// StepFrame executes update, 3D scene pass, 2D orthographic HUD pass, ImGui overlay pass,
// buffer swap, and event polling.
func (a *GLApp) StepFrame(dt float32) {
	if a.Context == nil || a.Context.Window == nil {
		return
	}

	a.Context.BeginFrame()
	a.Context.Update(dt)

	if a.Scene != nil {
		a.Scene.Update(dt)
		a.Scene.Draw3D()
		a.Scene.Draw2D()
	}

	// Orthographic 2D pass: runs after the 3D scene pass and before the debug ImGui pass.
	if a.Batcher2D != nil && a.Scene != nil {
		if sc, ok := a.Scene.(SceneController); ok {
			cmds := Extract2DFromHierarchy(sc.GetRoot(), float32(a.Context.Width), float32(a.Context.Height))
			if len(cmds) > 0 {
				a.Batcher2D.Begin(a.Context.Width, a.Context.Height)
				a.Batcher2D.DrawCommands(cmds)
				a.Batcher2D.End()
			}
		}
	}

	if a.Overlay != nil {
		if sc, ok := a.Scene.(SceneMetadata); ok {
			a.Overlay.SceneName = sc.GetName()
			a.Overlay.ObjectCount = sc.ObjectCount()
		}
		a.Overlay.MetricsFPS = a.Context.FPS
		a.Overlay.NewFrame()
		a.Overlay.DrawDebugWindows()
		a.Overlay.Render()
	}

	a.Context.EndFrame()
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

	for !a.Context.ShouldClose() {
		dt := a.Context.StepDelta()
		a.StepFrame(dt)
	}
	return nil
}

// Close shuts down the 2D batcher, ImGui overlay, and GLContext.
func (a *GLApp) Close() {
	if a.Batcher2D != nil {
		a.Batcher2D.Destroy()
	}
	if a.Overlay != nil {
		a.Overlay.Shutdown()
	}
	if a.Context != nil {
		a.Context.Close()
	}
	a.Window = nil
}
