-- examples/lua/01_hello_shell_commands.lua
-- Run in Developer Console: scripts.run examples/lua/01_hello_shell_commands.lua
-- Then query docs:        docs greet
-- Execute command:        greet "World"

local M = {}

function M.load(scene)
  shell.register("greet", {
    description = "Greets a target and reports the caller sender & active scene.",
    usage = "greet [name]",
    examples = { "greet", "greet Alice" },
    handler = function(ctx, args)
      local who = args[1] or "Developer"
      local sceneName = scene and scene:GetName() or "headless"
      return true, string.format("Hello, %s! (sender=%s, scene=%s)", who, ctx.sender, sceneName)
    end,
  })

  shell.register("scene_summary", {
    description = "Lists workspace root paths and registered scene GameObjects.",
    usage = "scene_summary",
    examples = { "scene_summary" },
    handler = function(ctx, args)
      local wp = workspace_paths()
      local objs = list_objects()
      return true, string.format("Root=%s | Objects=%d", wp.root, #objs)
    end,
  })
end

function M.unload(scene)
  -- Dynamic commands registered via shell.register are automatically unbound on scripts.unload,
  -- and you can also perform custom teardown here.
  print("[01_hello_shell_commands] unloaded cleanly")
end

return M
