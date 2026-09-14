package services

import (
	"sync"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// communeCache mirrors wilayaCache in wilayas.go: process-local, read-heavy,
// never written at runtime (no admin edit UI for communes).
// ponytail: single-instance-safe cache, same as GetWilayas.
var (
	communeCacheMu sync.RWMutex
	communeCache   []models.Commune
)

// GetCommunes returns all communes ordered by id, loading them from the DB
// on first call and serving the cached copy after that.
func GetCommunes() ([]models.Commune, error) {
	communeCacheMu.RLock()
	if communeCache != nil {
		defer communeCacheMu.RUnlock()
		return communeCache, nil
	}
	communeCacheMu.RUnlock()

	communeCacheMu.Lock()
	defer communeCacheMu.Unlock()
	if communeCache != nil {
		return communeCache, nil
	}

	var communes []models.Commune
	if err := initializers.DB.Order("id ASC").Find(&communes).Error; err != nil {
		return nil, err
	}
	communeCache = communes
	return communeCache, nil
}
