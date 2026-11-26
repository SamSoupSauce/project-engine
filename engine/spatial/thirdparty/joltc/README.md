# JoltC Vendored Native Library

Built from:
- Upstream C-ABI: `amerkoleci/joltc`
- Upstream Physics: `jrouwe/JoltPhysics` (`v5.7.0`)

Compiler configuration (`windows/amd64`, MinGW-w64 GCC 16.2.0):
- C++17, `-O2 -fno-rtti -fno-exceptions -fvisibility=hidden`
- `-msse4.2 -mpopcnt -mavx -mavx2 -mbmi -mlzcnt -mf16c -mfma -mfpmath=sse`
- `-DJPH_OBJECT_LAYER_BITS=32 -DJPH_OBJECT_STREAM -DJPH_USE_CPU_COMPUTE`
- SIMD flags matching `JPH_USE_SSE4_1`, `JPH_USE_SSE4_2`, `JPH_USE_AVX`, `JPH_USE_AVX2`, `JPH_USE_LZCNT`, `JPH_USE_TZCNT`, `JPH_USE_F16C`, `JPH_USE_FMADD`
