package spatial

import (
	"fmt"
	"math"
	"reflect"

	"github.com/SamSoupSauce/project-engine/engine"
)

// FractureMode defines how a prop behaves when its structural health reaches zero.
type FractureMode uint8

const (
	FractureSplinter FractureMode = iota // Wooden drawers/furniture splintering into matchwood debris
	FractureShatter                      // Glass/ceramic props shattering into persistent floor HazardShards
	FractureDeform                       // Metal pans/toasters denting/deforming cleanly under heavy strikes
)

// PropIntegrityTier differentiates lightweight fragile props from heavy two-handed furniture.
type PropIntegrityTier uint8

const (
	IntegrityLow PropIntegrityTier = iota
	IntegrityStandard
	IntegrityHeavy
)

// DebrisPrefab configures sub-mesh physics fragment spawning upon prop destruction.
type DebrisPrefab struct {
	MeshID      string
	MaterialID  string
	Count       int
	Mass        float32 // Target mass (~0.05-0.1 kg for matchwood/ceramic splinters)
	HalfExtents engine.Vector3
	ScatterVel  float32
}

// DefaultMatchwoodPrefab returns low-mass (~0.075 kg) splinter debris settings.
func DefaultMatchwoodPrefab() DebrisPrefab {
	return DebrisPrefab{
		MeshID:      "matchwood_splinter",
		MaterialID:  "wood_splinter",
		Count:       6,
		Mass:        0.075,
		HalfExtents: engine.NewVector3(0.08, 0.02, 0.03),
		ScatterVel:  3.2,
	}
}

// DefaultCeramicShardPrefab returns ceramic/glass splinter fragment settings.
func DefaultCeramicShardPrefab() DebrisPrefab {
	return DebrisPrefab{
		MeshID:      "ceramic_fragment",
		MaterialID:  "ceramic_shard",
		Count:       5,
		Mass:        0.06,
		HalfExtents: engine.NewVector3(0.06, 0.015, 0.06),
		ScatterVel:  4.0,
	}
}

// FractureEvent describes a completed structural failure and spawned debris fragments.
type FractureEvent struct {
	Source         *engine.GameObject
	Mode           FractureMode
	ImpactPoint    engine.Vector3
	ImpactNormal   engine.Vector3
	ImpactImpulse  float32
	Debris         []*engine.GameObject
	DebrisBodies   []*RigidBody
	SpawnHazard    bool
	DeformedScale  engine.Vector3
}

// StrikeResult describes the kinetic transfer and prop state after hitting an entity capsule.
type StrikeResult struct {
	AppliedImpulse   float32
	StaggerFrames    int
	Knockdown        bool
	PropFractured    bool
	Deformed         bool
	FractureEvent    FractureEvent
}

// FractureComponent attaches structural durability, splinter/shatter debris spawning,
// carry weight rules, and impact strike resolution to a GameObject.
type FractureComponent struct {
	Owner                *engine.GameObject
	World                *PhysicsWorld
	Scene                engine.Scene
	Health               float32
	MaxHealth            float32
	MinDamageImpulse     float32
	FractureMode         FractureMode
	Integrity            PropIntegrityTier
	DebrisPrefab         DebrisPrefab
	RequiredCarryHands   int     // 1 for normal/light props, 2 for heavy nightstands/desks
	CameraLookDamping    float32 // 1.0 = normal, e.g. 0.25 = heavy damping while carrying
	KnockdownImpulse     float32 // High knockdown impulse for heavy props hitting targets
	MinStrikeImpulse     float32 // Low impulse cap when flimsy props hit an entity capsule
	Broken               bool
	DeformCount          int

	OnFracture func(ev FractureEvent)

	body        *RigidBody
	prevVel     engine.Vector3
	hasPrevVel  bool
	debrisSeq   int
}

