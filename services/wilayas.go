package services

import (
	"sync"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// wilayaCache is a process-local, manually-invalidated cache: the wilaya
// list is read on every shop creation (DeliveryRate seeding) but only ever
// written from the super-admin wilayas page, so re-querying Postgres on
// every read isn't worth it.
// ponytail: single-instance-safe cache (invalidated in-process on write).
// If this ever runs multi-instance, swap for a short-TTL Redis cache like
// services.SetFeatureFlagCache, so a write on instance A is seen by instance B.
var (
	wilayaCacheMu sync.RWMutex
	wilayaCache   []models.Wilaya
)

// GetWilayas returns all 58 wilayas ordered by id, loading them from the DB
// on first call and serving the cached copy after that.
func GetWilayas() ([]models.Wilaya, error) {
	wilayaCacheMu.RLock()
	if wilayaCache != nil {
		defer wilayaCacheMu.RUnlock()
		return wilayaCache, nil
	}
	wilayaCacheMu.RUnlock()

	wilayaCacheMu.Lock()
	defer wilayaCacheMu.Unlock()
	if wilayaCache != nil {
		return wilayaCache, nil
	}

	var wilayas []models.Wilaya
	if err := initializers.DB.Order("id ASC").Find(&wilayas).Error; err != nil {
		return nil, err
	}
	wilayaCache = wilayas
	return wilayaCache, nil
}

// InvalidateWilayaCache forces the next GetWilayas call to re-query the DB.
// Call after any admin update so delivery-rate seeding for the very next
// shop creation sees the new rates immediately.
func InvalidateWilayaCache() {
	wilayaCacheMu.Lock()
	wilayaCache = nil
	wilayaCacheMu.Unlock()
}
