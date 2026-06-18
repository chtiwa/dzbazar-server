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
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.ShopSubscription{}).Preload("Plan")

	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}

	var totalRows int64
	db.Count(&totalRows)

	var subs []models.ShopSubscription
	if err := db.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&subs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch subscriptions", "error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update subscription", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "subscription.set", "ShopSubscription", &sub.ID, gin.H{"shopId": shopID, "planId": planID, "expiresAt": expires})

	initializers.DB.Preload("Plan").First(&sub, "shop_id = ?", shopID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Subscription updated", "data": sub})
}
