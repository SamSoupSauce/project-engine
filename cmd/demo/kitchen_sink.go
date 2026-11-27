package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/hazards"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
	"github.com/SamSoupSauce/project-engine/pkg/tileimporter"
)

const kitchenSinkTMXRelPath = "assets/maps/kitchen_sink.tmx"

type demoPropSpec struct {
	ID          string
	Name        string
	DisplayName string
	DefaultVerb string
	ModelRel    string
	Position    engine.Vector3
	Scale       engine.Vector3
	Mass        float32
	Fracturable bool
}

var kitchenSinkPropSpecs = []demoPropSpec{
	{
		ID:          "rainbow_cube",
		Name:        "RainbowCube",
		DisplayName: "Rainbow Calibration Cube",
		DefaultVerb: "Inspect",
		ModelRel:    "assets/models/props/rainbow_cube.gltf",
		Position:    engine.NewVector3(-1.8, 0.95, 0.2),
		Scale:       engine.NewVector3(0.85, 0.85, 0.85),
		Mass:        1.5,
	},
	{
		ID:          "kitchen_toaster",
		Name:        "KitchenToaster",
		DisplayName: "Four-Slice Chrome Toaster",
		DefaultVerb: "Toast",
		ModelRel:    "assets/models/props/kitchen_toaster.gltf",
		Position:    engine.NewVector3(-0.4, 0.55, 0.5),
		Scale:       engine.NewVector3(2.2, 2.2, 2.2),
		Mass:        2.0,
	},
	{
		ID:          "lamp_ceramic",
		Name:        "CeramicLamp",
		DisplayName: "Ceramic Table Lamp",
		DefaultVerb: "Toggle",
		ModelRel:    "assets/models/props/lamp_ceramic.gltf",
		Position:    engine.NewVector3(0.9, 0.55, 0.5),
		Scale:       engine.NewVector3(1.8, 1.8, 1.8),
		Mass:        2.4,
		Fracturable: true,
	},
	{
		ID:          "nightstand_drawer",
		Name:        "NightstandDrawer",
		DisplayName: "Nightstand Drawer",
		DefaultVerb: "Pull",
		ModelRel:    "assets/models/props/nightstand_drawer.gltf",
		Position:    engine.NewVector3(2.2, 0.55, 0.5),
		Scale:       engine.NewVector3(1.8, 1.8, 1.8),
		Mass:        1.1,
		Fracturable: true,
	},
}

// TileMapStats summarizes parsed 3D tile primitives rendered in the kitchen-sink scene.
type TileMapStats struct {
	MapLoaded   bool
	SourcePath  string
	TotalTiles  int
	FloorCount  int
	WallCount   int
	PillarCount int
	StairCount  int
	ColliderCnt int
}

// KitchenSinkDemoScene is an OpenGL 3.3 Core kitchen-sink showcase combining:
//   - 3D TMX tile maps via pkg/tileimporter (floors, exterior/interior walls, corners, pillars, stairs)
//   - All 4 glTF props (rainbow_cube, kitchen_toaster, lamp_ceramic, nightstand_drawer)
//   - Jolt PhysicsWorld static map colliders, dynamic rigid bodies, interactive verbs, and fracture hazards
//   - 2D HUD Renderable2D accents + crosshair rendered via OrthoBatcher2D
type KitchenSinkDemoScene struct {
	*engine.BaseScene

	glCtx          *engine.GLContext
	paths          engine.ResolvedWorkspacePaths
	physicsWorld   *spatial.PhysicsWorld
	hazardSys      *hazards.HazardSystem
	worldMap       *tileimporter.WorldMap3D
	tileStats      TileMapStats
	tileMapObj     *engine.GameObject
	cubeObj        *engine.GameObject
	cubeTransform  *engine.TransformComponent
	propObjects    map[string]*engine.GameObject
	hudBarObj      *engine.GameObject
	hudStatusObj   *engine.GameObject
	crosshair      *engine.GameObject
	warnings       []string

	rotDeg        float32
	rotSpeed      float32
	autoRotate    bool
	showWireframe bool
	showTiles     bool
	simulatePhys  bool
	lampFractured bool

	orbitYawDeg   float32
	orbitPitchDeg float32
	orbitDistance float32
	cameraTarget  engine.Vector3
	hasLastMouse  bool
	lastMouseX    float64
	lastMouseY    float64
}

