package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/pkg/luabridge"
	"github.com/go-gl/glfw/v3.3/glfw"
)

const rainbowCubeGLTFPath = "assets/models/props/rainbow_cube.gltf"

// RainbowCubeDemoScene renders an OpenGL 3.3 Core rotating Rainbow Cube via
// TransformComponent + MeshComponent + MeshRenderable alongside 2D HUD Renderable2D
// elements and interactive camera/GLContext controls.
type RainbowCubeDemoScene struct {
	*engine.BaseScene

	glCtx          *engine.GLContext
	cubeObj        *engine.GameObject
	cubeTransform  *engine.TransformComponent
	cubeMesh       *engine.MeshComponent
	cubeRenderable *engine.MeshRenderable
	hudBarObj      *engine.GameObject
	crosshair      *engine.GameObject

	rotDeg        float32
	rotSpeed      float32
	autoRotate    bool
	showWireframe bool

	orbitYawDeg   float32
	orbitPitchDeg float32
	orbitDistance float32
	hasLastMouse  bool
	lastMouseX    float64
	lastMouseY    float64
}

func newRainbowCubeDemoScene(glCtx *engine.GLContext) *RainbowCubeDemoScene {
	base := engine.NewBaseScene("GLContext Demo Scene")
	s := &RainbowCubeDemoScene{
		BaseScene:     base,
		glCtx:         glCtx,
		rotSpeed:      48.0,
		autoRotate:    true,
		orbitYawDeg:   35.0,
		orbitPitchDeg: 22.0,
		orbitDistance: 4.2,
	}

	s.cubeObj = engine.NewGameObject("rainbow_cube", "RainbowCube")
	s.cubeTransform = engine.NewTransformComponent()
	s.cubeMesh = engine.NewMeshComponent(rainbowCubeGLTFPath)
	if s.cubeMesh.LoadErr != nil {
		log.Fatalf("failed loading %s: %v", rainbowCubeGLTFPath, s.cubeMesh.LoadErr)
	}
	s.cubeRenderable = engine.NewMeshRenderable()
	s.cubeObj.AddComponent(s.cubeTransform)
	s.cubeObj.AddComponent(s.cubeMesh)
	s.cubeObj.AddComponent(s.cubeRenderable)
	s.AddObject(s.cubeObj)

	// 2D HUD accent bar (top-left) and centered crosshair rendered by OrthoBatcher2D.
	s.hudBarObj = engine.NewGameObject("hud_accent", "HudAccentBar")
	s.hudBarObj.EnsureTransform().SetPosition(engine.NewVector3(16, 16, 0))
	bar := engine.NewRenderable2D(0, 220, 6)
	bar.Anchor = engine.AnchorTopLeft
	bar.Color = engine.NewColor(64, 210, 255, 220)
	bar.ZIndex = 10
	s.hudBarObj.AddComponent(bar)
	s.AddObject(s.hudBarObj)

	s.crosshair = engine.NewGameObject("hud_crosshair", "Crosshair")
	s.crosshair.EnsureTransform()
	dot := engine.NewRenderable2D(0, 6, 6)
	dot.Anchor = engine.AnchorCenter
	dot.Color = engine.NewColor(255, 255, 255, 190)
	dot.ZIndex = 20
	s.crosshair.AddComponent(dot)
	s.AddObject(s.crosshair)

	s.updateCameraPosition()
	return s
}

func (s *RainbowCubeDemoScene) Init() {
	s.BaseScene.Init()
	if s.cubeRenderable != nil && s.cubeRenderable.ConstraintErr != nil {
		log.Fatalf("demo cube renderable constraint error: %v", s.cubeRenderable.ConstraintErr)
	}
}

func (s *RainbowCubeDemoScene) Update(dt float32) {
	s.BaseScene.Update(dt)
	if s.autoRotate && s.cubeTransform != nil {
		s.rotDeg += s.rotSpeed * dt
		if s.rotDeg >= 360 {
			s.rotDeg -= 360
		}
		s.cubeTransform.SetRotation(engine.NewVector3(s.rotDeg*0.55, s.rotDeg, 0))
	}
	s.updateCameraPosition()
}

