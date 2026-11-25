package luabridge

import (
	"strings"
	"time"
)

// LineType classifies console messages for formatting and styling.
type LineType int

const (
	LineInput LineType = iota
	LineOutput
	LineInfo
	LineError
	LineSystem
)

// ConsoleLine represents a single output line in the console buffer.
type ConsoleLine struct {
	Text      string
	Type      LineType
	Timestamp string
}

// Console manages an interactive Quake-style dropdown developer console.
type Console struct {
	Bridge       *LuaBridge
	IsOpen       bool
	InputBuffer  []rune
	CursorPos    int
	History      []string
	HistoryIndex int
	Output       []ConsoleLine
	MaxLines     int
	ScrollOffset int
}

// NewConsole initializes a developer console backed by the LuaBridge.
func NewConsole(bridge *LuaBridge) *Console {
	if bridge == nil {
		bridge = NewLuaBridge()
	}
	c := &Console{
		Bridge:       bridge,
		IsOpen:       false,
		InputBuffer:  make([]rune, 0, 128),
		CursorPos:    0,
		History:      make([]string, 0, 64),
		HistoryIndex: -1,
		Output:       make([]ConsoleLine, 0, 256),
		MaxLines:     500,
		ScrollOffset: 0,
	}

	c.Print("Project Engine Developer Console Initialized. Type 'help' or Lua commands.", LineSystem)
	return c
}

// Print appends a new message line to the console.
func (c *Console) Print(text string, lineType LineType) {
	ts := time.Now().Format("15:04:05")
	c.Output = append(c.Output, ConsoleLine{
		Text:      text,
		Type:      lineType,
		Timestamp: ts,
	})

	if len(c.Output) > c.MaxLines {
		c.Output = c.Output[len(c.Output)-c.MaxLines:]
	}
	c.ScrollOffset = 0
}

// SubmitInput executes the current input buffer, records history, and returns the response.
func (c *Console) SubmitInput() string {
	raw := string(c.InputBuffer)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	c.Print("> "+raw, LineInput)
	c.History = append(c.History, raw)
	c.HistoryIndex = -1
	c.InputBuffer = c.InputBuffer[:0]
	c.CursorPos = 0

	out, err := c.Bridge.Execute(trimmed)
	if err != nil {
		c.Print("Error: "+err.Error(), LineError)
		return "Error: " + err.Error()
	}
	if out != "" {
		c.Print(out, LineOutput)
	}
	return out
}

// Toggle toggles the open/closed visibility of the console.
func (c *Console) Toggle() {
	c.IsOpen = !c.IsOpen
}
