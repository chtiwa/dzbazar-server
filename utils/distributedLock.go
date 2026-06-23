package utils

import (
	"log"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
)

// TryAcquireTickLock attempts to claim a fleet-wide lock for a single tick of
// a background job, using Redis SETNX with a TTL. Returns true if this
// instance won the tick and should run the job body; false if another
// instance already holds it, or if Redis is unreachable.
//
// Unlike middleware.OrderIPRateLimit (which fails open — a user-facing rate
// limiter must not block real traffic when Redis is down), this fails
// closed: on any Redis error we skip the tick rather than risk every
// replica running the job unguarded. Skipping one tick of a periodic job is
// harmless (the next tick retries); double-running it across replicas is
// not (duplicate emails, wasted third-party API calls).
//
// There is no explicit release: the TTL should be set just under the job's
// tick interval so the lock expires naturally before the next tick. These
// are "run at most once per tick across the fleet" jobs, not jobs needing an
// explicit handoff.
func TryAcquireTickLock(key string, ttl time.Duration) bool {
	ok, err := initializers.RClient.SetNX(initializers.Ctx, key, "1", ttl).Result()
	if err != nil {
		log.Printf("tick lock %q: redis error, skipping tick: %v", key, err)
		return false
	}
	return ok
}
