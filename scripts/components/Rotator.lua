-- @data: {"speed":48,"bob_amp":0.06,"phase":0}
-- @hooks: {"tick":["number"]}
-- @component: Rotator
Rotator = Component("Rotator", { speed = 48, bob_amp = 0.06, phase = 0 }, { ["tick"] = { "number" } })
-- @end-component

function Rotator:tick(dtMs)
  local dt = (dtMs or 16) / 1000.0
  self.phase = (self.phase or 0) + dt * 2.0
  local owner = self:GetOwner()
  if owner == nil then
    return
  end
  local tr = owner:GetComponent("Transform")
  if tr == nil then
    return
  end
  local rx, ry, rz = tr:GetRotation()
  local nextY = (ry + (self.speed or 48) * dt) % 360
  tr:SetRotation(nextY * 0.55, nextY, rz)
end