// NewFractureComponent creates a configured FractureComponent tailored to mode and integrity tier.
func NewFractureComponent(mode FractureMode, integrity PropIntegrityTier, health float32) *FractureComponent {
	if health <= 0 {
		health = 10.0
	}
	fc := &FractureComponent{
		Health:             health,
		MaxHealth:          health,
		MinDamageImpulse:   2.5,
		FractureMode:       mode,
		Integrity:          integrity,
		RequiredCarryHands: 1,
		CameraLookDamping:  1.0,
		KnockdownImpulse:   2.5,
		MinStrikeImpulse:   1.5,
	}

	switch mode {
	case FractureSplinter:
		fc.DebrisPrefab = DefaultMatchwoodPrefab()
	case FractureShatter:
		fc.DebrisPrefab = DefaultCeramicShardPrefab()
	case FractureDeform:
		fc.DebrisPrefab = DebrisPrefab{
			MeshID:      "metal_scrap",
			MaterialID:  "metal",
			Count:       2,
			Mass:        0.1,
			HalfExtents: engine.NewVector3(0.08, 0.03, 0.08),
			ScatterVel:  2.0,
		}
	}

	if integrity == IntegrityHeavy {
		fc.RequiredCarryHands = 2
		fc.CameraLookDamping = 0.25
		fc.KnockdownImpulse = 18.0
		if health < 100 {
			fc.Health = 120
			fc.MaxHealth = 120
		}
	}
	return fc
}

// Init implements engine.Component and binds sibling PhysicsWorld / RigidBody references.
func (fc *FractureComponent) Init(owner *engine.GameObject) {
	fc.Owner = owner
	if owner == nil {
		return
	}
	if fc.Scene == nil && owner.Scene != nil {
		fc.Scene = owner.Scene
	}
	fc.bindRigidBody()
	if ic := FindInteractiveComponent(owner); ic != nil && fc.Integrity == IntegrityHeavy {
		// Propagate heavy two-arm carry tier seamlessly if InteractiveComponent exists.
		_ = ic
	}
}

// Update implements engine.Component, tracking sharp Δv velocity collisions in Jolt.
func (fc *FractureComponent) Update(dt float32) {
	if fc == nil || fc.Broken {
		return
	}
	if fc.body == nil {
		fc.bindRigidBody()
	}
	if fc.body == nil || fc.body.BodyID == InvalidBodyID || dt <= 0 {
		return
	}

	curVel := fc.body.GetVelocity()
	if fc.hasPrevVel {
		dvx := curVel.X - fc.prevVel.X
		dvy := curVel.Y - fc.prevVel.Y
		dvz := curVel.Z - fc.prevVel.Z
		dv := float32(math.Sqrt(float64(dvx*dvx + dvy*dvy + dvz*dvz)))
		mass := fc.body.Mass
		if mass <= 0 {
			mass = 1.0
		}
		impulse := dv * mass
		if impulse >= fc.MinDamageImpulse {
			hitNormal := normVec3(engine.NewVector3(dvx, dvy, dvz))
			fc.ApplyImpact(impulse, fc.body.GetPosition(), hitNormal)
		}
	}
	fc.prevVel = curVel
	fc.hasPrevVel = true
}

// Destroy implements engine.Component.
func (fc *FractureComponent) Destroy() {
	if fc == nil {
		return
	}
	fc.body = nil
	fc.Owner = nil
}

func (fc *FractureComponent) bindRigidBody() {
	if fc.Owner == nil {
		return
	}
	comp := fc.Owner.GetComponent(reflect.TypeOf((*RigidBody)(nil)))
	if rb, ok := comp.(*RigidBody); ok && rb != nil {
		fc.body = rb
		if fc.World == nil && rb.World != nil {
			fc.World = rb.World
		}
	}
}

// ApplyImpact subtracts structural impulse from Health and triggers Splinter/Shatter/Deform when depleted.
func (fc *FractureComponent) ApplyImpact(impulse float32, point, normal engine.Vector3) (FractureEvent, bool) {
	if fc == nil || fc.Broken || impulse < fc.MinDamageImpulse {
		return FractureEvent{}, false
	}
	fc.Health -= impulse
	if fc.Health > 0 {
		return FractureEvent{}, false
	}

	if fc.FractureMode == FractureDeform {
		// Metal props dent/flatten visually and absorb the blow unless obliterated twice below -MaxHealth.
		fc.DeformCount++
		if fc.Owner != nil {
			tc := fc.Owner.EnsureTransform()
			sc := tc.GetScale()
			tc.SetScale(engine.NewVector3(sc.X*1.08, sc.Y*0.82, sc.Z))
		}
		fc.Health = fc.MaxHealth * 0.5
		if fc.DeformCount < 3 {
			return FractureEvent{
				Source:        fc.Owner,
				Mode:          FractureDeform,
				ImpactPoint:   point,
				ImpactNormal:  normal,
				ImpactImpulse: impulse,
				DeformedScale: fc.OwnerTransformScale(),
			}, false
		}
	}

	return fc.Fracture(point, normal, impulse), true
}

