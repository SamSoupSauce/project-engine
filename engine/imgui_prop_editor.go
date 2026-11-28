package engine

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
)

// PropBrowserBackend decouples PropBrowserWindow from pkg/luabridge.
type PropBrowserBackend interface {
	IsPropBrowserOpen() bool
	SetPropBrowserOpen(open bool)
	ListProps(query string) ([]PropBrowserSummary, error)
	ResolveProp(id string, seed int64) (PropBrowserResolution, error)
	PropTree(id string) ([]PropTreeNode, error)
	AddOption(id, path, pool, option, model string) error
	RemoveOption(id, path, pool, option string) error
	PinSelection(id, path, pool, option string) error
}

// PropBrowserSummary describes a prop manifest for the browser UI.
type PropBrowserSummary struct {
	ID   string
	Name string
	Path string
}

// PropBrowserLeaf describes one resolved leaf.
type PropBrowserLeaf struct {
	NodeID   string
	Model    string
	Material string
}

// PropBrowserResolution is the resolved output for preview.
type PropBrowserResolution struct {
	PropID    string
	LeafCount int
	Leaves    []PropBrowserLeaf
}

// PropTreeNode describes one row of the node/pool tree in the editor.
type PropTreeNode struct {
	ID          string
	PoolName    string
	OptionCount int
	Depth       int
}

// PropBrowserWindow renders the Prop Manager browser, tree, and resolution preview.
type PropBrowserWindow struct {
	Backend     PropBrowserBackend
	IsOpen      bool
	Title       string
	SearchQuery string
	Props       []PropBrowserSummary
	SelectedID  string
	Seed        int32
	Resolution  *PropBrowserResolution
	Tree        []PropTreeNode
	NewOptionID string
	NewModel    string
	StatusMsg   string
	StatusIsErr bool
	listLoaded  bool
}

// NewPropBrowserWindow creates a Prop Manager window bound to backend.
func NewPropBrowserWindow(backend PropBrowserBackend) *PropBrowserWindow {
	return &PropBrowserWindow{Backend: backend, Title: "Prop Manager"}
}

// Open shows the window and refreshes the prop list.
func (w *PropBrowserWindow) Open() error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prop browser backend is nil")
	}
	w.IsOpen = true
	w.Backend.SetPropBrowserOpen(true)
	return w.Refresh()
}

// Close hides the window.
func (w *PropBrowserWindow) Close() {
	if w == nil {
		return
	}
	w.IsOpen = false
	if w.Backend != nil {
		w.Backend.SetPropBrowserOpen(false)
	}
}

// Refresh reloads the prop list from the backend.
func (w *PropBrowserWindow) Refresh() error {
	if w == nil || w.Backend == nil {
		return nil
	}
	props, err := w.Backend.ListProps(w.SearchQuery)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.Props = props
	w.listLoaded = true
	return nil
}

// Resolve resolves the selected prop with the current seed for preview.
func (w *PropBrowserWindow) Resolve(id string, seed int64) (PropBrowserResolution, error) {
	if w == nil || w.Backend == nil {
		return PropBrowserResolution{}, fmt.Errorf("engine: prop browser backend is nil")
	}
	res, err := w.Backend.ResolveProp(id, seed)
	if err == nil {
		w.Resolution = &res
		w.StatusMsg = fmt.Sprintf("Resolved %s (%d leaves)", id, res.LeafCount)
		w.StatusIsErr = false
	} else {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
	}
	return res, err
}

// LoadTree fetches the node/pool tree for a prop.
func (w *PropBrowserWindow) LoadTree(id string) ([]PropTreeNode, error) {
	if w == nil || w.Backend == nil {
		return nil, fmt.Errorf("engine: prop browser backend is nil")
	}
	nodes, err := w.Backend.PropTree(id)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return nil, err
	}
	w.Tree = nodes
	return nodes, nil
}

// AddOption adds an option to a pool in the prop tree.
func (w *PropBrowserWindow) AddOption(id, path, pool, option, model string) error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prop browser backend is nil")
	}
	err := w.Backend.AddOption(id, path, pool, option, model)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.StatusMsg = fmt.Sprintf("Added option %s", option)
	w.StatusIsErr = false
	return nil
}

// RemoveOption removes an option from a pool in the prop tree.
func (w *PropBrowserWindow) RemoveOption(id, path, pool, option string) error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prop browser backend is nil")
	}
	err := w.Backend.RemoveOption(id, path, pool, option)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.StatusMsg = fmt.Sprintf("Removed option %s", option)
	w.StatusIsErr = false
	return nil
}

// PinSelection pins an option choice in the prop tree.
func (w *PropBrowserWindow) PinSelection(id, path, pool, option string) error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("engine: prop browser backend is nil")
	}
	err := w.Backend.PinSelection(id, path, pool, option)
	if err != nil {
		w.StatusMsg = err.Error()
		w.StatusIsErr = true
		return err
	}
	w.StatusMsg = fmt.Sprintf("Pinned %s", option)
	w.StatusIsErr = false
	return nil
}

