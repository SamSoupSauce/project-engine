package props

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/components"
	"github.com/SamSoupSauce/project-engine/engine/hazards"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

const (
	luaVec3TypeName       = "PropVec3"
	luaTransformCompName  = "TransformComponent"
	luaMeshCompName       = "MeshRendererComponent"
	luaRigidBodyCompName  = "RigidBodyComponent"
	luaConstraintCompName = "ConstraintComponent"
	luaHazardCompName     = "HazardComponent"
)

// AcousticAlertRecord logs an Engine.Audio.BroadcastAcousticAlert dispatch from Lua.
type AcousticAlertRecord struct {
	Origin       engine.Vector3
	Radius       float32
	SoundProfile string
}

// SplinterSpawnRecord logs an Engine.Debris.SpawnSplinters call from Lua.
type SplinterSpawnRecord struct {
	Origin   engine.Vector3
	Count    int
	Material string
	Velocity engine.Vector3
}

// PropManifest stores parsed declarative metadata and Lua hooks from a scripts/props/*.lua file.
type PropManifest struct {
	ID         string
	ScriptPath string
	Name       string
	ModelPath  string
	Material   string
	TwoHanded  bool
	Table      *lua.LTable
}

// PropRegistry manages scanning/caching Lua prop manifests, spawning generic entity instances,
// exposing the global `Engine` Lua API, and routing lifecycle/collision/joint hooks back into Lua.
type PropRegistry struct {
	L              *lua.LState
	World          *spatial.PhysicsWorld
	Scene          engine.Scene
	HazardSys      *hazards.HazardSystem
	ValidateAssets bool

	Manifests       map[string]*PropManifest
	Entities        map[string]*engine.GameObject
	EntityManifests map[string]string
	HandVelocity    engine.Vector3
	AcousticAlerts  []AcousticAlertRecord
	SplinterSpawns  []SplinterSpawnRecord
	DebrisObjects   []*engine.GameObject

	entitySeq int
	debrisSeq int
}

// NewPropRegistry allocates a PropRegistry and registers standard component & Engine APIs on L.
func NewPropRegistry(L *lua.LState, world *spatial.PhysicsWorld, scene engine.Scene, hazardSys *hazards.HazardSystem) *PropRegistry {
	if hazardSys == nil {
		hazardSys = hazards.NewHazardSystem(world, scene)
	}
	reg := &PropRegistry{
		L:               L,
		World:           world,
		Scene:           scene,
		HazardSys:       hazardSys,
		ValidateAssets:  true,
		Manifests:       make(map[string]*PropManifest),
		Entities:        make(map[string]*engine.GameObject),
		EntityManifests: make(map[string]string),
	}
	reg.registerLuaBindings()
	return reg
}

// LoadDirectory scans all *.lua manifests in dirPath, validates referenced .gltf/.glb models,
// and caches their declarative tables + hooks by prop ID (filename stem).
func (r *PropRegistry) LoadDirectory(dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("props: read dir %q: %w", dirPath, err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".lua") {
			continue
		}
		propID := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		fullPath := filepath.Join(dirPath, entry.Name())
		if _, err := r.LoadManifestFile(propID, fullPath); err != nil {
			return err
		}
	}
	return nil
}

