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

// LandingPageWithShop is a landing page plus the tenant display fields the
// cross-tenant list needs — same "batch-fetch then map" join used by
// ListOffers/ListCoupons, since the list only needs shop name/slug, not a
// full Shop preload.
type LandingPageWithShop struct {
	models.LandingPage
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
}

// ListLandingPages is a read-only cross-tenant view of every landing page on
// the platform, so support/ops can spot an abusive or deceptive page without
// needing per-shop access. Never mutates — same reasoning as ListOffers.
//
// landing_page_images is a detail/asset table, deliberately out of scope
// here — same "skip the child table" call as offer_events/coupon_products
// made earlier; this is a page-level list, not an image manager.
func ListLandingPages(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	shopID := strings.TrimSpace(c.Query("shopId"))
	status := strings.TrimSpace(c.Query("status")) // published|unpublished

	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.LandingPage{}).Where("deleted_at IS NULL")

	if search != "" {
		db = db.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}
	if status == "published" {
		db = db.Where("active = ?", true)
	} else if status == "unpublished" {
		db = db.Where("active = ?", false)
	}

	order := resolveSort(c, map[string]string{
		"title":      "title",
		"active":     "active",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var landingPages []models.LandingPage
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&landingPages).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch landing pages", err)
		return
	}

	shopIDs := make([]string, 0, len(landingPages))
	seen := map[string]bool{}
	for _, lp := range landingPages {
		idStr := lp.ShopID.String()
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

	result := make([]LandingPageWithShop, 0, len(landingPages))
	for _, lp := range landingPages {
		shop := shopByID[lp.ShopID.String()]
		result = append(result, LandingPageWithShop{LandingPage: lp, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// ForceUnpublishLandingPage takes a live landing page down platform-side.
// Kept out of the supportAccessible group (unlike the read-only list above)
// — this changes what a merchant's storefront actually shows, the same bar
// the offers/coupons force-disable actions are held to, so it stays
// super_admin-only.
func ForceUnpublishLandingPage(c *gin.Context) {
	landingPageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid landing page ID"})
		return
	}

	var landingPage models.LandingPage
	if err := initializers.DB.Where("deleted_at IS NULL").First(&landingPage, "id = ?", landingPageID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Landing page not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	previousActive := landingPage.Active

	if err := services.ForceUnpublishLandingPage(&landingPage); err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to unpublish landing page", err)
		return
	}

	services.InvalidateLandingPageCaches(landingPage.ShopID, landingPage.ID)

	utils.LogAudit(c, "landing_page.force_unpublish", "LandingPage", &landingPage.ID, gin.H{
		"title":          landingPage.Title,
		"shopId":         landingPage.ShopID,
		"previousActive": previousActive,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Landing page unpublished", "data": landingPage})
}
