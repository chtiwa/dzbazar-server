package services

import (
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// UpdateShopStatus suspends or reactivates a shop. reason is the operator's
// note on why the shop was suspended — only meaningful (and persisted) when
// suspending; reactivating always clears it.
func UpdateShopStatus(shop *models.Shop, isActive bool, reason *string) error {
	shop.IsActive = isActive
	if isActive {
		shop.SuspendReason = nil
	} else {
		shop.SuspendReason = reason
	}
	return initializers.DB.Model(shop).Select("IsActive", "SuspendReason").Updates(shop).Error
}
