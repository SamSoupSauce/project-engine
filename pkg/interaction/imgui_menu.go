package interaction

import (
	"fmt"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

// RadialSlot describes the 2D screen anchor computed for a context menu option.
type RadialSlot struct {
	Index  int
	Option spatial.InteractionOption
	PosX   float32
	PosY   float32
}

// ComputeRadialLayout arranges N context menu options evenly around the screen center.
func ComputeRadialLayout(centerX, centerY, radius float32, options []spatial.InteractionOption) []RadialSlot {
	n := len(options)
	if n == 0 {
		return nil
	}
	if radius <= 0 {
		radius = 96
	}
	slots := make([]RadialSlot, n)
	step := (2.0 * math.Pi) / float64(n)
	start := -math.Pi / 2.0
	for i := 0; i < n; i++ {
		angle := start + float64(i)*step
		slots[i] = RadialSlot{
			Index:  i,
			Option: options[i],
			PosX:   centerX + float32(math.Cos(angle))*radius,
			PosY:   centerY + float32(math.Sin(angle))*radius,
		}
	}
	return slots
}

// DrawImGui renders the reticle hover prompt and, when State.IsMenuOpen is true,
// the Dear ImGui contextual/radial popup menu for secondary verbs.
func (c *Controller) DrawImGui(viewportW, viewportH float32) {
	if c == nil {
		return
	}
	if viewportW <= 0 {
		viewportW = 1280
	}
	if viewportH <= 0 {
		viewportH = 720
	}

	cx := viewportW * 0.5
	cy := viewportH * 0.5

	// 1. Reticle hover prompt badge when hovering an interactive entity and menu is closed.
	if c.State.HoveredComp != nil && !c.State.IsMenuOpen {
		c.drawHoverPrompt(cx, cy)
	}

	// 2. Held-prop minimal throw prompt.
	if c.State.ActiveHeldProp != nil && !c.State.IsMenuOpen {
		c.drawHeldPrompt(cx, cy)
	}

	// 3. Long-press radial / contextual popup menu.
	if c.State.IsMenuOpen && c.State.MenuTarget != nil && len(c.State.MenuOptions) > 0 {
		c.drawContextMenuWindow(cx, cy)
	}
}

func (c *Controller) drawHoverPrompt(cx, cy float32) {
	comp := c.State.HoveredComp
	if comp == nil {
		return
	}
	actionLabel := comp.DefaultVerb.Label
	if actionLabel == "" {
		actionLabel = "Interact"
	}
	name := comp.DisplayName
	if name == "" && c.State.HoveredEntity != nil {
		name = c.State.HoveredEntity.Name
	}

	imgui.SetNextWindowPosV(imgui.NewVec2(cx, cy+28), imgui.CondAlways, imgui.NewVec2(0.5, 0.0))
	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoMove | imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoInputs
	if imgui.BeginV("##InteractHoverPrompt", nil, flags) {
		imgui.Text(fmt.Sprintf("[E] %s %s", actionLabel, name))
		if len(comp.ContextVerbs) > 0 {
			imgui.TextDisabled("(Hold for Options)")
		}
	}
	imgui.End()
}

func (c *Controller) drawHeldPrompt(cx, cy float32) {
	imgui.SetNextWindowPosV(imgui.NewVec2(cx, cy+28), imgui.CondAlways, imgui.NewVec2(0.5, 0.0))
	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoMove | imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoInputs
	if imgui.BeginV("##InteractHeldPrompt", nil, flags) {
		imgui.Text(fmt.Sprintf("[Click] Throw %s", c.State.ActiveHeldProp.Name))
	}
	imgui.End()
}

func (c *Controller) drawContextMenuWindow(cx, cy float32) {
	title := ""
	if c.State.MenuTarget != nil {
		if ic := spatial.FindInteractiveComponent(c.State.MenuTarget); ic != nil && ic.DisplayName != "" {
			title = ic.DisplayName
		} else {
			title = c.State.MenuTarget.Name
		}
	}

	imgui.SetNextWindowPosV(imgui.NewVec2(cx+24, cy), imgui.CondAlways, imgui.NewVec2(0.0, 0.5))
	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoMove | imgui.WindowFlagsNoSavedSettings
	if imgui.BeginV("##InteractContextMenu", nil, flags) {
		if title != "" {
			imgui.TextDisabled(title)
			imgui.Separator()
		}

		for i, opt := range c.State.MenuOptions {
			selected := c.State.SelectedOptionIndex == i
			label := opt.Label
			if label == "" {
				label = fmt.Sprintf("Action %d", i+1)
			}
			if imgui.SelectableBoolV(label, selected, imgui.SelectableFlagsNone, imgui.NewVec2(160, 0)) {
				c.SetHoveredMenuOption(i)
				_, _ = c.SelectMenuOption(i)
			}
			if imgui.IsItemHovered() {
				c.SetHoveredMenuOption(i)
			}
		}
	}
	imgui.End()
}