// Draw renders the window. Call each frame from ImGuiOverlay.
func (w *PropBrowserWindow) Draw() {
	if w == nil || w.Backend == nil {
		return
	}
	if w.Backend.IsPropBrowserOpen() != w.IsOpen {
		w.IsOpen = w.Backend.IsPropBrowserOpen()
	}
	if !w.IsOpen {
		return
	}
	if !w.listLoaded {
		_ = w.Refresh()
	}

	open := true
	imgui.SetNextWindowSizeV(imgui.NewVec2(1000, 620), imgui.CondFirstUseEver)
	if !imgui.BeginV("Prop Manager###PropManagerWindow", &open, imgui.WindowFlagsNone) {
		imgui.End()
		return
	}
	if !open {
		w.Close()
		imgui.End()
		return
	}

	avail := imgui.ContentRegionAvail()
	leftW := float32(230)
	if avail.X < 600 {
		leftW = avail.X * 0.35
	}

	// Left: prop list
	imgui.BeginChildStrV("##PropBrowserListPane", imgui.NewVec2(leftW, 0), imgui.ChildFlagsBorders, imgui.WindowFlagsNone)
	imgui.TextUnformatted("Props")
	imgui.InputTextWithHint("##PropBrowserSearch", "Search props...", &w.SearchQuery, imgui.InputTextFlagsNone, nil)
	if imgui.Button("Refresh") {
		_ = w.Refresh()
	}
	imgui.BeginChildStrV("##PropBrowserListScroll", imgui.NewVec2(0, 0), imgui.ChildFlagsNone, imgui.WindowFlagsNone)
	for i := range w.Props {
		p := &w.Props[i]
		label := fmt.Sprintf("%s##prop_%d", p.ID, i)
		if imgui.SelectableBoolV(label, w.SelectedID == p.ID, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
			w.SelectedID = p.ID
			w.Resolution = nil
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip(p.Path)
		}
	}
	if len(w.Props) == 0 {
		imgui.TextDisabled("(no *.prop.json manifests found)")
	}
	imgui.EndChild()
	imgui.EndChild()

	imgui.SameLine()

	// Right: node/pool tree placeholder + resolution preview
	imgui.BeginChildStrV("##PropBrowserDetailPane", imgui.NewVec2(0, 0), imgui.ChildFlagsBorders, imgui.WindowFlagsNone)
	if w.SelectedID == "" {
		imgui.TextColored(imgui.NewVec4(0.7, 0.85, 1.0, 1.0), "Select a prop on the left.")
	} else {
		imgui.TextColored(imgui.NewVec4(0.7, 0.85, 1.0, 1.0), "Prop: "+w.SelectedID)
		imgui.TextUnformatted("Node / Pool Tree")
		if len(w.Tree) == 0 && w.SelectedID != "" {
			_, _ = w.LoadTree(w.SelectedID)
		}
		imgui.BeginChildStrV("##PropTreeScroll", imgui.NewVec2(0, 180), imgui.ChildFlagsBorders, imgui.WindowFlagsNone)
		for i := range w.Tree {
			n := &w.Tree[i]
			indent := ""
			for d := 0; d < n.Depth; d++ { indent += "  " }
			imgui.TextUnformatted(fmt.Sprintf("%s%s [pool:%s, %d options]", indent, n.ID, n.PoolName, n.OptionCount))
		}
		imgui.EndChild()
		imgui.TextUnformatted("Add Option")
		imgui.InputTextWithHint("##PropNewOption", "option id", &w.NewOptionID, imgui.InputTextFlagsNone, nil)
		imgui.InputTextWithHint("##PropNewModel", "model path", &w.NewModel, imgui.InputTextFlagsNone, nil)
		if imgui.Button("Add") && w.NewOptionID != "" {
			if err := w.AddOption(w.SelectedID, "root", "type", w.NewOptionID, w.NewModel); err != nil {
				w.StatusMsg = err.Error()
				w.StatusIsErr = true
			}
			w.NewOptionID = ""
			w.NewModel = ""
		}
		imgui.Separator()
		imgui.TextUnformatted("Resolution Preview")
		imgui.SliderIntV("Seed", &w.Seed, 0, 100000, "%d", imgui.SliderFlagsNone)
		if imgui.Button("Resolve Variation") {
			_, _ = w.Resolve(w.SelectedID, int64(w.Seed))
		}
		if w.Resolution != nil {
			imgui.TextUnformatted(fmt.Sprintf("%d leaves:", w.Resolution.LeafCount))
			imgui.BeginChildStrV("##PropResolutionLeaves", imgui.NewVec2(0, 140), imgui.ChildFlagsBorders, imgui.WindowFlagsNone)
			for _, leaf := range w.Resolution.Leaves {
				imgui.TextUnformatted(fmt.Sprintf("  %s -> %s", leaf.NodeID, leaf.Model))
			}
			imgui.EndChild()
		}
	}
	imgui.Separator()
	if w.StatusIsErr {
		imgui.TextColored(imgui.NewVec4(1.0, 0.42, 0.42, 1.0), w.StatusMsg)
	} else if strings.TrimSpace(w.StatusMsg) != "" {
		imgui.TextColored(imgui.NewVec4(0.55, 0.95, 0.65, 1.0), w.StatusMsg)
	} else {
		imgui.TextDisabled("Ready")
	}
	imgui.End()
}