// LoadManifestFile executes a single prop manifest .lua script and caches its returned table.
func (r *PropRegistry) LoadManifestFile(propID, scriptPath string) (*PropManifest, error) {
	if err := r.L.DoFile(scriptPath); err != nil {
		return nil, fmt.Errorf("props: load %q: %w", scriptPath, err)
	}
	tbl, ok := r.L.Get(-1).(*lua.LTable)
	r.L.Pop(1)
	if !ok || tbl == nil {
		return nil, fmt.Errorf("props: %q did not return a manifest table", scriptPath)
	}

	metaTbl, _ := tbl.RawGetString("meta").(*lua.LTable)
	var name, modelPath, material string
	if metaTbl != nil {
		name = metaTbl.RawGetString("name").String()
		modelPath = metaTbl.RawGetString("model").String()
		material = metaTbl.RawGetString("material").String()
	}
	twoHanded := false
	if carryTbl, ok := tbl.RawGetString("carry").(*lua.LTable); ok && carryTbl != nil {
		twoHanded = lua.LVAsBool(carryTbl.RawGetString("two_handed"))
	}

	if r.ValidateAssets {
		if modelPath == "" {
			return nil, fmt.Errorf("props: %q meta.model is empty", scriptPath)
		}
		if _, err := engine.LoadGLTFAsset(modelPath); err != nil {
			return nil, fmt.Errorf("props: validate asset for %q: %w", propID, err)
		}
	}

	m := &PropManifest{
		ID:         propID,
		ScriptPath: scriptPath,
		Name:       name,
		ModelPath:  modelPath,
		Material:   material,
		TwoHanded:  twoHanded,
		Table:      tbl,
	}
	r.Manifests[propID] = m
	return m, nil
}

// CreateEntity spawns a base GameObject outfitted with standard engine components and registers it under a unique ID.
func (r *PropRegistry) CreateEntity(name string) *engine.GameObject {
	r.entitySeq++
	id := fmt.Sprintf("entity_%d", r.entitySeq)
	if name == "" {
		name = id
	}

	obj := engine.NewGameObject(id, name)
	obj.AddComponent(components.NewTransformComponent())
	obj.AddComponent(components.NewMeshRendererComponent())
	obj.AddComponent(components.NewRigidBodyComponent(r.World))
	obj.AddComponent(components.NewConstraintComponent(r.World))
	if name == hazards.HazardShardTag || strings.Contains(name, "Hazard") {
		obj.AddComponent(components.NewHazardComponent(r.HazardSys))
	}

	if r.Scene != nil {
		if adder, ok := r.Scene.(interface{ AddObject(*engine.GameObject) }); ok {
			adder.AddObject(obj)
		}
	}
	r.Entities[id] = obj
	return obj
}

// DestroyEntity removes an entity from active scene/physics and marks it inactive.
func (r *PropRegistry) DestroyEntity(entityID string) {
	obj, ok := r.Entities[entityID]
	if !ok || obj == nil {
		return
	}
	delete(r.Entities, entityID)
	delete(r.EntityManifests, entityID)
	obj.Destroy()
}

// SpawnProp instantiates an entity from a loaded Lua prop manifest at pos and triggers manifest.on_spawn(entity_id).
func (r *PropRegistry) SpawnProp(propID string, pos engine.Vector3) (string, error) {
	m, ok := r.Manifests[propID]
	if !ok {
		return "", fmt.Errorf("props: unknown prop manifest %q", propID)
	}
	obj := r.CreateEntity(propID)
	obj.EnsureTransform().SetPosition(pos)
	r.EntityManifests[obj.ID] = propID

	if fn, ok := m.Table.RawGetString("on_spawn").(*lua.LFunction); ok && fn != nil {
		if err := r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, lua.LString(obj.ID)); err != nil {
			return "", fmt.Errorf("props: %s.on_spawn failed: %w", propID, err)
		}
	}

	// Ensure physics body inherits spawned world coordinates if rebuilt by Lua.
	if rbc := getComponent[*components.RigidBodyComponent](obj); rbc != nil && rbc.Body != nil {
		rbc.Body.SetPosition(pos)
	}
	return obj.ID, nil
}

// DispatchCollision invokes manifest.on_collision(entity_id, hit_data) for the entity's Lua definition.
func (r *PropRegistry) DispatchCollision(entityID string, point, normal engine.Vector3, impulse float32) error {
	propID, ok := r.EntityManifests[entityID]
	if !ok {
		return nil
	}
	m, ok := r.Manifests[propID]
	if !ok {
		return nil
	}
	fn, ok := m.Table.RawGetString("on_collision").(*lua.LFunction)
	if !ok || fn == nil {
		return nil
	}

	hitTbl := r.L.NewTable()
	r.L.SetField(hitTbl, "impulse", lua.LNumber(impulse))
	r.L.SetField(hitTbl, "point", r.newVec3UD(point))
	r.L.SetField(hitTbl, "normal", r.newVec3UD(normal))
	return r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, lua.LString(entityID), hitTbl)
}

