package engine

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// DefaultBatcher2DCapacity is the default number of quads pre-allocated in OrthoBatcher2D.
const DefaultBatcher2DCapacity = 1024

// Vertex2D represents a single vertex submitted to OrthoBatcher2D (32 bytes).
type Vertex2D struct {
	X, Y       float32
	U, V       float32
	R, G, B, A float32
}

// Batcher2DStats records per-frame batching telemetry.
type Batcher2DStats struct {
	DrawCalls     int
	QuadsRendered int
	BatchFlushes  int
}

// BatcherGLAPI abstracts the OpenGL 3.3 Core calls needed by OrthoBatcher2D so
// batching, projection updates, and state changes can be tested both live and headlessly.
type BatcherGLAPI interface {
	CreateProgram(vertSrc, fragSrc string) (uint32, error)
	DeleteProgram(program uint32)
	UseProgram(program uint32)
	GetUniformLocation(program uint32, name string) int32
	UniformMatrix4fv(location int32, count int32, transpose bool, value *float32)
	Uniform1i(location int32, v0 int32)

	GenVertexArray() uint32
	DeleteVertexArray(vao uint32)
	BindVertexArray(vao uint32)

	GenBuffer() uint32
	DeleteBuffer(buf uint32)
	BindBuffer(target, buf uint32)
	BufferData(target uint32, size int, data unsafe.Pointer, usage uint32)
	BufferSubData(target uint32, offset, size int, data unsafe.Pointer)

	EnableVertexAttribArray(index uint32)
	VertexAttribPointer(index uint32, size int32, xtype uint32, normalized bool, stride int32, offset int)

	GenTexture() uint32
	DeleteTexture(tex uint32)
	ActiveTexture(texture uint32)
	BindTexture(target, texture uint32)
	TexParameteri(target, pname uint32, param int32)
	TexImage2D(target uint32, level, internalformat, width, height, border int32, format, xtype uint32, pixels unsafe.Pointer)

	DrawElements(mode uint32, count int32, xtype uint32, offset int)
	Enable(cap uint32)
	Disable(cap uint32)
	IsEnabled(cap uint32) bool
	GetBooleanv(pname uint32, data *bool)
	BlendFunc(sfactor, dfactor uint32)
	DepthMask(flag bool)
}

// RealBatcherGLAPI delegates BatcherGLAPI calls directly to OpenGL 3.3 Core.
type RealBatcherGLAPI struct{}

func (RealBatcherGLAPI) CreateProgram(vertSrc, fragSrc string) (uint32, error) {
	vs, err := compileGLShader(gl.VERTEX_SHADER, vertSrc)
	if err != nil {
		return 0, err
	}
	defer gl.DeleteShader(vs)

	fs, err := compileGLShader(gl.FRAGMENT_SHADER, fragSrc)
	if err != nil {
		return 0, err
	}
	defer gl.DeleteShader(fs)

	prog := gl.CreateProgram()
	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)
	gl.LinkProgram(prog)

	var status int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(prog, gl.INFO_LOG_LENGTH, &logLen)
		msg := strings.Repeat("\x00", int(logLen+1))
		gl.GetProgramInfoLog(prog, logLen, nil, gl.Str(msg))
		gl.DeleteProgram(prog)
		return 0, fmt.Errorf("engine: shader link failed: %s", strings.TrimRight(msg, "\x00"))
	}
	return prog, nil
}

func compileGLShader(shaderType uint32, src string) (uint32, error) {
	shader := gl.CreateShader(shaderType)
	cSources, free := gl.Strs(src + "\x00")
	gl.ShaderSource(shader, 1, cSources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLen)
		msg := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(shader, logLen, nil, gl.Str(msg))
		gl.DeleteShader(shader)
		return 0, fmt.Errorf("engine: shader compile failed: %s", strings.TrimRight(msg, "\x00"))
	}
	return shader, nil
}

func (RealBatcherGLAPI) DeleteProgram(p uint32)              { gl.DeleteProgram(p) }
func (RealBatcherGLAPI) UseProgram(p uint32)                 { gl.UseProgram(p) }
func (RealBatcherGLAPI) GetUniformLocation(p uint32, name string) int32 {
	return gl.GetUniformLocation(p, gl.Str(name+"\x00"))
}
func (RealBatcherGLAPI) UniformMatrix4fv(loc, count int32, transpose bool, val *float32) {
	gl.UniformMatrix4fv(loc, count, transpose, val)
}
func (RealBatcherGLAPI) Uniform1i(loc, v0 int32) { gl.Uniform1i(loc, v0) }

