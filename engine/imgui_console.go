package engine

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// DebugConsoleLineKind classifies console log entries for UI color-coding.
type DebugConsoleLineKind int

const (
	ConsoleLineInput DebugConsoleLineKind = iota
	ConsoleLineOutput
	ConsoleLineInfo
	ConsoleLineError
	ConsoleLineSystem
)

// DebugConsoleLine represents a single rendered line in the debug console overlay.
type DebugConsoleLine struct {
	Text      string
	Kind      DebugConsoleLineKind
	Timestamp string
}

// DebugConsoleBackend decouples the engine ImGui console window from pkg/luabridge
// to prevent circular package imports.
type DebugConsoleBackend interface {
	ExecuteCommand(input string) string
	GetOutputLines() []DebugConsoleLine
	ClearOutput()
	IsConsoleOpen() bool
	SetConsoleOpen(open bool)
	ToggleConsole()
	AppendLog(text string, kind DebugConsoleLineKind)
	GetHistory() []string
}

// DebugConsoleWindow renders a toggleable Dear ImGui developer console backed by
// a DebugConsoleBackend (such as pkg/luabridge.Console).
type DebugConsoleWindow struct {
	Backend    DebugConsoleBackend
	Title      string
	ToggleKey  glfw.Key
	AutoScroll bool

	inputBuf       string
	historyIdx     int
	reclaimFocus   bool
	scrollToBottom bool
}

// NewDebugConsoleWindow creates a Dear ImGui developer console window bound to the given backend.
func NewDebugConsoleWindow(backend DebugConsoleBackend) *DebugConsoleWindow {
	return &DebugConsoleWindow{
		Backend:    backend,
		Title:      "Developer Console",
		ToggleKey:  glfw.KeyGraveAccent,
		AutoScroll: true,
		historyIdx: -1,
	}
}

// HandleToggleKey toggles console visibility when the configured key is pressed.
// Returns true if the toggle key handled the event.
func (c *DebugConsoleWindow) HandleToggleKey(key glfw.Key, action glfw.Action) bool {
	if c == nil || c.Backend == nil {
		return false
	}
	toggleKey := c.ToggleKey
	if toggleKey == 0 {
		toggleKey = glfw.KeyGraveAccent
	}
	if key == toggleKey && action == glfw.Press {
		c.Backend.ToggleConsole()
		if c.Backend.IsConsoleOpen() {
			c.reclaimFocus = true
		}
		return true
	}
	return false
}

// IsOpen reports whether the console window is currently open.
func (c *DebugConsoleWindow) IsOpen() bool {
	return c != nil && c.Backend != nil && c.Backend.IsConsoleOpen()
}

// Submit executes a command through the attached console backend and resets input state.
func (c *DebugConsoleWindow) Submit(command string) string {
	if c == nil || c.Backend == nil {
		return ""
	}
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ""
	}
	res := c.Backend.ExecuteCommand(trimmed)
	c.inputBuf = ""
	c.historyIdx = -1
	c.scrollToBottom = true
	c.reclaimFocus = true
	return res
}

// Draw renders the Dear ImGui console window when open.
func (c *DebugConsoleWindow) Draw() {
	if c == nil || c.Backend == nil || !c.Backend.IsConsoleOpen() {
		return
	}

	title := c.Title
	if title == "" {
		title = "Developer Console"
	}

	open := true
	imgui.SetNextWindowSizeV(imgui.NewVec2(640, 360), imgui.CondFirstUseEver)
	if imgui.BeginV(title, &open, imgui.WindowFlagsNone) {
		if imgui.Button("Clear") {
			c.Backend.ClearOutput()
		}
		imgui.SameLine()
		imgui.Checkbox("Auto-scroll", &c.AutoScroll)
		imgui.Separator()

		// Reserve space for separator and input bar at bottom
		footerHeight := imgui.FrameHeightWithSpacing() + imgui.CurrentStyle().ItemSpacing().Y
		if imgui.BeginChildStrV("##ConsoleScrollRegion", imgui.NewVec2(0, -footerHeight), imgui.ChildFlagsNone, imgui.WindowFlagsHorizontalScrollbar) {
			lines := c.Backend.GetOutputLines()
			for _, line := range lines {
				col := consoleLineColor(line.Kind)
				var formatted string
				if line.Timestamp != "" {
					formatted = fmt.Sprintf("[%s] %s", line.Timestamp, line.Text)
				} else {
					formatted = line.Text
				}
				imgui.TextColored(col, formatted)
			}
			if c.scrollToBottom || (c.AutoScroll && imgui.ScrollY() >= imgui.ScrollMaxY()) {
				imgui.SetScrollHereYV(1.0)
			}
			c.scrollToBottom = false
		}
		imgui.EndChild()

		imgui.Separator()

		if c.reclaimFocus {
			imgui.SetKeyboardFocusHere()
			c.reclaimFocus = false
		}

		flags := imgui.InputTextFlagsEnterReturnsTrue | imgui.InputTextFlagsCallbackHistory
		submitted := imgui.InputTextWithHint("##ConsoleInput", "Enter Lua command or 'help'...", &c.inputBuf, flags, c.historyCallback)
		imgui.SameLine()
		if imgui.Button("Run") {
			submitted = true
		}
		if submitted {
			c.Submit(c.inputBuf)
		}
	}
	imgui.End()

	if !open {
		c.Backend.SetConsoleOpen(false)
	}
}

func (c *DebugConsoleWindow) historyCallback(data imgui.InputTextCallbackData) int {
	if data.EventFlag() != imgui.InputTextFlagsCallbackHistory || c.Backend == nil {
		return 0
	}
	history := c.Backend.GetHistory()
	if len(history) == 0 {
		return 0
	}

	prevIdx := c.historyIdx
	switch data.EventKey() {
	case imgui.KeyUpArrow:
		if c.historyIdx == -1 {
			c.historyIdx = len(history) - 1
		} else if c.historyIdx > 0 {
			c.historyIdx--
		}
	case imgui.KeyDownArrow:
		if c.historyIdx != -1 {
			c.historyIdx++
			if c.historyIdx >= len(history) {
				c.historyIdx = -1
			}
		}
	}

	if prevIdx != c.historyIdx {
		replacement := ""
		if c.historyIdx >= 0 && c.historyIdx < len(history) {
			replacement = history[c.historyIdx]
		}
		data.DeleteChars(0, data.BufTextLen())
		data.InsertChars(0, replacement)
	}
	return 0
}

func consoleLineColor(kind DebugConsoleLineKind) imgui.Vec4 {
	switch kind {
	case ConsoleLineInput:
		return imgui.NewVec4(0.65, 0.85, 1.0, 1.0)
	case ConsoleLineError:
		return imgui.NewVec4(1.0, 0.4, 0.4, 1.0)
	case ConsoleLineSystem:
		return imgui.NewVec4(0.55, 0.95, 0.65, 1.0)
	case ConsoleLineInfo:
		return imgui.NewVec4(0.95, 0.9, 0.55, 1.0)
	default:
		return imgui.NewVec4(0.92, 0.92, 0.92, 1.0)
	}
}
