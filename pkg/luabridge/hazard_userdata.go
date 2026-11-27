package luabridge

import (
	"reflect"

	lua "github.com/yuin/gopher-lua"
	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/SamSoupSauce/project-engine/engine/hazards"
	"github.com/SamSoupSauce/project-engine/engine/spatial"
)

const (
	luaFractureTypeName     = "FractureComponent"
	luaHazardSystemTypeName = "HazardSystem"
)

func (b *LuaBridge) registerHazardTypes() {
	b.registerFractureType()
	b.registerHazardSystemType()

	// Enum tables for FractureMode, PropIntegrity, and Footwear.
	modes := b.L.NewTable()
	b.L.SetField(modes, "Splinter", lua.LNumber(spatial.FractureSplinter))
	b.L.SetField(modes, "Shatter", lua.LNumber(spatial.FractureShatter))
	b.L.SetField(modes, "Deform", lua.LNumber(spatial.FractureDeform))
	b.L.SetGlobal("FractureMode", modes)

	integrity := b.L.NewTable()
	b.L.SetField(integrity, "Low", lua.LNumber(spatial.IntegrityLow))
	b.L.SetField(integrity, "Standard", lua.LNumber(spatial.IntegrityStandard))
	b.L.SetField(integrity, "Heavy", lua.LNumber(spatial.IntegrityHeavy))
	b.L.SetGlobal("PropIntegrity", integrity)

	footwear := b.L.NewTable()
	b.L.SetField(footwear, "Barefoot", lua.LNumber(hazards.FootwearBarefoot))
	b.L.SetField(footwear, "Torn", lua.LNumber(hazards.FootwearTorn))
	b.L.SetField(footwear, "Boots", lua.LNumber(hazards.FootwearBoots))
	b.L.SetGlobal("Footwear", footwear)

	b.L.SetGlobal("new_hazard_system", b.L.NewFunction(func(L *lua.LState) int {
		hs := hazards.NewHazardSystem(nil, nil)
		pushHazardSystem(L, hs)
		return 1
	}))

	// Expose global on_sweep(hazardSystem, ox, oy, oz, dx, dy, dz, [reach, impulse])
	b.L.SetGlobal("on_sweep", b.L.NewFunction(func(L *lua.LState) int {
		hs := checkHazardSystem(L, 1)
		origin := engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4)))
		dir := engine.NewVector3(float32(L.CheckNumber(5)), float32(L.CheckNumber(6)), float32(L.CheckNumber(7)))
		reach := float32(L.OptNumber(8, 2.0))
		impulse := float32(L.OptNumber(9, 6.0))
		L.Push(lua.LNumber(hs.OnSweep(origin, dir, reach, impulse)))
		return 1
	}))

	// Expose global kick_hazards(hazardSystem, ox, oy, oz, dx, dy, dz, [reach, impulse])
	b.L.SetGlobal("kick_hazards", b.L.NewFunction(func(L *lua.LState) int {
		hs := checkHazardSystem(L, 1)
		origin := engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4)))
		dir := engine.NewVector3(float32(L.CheckNumber(5)), float32(L.CheckNumber(6)), float32(L.CheckNumber(7)))
		reach := float32(L.OptNumber(8, 1.6))
		impulse := float32(L.OptNumber(9, 8.0))
		L.Push(lua.LNumber(hs.Kick(origin, dir, reach, impulse)))
		return 1
	}))
}

func (b *LuaBridge) registerFractureType() {
	mt := b.L.NewTypeMetatable(luaFractureTypeName)
	methods := b.L.NewTable()
	for name, fn := range fractureMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}
	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		_ = checkFracture(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))
}

func fractureMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"GetHealth": func(L *lua.LState) int {
			fc := checkFracture(L, 1)
			L.Push(lua.LNumber(fc.Health))
			return 1
		},
		"SetHealth": func(L *lua.LState) int {
			fc := checkFracture(L, 1)
			fc.Health = float32(L.CheckNumber(2))
			return 0
		},
		"IsBroken": func(L *lua.LState) int {
			fc := checkFracture(L, 1)
			L.Push(lua.LBool(fc.Broken))
			return 1
		},
		"GetRequiredCarryHands": func(L *lua.LState) int {
			fc := checkFracture(L, 1)
			L.Push(lua.LNumber(fc.RequiredCarryHands))
			return 1
		},
		"GetCameraLookDamping": func(L *lua.LState) int {
			fc := checkFracture(L, 1)
			L.Push(lua.LNumber(fc.CameraLookDamping))
			return 1
		},
		"ApplyImpact": func(L *lua.LState) int {
			fc := checkFracture(L, 1)
			imp := float32(L.CheckNumber(2))
			_, broke := fc.ApplyImpact(imp, engine.Vector3{}, engine.NewVector3(0, 1, 0))
			L.Push(lua.LBool(broke))
			return 1
		},
		"Fracture": func(L *lua.LState) int {
			fc := checkFracture(L, 1)
			ev := fc.Fracture(engine.Vector3{}, engine.NewVector3(0, 1, 0), fc.Health)
			L.Push(lua.LNumber(len(ev.Debris)))
			return 1
		},
	}
}