// StrikeEntity resolves this prop striking an enemy/player capsule or target RigidBody.
// Lightweight props deliver minimal impulse and shatter into matchwood splinters on hit,
// while heavy props apply strong knockdown impulse without self-destructing.
func (fc *FractureComponent) StrikeEntity(targetBody *RigidBody, targetChar *CharacterController, dir engine.Vector3, impactSpeed float32) StrikeResult {
	if fc == nil || fc.Broken {
		return StrikeResult{}
	}
	d := normVec3(dir)
	if d.X == 0 && d.Y == 0 && d.Z == 0 {
		d = engine.NewVector3(0, 0, -1)
	}
	if impactSpeed <= 0 {
		impactSpeed = 6.0
	}

	var res StrikeResult
	switch fc.Integrity {
	case IntegrityLow:
		// Minimal kinetic impulse and stagger; flimsy wooden drawer disintegrates immediately into matchwood.
		res.AppliedImpulse = fc.MinStrikeImpulse
		if res.AppliedImpulse <= 0 {
			res.AppliedImpulse = 1.5
		}
		res.StaggerFrames = 1
		res.Knockdown = false
		if targetBody != nil {
			targetBody.AddImpulse(engine.NewVector3(d.X*res.AppliedImpulse, d.Y*res.AppliedImpulse+0.25, d.Z*res.AppliedImpulse))
		}
		if targetChar != nil {
			targetChar.SetDesiredVelocity(engine.NewVector3(d.X*1.2, 0, d.Z*1.2))
		}
		point := fc.propPosition()
		ev := fc.Fracture(point, d, fc.Health+fc.MinDamageImpulse)
		res.PropFractured = true
		res.FractureEvent = ev

	case IntegrityHeavy:
		// Heavy nightstand/desk transfers high knockdown impulse without breaking on impact.
		res.AppliedImpulse = fc.KnockdownImpulse
		if res.AppliedImpulse <= 0 {
			res.AppliedImpulse = 18.0
		}
		res.StaggerFrames = 18
		res.Knockdown = true
		if targetBody != nil {
			targetBody.AddImpulse(engine.NewVector3(d.X*res.AppliedImpulse, d.Y*res.AppliedImpulse+4.0, d.Z*res.AppliedImpulse))
		}
		if targetChar != nil {
			targetChar.SetDesiredVelocity(engine.NewVector3(d.X*7.5, 0, d.Z*7.5))
		}
		res.PropFractured = false

	default:
		mass := float32(1.5)
		if fc.body != nil && fc.body.Mass > 0 {
			mass = fc.body.Mass
		}
		imp := mass * impactSpeed
		res.AppliedImpulse = imp
		res.StaggerFrames = 5
		if targetBody != nil {
			targetBody.AddImpulse(engine.NewVector3(d.X*imp, d.Y*imp+1.0, d.Z*imp))
		}
		ev, broke := fc.ApplyImpact(imp, fc.propPosition(), d)
		res.PropFractured = broke
		res.Deformed = fc.FractureMode == FractureDeform && !broke
		res.FractureEvent = ev
	}
	return res
}