func (RealBatcherGLAPI) GenVertexArray() uint32 {
	var vao uint32
	gl.GenVertexArrays(1, &vao)
	return vao
}
func (RealBatcherGLAPI) DeleteVertexArray(vao uint32) { gl.DeleteVertexArrays(1, &vao) }
func (RealBatcherGLAPI) BindVertexArray(vao uint32)   { gl.BindVertexArray(vao) }

func (RealBatcherGLAPI) GenBuffer() uint32 {
	var buf uint32
	gl.GenBuffers(1, &buf)
	return buf
}
func (RealBatcherGLAPI) DeleteBuffer(buf uint32)        { gl.DeleteBuffers(1, &buf) }
func (RealBatcherGLAPI) BindBuffer(target, buf uint32)  { gl.BindBuffer(target, buf) }
func (RealBatcherGLAPI) BufferData(target uint32, size int, data unsafe.Pointer, usage uint32) {
	gl.BufferData(target, size, data, usage)
}
func (RealBatcherGLAPI) BufferSubData(target uint32, offset, size int, data unsafe.Pointer) {
	gl.BufferSubData(target, offset, size, data)
}
func (RealBatcherGLAPI) EnableVertexAttribArray(idx uint32) { gl.EnableVertexAttribArray(idx) }
func (RealBatcherGLAPI) VertexAttribPointer(idx uint32, size int32, xtype uint32, norm bool, stride int32, offset int) {
	gl.VertexAttribPointerWithOffset(idx, size, xtype, norm, stride, uintptr(offset))
}
func (RealBatcherGLAPI) GenTexture() uint32 {
	var tex uint32
	gl.GenTextures(1, &tex)
	return tex
}
func (RealBatcherGLAPI) DeleteTexture(tex uint32)          { gl.DeleteTextures(1, &tex) }
func (RealBatcherGLAPI) ActiveTexture(t uint32)            { gl.ActiveTexture(t) }
func (RealBatcherGLAPI) BindTexture(target, tex uint32)    { gl.BindTexture(target, tex) }
func (RealBatcherGLAPI) TexParameteri(t, p uint32, v int32) { gl.TexParameteri(t, p, v) }
func (RealBatcherGLAPI) TexImage2D(t uint32, lvl, ifmt, w, h, b int32, fmt, xt uint32, px unsafe.Pointer) {
	gl.TexImage2D(t, lvl, ifmt, w, h, b, fmt, xt, px)
}
func (RealBatcherGLAPI) DrawElements(mode uint32, count int32, xtype uint32, offset int) {
	gl.DrawElementsWithOffset(mode, count, xtype, uintptr(offset))
}
func (RealBatcherGLAPI) Enable(cap uint32)                 { gl.Enable(cap) }
func (RealBatcherGLAPI) Disable(cap uint32)                { gl.Disable(cap) }
func (RealBatcherGLAPI) IsEnabled(cap uint32) bool         { return gl.IsEnabled(cap) }
func (RealBatcherGLAPI) GetBooleanv(p uint32, data *bool)  { gl.GetBooleanv(p, data) }
func (RealBatcherGLAPI) BlendFunc(s, d uint32)             { gl.BlendFunc(s, d) }
func (RealBatcherGLAPI) DepthMask(flag bool)               { gl.DepthMask(flag) }

const ortho2DVertexShader = `#version 330 core
layout (location = 0) in vec2 aPos;
layout (location = 1) in vec2 aUV;
layout (location = 2) in vec4 aColor;

uniform mat4 uProjection;

out vec2 vUV;
out vec4 vColor;

void main() {
	gl_Position = uProjection * vec4(aPos, 0.0, 1.0);
	vUV = aUV;
	vColor = aColor;
}
`

const ortho2DFragmentShader = `#version 330 core
in vec2 vUV;
in vec4 vColor;

uniform sampler2D uTexture;

out vec4 FragColor;

void main() {
	FragColor = texture(uTexture, vUV) * vColor;
}
`

