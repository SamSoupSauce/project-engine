-- scripts/props/lamp_ceramic.lua
local Lamp = {
    meta = {
        name = "Ceramic Table Lamp",
        model = "assets/models/props/lamp_ceramic.gltf",
        material = "ceramic",
    },
    physics = {
        mass = 2.4,
        shape = "convex_hull",
        shatter_impulse = 12.0,
    },
}

function Lamp.on_spawn(entity_id)
    local mesh = Engine.GetComponent(entity_id, "MeshRendererComponent")
    mesh:LoadGLTF(Lamp.meta.model)
    mesh:SetCastShadow(true)

    local rb = Engine.GetComponent(entity_id, "RigidBodyComponent")
    rb:SetMass(Lamp.physics.mass)
    rb:SetMaterial(Lamp.meta.material)
    rb:SetShape(Lamp.physics.shape)
end

function Lamp.on_collision(entity_id, hit_data)
    if hit_data.impulse >= Lamp.physics.shatter_impulse then
        local shard_id = Engine.CreateEntity("FloorShardHazard")
        local shard_hz = Engine.GetComponent(shard_id, "HazardComponent")
        shard_hz:Configure({
            hazard_type = "sharp_debris",
            bleed_chance = 0.85,
            crunch_radius = 16.0,
            bounds = { 1.2, 0.05, 1.2 },
            position = hit_data.point,
        })

        Engine.DestroyEntity(entity_id)
    end
end

return Lamp
