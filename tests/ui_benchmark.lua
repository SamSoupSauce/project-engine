local ffi = require("ffi")

local benchmark_a = ffi.new("Rect")
local benchmark_b = ffi.new("Rect")
local benchmark_out = ffi.new("Rect")

benchmark_a.x = 0.0
benchmark_a.y = 0.0
benchmark_a.w = 100.0
benchmark_a.h = 40.0

benchmark_b.x = 50.0
benchmark_b.y = 10.0
benchmark_b.w = 200.0
benchmark_b.h = 80.0

local function run_frames(count)
	for _ = 1, count do
		run_ui_tick(1.0 / 60.0)
		lerp_rect(benchmark_out, benchmark_a, benchmark_b, 0.5)
		intersect_rects(benchmark_out, benchmark_out, benchmark_b)
		render_ui_frame(nil, 1280.0, 720.0)
	end
end

-- Run warm-up frames to trigger trace compilation
run_frames(2000)

collectgarbage("collect")
collectgarbage("collect")
run_frames(1)
local mem_before = collectgarbage("count")

-- Execute 1,000 continuous UI frames
run_frames(1000)

local mem_after = collectgarbage("count")
assert(mem_before == mem_after, string.format("Memory leaked in UI loop: %.2f KB allocated", mem_after - mem_before))
