package engine

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

var (
	// ErrMissingMeshComponent indicates a MeshRenderable owner lacks an attached MeshComponent.
	ErrMissingMeshComponent = errors.New("renderable: owner GameObject requires an attached MeshComponent")
	// ErrMeshNotLoaded indicates the attached MeshComponent has no loaded GLTFAsset.
	ErrMeshNotLoaded = errors.New("renderable: MeshComponent has no loaded GLTF/GLB asset")
)

const meshVertShaderSrc = `#version 330 core
layout (location = 0) in vec3 aPos;
layout (location = 1) in vec3 aColor;

uniform mat4 uMVP;

out vec3 vColor;

void main() {
    gl_Position = uMVP * vec4(aPos, 1.0);
    vColor = aColor;
}
`

const meshFragShaderSrc = `#version 330 core
in vec3 vColor;
out vec4 FragColor;

void main() {
    FragColor = vec4(vColor, 1.0);
}
`

// MeshGLAPI abstracts OpenGL 3.3 vertex buffer and shader calls so MeshRenderable can be unit tested headlessly.
type MeshGLAPI interface {
	CreateShaderProgram() (program uint32, mvpLoc int32, err error)
	DeleteProgram(program uint32)
	GenVertexArray() uint32
	DeleteVertexArray(vao uint32)
	BindVertexArray(vao uint32)
	GenBuffer() uint32
	DeleteBuffer(vbo uint32)
	BindBuffer(target, buffer uint32)
	BufferDataFloat32(target uint32, data []float32)
	SetupPosColorAttributes()
	UseProgram(program uint32)
	UniformMatrix4fv(location int32, count int32, transpose bool, value *float32)
	PolygonModeWireframe(wireframe bool)
	DrawArraysTriangles(first, count int32)
}

// RealMeshGLAPI executes MeshGLAPI calls against OpenGL 3.3 Core.
type RealMeshGLAPI struct{}

func (RealMeshGLAPI) CreateShaderProgram() (uint32, int32, error) {
	vs, err := compileGLShader(gl.VERTEX_SHADER, meshVertShaderSrc)
	if err != nil {
		return 0, -1, err
	}
	defer gl.DeleteShader(vs)

	fs, err := compileGLShader(gl.FRAGMENT_SHADER, meshFragShaderSrc)
	if err != nil {
		return 0, -1, err
	}
	defer gl.DeleteShader(fs)

	prog := gl.CreateProgram()
	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)
	gl.LinkProgram(prog)

	var ok int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &ok)
	if ok == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(prog, gl.INFO_LOG_LENGTH, &logLen)
		msg := make([]byte, logLen+1)
		gl.GetProgramInfoLog(prog, logLen, nil, &msg[0])
		gl.DeleteProgram(prog)
		return 0, -1, fmt.Errorf("mesh shader link failed: %s", string(msg))
	}
	mvpLoc := gl.GetUniformLocation(prog, gl.Str("uMVP\x00"))
	return prog, mvpLoc, nil
}

func (RealMeshGLAPI) DeleteProgram(p uint32)   { gl.DeleteProgram(p) }
func (RealMeshGLAPI) BindVertexArray(v uint32) { gl.BindVertexArray(v) }
func (RealMeshGLAPI) GenVertexArray() uint32 {
	var vao uint32
	gl.GenVertexArrays(1, &vao)
	return vao
}
func (RealMeshGLAPI) DeleteVertexArray(vao uint32) { gl.DeleteVertexArrays(1, &vao) }
func (RealMeshGLAPI) GenBuffer() uint32 {
	var vbo uint32
	gl.GenBuffers(1, &vbo)
	return vbo
}
func (RealMeshGLAPI) DeleteBuffer(vbo uint32)        { gl.DeleteBuffers(1, &vbo) }
func (RealMeshGLAPI) BindBuffer(target, buf uint32) { gl.BindBuffer(target, buf) }
func (RealMeshGLAPI) BufferDataFloat32(target uint32, data []float32) {
	if len(data) == 0 {
		return
	}
	gl.BufferData(target, len(data)*4, unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
}
func (RealMeshGLAPI) SetupPosColorAttributes() {
	stride := int32(6 * 4)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, stride, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 3, gl.FLOAT, false, stride, 3*4)
}
func (RealMeshGLAPI) UseProgram(p uint32) { gl.UseProgram(p) }
func (RealMeshGLAPI) UniformMatrix4fv(loc, count int32, transpose bool, value *float32) {
	gl.UniformMatrix4fv(loc, count, transpose, value)
}
func (RealMeshGLAPI) PolygonModeWireframe(wireframe bool) {
	if wireframe {
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
	} else {
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
	}
}
func (RealMeshGLAPI) DrawArraysTriangles(first, count int32) {
	gl.DrawArrays(gl.TRIANGLES, first, count)
}