func newKitchenSinkDemoScene(glCtx *engine.GLContext, paths engine.ResolvedWorkspacePaths) *KitchenSinkDemoScene {
	base := engine.NewBaseScene("Kitchen Sink Demo Scene")
	s := &KitchenSinkDemoScene{
		BaseScene:     base,
		glCtx:         glCtx,
		paths:         paths,
		propObjects:   make(map[string]*engine.GameObject),
		rotSpeed:      48.0,
		autoRotate:    true,
		showTiles:     true,
		simulatePhys:  true,
		orbitYawDeg:   28.0,
		orbitPitchDeg: 28.0,
		orbitDistance: 9.5,
		cameraTarget:  engine.NewVector3(0.2, 0.7, 0.0),
	}

	if pw, err := spatial.NewPhysicsWorld(spatial.DefaultPhysicsWorldConfig()); err == nil {
		s.physicsWorld = pw
		s.hazardSys = hazards.NewHazardSystem(pw, base)
	} else {
		s.warnings = append(s.warnings, fmt.Sprintf("physics disabled: %v", err))
	}

	s.loadTileMapStage()
	s.spawnShowcaseProps()
	s.setupHUDOverlays()
	s.updateCameraPosition()
	return s
}

func (s *KitchenSinkDemoScene) loadTileMapStage() {
	tmxPath := s.paths.ResolvePath(kitchenSinkTMXRelPath)
	raw, err := os.ReadFile(tmxPath)
	if err != nil {
		s.warnings = append(s.warnings, fmt.Sprintf("TMX map %q not found (%v); using minimal tile stage", filepath.ToSlash(tmxPath), err))
		return
	}
	parsed, err := tileimporter.ParseTMX(raw)
	if err != nil {
		s.warnings = append(s.warnings, fmt.Sprintf("TMX parse failed (%v); skipping tile stage", err))
		return
	}

	worldMap := tileimporter.NewWorldMap3D(parsed, 1.25, -0.08)
	s.worldMap = worldMap
	s.tileStats = computeTileMapStats(tmxPath, worldMap)

	if s.physicsWorld != nil {
		if _, colErr := spatial.BuildWorldMapCollider(s.physicsWorld, worldMap); colErr != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("world map collider error: %v", colErr))
		}
	}

	tileAsset := BuildTileMapGLTFAsset(worldMap)
	if tileAsset != nil {
		tileObj := engine.NewGameObject("kitchen_sink_tiles", "TileMapWorld")
		tileObj.AddComponent(engine.NewTransformComponent())
		mc := &engine.MeshComponent{
			FilePath: filepath.ToSlash(tmxPath),
			Asset:    tileAsset,
		}
		mr := engine.NewMeshRenderable()
		tileObj.AddComponent(mc)
		tileObj.AddComponent(mr)
		s.AddObject(tileObj)
		s.tileMapObj = tileObj
	}
}

