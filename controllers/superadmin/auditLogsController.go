package superadmin

import (
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
)

// ListAuditLogs is read-only by design — audit entries are never edited or
// deleted through the API.
func ListAuditLogs(c *gin.Context) {
	action := strings.TrimSpace(c.Query("action"))
	targetType := strings.TrimSpace(c.Query("targetType"))
	actorEmail := strings.TrimSpace(c.Query("actorEmail"))
	dateFrom := c.Query("dateFrom")
	dateTo := c.Query("dateTo")
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.AuditLog{})

	if action != "" {
		db = db.Where("action = ?", action)
	}
	if targetType != "" {
		db = db.Where("target_type = ?", targetType)
	}
	if actorEmail != "" {
		db = db.Where("LOWER(actor_email) LIKE ?", "%"+strings.ToLower(actorEmail)+"%")
	}
	// A start date with no end date means "that single day only".
	if dateTo == "" {
		dateTo = dateFrom
	}
	if dateFrom != "" {
		if parsed, err := time.Parse("2006-01-02", dateFrom); err == nil {
			db = db.Where("created_at >= ?", parsed)
		}
	}
	if dateTo != "" {
		if parsed, err := time.Parse("2006-01-02", dateTo); err == nil {
			db = db.Where("created_at < ?", parsed.AddDate(0, 0, 1))
		}
	}

	order := resolveSort(c, map[string]string{
		"actorEmail": "actor_email",
		"action":     "action",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var logs []models.AuditLog
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&logs).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch audit logs", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       logs,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}
