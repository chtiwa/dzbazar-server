package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CouponWithShop is a coupon plus the tenant display fields and redemption
// count the cross-tenant list needs — same "batch-fetch then map" join used
// by ListOffers, since Coupon only holds ShopID, not a Shop association.
type CouponWithShop struct {
	models.Coupon
	ShopName   string `json:"shopName"`
	ShopSlug   string `json:"shopSlug"`
	UsageCount int64  `json:"usageCount"`
}

type couponUsageRow struct {
	CouponID uuid.UUID `gorm:"column:coupon_id"`
	Count    int64     `gorm:"column:count"`
}

// ListCoupons is a read-only cross-tenant view of every discount code on the
// platform, so support/ops can spot an abusive or leaked coupon without
// needing per-shop access. Never mutates — same reasoning as ListOffers.
func ListCoupons(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	shopID := strings.TrimSpace(c.Query("shopId"))
	status := strings.TrimSpace(c.Query("status")) // active|inactive

	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Coupon{}).Where("deleted_at IS NULL")

	if search != "" {
		db = db.Where("LOWER(code) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}
	if status == "active" {
		db = db.Where("active = ?", true)
	} else if status == "inactive" {
		db = db.Where("active = ?", false)
	}

	order := resolveSort(c, map[string]string{
		"code":       "code",
		"percent":    "percent",
		"active":     "active",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var coupons []models.Coupon
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&coupons).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch coupons", err)
		return
	}

	shopIDs := make([]string, 0, len(coupons))
	couponIDs := make([]uuid.UUID, 0, len(coupons))
	seen := map[string]bool{}
	for _, cp := range coupons {
		idStr := cp.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
		couponIDs = append(couponIDs, cp.ID)
	}

	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		var shops []models.Shop
		initializers.DB.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, s := range shops {
			shopByID[s.ID.String()] = s
		}
	}

	// Usage = orders that redeemed this coupon (Order.CouponID). Counted in
	// one grouped query rather than N+1 per row — same batch shape as the
	// shop lookup above. No dedicated redemption-tracking table exists, so
	// this is the only usage signal available; good enough for a moderation
	// list, not a stats dashboard.
	usageByCoupon := map[string]int64{}
	if len(couponIDs) > 0 {
		var rows []couponUsageRow
		initializers.DB.Model(&models.Order{}).
			Select("coupon_id, COUNT(*) as count").
			Where("coupon_id IN ? AND deleted_at IS NULL", couponIDs).
			Group("coupon_id").
			Scan(&rows)
		for _, r := range rows {
			usageByCoupon[r.CouponID.String()] = r.Count
		}
	}

	result := make([]CouponWithShop, 0, len(coupons))
	for _, cp := range coupons {
		shop := shopByID[cp.ShopID.String()]
		result = append(result, CouponWithShop{
			Coupon:     cp,
			ShopName:   shop.Name,
			ShopSlug:   shop.Slug,
			UsageCount: usageByCoupon[cp.ID.String()],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// ForceDisableCoupon deactivates a live discount code platform-side. Kept
// out of the supportAccessible group (unlike the read-only list above) —
// this kills a coupon a merchant is actively running, the same bar the
// offers force-disable action is held to, so it stays super_admin-only.
func ForceDisableCoupon(c *gin.Context) {
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid coupon ID"})
		return
	}

	var coupon models.Coupon
	if err := initializers.DB.Where("deleted_at IS NULL").First(&coupon, "id = ?", couponID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Coupon not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	previousActive := coupon.Active

	if err := services.ForceDisableCoupon(&coupon); err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to disable coupon", err)
		return
	}

	utils.LogAudit(c, "coupon.force_disable", "Coupon", &coupon.ID, gin.H{
		"code":           coupon.Code,
		"shopId":         coupon.ShopID,
		"previousActive": previousActive,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Coupon disabled", "data": coupon})
}
