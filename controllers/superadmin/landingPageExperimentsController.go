package superadmin

import (
	"errors"
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

// ExperimentWithShop is a landing-page A/B test plus the tenant display
// fields and set count the cross-tenant list needs — same "batch-fetch then
// map" join used by ListLandingPages/ListOffers/ListCoupons, since the list
// only needs shop name/slug and a set count, not the per-set views/orders
// standings the merchant dashboard computes.
//
// landing_page_experiment_assignments (which visitor got which variant) is a
// detail/analytics table, deliberately out of scope here — same "skip the
// child table" call as offer_events/coupon_products/landing_page_images
// before it; this is an experiment-level list, not an assignment dashboard.
type ExperimentWithShop struct {
	models.LandingPageExperiment
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
	SetCount int64  `json:"setCount"`
}

// ListExperiments is a read-only cross-tenant view of every landing-page A/B
// test on the platform, so support/ops can spot a runaway or abandoned test
// without needing per-shop access. Never mutates — same reasoning as
// ListLandingPages/ListOffers/ListCoupons.
func ListExperiments(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	shopID := strings.TrimSpace(c.Query("shopId"))
	status := strings.TrimSpace(c.Query("status")) // running|decided|stopped

	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.LandingPageExperiment{}).Where("deleted_at IS NULL")

	if search != "" {
		db = db.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if shopID != "" {
		db = db.Where("shop_id = ?", shopID)
	}
	switch status {
	case models.ExperimentStatusRunning, models.ExperimentStatusDecided, models.ExperimentStatusStopped:
		db = db.Where("status = ?", status)
	}

	order := resolveSort(c, map[string]string{
		"name":       "name",
		"status":     "status",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var experiments []models.LandingPageExperiment
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&experiments).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch experiments", err)
		return
	}

	shopIDs := make([]string, 0, len(experiments))
	experimentIDs := make([]uuid.UUID, 0, len(experiments))
	seen := map[string]bool{}
	for _, e := range experiments {
		idStr := e.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
		experimentIDs = append(experimentIDs, e.ID)
	}

	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		var shops []models.Shop
		initializers.DB.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, s := range shops {
			shopByID[s.ID.String()] = s
		}
	}

	setCountByExperiment := map[string]int64{}
	if len(experimentIDs) > 0 {
		var rows []struct {
			ExperimentID uuid.UUID
			Count        int64
		}
		initializers.DB.Model(&models.LandingPage{}).
			Select("experiment_id, COUNT(*) as count").
			Where("experiment_id IN ?", experimentIDs).
			Group("experiment_id").
			Scan(&rows)
		for _, r := range rows {
			setCountByExperiment[r.ExperimentID.String()] = r.Count
		}
	}

	result := make([]ExperimentWithShop, 0, len(experiments))
	for _, e := range experiments {
		shop := shopByID[e.ShopID.String()]
		result = append(result, ExperimentWithShop{
			LandingPageExperiment: e,
			ShopName:              shop.Name,
			ShopSlug:              shop.Slug,
			SetCount:              setCountByExperiment[e.ID.String()],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// ForceStopExperiment halts a live A/B test platform-side. Kept out of the
// supportAccessible group (unlike the read-only list above) — stopping
// changes which landing page every subsequent visitor actually sees (a
// stopped experiment's public assign endpoint starts rejecting new
// visitors — see AssignExperimentVariant's ErrExperimentStopped path), the
// same bar the offers/coupons/landing-pages force-actions are held to, so it
// stays super_admin-only.
func ForceStopExperiment(c *gin.Context) {
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid experiment ID"})
		return
	}

	var experiment models.LandingPageExperiment
	if err := initializers.DB.Where("deleted_at IS NULL").First(&experiment, "id = ?", experimentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Experiment not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	previousStatus := experiment.Status

	if err := services.ForceStopExperiment(&experiment); err != nil {
		if errors.Is(err, services.ErrExperimentNotRunning) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Only a running experiment can be force-stopped"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Failed to stop experiment", err)
		return
	}

	utils.LogAudit(c, "landing_page_experiment.force_stop", "LandingPageExperiment", &experiment.ID, gin.H{
		"name":           experiment.Name,
		"shopId":         experiment.ShopID,
		"previousStatus": previousStatus,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Experiment stopped", "data": experiment})
}
