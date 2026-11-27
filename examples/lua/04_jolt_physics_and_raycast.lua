-- examples/lua/04_jolt_physics_and_raycast.lua
-- Demonstrates inspecting Jolt physics/interaction/fracture components from LuaBridge
-- and performing zero-allocation FFI raycasting under LuaJITBridge (ffi_jolt).

local M = {}

function M.load(scene)
  shell.register("prop_status", {
    description = "Reports RigidBody, InteractiveComponent, and FractureComponent status for an object.",
    usage = "prop_status <object_id_or_name>",
    examples = { "prop_status lamp_ceramic", "prop_status kitchen_toaster" },
    handler = function(ctx, args)
      local target = args[1]
      if not target or target == "" then
        return false, "usage: prop_status <object_id_or_name>"
      end
      local obj = pick(target)
      if obj == nil then
        return false, "object not found: " .. target
      end

      local rb = obj:GetComponent("RigidBody")
      local ic = obj:GetComponent("InteractiveComponent")
      local fc = obj:GetComponent("FractureComponent")

      local lines = {
        string.format("Object: %s (%s)", obj:GetName(), obj:GetID()),
        string.format("  RigidBody:            %s", rb ~= nil and "attached" or "none"),
        string.format("  InteractiveComponent: %s", ic ~= nil and "attached" or "none"),
        string.format("  FractureComponent:    %s", fc ~= nil and "attached" or "none"),
      }
      if fc ~= nil then
        table.insert(lines, string.format("  Fracture Health:      %.1f", fc:GetHealth()))
      end
      return true, table.concat(lines, "\n")
    end,
  })
end

-- Example LuaJIT FFI raycast helper (run inside LuaJITBridge where `ffi_jolt` is loaded):
function M.luajit_raycast_example(world_ptr)
  local ffi_jolt = require("ffi_jolt")
  local hit = ffi_jolt.cast_ray(world_ptr, 0.0, 2.0, 4.0, 0.0, -0.3, -1.0, 25.0, 0xFFFF)
  if hit ~= nil then
    return hit.body_id, hit.fraction, hit.hit_position.x, hit.hit_position.y, hit.hit_position.z
  end
  return nil
end

return M
