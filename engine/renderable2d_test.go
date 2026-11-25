package engine

import (
	"testing"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type drawCallRecord struct {
	texID uint32
	count int32
}

type mockBatcherGLAPI struct {
	nextID        uint32
	boundTex      uint32
	blendEnabled  bool
	depthEnabled  bool
	depthMask     bool
	blendSrc      uint32
	blendDst      uint32
	lastProj      [16]float32
	drawCalls     []drawCallRecord
	uploadedBytes int
}

func newMockBatcherGLAPI() *mockBatcherGLAPI {
	return &mockBatcherGLAPI{
		nextID:       100,
		depthEnabled: true,
		depthMask:    true,
		blendEnabled: false,
	}
}

func (m *mockBatcherGLAPI) CreateProgram(_, _ string) (uint32, error) { m.nextID++; return m.nextID, nil }
func (m *mockBatcherGLAPI) DeleteProgram(_ uint32)                    {}
func (m *mockBatcherGLAPI) UseProgram(_ uint32)                       {}
func (m *mockBatcherGLAPI) GetUniformLocation(_ uint32, _ string) int32 { return 1 }
func (m *mockBatcherGLAPI) UniformMatrix4fv(_ int32, _ int32, _ bool, val *float32) {
	copy(m.lastProj[:], unsafe.Slice(val, 16))
}
func (m *mockBatcherGLAPI) Uniform1i(_ int32, _ int32) {}
func (m *mockBatcherGLAPI) GenVertexArray() uint32     { m.nextID++; return m.nextID }
func (m *mockBatcherGLAPI) DeleteVertexArray(_ uint32) {}
func (m *mockBatcherGLAPI) BindVertexArray(_ uint32)   {}
func (m *mockBatcherGLAPI) GenBuffer() uint32          { m.nextID++; return m.nextID }
func (m *mockBatcherGLAPI) DeleteBuffer(_ uint32)      {}
func (m *mockBatcherGLAPI) BindBuffer(_, _ uint32)     {}
func (m *mockBatcherGLAPI) BufferData(_ uint32, _ int, _ unsafe.Pointer, _ uint32) {
}
func (m *mockBatcherGLAPI) BufferSubData(_ uint32, _, size int, _ unsafe.Pointer) {
	m.uploadedBytes += size
}
func (m *mockBatcherGLAPI) EnableVertexAttribArray(_ uint32) {}
func (m *mockBatcherGLAPI) VertexAttribPointer(_ uint32, _ int32, _ uint32, _ bool, _ int32, _ int) {
}
func (m *mockBatcherGLAPI) GenTexture() uint32           { m.nextID++; return m.nextID }
func (m *mockBatcherGLAPI) DeleteTexture(_ uint32)       {}
func (m *mockBatcherGLAPI) ActiveTexture(_ uint32)       {}
func (m *mockBatcherGLAPI) BindTexture(_, tex uint32)    { m.boundTex = tex }
func (m *mockBatcherGLAPI) TexParameteri(_, _ uint32, _ int32) {}
func (m *mockBatcherGLAPI) TexImage2D(_ uint32, _, _, _, _, _ int32, _, _ uint32, _ unsafe.Pointer) {
}
func (m *mockBatcherGLAPI) DrawElements(_ uint32, count int32, _ uint32, _ int) {
	m.drawCalls = append(m.drawCalls, drawCallRecord{texID: m.boundTex, count: count})
}
func (m *mockBatcherGLAPI) Enable(cap uint32) {
	if cap == gl.BLEND {
		m.blendEnabled = true
	}
	if cap == gl.DEPTH_TEST {
		m.depthEnabled = true
	}
}
func (m *mockBatcherGLAPI) Disable(cap uint32) {
	if cap == gl.BLEND {
		m.blendEnabled = false
	}
	if cap == gl.DEPTH_TEST {
		m.depthEnabled = false
	}
}
func (m *mockBatcherGLAPI) IsEnabled(cap uint32) bool {
	if cap == gl.BLEND {
		return m.blendEnabled
	}
	if cap == gl.DEPTH_TEST {
		return m.depthEnabled
	}
	return false
}
func (m *mockBatcherGLAPI) GetBooleanv(p uint32, data *bool) {
	if p == gl.DEPTH_WRITEMASK {
		*data = m.depthMask
	}
}
func (m *mockBatcherGLAPI) BlendFunc(s, d uint32) { m.blendSrc = s; m.blendDst = d }
func (m *mockBatcherGLAPI) DepthMask(flag bool)   { m.depthMask = flag }

func TestOrthoBatcher2DSingleAtlasBatchingAndState(t *testing.T) {
	mockGL := newMockBatcherGLAPI()
	batcher := NewOrthoBatcher2D(8)
	batcher.SetGLAPI(mockGL)
	if err := batcher.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer batcher.Destroy()

	batcher.Begin(1280, 720)
	// During pass: depth writes disabled, depth test disabled, alpha blending active.
	if mockGL.depthMask || mockGL.depthEnabled {
		t.Errorf("expected depth write and depth test disabled during 2D pass")
	}
	if !mockGL.blendEnabled || mockGL.blendSrc != gl.SRC_ALPHA || mockGL.blendDst != gl.ONE_MINUS_SRC_ALPHA {
		t.Errorf("expected SRC_ALPHA / ONE_MINUS_SRC_ALPHA blending during 2D pass")
	}

	// Submit 4 quads sharing TextureID 10 -> must produce 1 single batched glDrawElements call (24 indices).
	for i := 0; i < 4; i++ {
		batcher.DrawQuad(QuadCommand2D{
			X: float32(i * 32), Y: 10, Width: 24, Height: 24,
			SourceRect: Rect2D{U: 0, V: 0, W: 0.5, H: 0.5},
			TextureID:  10,
			Color:      White,
		})
	}
	// Switching to TextureID 20 flushes batch 1 and starts batch 2.
	batcher.DrawQuad(QuadCommand2D{
		X: 200, Y: 10, Width: 64, Height: 16,
		SourceRect: Rect2D{U: 0, V: 0, W: 1, H: 1},
		TextureID:  20,
		Color:      NewColor(255, 0, 0, 255),
	})
	batcher.End()

	if len(mockGL.drawCalls) != 2 {
		t.Fatalf("expected 2 batched draw calls (atlas 10 then atlas 20), got %d", len(mockGL.drawCalls))
	}
	if mockGL.drawCalls[0].texID != 10 || mockGL.drawCalls[0].count != 24 {
		t.Errorf("unexpected first batch: %+v", mockGL.drawCalls[0])
	}
	if mockGL.drawCalls[1].texID != 20 || mockGL.drawCalls[1].count != 6 {
		t.Errorf("unexpected second batch: %+v", mockGL.drawCalls[1])
	}
	if batcher.Stats.DrawCalls != 2 || batcher.Stats.QuadsRendered != 5 {
		t.Errorf("unexpected batch stats: %+v", batcher.Stats)
	}

	// After End(): depth write, depth test, and blend state restored to pre-pass values.
	if !mockGL.depthMask || !mockGL.depthEnabled || mockGL.blendEnabled {
		t.Errorf("expected GL depth/blend state restored after End(): mask=%v depth=%v blend=%v",
			mockGL.depthMask, mockGL.depthEnabled, mockGL.blendEnabled)
	}
}

func TestOrthoBatcher2DCapacityFlushAndProjectionResize(t *testing.T) {
	mockGL := newMockBatcherGLAPI()
	batcher := NewOrthoBatcher2D(2) // small capacity to test auto-flush
	batcher.SetGLAPI(mockGL)
	if err := batcher.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer batcher.Destroy()

	batcher.Begin(800, 600)
	p1 := batcher.Projection()
	for i := 0; i < 5; i++ {
		batcher.DrawQuad(QuadCommand2D{X: float32(i), Y: 0, Width: 10, Height: 10, TextureID: 1, Color: White})
	}
	batcher.End()

	// 5 quads with maxQuads=2 -> batches of 2, 2, 1 => 3 draw calls
	if len(mockGL.drawCalls) != 3 {
		t.Fatalf("expected 3 capacity-split draw calls, got %d", len(mockGL.drawCalls))
	}

	batcher.Begin(1920, 1080)
	p2 := batcher.Projection()
	batcher.End()
	if p1 == p2 {
		t.Fatalf("expected orthographic projection matrix to update on viewport resize")
	}
	// Verify ortho matrix scale: m[0] = 2 / width
	if p2[0] != 2.0/1920.0 {
		t.Errorf("expected m[0] == 2/1920, got %f", p2[0])
	}
}

func TestRenderable2DZIndexSortingAnchorsAndVisibility(t *testing.T) {
	root := NewGameObject("hud", "HUD")

	fg := NewGameObject("crosshair", "Crosshair")
	fgR := NewRenderable2D(2, 16, 16)
	fgR.Anchor = AnchorCenter
	fgR.ZIndex = 100
	fg.AddComponent(fgR)

	bg := NewGameObject("panel", "Panel")
	bg.Transform.Position = NewVector3(10, 20, 0)
	bgR := NewRenderable2D(1, 200, 80)
	bgR.Anchor = AnchorTopLeft
	bgR.ZIndex = 1
	bg.AddComponent(bgR)

	hidden := NewGameObject("hidden", "Hidden")
	hiddenR := NewRenderable2D(1, 50, 50)
	hiddenR.SetVisible(false)
	hidden.AddComponent(hiddenR)

	corner := NewGameObject("minimap", "Minimap")
	corner.Transform.Position = NewVector3(-10, -10, 0)
	cornerR := NewRenderable2D(3, 100, 100)
	cornerR.Anchor = AnchorBottomRight
	cornerR.ZIndex = 10
	corner.AddComponent(cornerR)

	// Add foreground before background to verify ZIndex sorting reorders them.
	root.AddChild(fg)
	root.AddChild(corner)
	root.AddChild(bg)
	root.AddChild(hidden)

	cmds := Extract2DFromHierarchy(root, 1280, 720)
	if len(cmds) != 3 {
		t.Fatalf("expected 3 visible 2D commands, got %d", len(cmds))
	}
	if cmds[0].ZIndex != 1 || cmds[1].ZIndex != 10 || cmds[2].ZIndex != 100 {
		t.Fatalf("expected sorted ZIndex [1, 10, 100], got [%d, %d, %d]", cmds[0].ZIndex, cmds[1].ZIndex, cmds[2].ZIndex)
	}

	// Verify AnchorCenter on 1280x720 -> (640 - 8, 360 - 8) = (632, 352)
	if cmds[2].X != 632 || cmds[2].Y != 352 {
		t.Errorf("expected centered crosshair at (632, 352), got (%f, %f)", cmds[2].X, cmds[2].Y)
	}
	// Resize viewport to 1920x1080 -> AnchorCenter updates to (952, 532), AnchorBottomRight to (1810, 970)
	cmdsResized := Extract2DFromHierarchy(root, 1920, 1080)
	if cmdsResized[2].X != 952 || cmdsResized[2].Y != 532 {
		t.Errorf("expected resized centered crosshair at (952, 532), got (%f, %f)", cmdsResized[2].X, cmdsResized[2].Y)
	}
	if cmdsResized[1].X != 1810 || cmdsResized[1].Y != 970 {
		t.Errorf("expected resized bottom-right minimap at (1810, 970), got (%f, %f)", cmdsResized[1].X, cmdsResized[1].Y)
	}
}

func TestOrthoBatcher2DLiveGLFWOpenGL3(t *testing.T) {
	cfg := DefaultGLWindowConfig()
	cfg.Visible = false
	cfg.Width = 640
	cfg.Height = 480

	scene := NewBaseScene("HUDScene")
	overlay := NewImGuiOverlay()
	app := NewGLApp(cfg, scene, overlay)
	if err := app.InitWindow(); err != nil {
		t.Skipf("skipping live GLFW/OpenGL3 test: %v", err)
	}
	defer app.Close()

	// Create a 2x2 RGBA sprite texture with transparent border pixels and opaque center.
	pixels := []byte{
		0, 0, 0, 0, 255, 255, 255, 255,
		255, 255, 255, 255, 0, 0, 0, 0,
	}
	texID, err := app.Batcher2D.CreateTextureRGBA(2, 2, pixels)
	if err != nil {
		t.Fatalf("CreateTextureRGBA failed: %v", err)
	}
	defer app.Batcher2D.glAPI.DeleteTexture(texID)

	for i := 0; i < 3; i++ {
		go2d := NewGameObject("sprite_"+string(rune('a'+i)), "Sprite")
		go2d.Transform.Position = NewVector3(float32(i*40), 20, 0)
		r2d := NewRenderable2D(texID, 32, 32)
		r2d.ZIndex = int32(i)
		go2d.AddComponent(r2d)
		scene.AddObject(go2d)
	}
	scene.Init()
	defer scene.Unload()

	app.StepFrame(0.016)

	if app.Batcher2D.Stats.DrawCalls != 1 {
		t.Errorf("expected 1 batched draw call for 3 quads sharing texID %d, got %d", texID, app.Batcher2D.Stats.DrawCalls)
	}
	if app.Batcher2D.Stats.QuadsRendered != 3 {
		t.Errorf("expected 3 quads rendered, got %d", app.Batcher2D.Stats.QuadsRendered)
	}
}
