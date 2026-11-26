package components

import (
	"fmt"
	"reflect"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/hazards"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

// TransformComponent aliases engine.TransformComponent so all engine packages share one canonical component.
type TransformComponent = engine.TransformComponent

func NewTransformComponent() *TransformComponent { return engine.NewTransformComponent() }

// MeshRendererComponent manages .gltf/.glb geometry, PBR material overrides, visibility flags,
// and named attachment sockets bound to an engine.Renderable.
type MeshRendererComponent struct {
	Owner             *engine.GameObject
	Renderable        *engine.Renderable
	Asset             *engine.GLTFAsset
	MeshPath          string
	MaterialOverrides map[int]string
	Attachments       map[string]engine.Transform
}

// NewMeshRendererComponent creates a MeshRendererComponent ready for Lua or Go configuration.
func NewMeshRendererComponent() *MeshRendererComponent {
	return &MeshRendererComponent{
		MaterialOverrides: make(map[int]string),
		Attachments:       make(map[string]engine.Transform),
	}
}

// Init binds or creates the sibling engine.Renderable component on owner.
func (m *MeshRendererComponent) Init(owner *engine.GameObject) {
	m.Owner = owner
	if owner == nil {
		return
	}
	if comp := owner.GetComponent(reflect.TypeOf((*engine.Renderable)(nil))); comp != nil {
		if r, ok := comp.(*engine.Renderable); ok && r != nil {
			m.Renderable = r
		}
	}
	if m.Renderable == nil {
		m.Renderable = engine.NewRenderable(m.MeshPath, "default")
		owner.AddComponent(m.Renderable)
	}
}

func (m *MeshRendererComponent) Update(_ float32) {}

func (m *MeshRendererComponent) Destroy() {
	m.Renderable = nil
	m.Owner = nil
}

// LoadGLTF parses a .gltf or .glb model asset, extracts bounds and node sockets, and syncs the Renderable.
func (m *MeshRendererComponent) LoadGLTF(filepath string) error {
	asset, err := engine.LoadGLTFAsset(filepath)
	if err != nil {
		return err
	}
	m.Asset = asset
	m.MeshPath = filepath

	for _, node := range asset.Nodes {
		if node.Name != "" {
			m.Attachments[node.Name] = node.Transform
		}
	}

	if m.Renderable != nil {
		m.Renderable.MeshID = filepath
		size := engine.NewVector3(
			asset.Bounds.Max.X-asset.Bounds.Min.X,
			asset.Bounds.Max.Y-asset.Bounds.Min.Y,
			asset.Bounds.Max.Z-asset.Bounds.Min.Z,
		)
		m.Renderable.CustomSize = &size
		if len(asset.Materials) > 0 && asset.Materials[0].Name != "" {
			m.Renderable.MaterialID = asset.Materials[0].Name
		}
	}
	return nil
}

// SetMesh assigns an explicit mesh handle on the underlying Renderable.
func (m *MeshRendererComponent) SetMesh(meshID string) {
	m.MeshPath = meshID
	if m.Renderable != nil {
		m.Renderable.MeshID = meshID
	}
}

// SetVisible toggles FlagVisible on the underlying Renderable.
func (m *MeshRendererComponent) SetVisible(visible bool) {
	if m.Renderable != nil {
		m.Renderable.SetVisible(visible)
	}
}

// IsVisible returns current visibility of the Renderable.
func (m *MeshRendererComponent) IsVisible() bool {
	return m.Renderable != nil && m.Renderable.IsVisible()
}

// SetCastShadow toggles FlagCastShadow on the underlying Renderable.
func (m *MeshRendererComponent) SetCastShadow(cast bool) {
	if m.Renderable == nil {
		return
	}
	if cast {
		m.Renderable.Flags |= engine.FlagCastShadow
	} else {
		m.Renderable.Flags &^= engine.FlagCastShadow
	}
}

// CastShadow returns whether FlagCastShadow is enabled.
func (m *MeshRendererComponent) CastShadow() bool {
	return m.Renderable != nil && m.Renderable.Flags.Has(engine.FlagCastShadow)
}

// SetMaterialOverride overrides material name for subMeshIndex (and default Renderable if index == 0).
func (m *MeshRendererComponent) SetMaterialOverride(subMeshIndex int, materialID string) {
	if m.MaterialOverrides == nil {
		m.MaterialOverrides = make(map[int]string)
	}
	m.MaterialOverrides[subMeshIndex] = materialID
	if subMeshIndex == 0 && m.Renderable != nil {
		m.Renderable.MaterialID = materialID
	}
}

// RigidBodyComponent wraps a Jolt spatial.RigidBody with Lua-configurable mass, material,
// shape synthesis from sibling GLTF meshes, velocity, impulse, and motion type toggles.
type RigidBodyComponent struct {
	Owner       *engine.GameObject
	World       *spatial.PhysicsWorld
	Body        *spatial.RigidBody
	Material    string
	ShapeName   string
	Mass        float32
	Friction    float32
	Restitution float32
	BodyType    engine.PhysicsBodyType
}

// NewRigidBodyComponent creates a default dynamic RigidBodyComponent bound to world (may be nil for dry tests).
func NewRigidBodyComponent(world *spatial.PhysicsWorld) *RigidBodyComponent {
	return &RigidBodyComponent{
		World:       world,
		Material:    "default",
		ShapeName:   "box",
		Mass:        1.0,
		Friction:    0.5,
		Restitution: 0.1,
		BodyType:    engine.BodyDynamic,
	}
}

// Init attaches or locates a spatial.RigidBody on owner and configures its initial shape.
func (r *RigidBodyComponent) Init(owner *engine.GameObject) {
	r.Owner = owner
	if owner == nil {
		return
	}
	if comp := owner.GetComponent(reflect.TypeOf((*spatial.RigidBody)(nil))); comp != nil {
		if rb, ok := comp.(*spatial.RigidBody); ok && rb != nil {
			r.Body = rb
			if r.World == nil && rb.World != nil {
				r.World = rb.World
			}
			return
		}
	}
	r.EnsureBody()
}

func (r *RigidBodyComponent) Update(_ float32) {}

func (r *RigidBodyComponent) Destroy() {
	if r.Body != nil {
		r.Body.Destroy()
		r.Body = nil
	}
	r.Owner = nil
}

// EnsureBody builds or rebuilds the underlying spatial.RigidBody when a PhysicsWorld is present.
func (r *RigidBodyComponent) EnsureBody() {
	if r.Owner == nil || r.World == nil {
		return
	}
	if r.Body != nil {
		r.Body.Destroy()
		r.Body = nil
	}
	shape := r.buildShapeDesc()
	rb := spatial.NewRigidBody(r.Owner.ID+"_rb", r.World, r.BodyType, shape)
	rb.Mass = r.Mass
	rb.Friction = r.Friction
	rb.Restitution = r.Restitution
	rb.ObjectLayer = spatial.LayerProp
	rb.HasCustomLayer = true
	r.Owner.AddComponent(rb)
	r.World.RegisterSyncTarget(rb.BodyID, r.Owner)
	r.Body = rb
}

func (r *RigidBodyComponent) buildShapeDesc() spatial.ShapeDesc {
	half := engine.NewVector3(0.25, 0.25, 0.25)
	var hull []engine.Vector3
	if r.Owner != nil {
		if comp := r.Owner.GetComponent(reflect.TypeOf((*MeshRendererComponent)(nil))); comp != nil {
			if mesh, ok := comp.(*MeshRendererComponent); ok && mesh != nil && mesh.Asset != nil {
				b := mesh.Asset.Bounds
				hx := (b.Max.X - b.Min.X) * 0.5
				hy := (b.Max.Y - b.Min.Y) * 0.5
				hz := (b.Max.Z - b.Min.Z) * 0.5
				if hx > 0.01 && hy > 0.01 && hz > 0.01 {
					half = engine.NewVector3(hx, hy, hz)
				}
				hull = mesh.Asset.HullPoints
			}
		}
	}

	switch r.ShapeName {
	case "convex_hull":
		if len(hull) >= 4 {
			return spatial.ConvexHullShapeDesc(hull, half)
		}
		return spatial.BoxShapeDesc(half.X, half.Y, half.Z)
	case "sphere":
		rad := half.X
		if half.Y > rad {
			rad = half.Y
		}
		if half.Z > rad {
			rad = half.Z
		}
		return spatial.SphereShapeDesc(rad)
	default:
		return spatial.BoxShapeDesc(half.X, half.Y, half.Z)
	}
}

// SetMass updates mass metadata and rebuilds the Jolt body if attached.
func (r *RigidBodyComponent) SetMass(mass float32) {
	if mass > 0 {
		r.Mass = mass
	}
	if r.Body != nil {
		r.Body.Mass = r.Mass
		if r.World != nil {
			r.EnsureBody()
		}
	}
}

// SetMaterial configures physical surface preset metadata (e.g., "metal_hollow", "wood_light", "ceramic").
func (r *RigidBodyComponent) SetMaterial(mat string) {
	r.Material = mat
	switch mat {
	case "metal_hollow":
		r.Restitution = 0.25
		r.Friction = 0.45
	case "wood_light":
		r.Restitution = 0.12
		r.Friction = 0.55
	case "ceramic":
		r.Restitution = 0.08
		r.Friction = 0.50
	}
	if r.Body != nil {
		r.Body.Friction = r.Friction
		r.Body.Restitution = r.Restitution
	}
}

// SetShape sets collision geometry ("box", "convex_hull", "sphere") and rebuilds if attached.
func (r *RigidBodyComponent) SetShape(shapeName string) {
	r.ShapeName = shapeName
	if r.World != nil && r.Owner != nil {
		r.EnsureBody()
	}
}

func (r *RigidBodyComponent) SetLinearVelocity(v engine.Vector3) {
	if r.Body != nil {
		r.Body.SetVelocity(v)
	}
}

func (r *RigidBodyComponent) GetLinearVelocity() engine.Vector3 {
	if r.Body != nil {
		return r.Body.GetVelocity()
	}
	return engine.Vector3{}
}

func (r *RigidBodyComponent) AddImpulse(imp engine.Vector3) {
	if r.Body != nil {
		r.Body.AddImpulse(imp)
	}
}

func (r *RigidBodyComponent) SetDynamic() {
	r.BodyType = engine.BodyDynamic
	if r.World != nil && r.Owner != nil {
		r.EnsureBody()
	}
}

func (r *RigidBodyComponent) SetKinematic() {
	r.BodyType = engine.BodyKinematic
	if r.World != nil && r.Owner != nil {
		r.EnsureBody()
	}
}

func (r *RigidBodyComponent) BodyID() uint32 {
	if r.Body == nil {
		return spatial.InvalidBodyID
	}
	return r.Body.BodyID
}

// ConstraintComponent exposes 1D slider (prismatic) and revolute (hinge) joints with runtime break limits.
type ConstraintComponent struct {
	Owner        *engine.GameObject
	World        *spatial.PhysicsWorld
	Joint        *spatial.BreakableJoint
	Kind         spatial.JointKind
	Axis         engine.Vector3
	Anchor       engine.Vector3
	MinLimit     float32
	MaxLimit     float32
	BreakForce   float32
	BreakTorque  float32
	OnJointBreak func(child, parent *engine.GameObject, pullVec engine.Vector3)
}

func NewConstraintComponent(world *spatial.PhysicsWorld) *ConstraintComponent {
	return &ConstraintComponent{
		World: world,
		Kind:  spatial.JointSlider,
		Axis:  engine.NewVector3(0, 0, 1),
	}
}

func (c *ConstraintComponent) Init(owner *engine.GameObject) { c.Owner = owner }
func (c *ConstraintComponent) Update(_ float32)              {}
func (c *ConstraintComponent) Destroy()                      { c.Owner = nil; c.Joint = nil }

func (c *ConstraintComponent) SetPrismaticAxis(axis engine.Vector3) {
	c.Kind = spatial.JointSlider
	c.Axis = axis
}

func (c *ConstraintComponent) SetHingeAxis(axis engine.Vector3) {
	c.Kind = spatial.JointHinge
	c.Axis = axis
}

func (c *ConstraintComponent) SetLimits(minLim, maxLim float32) {
	c.MinLimit = minLim
	c.MaxLimit = maxLim
}

func (c *ConstraintComponent) SetBreakForce(force float32) {
	c.BreakForce = force
	if c.Joint != nil {
		c.Joint.BreakForce = force
	}
}

func (c *ConstraintComponent) SetBreakTorque(torque float32) {
	c.BreakTorque = torque
	if c.Joint != nil {
		c.Joint.BreakTorque = torque
	}
}

// Attach creates the underlying Jolt BreakableJoint between parentObj's RigidBody and this entity's RigidBody.
func (c *ConstraintComponent) Attach(parentObj *engine.GameObject) error {
	if c.Owner == nil || c.World == nil {
		return fmt.Errorf("components: ConstraintComponent missing Owner or PhysicsWorld")
	}
	var childRB, parentRB *spatial.RigidBody
	if rbComp := c.Owner.GetComponent(reflect.TypeOf((*RigidBodyComponent)(nil))); rbComp != nil {
		if rbc, ok := rbComp.(*RigidBodyComponent); ok && rbc != nil {
			if rbc.Body == nil {
				rbc.EnsureBody()
			}
			childRB = rbc.Body
		}
	}
	if parentObj != nil {
		if rbComp := parentObj.GetComponent(reflect.TypeOf((*RigidBodyComponent)(nil))); rbComp != nil {
			if rbc, ok := rbComp.(*RigidBodyComponent); ok && rbc != nil {
				if rbc.Body == nil {
					rbc.EnsureBody()
				}
				parentRB = rbc.Body
			}
		}
		if parentRB == nil {
			if raw := parentObj.GetComponent(reflect.TypeOf((*spatial.RigidBody)(nil))); raw != nil {
				parentRB, _ = raw.(*spatial.RigidBody)
			}
		}
	}
	if childRB == nil || parentRB == nil {
		return fmt.Errorf("components: Attach requires RigidBody on both parent and child")
	}

	anchor := c.Anchor
	if anchor.X == 0 && anchor.Y == 0 && anchor.Z == 0 {
		anchor = c.Owner.Transform.Position
	}
	joint, err := c.World.CreateBreakableJoint(parentRB, childRB, spatial.JointDesc{
		ID:          c.Owner.ID + "_joint",
		Kind:        c.Kind,
		Anchor:      anchor,
		Axis:        c.Axis,
		MinLimit:    c.MinLimit,
		MaxLimit:    c.MaxLimit,
		BreakForce:  c.BreakForce,
		BreakTorque: c.BreakTorque,
	})
	if err != nil {
		return err
	}
	joint.OnBreak = func(ev spatial.JointBreakEvent) {
		if c.OnJointBreak != nil {
			c.OnJointBreak(c.Owner, parentObj, ev.PullVector)
		}
	}
	c.Joint = joint
	return nil
}

func (c *ConstraintComponent) IsBroken() bool {
	return c.Joint != nil && c.Joint.Broken
}

// HazardConfig holds Lua/Go parameters passed to HazardComponent.Configure.
type HazardConfig struct {
	HazardType   string
	BleedChance  float32
	CrunchRadius float32
	Bounds       engine.Vector3
	Position     engine.Vector3
}

// HazardComponent attaches persistent floor hazard behavior backed by hazards.HazardSystem.
type HazardComponent struct {
	Owner        *engine.GameObject
	System       *hazards.HazardSystem
	Shard        *hazards.HazardShard
	HazardType   string
	BleedChance  float32
	CrunchRadius float32
	Bounds       engine.Vector3
}

func NewHazardComponent(sys *hazards.HazardSystem) *HazardComponent {
	return &HazardComponent{
		System:       sys,
		HazardType:   "sharp_debris",
		BleedChance:  0.85,
		CrunchRadius: hazards.DefaultShardCrunchRadius,
		Bounds:       engine.NewVector3(1.0, 0.05, 1.0),
	}
}

func (h *HazardComponent) Init(owner *engine.GameObject) { h.Owner = owner }
func (h *HazardComponent) Update(_ float32)              {}
func (h *HazardComponent) Destroy()                      { h.Owner = nil }

// Configure initializes/spawns a persistent HazardShard volume matching cfg.
func (h *HazardComponent) Configure(cfg HazardConfig) *hazards.HazardShard {
	if cfg.HazardType != "" {
		h.HazardType = cfg.HazardType
	}
	if cfg.BleedChance > 0 {
		h.BleedChance = cfg.BleedChance
	}
	if cfg.CrunchRadius > 0 {
		h.CrunchRadius = cfg.CrunchRadius
	}
	if cfg.Bounds.X > 0 && cfg.Bounds.Z > 0 {
		h.Bounds = cfg.Bounds
	}
	pos := cfg.Position
	if h.Owner != nil {
		h.Owner.Transform.Position = pos
	}

	if h.System == nil {
		h.System = hazards.NewHazardSystem(nil, nil)
	}
	radius := h.Bounds.X * 0.5
	if h.Bounds.Z*0.5 > radius {
		radius = h.Bounds.Z * 0.5
	}
	if radius <= 0 {
		radius = 0.5
	}
	shard := h.System.SpawnShard(pos, radius, hazards.DefaultLacerationDamage)
	if shard != nil && h.Owner != nil {
		shard.Owner = h.Owner
	}
	h.Shard = shard
	return shard
}