func (s *KitchenSinkDemoScene) spawnShowcaseProps() {
	for _, spec := range kitchenSinkPropSpecs {
		modelPath := s.paths.ResolvePath(spec.ModelRel)
		obj := engine.NewGameObject(spec.ID, spec.Name)
		tc := engine.NewTransformComponent()
		tc.SetPosition(spec.Position)
		tc.SetScale(spec.Scale)
		obj.AddComponent(tc)

		mc := engine.NewMeshComponent(modelPath)
		if mc.LoadErr != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("prop %s model %q failed: %v", spec.ID, filepath.ToSlash(modelPath), mc.LoadErr))
		} else {
			obj.AddComponent(mc)
			obj.AddComponent(engine.NewMeshRenderable())
		}

		ic := spatial.NewInteractiveComponent(spec.DisplayName, spec.ID != "rainbow_cube")
		ic.SetDefaultVerb(spatial.VerbToggleState, spec.DefaultVerb, "")
		ic.AddContextVerb(spatial.VerbCustomLua, "Inspect", "")
		if spec.Fracturable {
			ic.AddContextVerb(spatial.VerbDismantle, "Fracture", "")
		}
		obj.AddComponent(ic)

		if spec.Fracturable {
			mode := spatial.FractureShatter
			if spec.ID == "nightstand_drawer" {
				mode = spatial.FractureSplinter
			}
			fc := spatial.NewFractureComponent(mode, spatial.IntegrityLow, 35.0)
			if s.hazardSys != nil && mode == spatial.FractureShatter {
				s.hazardSys.WireFracture(fc)
			}
			obj.AddComponent(fc)
		}

		if s.physicsWorld != nil {
			bodyType := engine.BodyDynamic
			if spec.ID == "rainbow_cube" {
				bodyType = engine.BodyKinematic
			}
			shape := spatial.BoxShapeDesc(0.35, 0.35, 0.35)
			if mc.LoadErr == nil && mc.Asset != nil {
				b := mc.Bounds()
				hx := (b.Max.X - b.Min.X) * 0.5 * spec.Scale.X
				hy := (b.Max.Y - b.Min.Y) * 0.5 * spec.Scale.Y
				hz := (b.Max.Z - b.Min.Z) * 0.5 * spec.Scale.Z
				if hx > 0.02 && hy > 0.02 && hz > 0.02 {
					shape = spatial.BoxShapeDesc(hx, hy, hz)
				}
			}
			rb := spatial.NewRigidBody(spec.ID, s.physicsWorld, bodyType, shape)
			rb.Mass = spec.Mass
			obj.AddComponent(rb)
		}

		s.AddObject(obj)
		s.propObjects[spec.ID] = obj
		if spec.ID == "rainbow_cube" {
			s.cubeObj = obj
			s.cubeTransform = tc
		}
	}
}

func (s *KitchenSinkDemoScene) setupHUDOverlays() {
	s.hudBarObj = engine.NewGameObject("hud_accent", "HudAccentBar")
	s.hudBarObj.EnsureTransform().SetPosition(engine.NewVector3(16, 16, 0))
	bar := engine.NewRenderable2D(0, 240, 6)
	bar.Anchor = engine.AnchorTopLeft
	bar.Color = engine.NewColor(64, 210, 255, 220)
	bar.ZIndex = 10
	s.hudBarObj.AddComponent(bar)
	s.AddObject(s.hudBarObj)

	s.hudStatusObj = engine.NewGameObject("hud_status", "HudStatusStrip")
	s.hudStatusObj.EnsureTransform().SetPosition(engine.NewVector3(16, -24, 0))
	strip := engine.NewRenderable2D(0, 180, 4)
	strip.Anchor = engine.AnchorBottomLeft
	strip.Color = engine.NewColor(120, 240, 160, 210)
	strip.ZIndex = 10
	s.hudStatusObj.AddComponent(strip)
	s.AddObject(s.hudStatusObj)

	s.crosshair = engine.NewGameObject("hud_crosshair", "Crosshair")
	s.crosshair.EnsureTransform()
	dot := engine.NewRenderable2D(0, 6, 6)
	dot.Anchor = engine.AnchorCenter
	dot.Color = engine.NewColor(255, 255, 255, 190)
	dot.ZIndex = 20
	s.crosshair.AddComponent(dot)
	s.AddObject(s.crosshair)
}

func (s *KitchenSinkDemoScene) Init() {
	s.BaseScene.Init()
}

func (s *KitchenSinkDemoScene) Update(dt float32) {
	s.BaseScene.Update(dt)

	if s.simulatePhys && s.physicsWorld != nil && dt > 0 {
		s.physicsWorld.StepSimulation(dt)
		rbType := reflect.TypeOf((*spatial.RigidBody)(nil))
		for id, obj := range s.propObjects {
			if obj == nil || !obj.Active || id == "rainbow_cube" {
				continue
			}
			if body, ok := obj.GetComponent(rbType).(*spatial.RigidBody); ok && body != nil && body.BodyType == engine.BodyDynamic && body.BodyID != spatial.InvalidBodyID {
				pos := body.GetPosition()
				if tc := obj.GetTransformComponent(); tc != nil {
					tc.SetPosition(pos)
				}
			}
		}
	}

	if s.autoRotate && s.cubeTransform != nil {
		s.rotDeg += s.rotSpeed * dt
		if s.rotDeg >= 360 {
			s.rotDeg -= 360
		}
		s.cubeTransform.SetRotation(engine.NewVector3(s.rotDeg*0.55, s.rotDeg, 0))
	}
	s.updateCameraPosition()
}

