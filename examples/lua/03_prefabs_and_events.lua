-- examples/lua/03_prefabs_and_events.lua
-- Demonstrates prefab spawning, object picking, and event emission.
-- Run: scripts.run examples/lua/03_prefabs_and_events.lua

local M = {}

function M.load(scene)
  shell.register("spawn_showcase_cube", {
    description = "Spawns a RainbowCube prefab at (x, y, z) and selects it.",
    usage = "spawn_showcase_cube [x] [y] [z]",
    examples = { "spawn_showcase_cube 0 1.5 0" },
    handler = function(ctx, args)
      local x = tonumber(args[1]) or 0.0
      local y = tonumber(args[2]) or 1.5
      local z = tonumber(args[3]) or 0.0

      local obj = spawn_prefab("rainbow_cube", {
        position = { x = x, y = y, z = z },
        pick = true,
      })
      if obj == nil then
        return false, "failed to spawn rainbow_cube prefab"
      end

      obj:RegisterHookListener("event", function(name, selfObj, sender, data)
        print(string.format("[Event] %s received %s", selfObj:GetID(), tostring(name)))
      end)
      obj:Emit("showcase_spawned", { source = ctx.sender, x = x, y = y, z = z })

      return true, string.format("Spawned %s (%s) at (%.2f, %.2f, %.2f)", obj:GetName(), obj:GetID(), x, y, z)
    end,
  })
end

return M
