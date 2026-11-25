package ui

// FlexDirection selects the primary layout axis for child placement.
type FlexDirection uint8

const (
	FlexRow FlexDirection = iota
	FlexColumn
)

// AlignItems selects cross-axis alignment for children within a flex container.
type AlignItems uint8

const (
	AlignStretch AlignItems = iota
	AlignStart
	AlignCenter
	AlignEnd
)

// LayoutStyle defines flexbox sizing, spacing, and visual attributes for a UINode.
type LayoutStyle struct {
	Direction FlexDirection
	Align     AlignItems
	Grow      float32
	Shrink    float32
	Gap       float32
	Padding   [4]float32 // top, right, bottom, left
	Width     float32    // fixed pixels (> 0)
	Height    float32    // fixed pixels (> 0)
	WidthPct  float32    // percentage of parent width (0..100)
	HeightPct float32    // percentage of parent height (0..100)
	Rounding  float32
	Color     uint32 // packed ABGR color (ImDrawList format: 0xAABBGGRR)
}

// UINode represents a retained, stateful node in the declarative UI tree.
type UINode struct {
	ID       string
	Style    LayoutStyle
	Bounds   [4]float32 // computed: x, y, w, h
	Dirty    bool
	Parent   *UINode
	Children []*UINode
	OnClick  string
	OnHover  string
	Text     string

	lastOriginX float32
	lastOriginY float32
	lastParentW float32
	lastParentH float32
	hasCached   bool
}

// MarkDirty flags this node and its ancestor chain as needing layout recomputation.
func (n *UINode) MarkDirty() {
	for cur := n; cur != nil; cur = cur.Parent {
		cur.Dirty = true
	}
}

// AddChild links a child node into this container and marks the subtree dirty.
func (n *UINode) AddChild(child *UINode) {
	if child == nil || child == n {
		return
	}
	child.Parent = n
	n.Children = append(n.Children, child)
	n.MarkDirty()
}

// ComputeLayout resolves bounding rectangles [x, y, w, h] across dirty subtrees
// without heap allocations on clean frames.
func ComputeLayout(root *UINode, parentWidth, parentHeight float32) {
	if root == nil {
		return
	}
	rootW := resolveDim(root.Style.Width, root.Style.WidthPct, parentWidth, parentWidth)
	rootH := resolveDim(root.Style.Height, root.Style.HeightPct, parentHeight, parentHeight)
	layoutNode(root, 0, 0, rootW, rootH, true)
}

