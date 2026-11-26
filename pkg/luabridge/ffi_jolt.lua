local ffi = require("ffi")

if not pcall(ffi.typeof, "JoltVec3") then
	ffi.cdef[[
		typedef struct {
			float x, y, z;
		} JoltVec3;
	]]
end

if not pcall(ffi.typeof, "JoltRaycastHit") then
	ffi.cdef[[
		typedef struct {
			bool     has_hit;
			uint32_t body_id;
			float    fraction;
			JoltVec3 hit_position;
			JoltVec3 hit_normal;
			uint32_t user_data;
		} JoltRaycastHit;

		bool Jolt_CastRay(
			void* physics_system_ptr,
			const JoltVec3* origin,
			const JoltVec3* direction,
			float max_distance,
			uint16_t collision_layer_mask,
			JoltRaycastHit* out_hit
		);
	]]
end

-- Load native engine binary containing compiled JoltC exports.
local JoltLib = ffi.C

local M = {}

-- Preallocate cdata buffers to avoid per-frame GC churn on tick/hover.
local ray_origin = ffi.new("JoltVec3")
local ray_dir    = ffi.new("JoltVec3")
local hit_result = ffi.new("JoltRaycastHit")

function M.cast_ray(world_ptr, ox, oy, oz, dx, dy, dz, max_dist, layer_mask)
	if world_ptr == nil then
		return nil
	end
	ray_origin.x, ray_origin.y, ray_origin.z = ox, oy, oz
	ray_dir.x,    ray_dir.y,    ray_dir.z    = dx, dy, dz

	local hit = JoltLib.Jolt_CastRay(
		world_ptr,
		ray_origin,
		ray_dir,
		max_dist or 100.0,
		layer_mask or 0xFFFF,
		hit_result
	)

	if hit and hit_result.has_hit then
		return hit_result
	end
	return nil
end

function M.cast_ray_into(world_ptr, ox, oy, oz, dx, dy, dz, max_dist, layer_mask, out_hit)
	if world_ptr == nil then
		return false
	end
	ray_origin.x, ray_origin.y, ray_origin.z = ox, oy, oz
	ray_dir.x,    ray_dir.y,    ray_dir.z    = dx, dy, dz

	return JoltLib.Jolt_CastRay(
		world_ptr,
		ray_origin,
		ray_dir,
		max_dist or 100.0,
		layer_mask or 0xFFFF,
		out_hit
	)
end

return M
