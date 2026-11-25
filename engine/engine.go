package engine

import (
	"github.com/go-gl/glfw/v3.3/glfw"
)

// Scene defines the contract for scenes hosted within the engine.
type Scene interface {
	Init()
	Update(dt float32)
	Draw3D()
	Draw2D()
	GetCamera() Camera3D
	Unload()
}

// Config specifies display and initialization properties for the 3D window.
type Config struct {
	Width      int32
	Height     int32
	Title      string
	TargetFPS  int32
	ClearColor Color
	EnableMSAA bool
	Resizable  bool
}

// DefaultConfig provides sensible, production defaults.
func DefaultConfig() Config {
	return Config{
		Width:      1280,
		Height:     720,
		Title:      "Project Engine - 3D Runtime",
		TargetFPS:  60,
		ClearColor: NewColor(10, 14, 22, 255),
		EnableMSAA: true,
		Resizable:  true,
	}
}

// Engine coordinates window lifecycle, timing, graphics context, and scene execution.
type Engine struct {
	Config      Config
	ActiveScene Scene
	Overlay     *ImGuiOverlay
	App         *GLApp
	IsRunning   bool
}

// NewEngine initializes an Engine instance.
func NewEngine(cfg Config) *Engine {
	return &Engine{
		Config:  cfg,
		Overlay: NewImGuiOverlay(),
	}
}

// SetScene replaces the active scene, transitioning between Unload and Init.
func (e *Engine) SetScene(s Scene) {
	if e.ActiveScene != nil {
		e.ActiveScene.Unload()
	}
	e.ActiveScene = s
	if e.App != nil {
		e.App.Scene = s
	}
	if e.ActiveScene != nil && e.IsRunning {
		e.ActiveScene.Init()
	}
}

// Run boots the GLFW + OpenGL 3 window and drives the main rendering, scene, and ImGui overlay loop.
func (e *Engine) Run() error {
	winCfg := GLWindowConfig{
		Width:     int(e.Config.Width),
		Height:    int(e.Config.Height),
		Title:     e.Config.Title,
		Visible:   true,
		Resizable: e.Config.Resizable,
		VSync:     e.Config.TargetFPS > 0,
		ClearRGB: [4]float32{
			float32(e.Config.ClearColor.R) / 255.0,
			float32(e.Config.ClearColor.G) / 255.0,
			float32(e.Config.ClearColor.B) / 255.0,
			float32(e.Config.ClearColor.A) / 255.0,
		},
	}

	if e.Overlay == nil {
		e.Overlay = NewImGuiOverlay()
	}

	e.App = NewGLApp(winCfg, e.ActiveScene, e.Overlay)
	if err := e.App.InitWindow(); err != nil {
		return err
	}
	defer func() {
		e.IsRunning = false
		if e.ActiveScene != nil {
			e.ActiveScene.Unload()
		}
		e.App.Close()
		e.App = nil
	}()

	e.IsRunning = true
	if e.ActiveScene != nil {
		e.ActiveScene.Init()
	}

	lastTime := glfw.GetTime()
	for !e.App.Window.ShouldClose() {
		now := glfw.GetTime()
		dt := float32(now - lastTime)
		lastTime = now
		e.App.Scene = e.ActiveScene
		e.App.StepFrame(dt)
	}
	return nil
}
