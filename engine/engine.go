package engine

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Scene defines the contract for scenes hosted within the engine.
type Scene interface {
	Init()
	Update(dt float32)
	Draw3D()
	Draw2D()
	GetCamera() rl.Camera3D
	Unload()
}

// Config specifies display and initialization properties for the 3D window.
type Config struct {
	Width      int32
	Height     int32
	Title      string
	TargetFPS  int32
	ClearColor rl.Color
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
		ClearColor: rl.NewColor(10, 14, 22, 255),
		EnableMSAA: true,
		Resizable:  true,
	}
}

// Engine coordinates window lifecycle, timing, graphics context, and scene execution.
type Engine struct {
	Config      Config
	ActiveScene Scene
	IsRunning   bool
}

// NewEngine initializes an Engine instance.
func NewEngine(cfg Config) *Engine {
	return &Engine{
		Config: cfg,
	}
}

// SetScene replaces the active scene, transitioning between Unload and Init.
func (e *Engine) SetScene(s Scene) {
	if e.ActiveScene != nil {
		e.ActiveScene.Unload()
	}
	e.ActiveScene = s
	if e.ActiveScene != nil && e.IsRunning {
		e.ActiveScene.Init()
	}
}

// Run boots the window and drives the main rendering and simulation loop.
func (e *Engine) Run() {
	var flags uint32 = rl.FlagVsyncHint
	if e.Config.EnableMSAA {
		flags |= rl.FlagMsaa4xHint
	}
	if e.Config.Resizable {
		flags |= rl.FlagWindowResizable
	}

	rl.SetConfigFlags(flags)
	rl.InitWindow(e.Config.Width, e.Config.Height, e.Config.Title)
	defer rl.CloseWindow()

	rl.SetTargetFPS(e.Config.TargetFPS)
	e.IsRunning = true

	if e.ActiveScene != nil {
		e.ActiveScene.Init()
	}

	for !rl.WindowShouldClose() {
		if rl.IsKeyPressed(rl.KeyF) {
			rl.ToggleFullscreen()
		}

		dt := rl.GetFrameTime()

		if e.ActiveScene != nil {
			e.ActiveScene.Update(dt)
		}

		rl.BeginDrawing()
		rl.ClearBackground(e.Config.ClearColor)

		if e.ActiveScene != nil {
			cam := e.ActiveScene.GetCamera()
			rl.BeginMode3D(cam)
			e.ActiveScene.Draw3D()
			rl.EndMode3D()

			e.ActiveScene.Draw2D()
		}

		rl.EndDrawing()
	}

	e.IsRunning = false
	if e.ActiveScene != nil {
		e.ActiveScene.Unload()
	}
}
