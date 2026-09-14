package controllers

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func GetMyNotifications(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	page := 1
	perPage := 10

	if p := c.Query("page"); p != "" {
		if parsed, parseErr := strconv.Atoi(p); parseErr == nil && parsed > 0 {
			page = parsed
		}
	}
	if pp := c.Query("perPage"); pp != "" {
		if parsed, parseErr := strconv.Atoi(pp); parseErr == nil && parsed > 0 {
			perPage = parsed
		}
	}

	shopID := c.GetString("activeShopID")
	baseQuery := initializers.DB.Model(&models.Notification{}).Where("recipient_user_id = ? AND shop_id = ?", user.ID, shopID)

	var totalRows int64
	if err := baseQuery.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to count notifications"})
		return
	}

	totalPages := int(math.Ceil(float64(totalRows) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage

	var notifications []models.Notification
	if err := initializers.DB.
		Preload("Shop", func(db *gorm.DB) *gorm.DB { return db.Select("id", "name") }).
		Where("recipient_user_id = ? AND shop_id = ?", user.ID, shopID).
		Order("created_at DESC").
		Limit(perPage).Offset(offset).
		Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch notifications"})
		return
	}

	pagination := utils.GetPaginationData(page, totalPages, "/notifications")

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Notifications retrieved successfully",
		"data":       notifications,
		"pagination": pagination,
	})
}

func MarkNotificationRead(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid notification ID"})
		return
	}

	shopID := c.GetString("activeShopID")
	now := time.Now()
	if err := initializers.DB.Model(&models.Notification{}).
		Where("id = ? AND recipient_user_id = ? AND shop_id = ?", id, user.ID, shopID).
		Update("read_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to mark notification as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Notification marked as read"})
}

func MarkAllNotificationsRead(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	shopID := c.GetString("activeShopID")
	now := time.Now()
	if err := initializers.DB.Model(&models.Notification{}).
		Where("recipient_user_id = ? AND shop_id = ? AND read_at IS NULL", user.ID, shopID).
		Update("read_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to mark notifications as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "All notifications marked as read"})
}

func GetUnreadNotificationCount(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	shopID := c.GetString("activeShopID")
	var count int64
	if err := initializers.DB.Model(&models.Notification{}).
		Where("recipient_user_id = ? AND shop_id = ? AND read_at IS NULL", user.ID, shopID).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to count unread notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Unread count retrieved successfully", "data": gin.H{"count": count}})
}