// OrthoBatcher2D batches 2D textured and colored quads into a dynamic VBO/EBO
// and submits single glDrawElements calls per contiguous texture and blend state.
type OrthoBatcher2D struct {
	glAPI       BatcherGLAPI
	maxQuads    int
	vertices    []Vertex2D
	quadCount   int
	initialized bool
	inPass      bool

	vao          uint32
	vbo          uint32
	ebo          uint32
	program      uint32
	projLoc      int32
	texLoc       int32
	whiteTexture uint32

	currentTexture uint32
	currentBlend   BlendMode2D
	projection     [16]float32

	prevDepthTest  bool
	prevDepthWrite bool
	prevBlend      bool

	Stats Batcher2DStats
}

// NewOrthoBatcher2D creates an uninitialized OrthoBatcher2D with the specified quad capacity.
func NewOrthoBatcher2D(maxQuads int) *OrthoBatcher2D {
	if maxQuads <= 0 {
		maxQuads = DefaultBatcher2DCapacity
	}
	return &OrthoBatcher2D{
		glAPI:    RealBatcherGLAPI{},
		maxQuads: maxQuads,
		vertices: make([]Vertex2D, maxQuads*4),
	}
}

// SetGLAPI overrides the OpenGL API backend (useful for headless tests).
func (b *OrthoBatcher2D) SetGLAPI(api BatcherGLAPI) {
	if api == nil {
		b.glAPI = RealBatcherGLAPI{}
		return
	}
	b.glAPI = api
}

// Init allocates the dynamic VBO, pre-populated EBO, VAO, shader program, and 1x1 white texture.
func (b *OrthoBatcher2D) Init() error {
	if b.initialized {
		return nil
	}
	if b.glAPI == nil {
		b.glAPI = RealBatcherGLAPI{}
	}

	prog, err := b.glAPI.CreateProgram(ortho2DVertexShader, ortho2DFragmentShader)
	if err != nil {
		return err
	}
	b.program = prog
	b.projLoc = b.glAPI.GetUniformLocation(prog, "uProjection")
	b.texLoc = b.glAPI.GetUniformLocation(prog, "uTexture")

	b.vao = b.glAPI.GenVertexArray()
	b.vbo = b.glAPI.GenBuffer()
	b.ebo = b.glAPI.GenBuffer()

	b.glAPI.BindVertexArray(b.vao)

	vboBytes := b.maxQuads * 4 * int(unsafe.Sizeof(Vertex2D{}))
	b.glAPI.BindBuffer(gl.ARRAY_BUFFER, b.vbo)
	b.glAPI.BufferData(gl.ARRAY_BUFFER, vboBytes, nil, gl.DYNAMIC_DRAW)

	indices := make([]uint32, b.maxQuads*6)
	for i := 0; i < b.maxQuads; i++ {
		baseV := uint32(i * 4)
		baseI := i * 6
		indices[baseI+0] = baseV + 0
		indices[baseI+1] = baseV + 1
		indices[baseI+2] = baseV + 2
		indices[baseI+3] = baseV + 2
		indices[baseI+4] = baseV + 3
		indices[baseI+5] = baseV + 0
	}
	b.glAPI.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.ebo)
	b.glAPI.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, unsafe.Pointer(&indices[0]), gl.STATIC_DRAW)

	stride := int32(unsafe.Sizeof(Vertex2D{}))
	// layout 0: aPos (vec2)
	b.glAPI.EnableVertexAttribArray(0)
	b.glAPI.VertexAttribPointer(0, 2, gl.FLOAT, false, stride, 0)
	// layout 1: aUV (vec2)
	b.glAPI.EnableVertexAttribArray(1)
	b.glAPI.VertexAttribPointer(1, 2, gl.FLOAT, false, stride, 2*4)
	// layout 2: aColor (vec4)
	b.glAPI.EnableVertexAttribArray(2)
	b.glAPI.VertexAttribPointer(2, 4, gl.FLOAT, false, stride, 4*4)

	b.glAPI.BindVertexArray(0)

	// 1x1 solid white RGBA texture for untextured colored UI quads
	whitePixel := [4]uint8{255, 255, 255, 255}
	b.whiteTexture = b.glAPI.GenTexture()
	b.glAPI.BindTexture(gl.TEXTURE_2D, b.whiteTexture)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	b.glAPI.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&whitePixel[0]))
	b.glAPI.BindTexture(gl.TEXTURE_2D, 0)

	b.initialized = true
	return nil
}

