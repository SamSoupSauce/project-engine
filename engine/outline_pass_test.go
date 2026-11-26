package engine

import (
	"testing"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type mockOutlineGLAPI struct {
	enabledCaps  []uint32
	disabledCaps []uint32
	stencilMasks []uint32
	stencilFuncs []struct {
		fn   uint32
		ref  int32
		mask uint32
	}
	stencilOps []struct {
		fail, zfail, zpass uint32
	}
	depthMasks     []bool
	usedPrograms   []uint32
	uniform4fCalls [][4]float32
	uniform1fCalls []float32
	clearedMasks   []uint32
	programDeleted bool
}

func (m *mockOutlineGLAPI) Enable(cap uint32)  { m.enabledCaps = append(m.enabledCaps, cap) }
func (m *mockOutlineGLAPI) Disable(cap uint32) { m.disabledCaps = append(m.disabledCaps, cap) }
func (m *mockOutlineGLAPI) StencilMask(mask uint32) {
	m.stencilMasks = append(m.stencilMasks, mask)
}
func (m *mockOutlineGLAPI) StencilFunc(fn uint32, ref int32, mask uint32) {
	m.stencilFuncs = append(m.stencilFuncs, struct {
		fn   uint32
		ref  int32
		mask uint32
	}{fn, ref, mask})
}
func (m *mockOutlineGLAPI) StencilOp(fail, zfail, zpass uint32) {
	m.stencilOps = append(m.stencilOps, struct {
		fail, zfail, zpass uint32
	}{fail, zfail, zpass})
}
func (m *mockOutlineGLAPI) DepthMask(flag bool) { m.depthMasks = append(m.depthMasks, flag) }
func (m *mockOutlineGLAPI) ClearStencil(int32)  {}
func (m *mockOutlineGLAPI) Clear(mask uint32)   { m.clearedMasks = append(m.clearedMasks, mask) }
func (m *mockOutlineGLAPI) UseProgram(p uint32) { m.usedPrograms = append(m.usedPrograms, p) }
func (m *mockOutlineGLAPI) CreateOutlineProgram() (uint32, int32, int32, error) {
	return 77, 1, 2, nil
}
func (m *mockOutlineGLAPI) DeleteProgram(uint32) { m.programDeleted = true }
func (m *mockOutlineGLAPI) Uniform4f(_ int32, v0, v1, v2, v3 float32) {
	m.uniform4fCalls = append(m.uniform4fCalls, [4]float32{v0, v1, v2, v3})
}
func (m *mockOutlineGLAPI) Uniform1f(_ int32, v0 float32) {
	m.uniform1fCalls = append(m.uniform1fCalls, v0)
}

func TestOutlinePassStencilAndExtraction(t *testing.T) {
	root := NewGameObject("root", "Root")
	crate := NewGameObject("crate", "Crate")
	r := NewRenderable("crate_mesh", "wood")
	r.SetOutlined(true)
	r.SetOutlineColor(Color{R: 51, G: 230, B: 255, A: 255})
	crate.AddComponent(r)

	plain := NewGameObject("wall", "Wall")
	plain.AddComponent(NewRenderable("wall_mesh", "concrete"))

	root.AddChild(crate)
	root.AddChild(plain)

	buf := NewRenderPassBuffer(8)
	buf.ExtractFromHierarchy(root)
	if len(buf.Commands) != 2 {
		t.Fatalf("expected 2 DrawCommands, got %d", len(buf.Commands))
	}

	mockGL := &mockOutlineGLAPI{}
	pass := NewOutlinePass(mockGL)
	if err := pass.Init(); err != nil {
		t.Fatalf("OutlinePass.Init failed: %v", err)
	}
	defer pass.Destroy()

	var drawSequence []bool
	drawnCount := pass.Execute(buf.Commands, func(cmd DrawCommand, isOutlinePass bool) {
		if cmd.MeshID != "crate_mesh" {
			t.Errorf("expected only crate_mesh in outline pass, got %s", cmd.MeshID)
		}
		drawSequence = append(drawSequence, isOutlinePass)
	})
	if drawnCount != 1 {
		t.Fatalf("expected 1 outlined command drawn, got %d", drawnCount)
	}
	if len(drawSequence) != 2 || drawSequence[0] || !drawSequence[1] {
		t.Fatalf("expected [stencilWrite(false), outlineDraw(true)], got %v", drawSequence)
	}

	if len(mockGL.stencilFuncs) < 2 ||
		mockGL.stencilFuncs[0].fn != gl.ALWAYS || mockGL.stencilFuncs[0].ref != int32(OutlineStencilBit) ||
		mockGL.stencilFuncs[1].fn != gl.NOTEQUAL || mockGL.stencilFuncs[1].mask != OutlineStencilBit {
		t.Fatalf("unexpected stencil funcs sequence: %+v", mockGL.stencilFuncs)
	}
	if len(mockGL.uniform4fCalls) != 1 || mockGL.uniform4fCalls[0][1] < 0.89 {
		t.Fatalf("expected cyan outline uniform color, got %+v", mockGL.uniform4fCalls)
	}
	if len(mockGL.depthMasks) < 2 || mockGL.depthMasks[0] || !mockGL.depthMasks[len(mockGL.depthMasks)-1] {
		t.Fatalf("expected depth mask disabled during outline and restored on end: %v", mockGL.depthMasks)
	}

	// Clearing outline stops GL calls on subsequent frame.
	r.SetOutlined(false)
	buf.Reset()
	buf.ExtractFromHierarchy(root)
	mockGL2 := &mockOutlineGLAPI{}
	pass2 := NewOutlinePass(mockGL2)
	if count := pass2.Execute(buf.Commands, nil); count != 0 {
		t.Fatalf("expected 0 outlined draws after clearing FlagOutline, got %d", count)
	}
	if len(mockGL2.enabledCaps) != 0 {
		t.Fatalf("expected 0 GL calls when no outline flags are active, got %v", mockGL2.enabledCaps)
	}
}
