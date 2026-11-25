local ffi = require("ffi")

-- Module-scoped pre-allocated scratch structs and contiguous node pool.
-- Strictly zero table allocations, zero string concatenations, and numeric loops only.
local MAX_NODES = 64
local NODE_POOL = ffi.new("UILayoutNode[?]", MAX_NODES)
local SCRATCH_RECT = ffi.new("Rect")
local SCRATCH_VEC = ffi.new("Vec2")
local SCRATCH_COLOR = ffi.new("Color32")
local CLIP_RECT = ffi.new("Rect")
local VIEW_RECT = ffi.new("Rect")

local active_count = 4
local anim_phase = 0.0
local click_count = 0
local hover_count = 0

local LABEL_HUD = "HUD"
local LABEL_PLAY = "Play"

-- In-place rectangle intersection writing directly into out (Rect*).
function intersect_rects(out, a, b)
	local x0 = a.x > b.x and a.x or b.x
	local y0 = a.y > b.y and a.y or b.y
	local ax1 = a.x + a.w
	local bx1 = b.x + b.w
	local ay1 = a.y + a.h
	local by1 = b.y + b.h
	local x1 = ax1 < bx1 and ax1 or bx1
	local y1 = ay1 < by1 and ay1 or by1
	out.x = x0
	out.y = y0
	out.w = x1 > x0 and (x1 - x0) or 0.0
	out.h = y1 > y0 and (y1 - y0) or 0.0
end

-- In-place linear easing operating strictly on primitive floats and writing into out (Rect*).
function lerp_rect(out, a, b, t)
	out.x = a.x + (b.x - a.x) * t
	out.y = a.y + (b.y - a.y) * t
	out.w = a.w + (b.w - a.w) * t
	out.h = a.h + (b.h - a.h) * t
end

-- Cubic bezier easing operating strictly on primitive floats.
function bezier_ease(t)
	local inv = 1.0 - t
	return 3.0 * inv * inv * t * 0.25 + 3.0 * inv * t * t * 0.9 + t * t * t
end

local function init_nodes()
	for i = 0, active_count - 1 do
		local node = NODE_POOL[i]
		node.bounds.x = 20.0 + i * 150.0
		node.bounds.y = 20.0
		node.bounds.w = 130.0
		node.bounds.h = 44.0
		node.color.r = 41
		node.color.g = 61
		node.color.b = 92
		node.color.a = 242
		node.rounding = 6.0
		node.id_hash = i + 1
		node.visible = 1
		node.hovered = 0
	end
end

init_nodes()

function run_ui_tick(dt)
	anim_phase = anim_phase + dt
	anim_phase = anim_phase - math.floor(anim_phase)

	local eased = bezier_ease(anim_phase)
	VIEW_RECT.x = 0.0
	VIEW_RECT.y = 0.0
	VIEW_RECT.w = 1280.0
	VIEW_RECT.h = 720.0

	for i = 0, active_count - 1 do
		if NODE_POOL[i].visible == 1 then
			SCRATCH_VEC.x = 20.0 + i * 150.0
			SCRATCH_VEC.y = 20.0 + eased * 4.0

			SCRATCH_RECT.x = SCRATCH_VEC.x
			SCRATCH_RECT.y = SCRATCH_VEC.y
			SCRATCH_RECT.w = 130.0
			SCRATCH_RECT.h = 44.0

			intersect_rects(SCRATCH_RECT, SCRATCH_RECT, VIEW_RECT)
			NODE_POOL[i].bounds.x = SCRATCH_RECT.x
			NODE_POOL[i].bounds.y = SCRATCH_RECT.y
			NODE_POOL[i].bounds.w = SCRATCH_RECT.w
			NODE_POOL[i].bounds.h = SCRATCH_RECT.h

			if NODE_POOL[i].hovered == 1 then
				SCRATCH_COLOR.r = 89
				SCRATCH_COLOR.g = 179
				SCRATCH_COLOR.b = 242
				SCRATCH_COLOR.a = 255
			else
				SCRATCH_COLOR.r = 41
				SCRATCH_COLOR.g = 61
				SCRATCH_COLOR.b = 92
				SCRATCH_COLOR.a = 242
			end
			NODE_POOL[i].color.r = SCRATCH_COLOR.r
			NODE_POOL[i].color.g = SCRATCH_COLOR.g
			NODE_POOL[i].color.b = SCRATCH_COLOR.b
			NODE_POOL[i].color.a = SCRATCH_COLOR.a
		end
	end
end

function render_ui_frame(draw_list_addr, width, height)
	local draw_list = nil
	if draw_list_addr ~= nil then
		draw_list = ffi.cast("void*", draw_list_addr)
	end

	CLIP_RECT.x = 0.0
	CLIP_RECT.y = 0.0
	CLIP_RECT.w = width
	CLIP_RECT.h = height

	ffi.C.igImDrawList_PushClipRect(draw_list, CLIP_RECT.x, CLIP_RECT.y, CLIP_RECT.w, CLIP_RECT.h, true)
	for i = 0, active_count - 1 do
		if NODE_POOL[i].visible == 1 then
			local packed_col = bit.bor(
				bit.lshift(NODE_POOL[i].color.a, 24),
				bit.lshift(NODE_POOL[i].color.b, 16),
				bit.lshift(NODE_POOL[i].color.g, 8),
				NODE_POOL[i].color.r
			)
			ffi.C.igImDrawList_AddRectFilled(
				draw_list,
				NODE_POOL[i].bounds.x,
				NODE_POOL[i].bounds.y,
				NODE_POOL[i].bounds.x + NODE_POOL[i].bounds.w,
				NODE_POOL[i].bounds.y + NODE_POOL[i].bounds.h,
				packed_col,
				NODE_POOL[i].rounding
			)
		end
	end
	ffi.C.igImDrawList_AddText(draw_list, 28.0, 32.0, 0xFFFFFFFF, LABEL_HUD)
	ffi.C.igImDrawList_AddText(draw_list, 178.0, 32.0, 0xFFFFFFFF, LABEL_PLAY)
	ffi.C.igImDrawList_PopClipRect(draw_list)
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