// CreateTextureRGBA uploads an RGBA8 pixel buffer as an OpenGL 2D texture with
// clamp-to-edge wrapping and linear filtering for clean alpha-blended sprites.
func (b *OrthoBatcher2D) CreateTextureRGBA(width, height int, pixels []byte) (uint32, error) {
	if !b.initialized {
		return 0, fmt.Errorf("engine: OrthoBatcher2D must be initialized before creating textures")
	}
	if width <= 0 || height <= 0 || len(pixels) < width*height*4 {
		return 0, fmt.Errorf("engine: invalid RGBA texture dimensions %dx%d", width, height)
	}
	tex := b.glAPI.GenTexture()
	b.glAPI.BindTexture(gl.TEXTURE_2D, tex)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	b.glAPI.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	b.glAPI.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(width), int32(height), 0, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	b.glAPI.BindTexture(gl.TEXTURE_2D, 0)
	return tex, nil
}

// Projection returns the current 4x4 column-major orthographic projection matrix.
func (b *OrthoBatcher2D) Projection() [16]float32 {
	return b.projection
}

// Ortho2DMatrix computes a column-major OpenGL orthographic projection mapping
// (0,0) top-left to (width, height) bottom-right.
func Ortho2DMatrix(left, right, bottom, top, nearVal, farVal float32) [16]float32 {
	rl := right - left
	tb := top - bottom
	fn := farVal - nearVal
	if rl == 0 {
		rl = 1
	}
	if tb == 0 {
		tb = 1
	}
	if fn == 0 {
		fn = 1
	}
	return [16]float32{
		2 / rl, 0, 0, 0,
		0, 2 / tb, 0, 0,
		0, 0, -2 / fn, 0,
		-(right + left) / rl, -(top + bottom) / tb, -(farVal + nearVal) / fn, 1,
	}
}

// Begin starts an orthographic 2D pass matching the given window width and height,
// disabling depth writes and enabling standard alpha blending.
func (b *OrthoBatcher2D) Begin(viewportW, viewportH int) {
	if !b.initialized {
		return
	}
	if viewportW <= 0 {
		viewportW = 1
	}
	if viewportH <= 0 {
		viewportH = 1
	}

	b.Stats = Batcher2DStats{}
	b.quadCount = 0
	b.currentTexture = 0
	b.currentBlend = BlendAlpha
	b.inPass = true

	b.prevDepthTest = b.glAPI.IsEnabled(gl.DEPTH_TEST)
	b.prevBlend = b.glAPI.IsEnabled(gl.BLEND)
	b.glAPI.GetBooleanv(gl.DEPTH_WRITEMASK, &b.prevDepthWrite)

	b.glAPI.Disable(gl.DEPTH_TEST)
	b.glAPI.DepthMask(false)
	b.glAPI.Enable(gl.BLEND)
	b.glAPI.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	b.projection = Ortho2DMatrix(0, float32(viewportW), float32(viewportH), 0, -1, 1)

	b.glAPI.UseProgram(b.program)
	b.glAPI.UniformMatrix4fv(b.projLoc, 1, false, &b.projection[0])
	b.glAPI.Uniform1i(b.texLoc, 0)
	b.glAPI.BindVertexArray(b.vao)
}

// DrawCommands submits a slice of resolved QuadCommand2D instructions to the batcher.
func (b *OrthoBatcher2D) DrawCommands(cmds []QuadCommand2D) {
	for _, cmd := range cmds {
		b.DrawQuad(cmd)
	}
}

