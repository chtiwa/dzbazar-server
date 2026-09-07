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
	CreditsPerMonth: 0,
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

// IsFreeTier reports whether the shop is on a zero-price plan — Trial, no
// subscription at all, or a lapsed one. Free tiers get metered help chat;
// paid tiers are unlimited.
func IsFreeTier(shopID uuid.UUID) (bool, error) {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return false, err
	}
	return sub.Plan.Price == 0, nil
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

// Credit cost per AI action. Descriptions take a single prompt with no
// length tiers, so one flat rate; image generation cost varies by the model
// the merchant picked (see AIImageModelCredits).
const CreditCostDescription = 5

// creditCostForModel bills an unrecognized/empty model (e.g. usage rows from
// before per-model billing existed) at the priciest known tier — fails
// closed rather than under-billing.
func creditCostForModel(model string) int {
	if cost, ok := AIImageModelCredits[AIImageModel(model)]; ok {
		return cost
	}
	max := 0
	for _, cost := range AIImageModelCredits {
		if cost > max {
			max = cost
		}
	}
	return max
}

// creditsUsed is the weighted spend for the shop's current subscription
// period: the same COUNT(*)-since-StartedAt window the old per-feature caps
// used for descriptions, plus each image row billed at its own model's cost.
// Computed on the fly rather than stored, so renewal/upgrade (which rewrites
// StartedAt) is the only reset needed — same rule as CheckOrderLimit.
func creditsUsed(shopID uuid.UUID, sub models.ShopSubscription) (int64, error) {
	descriptions, err := periodScopedCount(shopID, sub, &models.AiDescriptionUsage{})
	if err != nil {
		return 0, err
	}

	query := initializers.DB.Model(&models.AiImageToolUsage{}).Where("shop_id = ?", shopID)
	if !sub.StartedAt.IsZero() {
		query = query.Where("created_at >= ?", sub.StartedAt)
	}
	var imageModels []string
	if err := query.Pluck("model", &imageModels).Error; err != nil {
		return 0, err
	}
	var imagesCost int64
	for _, m := range imageModels {
		imagesCost += int64(creditCostForModel(m))
	}

	return descriptions*CreditCostDescription + imagesCost, nil
}

// CheckCreditBudget requires `need` credits to still be available this
// period. Every AI caller pre-flights its full cost (5 for a description,
// 10 for a generated image) so a call fails fast instead of burning quota
// partway through.
func CheckCreditBudget(shopID uuid.UUID, need int) error {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return err
	}
	if sub.Plan.CreditsPerMonth == -1 {
		return nil
	}
	used, err := creditsUsed(shopID, sub)
	if err != nil {
		return err
	}
	if used+int64(need) > int64(sub.Plan.CreditsPerMonth) {
		return ErrPlanLimitReached
	}
	return nil
}

// CreditsSummary is the merchant-facing "X of Y credits used" figure shown in
// the navbar, Settings and the pricing page, reusing the exact same
// accounting CheckCreditBudget enforces against.
type CreditsSummary struct {
	Used  int64 `json:"used"`
	Total int   `json:"total"` // -1 = unlimited
}

func GetCreditsSummary(shopID uuid.UUID) (CreditsSummary, error) {
	sub, err := shopSubscription(shopID)
	if err != nil {
		return CreditsSummary{}, err
	}
	used, err := creditsUsed(shopID, sub)
	if err != nil {
		return CreditsSummary{}, err
	}
	return CreditsSummary{Used: used, Total: sub.Plan.CreditsPerMonth}, nil
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
