package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// featureFlagCacheTTL is short on purpose: a flag is an incident kill switch,
// so an operator toggling one needs it live in seconds, not minutes, even on
// the (rare) read that misses the immediate SetFeatureFlagCache below.
const featureFlagCacheTTL = 10 * time.Second

func featureFlagCacheKey(key string) string {
	return fmt.Sprintf("feature-flag:%s", key)
}

// IsFeatureEnabled reports a flag's current value, Redis-cached so a hot
// write path never hits Postgres per-request. A cache/DB miss (including an
// unknown key) defaults to enabled — a flag that doesn't exist yet, or a
// lookup error, must never accidentally kill a feature platform-wide.
func IsFeatureEnabled(key string) bool {
	cacheKey := featureFlagCacheKey(key)
	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Bytes(); err == nil {
		var enabled bool
		if json.Unmarshal(cached, &enabled) == nil {
			return enabled
		}
	}

	enabled := true
	var flag models.FeatureFlag
	if err := initializers.DB.Where("key = ?", key).First(&flag).Error; err == nil {
		enabled = flag.IsEnabled
	}

	if encoded, err := json.Marshal(enabled); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, encoded, featureFlagCacheTTL)
	}
	return enabled
}

// SetFeatureFlagCache overwrites the cached value right after a superadmin
// create/toggle, so the new state is live across every server instance
// immediately instead of waiting out featureFlagCacheTTL.
func SetFeatureFlagCache(key string, enabled bool) {
	if encoded, err := json.Marshal(enabled); err == nil {
		initializers.RClient.Set(initializers.Ctx, featureFlagCacheKey(key), encoded, featureFlagCacheTTL)
	}
}
