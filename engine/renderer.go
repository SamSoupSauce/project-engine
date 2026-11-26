package engine

import "sort"

// Renderer is the unified per-frame drawing context (`gfx`) passed to per-instance
// render hooks on spawned GameObjects with a Renderable component.
// It collects 3D DrawCommands into a reusable RenderPassBuffer and 2D QuadCommand2Ds
// for OrthoBatcher2D.
type Renderer struct {
	ViewportW  float32
	ViewportH  float32
	Buffer3D   *RenderPassBuffer
	Commands2D []QuadCommand2D
}

// NewRenderer allocates a Renderer with pre-sized 3D and 2D command buffers.
func NewRenderer(viewportW, viewportH float32) *Renderer {
	if viewportW <= 0 {
		viewportW = 1280
	}
	if viewportH <= 0 {
		viewportH = 720
	}
	return &Renderer{
		ViewportW:  viewportW,
		ViewportH:  viewportH,
		Buffer3D:   NewRenderPassBuffer(256),
		Commands2D: make([]QuadCommand2D, 0, 256),
	}
}

// Reset clears buffered 3D and 2D draw commands for a new frame without deallocating capacity.
func (r *Renderer) Reset(viewportW, viewportH float32) {
	if viewportW > 0 {
		r.ViewportW = viewportW
	}
	if viewportH > 0 {
		r.ViewportH = viewportH
	}
	if r.Buffer3D == nil {
		r.Buffer3D = NewRenderPassBuffer(256)
	} else {
		r.Buffer3D.Reset()
	}
	r.Commands2D = r.Commands2D[:0]
}

// SetViewportSize updates the active viewport dimensions reported to scripts.
func (r *Renderer) SetViewportSize(width, height float32) {
	if width > 0 {
		r.ViewportW = width
	}
	if height > 0 {
		r.ViewportH = height
	}
}

// ViewportSize returns the current viewport width and height in pixels.
func (r *Renderer) ViewportSize() (float32, float32) {
	return r.ViewportW, r.ViewportH
}

// SubmitDrawCommand appends a 3D DrawCommand to the frame's RenderPassBuffer.
func (r *Renderer) SubmitDrawCommand(cmd DrawCommand) {
	if r.Buffer3D == nil {
		r.Buffer3D = NewRenderPassBuffer(256)
	}
	r.Buffer3D.Commands = append(r.Buffer3D.Commands, cmd)
}

// DrawMesh constructs and submits a 3D DrawCommand with the specified parameters.
func (r *Renderer) DrawMesh(meshID, materialID string, transform Transform, color Color, flags RenderFlags) {
	r.SubmitDrawCommand(DrawCommand{
		WorldTransform: transform,
		MeshID:         meshID,
		MaterialID:     materialID,
		Flags:          flags,
		Color:          color,
		WireColor:      Black,
	})
}

// DrawQuad appends a 2D QuadCommand2D instruction for orthographic rendering.
func (r *Renderer) DrawQuad(cmd QuadCommand2D) {
	cmd.Order = len(r.Commands2D)
	r.Commands2D = append(r.Commands2D, cmd)
}

// DrawRect appends an untextured solid-colored 2D quad at (x, y) with size (width, height).
func (r *Renderer) DrawRect(x, y, width, height float32, color Color, zIndex int32) {
	if width <= 0 || height <= 0 {
		return
	}
	r.DrawQuad(QuadCommand2D{
		X:          x,
		Y:          y,
		Width:      width,
		Height:     height,
		SourceRect: Rect2D{U: 0, V: 0, W: 1, H: 1},
		TextureID:  0,
		Color:      color,
		ZIndex:     zIndex,
		BlendMode:  BlendAlpha,
	})
}

// DrawTexturedQuad appends a textured 2D quad with custom UVs, tint color, z-index, and blend mode.
func (r *Renderer) DrawTexturedQuad(x, y, width, height float32, textureID uint32, uv Rect2D, color Color, zIndex int32, blend BlendMode2D) {
	if width <= 0 || height <= 0 {
		return
	}
	r.DrawQuad(QuadCommand2D{
		X:          x,
		Y:          y,
		Width:      width,
		Height:     height,
		SourceRect: uv,
		TextureID:  textureID,
		Color:      color,
		ZIndex:     zIndex,
		BlendMode:  blend,
	})
}

// Commands3D returns the 3D DrawCommands submitted or extracted for the current frame.
func (r *Renderer) Commands3D() []DrawCommand {
	if r.Buffer3D == nil {
		return nil
	}
	return r.Buffer3D.Commands
}

// SortedCommands2D sorts buffered 2D quad commands stably by ZIndex ascending and submission order.
func (r *Renderer) SortedCommands2D() []QuadCommand2D {
	sort.SliceStable(r.Commands2D, func(i, j int) bool {
		if r.Commands2D[i].ZIndex != r.Commands2D[j].ZIndex {
			return r.Commands2D[i].ZIndex < r.Commands2D[j].ZIndex
		}
		return r.Commands2D[i].Order < r.Commands2D[j].Order
	})
	return r.Commands2D
}

// Flush2D submits buffered 2D quad commands into the provided OrthoBatcher2D.
func (r *Renderer) Flush2D(batcher *OrthoBatcher2D) {
	if batcher == nil || len(r.Commands2D) == 0 {
		return
	}
	cmds := r.SortedCommands2D()
	batcher.Begin(int(r.ViewportW), int(r.ViewportH))
	batcher.DrawCommands(cmds)
	batcher.End()
}
