package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/pkg/luabridge"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type demoServerOptions struct {
	Version      bool
	Root         string
	RootExplicit bool
	Config       string
	InitDirs     bool
	Headless     bool
	Serve        bool
	Tunnel       bool
	Address      string
	Token        string
	TileMap      string
}

func parseDemoServerFlags(args []string) (demoServerOptions, error) {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	var opts demoServerOptions
	fs.BoolVar(&opts.Version, "version", false, "Print engine version and exit")
	fs.StringVar(&opts.Root, "root", ".", "Game/script/config/data root directory containing assets/, scripts/, prefabs/, config/")
	fs.StringVar(&opts.Config, "config", "", "Optional path to engine.json configuration file")
	fs.BoolVar(&opts.InitDirs, "init-dirs", false, "Initialize missing standard directory structure under --root on startup")
	fs.BoolVar(&opts.Headless, "headless", false, "Run without a GLFW window or OpenGL context (scene ticks and --serve still run)")
	fs.BoolVar(&opts.Serve, "serve", false, "Start dual-transport HTTP JSON-RPC + WebSocket agent bridge server")
	fs.BoolVar(&opts.Tunnel, "tunnel", false, "Auto-locate/download cloudflared and open a quick tunnel to --address (implies --serve)")
	fs.StringVar(&opts.Address, "address", luabridge.DefaultAgentServerAddress, "Bind address for agent bridge server")
	fs.StringVar(&opts.Token, "token", "", "Required authentication token when --serve or --tunnel is enabled (may be empty)")
	fs.StringVar(&opts.TileMap, "tilemap", "", "Override the tile map path (relative to workspace root or absolute)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.Tunnel {
		opts.Serve = true
	}
	tokenProvided := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "token":
			tokenProvided = true
		case "root":
			opts.RootExplicit = true
		}
	})
	if opts.Serve && !tokenProvided {
		return opts, fmt.Errorf("--serve / --tunnel requires --token to be explicitly provided (e.g. --token=\"\" or --token=secret)")
	}
	return opts, nil
}

// defaultHeadlessTickInterval matches BaseScene's default fixed simulation tick.
const defaultHeadlessTickInterval = 16 * time.Millisecond

// runHeadlessLoop drives scene ticks without a GLFW window or OpenGL context.
// AgentServer commands are drained by the scene's pre-tick hook, so ticking is
// sufficient to serve RPC/WebSocket traffic in headless mode.
func runHeadlessLoop(ctx context.Context, scene engine.Scene, interval time.Duration) {
	if scene == nil || ctx == nil {
		return
	}
	if interval <= 0 {
		interval = defaultHeadlessTickInterval
	}
	scene.Init()
	defer scene.Unload()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scene.Update(float32(interval.Seconds()))
		}
	}
}

