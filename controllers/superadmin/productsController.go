package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProductWithShop struct {
	models.Product
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
}

// ListProducts is a read-only cross-tenant product overview. Bulk editing
// another tenant's catalog from outside is a correctness risk, not a feature,
// so this endpoint never mutates. The one narrow exception is
// ForceHideProduct below — a single platform-owned moderation flag, not
// general edit access; see that function's doc comment.
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

	order := resolveSort(c, map[string]string{
		"title":      "title",
		"price":      "price",
		"active":     "active",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var products []models.Product
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Preload("Images").
		Find(&products).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch products", err)
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

type ToggleProductHiddenInput struct {
	Hidden bool `json:"hidden"`
}

// ForceHideProduct toggles a single product's platform-owned moderation
// flag (Product.HiddenByPlatformAt) so a super admin can take down one
// fraudulent listing without suspending the whole shop or touching any
// other field on the product — see ListProducts's doc comment above. Every
// customer-facing storefront read (controllers/productsController.go's
// GetActiveProductsBySlug/GetProductsBySearchBySlug/IndexProductBySlug, plus
// IndexLandingPage) excludes hidden products, so this is what actually
// takes a listing off the storefront. Reversible via the same endpoint
// (hidden: false), same bar as offers/coupons/landing-pages force-actions,
// so it stays super_admin-only.
func ForceHideProduct(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID"})
		return
	}

	var body ToggleProductHiddenInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	var product models.Product
	if err := initializers.DB.First(&product, "id = ?", productID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Product not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	action := "product.force_hide"
	if body.Hidden {
		if err := services.ForceHideProduct(&product); err != nil {
			RespondError(c, http.StatusInternalServerError, "Failed to hide product", err)
			return
		}
	} else {
		action = "product.unhide"
		if err := services.UnhideProduct(&product); err != nil {
			RespondError(c, http.StatusInternalServerError, "Failed to unhide product", err)
			return
		}
	}

	// Product + landing-page reads are cached (10min TTL, see
	// productsController.go) — bust the same keys a merchant-initiated
	// product edit does, or the storefront would keep serving the old
	// visibility for up to the TTL.
	controllers.InvalidateProductCaches(product.ID, product.ShopID)

	utils.LogAudit(c, action, "Product", &product.ID, gin.H{
		"title":  product.Title,
		"shopId": product.ShopID,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Product visibility updated", "data": product})
}
