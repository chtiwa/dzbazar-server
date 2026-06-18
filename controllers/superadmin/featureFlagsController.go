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

func ListFeatureFlags(c *gin.Context) {
	var flags []models.FeatureFlag
	if err := initializers.DB.Order("key ASC").Find(&flags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch feature flags", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": flags})
}

type CreateFeatureFlagInput struct {
	Key         string `json:"key" binding:"required"`
	Label       string `json:"label" binding:"required"`
	Description string `json:"description"`
	IsEnabled   bool   `json:"isEnabled"`
}

func CreateFeatureFlag(c *gin.Context) {
	var body CreateFeatureFlagInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	flag := models.FeatureFlag{
		Key:         strings.TrimSpace(body.Key),
		Label:       strings.TrimSpace(body.Label),
		Description: body.Description,
		IsEnabled:   body.IsEnabled,
	}

	if err := initializers.DB.Create(&flag).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create feature flag — key may already exist", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "feature_flag.create", "FeatureFlag", &flag.ID, gin.H{"key": flag.Key, "isEnabled": flag.IsEnabled})
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Feature flag created", "data": flag})
}

type UpdateFeatureFlagInput struct {
	Label       *string `json:"label"`
	Description *string `json:"description"`
	IsEnabled   *bool   `json:"isEnabled"`
}

func UpdateFeatureFlag(c *gin.Context) {
	flagID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid feature flag ID"})
		return
	}

	var body UpdateFeatureFlagInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var flag models.FeatureFlag
	if err := initializers.DB.First(&flag, "id = ?", flagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Feature flag not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	updates := map[string]any{}
	if body.Label != nil {
		updates["label"] = *body.Label
	}
	if body.Description != nil {
		updates["description"] = *body.Description
	}
	if body.IsEnabled != nil {
		updates["is_enabled"] = *body.IsEnabled
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "No fields provided for update"})
		return
	}

	if err := initializers.DB.Model(&flag).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update feature flag", "error": err.Error()})
		return
	}

	if body.IsEnabled != nil {
		utils.LogAudit(c, "feature_flag.toggle", "FeatureFlag", &flag.ID, gin.H{"key": flag.Key, "isEnabled": *body.IsEnabled})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Feature flag updated", "data": flag})
}