// Fracture breaks the prop immediately, converts/spawns DebrisPrefab fragments, and disables/destroys the original body.
func (fc *FractureComponent) Fracture(point, normal engine.Vector3, impulse float32) FractureEvent {
	if fc == nil || fc.Broken {
		return FractureEvent{}
	}
	fc.Broken = true

	pos := point
	if pos.X == 0 && pos.Y == 0 && pos.Z == 0 {
		pos = fc.propPosition()
	}

	ev := FractureEvent{
		Source:        fc.Owner,
		Mode:          fc.FractureMode,
		ImpactPoint:   pos,
		ImpactNormal:  normVec3(normal),
		ImpactImpulse: impulse,
		SpawnHazard:   fc.FractureMode == FractureShatter,
	}

	// Detach from any active physics grab cleanly before tearing down source RigidBody.
	if fc.World != nil && fc.body != nil && fc.World.HeldBody() == fc.body {
		fc.World.ReleaseGrab()
	}

	prefab := fc.DebrisPrefab
	count := prefab.Count
	if count <= 0 {
		count = 4
	}
	mass := prefab.Mass
	if mass < 0.05 {
		mass = 0.05
	} else if mass > 0.1 && (fc.FractureMode == FractureSplinter || fc.FractureMode == FractureShatter) {
		mass = 0.08
	}
	half := prefab.HalfExtents
	if half.X <= 0 || half.Y <= 0 || half.Z <= 0 {
		half = engine.NewVector3(0.07, 0.02, 0.03)
	}
	scatter := prefab.ScatterVel
	if scatter <= 0 {
		scatter = 3.0
	}

	for i := 0; i < count; i++ {
		fc.debrisSeq++
		angle := float64(i) * (2.0 * math.Pi / float64(count))
		dx := float32(math.Cos(angle))
		dz := float32(math.Sin(angle))

		fragID := fmt.Sprintf("debris_%d", fc.debrisSeq)
		if fc.Owner != nil && fc.Owner.ID != "" {
			fragID = fmt.Sprintf("%s_debris_%d", fc.Owner.ID, fc.debrisSeq)
		}
		frag := engine.NewGameObject(fragID, fragID)
		fragTC := frag.EnsureTransform()
		fragTC.SetPosition(engine.NewVector3(
			pos.X+dx*0.18,
			pos.Y+0.06+float32(i%2)*0.05,
			pos.Z+dz*0.18,
		))
		fragTC.SetRotation(engine.NewVector3(0, float32(i*37%360), 0))
		frag.Active = true

		if prefab.MeshID != "" {
			frag.AddComponent(NewRenderableCompat(prefab.MeshID, prefab.MaterialID))
		}

		var fragRB *RigidBody
		if fc.World != nil {
			fragRB = NewRigidBody(fragID+"_rb", fc.World, engine.BodyDynamic, BoxShapeDesc(half.X, half.Y, half.Z))
			fragRB.Mass = mass
			fragRB.ObjectLayer = LayerProp
			fragRB.HasCustomLayer = true
			frag.AddComponent(fragRB)
			fc.World.RegisterSyncTarget(fragRB.BodyID, frag)
			fragRB.SetVelocity(engine.NewVector3(
				dx*scatter+ev.ImpactNormal.X*1.2,
				1.2+float32(i%3)*0.4,
				dz*scatter+ev.ImpactNormal.Z*1.2,
			))
			ev.DebrisBodies = append(ev.DebrisBodies, fragRB)
		}

		if fc.Scene != nil {
			if adder, ok := fc.Scene.(interface{ AddObject(*engine.GameObject) }); ok {
				adder.AddObject(frag)
			}
		}
		ev.Debris = append(ev.Debris, frag)
	}

	if fc.body != nil {
		fc.body.Destroy()
		fc.body = nil
	}
	if fc.Owner != nil {
		fc.Owner.Active = false
		if rComp := fc.Owner.GetComponent(reflect.TypeOf((*engine.Renderable)(nil))); rComp != nil {
			if r, ok := rComp.(*engine.Renderable); ok && r != nil {
				r.SetVisible(false)
				r.SetOutlined(false)
			}
		}
	}

	if fc.OnFracture != nil {
		fc.OnFracture(ev)
	}
	return ev
}

func (fc *FractureComponent) propPosition() engine.Vector3 {
	if fc.body != nil && fc.body.BodyID != InvalidBodyID {
		return fc.body.GetPosition()
	}
	if fc.Owner != nil {
		return fc.Owner.GetWorldTransform().Position
	}
	return engine.Vector3{}
}

func (fc *FractureComponent) OwnerTransformScale() engine.Vector3 {
	if fc.Owner != nil {
		return fc.Owner.GetWorldTransform().Scale
	}
	return engine.NewVector3(1, 1, 1)
}

// NewRenderableCompat creates an engine.Renderable component for debris GameObjects.
func NewRenderableCompat(meshID, materialID string) *engine.Renderable {
	r := engine.NewRenderable(meshID, materialID)
	return r
}

// FindFractureComponent returns the FractureComponent attached to obj, or nil.
func FindFractureComponent(obj *engine.GameObject) *FractureComponent {
	if obj == nil {
		return nil
	}
	comp := obj.GetComponent(reflect.TypeOf((*FractureComponent)(nil)))
	if fc, ok := comp.(*FractureComponent); ok && fc != nil {
		return fc
	}
	return nil
}