func (b *LuaBridge) registerHazardSystemType() {
	mt := b.L.NewTypeMetatable(luaHazardSystemTypeName)
	methods := b.L.NewTable()
	for name, fn := range hazardSystemMethods() {
		b.L.SetField(methods, name, b.L.NewFunction(fn))
	}
	b.L.SetField(mt, "__index", b.L.NewFunction(func(L *lua.LState) int {
		_ = checkHazardSystem(L, 1)
		key := L.CheckString(2)
		if fn := L.GetField(methods, key); fn != lua.LNil {
			L.Push(fn)
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))
}

func hazardSystemMethods() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"SpawnShard": func(L *lua.LState) int {
			hs := checkHazardSystem(L, 1)
			pos := engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4)))
			radius := float32(L.OptNumber(5, 0.45))
			sharpness := float32(L.OptNumber(6, lua.LNumber(hazards.DefaultLacerationDamage)))
			s := hs.SpawnShard(pos, radius, sharpness)
			if s == nil {
				L.Push(lua.LString(""))
				return 1
			}
			L.Push(lua.LString(s.ID))
			return 1
		},
		"ActiveShardCount": func(L *lua.LState) int {
			hs := checkHazardSystem(L, 1)
			L.Push(lua.LNumber(hs.ActiveShardCount()))
			return 1
		},
		"RegisterZombie": func(L *lua.LState) int {
			hs := checkHazardSystem(L, 1)
			id := L.CheckString(2)
			pos := engine.NewVector3(float32(L.CheckNumber(3)), float32(L.CheckNumber(4)), float32(L.CheckNumber(5)))
			hs.RegisterZombie(&hazards.ZombieListener{ID: id, Position: pos})
			return 0
		},
		"IsZombieAlerted": func(L *lua.LState) int {
			hs := checkHazardSystem(L, 1)
			id := L.CheckString(2)
			for _, z := range hs.Zombies {
				if z != nil && z.ID == id {
					L.Push(lua.LBool(z.Alerted))
					return 1
				}
			}
			L.Push(lua.LBool(false))
			return 1
		},
		"EvaluateTraversal": func(L *lua.LState) int {
			hs := checkHazardSystem(L, 1)
			actor := &hazards.TraversalActor{
				EntityID:       L.CheckString(2),
				Position:       engine.NewVector3(float32(L.CheckNumber(3)), float32(L.CheckNumber(4)), float32(L.CheckNumber(5))),
				Footwear:       hazards.FootwearKind(L.CheckInt(6)),
				Health:         float32(L.OptNumber(7, 100.0)),
				LimpSpeedScale: 1.0,
			}
			res := hs.EvaluateTraversal(actor)
			tbl := L.NewTable()
			L.SetField(tbl, "damaged", lua.LBool(res.Damaged))
			L.SetField(tbl, "health", lua.LNumber(actor.Health))
			L.SetField(tbl, "bleeding", lua.LBool(actor.Bleeding))
			L.SetField(tbl, "limp_scale", lua.LNumber(actor.LimpSpeedScale))
			L.SetField(tbl, "screamed", lua.LBool(res.Screamed))
			L.SetField(tbl, "crunch_alerted", lua.LBool(res.CrunchAlerted))
			L.SetField(tbl, "alerted_zombies", lua.LNumber(res.AlertedZombies))
			L.Push(tbl)
			return 1
		},
		"OnSweep": func(L *lua.LState) int {
			hs := checkHazardSystem(L, 1)
			origin := engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4)))
			dir := engine.NewVector3(float32(L.CheckNumber(5)), float32(L.CheckNumber(6)), float32(L.CheckNumber(7)))
			reach := float32(L.OptNumber(8, 2.0))
			impulse := float32(L.OptNumber(9, 6.0))
			L.Push(lua.LNumber(hs.OnSweep(origin, dir, reach, impulse)))
			return 1
		},
		"Kick": func(L *lua.LState) int {
			hs := checkHazardSystem(L, 1)
			origin := engine.NewVector3(float32(L.CheckNumber(2)), float32(L.CheckNumber(3)), float32(L.CheckNumber(4)))
			dir := engine.NewVector3(float32(L.CheckNumber(5)), float32(L.CheckNumber(6)), float32(L.CheckNumber(7)))
			reach := float32(L.OptNumber(8, 1.6))
			impulse := float32(L.OptNumber(9, 8.0))
			L.Push(lua.LNumber(hs.Kick(origin, dir, reach, impulse)))
			return 1
		},
	}
}

func findFractureComponent(obj *engine.GameObject) *spatial.FractureComponent {
	if obj == nil {
		return nil
	}
	comp := obj.GetComponent(reflect.TypeOf((*spatial.FractureComponent)(nil)))
	if fc, ok := comp.(*spatial.FractureComponent); ok && fc != nil {
		return fc
	}
	return nil
}

func pushFracture(L *lua.LState, fc *spatial.FractureComponent) {
	if fc == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = fc
	L.SetMetatable(ud, L.GetTypeMetatable(luaFractureTypeName))
	L.Push(ud)
}

func checkFracture(L *lua.LState, idx int) *spatial.FractureComponent {
	ud := L.CheckUserData(idx)
	if fc, ok := ud.Value.(*spatial.FractureComponent); ok && fc != nil {
		return fc
	}
	L.ArgError(idx, "FractureComponent expected")
	return nil
}

func pushHazardSystem(L *lua.LState, hs *hazards.HazardSystem) {
	if hs == nil {
		L.Push(lua.LNil)
		return
	}
	ud := L.NewUserData()
	ud.Value = hs
	L.SetMetatable(ud, L.GetTypeMetatable(luaHazardSystemTypeName))
	L.Push(ud)
}

func checkHazardSystem(L *lua.LState, idx int) *hazards.HazardSystem {
	ud := L.CheckUserData(idx)
	if hs, ok := ud.Value.(*hazards.HazardSystem); ok && hs != nil {
		return hs
	}
	L.ArgError(idx, "HazardSystem expected")
	return nil
}
