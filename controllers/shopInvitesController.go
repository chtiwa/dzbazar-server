package controllers

import (
	"errors"
	"net/http"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

// ListMyInvites returns the caller's pending shop invites.
func ListMyInvites(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	var invites []models.ShopInvite
	if err := initializers.DB.Preload("Shop").Where("user_id = ? AND expires_at > now()", user.ID).
		Order("created_at DESC").Find(&invites).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch invites", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": invites})
}

func loadMyInvite(c *gin.Context) (models.ShopInvite, bool) {
	user := c.MustGet("user").(models.User)
	var inv models.ShopInvite
	id, err := uuid.Parse(c.Param("inviteId"))
	if err == nil {
		err = initializers.DB.Where("id = ? AND user_id = ? AND expires_at > now()", id, user.ID).First(&inv).Error
	}
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Invite not found"})
		return inv, false
	}
	return inv, true
}

func AcceptInvite(c *gin.Context) {
	inv, ok := loadMyInvite(c)
	if !ok {
		return
	}
	if err := services.CheckUserLimit(inv.ShopID); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "This shop has reached its staff limit", "code": "PLAN_LIMIT_REACHED"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Failed to verify plan limits", err)
		return
	}
	tx := initializers.DB.Begin()
	m := models.ShopMember{ShopID: inv.ShopID, UserID: inv.UserID, Role: inv.Role}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error; err != nil {
		tx.Rollback()
		RespondError(c, http.StatusInternalServerError, "Failed to join shop", err)
		return
	}
	if err := tx.Unscoped().Delete(&inv).Error; err != nil {
		tx.Rollback()
		RespondError(c, http.StatusInternalServerError, "Failed to join shop", err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to join shop", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Invite accepted"})
}

func DeclineInvite(c *gin.Context) {
	inv, ok := loadMyInvite(c)
	if !ok {
		return
	}
	if err := initializers.DB.Unscoped().Delete(&inv).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to decline invite", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Invite declined"})
}

// ListShopInvites returns the shop's pending (unexpired) invites.
func ListShopInvites(c *gin.Context) {
	var invites []models.ShopInvite
	if err := initializers.DB.Preload("User").Where("shop_id = ? AND expires_at > now()", c.Param("shopId")).
		Order("created_at DESC").Find(&invites).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch invites", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": invites})
}

// RevokeShopInvite cancels a pending invite of this shop.
func RevokeShopInvite(c *gin.Context) {
	res := initializers.DB.Unscoped().Where("id = ? AND shop_id = ?", c.Param("inviteId"), c.Param("shopId")).
		Delete(&models.ShopInvite{})
	if res.Error != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to revoke invite", res.Error)
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Invite not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Invite revoked"})
}
