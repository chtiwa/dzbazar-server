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

// ListPlanSwitchRequests is the queue a super admin works from — merchant
// plan switches (plansController.SubscribeShopToPlan) land here as "pending"
// instead of applying instantly.
func ListPlanSwitchRequests(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.PlanSwitchRequest{}).Preload("Plan")
	if status != "" {
		db = db.Where("status = ?", status)
	}

	order := resolveSort(c, map[string]string{
		"status":     "status",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var requests []models.PlanSwitchRequest
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&requests).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch plan switch requests", err)
		return
	}

	shopIDs := make([]string, 0, len(requests))
	seen := map[string]bool{}
	for _, r := range requests {
		idStr := r.ShopID.String()
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

	type requestWithShop struct {
		models.PlanSwitchRequest
		ShopName string `json:"shopName"`
		ShopSlug string `json:"shopSlug"`
	}

	result := make([]requestWithShop, 0, len(requests))
	for _, r := range requests {
		shop := shopByID[r.ShopID.String()]
		result = append(result, requestWithShop{PlanSwitchRequest: r, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// ApprovePlanSwitchRequest applies the requested plan to the shop's
// subscription — same upsert-by-shop-id logic as the old direct
// SetShopSubscription/SubscribeShopToPlan — then marks the request approved.
func ApprovePlanSwitchRequest(c *gin.Context) {
	requestID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request ID"})
		return
	}

	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	var request models.PlanSwitchRequest
	if err := initializers.DB.First(&request, "id = ?", requestID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Plan switch request not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}
	if request.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Request already reviewed"})
		return
	}

	start := time.Now()
	expires := start.AddDate(0, 0, 30)

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var sub models.ShopSubscription
		existing := tx.Where("shop_id = ?", request.ShopID).First(&sub)
		if existing.Error != nil && existing.Error != gorm.ErrRecordNotFound {
			return existing.Error
		}

		if existing.Error == gorm.ErrRecordNotFound {
			sub = models.ShopSubscription{ShopID: request.ShopID, PlanID: request.PlanID, StartedAt: start, ExpiresAt: &expires}
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&sub).Updates(map[string]any{
			"plan_id":                 request.PlanID,
			"started_at":              start,
			"expires_at":              expires,
			"expiry_reminder_sent_at": nil,
		}).Error; err != nil {
			return err
		}

		now := time.Now()
		return tx.Model(&request).Updates(map[string]any{
			"status":      "approved",
			"reviewed_by": actorUser.ID,
			"reviewed_at": now,
		}).Error
	})

	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to approve request", err)
		return
	}

	utils.LogAudit(c, "plan_switch_request.approve", "PlanSwitchRequest", &request.ID, gin.H{"shopId": request.ShopID, "planId": request.PlanID})

	initializers.DB.Preload("Plan").First(&request, "id = ?", requestID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Plan switch approved", "data": request})
}

// RejectPlanSwitchRequest leaves the shop's current subscription untouched.
func RejectPlanSwitchRequest(c *gin.Context) {
	requestID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request ID"})
		return
	}

	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	var request models.PlanSwitchRequest
	if err := initializers.DB.First(&request, "id = ?", requestID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Plan switch request not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}
	if request.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Request already reviewed"})
		return
	}

	now := time.Now()
	if err := initializers.DB.Model(&request).Updates(map[string]any{
		"status":      "rejected",
		"reviewed_by": actorUser.ID,
		"reviewed_at": now,
	}).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to reject request", err)
		return
	}

	utils.LogAudit(c, "plan_switch_request.reject", "PlanSwitchRequest", &request.ID, gin.H{"shopId": request.ShopID, "planId": request.PlanID})

	initializers.DB.Preload("Plan").First(&request, "id = ?", requestID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Plan switch rejected", "data": request})
}
