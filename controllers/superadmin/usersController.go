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

func sanitize(user *models.User) {
	user.Password = ""
	user.EmailOTP = ""
	user.EmailOTPExpiresAt = nil
}

func ListUsers(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	statusFilter := c.Query("status") // "active" | "suspended" | ""
	shopFilter := strings.TrimSpace(c.Query("shopId"))
	platformRoleFilter := c.Query("platformRole") // "none" | "support" | "super_admin" | ""
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
	if shopFilter != "" {
		if _, err := uuid.Parse(shopFilter); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
			return
		}
		// Membership is preloaded separately above; this is a plain filter on
		// which users to select, not a join, so it can't duplicate rows.
		db = db.Where("id IN (SELECT user_id FROM shop_members WHERE shop_id = ?)", shopFilter)
	}
	switch platformRoleFilter {
	case "none":
		db = db.Where("platform_role = ''")
	case "support", "super_admin":
		db = db.Where("platform_role = ?", platformRoleFilter)
	}

	order := resolveSort(c, map[string]string{
		"email":        "email",
		"status":       "is_suspended",
		"platformRole": "platform_role",
		"created_at":   "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var users []models.User
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&users).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch users", err)
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
		RespondError(c, http.StatusInternalServerError, "Database error", err)
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
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	var user models.User
	if err := initializers.DB.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "User not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if user.PlatformRole == "super_admin" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Cannot suspend another super admin from here"})
		return
	}

	if err := initializers.DB.Model(&user).Update("is_suspended", body.IsSuspended).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to update user status", err)
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

type SetPlatformRoleInput struct {
	// PlatformRole is validated against models.User.PlatformRole's documented
	// values by services.SetPlatformRole — "" | "support" | "super_admin".
	PlatformRole string `json:"platformRole"`
}

// SetPlatformRole grants or revokes a user's platform-wide role — the field
// that gates access to the entire super-admin panel (see
// middleware.RequirePlatformRole). super_admin-only, audit-logged with the
// old → new role.
func SetPlatformRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid user ID"})
		return
	}

	var body SetPlatformRoleInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	actorIf, _ := c.Get("user")
	actingUser, ok := actorIf.(models.User)
	if !ok {
		RespondError(c, http.StatusInternalServerError, "Internal server error: invalid user format", nil)
		return
	}

	target, previousRole, err := services.SetPlatformRole(actingUser, userID, body.PlatformRole)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidPlatformRole):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid platform role"})
		case errors.Is(err, services.ErrSelfDemotion):
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "You cannot remove your own super admin role"})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "User not found"})
		default:
			RespondError(c, http.StatusInternalServerError, "Failed to update platform role", err)
		}
		return
	}

	utils.LogAudit(c, "user.platform_role_change", "User", &target.ID, gin.H{
		"email":        target.Email,
		"previousRole": previousRole,
		"newRole":      target.PlatformRole,
	})

	sanitize(&target)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Platform role updated", "data": target})
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
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if user.PlatformRole == "super_admin" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Cannot delete another super admin from here"})
		return
	}

	if err := initializers.DB.Delete(&user).Error; err != nil {
		RespondError(c, http.StatusConflict, "Failed to delete user — they may still own a shop", err)
		return
	}

	utils.LogAudit(c, "user.delete", "User", &user.ID, gin.H{"email": user.Email})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "User deleted permanently"})
}