// MeshRenderable is the concrete Renderable component for GameObjects with a MeshComponent
// and a TransformComponent. It uploads loaded .gltf/.glb geometry to OpenGL buffers and draws
// the entity using the TransformComponent's world-space transform.
type MeshRenderable struct {
	Owner         *GameObject
	Visible       bool
	Wireframe     bool
	GL            MeshGLAPI
	ConstraintErr error

	uploadedAsset *GLTFAsset
	program       uint32
	mvpLoc        int32
	vao           uint32
	vbo           uint32
	vertexCount   int32
}

// NewMeshRenderable creates a visible MeshRenderable using RealMeshGLAPI by default.
func NewMeshRenderable() *MeshRenderable {
	return &MeshRenderable{
		Visible: true,
		GL:      RealMeshGLAPI{},
	}
}

// Init implements Component and checks TransformComponent + MeshComponent constraints.
func (m *MeshRenderable) Init(owner *GameObject) {
	m.Owner = owner
	m.ConstraintErr = m.ValidateConstraint()
}

// Update implements Component.
func (m *MeshRenderable) Update(_ float32) {}

// Destroy releases OpenGL shader/buffer objects cleanly.
func (m *MeshRenderable) Destroy() {
	m.DisposeGL()
	m.Owner = nil
}

// DisposeGL frees GPU VAO/VBO/shader resources allocated for this MeshRenderable.
func (m *MeshRenderable) DisposeGL() {
	if m == nil || m.GL == nil {
		return
	}
	if m.vbo != 0 {
		m.GL.DeleteBuffer(m.vbo)
		m.vbo = 0
	}
	if m.vao != 0 {
		m.GL.DeleteVertexArray(m.vao)
		m.vao = 0
	}
	if m.program != 0 {
		m.GL.DeleteProgram(m.program)
		m.program = 0
	}
	m.uploadedAsset = nil
	m.vertexCount = 0
}

// ValidateConstraint verifies that Owner has both TransformComponent and a loaded MeshComponent.
func (m *MeshRenderable) ValidateConstraint() error {
	if m == nil || m.Owner == nil {
		return ErrMissingTransformComponent
	}
	if m.Owner.GetTransformComponent() == nil {
		return ErrMissingTransformComponent
	}
	mc := m.Owner.GetMeshComponent()
	if mc == nil {
		return ErrMissingMeshComponent
	}
	if mc.Asset == nil {
		return ErrMeshNotLoaded
	}
	return nil
}

// Draw validates constraints, lazily uploads GL buffers from MeshComponent.Asset, computes MVP from
// TransformComponent.WorldTransform(), and issues OpenGL draw commands.
func (m *MeshRenderable) Draw(viewProj [16]float32) error {
	if m == nil || !m.Visible {
		return nil
	}
	if err := m.ValidateConstraint(); err != nil {
		m.ConstraintErr = err
		return err
	}
	m.ConstraintErr = nil

	if m.GL == nil {
		m.GL = RealMeshGLAPI{}
	}

	mc := m.Owner.GetMeshComponent()
	if m.uploadedAsset != mc.Asset || m.vao == 0 || m.program == 0 {
		if err := m.uploadFromAsset(mc.Asset); err != nil {
			return err
		}
	}
	if m.vertexCount == 0 {
		return nil
	}

	wt := m.Owner.GetTransformComponent().WorldTransform()
	model := transformToMat4(wt)
	mvp := mat4MulEngine(viewProj, model)

	m.GL.UseProgram(m.program)
	m.GL.UniformMatrix4fv(m.mvpLoc, 1, false, &mvp[0])
	if m.Wireframe {
		m.GL.PolygonModeWireframe(true)
	}
	m.GL.BindVertexArray(m.vao)
	m.GL.DrawArraysTriangles(0, m.vertexCount)
	m.GL.BindVertexArray(0)
	if m.Wireframe {
		m.GL.PolygonModeWireframe(false)
	}
	m.GL.UseProgram(0)
	return nil
}

