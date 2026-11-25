package ui

// PointerHandler handles a dispatched pointer event for a UINode.
// Returning true stops bubbling to ancestor nodes.
type PointerHandler func(handlerName, nodeID string, mouseX, mouseY float32) bool

// HitTest performs a front-to-back 2D bounding-box traversal and returns the deepest
// UINode whose Bounds [x, y, w, h] contain (mouseX, mouseY).
func HitTest(root *UINode, mouseX, mouseY float32) *UINode {
	if root == nil || !containsPoint(root.Bounds, mouseX, mouseY) {
		return nil
	}
	for i := len(root.Children) - 1; i >= 0; i-- {
		if hit := HitTest(root.Children[i], mouseX, mouseY); hit != nil {
			return hit
		}
	}
	return root
}

// FindNodeByID locates the first node in the subtree matching id, or nil.
func FindNodeByID(root *UINode, id string) *UINode {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	for i := 0; i < len(root.Children); i++ {
		if found := FindNodeByID(root.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}

// PointerEventDispatcher routes pointer click and hover events to UINode handlers
// with bubbling from the deepest hit target toward the root.
type PointerEventDispatcher struct {
	OnDispatch PointerHandler
}

// DispatchClick hit-tests root at (mouseX, mouseY) and bubbles OnClick handlers.
// Returns the hit node (if any) and whether any handler handled the event.
func (d *PointerEventDispatcher) DispatchClick(root *UINode, mouseX, mouseY float32) (*UINode, bool) {
	return d.dispatch(root, mouseX, mouseY, true)
}

// DispatchHover hit-tests root at (mouseX, mouseY) and bubbles OnHover handlers.
// Returns the hit node (if any) and whether any handler handled the event.
func (d *PointerEventDispatcher) DispatchHover(root *UINode, mouseX, mouseY float32) (*UINode, bool) {
	return d.dispatch(root, mouseX, mouseY, false)
}

func (d *PointerEventDispatcher) dispatch(root *UINode, mouseX, mouseY float32, isClick bool) (*UINode, bool) {
	target := HitTest(root, mouseX, mouseY)
	if target == nil || d == nil || d.OnDispatch == nil {
		return target, false
	}

	for cur := target; cur != nil; cur = cur.Parent {
		handler := cur.OnHover
		if isClick {
			handler = cur.OnClick
		}
		if handler == "" {
			continue
		}
		if d.OnDispatch(handler, cur.ID, mouseX, mouseY) {
			return target, true
		}
	}
	return target, false
}

func containsPoint(bounds [4]float32, x, y float32) bool {
	return x >= bounds[0] &&
		y >= bounds[1] &&
		x <= bounds[0]+bounds[2] &&
		y <= bounds[1]+bounds[3]
}
