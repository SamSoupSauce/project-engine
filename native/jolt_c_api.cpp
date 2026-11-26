#include "jolt_c_api.h"
#include "joltc.h"

#include <atomic>
#include <math.h>
#include <string.h>

namespace {

constexpr uint32_t kInvalidBodyID = 0xFFFFFFFFu;
constexpr uint16_t kAllLayersMask = 0xFFFFu;
constexpr size_t   kCachedMaskCount = 16;

bool JPH_API_CALL LayerMaskShouldCollide(void* userData, JPH_ObjectLayer layer) {
    if (layer >= 16u) {
        return false;
    }
    const uintptr_t mask = reinterpret_cast<uintptr_t>(userData);
    return (mask & (static_cast<uintptr_t>(1u) << layer)) != 0;
}

JPH_ObjectLayerFilter_Procs g_layerFilterProcs = { &LayerMaskShouldCollide };
std::atomic<JPH_ObjectLayerFilter*> g_cachedLayerFilters[kCachedMaskCount] = {};

void EnsureLayerFilterProcs() {
    static std::atomic<int> initState{0};
    int expected = 0;
    if (initState.compare_exchange_strong(expected, 1, std::memory_order_acq_rel)) {
        JPH_ObjectLayerFilter_SetProcs(&g_layerFilterProcs);
        initState.store(2, std::memory_order_release);
    } else {
        while (initState.load(std::memory_order_acquire) != 2) {
        }
    }
}

JPH_ObjectLayerFilter* AcquireLayerFilter(uint16_t mask, bool* outShouldDestroy) {
    *outShouldDestroy = false;
    if (mask == kAllLayersMask) {
        return nullptr;
    }
    EnsureLayerFilterProcs();

    if (mask < kCachedMaskCount) {
        JPH_ObjectLayerFilter* cached = g_cachedLayerFilters[mask].load(std::memory_order_acquire);
        if (cached != nullptr) {
            return cached;
        }
        JPH_ObjectLayerFilter* created = JPH_ObjectLayerFilter_Create(
            reinterpret_cast<void*>(static_cast<uintptr_t>(mask))
        );
        JPH_ObjectLayerFilter* prev = nullptr;
        if (!g_cachedLayerFilters[mask].compare_exchange_strong(prev, created, std::memory_order_acq_rel)) {
            JPH_ObjectLayerFilter_Destroy(created);
            return prev;
        }
        return created;
    }

    *outShouldDestroy = true;
    return JPH_ObjectLayerFilter_Create(
        reinterpret_cast<void*>(static_cast<uintptr_t>(mask))
    );
}

} // namespace

extern "C" JOLT_C_API_EXPORT bool Jolt_CastRay(
    void* physics_system_ptr,
    const JoltVec3* origin,
    const JoltVec3* direction,
    float max_distance,
    uint16_t collision_layer_mask,
    JoltRaycastHit* out_hit
) {
    if (!out_hit) {
        return false;
    }
    memset(out_hit, 0, sizeof(JoltRaycastHit));
    out_hit->body_id = kInvalidBodyID;
    out_hit->fraction = 1.0f;

    if (!physics_system_ptr || !origin || !direction || max_distance <= 0.0f || collision_layer_mask == 0) {
        return false;
    }

    const float dx = direction->x;
    const float dy = direction->y;
    const float dz = direction->z;
    const float lenSq = dx * dx + dy * dy + dz * dz;
    if (lenSq <= 1e-12f) {
        return false;
    }

    const float invLen = 1.0f / sqrtf(lenSq);
    const JPH_RVec3 jOrigin = { origin->x, origin->y, origin->z };
    const JPH_Vec3 jDir = {
        dx * invLen * max_distance,
        dy * invLen * max_distance,
        dz * invLen * max_distance
    };

    auto* sys = reinterpret_cast<JPH_PhysicsSystem*>(physics_system_ptr);
    const JPH_NarrowPhaseQuery* nq = JPH_PhysicsSystem_GetNarrowPhaseQuery(sys);
    if (!nq) {
        return false;
    }

    bool shouldDestroyFilter = false;
    JPH_ObjectLayerFilter* layerFilter = AcquireLayerFilter(collision_layer_mask, &shouldDestroyFilter);

    JPH_RayCastResult rayHit{};
    rayHit.bodyID = kInvalidBodyID;
    rayHit.fraction = 1.0f;

    const bool didHit = JPH_NarrowPhaseQuery_CastRay(
        nq,
        &jOrigin,
        &jDir,
        &rayHit,
        nullptr,
        layerFilter,
        nullptr
    );

    if (shouldDestroyFilter && layerFilter != nullptr) {
        JPH_ObjectLayerFilter_Destroy(layerFilter);
    }

    if (!didHit || rayHit.bodyID == kInvalidBodyID) {
        return false;
    }

    out_hit->has_hit = true;
    out_hit->body_id = rayHit.bodyID;
    out_hit->fraction = rayHit.fraction;

    const JPH_RVec3 hitPoint = {
        jOrigin.x + jDir.x * rayHit.fraction,
        jOrigin.y + jDir.y * rayHit.fraction,
        jOrigin.z + jDir.z * rayHit.fraction
    };
    out_hit->hit_position.x = hitPoint.x;
    out_hit->hit_position.y = hitPoint.y;
    out_hit->hit_position.z = hitPoint.z;

    const JPH_BodyLockInterface* lockIface = JPH_PhysicsSystem_GetBodyLockInterface(sys);
    if (lockIface) {
        JPH_BodyLockRead lockRead{};
        JPH_BodyLockInterface_LockRead(lockIface, rayHit.bodyID, &lockRead);
        if (lockRead.body) {
            JPH_Vec3 normal = { 0.0f, 1.0f, 0.0f };
            JPH_Body_GetWorldSpaceSurfaceNormal(lockRead.body, rayHit.subShapeID2, &hitPoint, &normal);
            out_hit->hit_normal.x = normal.x;
            out_hit->hit_normal.y = normal.y;
            out_hit->hit_normal.z = normal.z;

            const uint64_t ud = JPH_Body_GetUserData(const_cast<JPH_Body*>(lockRead.body));
            out_hit->user_data = static_cast<uint32_t>(ud & 0xFFFFFFFFu);
        }
        JPH_BodyLockInterface_UnlockRead(lockIface, &lockRead);
    }

    return true;
}
