package superadmin

import (
	"net/http"
	"strings"

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

	var totalRows int64
	db.Count(&totalRows)

	var logs []models.AuditLog
	if err := db.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch audit logs", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       logs,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}
