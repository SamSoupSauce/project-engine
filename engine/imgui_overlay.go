package engine

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/impl/opengl3"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// ImGuiOverlay manages the Dear ImGui lifecycle, GLFW input callback hooks,
// OpenGL 3 rendering pass with pipeline state backup/restore, and debug overlays.
type ImGuiOverlay struct {
	ctx         *imgui.Context
	window      *glfw.Window
	glAPI       GLStateAPI
	initialized bool
	headless    bool
	lastTime    float64

	Console      *DebugConsoleWindow
	ScriptEditor *ScriptEditorWindow
	ShowDemo     bool
	ShowMetrics  bool

	MetricsFPS  float32
	SceneName   string
	ObjectCount int

	DrawUI         func()
	onHeadlessDraw func(dd *imgui.DrawData)

	// Game input callbacks invoked only when ImGui is not capturing input.
	OnGameCursorPos   func(x, y float64)
	OnGameMouseButton func(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey)
	OnGameScroll      func(xoff, yoff float64)
	OnGameKey         func(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)
	OnGameChar        func(char rune)

	prevCursorPos   glfw.CursorPosCallback
	prevMouseButton glfw.MouseButtonCallback
	prevScroll      glfw.ScrollCallback
	prevKey         glfw.KeyCallback
	prevChar        glfw.CharCallback
	prevFocus       glfw.FocusCallback
	prevCursorEnter glfw.CursorEnterCallback
}

// NewImGuiOverlay creates an uninitialized ImGuiOverlay instance.
func NewImGuiOverlay() *ImGuiOverlay {
	return &ImGuiOverlay{}
}

// SetGLStateAPI overrides the OpenGL state backend (useful for headless tests).
func (o *ImGuiOverlay) SetGLStateAPI(api GLStateAPI) {
	o.glAPI = api
}

// Init sets up the Dear ImGui context, font atlas, GLFW input hooks, and OpenGL 3 shader pipeline.
func (o *ImGuiOverlay) Init(window *glfw.Window) error {
	if window == nil {
		return fmt.Errorf("engine: glfw window must not be nil")
	}
	if o.initialized {
		return nil
	}
	if o.glAPI == nil {
		o.glAPI = RealGLStateAPI{}
	}

	window.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		return fmt.Errorf("engine: failed to initialize OpenGL: %w", err)
	}

	o.ctx = imgui.CreateContext()
	imgui.SetCurrentContext(o.ctx)

	io := imgui.CurrentIO()
	io.SetConfigFlags(io.ConfigFlags() | imgui.ConfigFlagsNavEnableKeyboard)
	io.Fonts().AddFontDefault()

	w, h := window.GetSize()
	fbW, fbH := window.GetFramebufferSize()
	if w > 0 && h > 0 {
		io.SetDisplaySize(imgui.NewVec2(float32(w), float32(h)))
		io.SetDisplayFramebufferScale(imgui.NewVec2(float32(fbW)/float32(w), float32(fbH)/float32(h)))
	}
	io.SetDeltaTime(1.0 / 60.0)

	if !opengl3.InitV("#version 330 core") {
		imgui.DestroyContextV(o.ctx)
		o.ctx = nil
		return fmt.Errorf("engine: ImGui_ImplOpenGL3_Init failed")
	}

	o.window = window
	o.headless = false
	o.lastTime = glfw.GetTime()
	o.initialized = true
	o.installGLFWCallbacks(window)
	return nil
}

// InitHeadless creates a Dear ImGui context configured for offscreen/headless UI and input tests
// without requiring a GLFW window or OpenGL context.
func (o *ImGuiOverlay) InitHeadless(width, height float32) {
	if o.initialized {
		return
	}
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 720
	}

	o.ctx = imgui.CreateContext()
	imgui.SetCurrentContext(o.ctx)

	io := imgui.CurrentIO()
	io.SetBackendFlags(imgui.BackendFlagsRendererHasTextures)
	io.SetDisplaySize(imgui.NewVec2(width, height))
	io.SetDeltaTime(1.0 / 60.0)
	io.Fonts().AddFontDefault()
	imgui.InternalImFontAtlasBuildMain(io.Fonts())

	// Ensure font atlas texture reference is initialized cleanly for headless frames.
	texRef := io.Fonts().TexRef()
	if texRef.TexID() == 0 {
		io.Fonts().SetTexRef(*imgui.NewTextureRefTextureID(1))
	}

	o.headless = true
	o.initialized = true
}

