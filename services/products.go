package services

import (
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// ForceHideProduct hides a single product from every customer-facing
// storefront view, platform-side (super-admin taking down a fraudulent
// listing without suspending the whole shop — see ListProducts's
// read-only-except-this-flag doc comment in
// controllers/superadmin/productsController.go). Sets only
// HiddenByPlatformAt — never touches any other, merchant-owned field.
//
// Caller must also call InvalidateProductCaches after this — product and
// landing-page reads are cached (see productsController.go), so skipping it
// would let the storefront keep serving the product as visible for up to
// the cache TTL.
func ForceHideProduct(product *models.Product) error {
	now := time.Now()
	if err := initializers.DB.Model(product).Update("hidden_by_platform_at", now).Error; err != nil {
		return err
	}
	product.HiddenByPlatformAt = &now
	return nil
}

// UnhideProduct reverses ForceHideProduct. Same cache-invalidation
// requirement applies to the caller.
func UnhideProduct(product *models.Product) error {
	if err := initializers.DB.Model(product).Update("hidden_by_platform_at", nil).Error; err != nil {
		return err
	}
	product.HiddenByPlatformAt = nil
	return nil
}
