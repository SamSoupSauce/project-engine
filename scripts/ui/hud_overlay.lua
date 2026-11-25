local ffi = require("ffi")

-- Pre-allocated module-scoped scratch structs and contiguous node pool.
-- Zero table constructors or string concatenations occur in frame loops.
local MAX_NODES = 16
local NODE_POOL = ffi.new("UILayoutNode[?]", MAX_NODES)
local SCRATCH_RECT = ffi.new("UIRect")
local SCRATCH_COLOR = ffi.new("UIColor")

local active_count = 4
local anim_phase = 0.0
local click_count = 0
local hover_count = 0

local LABEL_HUD = "HUD"
local LABEL_PLAY = "Play"

local function init_nodes()
	local i = 0
	for i = 0, active_count - 1 do
		NODE_POOL[i].bounds.x = 20.0 + i * 150.0
		NODE_POOL[i].bounds.y = 20.0
		NODE_POOL[i].bounds.w = 130.0
		NODE_POOL[i].bounds.h = 44.0
		NODE_POOL[i].rounding = 6.0
		NODE_POOL[i].visible = 1
		NODE_POOL[i].hovered = 0
	end
end

init_nodes()

function run_ui_tick(dt)
	anim_phase = anim_phase + dt
	if anim_phase > 6.2831853 then
		anim_phase = anim_phase - 6.2831853
	end

	local pulse = 0.5 + 0.5 * math.sin(anim_phase * 2.0)
	local i = 0
	for i = 0, active_count - 1 do
		local node = NODE_POOL[i]
		if node.visible == 1 then
			SCRATCH_RECT.x = 20.0 + i * 150.0
			SCRATCH_RECT.y = 20.0 + pulse * 4.0
			SCRATCH_RECT.w = 130.0
			SCRATCH_RECT.h = 44.0

			node.bounds.x = SCRATCH_RECT.x
			node.bounds.y = SCRATCH_RECT.y
			node.bounds.w = SCRATCH_RECT.w
			node.bounds.h = SCRATCH_RECT.h

			if node.hovered == 1 then
				SCRATCH_COLOR.r = 0.35
				SCRATCH_COLOR.g = 0.70
				SCRATCH_COLOR.b = 0.95
				SCRATCH_COLOR.a = 1.0
			else
				SCRATCH_COLOR.r = 0.16
				SCRATCH_COLOR.g = 0.24
				SCRATCH_COLOR.b = 0.36
				SCRATCH_COLOR.a = 0.95
			end
			node.color = SCRATCH_COLOR
		end
	end
end

function render_ui_frame(draw_list_addr, width, height)
	local draw_list = nil
	if draw_list_addr ~= nil then
		draw_list = ffi.cast("void*", draw_list_addr)
	end
	ffi.C.DrawList_PushClipRect(draw_list, 0.0, 0.0, width, height, true)
	local i = 0
	for i = 0, active_count - 1 do
		local node = NODE_POOL[i]
		if node.visible == 1 then
			local min_x = node.bounds.x
			local min_y = node.bounds.y
			local max_x = min_x + node.bounds.w
			local max_y = min_y + node.bounds.h
			local packed_col = 0xFF5C3D29
			if node.hovered == 1 then
				packed_col = 0xFFF2B359
			end
			ffi.C.DrawList_AddRectFilled(draw_list, min_x, min_y, max_x, max_y, packed_col, node.rounding)
		end
	end
	ffi.C.DrawList_AddText(draw_list, 28.0, 32.0, 0xFFFFFFFF, LABEL_HUD)
	ffi.C.DrawList_AddText(draw_list, 178.0, 32.0, 0xFFFFFFFF, LABEL_PLAY)
	ffi.C.DrawList_PopClipRect(draw_list)
end

function on_play_click(node_id, mx, my)
	click_count = click_count + 1
	NODE_POOL[0].hovered = 1
	return true
end

function on_panel_hover(node_id, mx, my)
	hover_count = hover_count + 1
	NODE_POOL[0].hovered = 1
	return true
end

function get_ui_stats()
	return click_count, hover_count
end
