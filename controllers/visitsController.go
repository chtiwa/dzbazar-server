package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RecordVisitInput struct {
	VisitorID string `json:"visitorId" binding:"required"`
}

// RecordVisit — public storefront beacon. Upserts one row per (shop, today,
// visitor); duplicates within the day are no-ops via ON CONFLICT DO NOTHING.
func RecordVisit(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body RecordVisitInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "visitorId required"})
		return
	}

	visitorID := strings.TrimSpace(body.VisitorID)
	if visitorID == "" || len(visitorID) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid visitorId"})
		return
	}

	// CURRENT_DATE = server-local date. ON CONFLICT matches the composite unique
	// index by column set regardless of GORM's generated index name.
	if err := initializers.DB.Exec(`
		INSERT INTO shop_visits (id, shop_id, day, visitor_id, created_at)
		VALUES (uuid_generate_v4(), ?, CURRENT_DATE, ?, NOW())
		ON CONFLICT (shop_id, day, visitor_id) DO NOTHING
	`, shopID, visitorID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to record visit"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

type RecordPageVisitInput struct {
	VisitorID string `json:"visitorId" binding:"required"`
	PageType  string `json:"pageType" binding:"required"`
	EntityID  string `json:"entityId" binding:"required"`
}

// RecordPageVisit — public storefront beacon for a product page or landing
// page view. Same upsert-dedup shape as RecordVisit, one dimension wider
// (page_type + entity_id) so product and landing-page views are counted
// separately from each other and from the shop-level beacon.
func RecordPageVisit(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body RecordPageVisitInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "visitorId, pageType and entityId required"})
		return
	}

	visitorID := strings.TrimSpace(body.VisitorID)
	if visitorID == "" || len(visitorID) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid visitorId"})
		return
	}

	if body.PageType != "product" && body.PageType != "landing_page" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid pageType"})
		return
	}

	entityID, err := uuid.Parse(body.EntityID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid entityId"})
		return
	}

	if err := initializers.DB.Exec(`
		INSERT INTO page_visits (id, shop_id, page_type, entity_id, day, visitor_id, created_at)
		VALUES (uuid_generate_v4(), ?, ?, ?, CURRENT_DATE, ?, NOW())
		ON CONFLICT (shop_id, page_type, entity_id, day, visitor_id) DO NOTHING
	`, shopID, body.PageType, entityID, visitorID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to record visit"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// GetVisits — protected. Unique daily visitors for the last N days (default 30).
// Reuses TimeCount from dashboardController.
func GetVisits(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	days := 30
	if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
		days = d
	}

	daily := []TimeCount{}
	if err := initializers.DB.Table("shop_visits").
		Where("shop_id = ? AND day > CURRENT_DATE - ?::int", shopID, days).
		Select("day::text AS label, COUNT(*) AS count").
		Group("day").Order("day ASC").Scan(&daily).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch visits", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": daily})
}
