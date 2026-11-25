package ui

import (
	"testing"

	"github.com/AllenDang/cimgui-go/imgui"
)

func TestComputeLayoutNestedFlexRowAndColumn(t *testing.T) {
	root := &UINode{
		ID: "root",
		Style: LayoutStyle{
			Direction: FlexRow,
			Align:     AlignStretch,
			Gap:       10,
			Padding:   [4]float32{20, 20, 20, 20},
		},
		Dirty: true,
	}

	sidebar := &UINode{
		ID: "sidebar",
		Style: LayoutStyle{
			Direction: FlexColumn,
			Align:     AlignCenter,
			Width:     200,
			Gap:       8,
			Padding:   [4]float32{10, 10, 10, 10},
		},
		Dirty: true,
	}
	btn1 := &UINode{ID: "btn1", Style: LayoutStyle{Width: 120, Height: 40}, Dirty: true}
	btn2 := &UINode{ID: "btn2", Style: LayoutStyle{Width: 120, Grow: 1}, Dirty: true}
	sidebar.AddChild(btn1)
	sidebar.AddChild(btn2)

	content := &UINode{
		ID: "content",
		Style: LayoutStyle{
			Direction: FlexColumn,
			Grow:      1,
		},
		Dirty: true,
	}

	root.AddChild(sidebar)
	root.AddChild(content)

	ComputeLayout(root, 1040, 640)

	// Root inner: 1000 x 600 starting at (20, 20).
	// Sidebar: x=20, y=20, w=200, h=600.
	// Content: x=20+200+10=230, y=20, w=1000-200-10=790, h=600.
	if sidebar.Bounds != [4]float32{20, 20, 200, 600} {
		t.Fatalf("unexpected sidebar bounds: %v", sidebar.Bounds)
	}
	if content.Bounds != [4]float32{230, 20, 790, 600} {
		t.Fatalf("unexpected content bounds: %v", content.Bounds)
	}

	// Sidebar inner: 180 x 580 starting at (30, 30).
	// btn1 centered horizontally: x = 30 + (180-120)/2 = 60, y=30, w=120, h=40.
	// btn2 grows into remaining height: 580 - 40 - 8 = 532, y=30+40+8=78.
	if btn1.Bounds != [4]float32{60, 30, 120, 40} {
		t.Fatalf("unexpected btn1 bounds: %v", btn1.Bounds)
	}
	if btn2.Bounds != [4]float32{60, 78, 120, 532} {
		t.Fatalf("unexpected btn2 bounds: %v", btn2.Bounds)
	}
}

func TestComputeLayoutZeroAllocsOnCleanFrames(t *testing.T) {
	root := &UINode{
		ID:    "root",
		Style: LayoutStyle{Direction: FlexRow, Gap: 4, Padding: [4]float32{8, 8, 8, 8}},
		Dirty: true,
	}
	for i := 0; i < 8; i++ {
		root.AddChild(&UINode{
			ID:    "item",
			Style: LayoutStyle{Width: 50, Grow: 1},
			Dirty: true,
		})
	}
	// Warm layout pass
	ComputeLayout(root, 1280, 720)

	allocs := testing.AllocsPerRun(200, func() {
		ComputeLayout(root, 1280, 720)
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocations on clean frames, got %v", allocs)
	}

	// Marking one child dirty recomputes its subtree accurately.
	root.Children[0].Style.Width = 100
	root.Children[0].MarkDirty()
	ComputeLayout(root, 1280, 720)
	if root.Children[0].Bounds[2] <= 100 {
		t.Errorf("expected dirty child width update to recompute, got %v", root.Children[0].Bounds)
	}
}

func TestJSONLoaderHitTestAndEventBubbling(t *testing.T) {
	jsonSrc := []byte(`{
		"id": "hud_root",
		"style": { "direction": "row", "padding": 16, "gap": 12, "color": "#101820FF" },
		"on_click": "on_root_click",
		"children": [
			{
				"id": "panel_left",
				"style": { "direction": "column", "width": 240, "padding": [8, 8, 8, 8], "color": "#203040FF", "rounding": 4 },
				"on_hover": "on_panel_hover",
				"children": [
					{
						"id": "play_btn",
						"text": "Play",
						"style": { "height": 48, "color": "#33AA55FF" },
						"on_click": "on_play_click"
					}
				]
			}
		]
	}`)

	root, err := LoadUIJSON(jsonSrc)
	if err != nil {
		t.Fatalf("LoadUIJSON failed: %v", err)
	}
	ComputeLayout(root, 1280, 720)

	// play_btn is at x=16+8=24, y=16+8=24, w=240-16=224, h=48
	playBtn := FindNodeByID(root, "play_btn")
	if playBtn == nil {
		t.Fatalf("expected play_btn node")
	}
	if playBtn.Bounds != [4]float32{24, 24, 224, 48} {
		t.Fatalf("unexpected play_btn bounds: %v", playBtn.Bounds)
	}

	var dispatched []string
	dispatcher := &PointerEventDispatcher{
		OnDispatch: func(handlerName, nodeID string, mx, my float32) bool {
			dispatched = append(dispatched, handlerName+"@"+nodeID)
			return handlerName == "on_play_click"
		},
	}

	hit, handled := dispatcher.DispatchClick(root, 50, 40)
	if !handled || hit != playBtn {
		t.Fatalf("expected click on play_btn to be handled, got hit=%v handled=%v", hit, handled)
	}
	if len(dispatched) != 1 || dispatched[0] != "on_play_click@play_btn" {
		t.Fatalf("unexpected dispatched click handlers: %v", dispatched)
	}

	// Hovering over play_btn has no on_hover on play_btn, so it bubbles up to panel_left!
	dispatched = nil
	hit, handled = dispatcher.DispatchHover(root, 50, 40)
	if hit != playBtn || len(dispatched) != 1 || dispatched[0] != "on_panel_hover@panel_left" {
		t.Fatalf("expected hover to bubble to panel_left, got hit=%v dispatched=%v", hit, dispatched)
	}
}

func TestPaintTreeToImDrawList(t *testing.T) {
	ctx := imgui.CreateContext()
	defer imgui.DestroyContextV(ctx)
	io := imgui.CurrentIO()
	io.SetBackendFlags(imgui.BackendFlagsRendererHasTextures)
	io.SetDisplaySize(imgui.NewVec2(800, 600))
	io.SetDeltaTime(1.0 / 60.0)
	io.Fonts().AddFontDefault()
	imgui.InternalImFontAtlasBuildMain(io.Fonts())

	root, err := LoadUIJSON([]byte(`{
		"id": "panel",
		"style": { "color": "#224466FF", "rounding": 6, "padding": 10 },
		"children": [
			{ "id": "lbl", "text": "HP: 100", "style": { "width": 120, "height": 30, "color": "#CC3333FF" } }
		]
	}`))
	if err != nil {
		t.Fatalf("LoadUIJSON failed: %v", err)
	}
	ComputeLayout(root, 800, 600)

	imgui.NewFrame()
	dl := imgui.ForegroundDrawListViewportPtr()
	PaintTree(dl, root)
	imgui.Render()

	dd := imgui.CurrentDrawData()
	if dd == nil || !dd.Valid() || dd.TotalVtxCount() <= 0 {
		t.Fatalf("expected non-empty ImDrawData after PaintTree")
	}
}
