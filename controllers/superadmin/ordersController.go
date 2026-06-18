package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
)

type OrderWithShop struct {
	models.Order
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
}

// ListOrders is a read-only cross-tenant order overview, filterable by shop
// and status. Never mutates another tenant's orders.
func ListOrders(c *gin.Context) {
	shopID := strings.TrimSpace(c.Query("shopId"))
	status := strings.TrimSpace(c.Query("status"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Order{})

	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}

	var totalRows int64
	db.Count(&totalRows)

	var orders []models.Order
	if err := db.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Preload("Client").
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch orders", "error": err.Error()})
		return
	}

	shopIDs := make([]string, 0, len(orders))
	seen := map[string]bool{}
	for _, o := range orders {
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

	result := make([]OrderWithShop, 0, len(orders))
	for _, o := range orders {
		shop := shopByID[o.ShopID.String()]
		result = append(result, OrderWithShop{Order: o, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}
