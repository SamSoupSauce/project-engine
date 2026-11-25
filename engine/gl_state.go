package engine

import (
	"github.com/go-gl/gl/v3.3-core/gl"
)

// GLState captures the OpenGL pipeline state that must be preserved around
// the Dear ImGui overlay draw pass so 3D/2D scene rendering is unaffected.
type GLState struct {
	Viewport           [4]int32
	ScissorBox         [4]int32
	ActiveProgram      int32
	ActiveTexture      int32
	ArrayBuffer        int32
	VertexArray        int32
	BlendEnabled       bool
	BlendSrcRGB        int32
	BlendDstRGB        int32
	BlendSrcAlpha      int32
	BlendDstAlpha      int32
	BlendEqRGB         int32
	BlendEqAlpha       int32
	DepthTestEnabled   bool
	DepthWriteMask     bool
	CullFaceEnabled    bool
	ScissorTestEnabled bool
}

// GLStateAPI abstracts OpenGL state queries and mutations for both live GL3 contexts
// and headless unit-test verification.
type GLStateAPI interface {
	GetIntegerv(pname uint32, data *int32)
	GetBooleanv(pname uint32, data *bool)
	IsEnabled(cap uint32) bool
	Enable(cap uint32)
	Disable(cap uint32)
	Viewport(x, y, width, height int32)
	Scissor(x, y, width, height int32)
	UseProgram(program uint32)
	ActiveTexture(texture uint32)
	BindBuffer(target, buffer uint32)
	BindVertexArray(array uint32)
	BlendFuncSeparate(sfactorRGB, dfactorRGB, sfactorAlpha, dfactorAlpha uint32)
	BlendEquationSeparate(modeRGB, modeAlpha uint32)
	DepthMask(flag bool)
}

// RealGLStateAPI delegates GLStateAPI calls directly to go-gl OpenGL 3.3 Core.
type RealGLStateAPI struct{}

func (RealGLStateAPI) GetIntegerv(pname uint32, data *int32) { gl.GetIntegerv(pname, data) }
func (RealGLStateAPI) GetBooleanv(pname uint32, data *bool)  { gl.GetBooleanv(pname, data) }
func (RealGLStateAPI) IsEnabled(cap uint32) bool             { return gl.IsEnabled(cap) }
func (RealGLStateAPI) Enable(cap uint32)                     { gl.Enable(cap) }
func (RealGLStateAPI) Disable(cap uint32)                    { gl.Disable(cap) }
func (RealGLStateAPI) Viewport(x, y, w, h int32)             { gl.Viewport(x, y, w, h) }
func (RealGLStateAPI) Scissor(x, y, w, h int32)              { gl.Scissor(x, y, w, h) }
func (RealGLStateAPI) UseProgram(program uint32)             { gl.UseProgram(program) }
func (RealGLStateAPI) ActiveTexture(texture uint32)          { gl.ActiveTexture(texture) }
func (RealGLStateAPI) BindBuffer(target, buffer uint32)      { gl.BindBuffer(target, buffer) }
func (RealGLStateAPI) BindVertexArray(array uint32)          { gl.BindVertexArray(array) }
func (RealGLStateAPI) BlendFuncSeparate(sRGB, dRGB, sA, dA uint32) {
	gl.BlendFuncSeparate(sRGB, dRGB, sA, dA)
}
func (RealGLStateAPI) BlendEquationSeparate(mRGB, mA uint32) { gl.BlendEquationSeparate(mRGB, mA) }
func (RealGLStateAPI) DepthMask(flag bool)                   { gl.DepthMask(flag) }

// BackupGLState snapshots the current OpenGL pipeline state prior to rendering ImGui.
func BackupGLState(api GLStateAPI) GLState {
	if api == nil {
		api = RealGLStateAPI{}
	}
	var s GLState
	api.GetIntegerv(gl.VIEWPORT, &s.Viewport[0])
	api.GetIntegerv(gl.SCISSOR_BOX, &s.ScissorBox[0])
	api.GetIntegerv(gl.CURRENT_PROGRAM, &s.ActiveProgram)
	api.GetIntegerv(gl.ACTIVE_TEXTURE, &s.ActiveTexture)
	api.GetIntegerv(gl.ARRAY_BUFFER_BINDING, &s.ArrayBuffer)
	api.GetIntegerv(gl.VERTEX_ARRAY_BINDING, &s.VertexArray)
	s.BlendEnabled = api.IsEnabled(gl.BLEND)
	api.GetIntegerv(gl.BLEND_SRC_RGB, &s.BlendSrcRGB)
	api.GetIntegerv(gl.BLEND_DST_RGB, &s.BlendDstRGB)
	api.GetIntegerv(gl.BLEND_SRC_ALPHA, &s.BlendSrcAlpha)
	api.GetIntegerv(gl.BLEND_DST_ALPHA, &s.BlendDstAlpha)
	api.GetIntegerv(gl.BLEND_EQUATION_RGB, &s.BlendEqRGB)
	api.GetIntegerv(gl.BLEND_EQUATION_ALPHA, &s.BlendEqAlpha)
	s.DepthTestEnabled = api.IsEnabled(gl.DEPTH_TEST)
	api.GetBooleanv(gl.DEPTH_WRITEMASK, &s.DepthWriteMask)
	s.CullFaceEnabled = api.IsEnabled(gl.CULL_FACE)
	s.ScissorTestEnabled = api.IsEnabled(gl.SCISSOR_TEST)
	return s
}

// RestoreGLState restores a previously captured OpenGL pipeline state.
func RestoreGLState(api GLStateAPI, s GLState) {
	if api == nil {
		api = RealGLStateAPI{}
	}
	api.UseProgram(uint32(s.ActiveProgram))
	api.ActiveTexture(uint32(s.ActiveTexture))
	api.BindVertexArray(uint32(s.VertexArray))
	api.BindBuffer(gl.ARRAY_BUFFER, uint32(s.ArrayBuffer))
	api.BlendEquationSeparate(uint32(s.BlendEqRGB), uint32(s.BlendEqAlpha))
	api.BlendFuncSeparate(
		uint32(s.BlendSrcRGB),
		uint32(s.BlendDstRGB),
		uint32(s.BlendSrcAlpha),
		uint32(s.BlendDstAlpha),
	)
	setGLCap(api, gl.BLEND, s.BlendEnabled)
	setGLCap(api, gl.DEPTH_TEST, s.DepthTestEnabled)
	setGLCap(api, gl.CULL_FACE, s.CullFaceEnabled)
	setGLCap(api, gl.SCISSOR_TEST, s.ScissorTestEnabled)
	api.DepthMask(s.DepthWriteMask)
	api.Viewport(s.Viewport[0], s.Viewport[1], s.Viewport[2], s.Viewport[3])
	api.Scissor(s.ScissorBox[0], s.ScissorBox[1], s.ScissorBox[2], s.ScissorBox[3])
}

func setGLCap(api GLStateAPI, cap uint32, enabled bool) {
	if enabled {
		api.Enable(cap)
	} else {
		api.Disable(cap)
	}
}
