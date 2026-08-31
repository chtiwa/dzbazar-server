package services

import (
	"errors"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrPlanLimitReached is returned by the Check* functions below when a shop
// has hit its plan's cap for a resource. Controllers map it to a 403 response.
var ErrPlanLimitReached = errors.New("plan limit reached")

// unsubscribedPlan caps a shop with no ShopSubscription row (pre-billing shops,
// or a cancelled subscription) at the Basic tier's limits.
var unsubscribedPlan = models.Plan{
	MaxShops: 1, MaxProducts: 30, MaxOrders: 500, MaxLandingPages: 3,
	MaxUsers: 2, MaxFacebookPixels: 1, MaxTikTokPixels: 1,
	MaxAiDescriptionsPerMonth: 0,
	MaxAiImagesPerMonth:       0,
}

// expiredPlan locks a shop out entirely once its trial or paid period has
// lapsed (ExpiresAt in the past) — every cap is 0 until they renew via an
// approved invoice, which resets StartedAt/ExpiresAt.
var expiredPlan = models.Plan{}

func shopSubscription(shopID uuid.UUID) (models.ShopSubscription, error) {
	var sub models.ShopSubscription
	err := initializers.DB.Preload("Plan").Where("shop_id = ?", shopID).First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ShopSubscription{Plan: unsubscribedPlan}, nil
	}
	if err != nil {
		return sub, err
	}
	if sub.ExpiresAt != nil && sub.ExpiresAt.Before(time.Now()) {
		sub.Plan = expiredPlan
	}
	return sub, nil
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

func periodScopedCount(shopID uuid.UUID, sub models.ShopSubscription, model any) (int64, error) {
	query := initializers.DB.Model(model).Where("shop_id = ?", shopID)
	if !sub.StartedAt.IsZero() {
		query = query.Where("created_at >= ?", sub.StartedAt)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

// CheckAiDescriptionLimit counts AI description generations since the
// current subscription period started, same reset rule as CheckOrderLimit —
// the cap resets on renewal/upgrade rather than blocking forever.
func CheckAiDescriptionLimit(shopID uuid.UUID) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	count, err := periodScopedCount(shopID, sub, &models.AiDescriptionUsage{})
	if err != nil {
		return err
	}
	return checkCap(sub.Plan.MaxAiDescriptionsPerMonth, count)
}

// CheckLandingPageImageGenLimit counts AI image generations since the
// current subscription period started, same reset rule as CheckOrderLimit.
func CheckLandingPageImageGenLimit(shopID uuid.UUID) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	count, err := periodScopedCount(shopID, sub, &models.LandingPageImageGenUsage{})
	if err != nil {
		return err
	}
	return checkCap(sub.Plan.MaxAiImagesPerMonth, count)
}

// CheckLandingPageImageGenBudget requires at least `need` image-gen slots to
// still be free this period — used before a multi-image batch (e.g. the
// 5-image section set) so it fails fast instead of burning quota partway
// through and leaving a half-finished set.
func CheckLandingPageImageGenBudget(shopID uuid.UUID, need int) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	if sub.Plan.MaxAiImagesPerMonth == -1 {
		return nil
	}
	count, err := periodScopedCount(shopID, sub, &models.LandingPageImageGenUsage{})
	if err != nil {
		return err
	}
	if count+int64(need) > int64(sub.Plan.MaxAiImagesPerMonth) {
		return ErrPlanLimitReached
	}
	return nil
}

// AiUsageSummary is the merchant-facing "X of Y used this period" figures
// shown in Settings/Plans, reusing the exact same counting rule the
// Check*Limit functions enforce against.
type AiUsageSummary struct {
	DescriptionsUsed int64 `json:"descriptionsUsed"`
	ImagesUsed       int64 `json:"imagesUsed"`
}

func GetAiUsageSummary(shopID uuid.UUID) (AiUsageSummary, error) {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return AiUsageSummary{}, err
	}
	descriptionsUsed, err := periodScopedCount(shopID, sub, &models.AiDescriptionUsage{})
	if err != nil {
		return AiUsageSummary{}, err
	}
	imagesUsed, err := periodScopedCount(shopID, sub, &models.LandingPageImageGenUsage{})
	if err != nil {
		return AiUsageSummary{}, err
	}
	return AiUsageSummary{DescriptionsUsed: descriptionsUsed, ImagesUsed: imagesUsed}, nil
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
