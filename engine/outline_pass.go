package engine

import (
	"fmt"

	"github.com/go-gl/gl/v3.3-core/gl"
)

const (
	// OutlineStencilBit is the stencil buffer bit (0x02) reserved for hovered entity highlight outlines.
	OutlineStencilBit  uint32 = 0x02
	OutlineStencilRef  int32  = 0x02
	DefaultOutlineGain float32 = 1.04
)

// OutlineVertexShader expands vertices slightly along model-space normals to form a crisp hull outline.
const OutlineVertexShader = `#version 330 core
layout (location = 0) in vec3 aPos;
layout (location = 1) in vec3 aNormal;

uniform mat4 uViewProj;
uniform mat4 uModel;
uniform float uOutlineExpand;

void main() {
    vec3 expanded = aPos + normalize(aNormal) * uOutlineExpand;
    gl_Position = uViewProj * uModel * vec4(expanded, 1.0);
}
`

// OutlineFragmentShader emits a flat, bright emissive highlight tint for pixels passing the stencil check.
const OutlineFragmentShader = `#version 330 core
uniform vec4 uOutlineColor;
out vec4 FragColor;

void main() {
    FragColor = uOutlineColor;
}
`

// OutlineGLAPI abstracts OpenGL stencil and shader calls so OutlinePass can be verified headlessly.
type OutlineGLAPI interface {
	Enable(cap uint32)
	Disable(cap uint32)
	StencilMask(mask uint32)
	StencilFunc(xfunc uint32, ref int32, mask uint32)
	StencilOp(fail, zfail, zpass uint32)
	DepthMask(flag bool)
	ClearStencil(s int32)
	Clear(mask uint32)
	UseProgram(program uint32)
	CreateOutlineProgram() (program uint32, colorLoc, expandLoc int32, err error)
	DeleteProgram(program uint32)
	Uniform4f(location int32, v0, v1, v2, v3 float32)
	Uniform1f(location int32, v0 float32)
}

// RealOutlineGLAPI dispatches OutlineGLAPI calls to OpenGL 3.3 Core.
type RealOutlineGLAPI struct{}

func (RealOutlineGLAPI) Enable(cap uint32)                             { gl.Enable(cap) }
func (RealOutlineGLAPI) Disable(cap uint32)                            { gl.Disable(cap) }
func (RealOutlineGLAPI) StencilMask(mask uint32)                       { gl.StencilMask(mask) }
func (RealOutlineGLAPI) StencilFunc(xfunc uint32, ref int32, m uint32) { gl.StencilFunc(xfunc, ref, m) }
func (RealOutlineGLAPI) StencilOp(fail, zfail, zpass uint32)           { gl.StencilOp(fail, zfail, zpass) }
func (RealOutlineGLAPI) DepthMask(flag bool)                           { gl.DepthMask(flag) }
func (RealOutlineGLAPI) ClearStencil(s int32)                          { gl.ClearStencil(s) }
func (RealOutlineGLAPI) Clear(mask uint32)                             { gl.Clear(mask) }
func (RealOutlineGLAPI) UseProgram(program uint32)                     { gl.UseProgram(program) }
func (RealOutlineGLAPI) DeleteProgram(program uint32)                  { gl.DeleteProgram(program) }
func (RealOutlineGLAPI) Uniform4f(loc int32, v0, v1, v2, v3 float32)   { gl.Uniform4f(loc, v0, v1, v2, v3) }
func (RealOutlineGLAPI) Uniform1f(loc int32, v0 float32)               { gl.Uniform1f(loc, v0) }

func (RealOutlineGLAPI) CreateOutlineProgram() (uint32, int32, int32, error) {
	vs, err := compileGLShader(gl.VERTEX_SHADER, OutlineVertexShader)
	if err != nil {
		return 0, -1, -1, err
	}
	defer gl.DeleteShader(vs)

	fs, err := compileGLShader(gl.FRAGMENT_SHADER, OutlineFragmentShader)
	if err != nil {
		return 0, -1, -1, err
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
		logMsg := make([]byte, logLen+1)
		gl.GetProgramInfoLog(prog, logLen, nil, &logMsg[0])
		gl.DeleteProgram(prog)
		return 0, -1, -1, fmt.Errorf("outline program link failed: %s", string(logMsg))
	}

	colorLoc := gl.GetUniformLocation(prog, gl.Str("uOutlineColor\x00"))
	expandLoc := gl.GetUniformLocation(prog, gl.Str("uOutlineExpand\x00"))
	return prog, colorLoc, expandLoc, nil
}

// OutlinePass renders stencil-masked silhouette outlines for any DrawCommand flagged with FlagOutline.
type OutlinePass struct {
	API       OutlineGLAPI
	Program   uint32
	ColorLoc  int32
	ExpandLoc int32

	filtered []DrawCommand
}

