package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
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
