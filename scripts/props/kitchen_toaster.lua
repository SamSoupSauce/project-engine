-- scripts/props/kitchen_toaster.lua
local Toaster = {
    meta = {
        name = "Four-Slice Chrome Toaster",
        model = "assets/models/props/kitchen_toaster.gltf",
        material = "metal_hollow",
    },
    physics = {
        mass = 2.0,
        shape = "convex_hull",
        restitution = 0.15,
        friction = 0.6,
        break_impulse = 0,
    },
    carry = {
        two_handed = false,
        hold_offset = { 0.0, -0.1, 0.25 },
        hold_rotation = { 0.0, 0.0, 0.0 },
    },
}

function Toaster.on_spawn(entity_id)
    local mesh = Engine.GetComponent(entity_id, "MeshRendererComponent")
    mesh:LoadGLTF(Toaster.meta.model)
    mesh:SetCastShadow(true)

    local rb = Engine.GetComponent(entity_id, "RigidBodyComponent")
    rb:SetMass(Toaster.physics.mass)
    rb:SetMaterial(Toaster.meta.material)
    rb:SetShape(Toaster.physics.shape)
end

function Toaster.on_collision(entity_id, hit_data)
    if hit_data.impulse > 8.0 then
        Engine.Audio.BroadcastAcousticAlert({
            origin = hit_data.point,
            radius = math.min(30.0, hit_data.impulse * 2.2),
            sound_profile = "metal_clatter_loud",
        })
    end
end

return Toaster
