package engine

import "sort"

// Vector2 represents a 2D dimension or coordinate.
type Vector2 struct {
	X float32
	Y float32
}

// NewVector2 constructs a Vector2.
func NewVector2(x, y float32) Vector2 {
	return Vector2{X: x, Y: y}
}

// Rect2D represents a normalized UV or pixel rectangle (U, V, W, H).
type Rect2D struct {
	U float32
	V float32
	W float32
	H float32
}

// Anchor2D specifies screen-relative alignment for 2D UI/HUD elements.
type Anchor2D uint8

const (
	AnchorTopLeft Anchor2D = iota
	AnchorTopCenter
	AnchorTopRight
	AnchorCenterLeft
	AnchorCenter
	AnchorCenterRight
	AnchorBottomLeft
	AnchorBottomCenter
	AnchorBottomRight
)

// BlendMode2D identifies blending configuration for batched 2D quads.
type BlendMode2D uint8

const (
	BlendAlpha BlendMode2D = iota
	BlendAdditive
)

// Renderable2D is a component attached to a GameObject for orthographic 2D UI,
// HUD elements, sprites, health bars, and crosshairs.
type Renderable2D struct {
	TextureID  uint32
	SourceRect Rect2D
	Size       Vector2
	Anchor     Anchor2D
	Color      Color
	ZIndex     int32
	Flags      RenderFlags
	BlendMode  BlendMode2D

	Owner *GameObject
}

// NewRenderable2D creates a visible Renderable2D with unit UVs, white tint, and top-left anchor.
func NewRenderable2D(textureID uint32, width, height float32) *Renderable2D {
	return &Renderable2D{
		TextureID:  textureID,
		SourceRect: Rect2D{U: 0, V: 0, W: 1, H: 1},
		Size:       Vector2{X: width, Y: height},
		Anchor:     AnchorTopLeft,
		Color:      White,
		ZIndex:     0,
		Flags:      FlagVisible,
		BlendMode:  BlendAlpha,
	}
}

// Init registers the owning GameObject.
func (r *Renderable2D) Init(owner *GameObject) {
	r.Owner = owner
}

// Update satisfies the engine.Component interface.
func (r *Renderable2D) Update(dt float32) {}

// Destroy clears the owner reference.
func (r *Renderable2D) Destroy() {
	r.Owner = nil
}

// SetVisible toggles FlagVisible on this Renderable2D.
func (r *Renderable2D) SetVisible(visible bool) {
	if visible {
		r.Flags |= FlagVisible
	} else {
		r.Flags &^= FlagVisible
	}
}

// IsVisible reports whether FlagVisible is enabled.
func (r *Renderable2D) IsVisible() bool {
	return r.Flags.Has(FlagVisible)
}

// QuadCommand2D is a flat, resolved 2D draw instruction ready for OrthoBatcher2D.
type QuadCommand2D struct {
	X          float32
	Y          float32
	Width      float32
	Height     float32
	SourceRect Rect2D
	TextureID  uint32
	Color      Color
	ZIndex     int32
	BlendMode  BlendMode2D
	Order      int
}

// AnchorFactors returns the normalized (ax, ay) screen and pivot anchor ratios in [0, 1].
func AnchorFactors(anchor Anchor2D) (float32, float32) {
	switch anchor {
	case AnchorTopCenter:
		return 0.5, 0.0
	case AnchorTopRight:
		return 1.0, 0.0
	case AnchorCenterLeft:
		return 0.0, 0.5
	case AnchorCenter:
		return 0.5, 0.5
	case AnchorCenterRight:
		return 1.0, 0.5
	case AnchorBottomLeft:
		return 0.0, 1.0
	case AnchorBottomCenter:
		return 0.5, 1.0
	case AnchorBottomRight:
		return 1.0, 1.0
	default:
		return 0.0, 0.0
	}
}

// ResolveAnchoredBounds computes top-left pixel coordinates (x, y) for an element of
// size (w, h) given its local/world offset (offsetX, offsetY), anchor, and viewport dimensions.
func ResolveAnchoredBounds(offsetX, offsetY, width, height float32, anchor Anchor2D, viewportW, viewportH float32) (float32, float32) {
	ax, ay := AnchorFactors(anchor)
	x := ax*viewportW + offsetX - ax*width
	y := ay*viewportH + offsetY - ay*height
	return x, y
}

// Extract2DFromHierarchy traverses a GameObject tree, cascades visibility flags,
// resolves anchored screen coordinates, and returns commands sorted by ZIndex ascending.
func Extract2DFromHierarchy(root *GameObject, viewportW, viewportH float32) []QuadCommand2D {
	if root == nil || !root.Active {
		return nil
	}
	var cmds []QuadCommand2D
	extract2DNode(root, true, viewportW, viewportH, &cmds)

	sort.SliceStable(cmds, func(i, j int) bool {
		if cmds[i].ZIndex != cmds[j].ZIndex {
			return cmds[i].ZIndex < cmds[j].ZIndex
		}
		return cmds[i].Order < cmds[j].Order
	})
	return cmds
}

func extract2DNode(node *GameObject, parentVisible bool, viewportW, viewportH float32, out *[]QuadCommand2D) {
	if node == nil || !node.Active {
		return
	}

	effectiveVisible := parentVisible

	node.mu.RLock()
	components := make([]Component, len(node.Components))
	copy(components, node.Components)
	children := make([]*GameObject, len(node.Children))
	copy(children, node.Children)
	node.mu.RUnlock()

	for _, c := range components {
		r2d, ok := c.(*Renderable2D)
		if !ok {
			continue
		}
		if r2d.Flags.Has(FlagOverrideParent) {
			effectiveVisible = r2d.Flags.Has(FlagVisible)
		} else {
			effectiveVisible = parentVisible && r2d.Flags.Has(FlagVisible)
		}

		if effectiveVisible && r2d.Size.X > 0 && r2d.Size.Y > 0 {
			worldPos := node.GetWorldTransform().Position
			x, y := ResolveAnchoredBounds(
				worldPos.X, worldPos.Y,
				r2d.Size.X, r2d.Size.Y,
				r2d.Anchor, viewportW, viewportH,
			)
			*out = append(*out, QuadCommand2D{
				X:          x,
				Y:          y,
				Width:      r2d.Size.X,
				Height:     r2d.Size.Y,
				SourceRect: r2d.SourceRect,
				TextureID:  r2d.TextureID,
				Color:      r2d.Color,
				ZIndex:     r2d.ZIndex,
				BlendMode:  r2d.BlendMode,
				Order:      len(*out),
			})
		}
	}

	for _, child := range children {
		extract2DNode(child, effectiveVisible, viewportW, viewportH, out)
	}
}
