package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListSettings(c *gin.Context) {
	var settings []models.GlobalSetting
	if err := initializers.DB.Order("key ASC").Find(&settings).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch settings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

type UpsertSettingInput struct {
	Value       string `json:"value"`
	ValueType   string `json:"valueType"`
	Description string `json:"description"`
}

// UpsertSetting creates or updates a single key. Settings are intentionally
// managed one key at a time (not bulk) so every change is individually audited.
func UpsertSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Setting key is required"})
		return
	}

	var body UpsertSettingInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}
	if body.ValueType == "" {
		body.ValueType = "string"
	}

	var setting models.GlobalSetting
	err := initializers.DB.Where("key = ?", key).First(&setting).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if err == gorm.ErrRecordNotFound {
		setting = models.GlobalSetting{Key: key, Value: body.Value, ValueType: body.ValueType, Description: body.Description}
		if err := initializers.DB.Create(&setting).Error; err != nil {
			RespondError(c, http.StatusInternalServerError, "Failed to create setting", err)
			return
		}
	} else {
		if err := initializers.DB.Model(&setting).Updates(map[string]any{
			"value":       body.Value,
			"value_type":  body.ValueType,
			"description": body.Description,
		}).Error; err != nil {
			RespondError(c, http.StatusInternalServerError, "Failed to update setting", err)
			return
		}
		setting.Value = body.Value
		setting.ValueType = body.ValueType
		setting.Description = body.Description
	}

	utils.LogAudit(c, "setting.update", "GlobalSetting", &setting.ID, gin.H{"key": key, "value": body.Value})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Setting saved", "data": setting})
}

// DeleteSetting removes a global_settings row outright — no soft delete, per
// panel convention. There's no cache or gate reading this table (unlike
// feature flags), so removing a key just makes it absent until re-created.
func DeleteSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Setting key is required"})
		return
	}

	var setting models.GlobalSetting
	if err := initializers.DB.Where("key = ?", key).First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Setting not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	if err := initializers.DB.Delete(&setting).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to delete setting", err)
		return
	}

	utils.LogAudit(c, "setting.delete", "GlobalSetting", &setting.ID, gin.H{"key": key})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Setting deleted"})
}