func (s *KitchenSinkDemoScene) Draw3D() {
	aspect := float32(1280.0 / 720.0)
	if s.glCtx != nil && s.glCtx.FramebufferH > 0 {
		aspect = float32(s.glCtx.FramebufferW) / float32(s.glCtx.FramebufferH)
	}

	proj := mat4Perspective(s.Camera.Fovy, aspect, 0.1, 150.0)
	view := mat4LookAt(s.Camera.Position, s.Camera.Target, s.Camera.Up)
	viewProj := mat4Mul(proj, view)

	if s.tileMapObj != nil {
		if mr := s.tileMapObj.GetMeshRenderable(); mr != nil {
			mr.Visible = s.showTiles
			mr.Wireframe = s.showWireframe
		}
	}
	for _, obj := range s.propObjects {
		if obj != nil {
			if mr := obj.GetMeshRenderable(); mr != nil {
				mr.Wireframe = s.showWireframe
			}
		}
	}
	s.DrawMeshRenderables(viewProj)
}

func (s *KitchenSinkDemoScene) Unload() {
	for _, obj := range s.propObjects {
		if obj != nil {
			if mr := obj.GetMeshRenderable(); mr != nil {
				mr.DisposeGL()
			}
		}
	}
	if s.tileMapObj != nil {
		if mr := s.tileMapObj.GetMeshRenderable(); mr != nil {
			mr.DisposeGL()
		}
	}
	if s.physicsWorld != nil {
		s.physicsWorld.Destroy()
		s.physicsWorld = nil
	}
	s.BaseScene.Unload()
}

func (s *KitchenSinkDemoScene) ResetShowcaseProps() {
	s.rotDeg = 0
	s.lampFractured = false
	rbType := reflect.TypeOf((*spatial.RigidBody)(nil))
	for _, spec := range kitchenSinkPropSpecs {
		obj := s.propObjects[spec.ID]
		if obj == nil {
			continue
		}
		obj.Active = true
		if tc := obj.GetTransformComponent(); tc != nil {
			tc.SetPosition(spec.Position)
			tc.SetRotation(engine.NewVector3(0, 0, 0))
			tc.SetScale(spec.Scale)
		}
		if mr := obj.GetMeshRenderable(); mr != nil {
			mr.Visible = true
		}
		if rb, ok := obj.GetComponent(rbType).(*spatial.RigidBody); ok && rb != nil {
			rb.SetPosition(spec.Position)
			rb.SetVelocity(engine.NewVector3(0, 0, 0))
		}
	}
}

func (s *KitchenSinkDemoScene) PulseShowcaseProps() {
	rbType := reflect.TypeOf((*spatial.RigidBody)(nil))
	for id, obj := range s.propObjects {
		if obj == nil || !obj.Active || id == "rainbow_cube" {
			continue
		}
		if rb, ok := obj.GetComponent(rbType).(*spatial.RigidBody); ok && rb != nil {
			rb.AddImpulse(engine.NewVector3(0.4, 3.2, -0.3))
		}
	}
}

func (s *KitchenSinkDemoScene) FractureDemoLamp() {
	obj := s.propObjects["lamp_ceramic"]
	if obj == nil || s.lampFractured {
		return
	}
	fcType := reflect.TypeOf((*spatial.FractureComponent)(nil))
	if fc, ok := obj.GetComponent(fcType).(*spatial.FractureComponent); ok && fc != nil {
		pos := obj.GetWorldTransform().Position
		fc.ApplyImpact(40.0, pos, engine.NewVector3(0, 1, 0))
	}
	if mr := obj.GetMeshRenderable(); mr != nil {
		mr.Visible = false
	}
	s.lampFractured = true
}