func (s *RainbowCubeDemoScene) Draw3D() {
	aspect := float32(1280.0 / 720.0)
	if s.glCtx != nil && s.glCtx.FramebufferH > 0 {
		aspect = float32(s.glCtx.FramebufferW) / float32(s.glCtx.FramebufferH)
	}

	proj := mat4Perspective(s.Camera.Fovy, aspect, 0.1, 100.0)
	view := mat4LookAt(s.Camera.Position, s.Camera.Target, s.Camera.Up)
	viewProj := mat4Mul(proj, view)

	if s.cubeRenderable != nil {
		s.cubeRenderable.Wireframe = s.showWireframe
	}
	s.DrawMeshRenderables(viewProj)
}

func (s *RainbowCubeDemoScene) Unload() {
	if s.cubeRenderable != nil {
		s.cubeRenderable.DisposeGL()
	}
	s.BaseScene.Unload()
}

func (s *RainbowCubeDemoScene) onCursorMove(x, y float64) {
	if s.glCtx == nil || !s.glCtx.CursorLocked {
		s.hasLastMouse = false
		return
	}
	if !s.hasLastMouse {
		s.lastMouseX = x
		s.lastMouseY = y
		s.hasLastMouse = true
		return
	}
	dx := float32(x - s.lastMouseX)
	dy := float32(y - s.lastMouseY)
	s.lastMouseX = x
	s.lastMouseY = y

	s.orbitYawDeg -= dx * 0.22
	s.orbitPitchDeg += dy * 0.22
	if s.orbitPitchDeg > 80 {
		s.orbitPitchDeg = 80
	} else if s.orbitPitchDeg < -80 {
		s.orbitPitchDeg = -80
	}
}

func (s *RainbowCubeDemoScene) updateCameraPosition() {
	yaw := float64(degToRad(s.orbitYawDeg))
	pitch := float64(degToRad(s.orbitPitchDeg))
	d := float64(s.orbitDistance)

	s.Camera.Position = engine.NewVector3(
		float32(d*math.Cos(pitch)*math.Sin(yaw)),
		float32(d*math.Sin(pitch)),
		float32(d*math.Cos(pitch)*math.Cos(yaw)),
	)
	s.Camera.Target = engine.NewVector3(0, 0, 0)
	s.Camera.Up = engine.NewVector3(0, 1, 0)
	if s.Camera.Fovy <= 0 {
		s.Camera.Fovy = 50
	}
}

type demoServerOptions struct {
	Serve   bool
	Tunnel  bool
	Address string
	Token   string
}

func parseDemoServerFlags(args []string) (demoServerOptions, error) {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	var opts demoServerOptions
	fs.BoolVar(&opts.Serve, "serve", false, "Start dual-transport HTTP JSON-RPC + WebSocket agent bridge server")
	fs.BoolVar(&opts.Tunnel, "tunnel", false, "Auto-locate/download cloudflared and open a quick tunnel to --address (implies --serve)")
	fs.StringVar(&opts.Address, "address", luabridge.DefaultAgentServerAddress, "Bind address for agent bridge server")
	fs.StringVar(&opts.Token, "token", "", "Required authentication token when --serve or --tunnel is enabled (may be empty)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.Tunnel {
		opts.Serve = true
	}
	tokenProvided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "token" {
			tokenProvided = true
		}
	})
	if opts.Serve && !tokenProvided {
		return opts, fmt.Errorf("--serve / --tunnel requires --token to be explicitly provided (e.g. --token=\"\" or --token=secret)")
	}
	return opts, nil
}

