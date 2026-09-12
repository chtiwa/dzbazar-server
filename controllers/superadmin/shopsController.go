package superadmin

import (
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SuperAdminPixel is the pixel projection exposed to super-admin. AccessToken
// never appears here — omitempty on the model alone isn't a safe guard for a
// populated secret, so this is an explicit field-by-field copy instead.
type SuperAdminPixel struct {
	ID             uuid.UUID `json:"id"`
	Platform       string    `json:"platform"`
	Title          string    `json:"title"`
	PixelID        string    `json:"pixelId"`
	HasAccessToken bool      `json:"hasAccessToken"`
	IsActive       bool      `json:"isActive"`
	CreatedAt      time.Time `json:"created_at"`
}

func toSuperAdminPixels(pixels []models.Pixel) []SuperAdminPixel {
	out := make([]SuperAdminPixel, 0, len(pixels))
	for _, p := range pixels {
		out = append(out, SuperAdminPixel{
			ID:             p.ID,
			Platform:       p.Platform,
			Title:          p.Title,
			PixelID:        p.PixelID,
			HasAccessToken: p.HasAccessToken,
			IsActive:       p.IsActive,
			CreatedAt:      p.CreatedAt,
		})
	}
	return out
}

func ListShops(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	statusFilter := c.Query("status") // "active" | "suspended" | ""
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Shop{}).Preload("Owner").Preload("LogoImage")

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(slug) LIKE ?", like, like)
	}
	if statusFilter == "active" {
		db = db.Where("is_active = true")
	} else if statusFilter == "suspended" {
		db = db.Where("is_active = false")
	}

	order := resolveSort(c, map[string]string{
		"name":       "name",
		"status":     "is_active",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var shops []models.Shop
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&shops).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch shops", err)
		return
	}

	for i := range shops {
		sanitize(&shops[i].Owner)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       shops,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

func GetShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var shop models.Shop
	if err := initializers.DB.
		Preload("Owner").
		Preload("LogoImage").
		Preload("Members").
		Preload("Members.User").
		First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	sanitize(&shop.Owner)
	for i := range shop.Members {
		sanitize(&shop.Members[i].User)
	}

	var subscription models.ShopSubscription
	subErr := initializers.DB.Preload("Plan").Where("shop_id = ?", shopID).First(&subscription).Error

	var productCount, orderCount int64
	initializers.DB.Model(&models.Product{}).Where("shop_id = ?", shopID).Count(&productCount)
	initializers.DB.Model(&models.Order{}).Where("shop_id = ?", shopID).Count(&orderCount)

	var pixels []models.Pixel
	initializers.DB.Where("shop_id = ?", shopID).Order("created_at DESC").Find(&pixels)

	// AI description usage since the current subscription period started —
	// same window services.CheckCreditBudget enforces, so an operator
	// sees the number the merchant's quota is actually measured against.
	newAiUsageQuery := func() *gorm.DB {
		q := initializers.DB.Model(&models.AiDescriptionUsage{}).Where("shop_id = ?", shopID)
		if subErr == nil && !subscription.StartedAt.IsZero() {
			q = q.Where("created_at >= ?", subscription.StartedAt)
		}
		return q
	}
	var aiCallsThisMonth, aiTokensThisMonth int64
	newAiUsageQuery().Count(&aiCallsThisMonth)
	newAiUsageQuery().Select("COALESCE(SUM(total_tokens), 0)").Scan(&aiTokensThisMonth)

	// Same window/reasoning as above, for services.CheckCreditBudget's AI
	// image tool path — image generation costs far more per call than a text
	// completion, so it's worth an operator seeing this count separately.
	newAiImageUsageQuery := func() *gorm.DB {
		q := initializers.DB.Model(&models.AiImageToolUsage{}).Where("shop_id = ?", shopID)
		if subErr == nil && !subscription.StartedAt.IsZero() {
			q = q.Where("created_at >= ?", subscription.StartedAt)
		}
		return q
	}
	var aiImagesThisMonth int64
	newAiImageUsageQuery().Count(&aiImagesThisMonth)

	// The merchant-facing figure, so an operator sees the same number the
	// shop's quota is measured against — not just the raw call counts above.
	// Images are billed per-model now, so this reuses the same accounting
	// CheckCreditBudget enforces rather than a flat per-image rate.
	creditsSummary, _ := services.GetCreditsSummary(shopID)
	creditsUsed := creditsSummary.Used

	resp := gin.H{
		"shop":              shop,
		"productCount":      productCount,
		"orderCount":        orderCount,
		"pixels":            toSuperAdminPixels(pixels),
		"aiCallsThisMonth":  aiCallsThisMonth,
		"aiTokensThisMonth": aiTokensThisMonth,
		"aiImagesThisMonth": aiImagesThisMonth,
		"creditsUsed":       creditsUsed,
	}
	if subErr == nil {
		resp["subscription"] = subscription
	} else {
		resp["subscription"] = nil
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

type UpdateShopStatusInput struct {
	IsActive bool    `json:"isActive"`
	Reason   *string `json:"reason"`
}

// UpdateShopStatus suspends or reactivates a shop's storefront platform-wide.
func UpdateShopStatus(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body UpdateShopStatusInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	var shop models.Shop
	if err := initializers.DB.First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if err := services.UpdateShopStatus(&shop, body.IsActive, body.Reason); err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to update shop status", err)
		return
	}

	action := "shop.suspend"
	if body.IsActive {
		action = "shop.activate"
	}
	utils.LogAudit(c, action, "Shop", &shop.ID, gin.H{
		"name":   shop.Name,
		"slug":   shop.Slug,
		"reason": shop.SuspendReason,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Shop status updated", "data": shop})
}

// DeleteShopByAdmin permanently deletes a shop and everything that cascades
// from it (members, products, orders, clients, pixels). Irreversible.
func DeleteShopByAdmin(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var shop models.Shop
	if err := initializers.DB.First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	snapshot := gin.H{"name": shop.Name, "slug": shop.Slug, "ownerId": shop.OwnerID}

	if err := initializers.DB.Delete(&shop).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to delete shop", err)
		return
	}

	utils.LogAudit(c, "shop.delete", "Shop", &shop.ID, snapshot)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Shop deleted permanently"})
}

type UpdateShopPixelStatusInput struct {
	IsActive bool `json:"isActive"`
}

// UpdateShopPixelStatus lets a super admin disable a misconfigured pixel
// platform-wide (bad PixelID, leaked token, etc.) without touching its
// config — same bar as the offers/coupons/landing-pages force-disable
// actions in superAdminRoutes.go, since it changes what a merchant's live
// tracking setup actually does.
func UpdateShopPixelStatus(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	pixelID, err := uuid.Parse(c.Param("pixelId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid pixel ID"})
		return
	}

	var body UpdateShopPixelStatusInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	var pixel models.Pixel
	if err := initializers.DB.Where("id = ? AND shop_id = ?", pixelID, shopID).First(&pixel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Pixel not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if err := initializers.DB.Model(&pixel).Update("is_active", body.IsActive).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to update pixel status", err)
		return
	}
	pixel.IsActive = body.IsActive

	action := "shop.pixel.disable"
	if body.IsActive {
		action = "shop.pixel.enable"
	}
	utils.LogAudit(c, action, "Pixel", &pixel.ID, gin.H{
		"shopId":   shopID,
		"platform": pixel.Platform,
		"title":    pixel.Title,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Pixel status updated", "data": toSuperAdminPixels([]models.Pixel{pixel})[0]})
}
