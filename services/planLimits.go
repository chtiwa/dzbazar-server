package services

import (
	"errors"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrPlanLimitReached is returned by the Check* functions below when a shop
// has hit its plan's cap for a resource. Controllers map it to a 403 response.
var ErrPlanLimitReached = errors.New("plan limit reached")

// unsubscribedPlan caps a shop with no ShopSubscription row (pre-billing shops,
// or a cancelled subscription) at the Basic tier's limits (see cmd/seedplans).
var unsubscribedPlan = models.Plan{
	MaxShops: 1, MaxProducts: 30, MaxOrders: 500, MaxLandingPages: 3,
	MaxUsers: 2, MaxFacebookPixels: 1, MaxTikTokPixels: 1,
}

func shopSubscription(shopID uuid.UUID) (models.ShopSubscription, error) {
	var sub models.ShopSubscription
	err := initializers.DB.Preload("Plan").Where("shop_id = ?", shopID).First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ShopSubscription{Plan: unsubscribedPlan}, nil
	}
	return sub, err
}

func checkCap(max int, count int64) error {
	if max == -1 || count < int64(max) {
		return nil
	}
	return ErrPlanLimitReached
}

func countCap(query *gorm.DB, max int) error {
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	return checkCap(max, count)
}

func CheckProductLimit(shopID uuid.UUID) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	return countCap(initializers.DB.Model(&models.Product{}).
		Where("shop_id = ? AND deleted_at IS NULL", shopID), sub.Plan.MaxProducts)
}

// CheckOrderLimit counts orders placed since the current subscription started,
// so the cap resets whenever the shop renews/upgrades rather than blocking forever.
func CheckOrderLimit(shopID uuid.UUID) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	query := initializers.DB.Model(&models.Order{}).Where("shop_id = ? AND deleted_at IS NULL", shopID)
	if !sub.StartedAt.IsZero() {
		query = query.Where("created_at >= ?", sub.StartedAt)
	}
	return countCap(query, sub.Plan.MaxOrders)
}

func CheckLandingPageLimit(shopID uuid.UUID) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	return countCap(initializers.DB.Model(&models.LandingPage{}).
		Where("shop_id = ? AND deleted_at IS NULL", shopID), sub.Plan.MaxLandingPages)
}

func CheckUserLimit(shopID uuid.UUID) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	return countCap(initializers.DB.Model(&models.ShopMember{}).
		Where("shop_id = ?", shopID), sub.Plan.MaxUsers)
}

func CheckPixelLimit(shopID uuid.UUID, platform string) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	max := sub.Plan.MaxFacebookPixels
	if platform == "tiktok" {
		max = sub.Plan.MaxTikTokPixels
	}
	return countCap(initializers.DB.Model(&models.Pixel{}).
		Where("shop_id = ? AND platform = ?", shopID, platform), max)
}

// CheckShopLimit caps how many shops one owner can hold. A user's first shop
// is always free; from the second shop on, the cap is the highest MaxShops
// among plans already subscribed to on their existing shops.
func CheckShopLimit(ownerID uuid.UUID) error {
	var shopIDs []uuid.UUID
	if err := initializers.DB.Model(&models.Shop{}).
		Where("owner_id = ? AND deleted_at IS NULL", ownerID).Pluck("id", &shopIDs).Error; err != nil {
		return err
	}
	if len(shopIDs) == 0 {
		return nil
	}

	var maxAllowed int
	if err := initializers.DB.Model(&models.ShopSubscription{}).
		Joins("JOIN plans ON plans.id = shop_subscriptions.plan_id").
		Where("shop_subscriptions.shop_id IN ?", shopIDs).
		Select("COALESCE(MAX(plans.max_shops), 1)").
		Scan(&maxAllowed).Error; err != nil {
		return err
	}

	return checkCap(maxAllowed, int64(len(shopIDs)))
}
