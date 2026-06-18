package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
)

type ProductWithShop struct {
	models.Product
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
}

// ListProducts is a read-only cross-tenant product overview. Bulk editing
// another tenant's catalog from outside is a correctness risk, not a feature,
// so this endpoint never mutates.
func ListProducts(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	shopID := strings.TrimSpace(c.Query("shopId"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Product{})

	if search != "" {
		db = db.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}

	var totalRows int64
	db.Count(&totalRows)

	var products []models.Product
	if err := db.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Preload("Images").
		Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch products", "error": err.Error()})
		return
	}

	shopIDs := make([]string, 0, len(products))
	seen := map[string]bool{}
	for _, p := range products {
		idStr := p.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
	}

	var shops []models.Shop
	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		initializers.DB.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, s := range shops {
			shopByID[s.ID.String()] = s
		}
	}

	result := make([]ProductWithShop, 0, len(products))
	for _, p := range products {
		shop := shopByID[p.ShopID.String()]
		result = append(result, ProductWithShop{Product: p, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}
