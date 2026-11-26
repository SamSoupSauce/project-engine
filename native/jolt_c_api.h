#ifndef JOLT_C_API_H
#define JOLT_C_API_H

#include <stdbool.h>
#include <stdint.h>

#ifdef _WIN32
#define JOLT_C_API_EXPORT __declspec(dllexport)
#else
#define JOLT_C_API_EXPORT __attribute__((visibility("default")))
#endif

#ifdef __cplusplus
extern "C" {
#endif

#ifndef JOLT_VEC3_DEFINED
#define JOLT_VEC3_DEFINED
typedef struct JoltVec3 {
    float x, y, z;
} JoltVec3;
#endif

typedef struct JoltRaycastHit {
    bool     has_hit;
    uint32_t body_id;
    float    fraction;
    JoltVec3 hit_position;
    JoltVec3 hit_normal;
    uint32_t user_data; // Maps to GameObject / Entity ID
} JoltRaycastHit;

// Raycast against the active Jolt physics world.
JOLT_C_API_EXPORT bool Jolt_CastRay(
    void* physics_system_ptr,
    const JoltVec3* origin,
    const JoltVec3* direction,
    float max_distance,
    uint16_t collision_layer_mask,
    JoltRaycastHit* out_hit
);

#ifdef __cplusplus
}
#endif

#endif // JOLT_C_API_H
