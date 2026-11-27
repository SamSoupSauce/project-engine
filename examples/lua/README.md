# Project Engine Lua Examples

These opt-in Lua examples demonstrate Project Engine's scripting subsystems. They live in `examples/lua/` rather than `scripts/` so they do not auto-register commands until you explicitly run them.

## Running Any Example

Inside the in-game Developer Console (`[~]`) or via `engine.console_exec` / WebSocket:

```text
scripts.run examples/lua/01_hello_shell_commands.lua
scripts.run examples/lua/02_custom_component.lua
scripts.run examples/lua/03_prefabs_and_events.lua
scripts.run examples/lua/04_jolt_physics_and_raycast.lua
```

To reload or unload after editing:

```text
scripts.reload examples/lua/01_hello_shell_commands.lua
scripts.unload examples/lua/01_hello_shell_commands.lua
```

## Example Index

1. [`01_hello_shell_commands.lua`](01_hello_shell_commands.lua) — Registering dynamic `shell.register` commands with `description`, `usage`, `examples`, sender attribution (`ctx.sender`), and automatic `unload` cleanup.
2. [`02_custom_component.lua`](02_custom_component.lua) — Defining a custom ECS `Component(...)` with `-- @data:` / `-- @hooks:` metadata for the Script Editor and `tick(self, dtMs)` / `spawn(self, sender)` hooks.
3. [`03_prefabs_and_events.lua`](03_prefabs_and_events.lua) — Spawning prefabs (`spawn_prefab`), picking (`pick`), inspecting `workspace_paths()`, and dispatching entity events (`Emit`, `RegisterHookListener`).
4. [`04_jolt_physics_and_raycast.lua`](04_jolt_physics_and_raycast.lua) — Inspecting `RigidBody`, `InteractiveComponent`, and `FractureComponent` on scene objects plus LuaJIT FFI raycasting (`ffi_jolt`).
5. [`05_zerocopy_hud_luajit.lua`](05_zerocopy_hud_luajit.lua) — Zero-allocation LuaJIT 2.1 FFI 2D HUD rendering using pre-allocated `ffi.new` pools and `igImDrawList_*` (executed under `LuaJITBridge`).