func main() {
	serverOpts, err := parseDemoServerFlags(os.Args[1:])
	if err != nil {
		log.Fatalf("flag error: %v", err)
	}

	cfg := engine.DefaultGLWindowConfig()
	cfg.Title = "Project Engine - GLContext OpenGL 3.3 Demo"

	// Instantiate GLContext as an engine.Component attached to a GameObject.
	ctxObj := engine.NewGameObject("gl_context", "GLContextOwner")
	glCtx := engine.NewGLContext(cfg)
	glCtx.AutoOpen = true
	ctxObj.AddComponent(glCtx)
	if glCtx.InitErr != nil {
		log.Fatalf("failed to open OpenGL context: %v", glCtx.InitErr)
	}
	defer ctxObj.Destroy()

	scene := newRainbowCubeDemoScene(glCtx)

	bridge := luabridge.NewLuaBridge()
	defer bridge.Close()
	_, _ = bridge.LoadScriptsDir("scripts")
	bridge.RegisterGameObject("context", ctxObj)
	bridge.RegisterGameObject("cube", scene.cubeObj)
	bridge.RegisterGameObject("hud_accent", scene.hudBarObj)
	bridge.RegisterGameObject("hud_crosshair", scene.crosshair)
	console := luabridge.NewConsole(bridge)
	console.BindScene(scene.BaseScene)
	if err := console.RedirectStdio(); err == nil {
		defer console.RestoreStdio()
	}

	overlay := engine.NewImGuiOverlay()
	overlay.SetConsole(console)
	overlay.SetScriptEditor(bridge.ScriptEditor())
	overlay.SetPrefabEditor(bridge.PrefabEditor())

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
		if !imgui.Begin("GLContext & Rainbow Cube Demo") {
			imgui.End()
			return
		}
		imgui.TextUnformatted(fmt.Sprintf("Framebuffer: %dx%d | FPS: %.1f", glCtx.FramebufferW, glCtx.FramebufferH, glCtx.FPS))
		imgui.TextUnformatted(fmt.Sprintf("Fullscreen: %v | Cursor Locked: %v", glCtx.IsFullscreen, glCtx.CursorLocked))
		if tURL := overlay.GetTunnelURL(); tURL != "" {
			imgui.TextColored(imgui.NewVec4(0.55, 0.95, 0.65, 1.0), fmt.Sprintf("Tunnel URL: %s", tURL))
		}
		imgui.TextUnformatted("Shortcuts: [F] Fullscreen  [L] Cursor Lock  [~] Lua Console  [Esc] Unlock/Quit")

		imgui.Checkbox("Auto Rotate", &scene.autoRotate)
		imgui.SameLine()
		imgui.Checkbox("Wireframe", &scene.showWireframe)
		imgui.SliderFloat("Rotation Speed", &scene.rotSpeed, -180.0, 180.0)
		imgui.SliderFloat("Camera Distance", &scene.orbitDistance, 2.0, 10.0)

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
		if imgui.Button("Reset Cube") {
			scene.rotDeg = 0
			if scene.cubeTransform != nil {
				scene.cubeTransform.SetRotation(engine.NewVector3(0, 0, 0))
			}
			scene.orbitYawDeg = 35
			scene.orbitPitchDeg = 22
			scene.orbitDistance = 4.2
		}
		imgui.End()
	}

	app := engine.NewGLApp(cfg, scene, overlay)
	app.Context = glCtx

	if serverOpts.Serve {
		agentServer := luabridge.NewAgentServer(bridge, console, scene.BaseScene, luabridge.AgentServerConfig{
			Address: serverOpts.Address,
			Token:   serverOpts.Token,
		})
		agentServer.DeferFrameCapture = true
		if err := agentServer.Start(); err != nil {
			log.Fatalf("failed to start agent bridge server: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = agentServer.Stop(ctx)
		}()
		app.OnPostRender = func() {
			agentServer.FlushFrameCaptures(glCtx.FramebufferW, glCtx.FramebufferH)
		}
		console.Print(fmt.Sprintf("[AgentServer] Listening on %s (/rpc, /ws)", agentServer.BoundAddress()), luabridge.LineSystem)

		if serverOpts.Tunnel {
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

	if err := app.Run(); err != nil {
		log.Fatalf("demo runtime error: %v", err)
	}
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