// DispatchJointBreak invokes manifest.on_joint_break(entity_id, parent_id).
func (r *PropRegistry) DispatchJointBreak(entityID, parentID string) error {
	propID, ok := r.EntityManifests[entityID]
	if !ok {
		return nil
	}
	m, ok := r.Manifests[propID]
	if !ok {
		return nil
	}
	fn, ok := m.Table.RawGetString("on_joint_break").(*lua.LFunction)
	if !ok || fn == nil {
		return nil
	}
	return r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, lua.LString(entityID), lua.LString(parentID))
}

func (r *PropRegistry) registerLuaBindings() {
	r.registerVec3Type()
	r.registerComponentMetatables()

	engineTbl := r.L.NewTable()
	r.L.SetField(engineTbl, "CreateEntity", r.L.NewFunction(func(L *lua.LState) int {
		name := L.OptString(1, "Entity")
		obj := r.CreateEntity(name)
		L.Push(lua.LString(obj.ID))
		return 1
	}))
	r.L.SetField(engineTbl, "DestroyEntity", r.L.NewFunction(func(L *lua.LState) int {
		r.DestroyEntity(L.CheckString(1))
		return 0
	}))
	r.L.SetField(engineTbl, "GetComponent", r.L.NewFunction(func(L *lua.LState) int {
		entityID := L.CheckString(1)
		compName := L.CheckString(2)
		obj := r.Entities[entityID]
		if obj == nil {
			L.Push(lua.LNil)
			return 1
		}
		return r.pushComponentByName(L, obj, compName)
	}))

	audioTbl := r.L.NewTable()
	r.L.SetField(audioTbl, "BroadcastAcousticAlert", r.L.NewFunction(func(L *lua.LState) int {
		cfg := L.CheckTable(1)
		origin := luaValueToVec3(cfg.RawGetString("origin"))
		radius := float32(lua.LVAsNumber(cfg.RawGetString("radius")))
		profile := cfg.RawGetString("sound_profile").String()
		rec := AcousticAlertRecord{Origin: origin, Radius: radius, SoundProfile: profile}
		r.AcousticAlerts = append(r.AcousticAlerts, rec)
		if r.HazardSys != nil && radius > 0 {
			for _, z := range r.HazardSys.Zombies {
				if z == nil {
					continue
				}
				dx := z.Position.X - origin.X
				dy := z.Position.Y - origin.Y
				dz := z.Position.Z - origin.Z
				distSq := dx*dx + dy*dy + dz*dz
				if distSq <= radius*radius {
					z.Alerted = true
					z.AlertTarget = origin
					z.LastSoundID = profile
					z.AlertDistance = float32(math.Sqrt(float64(distSq)))
				}
			}
		}
		return 0
	}))
	r.L.SetField(engineTbl, "Audio", audioTbl)

	debrisTbl := r.L.NewTable()
	r.L.SetField(debrisTbl, "SpawnSplinters", r.L.NewFunction(func(L *lua.LState) int {
		cfg := L.CheckTable(1)
		origin := luaValueToVec3(cfg.RawGetString("origin"))
		count := int(lua.LVAsNumber(cfg.RawGetString("count")))
		if count <= 0 {
			count = 6
		}
		material := cfg.RawGetString("material").String()
		vel := luaValueToVec3(cfg.RawGetString("velocity"))
		r.SplinterSpawns = append(r.SplinterSpawns, SplinterSpawnRecord{
			Origin:   origin,
			Count:    count,
			Material: material,
			Velocity: vel,
		})
		for i := 0; i < count; i++ {
			r.debrisSeq++
			ang := float64(i) * (2.0 * math.Pi / float64(count))
			id := fmt.Sprintf("splinter_%d", r.debrisSeq)
			frag := engine.NewGameObject(id, id)
			frag.EnsureTransform().SetPosition(engine.NewVector3(
				origin.X+float32(math.Cos(ang))*0.15,
				origin.Y+0.05,
				origin.Z+float32(math.Sin(ang))*0.15,
			))
			frag.AddComponent(engine.NewRenderable("splinter_mesh", material))
			if r.World != nil {
				rb := spatial.NewRigidBody(id+"_rb", r.World, engine.BodyDynamic, spatial.BoxShapeDesc(0.06, 0.015, 0.02))
				rb.Mass = 0.075
				rb.ObjectLayer = spatial.LayerProp
				rb.HasCustomLayer = true
				frag.AddComponent(rb)
				rb.SetVelocity(engine.NewVector3(vel.X+float32(math.Cos(ang))*1.5, vel.Y+1.0, vel.Z+float32(math.Sin(ang))*1.5))
			}
			r.DebrisObjects = append(r.DebrisObjects, frag)
		}
		return 0
	}))
	r.L.SetField(engineTbl, "Debris", debrisTbl)

	playerTbl := r.L.NewTable()
	r.L.SetField(playerTbl, "GetHandVelocity", r.L.NewFunction(func(L *lua.LState) int {
		L.Push(r.newVec3UD(r.HandVelocity))
		return 1
	}))
	r.L.SetField(engineTbl, "Player", playerTbl)

	r.L.SetGlobal("Engine", engineTbl)
}

