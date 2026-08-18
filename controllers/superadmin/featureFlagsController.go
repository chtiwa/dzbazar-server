package superadmin

import (
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

func ListFeatureFlags(c *gin.Context) {
	var flags []models.FeatureFlag
	if err := initializers.DB.Order("key ASC").Find(&flags).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch feature flags", err)
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
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	flag := models.FeatureFlag{
		Key:         strings.TrimSpace(body.Key),
		Label:       strings.TrimSpace(body.Label),
		Description: body.Description,
		IsEnabled:   body.IsEnabled,
	}

	if err := initializers.DB.Create(&flag).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to create feature flag — key may already exist", err)
		return
	}

	services.SetFeatureFlagCache(flag.Key, flag.IsEnabled)
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
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	var flag models.FeatureFlag
	if err := initializers.DB.First(&flag, "id = ?", flagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Feature flag not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
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
		RespondError(c, http.StatusInternalServerError, "Failed to update feature flag", err)
		return
	}

	if body.IsEnabled != nil {
		services.SetFeatureFlagCache(flag.Key, *body.IsEnabled)
		utils.LogAudit(c, "feature_flag.toggle", "FeatureFlag", &flag.ID, gin.H{"key": flag.Key, "isEnabled": *body.IsEnabled})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Feature flag updated", "data": flag})
}

// DeleteFeatureFlag removes a flag row outright — no soft delete, per panel
// convention. Safe to allow unconditionally: IsFeatureEnabled (services/featureFlags.go)
// fails OPEN on a missing key (defaults enabled=true), so deleting a flag —
// including a live kill-switch like coupons_enabled — can never leave a gate
// stuck closed. It does mean the gate silently becomes a permanent "on" with
// no record of the toggle it used to represent, which is why this stays
// super_admin-only rather than being handed to support.
func DeleteFeatureFlag(c *gin.Context) {
	flagID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid feature flag ID"})
		return
	}

	var flag models.FeatureFlag
	if err := initializers.DB.First(&flag, "id = ?", flagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Feature flag not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if err := initializers.DB.Delete(&flag).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to delete feature flag", err)
		return
	}

	utils.LogAudit(c, "feature_flag.delete", "FeatureFlag", &flag.ID, gin.H{"key": flag.Key})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Feature flag deleted"})
}
