package engine

import (
	"errors"
	"testing"
)

type mockMeshGLAPI struct {
	progCalls  int
	vaoCreated uint32
	vboCreated uint32
	uploaded   []float32
	drawnVerts int32
	disposed   bool
}

func (m *mockMeshGLAPI) CreateShaderProgram() (uint32, int32, error) {
	m.progCalls++
	return 11, 2, nil
}
func (m *mockMeshGLAPI) DeleteProgram(uint32)       { m.disposed = true }
func (m *mockMeshGLAPI) GenVertexArray() uint32     { m.vaoCreated = 21; return 21 }
func (m *mockMeshGLAPI) DeleteVertexArray(uint32)   {}
func (m *mockMeshGLAPI) BindVertexArray(uint32)     {}
func (m *mockMeshGLAPI) GenBuffer() uint32          { m.vboCreated = 31; return 31 }
func (m *mockMeshGLAPI) DeleteBuffer(uint32)        {}
func (m *mockMeshGLAPI) BindBuffer(uint32, uint32)  {}
func (m *mockMeshGLAPI) BufferDataFloat32(_ uint32, d []float32) {
	m.uploaded = append([]float32(nil), d...)
}
func (m *mockMeshGLAPI) SetupPosColorAttributes()                         {}
func (m *mockMeshGLAPI) UseProgram(uint32)                                {}
func (m *mockMeshGLAPI) UniformMatrix4fv(int32, int32, bool, *float32)    {}
func (m *mockMeshGLAPI) PolygonModeWireframe(bool)                        {}
func (m *mockMeshGLAPI) DrawArraysTriangles(_ int32, count int32)         { m.drawnVerts = count }

func TestMeshComponentAndMeshRenderableConstraints(t *testing.T) {
	obj := NewGameObject("toaster", "Toaster")

	mr := NewMeshRenderable()
	mockGL := &mockMeshGLAPI{}
	mr.GL = mockGL
	obj.AddComponent(mr)

	// Missing TransformComponent first.
	if err := mr.ValidateConstraint(); !errors.Is(err, ErrMissingTransformComponent) {
		t.Fatalf("expected ErrMissingTransformComponent, got %v", err)
	}

	// Add TransformComponent -> next constraint is Missing MeshComponent.
	tc := NewTransformComponentAt(NewVector3(1, 2, 3))
	obj.AddComponent(tc)
	if err := mr.ValidateConstraint(); !errors.Is(err, ErrMissingMeshComponent) {
		t.Fatalf("expected ErrMissingMeshComponent, got %v", err)
	}

	// Drawing without MeshComponent returns ErrMissingMeshComponent without GL draw.
	var identityMVP [16]float32
	identityMVP[0], identityMVP[5], identityMVP[10], identityMVP[15] = 1, 1, 1, 1
	if err := mr.Draw(identityMVP); !errors.Is(err, ErrMissingMeshComponent) {
		t.Fatalf("expected Draw to fail without MeshComponent, got %v", err)
	}
	if mockGL.drawnVerts != 0 {
		t.Fatalf("expected 0 drawn verts when constraint unsatisfied")
	}

	// Add valid GLTF MeshComponent -> constraints satisfied & BaseScene renders it cleanly.
	mc := NewMeshComponent("../assets/models/props/kitchen_toaster.gltf")
	if mc.LoadErr != nil {
		t.Fatalf("NewMeshComponent load failed: %v", mc.LoadErr)
	}
	obj.AddComponent(mc)

	if err := mr.ValidateConstraint(); err != nil {
		t.Fatalf("expected all constraints satisfied, got %v", err)
	}

	scene := NewBaseScene("mesh_render_scene")
	scene.AddObject(obj)
	drawn := scene.DrawMeshRenderables(identityMVP)
	if drawn != 1 {
		t.Fatalf("expected BaseScene.DrawMeshRenderables to draw 1 object, got %d", drawn)
	}
	if mockGL.drawnVerts != 36 || len(mockGL.uploaded) != 36*6 {
		t.Fatalf("expected 36 triangle vertices (216 floats) uploaded/drawn, got verts=%d floats=%d",
			mockGL.drawnVerts, len(mockGL.uploaded))
	}

	mr.DisposeGL()
	if !mockGL.disposed {
		t.Fatalf("expected DisposeGL to release shader program")
	}
}
