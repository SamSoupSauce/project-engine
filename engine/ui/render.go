package ui

import "github.com/AllenDang/cimgui-go/imgui"

// DrawListRenderer paints a retained UINode hierarchy directly into an ImDrawList
// without heap allocations on clean layout frames.
func PaintTree(drawList *imgui.DrawList, root *UINode) {
	if drawList == nil || root == nil {
		return
	}
	paintNode(drawList, root)
}

func paintNode(dl *imgui.DrawList, node *UINode) {
	if node == nil {
		return
	}
	minPt := imgui.NewVec2(node.Bounds[0], node.Bounds[1])
	maxPt := imgui.NewVec2(node.Bounds[0]+node.Bounds[2], node.Bounds[1]+node.Bounds[3])

	if node.Style.Color != 0 {
		if node.Style.Rounding > 0 {
			dl.AddRectFilledV(minPt, maxPt, node.Style.Color, node.Style.Rounding, imgui.DrawFlagsNone)
		} else {
			dl.AddRectFilled(minPt, maxPt, node.Style.Color)
		}
	}

	if node.Text != "" {
		textPos := imgui.NewVec2(node.Bounds[0]+node.Style.Padding[3], node.Bounds[1]+node.Style.Padding[0])
		dl.AddTextVec2(textPos, PackABGR(255, 255, 255, 255), node.Text)
	}

	if len(node.Children) > 0 {
		dl.PushClipRectV(minPt, maxPt, true)
		for i := 0; i < len(node.Children); i++ {
			paintNode(dl, node.Children[i])
		}
		dl.PopClipRect()
	}
}
