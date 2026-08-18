package services

import (
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// ForceDisableCoupon deactivates a coupon platform-side (super-admin shutting
// down an abusive/leaked discount code across tenants). Sets the same
// Active=false a merchant reaches via UpdateCoupon — no separate disabled
// state — so every other code path (couponDiscount, CouponAvailableForProduct)
// already treats it as off.
func ForceDisableCoupon(coupon *models.Coupon) error {
	coupon.Active = false
	return initializers.DB.Model(coupon).Select("Active").Updates(coupon).Error
}
