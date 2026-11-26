#ifndef PROJECT_ENGINE_JOLT_FFI_H
#define PROJECT_ENGINE_JOLT_FFI_H

#include <stdbool.h>
#include <stdint.h>

#ifdef _WIN32
#define JOLT_FFI_EXPORT __declspec(dllexport)
#else
#define JOLT_FFI_EXPORT __attribute__((visibility("default")))
#endif

#ifdef __cplusplus
extern "C" {
#endif

#ifndef JOLT_VEC3_DEFINED
#define JOLT_VEC3_DEFINED
typedef struct JoltVec3 {
    float x;
    float y;
    float z;
} JoltVec3;
#endif

typedef struct JoltHitResult {
    uint8_t hit;
    uint8_t _pad[3];
    uint32_t body_id;
    float distance;
    float fraction;
    float point_x;
    float point_y;
    float point_z;
    float normal_x;
    float normal_y;
    float normal_z;
} JoltHitResult;

JOLT_FFI_EXPORT bool Jolt_RaycastOut(void* physics_system, const JoltVec3* origin, const JoltVec3* dir, float max_distance, JoltHitResult* out_hit);
JOLT_FFI_EXPORT JoltHitResult Jolt_Raycast(void* physics_system, float ox, float oy, float oz, float dx, float dy, float dz, float max_distance);
JOLT_FFI_EXPORT void Jolt_AddImpulseVec3(void* body_interface, uint32_t body_id, const JoltVec3* impulse);
JOLT_FFI_EXPORT void Jolt_AddImpulse(void* body_interface, uint32_t body_id, float ix, float iy, float iz);
JOLT_FFI_EXPORT void Jolt_SetVelocityVec3(void* body_interface, uint32_t body_id, const JoltVec3* vel);
JOLT_FFI_EXPORT void Jolt_SetVelocity(void* body_interface, uint32_t body_id, float vx, float vy, float vz);

#ifdef __cplusplus
}
#endif

#endif
