#include "jolt_ffi.h"
#include "joltc.h"
#include <math.h>
#include <string.h>

#define JOLT_INVALID_BODY_ID 0xFFFFFFFFu

JOLT_FFI_EXPORT bool Jolt_RaycastOut(void* physics_system, const JoltVec3* origin, const JoltVec3* dir, float max_distance, JoltHitResult* out_hit) {
    if (!out_hit) {
        return false;
    }
    memset(out_hit, 0, sizeof(JoltHitResult));
    out_hit->body_id = JOLT_INVALID_BODY_ID;

    if (!physics_system || !origin || !dir || max_distance <= 0.0f) {
        return false;
    }

    float dx = dir->x;
    float dy = dir->y;
    float dz = dir->z;
    float len = sqrtf(dx * dx + dy * dy + dz * dz);
    if (len <= 1e-8f) {
        return false;
    }

    float inv = 1.0f / len;
    JPH_RVec3 jOrigin = { origin->x, origin->y, origin->z };
    JPH_Vec3 jDir = { dx * inv * max_distance, dy * inv * max_distance, dz * inv * max_distance };

    JPH_PhysicsSystem* sys = (JPH_PhysicsSystem*)physics_system;
    const JPH_NarrowPhaseQuery* nq = JPH_PhysicsSystem_GetNarrowPhaseQuery(sys);
    if (!nq) {
        return false;
    }

    JPH_RayCastResult rayHit;
    memset(&rayHit, 0, sizeof(rayHit));
    rayHit.bodyID = JOLT_INVALID_BODY_ID;
    rayHit.fraction = 1.0f;

    if (!JPH_NarrowPhaseQuery_CastRay(nq, &jOrigin, &jDir, &rayHit, NULL, NULL, NULL)) {
        return false;
    }

    out_hit->hit = 1;
    out_hit->body_id = rayHit.bodyID;
    out_hit->fraction = rayHit.fraction;
    out_hit->distance = rayHit.fraction * max_distance;

    JPH_RVec3 hitPoint = {
        jOrigin.x + jDir.x * rayHit.fraction,
        jOrigin.y + jDir.y * rayHit.fraction,
        jOrigin.z + jDir.z * rayHit.fraction
    };
    out_hit->point_x = hitPoint.x;
    out_hit->point_y = hitPoint.y;
    out_hit->point_z = hitPoint.z;

    const JPH_BodyLockInterface* lockIface = JPH_PhysicsSystem_GetBodyLockInterface(sys);
    if (lockIface) {
        JPH_BodyLockRead lockRead;
        memset(&lockRead, 0, sizeof(lockRead));
        JPH_BodyLockInterface_LockRead(lockIface, rayHit.bodyID, &lockRead);
        if (lockRead.body) {
            JPH_Vec3 normal = { 0.0f, 1.0f, 0.0f };
            JPH_Body_GetWorldSpaceSurfaceNormal(lockRead.body, rayHit.subShapeID2, &hitPoint, &normal);
            out_hit->normal_x = normal.x;
            out_hit->normal_y = normal.y;
            out_hit->normal_z = normal.z;
        }
        JPH_BodyLockInterface_UnlockRead(lockIface, &lockRead);
    }

    return true;
}

JOLT_FFI_EXPORT JoltHitResult Jolt_Raycast(void* physics_system, float ox, float oy, float oz, float dx, float dy, float dz, float max_distance) {
    JoltVec3 origin = { ox, oy, oz };
    JoltVec3 dir = { dx, dy, dz };
    JoltHitResult result;
    Jolt_RaycastOut(physics_system, &origin, &dir, max_distance, &result);
    return result;
}

JOLT_FFI_EXPORT void Jolt_AddImpulseVec3(void* body_interface, uint32_t body_id, const JoltVec3* impulse) {
    if (!body_interface || !impulse || body_id == JOLT_INVALID_BODY_ID) {
        return;
    }
    JPH_Vec3 v = { impulse->x, impulse->y, impulse->z };
    JPH_BodyInterface_AddImpulse((JPH_BodyInterface*)body_interface, (JPH_BodyID)body_id, &v);
}

JOLT_FFI_EXPORT void Jolt_AddImpulse(void* body_interface, uint32_t body_id, float ix, float iy, float iz) {
    JoltVec3 v = { ix, iy, iz };
    Jolt_AddImpulseVec3(body_interface, body_id, &v);
}

JOLT_FFI_EXPORT void Jolt_SetVelocityVec3(void* body_interface, uint32_t body_id, const JoltVec3* vel) {
    if (!body_interface || !vel || body_id == JOLT_INVALID_BODY_ID) {
        return;
    }
    JPH_Vec3 v = { vel->x, vel->y, vel->z };
    JPH_BodyInterface* iface = (JPH_BodyInterface*)body_interface;
    if (v.x != 0.0f || v.y != 0.0f || v.z != 0.0f) {
        JPH_BodyInterface_ActivateBody(iface, (JPH_BodyID)body_id);
    }
    JPH_BodyInterface_SetLinearVelocity(iface, (JPH_BodyID)body_id, &v);
}

JOLT_FFI_EXPORT void Jolt_SetVelocity(void* body_interface, uint32_t body_id, float vx, float vy, float vz) {
    JoltVec3 v = { vx, vy, vz };
    Jolt_SetVelocityVec3(body_interface, body_id, &v);
}