func (m *MeshRenderable) uploadFromAsset(asset *GLTFAsset) error {
	m.DisposeGL()

	verts := BuildInterleavedPosColorVertices(asset)
	if len(verts) == 0 {
		return fmt.Errorf("renderable: glTF asset has zero drawable vertices")
	}

	prog, mvpLoc, err := m.GL.CreateShaderProgram()
	if err != nil {
		return err
	}
	vao := m.GL.GenVertexArray()
	vbo := m.GL.GenBuffer()

	m.GL.BindVertexArray(vao)
	m.GL.BindBuffer(gl.ARRAY_BUFFER, vbo)
	m.GL.BufferDataFloat32(gl.ARRAY_BUFFER, verts)
	m.GL.SetupPosColorAttributes()
	m.GL.BindVertexArray(0)

	m.program = prog
	m.mvpLoc = mvpLoc
	m.vao = vao
	m.vbo = vbo
	m.uploadedAsset = asset
	m.vertexCount = int32(len(verts) / 6)
	return nil
}

// BuildInterleavedPosColorVertices flattens glTF primitives into non-indexed [x, y, z, r, g, b] triangles.
func BuildInterleavedPosColorVertices(asset *GLTFAsset) []float32 {
	if asset == nil {
		return nil
	}
	var out []float32
	for _, mesh := range asset.Meshes {
		for _, p := range mesh.Primitives {
			baseCol := [3]float32{0.85, 0.85, 0.85}
			if p.MaterialIndex >= 0 && p.MaterialIndex < len(asset.Materials) {
				f := asset.Materials[p.MaterialIndex].BaseColorFactor
				baseCol = [3]float32{f[0], f[1], f[2]}
			}
			appendVert := func(idx int) {
				if idx < 0 || idx >= len(p.Positions) {
					return
				}
				pos := p.Positions[idx]
				col := baseCol
				if idx < len(p.Colors) {
					col = [3]float32{p.Colors[idx][0], p.Colors[idx][1], p.Colors[idx][2]}
				}
				out = append(out, pos.X, pos.Y, pos.Z, col[0], col[1], col[2])
			}
			if len(p.Indices) > 0 {
				for _, idx := range p.Indices {
					appendVert(int(idx))
				}
			} else {
				for idx := range p.Positions {
					appendVert(idx)
				}
			}
		}
	}
	return out
}

func transformToMat4(t Transform) [16]float32 {
	sx, sy, sz := t.Scale.X, t.Scale.Y, t.Scale.Z
	if sx == 0 {
		sx = 1
	}
	if sy == 0 {
		sy = 1
	}
	if sz == 0 {
		sz = 1
	}
	rx := float64(t.Rotation.X) * math.Pi / 180.0
	ry := float64(t.Rotation.Y) * math.Pi / 180.0
	rz := float64(t.Rotation.Z) * math.Pi / 180.0

	cx, sxx := float32(math.Cos(rx)), float32(math.Sin(rx))
	cy, syy := float32(math.Cos(ry)), float32(math.Sin(ry))
	cz, szz := float32(math.Cos(rz)), float32(math.Sin(rz))

	// Rotation order Y * X * Z
	m00 := cy*cz + syy*sxx*szz
	m01 := cz*syy*sxx - cy*szz
	m02 := syy * cx
	m10 := cx * szz
	m11 := cx * cz
	m12 := -sxx
	m20 := cy*sxx*szz - syy*cz
	m21 := syy*szz + cy*cz*sxx
	m22 := cy * cx

	return [16]float32{
		m00 * sx, m10 * sx, m20 * sx, 0,
		m01 * sy, m11 * sy, m21 * sy, 0,
		m02 * sz, m12 * sz, m22 * sz, 0,
		t.Position.X, t.Position.Y, t.Position.Z, 1,
	}
}

func mat4MulEngine(a, b [16]float32) [16]float32 {
	var out [16]float32
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			out[col*4+row] = sum
		}
	}
	return out
}

var meshRenderableType = reflect.TypeOf((*MeshRenderable)(nil))

// GetMeshRenderable returns the attached MeshRenderable on this GameObject, or nil.
func (go_ *GameObject) GetMeshRenderable() *MeshRenderable {
	if go_ == nil {
		return nil
	}
	if comp := go_.GetComponent(meshRenderableType); comp != nil {
		if mr, ok := comp.(*MeshRenderable); ok {
			return mr
		}
	}
	return nil
}

// DrawMeshRenderables renders all active GameObjects possessing a valid MeshRenderable + TransformComponent + MeshComponent.
func (s *BaseScene) DrawMeshRenderables(viewProj [16]float32) int {
	if s == nil || s.Root == nil {
		return 0
	}
	drawn := 0
	for _, obj := range s.collectAllObjects(false) {
		if obj == nil || !obj.Active {
			continue
		}
		if mr := obj.GetMeshRenderable(); mr != nil && mr.Visible {
			if err := mr.Draw(viewProj); err == nil {
				drawn++
			}
		}
	}
	return drawn
}
