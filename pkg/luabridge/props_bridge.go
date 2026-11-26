package luabridge

import (
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/hazards"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
	"github.com/SamSoupSauce/project-engine/pkg/props"
)

// NewPropRegistry constructs a standard PropRegistry attached to this LuaBridge's LState.
func (b *LuaBridge) NewPropRegistry(world *spatial.PhysicsWorld, scene engine.Scene, hazardSys *hazards.HazardSystem) *props.PropRegistry {
	if b == nil {
		return nil
	}
	return props.NewPropRegistry(b.L, world, scene, hazardSys)
}