func main() {
	opts, err := parseDemoServerFlags(os.Args[1:])
	if err != nil {
		log.Fatalf("flag error: %v", err)
	}
	if opts.Version {
		fmt.Printf("project-engine %s\n", engine.Version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runDemo(ctx, opts); err != nil {
		log.Fatalf("demo runtime error: %v", err)
	}
}

// runDemo owns the full demo lifecycle. Headless mode skips GLFW/OpenGL setup
// and drives the scene's fixed ticks directly, leaving the agent bridge usable
// on machines without a display.
func runDemo(ctx context.Context, opts demoServerOptions) error {
	wsCfg, paths, err := engine.LoadWorkspaceConfig(opts.Root, opts.RootExplicit, opts.Config)
	if err != nil {
		return fmt.Errorf("workspace config error: %w", err)
	}
	if opts.InitDirs {
		if created, initErr := engine.InitWorkspaceDirs(paths); initErr != nil {
			return fmt.Errorf("failed to initialize workspace directories: %w", initErr)
		} else if len(created) > 0 {
			log.Printf("[Workspace] Initialized %d directory(s) under %s", len(created), paths.RootDir)
		}
	}
	if err := engine.VerifyWorkspaceStructure(paths); err != nil {
		return err
	}

	cfg := engine.DefaultGLWindowConfig()
	cfg.Title = wsCfg.WindowTitle
	if cfg.Title == "" {
		cfg.Title = "Project Engine - Kitchen Sink Demo"
	}

	// Headless mode skips GLFW/OpenGL entirely; tests already construct the full
	// scene with a nil GLContext, so the same code path is safe here.
	var glCtx *engine.GLContext
	var ctxObj *engine.GameObject
	if !opts.Headless {
		// Instantiate GLContext as an engine.Component attached to a GameObject.
		ctxObj = engine.NewGameObject("gl_context", "GLContextOwner")
		glCtx = engine.NewGLContext(cfg)
		glCtx.AutoOpen = true
		ctxObj.AddComponent(glCtx)
		if glCtx.InitErr != nil {
			if errors.Is(glCtx.InitErr, engine.ErrNoDisplayServer) {
				return fmt.Errorf("failed to open OpenGL context: %w (pass --headless to run without a window)", glCtx.InitErr)
			}
			return fmt.Errorf("failed to open OpenGL context: %w", glCtx.InitErr)
		}
		defer ctxObj.Destroy()
	}

	scene := newKitchenSinkDemoScene(glCtx, paths)

	bridge := luabridge.NewLuaBridge()
	defer bridge.Close()
	bridge.SetWorkspacePaths(paths)
	if opts.TileMap != "" {
		paths.TileMapPath = opts.TileMap
		bridge.SetWorkspacePaths(paths)
	}
	if bridge.TileMapEditor() != nil {
		bridge.TileMapEditor().ReloadScene = scene.RebuildTileMap
	}
	if ctxObj != nil {
		bridge.RegisterGameObject("context", ctxObj)
	}
	for id, obj := range scene.propObjects {
		bridge.RegisterGameObject(id, obj)
	}
	if scene.tileMapObj != nil {
		bridge.RegisterGameObject("tiles", scene.tileMapObj)
	}
	bridge.RegisterGameObject("hud_accent", scene.hudBarObj)
	bridge.RegisterGameObject("hud_status", scene.hudStatusObj)
	bridge.RegisterGameObject("hud_crosshair", scene.crosshair)

	console := luabridge.NewConsole(bridge)
	console.BindScene(scene.BaseScene)
	if !opts.Headless {
		if err := console.RedirectStdio(); err == nil {
			defer console.RestoreStdio()
		}
	}

	overlay := engine.NewImGuiOverlay()
	if opts.Headless {
		overlay.InitHeadless(float32(cfg.Width), float32(cfg.Height))
		defer overlay.Shutdown()
	}
	overlay.SetConsole(console)
	overlay.SetScriptEditor(bridge.ScriptEditor())
	overlay.SetPrefabEditor(bridge.PrefabEditor())
	overlay.SetTileMapBrowser(bridge.TileMapEditor())
	overlay.Tutorial = engine.NewTutorialWindow(paths.TutorialStateFile)
	if bridge.TileMapEditor() != nil && strings.TrimSpace(paths.TileMapPath) != "" {
		if _, err := bridge.TileMapEditor().LoadMap(paths.TileMapPath); err != nil {
			fmt.Printf("tile map startup load: %v\n", err)
		}
	}

	bridge.Shell().RegisterNative(luabridge.ShellCommandDef{
		Name:        "tutorial",
		Description: "Opens the interactive Quick Start Tutorial window.",
		Usage:       "tutorial",
		Examples:    []string{"tutorial"},
		GoHandler: func(_ luabridge.ExecutionContext) (string, error) {
			if overlay.Tutorial != nil {
				overlay.Tutorial.Show()
			}
			return "Opened Quick Start Tutorial window.", nil
		},
	})

	// Graceful script loading: record errors without crashing startup or blocking valid scripts.
	if _, loadErr := bridge.LoadScriptsDir(paths.ScriptsDir); loadErr != nil {
		console.Print(fmt.Sprintf("[ScriptWarning] %v", loadErr), luabridge.LineError)
	}
	// Attach the Rotator custom Lua component to RainbowCube if registered.
	if scene.cubeObj != nil && len(bridge.SearchComponents("Rotator")) > 0 {
		if _, err := bridge.Execute("comp add rainbow_cube Rotator"); err == nil {
			scene.autoRotate = false // driven by Rotator:tick
		}
	}
	for _, w := range scene.warnings {
		console.Print("[Scene] "+w, luabridge.LineInfo)
	}

	if !opts.Headless {
		overlay.OnGameMouseButton = func(button glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
			if button == glfw.MouseButtonLeft && action == glfw.Press && glCtx.Window != nil {
				ww, wh := glCtx.Window.GetSize()
				mx, my := glCtx.Window.GetCursorPos()
				if glCtx.CursorLocked {
					mx, my = float64(ww)*0.5, float64(wh)*0.5
				}
				res := console.PickAtScreenPos(scene.Camera, float32(mx), float32(my), float32(ww), float32(wh))
				if res.Hit && console.IsPickMode() {
					console.SetPickMode(false)
				}
			}
		}

		overlay.OnGameCursorPos = func(x, y float64) {
			scene.onCursorMove(x, y)
		}
		overlay.OnGameKey = func(key glfw.Key, _ int, action glfw.Action, _ glfw.ModifierKey) {
			if action != glfw.Press {
				return
			}
			switch key {
			case glfw.KeyF:
				glCtx.ToggleFullscreen()
			case glfw.KeyL:
				if glCtx.CursorLocked {
					glCtx.UnlockCursor()
				} else {
					glCtx.LockCursor()
				}
			case glfw.KeyGraveAccent:
				console.Toggle()
				if console.IsOpen && glCtx.CursorLocked {
					glCtx.UnlockCursor()
				}
			case glfw.KeyEscape:
				if glCtx.CursorLocked {
					glCtx.UnlockCursor()
				} else if glCtx.Window != nil {
					glCtx.Window.SetShouldClose(true)
				}
			}
		}

		overlay.DrawUI = func() {
			imgui.SetNextWindowSizeV(imgui.NewVec2(430, 360), imgui.CondFirstUseEver)
			if !imgui.Begin("Project Engine - Kitchen Sink Demo") {
				imgui.End()
				return
			}
			imgui.TextUnformatted(fmt.Sprintf("Framebuffer: %dx%d | FPS: %.1f", glCtx.FramebufferW, glCtx.FramebufferH, glCtx.FPS))
			imgui.TextUnformatted(fmt.Sprintf("Root: %s", filepath.ToSlash(paths.RootDir)))
			imgui.TextUnformatted(fmt.Sprintf("Tiles: %d (Floors: %d, Walls: %d, Pillars: %d, Stairs: %d) | Props: %d",
				scene.tileStats.TotalTiles, scene.tileStats.FloorCount, scene.tileStats.WallCount,
				scene.tileStats.PillarCount, scene.tileStats.StairCount, len(scene.propObjects)))
			if tURL := overlay.GetTunnelURL(); tURL != "" {
				imgui.TextColored(imgui.NewVec4(0.55, 0.95, 0.65, 1.0), fmt.Sprintf("Tunnel URL: %s", tURL))
			}
			imgui.TextUnformatted("Shortcuts: [F] Fullscreen  [L] Cursor Lock  [~] Lua Console  [Esc] Unlock/Quit")

			imgui.Separator()
			imgui.Checkbox("Auto Rotate Cube", &scene.autoRotate)
			imgui.SameLine()
			imgui.Checkbox("Wireframe", &scene.showWireframe)
			imgui.SameLine()
			imgui.Checkbox("Show Tiles", &scene.showTiles)
			imgui.Checkbox("Simulate Physics", &scene.simulatePhys)
			imgui.SliderFloat("Rotation Speed", &scene.rotSpeed, -180.0, 180.0)
			imgui.SliderFloat("Camera Distance", &scene.orbitDistance, 2.5, 18.0)

			if imgui.Button("Overview Cam") {
				scene.SetCameraPreset("overview")
			}
			imgui.SameLine()
			if imgui.Button("Props Cam") {
				scene.SetCameraPreset("props")
			}
			imgui.SameLine()
			if imgui.Button("Stairs Cam") {
				scene.SetCameraPreset("stairs")
			}

			if imgui.Button("Impulse Props") {
				scene.PulseShowcaseProps()
			}
			imgui.SameLine()
			if imgui.Button("Fracture Lamp") {
				scene.FractureDemoLamp()
			}
			imgui.SameLine()
			if imgui.Button("Reset Props") {
				scene.ResetShowcaseProps()
			}

			if imgui.Button("Toggle Fullscreen") {
				glCtx.ToggleFullscreen()
			}
			imgui.SameLine()
			if imgui.Button("Toggle Cursor Lock") {
				if glCtx.CursorLocked {
					glCtx.UnlockCursor()
				} else {
					glCtx.LockCursor()
				}
			}
			imgui.SameLine()
			if imgui.Button("Tutorial") && overlay.Tutorial != nil {
				overlay.Tutorial.Show()
			}

			if scriptErrs := bridge.GetScriptErrors(); len(scriptErrs) > 0 {
				imgui.Separator()
				imgui.TextColored(imgui.NewVec4(1.0, 0.42, 0.42, 1.0), fmt.Sprintf("Script Errors (%d) - Engine running safely:", len(scriptErrs)))
				for i := len(scriptErrs) - 1; i >= 0 && i >= len(scriptErrs)-3; i-- {
					se := scriptErrs[i]
					imgui.TextWrapped(fmt.Sprintf("  %s [%s]: %s", filepath.Base(se.ScriptPath), se.Hook, se.Message))
				}
				if imgui.Button("Clear Script Errors") {
					bridge.ClearScriptErrors()
				}
			}
			imgui.End()
		}
	}

	var app *engine.GLApp
	if !opts.Headless {
		app = engine.NewGLApp(cfg, scene, overlay)
		app.Context = glCtx
	}

	if opts.Serve {
		agentServer := luabridge.NewAgentServer(bridge, console, scene.BaseScene, luabridge.AgentServerConfig{
			Address: opts.Address,
			Token:   opts.Token,
		})
		agentServer.DeferFrameCapture = !opts.Headless
		if err := agentServer.Start(); err != nil {
			return fmt.Errorf("failed to start agent bridge server: %w", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = agentServer.Stop(ctx)
		}()
		if !opts.Headless && app != nil {
			app.OnPostRender = func() {
				agentServer.FlushFrameCaptures(glCtx.FramebufferW, glCtx.FramebufferH)
			}
		}
		console.Print(fmt.Sprintf("[AgentServer] Listening on %s (/rpc, /ws)", agentServer.BoundAddress()), luabridge.LineSystem)

		if opts.Tunnel {
			tunnelCtx, tunnelCancel := context.WithCancel(context.Background())
			defer tunnelCancel()
			tm := luabridge.NewTunnelManager(console, luabridge.TunnelConfig{
				TargetAddress: agentServer.BoundAddress(),
				OnURL: func(url string) {
					overlay.SetTunnelURL(url)
				},
			})
			go func() {
				if err := tm.Start(tunnelCtx); err != nil {
					console.Print(fmt.Sprintf("[Tunnel] Error: %v", err), luabridge.LineError)
				}
			}()
			defer tm.Stop()
		}
	}

	if opts.Headless {
		log.Printf("[Headless] Running without a window; scene ticks and agent bridge are active. Press Ctrl+C to stop.")
		runHeadlessLoop(ctx, scene, 0)
		return nil
	}

	if err := app.Run(); err != nil {
		return fmt.Errorf("demo runtime error: %w", err)
	}
	return nil
}

func degToRad(deg float32) float32 {
	return deg * math.Pi / 180.0
}

// Column-major 4x4 matrix utilities matching OpenGL uniform layout.

func mat4Mul(a, b [16]float32) [16]float32 {
	var out [16]float32
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			out[col*4+row] = sum
		}
	}
	return out
}

func mat4Perspective(fovDeg, aspect, near, far float32) [16]float32 {
	f := float32(1.0 / math.Tan(float64(degToRad(fovDeg))/2.0))
	nf := 1.0 / (near - far)
	return [16]float32{
		f / aspect, 0, 0, 0,
		0, f, 0, 0,
		0, 0, (far + near) * nf, -1,
		0, 0, 2 * far * near * nf, 0,
	}
}

func mat4LookAt(eye, target, up engine.Vector3) [16]float32 {
	fx, fy, fz := target.X-eye.X, target.Y-eye.Y, target.Z-eye.Z
	flen := float32(math.Sqrt(float64(fx*fx + fy*fy + fz*fz)))
	if flen > 0 {
		fx, fy, fz = fx/flen, fy/flen, fz/flen
	}

	// s = normalize(f x up)
	sx := fy*up.Z - fz*up.Y
	sy := fz*up.X - fx*up.Z
	sz := fx*up.Y - fy*up.X
	slen := float32(math.Sqrt(float64(sx*sx + sy*sy + sz*sz)))
	if slen > 0 {
		sx, sy, sz = sx/slen, sy/slen, sz/slen
	}

	// u = s x f
	ux := sy*fz - sz*fy
	uy := sz*fx - sx*fz
	uz := sx*fy - sy*fx

	return [16]float32{
		sx, ux, -fx, 0,
		sy, uy, -fy, 0,
		sz, uz, -fz, 0,
		-(sx*eye.X + sy*eye.Y + sz*eye.Z),
		-(ux*eye.X + uy*eye.Y + uz*eye.Z),
		fx*eye.X + fy*eye.Y + fz*eye.Z,
		1,
	}
}
