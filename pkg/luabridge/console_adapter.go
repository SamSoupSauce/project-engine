package luabridge

import (
	"strings"

	"github.com/SamSoupSauce/project-engine/engine"
)

// ExecuteCommand submits an input string to the underlying LuaBridge and appends
// both the echoed input and output/error lines to the console buffer.
func (c *Console) ExecuteCommand(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	c.mu.Lock()
	c.InputBuffer = []rune(trimmed)
	c.mu.Unlock()
	return c.SubmitInput()
}

// GetOutputLines returns a snapshot of console output lines in engine.DebugConsoleLine format.
func (c *Console) GetOutputLines() []engine.DebugConsoleLine {
	c.mu.RLock()
	defer c.mu.RUnlock()
	lines := make([]engine.DebugConsoleLine, len(c.Output))
	for i, l := range c.Output {
		lines[i] = engine.DebugConsoleLine{
			Text:      l.Text,
			Kind:      engine.DebugConsoleLineKind(l.Type),
			Timestamp: l.Timestamp,
		}
	}
	return lines
}

// ClearOutput clears the stored console output lines.
func (c *Console) ClearOutput() {
	c.mu.Lock()
	c.Output = c.Output[:0]
	c.ScrollOffset = 0
	c.mu.Unlock()
}

// IsConsoleOpen reports whether the console overlay is currently open.
func (c *Console) IsConsoleOpen() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.IsOpen
}

// SetConsoleOpen explicitly sets the console overlay visibility.
func (c *Console) SetConsoleOpen(open bool) {
	c.mu.Lock()
	c.IsOpen = open
	c.mu.Unlock()
}

// ToggleConsole toggles the console overlay visibility.
func (c *Console) ToggleConsole() {
	c.Toggle()
}

// AppendLog appends a message line using engine.DebugConsoleLineKind classification.
func (c *Console) AppendLog(text string, kind engine.DebugConsoleLineKind) {
	c.Print(text, LineType(kind))
}

// GetHistory returns a copy of the submitted command history.
func (c *Console) GetHistory() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.History))
	copy(out, c.History)
	return out
}
