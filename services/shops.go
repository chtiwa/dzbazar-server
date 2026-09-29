package services

import (
	"fmt"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
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
	if err := initializers.DB.Model(shop).Select("IsActive", "SuspendReason").Updates(shop).Error; err != nil {
		return err
	}
	InvalidateShopActiveCache(shop.ID)
	return nil
}

func shopActiveCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("shop:active:%s", shopID.String())
}

// IsShopActive reports whether the shop is active (not super-admin
// suspended), Redis-cached for 60s so the public storefront paths (shop by
// slug, product by slug, landing page) don't hit Postgres on every request
// just to check one column. A suspended shop must 404 everywhere a customer
// could still reach it, not just the product list — see important.todo
// LAUNCH BLOCKER 6.
func IsShopActive(shopID uuid.UUID) (bool, error) {
	key := shopActiveCacheKey(shopID)
	if cached, err := initializers.RClient.Get(initializers.Ctx, key).Result(); err == nil {
		return cached == "1", nil
	}

	var shop models.Shop
	if err := initializers.DB.Select("is_active").First(&shop, "id = ?", shopID).Error; err != nil {
		return false, err
	}

	val := "0"
	if shop.IsActive {
		val = "1"
	}
	_ = initializers.RClient.Set(initializers.Ctx, key, val, 60*time.Second).Err()

	return shop.IsActive, nil
}

// InvalidateShopActiveCache must be called after any write to Shop.IsActive
// (currently only super-admin UpdateShopStatus) so a suspend/reactivate takes
// effect immediately instead of waiting out the 60s cache TTL.
func InvalidateShopActiveCache(shopID uuid.UUID) {
	initializers.RClient.Del(initializers.Ctx, shopActiveCacheKey(shopID))
}