func (s *KitchenSinkDemoScene) SetCameraPreset(name string) {
	switch name {
	case "overview":
		s.orbitYawDeg = 28
		s.orbitPitchDeg = 28
		s.orbitDistance = 9.5
		s.cameraTarget = engine.NewVector3(0.2, 0.7, 0.0)
	case "props":
		s.orbitYawDeg = 8
		s.orbitPitchDeg = 18
		s.orbitDistance = 5.2
		s.cameraTarget = engine.NewVector3(0.3, 0.7, 0.4)
	case "stairs":
		s.orbitYawDeg = -45
		s.orbitPitchDeg = 24
		s.orbitDistance = 6.5
		s.cameraTarget = engine.NewVector3(2.5, 1.0, -1.5)
	}
	s.updateCameraPosition()
}

func (s *KitchenSinkDemoScene) onCursorMove(x, y float64) {
	if s.glCtx == nil || !s.glCtx.CursorLocked {
		s.hasLastMouse = false
		return
	}
	if !s.hasLastMouse {
		s.lastMouseX = x
		s.lastMouseY = y
		s.hasLastMouse = true
		return
	}
	dx := float32(x - s.lastMouseX)
	dy := float32(y - s.lastMouseY)
	s.lastMouseX = x
	s.lastMouseY = y

	s.orbitYawDeg -= dx * 0.22
	s.orbitPitchDeg += dy * 0.22
	if s.orbitPitchDeg > 80 {
		s.orbitPitchDeg = 80
	} else if s.orbitPitchDeg < -80 {
		s.orbitPitchDeg = -80
	}
}

func (s *KitchenSinkDemoScene) updateCameraPosition() {
	yaw := float64(degToRad(s.orbitYawDeg))
	pitch := float64(degToRad(s.orbitPitchDeg))
	d := float64(s.orbitDistance)

	s.Camera.Position = engine.NewVector3(
		s.cameraTarget.X+float32(d*math.Cos(pitch)*math.Sin(yaw)),
		s.cameraTarget.Y+float32(d*math.Sin(pitch)),
		s.cameraTarget.Z+float32(d*math.Cos(pitch)*math.Cos(yaw)),
	)
	s.Camera.Target = s.cameraTarget
	s.Camera.Up = engine.NewVector3(0, 1, 0)
	if s.Camera.Fovy <= 0 {
		s.Camera.Fovy = 50
	}
}

func computeTileMapStats(path string, m3d *tileimporter.WorldMap3D) TileMapStats {
	stats := TileMapStats{
		MapLoaded:  m3d != nil,
		SourcePath: filepath.ToSlash(path),
	}
	if m3d == nil {
		return stats
	}
	stats.TotalTiles = len(m3d.Tiles)
	stats.ColliderCnt = len(m3d.ExportStaticColliderBounds())
	for _, t := range m3d.Tiles {
		if t.HasFlooring {
			stats.FloorCount++
		}
		stats.WallCount += len(t.WallSegments)
		if t.Type == tileimporter.TilePillar {
			stats.PillarCount++
		}
		if t.Type == tileimporter.TileStairs || t.IsStairs {
			stats.StairCount++
		}
	}
	return stats
}