// SetConsole attaches a DebugConsoleWindow (or wraps a DebugConsoleBackend) to the overlay.
func (o *ImGuiOverlay) SetConsole(backend DebugConsoleBackend) {
	if backend == nil {
		o.Console = nil
		return
	}
	o.Console = NewDebugConsoleWindow(backend)
}

// SetScriptEditor attaches a ScriptEditorWindow backed by backend to the overlay.
func (o *ImGuiOverlay) SetScriptEditor(backend ScriptEditorBackend) {
	if backend == nil {
		o.ScriptEditor = nil
		return
	}
	o.ScriptEditor = NewScriptEditorWindow(backend)
}

// NewFrame starts a new Dear ImGui frame across OpenGL 3, GLFW, and ImGui core.
func (o *ImGuiOverlay) NewFrame() {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	if !o.headless && o.window != nil {
		io := imgui.CurrentIO()
		w, h := o.window.GetSize()
		fbW, fbH := o.window.GetFramebufferSize()
		if w > 0 && h > 0 {
			io.SetDisplaySize(imgui.NewVec2(float32(w), float32(h)))
			io.SetDisplayFramebufferScale(imgui.NewVec2(float32(fbW)/float32(w), float32(fbH)/float32(h)))
		}
		now := glfw.GetTime()
		dt := float32(now - o.lastTime)
		if dt <= 0 {
			dt = 1.0 / 60.0
		}
		o.lastTime = now
		io.SetDeltaTime(dt)

		opengl3.NewFrame()
	}
	imgui.NewFrame()
}

// DrawDebugWindows renders built-in metrics, optional ImGui demo window, the Lua console,
// and any custom DrawUI callback. Call between NewFrame() and Render().
func (o *ImGuiOverlay) DrawDebugWindows() {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)

	if o.ShowDemo {
		imgui.ShowDemoWindowV(&o.ShowDemo)
	}

	if o.ShowMetrics {
		imgui.SetNextWindowSizeV(imgui.NewVec2(280, 140), imgui.CondFirstUseEver)
		if imgui.BeginV("Engine Metrics", &o.ShowMetrics, imgui.WindowFlagsNone) {
			fps := o.MetricsFPS
			if fps <= 0 {
				fps = imgui.CurrentIO().Framerate()
			}
			var frameMs float32
			if fps > 0 {
				frameMs = 1000.0 / fps
			}
			imgui.TextUnformatted(fmt.Sprintf("FPS: %.1f (%.2f ms)", fps, frameMs))
			if o.SceneName != "" {
				imgui.TextUnformatted(fmt.Sprintf("Scene: %s (Objects: %d)", o.SceneName, o.ObjectCount))
			}
			imgui.TextUnformatted(fmt.Sprintf("WantCaptureMouse: %v", o.WantCaptureMouse()))
			imgui.TextUnformatted(fmt.Sprintf("WantCaptureKeyboard: %v", o.WantCaptureKeyboard()))
		}
		imgui.End()
	}

	if o.Console != nil {
		o.Console.Draw()
	}

	if o.ScriptEditor != nil {
		o.ScriptEditor.Draw()
	}

	if o.DrawUI != nil {
		o.DrawUI()
	}
}

// Render finalizes the ImGui frame and flushes draw lists into OpenGL 3 while
// backing up and restoring the host OpenGL pipeline state.
func (o *ImGuiOverlay) Render() {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	imgui.Render()

	dd := imgui.CurrentDrawData()
	if o.glAPI != nil {
		saved := BackupGLState(o.glAPI)
		defer RestoreGLState(o.glAPI, saved)
	}

	if o.headless {
		if o.onHeadlessDraw != nil {
			o.onHeadlessDraw(dd)
		}
		return
	}

	if dd != nil && dd.Valid() {
		opengl3.RenderDrawData(dd)
	}
}

// Shutdown tears down the OpenGL 3 backend, GLFW hooks, and ImGui context.
func (o *ImGuiOverlay) Shutdown() {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)

	if !o.headless {
		if o.window != nil {
			o.restoreGLFWCallbacks(o.window)
		}
		opengl3.Shutdown()
	}

	if o.ctx != nil {
		imgui.DestroyContextV(o.ctx)
		o.ctx = nil
	}
	o.window = nil
	o.initialized = false
}