// DrawQuad appends a quad to the active batch, flushing automatically when
// TextureID, BlendMode, or buffer capacity changes.
func (b *OrthoBatcher2D) DrawQuad(cmd QuadCommand2D) {
	if !b.initialized || !b.inPass {
		return
	}

	texID := cmd.TextureID
	if texID == 0 {
		texID = b.whiteTexture
	}

	if b.quadCount > 0 && (texID != b.currentTexture || cmd.BlendMode != b.currentBlend || b.quadCount >= b.maxQuads) {
		b.Flush()
	}

	if b.quadCount == 0 {
		b.currentTexture = texID
		b.currentBlend = cmd.BlendMode
		if cmd.BlendMode == BlendAdditive {
			b.glAPI.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		} else {
			b.glAPI.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		}
	}

	r := float32(cmd.Color.R) / 255.0
	g := float32(cmd.Color.G) / 255.0
	bl := float32(cmd.Color.B) / 255.0
	a := float32(cmd.Color.A) / 255.0

	u0 := cmd.SourceRect.U
	v0 := cmd.SourceRect.V
	u1 := cmd.SourceRect.U + cmd.SourceRect.W
	v1 := cmd.SourceRect.V + cmd.SourceRect.H

	x0 := cmd.X
	y0 := cmd.Y
	x1 := cmd.X + cmd.Width
	y1 := cmd.Y + cmd.Height

	base := b.quadCount * 4
	b.vertices[base+0] = Vertex2D{X: x0, Y: y0, U: u0, V: v0, R: r, G: g, B: bl, A: a}
	b.vertices[base+1] = Vertex2D{X: x1, Y: y0, U: u1, V: v0, R: r, G: g, B: bl, A: a}
	b.vertices[base+2] = Vertex2D{X: x1, Y: y1, U: u1, V: v1, R: r, G: g, B: bl, A: a}
	b.vertices[base+3] = Vertex2D{X: x0, Y: y1, U: u0, V: v1, R: r, G: g, B: bl, A: a}
	b.quadCount++
}

// Flush uploads pending quad vertices and emits a single glDrawElements call.
func (b *OrthoBatcher2D) Flush() {
	if !b.initialized || b.quadCount == 0 {
		return
	}

	b.glAPI.ActiveTexture(gl.TEXTURE0)
	b.glAPI.BindTexture(gl.TEXTURE_2D, b.currentTexture)

	byteSize := b.quadCount * 4 * int(unsafe.Sizeof(Vertex2D{}))
	b.glAPI.BindBuffer(gl.ARRAY_BUFFER, b.vbo)
	b.glAPI.BufferSubData(gl.ARRAY_BUFFER, 0, byteSize, unsafe.Pointer(&b.vertices[0]))

	b.glAPI.DrawElements(gl.TRIANGLES, int32(b.quadCount*6), gl.UNSIGNED_INT, 0)

	b.Stats.DrawCalls++
	b.Stats.BatchFlushes++
	b.Stats.QuadsRendered += b.quadCount
	b.quadCount = 0
}

// End flushes remaining quads and restores depth-write, depth-test, and blend state.
func (b *OrthoBatcher2D) End() {
	if !b.initialized || !b.inPass {
		return
	}
	b.Flush()
	b.glAPI.BindTexture(gl.TEXTURE_2D, 0)
	b.glAPI.BindVertexArray(0)
	b.glAPI.UseProgram(0)

	b.glAPI.DepthMask(b.prevDepthWrite)
	if b.prevDepthTest {
		b.glAPI.Enable(gl.DEPTH_TEST)
	} else {
		b.glAPI.Disable(gl.DEPTH_TEST)
	}
	if b.prevBlend {
		b.glAPI.Enable(gl.BLEND)
	} else {
		b.glAPI.Disable(gl.BLEND)
	}
	b.inPass = false
}

// Destroy releases OpenGL buffers, shaders, and fallback textures owned by the batcher.
func (b *OrthoBatcher2D) Destroy() {
	if !b.initialized {
		return
	}
	if b.whiteTexture != 0 {
		b.glAPI.DeleteTexture(b.whiteTexture)
		b.whiteTexture = 0
	}
	if b.ebo != 0 {
		b.glAPI.DeleteBuffer(b.ebo)
		b.ebo = 0
	}
	if b.vbo != 0 {
		b.glAPI.DeleteBuffer(b.vbo)
		b.vbo = 0
	}
	if b.vao != 0 {
		b.glAPI.DeleteVertexArray(b.vao)
		b.vao = 0
	}
	if b.program != 0 {
		b.glAPI.DeleteProgram(b.program)
		b.program = 0
	}
	b.initialized = false
}
