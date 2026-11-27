# Lua & LuaJIT Scripting Guide

Project Engine provides two complementary Lua runtimes:
1. **`LuaBridge` (Lua 5.1 via `gopher-lua`)**: Gameplay logic, custom ECS components, shell commands, prefabs, and live console REPL (`scripts/components/`, `scripts/props/`, `examples/lua/`).
2. **`LuaJITBridge` (LuaJIT 2.1 C-FFI)**: High-frequency 2D HUD rendering and zero-allocation raycasting (`scripts/ui/`, `examples/lua/05_zerocopy_hud_luajit.lua`). See [Scripting Standards](scripting_standards.md) for zero-GC rules.

---

## 1. Custom ECS Components (`Component(...)`)

Place auto-loaded custom components in `scripts/components/*.lua` (loaded at startup via `LuaBridge.LoadScriptsDir`):

```lua
-- @data: {"speed":48}
-- @hooks: {"tick":["number"]}
-- @component: Rotator
Rotator = Component("Rotator", { speed = 48 }, { ["tick"] = { "number" } })
-- @end-component

function Rotator:tick(dtMs)
  local owner = self:GetOwner()
  if owner == nil then return end
  local tr = owner:GetComponent("Transform")
  if tr == nil then return end
  local _, ry, _ = tr:GetRotation()
  tr:SetRotation(0, (ry + self.speed * (dtMs / 1000.0)) % 360, 0)
end
```
Open any component in the tri-pane Script Editor from the console with `edit("Rotator")`.

---

## 2. Dynamic Shell Commands (`shell.register`)

Register console/agent commands dynamically from any Lua script:

```lua
local M = {}

function M.load(scene)
  shell.register("hello", {
    description = "Prints a greeting.",
    usage = "hello [name]",
    examples = { "hello", "hello World" },
    handler = function(ctx, args)
      return true, "Hello, " .. (args[1] or "World") .. " (from " .. ctx.sender .. ")"
    end,
  })
end

return M
```
Commands registered via `shell.register` automatically appear in `help`, `docs`, and `engine.query_docs`, and are cleanly unbound on `scripts.unload` or `scripts.reload`.

---

## 3. Lifecycle Hooks

- `load(scene)` — Called on scene init or `scripts.run` / `scripts.reload`
- `unload(scene)` — Called on scene unload or `scripts.unload`
- `spawn(self, sender)` — Called when the owning `GameObject` spawns
- `tick(self, dtMs)` — Fixed-timestep simulation step (default `16` ms)
- `event(name, self, sender, data)` — Triggered by `obj:Emit(name, data)` or `obj:EmitTo(target, name, data)`
- `despawn(self, sender, data)` — Called immediately before `Despawn`

---

## 4. Workspace & Prefab Helpers

- `workspace_paths()` — Returns `{ root, assets, scripts, prefabs, config }` for the active `--root` / `engine.json`
- `pick(nameOrID)` / `picked()` / `unpick()` / `list_objects()` — Inspect & select scene entities (`selected` global)
- `spawn_prefab(name, opts)` / `save_prefab(obj, name)` / `search_prefabs(query)` — Manage JSON prefabs under `prefabs/`
- `docs([topic])` / `help([target])` — Query live runtime documentation

---

## 5. Graceful Script Error Handling

Lua syntax errors, missing globals, and runtime hook errors never crash the engine or abort loading remaining scripts:
- `LoadScriptsDir` logs failing scripts via `LuaBridge.RecordScriptError` and continues loading all valid scripts.
- Runtime hook failures in `ScriptComponent`, `LuaComponent`, and `shell.register` handlers log `[ScriptError] <script> (<hook>): <message>` to the Developer Console and surface in the Demo Controls error panel until cleared.

---

## 6. Runnable Examples

See [`examples/lua/README.md`](../examples/lua/README.md) for five complete, runnable examples covering shell commands, custom components, prefabs/events, Jolt physics/raycasts, and LuaJIT FFI HUD rendering.
