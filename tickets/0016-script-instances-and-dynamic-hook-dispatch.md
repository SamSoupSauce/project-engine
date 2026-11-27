# Ticket 0016: Script Instances and Dynamic Hook Dispatch

## Status
Open

## Description
Implement a lightweight Script Container Component (`ScriptContainerComponent`) and a dynamic Entity Hook Dispatcher in `project-engine` (`engine/components/script.go`, `pkg/luabridge/hooks.go`). Any entity can host arbitrary independent Lua scripts. Scripts are plain tables with their own state. Hooks allow both internal engine events and external game code to broadcast named events across an entity's scripts without strict interface definitions.

## Architectural Design

### 1. Script Instance Lifecycle
When a script is attached to an entity:
1. `entity:AddScript(path_or_factory, [config_table])` loads the chunk.
2. A unique instance table is created and stored in the entity's `ScriptContainerComponent`.
3. If the script defines an `init(self, entity, config)` method, it is executed immediately.
4. The instance is registered with the entity's hook table for fast dispatch.

### 2. Dynamic Hook Dispatcher (`entity:CallHook(hook_name, ...)`)
- **Fast Table Lookup:** When `CallHook` is triggered, the engine loops over attached script instances.
- **Duck-Typing Execution:** Checks if `rawget(script_instance, hook_name)` is a function.
  - If `true`, invokes: `script_instance[hook_name](script_instance, entity, ...)`.
  - If `false`, silently continues with zero penalty.
- **Return Value Handling:** If a hook returns a value (e.g., returning `true` to cancel an action like `OnDoorOpen`), the dispatcher can short-circuit and propagate the cancelation back to the caller.

### 3. Example: Composable Script Behaviors

#### Script A: `scripts/props/destructible.lua`
```lua
local Destructible = {}

function Destructible:init(entity, config)
    self.hp = config.hp or 50
    self.fracture_threshold = config.fracture_threshold or 15.0
end

-- Hook: Responds to physics impacts
function Destructible:OnPhysicsImpact(entity, impulse, normal, hit_point)
    if impulse >= self.fracture_threshold then
        self.hp = self.hp - (impulse * 2)
        print(string.format("Prop took %f damage! Remaining HP: %f", impulse * 2, self.hp))
        
        if self.hp <= 0 then
            entity:CallHook("OnBreak", hit_point)
        end
    end
end

-- Hook: Responds to broken state
function Destructible:OnBreak(entity, hit_point)
    print("Shattering into pieces!")
    Engine.Destroy(entity)
end

return Destructible
```

---
<!-- Questions and clarifications log -->