func layoutNode(node *UINode, originX, originY, availW, availH float32, forceParentBounds bool) {
	if node == nil {
		return
	}

	if !node.Dirty && node.hasCached &&
		node.lastOriginX == originX &&
		node.lastOriginY == originY &&
		node.lastParentW == availW &&
		node.lastParentH == availH {
		// Node itself is clean; only visit children in case a descendant was marked dirty directly.
		innerX := node.Bounds[0] + node.Style.Padding[3]
		innerY := node.Bounds[1] + node.Style.Padding[0]
		innerW := clampNonNeg(node.Bounds[2] - node.Style.Padding[1] - node.Style.Padding[3])
		innerH := clampNonNeg(node.Bounds[3] - node.Style.Padding[0] - node.Style.Padding[2])
		layoutChildrenCleanCheck(node, innerX, innerY, innerW, innerH)
		return
	}

	w := availW
	h := availH
	if !forceParentBounds {
		w = resolveDim(node.Style.Width, node.Style.WidthPct, availW, availW)
		h = resolveDim(node.Style.Height, node.Style.HeightPct, availH, availH)
	}

	node.Bounds[0] = originX
	node.Bounds[1] = originY
	node.Bounds[2] = w
	node.Bounds[3] = h
	node.lastOriginX = originX
	node.lastOriginY = originY
	node.lastParentW = availW
	node.lastParentH = availH
	node.hasCached = true
	node.Dirty = false

	n := len(node.Children)
	if n == 0 {
		return
	}

	innerX := originX + node.Style.Padding[3]
	innerY := originY + node.Style.Padding[0]
	innerW := clampNonNeg(w - node.Style.Padding[1] - node.Style.Padding[3])
	innerH := clampNonNeg(h - node.Style.Padding[0] - node.Style.Padding[2])

	isRow := node.Style.Direction == FlexRow
	mainAvail := innerH
	crossAvail := innerW
	if isRow {
		mainAvail = innerW
		crossAvail = innerH
	}

	totalGap := node.Style.Gap * float32(n-1)
	var totalBaseMain, totalGrow, totalShrinkWeighted float32

	// Pass 1: measure base sizes and flex factors without heap allocations.
	for i := 0; i < n; i++ {
		child := node.Children[i]
		baseMain := childBaseMain(child, isRow, innerW, innerH)
		totalBaseMain += baseMain
		if child.Style.Grow > 0 {
			totalGrow += child.Style.Grow
		}
		if child.Style.Shrink > 0 && baseMain > 0 {
			totalShrinkWeighted += child.Style.Shrink * baseMain
		}
	}

	freeSpace := mainAvail - totalBaseMain - totalGap
	cursor := innerY
	if isRow {
		cursor = innerX
	}

	// Pass 2: assign main-axis sizes, cross-axis alignment, and recurse.
	for i := 0; i < n; i++ {
		child := node.Children[i]
		childMain := childBaseMain(child, isRow, innerW, innerH)
		if freeSpace > 0 && totalGrow > 0 && child.Style.Grow > 0 {
			childMain += freeSpace * (child.Style.Grow / totalGrow)
		} else if freeSpace < 0 && totalShrinkWeighted > 0 && child.Style.Shrink > 0 && childMain > 0 {
			shrink := (-freeSpace) * ((child.Style.Shrink * childMain) / totalShrinkWeighted)
			childMain = clampNonNeg(childMain - shrink)
		}

		childCross, hasExplicitCross := childBaseCross(child, isRow, innerW, innerH)
		if !hasExplicitCross {
			if node.Style.Align == AlignStretch {
				childCross = crossAvail
			} else {
				childCross = crossAvail
			}
		}

		var crossOffset float32
		switch node.Style.Align {
		case AlignCenter:
			crossOffset = (crossAvail - childCross) * 0.5
		case AlignEnd:
			crossOffset = crossAvail - childCross
		default: // AlignStart, AlignStretch
			crossOffset = 0
		}

		var cx, cy, cw, ch float32
		if isRow {
			cx = cursor
			cy = innerY + crossOffset
			cw = childMain
			ch = childCross
		} else {
			cx = innerX + crossOffset
			cy = cursor
			cw = childCross
			ch = childMain
		}

		layoutNode(child, cx, cy, cw, ch, true)
		cursor += childMain + node.Style.Gap
	}
}

func layoutChildrenCleanCheck(parent *UINode, innerX, innerY, innerW, innerH float32) {
	for i := 0; i < len(parent.Children); i++ {
		child := parent.Children[i]
		if hasAnyDirty(child) {
			// Recompute parent cleanly so flex siblings stay consistent.
			parent.Dirty = true
			layoutNode(parent, parent.Bounds[0], parent.Bounds[1], parent.Bounds[2], parent.Bounds[3], true)
			return
		}
	}
}

func hasAnyDirty(node *UINode) bool {
	if node == nil {
		return false
	}
	if node.Dirty {
		return true
	}
	for i := 0; i < len(node.Children); i++ {
		if hasAnyDirty(node.Children[i]) {
			return true
		}
	}
	return false
}

func resolveDim(fixed, pct, parentSize, fallback float32) float32 {
	if fixed > 0 {
		return fixed
	}
	if pct > 0 {
		return parentSize * (pct / 100.0)
	}
	return fallback
}

func childBaseMain(child *UINode, isRow bool, innerW, innerH float32) float32 {
	if isRow {
		return resolveDim(child.Style.Width, child.Style.WidthPct, innerW, 0)
	}
	return resolveDim(child.Style.Height, child.Style.HeightPct, innerH, 0)
}

func childBaseCross(child *UINode, isRow bool, innerW, innerH float32) (float32, bool) {
	if isRow {
		if child.Style.Height > 0 || child.Style.HeightPct > 0 {
			return resolveDim(child.Style.Height, child.Style.HeightPct, innerH, innerH), true
		}
		return 0, false
	}
	if child.Style.Width > 0 || child.Style.WidthPct > 0 {
		return resolveDim(child.Style.Width, child.Style.WidthPct, innerW, innerW), true
	}
	return 0, false
}

func clampNonNeg(v float32) float32 {
	if v < 0 {
		return 0
	}
	return v
}
