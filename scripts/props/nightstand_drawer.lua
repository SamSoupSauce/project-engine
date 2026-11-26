-- scripts/props/nightstand_drawer.lua
local Drawer = {
    meta = {
        name = "Nightstand Drawer",
        model = "assets/models/props/nightstand_drawer.gltf",
        material = "wood_light",
    },
    physics = {
        mass = 1.1,
        shape = "box",
        tensile_break_force = 120.0,
        fracture_impulse = 14.0,
    },
    prismatic_joint = {
        axis = { 0.0, 0.0, 1.0 },
        limit_min = 0.0,
        limit_max = 0.42,
    },
}

function Drawer.on_spawn(entity_id)
    local mesh = Engine.GetComponent(entity_id, "MeshRendererComponent")
    mesh:LoadGLTF(Drawer.meta.model)

    local rb = Engine.GetComponent(entity_id, "RigidBodyComponent")
    rb:SetMass(Drawer.physics.mass)
    rb:SetMaterial(Drawer.meta.material)

    local constraint = Engine.GetComponent(entity_id, "ConstraintComponent")
    constraint:SetPrismaticAxis(Drawer.prismatic_joint.axis)
    constraint:SetLimits(Drawer.prismatic_joint.limit_min, Drawer.prismatic_joint.limit_max)
    constraint:SetBreakForce(Drawer.physics.tensile_break_force)
end

function Drawer.on_joint_break(entity_id, parent_id)
    local rb = Engine.GetComponent(entity_id, "RigidBodyComponent")
    rb:SetLinearVelocity(Engine.Player.GetHandVelocity())
end

function Drawer.on_collision(entity_id, hit_data)
    if hit_data.impulse > Drawer.physics.fracture_impulse then
        local mesh = Engine.GetComponent(entity_id, "MeshRendererComponent")
        mesh:SetVisible(false)

        Engine.Debris.SpawnSplinters({
            origin = hit_data.point,
            count = 8,
            material = "wood_splinter",
            velocity = hit_data.normal * 2.0,
        })
        Engine.DestroyEntity(entity_id)
    end
end

return Drawer
