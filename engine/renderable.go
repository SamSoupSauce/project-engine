package engine

// RenderFlags defines pipeline configuration bitmasks for rendering entities.
type RenderFlags uint32

const (
	FlagNone           RenderFlags = 0
	FlagVisible        RenderFlags = 1 << 0
	FlagCastShadow     RenderFlags = 1 << 1
	FlagReceiveShadow  RenderFlags = 1 << 2
	FlagWireframe      RenderFlags = 1 << 3
	FlagTransparent    RenderFlags = 1 << 4
	FlagOverrideParent RenderFlags = 1 << 5 // Ignores inherited parent visibility/render modes
)

// Has checks if a specific flag bit is enabled.
func (f RenderFlags) Has(flag RenderFlags) bool {
	return f&flag != 0
}

// Renderable is a pipeline-driven rendering component attached to a GameObject.
// Rather than immediately calling GPU draw commands during scene traversal,
// Renderable instances are evaluated hierarchically and emitted into a flat DrawCommand buffer.
type Renderable struct {
	MeshID     string      // GPU buffer or primitive mesh handle (e.g. "cube", "quad", "mesh_42")
	MaterialID string      // Shader/pipeline state handle (e.g. "default", "unlit", "siding")
	Flags      RenderFlags // Pipeline bitmask
	Color      Color       // Surface base tint
	WireColor  Color       // Wireframe contour color
	CustomSize *Vector3    // Optional explicit dimensions (overriding mesh defaults)

	Owner *GameObject
}

// NewRenderable creates a visible renderable with default white color.
func NewRenderable(meshID, materialID string) *Renderable {
	return &Renderable{
		MeshID:     meshID,
		MaterialID: materialID,
		Flags:      FlagVisible | FlagCastShadow,
		Color:      White,
		WireColor:  Black,
	}
}

// Init registers the owner GameObject pointer.
func (r *Renderable) Init(owner *GameObject) {
	r.Owner = owner
}

// Update satisfies the Component interface.
func (r *Renderable) Update(dt float32) {
	// Passive component; updates handled during camera extraction pass
}

// Destroy cleans up component resources.
func (r *Renderable) Destroy() {
	r.Owner = nil
}

// DrawCommand is a flat, self-contained draw instruction produced during the camera extraction pass.
type DrawCommand struct {
	WorldTransform Transform
	MeshID         string
	MaterialID     string
	Flags          RenderFlags
	Color          Color
	WireColor      Color
	CustomSize     *Vector3
}

// RenderPassBuffer manages a reusable, allocation-free slice of DrawCommands across frames.
type RenderPassBuffer struct {
	Commands []DrawCommand
}

// NewRenderPassBuffer allocates a pre-sized buffer for draw commands.
func NewRenderPassBuffer(initialCapacity int) *RenderPassBuffer {
	if initialCapacity <= 0 {
		initialCapacity = 256
	}
	return &RenderPassBuffer{
		Commands: make([]DrawCommand, 0, initialCapacity),
	}
}

// Reset clears the command buffer without deallocating underlying memory.
func (b *RenderPassBuffer) Reset() {
	b.Commands = b.Commands[:0]
}

// ExtractFromHierarchy traverses a GameObject tree, cascades visibility and render modes,
// and appends draw commands into the buffer without per-frame heap allocations.
func (b *RenderPassBuffer) ExtractFromHierarchy(root *GameObject) {
	if root == nil || !root.Active {
		return
	}
	b.extractNode(root, FlagVisible, true)
}

func (b *RenderPassBuffer) extractNode(node *GameObject, inheritedFlags RenderFlags, parentVisible bool) {
	if node == nil || !node.Active {
		return
	}

	// Determine effective visibility and render modes
	effectiveVisible := parentVisible
	effectiveFlags := inheritedFlags

	// Search for attached Renderable component
	var renderable *Renderable
	for _, c := range node.Components {
		if r, ok := c.(*Renderable); ok {
			renderable = r
			break
		}
	}

	if renderable != nil {
		if renderable.Flags.Has(FlagOverrideParent) {
			effectiveVisible = renderable.Flags.Has(FlagVisible)
			effectiveFlags = renderable.Flags
		} else {
			// Cascade: visibility is AND of parent and child
			effectiveVisible = parentVisible && renderable.Flags.Has(FlagVisible)
			// Merge flags from parent and child
			effectiveFlags = (inheritedFlags | renderable.Flags)
			if !effectiveVisible {
				effectiveFlags &^= FlagVisible
			}
		}

		if effectiveVisible {
			worldT := node.GetWorldTransform()
			b.Commands = append(b.Commands, DrawCommand{
				WorldTransform: worldT,
				MeshID:         renderable.MeshID,
				MaterialID:     renderable.MaterialID,
				Flags:          effectiveFlags,
				Color:          renderable.Color,
				WireColor:      renderable.WireColor,
				CustomSize:     renderable.CustomSize,
			})
		}
	}

	// Recurse into children cascading the effective state
	for _, child := range node.Children {
		b.extractNode(child, effectiveFlags, effectiveVisible)
	}
}
