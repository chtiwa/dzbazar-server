package controllers

import (
	"errors"
	"net/http"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// findShopMember looks up the ShopMember row for :shopId/:id, the join every
// permission endpoint needs before it can read or write overrides.
func findShopMember(c *gin.Context) (models.ShopMember, bool) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID format"})
		return models.ShopMember{}, false
	}
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid user ID format"})
		return models.ShopMember{}, false
	}

	var member models.ShopMember
	err = initializers.DB.Where("shop_id = ? AND user_id = ?", shopID, userID).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "User not found in this shop"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while retrieving membership"})
		}
		return models.ShopMember{}, false
	}
	return member, true
}

func GetMemberPermissions(c *gin.Context) {
	member, ok := findShopMember(c)
	if !ok {
		return
	}

	overrides, err := services.ListOverrides(member.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while retrieving permissions"})
		return
	}
	actions, err := services.ListActionsForRole(member.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while retrieving actions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"overrides": overrides, "actions": actions},
	})
}

func SetMemberPermission(c *gin.Context) {
	member, ok := findShopMember(c)
	if !ok {
		return
	}
	if member.Role == "owner" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Cannot override permissions for an owner", "code": "OWNER_OVERRIDE"})
		return
	}

	var body struct {
		Allow bool `json:"allow"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body"})
		return
	}

	action := c.Param("action")
	if err := services.SetOverride(member.ID, action, body.Allow); err != nil {
		if errors.Is(err, services.ErrInvalidAction) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid action", "code": "INVALID_ACTION"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to set permission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Permission override set"})
}

func DeleteMemberPermission(c *gin.Context) {
	member, ok := findShopMember(c)
	if !ok {
		return
	}

	action := c.Param("action")
	if err := services.ClearOverride(member.ID, action); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to clear permission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Permission override cleared"})
}
