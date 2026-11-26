local ffi = require("ffi")
local physics = require("ffi_jolt")

-- Preallocated reticle layout & raycast result state (zero per-frame GC allocations).
local RETICLE_NODE = ffi.new("UILayoutNode")
local RETICLE_HIT_POS = ffi.new("JoltVec3")
local RETICLE_HIT_NORMAL = ffi.new("JoltVec3")

local reticle_hovered = 0
local reticle_body_id = 0
local reticle_user_data = 0
local reticle_fraction = 1.0

local function init_reticle()
	RETICLE_NODE.bounds.x = 0.0
	RETICLE_NODE.bounds.y = 0.0
	RETICLE_NODE.bounds.w = 8.0
	RETICLE_NODE.bounds.h = 8.0
	RETICLE_NODE.color.r = 230
	RETICLE_NODE.color.g = 230
	RETICLE_NODE.color.b = 230
	RETICLE_NODE.color.a = 200
	RETICLE_NODE.rounding = 4.0
	RETICLE_NODE.visible = 1
	RETICLE_NODE.hovered = 0
end

init_reticle()

-- Updates the first-person reticle hover state using zero-allocation Jolt_CastRay FFI.
function update_reticle_hover(world_ptr, ox, oy, oz, dx, dy, dz, max_dist, layer_mask)
	local ptr = world_ptr
	if ptr == nil then
		ptr = PHYSICS_SYSTEM_PTR
	end
	local hit = physics.cast_ray(ptr, ox, oy, oz, dx, dy, dz, max_dist, layer_mask)
	if hit ~= nil then
		reticle_hovered = 1
		reticle_body_id = hit.body_id
		reticle_user_data = hit.user_data
		reticle_fraction = hit.fraction
		RETICLE_HIT_POS.x = hit.hit_position.x
		RETICLE_HIT_POS.y = hit.hit_position.y
		RETICLE_HIT_POS.z = hit.hit_position.z
		RETICLE_HIT_NORMAL.x = hit.hit_normal.x
		RETICLE_HIT_NORMAL.y = hit.hit_normal.y
		RETICLE_HIT_NORMAL.z = hit.hit_normal.z

		RETICLE_NODE.hovered = 1
		RETICLE_NODE.bounds.w = 12.0
		RETICLE_NODE.bounds.h = 12.0
		RETICLE_NODE.color.r = 89
		RETICLE_NODE.color.g = 220
		RETICLE_NODE.color.b = 255
		RETICLE_NODE.color.a = 255
	else
		reticle_hovered = 0
		reticle_body_id = 0
		reticle_user_data = 0
		reticle_fraction = 1.0

		RETICLE_NODE.hovered = 0
		RETICLE_NODE.bounds.w = 8.0
		RETICLE_NODE.bounds.h = 8.0
		RETICLE_NODE.color.r = 230
		RETICLE_NODE.color.g = 230
		RETICLE_NODE.color.b = 230
		RETICLE_NODE.color.a = 200
	end
	return reticle_hovered == 1
end

function render_reticle(draw_list_addr, width, height)
	local draw_list = nil
	if draw_list_addr ~= nil then
		draw_list = ffi.cast("void*", draw_list_addr)
	end
	local half_w = RETICLE_NODE.bounds.w * 0.5
	local half_h = RETICLE_NODE.bounds.h * 0.5
	local cx = width * 0.5
	local cy = height * 0.5
	RETICLE_NODE.bounds.x = cx - half_w
	RETICLE_NODE.bounds.y = cy - half_h

	local packed_col = bit.bor(
		bit.lshift(RETICLE_NODE.color.a, 24),
		bit.lshift(RETICLE_NODE.color.b, 16),
		bit.lshift(RETICLE_NODE.color.g, 8),
		RETICLE_NODE.color.r
	)
	ffi.C.igImDrawList_AddRectFilled(
		draw_list,
		RETICLE_NODE.bounds.x,
		RETICLE_NODE.bounds.y,
		RETICLE_NODE.bounds.x + RETICLE_NODE.bounds.w,
		RETICLE_NODE.bounds.y + RETICLE_NODE.bounds.h,
		packed_col,
		RETICLE_NODE.rounding
	)
end

function get_reticle_state()
	return reticle_hovered, reticle_body_id, reticle_user_data, reticle_fraction
end
