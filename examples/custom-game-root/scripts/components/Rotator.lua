-- @data: {"speed":65}
-- @hooks: {"tick":["number"]}
-- @component: Rotator
Rotator = Component("Rotator", { speed = 65 }, { ["tick"] = { "number" } })
-- @end-component

function Rotator:tick(dtMs)
  local dt = (dtMs or 16) / 1000.0
  local owner = self:GetOwner()
  if owner == nil then return end
  local tr = owner:GetComponent("Transform")
  if tr == nil then return end
  local _, ry, _ = tr:GetRotation()
  tr:SetRotation(0, (ry + (self.speed or 65) * dt) % 360, 0)
end