// BuildTileMapGLTFAsset converts a tileimporter.WorldMap3D into an indexed/flat colored GLTFAsset
// renderable by engine.MeshRenderable in OpenGL 3.3 Core without extra shader pipelines.
func BuildTileMapGLTFAsset(m3d *tileimporter.WorldMap3D) *engine.GLTFAsset {
	if m3d == nil || len(m3d.Tiles) == 0 {
		return nil
	}
	prim := engine.GLTFPrimitive{MaterialIndex: -1}
	minB := engine.NewVector3(math.MaxFloat32, math.MaxFloat32, math.MaxFloat32)
	maxB := engine.NewVector3(-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32)

	appendBox := func(cx, cy, cz, sx, sy, sz float32, baseCol [3]float32) {
		if sx <= 0 || sy <= 0 || sz <= 0 {
			return
		}
		hx, hy, hz := sx*0.5, sy*0.5, sz*0.5
		x0, x1 := cx-hx, cx+hx
		y0, y1 := cy-hy, cy+hy
		z0, z1 := cz-hz, cz+hz

		if x0 < minB.X {
			minB.X = x0
		}
		if y0 < minB.Y {
			minB.Y = y0
		}
		if z0 < minB.Z {
			minB.Z = z0
		}
		if x1 > maxB.X {
			maxB.X = x1
		}
		if y1 > maxB.Y {
			maxB.Y = y1
		}
		if z1 > maxB.Z {
			maxB.Z = z1
		}

		addQuad := func(a, b, c, d engine.Vector3, shade float32) {
			col := [4]float32{baseCol[0] * shade, baseCol[1] * shade, baseCol[2] * shade, 1.0}
			prim.Positions = append(prim.Positions, a, b, c, a, c, d)
			for i := 0; i < 6; i++ {
				prim.Colors = append(prim.Colors, col)
			}
		}

		p000 := engine.NewVector3(x0, y0, z0)
		p100 := engine.NewVector3(x1, y0, z0)
		p110 := engine.NewVector3(x1, y1, z0)
		p010 := engine.NewVector3(x0, y1, z0)
		p001 := engine.NewVector3(x0, y0, z1)
		p101 := engine.NewVector3(x1, y0, z1)
		p111 := engine.NewVector3(x1, y1, z1)
		p011 := engine.NewVector3(x0, y1, z1)

		addQuad(p011, p111, p110, p010, 1.00) // Top (+Y)
		addQuad(p000, p100, p101, p001, 0.55) // Bottom (-Y)
		addQuad(p001, p101, p111, p011, 0.85) // Front (+Z)
		addQuad(p100, p000, p010, p110, 0.72) // Back (-Z)
		addQuad(p101, p100, p110, p111, 0.78) // Right (+X)
		addQuad(p000, p001, p011, p010, 0.66) // Left (-X)
	}

	for _, t := range m3d.Tiles {
		if t.HasFlooring {
			col := [3]float32{0.23, 0.27, 0.34}
			if (t.Col+t.Row)%2 == 1 {
				col = [3]float32{0.28, 0.32, 0.40}
			}
			if t.Type == tileimporter.TileGrass {
				col = [3]float32{0.22, 0.48, 0.25}
				if (t.Col+t.Row)%2 == 1 {
					col = [3]float32{0.26, 0.55, 0.29}
				}
			}
			appendBox(t.FloorPosition.X, t.FloorPosition.Y, t.FloorPosition.Z, t.FloorSize.X, t.FloorSize.Y, t.FloorSize.Z, col)
		}

		for _, ws := range t.WallSegments {
			col := [3]float32{0.56, 0.53, 0.48}
			if ws.IsInterior {
				col = [3]float32{0.72, 0.68, 0.60}
			}
			appendBox(ws.Position.X, ws.Position.Y, ws.Position.Z, ws.Size.X, ws.Size.Y, ws.Size.Z, col)
		}

		if t.Type == tileimporter.TilePillar {
			px, pz := t.FloorPosition.X, t.FloorPosition.Z
			baseTop := t.FloorPosition.Y + t.FloorSize.Y*0.5
			h := tileimporter.DefaultWallHeight
			appendBox(px, baseTop+h*0.5, pz, m3d.TileSize*0.42, h, m3d.TileSize*0.42, [3]float32{0.58, 0.44, 0.30})
		}

		if t.Type == tileimporter.TileStairs || t.IsStairs {
			for _, step := range t.Steps {
				appendBox(step.Position.X, step.Position.Y, step.Position.Z, step.Size.X, step.Size.Y, step.Size.Z, [3]float32{0.64, 0.46, 0.28})
			}
		}
	}

	if len(prim.Positions) == 0 {
		return nil
	}
	return &engine.GLTFAsset{
		FilePath: "procedural://kitchen_sink_tiles",
		Version:  "2.0",
		Meshes: []engine.GLTFMesh{
			{
				Name:       "TileMapWorldMesh",
				Primitives: []engine.GLTFPrimitive{prim},
			},
		},
		Bounds: engine.BoundingBox{Min: minB, Max: maxB},
	}
}
