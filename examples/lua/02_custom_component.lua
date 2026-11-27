-- examples/lua/02_custom_component.lua
-- Demonstrates defining a custom ECS Component with Script Editor metadata and typed hooks.
-- Run: scripts.run examples/lua/02_custom_component.lua

-- @data: {"orbit_radius":1.2,"orbit_speed":1.5,"angle":0}
-- @hooks: {"spawn":["GameObject"],"tick":["number"]}
-- @component: Orbiter
Orbiter = Component("Orbiter", { orbit_radius = 1.2, orbit_speed = 1.5, angle = 0 }, {
  ["spawn"] = { "GameObject" },
  ["tick"]  = { "number" },
})
-- @end-component

function Orbiter:spawn(sender)
  local owner = self:GetOwner()
  if owner ~= nil then
    print(string.format("[Orbiter] Attached to %s (%s)", owner:GetName(), owner:GetID()))
  end
end

function Orbiter:tick(dtMs)
  local owner = self:GetOwner()
  if owner == nil then return end
  local tr = owner:GetComponent("Transform")
  if tr == nil then return end

  local dt = (dtMs or 16) / 1000.0
  self.angle = (self.angle or 0) + (self.orbit_speed or 1.5) * dt
  local x = math.cos(self.angle) * (self.orbit_radius or 1.2)
  local z = math.sin(self.angle) * (self.orbit_radius or 1.2)
  tr:SetPosition(x, 0.85, z)
end
