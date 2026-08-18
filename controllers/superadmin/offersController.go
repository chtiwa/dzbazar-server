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

// OfferWithShop is an offer plus the tenant display fields the cross-tenant
// list needs — same "batch-fetch then map" join used by ListProducts, since
// Offer only holds ShopID, not a Shop association.
type OfferWithShop struct {
	models.Offer
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
}

// ListOffers is a read-only cross-tenant view of every offer/promotion on
// the platform, so support/ops can spot an abusive or deceptive promotion
// without needing per-shop access. Never mutates — same reasoning as
// ListProducts.
func ListOffers(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	shopID := strings.TrimSpace(c.Query("shopId"))
	status := strings.TrimSpace(c.Query("status")) // draft|published|archived
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Offer{}).Where("deleted_at IS NULL")

	if search != "" {
		db = db.Where("LOWER(internal_name) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}

	order := resolveSort(c, map[string]string{
		"internalName": "internal_name",
		"status":       "status",
		"created_at":   "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var offers []models.Offer
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Preload("TriggerProduct").
		Find(&offers).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch offers", err)
		return
	}

	shopIDs := make([]string, 0, len(offers))
	seen := map[string]bool{}
	for _, o := range offers {
		idStr := o.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
	}

	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		var shops []models.Shop
		initializers.DB.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, s := range shops {
			shopByID[s.ID.String()] = s
		}
	}

	result := make([]OfferWithShop, 0, len(offers))
	for _, o := range offers {
		shop := shopByID[o.ShopID.String()]
		result = append(result, OfferWithShop{Offer: o, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// ForceDisableOffer archives a live promotion platform-side. Kept out of the
// supportAccessible group (unlike the read-only list above) — this changes
// what a merchant's storefront actually shows, the same bar the shop-suspend
// action is held to, so it stays super_admin-only.
func ForceDisableOffer(c *gin.Context) {
	offerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offer ID"})
		return
	}

	var offer models.Offer
	if err := initializers.DB.Where("deleted_at IS NULL").First(&offer, "id = ?", offerID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Offer not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	previousStatus := offer.Status

	if err := services.ForceDisableOffer(&offer); err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to disable offer", err)
		return
	}

	utils.LogAudit(c, "offer.force_disable", "Offer", &offer.ID, gin.H{
		"internalName":   offer.InternalName,
		"shopId":         offer.ShopID,
		"previousStatus": previousStatus,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Offer disabled", "data": offer})
}