// WantCaptureMouse reports whether Dear ImGui currently owns mouse input.
func (o *ImGuiOverlay) WantCaptureMouse() bool {
	if !o.initialized {
		return false
	}
	imgui.SetCurrentContext(o.ctx)
	return imgui.CurrentIO().WantCaptureMouse()
}

// WantCaptureKeyboard reports whether Dear ImGui currently owns keyboard input.
func (o *ImGuiOverlay) WantCaptureKeyboard() bool {
	if !o.initialized {
		return false
	}
	imgui.SetCurrentContext(o.ctx)
	return imgui.CurrentIO().WantCaptureKeyboard()
}

func (o *ImGuiOverlay) installGLFWCallbacks(w *glfw.Window) {
	o.prevCursorPos = w.SetCursorPosCallback(func(_ *glfw.Window, xpos, ypos float64) {
		o.HandleCursorPos(xpos, ypos)
		if o.prevCursorPos != nil && !o.WantCaptureMouse() {
			o.prevCursorPos(w, xpos, ypos)
		}
	})
	o.prevMouseButton = w.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		o.HandleMouseButton(button, action, mods)
		if o.prevMouseButton != nil && !o.WantCaptureMouse() {
			o.prevMouseButton(w, button, action, mods)
		}
	})
	o.prevScroll = w.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		o.HandleScroll(xoff, yoff)
		if o.prevScroll != nil && !o.WantCaptureMouse() {
			o.prevScroll(w, xoff, yoff)
		}
	})
	o.prevKey = w.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		toggled := o.HandleKey(key, scancode, action, mods)
		if !toggled && o.prevKey != nil && !o.WantCaptureKeyboard() {
			o.prevKey(w, key, scancode, action, mods)
		}
	})
	o.prevChar = w.SetCharCallback(func(_ *glfw.Window, char rune) {
		o.HandleChar(char)
		if o.prevChar != nil && !o.WantCaptureKeyboard() {
			o.prevChar(w, char)
		}
	})
	o.prevFocus = w.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		o.HandleFocus(focused)
		if o.prevFocus != nil {
			o.prevFocus(w, focused)
		}
	})
	o.prevCursorEnter = w.SetCursorEnterCallback(func(_ *glfw.Window, entered bool) {
		o.HandleCursorEnter(entered)
		if o.prevCursorEnter != nil {
			o.prevCursorEnter(w, entered)
		}
	})
}

func (o *ImGuiOverlay) restoreGLFWCallbacks(w *glfw.Window) {
	w.SetCursorPosCallback(o.prevCursorPos)
	w.SetMouseButtonCallback(o.prevMouseButton)
	w.SetScrollCallback(o.prevScroll)
	w.SetKeyCallback(o.prevKey)
	w.SetCharCallback(o.prevChar)
	w.SetFocusCallback(o.prevFocus)
	w.SetCursorEnterCallback(o.prevCursorEnter)
}

// HandleCursorPos dispatches cursor movement to ImGui and forwards to OnGameCursorPos
// only when WantCaptureMouse is false.
func (o *ImGuiOverlay) HandleCursorPos(x, y float64) {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	imgui.CurrentIO().AddMousePosEvent(float32(x), float32(y))
	if !o.WantCaptureMouse() && o.OnGameCursorPos != nil {
		o.OnGameCursorPos(x, y)
	}
}

// HandleMouseButton dispatches mouse button events to ImGui and forwards to OnGameMouseButton
// only when WantCaptureMouse is false.
func (o *ImGuiOverlay) HandleMouseButton(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	imgui.CurrentIO().AddMouseButtonEvent(int32(button), action != glfw.Release)
	if !o.WantCaptureMouse() && o.OnGameMouseButton != nil {
		o.OnGameMouseButton(button, action, mods)
	}
}

// HandleScroll dispatches mouse wheel events to ImGui and forwards to OnGameScroll
// only when WantCaptureMouse is false.
func (o *ImGuiOverlay) HandleScroll(xoff, yoff float64) {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	imgui.CurrentIO().AddMouseWheelEvent(float32(xoff), float32(yoff))
	if !o.WantCaptureMouse() && o.OnGameScroll != nil {
		o.OnGameScroll(xoff, yoff)
	}
}

