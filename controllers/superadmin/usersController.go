package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func sanitize(user *models.User) {
	user.Password = ""
	user.EmailOTP = ""
	user.EmailOTPExpiresAt = nil
}

func ListUsers(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	statusFilter := c.Query("status") // "active" | "suspended" | ""
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.User{}).Preload("Memberships").Preload("Memberships.Shop")

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		db = db.Where("LOWER(email) LIKE ? OR LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ?", like, like, like)
	}
	if statusFilter == "active" {
		db = db.Where("is_suspended = false")
	} else if statusFilter == "suspended" {
		db = db.Where("is_suspended = true")
	}

	var totalRows int64
	db.Count(&totalRows)

	var users []models.User
	if err := db.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch users", "error": err.Error()})
		return
	}

	for i := range users {
		sanitize(&users[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       users,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

func GetUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid user ID"})
		return
	}

	var user models.User
	if err := initializers.DB.
		Preload("Memberships").
		Preload("Memberships.Shop").
		First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	sanitize(&user)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": user})
}

type UpdateUserStatusInput struct {
	IsSuspended bool `json:"isSuspended"`
}

func UpdateUserStatus(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid user ID"})
		return
	}

	var body UpdateUserStatusInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var user models.User
	if err := initializers.DB.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if user.PlatformRole == "super_admin" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Cannot suspend another super admin from here"})
		return
	}

	if err := initializers.DB.Model(&user).Update("is_suspended", body.IsSuspended).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update user status", "error": err.Error()})
		return
	}

	action := "user.suspend"
	if !body.IsSuspended {
		action = "user.activate"
	}
	utils.LogAudit(c, action, "User", &user.ID, gin.H{"email": user.Email})

	user.IsSuspended = body.IsSuspended
	sanitize(&user)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "User status updated", "data": user})
}

// DeleteUserByAdmin permanently deletes a user. Blocked at the DB level if the
// user still owns a shop (Shop.OwnerID has no cascade) — the operator must
// transfer or delete that shop first; the DB error surfaces as-is.
func DeleteUserByAdmin(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid user ID"})
		return
	}

	var user models.User
	if err := initializers.DB.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if user.PlatformRole == "super_admin" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Cannot delete another super admin from here"})
		return
	}

	if err := initializers.DB.Delete(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Failed to delete user — they may still own a shop",
			"error":   err.Error(),
		})
		return
	}

	utils.LogAudit(c, "user.delete", "User", &user.ID, gin.H{"email": user.Email})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "User deleted permanently"})
}
