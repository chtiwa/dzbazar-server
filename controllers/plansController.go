package controllers

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

// ---------------------------------------------------------------------------
// Plans (admin-managed global catalog)
// ---------------------------------------------------------------------------

type CreatePlanInput struct {
	Name     string  `json:"name" binding:"required"`
	Price    float64 `json:"price"`
	IsActive *bool   `json:"isActive"`

	// Caps (omit or send 0 to use default: -1 = unlimited, except MaxShops default 1)
	MaxShops        *int `json:"maxShops"`
	MaxProducts     *int `json:"maxProducts"`
	MaxOrders       *int `json:"maxOrders"`
	MaxLandingPages *int `json:"maxLandingPages"`
	MaxUsers        *int `json:"maxUsers"`
	MaxFacebookPixels *int `json:"maxFacebookPixels"`
	MaxTikTokPixels   *int `json:"maxTikTokPixels"`

	// Feature flags
	HasConfirmationOrders *bool `json:"hasConfirmationOrders"`
	HasAbandonedOrders    *bool `json:"hasAbandonedOrders"`
	HasOrderTracking      *bool `json:"hasOrderTracking"`
	HasClientTracking     *bool `json:"hasClientTracking"`
}

type UpdatePlanInput struct {
	Name     *string  `json:"name"`
	Price    *float64 `json:"price"`
	IsActive *bool    `json:"isActive"`

	MaxShops        *int `json:"maxShops"`
	MaxProducts     *int `json:"maxProducts"`
	MaxOrders       *int `json:"maxOrders"`
	MaxLandingPages *int `json:"maxLandingPages"`
	MaxUsers        *int `json:"maxUsers"`
	MaxFacebookPixels *int `json:"maxFacebookPixels"`
	MaxTikTokPixels   *int `json:"maxTikTokPixels"`

	HasConfirmationOrders *bool `json:"hasConfirmationOrders"`
	HasAbandonedOrders    *bool `json:"hasAbandonedOrders"`
	HasOrderTracking      *bool `json:"hasOrderTracking"`
	HasClientTracking     *bool `json:"hasClientTracking"`
}

func GetPlans(c *gin.Context) {
	var plans []models.Plan
	if err := initializers.DB.Where("is_active = true").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch plans", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": plans})
}

func CreatePlan(c *gin.Context) {
	var body CreatePlanInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	derefInt := func(p *int, def int) int {
		if p != nil {
			return *p
		}
		return def
	}
	derefBool := func(p *bool, def bool) bool {
		if p != nil {
			return *p
		}
		return def
	}

	plan := models.Plan{
		Name:     strings.TrimSpace(body.Name),
		Price:    body.Price,
		IsActive: derefBool(body.IsActive, true),

		MaxShops:          derefInt(body.MaxShops, 1),
		MaxProducts:       derefInt(body.MaxProducts, -1),
		MaxOrders:         derefInt(body.MaxOrders, -1),
		MaxLandingPages:   derefInt(body.MaxLandingPages, -1),
		MaxUsers:          derefInt(body.MaxUsers, -1),
		MaxFacebookPixels: derefInt(body.MaxFacebookPixels, 1),
		MaxTikTokPixels:   derefInt(body.MaxTikTokPixels, 1),

		HasConfirmationOrders: derefBool(body.HasConfirmationOrders, true),
		HasAbandonedOrders:    derefBool(body.HasAbandonedOrders, false),
		HasOrderTracking:      derefBool(body.HasOrderTracking, false),
		HasClientTracking:     derefBool(body.HasClientTracking, false),
	}

	if err := initializers.DB.Create(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create plan", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "plan.create", "Plan", &plan.ID, gin.H{"name": plan.Name, "price": plan.Price})

	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Plan created successfully", "data": plan})
}

func UpdatePlan(c *gin.Context) {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid plan ID"})
		return
	}

	var body UpdatePlanInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var plan models.Plan
	if err := initializers.DB.First(&plan, "id = ?", planID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	updates := map[string]any{}
	if body.Name != nil {
		updates["name"] = strings.TrimSpace(*body.Name)
	}
	if body.Price != nil {
		updates["price"] = *body.Price
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}
	if body.MaxShops != nil {
		updates["max_shops"] = *body.MaxShops
	}
	if body.MaxProducts != nil {
		updates["max_products"] = *body.MaxProducts
	}
	if body.MaxOrders != nil {
		updates["max_orders"] = *body.MaxOrders
	}
	if body.MaxLandingPages != nil {
		updates["max_landing_pages"] = *body.MaxLandingPages
	}
	if body.MaxUsers != nil {
		updates["max_users"] = *body.MaxUsers
	}
	if body.MaxFacebookPixels != nil {
		updates["max_facebook_pixels"] = *body.MaxFacebookPixels
	}
	if body.MaxTikTokPixels != nil {
		updates["max_tik_tok_pixels"] = *body.MaxTikTokPixels
	}
	if body.HasConfirmationOrders != nil {
		updates["has_confirmation_orders"] = *body.HasConfirmationOrders
	}
	if body.HasAbandonedOrders != nil {
		updates["has_abandoned_orders"] = *body.HasAbandonedOrders
	}
	if body.HasOrderTracking != nil {
		updates["has_order_tracking"] = *body.HasOrderTracking
	}
	if body.HasClientTracking != nil {
		updates["has_client_tracking"] = *body.HasClientTracking
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "No fields provided for update"})
		return
	}

	if err := initializers.DB.Model(&plan).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update plan", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "plan.update", "Plan", &plan.ID, updates)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Plan updated successfully", "data": plan})
}

func DeletePlan(c *gin.Context) {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid plan ID"})
		return
	}

	var plan models.Plan
	if err := initializers.DB.First(&plan, "id = ?", planID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Delete(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete plan", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "plan.delete", "Plan", &plan.ID, gin.H{"name": plan.Name})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Plan deleted successfully"})
}

// ---------------------------------------------------------------------------
// Shop subscriptions
// ---------------------------------------------------------------------------

type SubscribeInput struct {
	PlanID    string     `json:"planId" binding:"required"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func GetShopSubscription(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var sub models.ShopSubscription
	if err := initializers.DB.
		Preload("Plan").
		Where("shop_id = ?", shopID).
		First(&sub).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch subscription", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": sub})
}

func SubscribeShopToPlan(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body SubscribeInput
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

	var sub models.ShopSubscription
	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		// Upsert — one subscription per shop
		existing := tx.Where("shop_id = ?", shopID).First(&sub)
		if existing.Error != nil && existing.Error != gorm.ErrRecordNotFound {
			return existing.Error
		}

		sub.ShopID = shopID
		sub.PlanID = planID
		sub.StartedAt = time.Now()
		sub.ExpiresAt = body.ExpiresAt

		if existing.Error == gorm.ErrRecordNotFound {
			return tx.Create(&sub).Error
		}
		return tx.Model(&sub).Updates(map[string]any{
			"plan_id":    planID,
			"started_at": sub.StartedAt,
			"expires_at": body.ExpiresAt,
		}).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update subscription", "error": err.Error()})
		return
	}

	initializers.DB.Preload("Plan").First(&sub, "shop_id = ?", shopID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Subscription updated successfully", "data": sub})
}

func CancelShopSubscription(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var sub models.ShopSubscription
	if err := initializers.DB.Where("shop_id = ?", shopID).First(&sub).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "No active subscription found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Delete(&sub).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to cancel subscription", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Subscription cancelled successfully"})
}