func (r *PropRegistry) registerVec3Type() {
	mt := r.L.NewTypeMetatable(luaVec3TypeName)
	r.L.SetField(mt, "__mul", r.L.NewFunction(func(L *lua.LState) int {
		v := luaValueToVec3(L.Get(1))
		s := float32(L.CheckNumber(2))
		L.Push(r.newVec3UD(engine.NewVector3(v.X*s, v.Y*s, v.Z*s)))
		return 1
	}))
	r.L.SetField(mt, "__index", r.L.NewFunction(func(L *lua.LState) int {
		v := luaValueToVec3(L.Get(1))
		switch L.CheckString(2) {
		case "x", "X":
			L.Push(lua.LNumber(v.X))
		case "y", "Y":
			L.Push(lua.LNumber(v.Y))
		case "z", "Z":
			L.Push(lua.LNumber(v.Z))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
}

func (r *PropRegistry) newVec3UD(v engine.Vector3) *lua.LUserData {
	ud := r.L.NewUserData()
	ud.Value = v
	r.L.SetMetatable(ud, r.L.GetTypeMetatable(luaVec3TypeName))
	return ud
}

func (r *PropRegistry) registerComponentMetatables() {
	// TransformComponent
	tmt := r.L.NewTypeMetatable(luaTransformCompName)
	tMethods := r.L.NewTable()
	r.L.SetField(tMethods, "SetPosition", r.L.NewFunction(func(L *lua.LState) int {
		tc := L.CheckUserData(1).Value.(*components.TransformComponent)
		tc.SetPosition(luaValueToVec3(L.Get(2)))
		return 0
	}))
	r.L.SetField(tMethods, "GetPosition", r.L.NewFunction(func(L *lua.LState) int {
		tc := L.CheckUserData(1).Value.(*components.TransformComponent)
		L.Push(r.newVec3UD(tc.GetPosition()))
		return 1
	}))
	r.L.SetField(tmt, "__index", tMethods)

	// MeshRendererComponent
	mmt := r.L.NewTypeMetatable(luaMeshCompName)
	mMethods := r.L.NewTable()
	r.L.SetField(mMethods, "LoadGLTF", r.L.NewFunction(func(L *lua.LState) int {
		mc := L.CheckUserData(1).Value.(*components.MeshRendererComponent)
		if err := mc.LoadGLTF(L.CheckString(2)); err != nil {
			L.RaiseError("LoadGLTF failed: %v", err)
		}
		return 0
	}))
	r.L.SetField(mMethods, "SetMesh", r.L.NewFunction(func(L *lua.LState) int {
		mc := L.CheckUserData(1).Value.(*components.MeshRendererComponent)
		mc.SetMesh(L.CheckString(2))
		return 0
	}))
	r.L.SetField(mMethods, "SetVisible", r.L.NewFunction(func(L *lua.LState) int {
		mc := L.CheckUserData(1).Value.(*components.MeshRendererComponent)
		mc.SetVisible(L.CheckBool(2))
		return 0
	}))
	r.L.SetField(mMethods, "SetCastShadow", r.L.NewFunction(func(L *lua.LState) int {
		mc := L.CheckUserData(1).Value.(*components.MeshRendererComponent)
		mc.SetCastShadow(L.CheckBool(2))
		return 0
	}))
	r.L.SetField(mMethods, "SetMaterialOverride", r.L.NewFunction(func(L *lua.LState) int {
		mc := L.CheckUserData(1).Value.(*components.MeshRendererComponent)
		mc.SetMaterialOverride(L.CheckInt(2), L.CheckString(3))
		return 0
	}))
	r.L.SetField(mmt, "__index", mMethods)

	// RigidBodyComponent
	rmt := r.L.NewTypeMetatable(luaRigidBodyCompName)
	rMethods := r.L.NewTable()
	r.L.SetField(rMethods, "SetMass", r.L.NewFunction(func(L *lua.LState) int {
		rb := L.CheckUserData(1).Value.(*components.RigidBodyComponent)
		rb.SetMass(float32(L.CheckNumber(2)))
		return 0
	}))
	r.L.SetField(rMethods, "SetMaterial", r.L.NewFunction(func(L *lua.LState) int {
		rb := L.CheckUserData(1).Value.(*components.RigidBodyComponent)
		rb.SetMaterial(L.CheckString(2))
		return 0
	}))
	r.L.SetField(rMethods, "SetShape", r.L.NewFunction(func(L *lua.LState) int {
		rb := L.CheckUserData(1).Value.(*components.RigidBodyComponent)
		rb.SetShape(L.CheckString(2))
		return 0
	}))
	r.L.SetField(rMethods, "SetLinearVelocity", r.L.NewFunction(func(L *lua.LState) int {
		rb := L.CheckUserData(1).Value.(*components.RigidBodyComponent)
		rb.SetLinearVelocity(luaValueToVec3(L.Get(2)))
		return 0
	}))
	r.L.SetField(rMethods, "AddImpulse", r.L.NewFunction(func(L *lua.LState) int {
		rb := L.CheckUserData(1).Value.(*components.RigidBodyComponent)
		rb.AddImpulse(luaValueToVec3(L.Get(2)))
		return 0
	}))
	r.L.SetField(rMethods, "SetDynamic", r.L.NewFunction(func(L *lua.LState) int {
		rb := L.CheckUserData(1).Value.(*components.RigidBodyComponent)
		rb.SetDynamic()
		return 0
	}))
	r.L.SetField(rMethods, "SetKinematic", r.L.NewFunction(func(L *lua.LState) int {
		rb := L.CheckUserData(1).Value.(*components.RigidBodyComponent)
		rb.SetKinematic()
		return 0
	}))
	r.L.SetField(rmt, "__index", rMethods)

	// ConstraintComponent
	cmt := r.L.NewTypeMetatable(luaConstraintCompName)
	cMethods := r.L.NewTable()
	r.L.SetField(cMethods, "SetPrismaticAxis", r.L.NewFunction(func(L *lua.LState) int {
		cc := L.CheckUserData(1).Value.(*components.ConstraintComponent)
		cc.SetPrismaticAxis(luaValueToVec3(L.Get(2)))
		return 0
	}))
	r.L.SetField(cMethods, "SetHingeAxis", r.L.NewFunction(func(L *lua.LState) int {
		cc := L.CheckUserData(1).Value.(*components.ConstraintComponent)
		cc.SetHingeAxis(luaValueToVec3(L.Get(2)))
		return 0
	}))
	r.L.SetField(cMethods, "SetLimits", r.L.NewFunction(func(L *lua.LState) int {
		cc := L.CheckUserData(1).Value.(*components.ConstraintComponent)
		cc.SetLimits(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)))
		return 0
	}))
	r.L.SetField(cMethods, "SetBreakForce", r.L.NewFunction(func(L *lua.LState) int {
		cc := L.CheckUserData(1).Value.(*components.ConstraintComponent)
		cc.SetBreakForce(float32(L.CheckNumber(2)))
		return 0
	}))
	r.L.SetField(cMethods, "SetBreakTorque", r.L.NewFunction(func(L *lua.LState) int {
		cc := L.CheckUserData(1).Value.(*components.ConstraintComponent)
		cc.SetBreakTorque(float32(L.CheckNumber(2)))
		return 0
	}))
	r.L.SetField(cMethods, "Attach", r.L.NewFunction(func(L *lua.LState) int {
		cc := L.CheckUserData(1).Value.(*components.ConstraintComponent)
		parent := r.Entities[L.CheckString(2)]
		if err := cc.Attach(parent); err != nil {
			L.RaiseError("ConstraintComponent.Attach: %v", err)
		}
		return 0
	}))
	r.L.SetField(cmt, "__index", cMethods)

	// HazardComponent
	hmt := r.L.NewTypeMetatable(luaHazardCompName)
	hMethods := r.L.NewTable()
	r.L.SetField(hMethods, "Configure", r.L.NewFunction(func(L *lua.LState) int {
		hc := L.CheckUserData(1).Value.(*components.HazardComponent)
		cfgTbl := L.CheckTable(2)
		cfg := components.HazardConfig{
			HazardType:   cfgTbl.RawGetString("hazard_type").String(),
			BleedChance:  float32(lua.LVAsNumber(cfgTbl.RawGetString("bleed_chance"))),
			CrunchRadius: float32(lua.LVAsNumber(cfgTbl.RawGetString("crunch_radius"))),
			Bounds:       luaValueToVec3(cfgTbl.RawGetString("bounds")),
			Position:     luaValueToVec3(cfgTbl.RawGetString("position")),
		}
		hc.Configure(cfg)
		return 0
	}))
	r.L.SetField(hmt, "__index", hMethods)
}

func (r *PropRegistry) pushComponentByName(L *lua.LState, obj *engine.GameObject, name string) int {
	var val any
	switch name {
	case luaTransformCompName:
		val = getComponent[*components.TransformComponent](obj)
	case luaMeshCompName:
		val = getComponent[*components.MeshRendererComponent](obj)
	case luaRigidBodyCompName:
		val = getComponent[*components.RigidBodyComponent](obj)
	case luaConstraintCompName:
		val = getComponent[*components.ConstraintComponent](obj)
	case luaHazardCompName:
		hc := getComponent[*components.HazardComponent](obj)
		if hc == nil {
			hc = components.NewHazardComponent(r.HazardSys)
			obj.AddComponent(hc)
		}
		val = hc
	default:
		L.Push(lua.LNil)
		return 1
	}

	if val == nil {
		L.Push(lua.LNil)
		return 1
	}
	ud := L.NewUserData()
	ud.Value = val
	L.SetMetatable(ud, L.GetTypeMetatable(name))
	L.Push(ud)
	return 1
}

func getComponent[T engine.Component](obj *engine.GameObject) T {
	var zero T
	if obj == nil {
		return zero
	}
	comp := obj.GetComponent(reflect.TypeOf(zero))
	if typed, ok := comp.(T); ok {
		return typed
	}
	return zero
}

func luaValueToVec3(lv lua.LValue) engine.Vector3 {
	if lv == nil || lv == lua.LNil {
		return engine.Vector3{}
	}
	if ud, ok := lv.(*lua.LUserData); ok {
		if v, ok := ud.Value.(engine.Vector3); ok {
			return v
		}
	}
	if tbl, ok := lv.(*lua.LTable); ok {
		return engine.NewVector3(
			float32(lua.LVAsNumber(tbl.RawGetInt(1))),
			float32(lua.LVAsNumber(tbl.RawGetInt(2))),
			float32(lua.LVAsNumber(tbl.RawGetInt(3))),
		)
	}
	return engine.Vector3{}
}
