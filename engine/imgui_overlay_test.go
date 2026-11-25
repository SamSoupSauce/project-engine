package engine

import (
	"testing"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type mockGLStateAPI struct {
	viewport           [4]int32
	scissorBox         [4]int32
	activeProgram      int32
	activeTexture      int32
	arrayBuffer        int32
	vertexArray        int32
	blendEnabled       bool
	blendSrcRGB        int32
	blendDstRGB        int32
	blendSrcAlpha      int32
	blendDstAlpha      int32
	blendEqRGB         int32
	blendEqAlpha       int32
	depthTestEnabled   bool
	depthWriteMask     bool
	cullFaceEnabled    bool
	scissorTestEnabled bool
}

func (m *mockGLStateAPI) GetIntegerv(pname uint32, data *int32) {
	switch pname {
	case gl.VIEWPORT:
		copy((*(*[4]int32)(dataSlice4(data)))[:], m.viewport[:])
	case gl.SCISSOR_BOX:
		copy((*(*[4]int32)(dataSlice4(data)))[:], m.scissorBox[:])
	case gl.CURRENT_PROGRAM:
		*data = m.activeProgram
	case gl.ACTIVE_TEXTURE:
		*data = m.activeTexture
	case gl.ARRAY_BUFFER_BINDING:
		*data = m.arrayBuffer
	case gl.VERTEX_ARRAY_BINDING:
		*data = m.vertexArray
	case gl.BLEND_SRC_RGB:
		*data = m.blendSrcRGB
	case gl.BLEND_DST_RGB:
		*data = m.blendDstRGB
	case gl.BLEND_SRC_ALPHA:
		*data = m.blendSrcAlpha
	case gl.BLEND_DST_ALPHA:
		*data = m.blendDstAlpha
	case gl.BLEND_EQUATION_RGB:
		*data = m.blendEqRGB
	case gl.BLEND_EQUATION_ALPHA:
		*data = m.blendEqAlpha
	}
}

func dataSlice4(p *int32) *[4]int32 {
	// Access contiguous 4-element buffer passed by BackupGLState (&s.Viewport[0]).
	return (*[4]int32)(unsafePtrFromInt32(p))
}

func (m *mockGLStateAPI) GetBooleanv(pname uint32, data *bool) {
	if pname == gl.DEPTH_WRITEMASK {
		*data = m.depthWriteMask
	}
}

func (m *mockGLStateAPI) IsEnabled(cap uint32) bool {
	switch cap {
	case gl.BLEND:
		return m.blendEnabled
	case gl.DEPTH_TEST:
		return m.depthTestEnabled
	case gl.CULL_FACE:
		return m.cullFaceEnabled
	case gl.SCISSOR_TEST:
		return m.scissorTestEnabled
	default:
		return false
	}
}

func (m *mockGLStateAPI) Enable(cap uint32)  { m.setCap(cap, true) }
func (m *mockGLStateAPI) Disable(cap uint32) { m.setCap(cap, false) }

func (m *mockGLStateAPI) setCap(cap uint32, v bool) {
	switch cap {
	case gl.BLEND:
		m.blendEnabled = v
	case gl.DEPTH_TEST:
		m.depthTestEnabled = v
	case gl.CULL_FACE:
		m.cullFaceEnabled = v
	case gl.SCISSOR_TEST:
		m.scissorTestEnabled = v
	}
}

func (m *mockGLStateAPI) Viewport(x, y, w, h int32) { m.viewport = [4]int32{x, y, w, h} }
func (m *mockGLStateAPI) Scissor(x, y, w, h int32)  { m.scissorBox = [4]int32{x, y, w, h} }
func (m *mockGLStateAPI) UseProgram(program uint32) { m.activeProgram = int32(program) }
func (m *mockGLStateAPI) ActiveTexture(tex uint32)  { m.activeTexture = int32(tex) }
func (m *mockGLStateAPI) BindBuffer(target, buf uint32) {
	if target == gl.ARRAY_BUFFER {
		m.arrayBuffer = int32(buf)
	}
}
func (m *mockGLStateAPI) BindVertexArray(arr uint32) { m.vertexArray = int32(arr) }
func (m *mockGLStateAPI) BlendFuncSeparate(sRGB, dRGB, sA, dA uint32) {
	m.blendSrcRGB = int32(sRGB)
	m.blendDstRGB = int32(dRGB)
	m.blendSrcAlpha = int32(sA)
	m.blendDstAlpha = int32(dA)
}
func (m *mockGLStateAPI) BlendEquationSeparate(mRGB, mA uint32) {
	m.blendEqRGB = int32(mRGB)
	m.blendEqAlpha = int32(mA)
}
func (m *mockGLStateAPI) DepthMask(flag bool) { m.depthWriteMask = flag }

type mockConsoleBackend struct {
	open    bool
	lines   []DebugConsoleLine
	history []string
}

func (b *mockConsoleBackend) ExecuteCommand(input string) string {
	b.history = append(b.history, input)
	b.lines = append(b.lines, DebugConsoleLine{Text: "> " + input, Kind: ConsoleLineInput})
	out := "exec:" + input
	b.lines = append(b.lines, DebugConsoleLine{Text: out, Kind: ConsoleLineOutput})
	return out
}
func (b *mockConsoleBackend) GetOutputLines() []DebugConsoleLine { return b.lines }
func (b *mockConsoleBackend) ClearOutput()                       { b.lines = b.lines[:0] }
func (b *mockConsoleBackend) IsConsoleOpen() bool                { return b.open }
func (b *mockConsoleBackend) SetConsoleOpen(open bool)           { b.open = open }
func (b *mockConsoleBackend) ToggleConsole()                     { b.open = !b.open }
func (b *mockConsoleBackend) AppendLog(t string, k DebugConsoleLineKind) {
	b.lines = append(b.lines, DebugConsoleLine{Text: t, Kind: k})
}
func (b *mockConsoleBackend) GetHistory() []string { return b.history }

func TestImGuiOverlayHeadlessDemoAndMetricsRender(t *testing.T) {
	overlay := NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()

	overlay.ShowDemo = true
	overlay.ShowMetrics = true
	overlay.MetricsFPS = 60
	overlay.SceneName = "TestWorld"
	overlay.ObjectCount = 12

	var drawDataValid bool
	var vtxCount int32
	overlay.onHeadlessDraw = func(dd *imgui.DrawData) {
		if dd != nil {
			drawDataValid = dd.Valid()
			vtxCount = dd.TotalVtxCount()
		}
	}

	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	if !drawDataValid {
		t.Fatalf("expected valid DrawData after ImGui Render")
	}
	if vtxCount <= 0 {
		t.Fatalf("expected non-zero vertices from Demo/Metrics windows, got %d", vtxCount)
	}
}

func TestImGuiOverlayGLStatePreservation(t *testing.T) {
	mockGL := &mockGLStateAPI{
		viewport:           [4]int32{0, 0, 1280, 720},
		scissorBox:         [4]int32{0, 0, 1280, 720},
		activeProgram:      7,
		activeTexture:      gl.TEXTURE2,
		arrayBuffer:        11,
		vertexArray:        5,
		blendEnabled:       false,
		blendSrcRGB:        gl.ONE,
		blendDstRGB:        gl.ZERO,
		blendSrcAlpha:      gl.ONE,
		blendDstAlpha:      gl.ZERO,
		blendEqRGB:         gl.FUNC_ADD,
		blendEqAlpha:       gl.FUNC_ADD,
		depthTestEnabled:   true,
		depthWriteMask:     true,
		cullFaceEnabled:    true,
		scissorTestEnabled: false,
	}

	overlay := NewImGuiOverlay()
	overlay.SetGLStateAPI(mockGL)
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()

	overlay.ShowMetrics = true
	overlay.onHeadlessDraw = func(_ *imgui.DrawData) {
		// Simulate ImGui OpenGL3 backend mutating pipeline state during draw.
		mockGL.Viewport(10, 20, 300, 200)
		mockGL.Scissor(5, 5, 100, 100)
		mockGL.UseProgram(99)
		mockGL.ActiveTexture(gl.TEXTURE0)
		mockGL.BindBuffer(gl.ARRAY_BUFFER, 44)
		mockGL.BindVertexArray(33)
		mockGL.Enable(gl.BLEND)
		mockGL.Enable(gl.SCISSOR_TEST)
		mockGL.Disable(gl.DEPTH_TEST)
		mockGL.Disable(gl.CULL_FACE)
		mockGL.DepthMask(false)
		mockGL.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
	}

	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	if mockGL.viewport != [4]int32{0, 0, 1280, 720} {
		t.Errorf("viewport not restored: got %v", mockGL.viewport)
	}
	if mockGL.scissorBox != [4]int32{0, 0, 1280, 720} {
		t.Errorf("scissorBox not restored: got %v", mockGL.scissorBox)
	}
	if mockGL.activeProgram != 7 || mockGL.vertexArray != 5 || mockGL.arrayBuffer != 11 {
		t.Errorf("program/VAO/VBO not restored: prog=%d vao=%d vbo=%d", mockGL.activeProgram, mockGL.vertexArray, mockGL.arrayBuffer)
	}
	if mockGL.blendEnabled || !mockGL.depthTestEnabled || !mockGL.depthWriteMask || !mockGL.cullFaceEnabled || mockGL.scissorTestEnabled {
		t.Errorf("GL caps not restored: blend=%v depth=%v mask=%v cull=%v scissor=%v",
			mockGL.blendEnabled, mockGL.depthTestEnabled, mockGL.depthWriteMask, mockGL.cullFaceEnabled, mockGL.scissorTestEnabled)
	}
	if mockGL.blendSrcRGB != gl.ONE || mockGL.blendDstRGB != gl.ZERO {
		t.Errorf("blend funcs not restored: src=%d dst=%d", mockGL.blendSrcRGB, mockGL.blendDstRGB)
	}
}

func TestImGuiOverlayInputCaptureGuardsGameCallbacks(t *testing.T) {
	overlay := NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()

	var gameMouseClicks, gameKeys int
	overlay.OnGameMouseButton = func(_ glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
		if action == glfw.Press {
			gameMouseClicks++
		}
	}
	overlay.OnGameKey = func(_ glfw.Key, _ int, _ glfw.Action, _ glfw.ModifierKey) {
		gameKeys++
	}

	// Frame 1: draw a widget window at (0,0)-(400,400) and hover cursor inside it.
	overlay.DrawUI = func() {
		imgui.SetNextWindowPosV(imgui.NewVec2(0, 0), imgui.CondAlways, imgui.NewVec2(0, 0))
		imgui.SetNextWindowSizeV(imgui.NewVec2(400, 400), imgui.CondAlways)
		if imgui.Begin("CapturedPanel") {
			imgui.Button("Test")
		}
		imgui.End()
	}

	step := func() {
		overlay.NewFrame()
		overlay.DrawDebugWindows()
		overlay.Render()
	}

	// Frame 1: register the ImGui window layout at (0,0)-(400,400).
	// Dear ImGui computes WantCaptureMouse against windows from the previous frame.
	step()

	// Move mouse inside window before Frame 2 so ImGui updates WantCaptureMouse.
	overlay.HandleCursorPos(100, 100)
	step()

	if !overlay.WantCaptureMouse() {
		t.Fatalf("expected WantCaptureMouse=true when hovering ImGui window")
	}

	overlay.HandleMouseButton(glfw.MouseButtonLeft, glfw.Press, 0)
	step()
	if gameMouseClicks != 0 {
		t.Fatalf("expected mouse click not to leak to game when WantCaptureMouse=true, got %d", gameMouseClicks)
	}
	overlay.HandleMouseButton(glfw.MouseButtonLeft, glfw.Release, 0)
	overlay.HandleCursorPos(900, 600)
	step()
	step()

	if overlay.WantCaptureMouse() {
		t.Fatalf("expected WantCaptureMouse=false when cursor is outside ImGui window")
	}
	overlay.HandleMouseButton(glfw.MouseButtonLeft, glfw.Press, 0)
	overlay.HandleMouseButton(glfw.MouseButtonLeft, glfw.Release, 0)
	if gameMouseClicks != 1 {
		t.Fatalf("expected game mouse click when WantCaptureMouse=false, got %d", gameMouseClicks)
	}

	// Keyboard capture guard
	imgui.SetNextFrameWantCaptureKeyboard(true)
	step()

	if !overlay.WantCaptureKeyboard() {
		t.Fatalf("expected WantCaptureKeyboard=true")
	}
	overlay.HandleKey(glfw.KeyW, 0, glfw.Press, 0)
	if gameKeys != 0 {
		t.Fatalf("expected key event not to leak when WantCaptureKeyboard=true, got %d", gameKeys)
	}

	step()
	if overlay.WantCaptureKeyboard() {
		t.Fatalf("expected WantCaptureKeyboard=false on subsequent frame")
	}
	overlay.HandleKey(glfw.KeyW, 0, glfw.Press, 0)
	if gameKeys != 1 {
		t.Fatalf("expected game key event when WantCaptureKeyboard=false, got %d", gameKeys)
	}
}

func TestDebugConsoleToggleAndSubmitHeadless(t *testing.T) {
	overlay := NewImGuiOverlay()
	overlay.InitHeadless(1280, 720)
	defer overlay.Shutdown()

	backend := &mockConsoleBackend{}
	overlay.SetConsole(backend)

	if overlay.Console.IsOpen() {
		t.Fatalf("console should start closed")
	}

	// Toggle open with backtick/tilde
	if !overlay.HandleKey(glfw.KeyGraveAccent, 0, glfw.Press, 0) {
		t.Fatalf("expected grave accent key to toggle console")
	}
	if !overlay.Console.IsOpen() {
		t.Fatalf("expected console to be open after toggle key")
	}

	out := overlay.Console.Submit("ping")
	if out != "exec:ping" {
		t.Fatalf("expected 'exec:ping', got %q", out)
	}

	overlay.NewFrame()
	overlay.DrawDebugWindows()
	overlay.Render()

	// Toggle closed
	if !overlay.HandleKey(glfw.KeyGraveAccent, 0, glfw.Press, 0) {
		t.Fatalf("expected second grave accent key to close console")
	}
	if overlay.Console.IsOpen() {
		t.Fatalf("expected console to be closed")
	}
}

func TestImGuiOverlayLiveGLFWOpenGL3(t *testing.T) {
	cfg := DefaultGLWindowConfig()
	cfg.Visible = false
	cfg.Width = 320
	cfg.Height = 240

	overlay := NewImGuiOverlay()
	overlay.ShowDemo = true
	overlay.ShowMetrics = true
	app := NewGLApp(cfg, nil, overlay)
	if err := app.InitWindow(); err != nil {
		t.Skipf("skipping live GLFW/OpenGL3 test in headless environment: %v", err)
	}
	defer app.Close()

	// Configure host 3D GL state, run StepFrame, and confirm depth/blend/viewport remain intact.
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.BLEND)
	gl.Viewport(0, 0, 320, 240)

	app.StepFrame(0.016)

	if !gl.IsEnabled(gl.DEPTH_TEST) {
		t.Errorf("expected GL_DEPTH_TEST to remain enabled after ImGui overlay pass")
	}
	if gl.IsEnabled(gl.BLEND) {
		t.Errorf("expected GL_BLEND to remain disabled after ImGui overlay pass")
	}
	var vp [4]int32
	gl.GetIntegerv(gl.VIEWPORT, &vp[0])
	if vp != [4]int32{0, 0, 320, 240} {
		t.Errorf("expected viewport [0 0 320 240], got %v", vp)
	}
}
