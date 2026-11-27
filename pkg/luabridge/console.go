package luabridge

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/SamSoupSauce/project-engine/engine"
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
	Scene        *engine.BaseScene
	Physics      engine.PhysicsWorldHandle
	PickedObject *engine.GameObject
	PickMode     bool
	IsOpen       bool
	InputBuffer  []rune
	CursorPos    int
	History      []string
	HistoryIndex int
	Output       []ConsoleLine
	MaxLines     int
	ScrollOffset int
	TeeStdio     bool

	mu          sync.RWMutex
	stdioMu     sync.Mutex
	stdioActive bool
	origStdout  interface{}
	origStderr  interface{}
	origLogOut  io.Writer
	outPipeW    interface{}
	errPipeW    interface{}
	stdioWG     sync.WaitGroup
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
		TeeStdio:     true,
	}
	bridge.AttachConsole(c)

	c.Print("Project Engine Developer Console Initialized. Type 'help()' or Lua commands.", LineSystem)
	return c
}

// Print appends message lines (splitting multi-line text) to the console safely across goroutines.
func (c *Console) Print(text string, lineType LineType) {
	ts := time.Now().Format("15:04:05")
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	c.mu.Lock()
	for _, line := range parts {
		c.Output = append(c.Output, ConsoleLine{
			Text:      line,
			Type:      lineType,
			Timestamp: ts,
		})
	}
	if len(c.Output) > c.MaxLines && c.MaxLines > 0 {
		c.Output = c.Output[len(c.Output)-c.MaxLines:]
	}
	c.ScrollOffset = 0
	c.mu.Unlock()
}

// SubmitInput executes the current input buffer, records history, and returns the response.
func (c *Console) SubmitInput() string {
	c.mu.Lock()
	raw := string(c.InputBuffer)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		c.mu.Unlock()
		return ""
	}
	c.History = append(c.History, raw)
	c.HistoryIndex = -1
	c.InputBuffer = c.InputBuffer[:0]
	c.CursorPos = 0
	c.mu.Unlock()

	c.Print("> "+raw, LineInput)

	out, streamed, err := c.Bridge.executeWithStreamFlag(trimmed)
	if err != nil {
		c.Print("Error: "+err.Error(), LineError)
		return "Error: " + err.Error()
	}
	if out != "" && !streamed {
		c.Print(out, LineOutput)
	}
	return out
}

// Toggle toggles the open/closed visibility of the console.
func (c *Console) Toggle() {
	c.mu.Lock()
	c.IsOpen = !c.IsOpen
	c.mu.Unlock()
}
