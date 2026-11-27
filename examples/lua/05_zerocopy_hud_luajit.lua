-- examples/lua/05_zerocopy_hud_luajit.lua
-- Zero-allocation LuaJIT 2.1 FFI 2D HUD example adhering to docs/scripting_standards.md.
-- Intended for LuaJITBridge (zero GC table allocations, zero string concatenation in loops).

local ffi = require("ffi")

local HUD_COUNT = 2
local HUD_NODES = ffi.new("UILayoutNode[?]", HUD_COUNT)
local HUD_LABEL = "Project Engine HUD"
local SUB_LABEL = "Zero-Alloc FFI"

for i = 0, HUD_COUNT - 1 do
  HUD_NODES[i].bounds.x = 24.0 + i * 190.0
  HUD_NODES[i].bounds.y = 24.0
  HUD_NODES[i].bounds.w = 176.0
  HUD_NODES[i].bounds.h = 36.0
  HUD_NODES[i].color.r = 32
  HUD_NODES[i].color.g = 64
  HUD_NODES[i].color.b = 112
  HUD_NODES[i].color.a = 230
  HUD_NODES[i].rounding = 6.0
  HUD_NODES[i].visible = 1
end

function render_example_hud(draw_list_addr, width, height)
  local draw_list = nil
  if draw_list_addr ~= nil then
    draw_list = ffi.cast("void*", draw_list_addr)
  end

  ffi.C.igImDrawList_PushClipRect(draw_list, 0.0, 0.0, width, height, true)
  for i = 0, HUD_COUNT - 1 do
    local n = HUD_NODES[i]
    if n.visible == 1 then
      local col = bit.bor(
        bit.lshift(n.color.a, 24),
        bit.lshift(n.color.b, 16),
        bit.lshift(n.color.g, 8),
        n.color.r
      )
      ffi.C.igImDrawList_AddRectFilled(
        draw_list,
        n.bounds.x, n.bounds.y,
        n.bounds.x + n.bounds.w, n.bounds.y + n.bounds.h,
        col, n.rounding
      )
    end
  end
  ffi.C.igImDrawList_AddText(draw_list, 36.0, 34.0, 0xFFFFFFFF, HUD_LABEL)
  ffi.C.igImDrawList_AddText(draw_list, 226.0, 34.0, 0xFFB8E6FF, SUB_LABEL)
  ffi.C.igImDrawList_PopClipRect(draw_list)
end