// NewOutlinePass creates an OutlinePass backed by the provided OutlineGLAPI (or RealOutlineGLAPI if nil).
func NewOutlinePass(api OutlineGLAPI) *OutlinePass {
	if api == nil {
		api = RealOutlineGLAPI{}
	}
	return &OutlinePass{
		API:       api,
		ColorLoc:  -1,
		ExpandLoc: -1,
		filtered:  make([]DrawCommand, 0, 16),
	}
}

// Init compiles and links the outline shader program if not yet initialized.
func (p *OutlinePass) Init() error {
	if p == nil || p.Program != 0 {
		return nil
	}
	if p.API == nil {
		p.API = RealOutlineGLAPI{}
	}
	prog, colorLoc, expandLoc, err := p.API.CreateOutlineProgram()
	if err != nil {
		return err
	}
	p.Program = prog
	p.ColorLoc = colorLoc
	p.ExpandLoc = expandLoc
	return nil
}

// Destroy releases GPU shader resources held by the outline pass.
func (p *OutlinePass) Destroy() {
	if p == nil || p.Program == 0 || p.API == nil {
		return
	}
	p.API.DeleteProgram(p.Program)
	p.Program = 0
	p.ColorLoc = -1
	p.ExpandLoc = -1
}

// FilterOutlinedCommands returns a reusable slice of visible commands bearing FlagOutline.
func (p *OutlinePass) FilterOutlinedCommands(commands []DrawCommand) []DrawCommand {
	p.filtered = p.filtered[:0]
	for i := range commands {
		if commands[i].Flags.Has(FlagVisible) && commands[i].Flags.Has(FlagOutline) {
			p.filtered = append(p.filtered, commands[i])
		}
	}
	return p.filtered
}

// BeginStencilWrite configures the stencil buffer to write OutlineStencilBit (0x02) on target geometry.
func (p *OutlinePass) BeginStencilWrite() {
	p.API.Enable(gl.STENCIL_TEST)
	p.API.StencilMask(OutlineStencilBit)
	p.API.ClearStencil(0)
	p.API.Clear(gl.STENCIL_BUFFER_BIT)
	p.API.StencilFunc(gl.ALWAYS, OutlineStencilRef, OutlineStencilBit)
	p.API.StencilOp(gl.KEEP, gl.KEEP, gl.REPLACE)
}

// BeginOutlineDraw binds the outline shader and configures stencil testing to render only outside 0x02.
func (p *OutlinePass) BeginOutlineDraw(tint Color, width float32) {
	if width <= 0 {
		width = DefaultOutlineGain
	}
	if tint.A == 0 {
		tint = Color{R: 255, G: 199, B: 51, A: 255}
	}
	p.API.StencilFunc(gl.NOTEQUAL, OutlineStencilRef, OutlineStencilBit)
	p.API.StencilMask(0x00)
	p.API.DepthMask(false)
	if p.Program != 0 {
		p.API.UseProgram(p.Program)
		if p.ColorLoc >= 0 {
			p.API.Uniform4f(
				p.ColorLoc,
				float32(tint.R)/255.0,
				float32(tint.G)/255.0,
				float32(tint.B)/255.0,
				float32(tint.A)/255.0,
			)
		}
		if p.ExpandLoc >= 0 {
			expand := width - 1.0
			if expand <= 0 {
				expand = 0.04
			}
			p.API.Uniform1f(p.ExpandLoc, expand)
		}
	}
}

// EndOutlineDraw restores default stencil and depth masks after drawing outlines.
func (p *OutlinePass) EndOutlineDraw() {
	p.API.UseProgram(0)
	p.API.StencilMask(0xFF)
	p.API.DepthMask(true)
	p.API.Disable(gl.STENCIL_TEST)
}

// Execute runs the two-stage stencil mask + expanded silhouette outline pass over flagged commands.
// If drawMesh is provided, it receives each DrawCommand with isOutlinePass=false during stencil write
// and isOutlinePass=true during the emissive outline draw.
func (p *OutlinePass) Execute(commands []DrawCommand, drawMesh func(cmd DrawCommand, isOutlinePass bool)) int {
	outlined := p.FilterOutlinedCommands(commands)
	if len(outlined) == 0 {
		return 0
	}

	p.BeginStencilWrite()
	if drawMesh != nil {
		for i := range outlined {
			drawMesh(outlined[i], false)
		}
	}

	for i := range outlined {
		p.BeginOutlineDraw(outlined[i].OutlineColor, outlined[i].OutlineWidth)
		if drawMesh != nil {
			drawMesh(outlined[i], true)
		}
	}
	p.EndOutlineDraw()
	return len(outlined)
}
