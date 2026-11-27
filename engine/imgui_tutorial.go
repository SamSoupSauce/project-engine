package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AllenDang/cimgui-go/imgui"
)

// TutorialState persists whether the first-run quick-start tutorial should be hidden on future launches.
type TutorialState struct {
	DoNotShowAgain bool `json:"do_not_show_again"`
}

// LoadTutorialState reads TutorialState from path. A missing file returns DoNotShowAgain: false with nil error.
func LoadTutorialState(path string) (TutorialState, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return TutorialState{}, nil
	}
	raw, err := os.ReadFile(trimmed)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TutorialState{}, nil
		}
		return TutorialState{}, fmt.Errorf("tutorial state read %q: %w", trimmed, err)
	}
	var st TutorialState
	if err := json.Unmarshal(raw, &st); err != nil {
		return TutorialState{}, fmt.Errorf("tutorial state parse %q: %w", trimmed, err)
	}
	return st, nil
}

// SaveTutorialState persists TutorialState to path, creating parent directories if needed.
func SaveTutorialState(path string, state TutorialState) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Errorf("tutorial state path must not be empty")
	}
	dir := filepath.Dir(trimmed)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("tutorial state mkdir %q: %w", dir, err)
		}
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("tutorial state marshal: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(trimmed, raw, 0o644); err != nil {
		return fmt.Errorf("tutorial state write %q: %w", trimmed, err)
	}
	return nil
}

// TutorialWindow renders the first-run Dear ImGui quick-start tutorial with a persisted
// "Do not show again" checkbox.
type TutorialWindow struct {
	mu             sync.RWMutex
	Open           bool
	DoNotShowAgain bool
	StatePath      string
	LastSaveErr    error
}

// NewTutorialWindow initializes a TutorialWindow from statePath, opening automatically on first run
// unless DoNotShowAgain was previously saved as true.
func NewTutorialWindow(statePath string) *TutorialWindow {
	st, err := LoadTutorialState(statePath)
	w := &TutorialWindow{
		Open:           !st.DoNotShowAgain,
		DoNotShowAgain: st.DoNotShowAgain,
		StatePath:      statePath,
		LastSaveErr:    err,
	}
	return w
}

// Show opens the tutorial window manually (e.g., via Help button or `tutorial` console command).
func (w *TutorialWindow) Show() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.Open = true
	w.mu.Unlock()
}

// IsOpen reports whether the tutorial window is currently visible.
func (w *TutorialWindow) IsOpen() bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Open
}

// SetDoNotShowAgain updates the preference and persists it to StatePath when configured.
func (w *TutorialWindow) SetDoNotShowAgain(val bool) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	w.DoNotShowAgain = val
	path := w.StatePath
	w.mu.Unlock()

	if strings.TrimSpace(path) == "" {
		return nil
	}
	err := SaveTutorialState(path, TutorialState{DoNotShowAgain: val})
	w.mu.Lock()
	w.LastSaveErr = err
	w.mu.Unlock()
	return err
}

// Draw renders the Dear ImGui first-run tutorial window when Open is true.
func (w *TutorialWindow) Draw() {
	if w == nil {
		return
	}
	w.mu.Lock()
	open := w.Open
	doNotShow := w.DoNotShowAgain
	saveErr := w.LastSaveErr
	w.mu.Unlock()

	if !open {
		return
	}

	imgui.SetNextWindowSizeV(imgui.NewVec2(620, 460), imgui.CondFirstUseEver)
	imgui.SetNextWindowPosV(imgui.NewVec2(330, 110), imgui.CondFirstUseEver, imgui.NewVec2(0, 0))
	if imgui.BeginV("Welcome to Project Engine - Quick Start Tutorial", &open, imgui.WindowFlagsNone) {
		imgui.TextColored(imgui.NewVec4(0.45, 0.88, 1.0, 1.0), "Project Engine Quick Start Guide")
		imgui.Separator()
		imgui.TextWrapped("Welcome! This kitchen-sink demo showcases 3D glTF props, TMX 3D tile maps, Jolt physics & fracture hazards, 2D HUD overlays, Lua custom components, and live agent introspection.")

		imgui.Spacing()
		imgui.TextColored(imgui.NewVec4(1.0, 0.85, 0.4, 1.0), "1. Viewport & Controls")
		imgui.BulletText("[L] Toggle Cursor Lock + Mouse Orbit | [F] Toggle Fullscreen")
		imgui.BulletText("[~] Toggle Developer Console & Script/Prefab Tools")
		imgui.BulletText("[Left Click] Ray-pick 3D objects in the scene (binds Lua `selected`)")
		imgui.BulletText("[Esc] Unlock cursor or exit application")

		imgui.Spacing()
		imgui.TextColored(imgui.NewVec4(1.0, 0.85, 0.4, 1.0), "2. Kitchen Sink Scene")
		imgui.BulletText("Tile Map (assets/maps/kitchen_sink.tmx): floors, walls, corners, pillars & stairs")
		imgui.BulletText("Props: Rainbow Cube, Kitchen Toaster, Ceramic Lamp (fracturable), Nightstand Drawer")
		imgui.BulletText("Use the Demo Controls panel to trigger physics impulses, fracture/reset props, or switch camera presets")

		imgui.Spacing()
		imgui.TextColored(imgui.NewVec4(1.0, 0.85, 0.4, 1.0), "3. Developer Console & Live Authoring")
		imgui.BulletText("Run `help` or `docs` in the [~] console to inspect live commands & ECS components")
		imgui.BulletText("Run `scripts.list`, `scripts.run <path>`, `prefabs`, or `edit(\"Rotator\")` to hot-reload Lua")

		imgui.Spacing()
		imgui.TextColored(imgui.NewVec4(1.0, 0.85, 0.4, 1.0), "4. Custom Game Roots & Agent Bridge")
		imgui.BulletText("Launch with `--root <dir>` / `--config <file>` (or `--init-dirs`) to load any project root")
		imgui.BulletText("Launch with `--serve --token=\"\"` (and optional `--tunnel`) for external AI agents")

		imgui.Spacing()
		imgui.Separator()

		prev := doNotShow
		if imgui.Checkbox("Do not show again", &doNotShow) && doNotShow != prev {
			_ = w.SetDoNotShowAgain(doNotShow)
		}
		imgui.SameLine()
		if imgui.Button("Got it (Close)") {
			open = false
			if doNotShow != prev {
				_ = w.SetDoNotShowAgain(doNotShow)
			}
		}
		if saveErr != nil {
			imgui.TextColored(imgui.NewVec4(1.0, 0.45, 0.45, 1.0), fmt.Sprintf("Tutorial state warning: %v", saveErr))
		}
	}
	imgui.End()

	w.mu.Lock()
	w.Open = open
	w.DoNotShowAgain = doNotShow
	w.mu.Unlock()
}