// HandleKey dispatches keyboard events to the debug console toggle and ImGui,
// forwarding to OnGameKey only when WantCaptureKeyboard is false.
func (o *ImGuiOverlay) HandleKey(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
	if !o.initialized {
		return false
	}
	imgui.SetCurrentContext(o.ctx)
	if o.Console != nil && o.Console.HandleToggleKey(key, action) {
		return true
	}
	io := imgui.CurrentIO()
	io.AddKeyEvent(imgui.ModCtrl, mods&glfw.ModControl != 0)
	io.AddKeyEvent(imgui.ModShift, mods&glfw.ModShift != 0)
	io.AddKeyEvent(imgui.ModAlt, mods&glfw.ModAlt != 0)
	io.AddKeyEvent(imgui.ModSuper, mods&glfw.ModSuper != 0)
	io.AddKeyEvent(glfwKeyToImGuiKey(key), action != glfw.Release)
	if !o.WantCaptureKeyboard() && o.OnGameKey != nil {
		o.OnGameKey(key, scancode, action, mods)
	}
	return false
}

// HandleChar dispatches UTF-8 character input to ImGui and forwards to OnGameChar
// only when WantCaptureKeyboard is false.
func (o *ImGuiOverlay) HandleChar(char rune) {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	if o.Console != nil && (char == '`' || char == '~') && glfw.Key(char) == o.Console.ToggleKey {
		return
	}
	imgui.CurrentIO().AddInputCharacter(uint32(char))
	if !o.WantCaptureKeyboard() && o.OnGameChar != nil {
		o.OnGameChar(char)
	}
}

// HandleFocus dispatches window focus events to ImGui.
func (o *ImGuiOverlay) HandleFocus(focused bool) {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	imgui.CurrentIO().AddFocusEvent(focused)
}

// HandleCursorEnter dispatches cursor enter/leave events to ImGui.
func (o *ImGuiOverlay) HandleCursorEnter(entered bool) {
	if !o.initialized {
		return
	}
	imgui.SetCurrentContext(o.ctx)
	if !entered {
		imgui.CurrentIO().AddMousePosEvent(-1, -1)
	}
}

func glfwKeyToImGuiKey(key glfw.Key) imgui.Key {
	if key >= glfw.KeyA && key <= glfw.KeyZ {
		return imgui.KeyA + imgui.Key(key-glfw.KeyA)
	}
	if key >= glfw.Key0 && key <= glfw.Key9 {
		return imgui.Key0 + imgui.Key(key-glfw.Key0)
	}
	switch key {
	case glfw.KeyTab:
		return imgui.KeyTab
	case glfw.KeyLeft:
		return imgui.KeyLeftArrow
	case glfw.KeyRight:
		return imgui.KeyRightArrow
	case glfw.KeyUp:
		return imgui.KeyUpArrow
	case glfw.KeyDown:
		return imgui.KeyDownArrow
	case glfw.KeyPageUp:
		return imgui.KeyPageUp
	case glfw.KeyPageDown:
		return imgui.KeyPageDown
	case glfw.KeyHome:
		return imgui.KeyHome
	case glfw.KeyEnd:
		return imgui.KeyEnd
	case glfw.KeyInsert:
		return imgui.KeyInsert
	case glfw.KeyDelete:
		return imgui.KeyDelete
	case glfw.KeyBackspace:
		return imgui.KeyBackspace
	case glfw.KeySpace:
		return imgui.KeySpace
	case glfw.KeyEnter:
		return imgui.KeyEnter
	case glfw.KeyEscape:
		return imgui.KeyEscape
	case glfw.KeyLeftControl:
		return imgui.KeyLeftCtrl
	case glfw.KeyRightControl:
		return imgui.KeyRightCtrl
	case glfw.KeyLeftShift:
		return imgui.KeyLeftShift
	case glfw.KeyRightShift:
		return imgui.KeyRightShift
	case glfw.KeyLeftAlt:
		return imgui.KeyLeftAlt
	case glfw.KeyRightAlt:
		return imgui.KeyRightAlt
	case glfw.KeyGraveAccent:
		return imgui.KeyGraveAccent
	default:
		return imgui.KeyNone
	}
}
