package superadmin

import (
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListSubscriptions is the "Payments" page data source: a read-only ledger
// over ShopSubscription x Plan x Shop. There is no payment gateway wired up
// yet, so this is subscription history, not a record of money actually moved.
func ListSubscriptions(c *gin.Context) {
	shopID := strings.TrimSpace(c.Query("shopId"))
	search := strings.TrimSpace(c.Query("search"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.ShopSubscription{}).Preload("Plan")

	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}
	// Search by shop name or plan name — neither lives on shop_subscriptions
	// itself, so it needs a join. Select is pinned to shop_subscriptions.* to
	// avoid the id/created_at/etc columns shared via BaseModel colliding with
	// the same-named columns on shops/plans once joined.
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		db = db.Joins("JOIN shops ON shops.id = shop_subscriptions.shop_id").
			Joins("JOIN plans ON plans.id = shop_subscriptions.plan_id").
			Select("shop_subscriptions.*").
			Where("LOWER(shops.name) LIKE ? OR LOWER(plans.name) LIKE ?", like, like)
	}

	order := resolveSort(c, map[string]string{
		"startedAt":  "started_at",
		"expiresAt":  "expires_at",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var subs []models.ShopSubscription
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&subs).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch subscriptions", err)
		return
	}

	shopIDs := make([]string, 0, len(subs))
	seen := map[string]bool{}
	for _, s := range subs {
		idStr := s.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
	}

	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		var shops []models.Shop
		initializers.DB.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, sh := range shops {
			shopByID[sh.ID.String()] = sh
		}
	}

	type subWithShop struct {
		models.ShopSubscription
		ShopName string `json:"shopName"`
		ShopSlug string `json:"shopSlug"`
	}

	result := make([]subWithShop, 0, len(subs))
	for _, s := range subs {
		shop := shopByID[s.ShopID.String()]
		result = append(result, subWithShop{ShopSubscription: s, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

type SetSubscriptionInput struct {
	PlanID    string     `json:"planId" binding:"required"`
	StartedAt *time.Time `json:"startedAt"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// SetShopSubscription lets a super admin manually assign or change a shop's
// subscription plan, with an optional custom start/expiry date (default
// expiry: 30 days from start). Mirrors the upsert-by-shop-id pattern used by
// the owner-initiated SubscribeShopToPlan in plansController.go.
func SetShopSubscription(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body SetSubscriptionInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	planID, err := uuid.Parse(body.PlanID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid plan ID"})
		return
	}

	var plan models.Plan
	if err := initializers.DB.First(&plan, "id = ? AND is_active = true", planID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Plan not found or inactive"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	start := time.Now()
	if body.StartedAt != nil {
		start = *body.StartedAt
	}

	expires := start.AddDate(0, 0, 30)
	if body.ExpiresAt != nil {
		expires = *body.ExpiresAt
	}

	var sub models.ShopSubscription
	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		existing := tx.Where("shop_id = ?", shopID).First(&sub)
		if existing.Error != nil && existing.Error != gorm.ErrRecordNotFound {
			return existing.Error
		}

		if existing.Error == gorm.ErrRecordNotFound {
			sub = models.ShopSubscription{
				ShopID:    shopID,
				PlanID:    planID,
				StartedAt: start,
				ExpiresAt: &expires,
			}
			return tx.Create(&sub).Error
		}

		return tx.Model(&sub).Updates(map[string]any{
			"plan_id":                 planID,
			"started_at":              start,
			"expires_at":              expires,
			"expiry_reminder_sent_at": nil,
		}).Error
	})

	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to update subscription", err)
		return
	}

	utils.LogAudit(c, "subscription.set", "ShopSubscription", &sub.ID, gin.H{"shopId": shopID, "planId": planID, "expiresAt": expires})

	initializers.DB.Preload("Plan").First(&sub, "shop_id = ?", shopID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Subscription updated", "data": sub})
}

// CancelSubscription removes a shop's subscription entirely (as opposed to
// SetShopSubscription, which only ever swaps the plan). The model has no
// status field to flip to "cancelled" — DeletedAt on BaseModel is a plain
// *time.Time, not gorm.DeletedAt, so it isn't a soft-delete column GORM
// interprets automatically — so this is a real, hard delete of the row,
// same as DeleteShopByAdmin. It's also the pre-existing convention: a shop
// with no ShopSubscription row already falls back to the unsubscribed/Basic
// tier everywhere plan limits are checked (see shopSubscription() in
// services/planLimits.go, whose doc comment already calls a missing row
// "a cancelled subscription") and CheckShopLimit's MAX(plans.max_shops)
// query naturally excludes it too — so cancelling here immediately and
// correctly demotes the shop everywhere those checks run, with nothing
// left over that still reads as "active".
func CancelSubscription(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var sub models.ShopSubscription
	if err := initializers.DB.Preload("Plan").First(&sub, "shop_id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Subscription not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if err := initializers.DB.Delete(&sub).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to cancel subscription", err)
		return
	}

	utils.LogAudit(c, "subscription.cancel", "ShopSubscription", &sub.ID, gin.H{
		"shopId": shopID, "planId": sub.PlanID, "planName": sub.Plan.Name,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Subscription cancelled"})
}
