package hazards

import (
	"fmt"
	"math"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

const (
	// HazardShardTag identifies persistent floor collision shards spawned by fractured glass/ceramic.
	HazardShardTag = "HazardShard"
	// DefaultShardCrunchRadius is the acoustic propagation radius in meters when walking over shards in boots.
	DefaultShardCrunchRadius float32 = 15.0
	// DefaultLacerationDamage is health deducted per bare/torn-footwear shard traversal.
	DefaultLacerationDamage float32 = 15.0
	// DefaultLimpSpeedScale slows injured barefoot movement to 60% of normal speed.
	DefaultLimpSpeedScale float32 = 0.60
)

// FootwearKind classifies equipped footwear for surface shard traversal checks.
type FootwearKind uint8

const (
	FootwearBarefoot FootwearKind = iota
	FootwearTorn
	FootwearBoots
)

// TraversalActor represents a player or NPC entity stepping onto surface hazards.
type TraversalActor struct {
	EntityID       string
	Object         *engine.GameObject
	Position       engine.Vector3
	Radius         float32
	Footwear       FootwearKind
	Health         float32
	Bleeding       bool
	LimpSpeedScale float32
}

// VocalAudioEvent is emitted when an unbooted actor suffers shard laceration.
type VocalAudioEvent struct {
	EntityID string
	SoundID  string
	Position engine.Vector3
	Volume   float32
}

// AcousticAlertEvent is emitted when booted steps crunch shards and alert zombies within SoundRadius.
type AcousticAlertEvent struct {
	SourceID    string
	SoundID     string
	Position    engine.Vector3
	SoundRadius float32
}

// ZombieListener tracks an AI listener that reacts to nearby acoustic alerts.
type ZombieListener struct {
	ID            string
	Position      engine.Vector3
	Alerted       bool
	AlertTarget   engine.Vector3
	LastSoundID   string
	AlertDistance float32
	OnAlert       func(ev AcousticAlertEvent)
}

// TraversalResult reports all effects triggered by stepping through floor shards.
type TraversalResult struct {
	ContactedShards int
	Damaged         bool
	DamageDealt     float32
	BleedingApplied bool
	Limping         bool
	Screamed        bool
	CrunchAlerted   bool
	AlertedZombies  int
	VocalEvents     []VocalAudioEvent
	AcousticEvents  []AcousticAlertEvent
}

// HazardShard represents a persistent flat floor collision volume tagged HazardShard.
type HazardShard struct {
	ID          string
	Tag         string
	Owner       *engine.GameObject
	Body        *spatial.RigidBody
	Position    engine.Vector3
	HalfExtents engine.Vector3
	Radius      float32
	Sharpness   float32
	Active      bool
}

// HazardSystem manages spawned HazardShard zones, debris bodies, footwear traversal checks,
// acoustic zombie alerts, and sweep/kick clearing actions.
type HazardSystem struct {
	World        *spatial.PhysicsWorld
	Scene        engine.Scene
	Shards       []*HazardShard
	DebrisBodies []*spatial.RigidBody
	Zombies      []*ZombieListener

	OnVocalAudio  func(ev VocalAudioEvent)
	OnAcoustic    func(ev AcousticAlertEvent)
	OnSweepEffect func(shard *HazardShard, dir engine.Vector3)

	shardSeq int
}

// NewHazardSystem constructs a HazardSystem wired to an optional PhysicsWorld and Scene.
func NewHazardSystem(world *spatial.PhysicsWorld, scene engine.Scene) *HazardSystem {
	return &HazardSystem{
		World: world,
		Scene: scene,
	}
}

// WireFracture attaches an OnFracture listener so shattering props automatically spawn floor HazardShards.
func (hs *HazardSystem) WireFracture(fc *spatial.FractureComponent) {
	if hs == nil || fc == nil {
		return
	}
	prev := fc.OnFracture
	fc.OnFracture = func(ev spatial.FractureEvent) {
		if prev != nil {
			prev(ev)
		}
		if len(ev.DebrisBodies) > 0 {
			hs.DebrisBodies = append(hs.DebrisBodies, ev.DebrisBodies...)
		}
		if ev.Mode == spatial.FractureShatter || ev.SpawnHazard {
			hs.SpawnShardsFromFracture(ev)
		}
	}
}

// RegisterZombie adds an AI listener eligible to hear booted crunch alerts within SoundRadius.
func (hs *HazardSystem) RegisterZombie(z *ZombieListener) {
	if hs == nil || z == nil {
		return
	}
	hs.Zombies = append(hs.Zombies, z)
}

// SpawnShard creates a persistent flat HazardShard collision volume at floorPos.
func (hs *HazardSystem) SpawnShard(floorPos engine.Vector3, radius, sharpness float32) *HazardShard {
	if hs == nil {
		return nil
	}
	if radius <= 0 {
		radius = 0.45
	}
	if sharpness <= 0 {
		sharpness = DefaultLacerationDamage
	}

	hs.shardSeq++
	id := fmt.Sprintf("hazard_shard_%d", hs.shardSeq)
	obj := engine.NewGameObject(id, HazardShardTag)
	obj.Transform.Position = floorPos
	obj.AddComponent(engine.NewRenderable("shard_cluster_mesh", "ceramic_shard"))

	half := engine.NewVector3(radius*0.8, 0.025, radius*0.8)
	var rb *spatial.RigidBody
	if hs.World != nil {
		rb = spatial.NewRigidBody(id+"_rb", hs.World, engine.BodyDynamic, spatial.BoxShapeDesc(half.X, half.Y, half.Z))
		rb.Mass = 0.08
		rb.Friction = 0.9
		rb.ObjectLayer = spatial.LayerInteractable
		rb.HasCustomLayer = true
		obj.AddComponent(rb)
		hs.World.RegisterSyncTarget(rb.BodyID, obj)
	}
	if hs.Scene != nil {
		if adder, ok := hs.Scene.(interface{ AddObject(*engine.GameObject) }); ok {
			adder.AddObject(obj)
		}
	}

	shard := &HazardShard{
		ID:          id,
		Tag:         HazardShardTag,
		Owner:       obj,
		Body:        rb,
		Position:    floorPos,
		HalfExtents: half,
		Radius:      radius,
		Sharpness:   sharpness,
		Active:      true,
	}
	hs.Shards = append(hs.Shards, shard)
	return shard
}

// SpawnShardsFromFracture spawns a cluster of persistent flat HazardShard volumes from a shatter event.
func (hs *HazardSystem) SpawnShardsFromFracture(ev spatial.FractureEvent) []*HazardShard {
	if hs == nil {
		return nil
	}
	count := 3
	if len(ev.Debris) > 0 && len(ev.Debris) < count {
		count = len(ev.Debris)
	}
	spawned := make([]*HazardShard, 0, count)
	for i := 0; i < count; i++ {
		ang := float64(i) * (2.0 * math.Pi / float64(count))
		pos := engine.NewVector3(
			ev.ImpactPoint.X+float32(math.Cos(ang))*0.35,
			ev.ImpactPoint.Y,
			ev.ImpactPoint.Z+float32(math.Sin(ang))*0.35,
		)
		if pos.Y > 0.05 && pos.Y < 0.2 {
			pos.Y = 0.025
		}
		spawned = append(spawned, hs.SpawnShard(pos, 0.45, DefaultLacerationDamage))
	}
	return spawned
}

// EvaluateTraversal inspects whether actor contacts any active HazardShard and resolves
// footwear-based damage/limp/screams vs. 15.0m booted crunch acoustic alerts.
func (hs *HazardSystem) EvaluateTraversal(actor *TraversalActor) TraversalResult {
	if hs == nil || actor == nil {
		return TraversalResult{}
	}
	if actor.LimpSpeedScale <= 0 {
		actor.LimpSpeedScale = 1.0
	}
	actorRadius := actor.Radius
	if actorRadius <= 0 {
		actorRadius = 0.35
	}

	var res TraversalResult
	for _, shard := range hs.Shards {
		if shard == nil || !shard.Active {
			continue
		}
		pos := shard.currentPosition()
		dx := actor.Position.X - pos.X
		dz := actor.Position.Z - pos.Z
		dy := float32(math.Abs(float64(actor.Position.Y - pos.Y)))
		reach := actorRadius + shard.Radius
		if dx*dx+dz*dz > reach*reach || dy > 1.0 {
			continue
		}

		res.ContactedShards++
		if actor.Footwear == FootwearBoots {
			crunch := AcousticAlertEvent{
				SourceID:    actor.EntityID,
				SoundID:     "shard_crunch",
				Position:    pos,
				SoundRadius: DefaultShardCrunchRadius,
			}
			res.CrunchAlerted = true
			res.AcousticEvents = append(res.AcousticEvents, crunch)
			if hs.OnAcoustic != nil {
				hs.OnAcoustic(crunch)
			}
			res.AlertedZombies += hs.broadcastAcousticAlert(crunch)
			continue
		}

		// Barefoot or torn footwear suffers laceration, bleeding, limp speed penalty, and vocal scream.
		dmg := shard.Sharpness
		if dmg <= 0 {
			dmg = DefaultLacerationDamage
		}
		actor.Health -= dmg
		if actor.Health < 0 {
			actor.Health = 0
		}
		actor.Bleeding = true
		if actor.LimpSpeedScale > DefaultLimpSpeedScale {
			actor.LimpSpeedScale = DefaultLimpSpeedScale
		}

		vocal := VocalAudioEvent{
			EntityID: actor.EntityID,
			SoundID:  "pain_scream_shard",
			Position: actor.Position,
			Volume:   1.0,
		}
		res.Damaged = true
		res.DamageDealt += dmg
		res.BleedingApplied = true
		res.Limping = true
		res.Screamed = true
		res.VocalEvents = append(res.VocalEvents, vocal)
		if hs.OnVocalAudio != nil {
			hs.OnVocalAudio(vocal)
		}
	}
	return res
}

// OnSweep clears or displaces shards and debris in front of origin along dir (push broom action).
// Returns the number of shards/debris displaced or cleared.
func (hs *HazardSystem) OnSweep(origin, dir engine.Vector3, reach, pushImpulse float32) int {
	if hs == nil {
		return 0
	}
	if reach <= 0 {
		reach = 2.0
	}
	if pushImpulse <= 0 {
		pushImpulse = 6.0
	}
	nDir := normHazardVec3(dir)
	if nDir.X == 0 && nDir.Y == 0 && nDir.Z == 0 {
		nDir = engine.NewVector3(0, 0, -1)
	}

	affected := 0
	for _, shard := range hs.Shards {
		if shard == nil || !shard.Active {
			continue
		}
		pos := shard.currentPosition()
		if !inConeOrReach(origin, nDir, pos, reach, 0.25) {
			continue
		}
		// Push shard out of chokepoint or deactivate/clear it after displacement.
		shard.Position = engine.NewVector3(
			pos.X+nDir.X*reach*1.25,
			pos.Y,
			pos.Z+nDir.Z*reach*1.25,
		)
		if shard.Owner != nil {
			shard.Owner.Transform.Position = shard.Position
		}
		if shard.Body != nil {
			shard.Body.AddImpulse(engine.NewVector3(nDir.X*pushImpulse, 0.4, nDir.Z*pushImpulse))
		}
		shard.Active = false
		if shard.Owner != nil {
			shard.Owner.Active = false
		}
		if hs.OnSweepEffect != nil {
			hs.OnSweepEffect(shard, nDir)
		}
		affected++
	}

	for _, deb := range hs.DebrisBodies {
		if deb == nil || deb.BodyID == spatial.InvalidBodyID {
			continue
		}
		pos := deb.GetPosition()
		if inConeOrReach(origin, nDir, pos, reach, 0.25) {
			deb.AddImpulse(engine.NewVector3(nDir.X*pushImpulse, 0.6, nDir.Z*pushImpulse))
			affected++
		}
	}
	return affected
}

// Kick applies a physical scatter impulse to shards and debris directly ahead of origin along dir.
func (hs *HazardSystem) Kick(origin, dir engine.Vector3, reach, impulse float32) int {
	if hs == nil {
		return 0
	}
	if reach <= 0 {
		reach = 1.6
	}
	if impulse <= 0 {
		impulse = 8.0
	}
	nDir := normHazardVec3(dir)
	if nDir.X == 0 && nDir.Y == 0 && nDir.Z == 0 {
		nDir = engine.NewVector3(0, 0, -1)
	}

	kicked := 0
	for _, shard := range hs.Shards {
		if shard == nil || !shard.Active {
			continue
		}
		pos := shard.currentPosition()
		if !inConeOrReach(origin, nDir, pos, reach, 0.4) {
			continue
		}
		shard.Position = engine.NewVector3(
			pos.X+nDir.X*1.5,
			pos.Y,
			pos.Z+nDir.Z*1.5,
		)
		if shard.Owner != nil {
			shard.Owner.Transform.Position = shard.Position
		}
		if shard.Body != nil {
			shard.Body.AddImpulse(engine.NewVector3(nDir.X*impulse, 1.2, nDir.Z*impulse))
		}
		kicked++
	}

	for _, deb := range hs.DebrisBodies {
		if deb == nil || deb.BodyID == spatial.InvalidBodyID {
			continue
		}
		pos := deb.GetPosition()
		if inConeOrReach(origin, nDir, pos, reach, 0.4) {
			deb.AddImpulse(engine.NewVector3(nDir.X*impulse, 1.4, nDir.Z*impulse))
			kicked++
		}
	}
	return kicked
}

// ActiveShardCount returns the number of currently active floor HazardShard volumes.
func (hs *HazardSystem) ActiveShardCount() int {
	if hs == nil {
		return 0
	}
	count := 0
	for _, s := range hs.Shards {
		if s != nil && s.Active {
			count++
		}
	}
	return count
}

func (hs *HazardSystem) broadcastAcousticAlert(ev AcousticAlertEvent) int {
	rSq := ev.SoundRadius * ev.SoundRadius
	alerted := 0
	for _, z := range hs.Zombies {
		if z == nil {
			continue
		}
		dx := z.Position.X - ev.Position.X
		dy := z.Position.Y - ev.Position.Y
		dz := z.Position.Z - ev.Position.Z
		distSq := dx*dx + dy*dy + dz*dz
		if distSq <= rSq {
			z.Alerted = true
			z.AlertTarget = ev.Position
			z.LastSoundID = ev.SoundID
			z.AlertDistance = float32(math.Sqrt(float64(distSq)))
			if z.OnAlert != nil {
				z.OnAlert(ev)
			}
			alerted++
		}
	}
	return alerted
}

func (s *HazardShard) currentPosition() engine.Vector3 {
	if s.Body != nil && s.Body.BodyID != spatial.InvalidBodyID {
		return s.Body.GetPosition()
	}
	if s.Owner != nil {
		return s.Owner.Transform.Position
	}
	return s.Position
}

func inConeOrReach(origin, dir, target engine.Vector3, reach, minDot float32) bool {
	vx := target.X - origin.X
	vy := target.Y - origin.Y
	vz := target.Z - origin.Z
	distSq := vx*vx + vy*vy + vz*vz
	if distSq > reach*reach {
		return false
	}
	if distSq < 0.04 {
		return true
	}
	dist := float32(math.Sqrt(float64(distSq)))
	dot := (vx*dir.X + vy*dir.Y + vz*dir.Z) / dist
	return dot >= minDot
}

func normHazardVec3(v engine.Vector3) engine.Vector3 {
	m := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if m < 1e-6 {
		return engine.Vector3{}
	}
	return engine.NewVector3(v.X/m, v.Y/m, v.Z/m)
}
